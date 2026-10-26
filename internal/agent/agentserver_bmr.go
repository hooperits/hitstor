package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hooperits/hitstor/internal/bmr"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mediaChunkSize is how much ISO the agent puts in one stream message. Well
// under the default 4 MiB gRPC message limit, and large enough that a
// multi-gigabyte image does not become millions of round trips.
const mediaChunkSize = 1 << 20 // 1 MiB

// MediaOptions overrides where recovery media is built and which ReaR builds
// it. The zero value uses the packaged locations, which is what a normal
// install wants; distributions that lay files out differently, and tests, set
// these explicitly.
type MediaOptions struct {
	BuildDir   string
	ConfigPath string
	HookPath   string
	ReaRPath   string
	// SkipCheck builds even when the readiness check reports blockers.
	SkipCheck bool
}

// CheckRecoveryMedia implements AgentService.CheckRecoveryMedia.
func (s *AgentServer) CheckRecoveryMedia(ctx context.Context, req *pb.CheckRecoveryMediaRequest) (*pb.CheckRecoveryMediaResponse, error) {
	if err := s.verifyToken(req.Token); err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "agent: recovery media check: %v", err)
	}

	r := bmr.Check(ctx)
	resp := &pb.CheckRecoveryMediaResponse{
		Ready:         r.Ready,
		Platform:      r.Platform,
		RearInstalled: r.ReaRInstalled,
		RearVersion:   r.ReaRVersion,
		RearPath:      r.ReaRPath,
		Root:          r.Root,
		Blockers:      r.Blockers,
		Warnings:      r.Warnings,
		CheckedAtUnix: r.CheckedAt.Unix(),
	}
	for _, tool := range r.Tools {
		resp.Tools = append(resp.Tools, &pb.ToolStatus{
			Name:     tool.Name,
			Path:     tool.Path,
			Required: tool.Required,
			Found:    tool.Found,
			Purpose:  tool.Purpose,
		})
	}
	return resp, nil
}

// BuildRecoveryMedia implements AgentService.BuildRecoveryMedia: it runs ReaR
// on this host and streams the resulting ISO to the server.
//
// Progress lines and image bytes share one stream. The build takes minutes, so
// the server needs to show it happening; the image is far too large to buffer,
// so it is read back in chunks once ReaR has written it.
func (s *AgentServer) BuildRecoveryMedia(req *pb.BuildRecoveryMediaRequest, stream pb.AgentService_BuildRecoveryMediaServer) error {
	if err := s.verifyToken(req.Token); err != nil {
		return status.Errorf(codes.Unauthenticated, "agent: build recovery media: %v", err)
	}

	cfg := bmr.ConfigOptions{
		OutputDir:     s.Media.BuildDir,
		HookScript:    s.Media.HookPath,
		ServerAddr:    req.ServerAddr,
		AgentName:     req.AgentName,
		MigrationMode: req.MigrationMode,
	}
	// The rescue image must carry the very binary serving this call: whatever
	// the packager installed, wherever they put it.
	if self, err := os.Executable(); err == nil {
		cfg.AgentBinary = self
	}

	slog.Info("building bare-metal recovery media",
		"migration_mode", req.MigrationMode, "output_dir", cfg.OutputDir)

	// gRPC streams are not safe for concurrent Send. BuildMedia serializes its
	// progress callbacks, and the image is streamed only after it returns, so
	// every Send below happens on one goroutine at a time.
	sendProgress := func(line string) {
		_ = stream.Send(&pb.MediaChunk{
			Payload: &pb.MediaChunk_Progress{Progress: line},
		})
	}

	media, err := bmr.BuildMedia(stream.Context(), bmr.BuildOptions{
		Config:     cfg,
		ConfigPath: s.Media.ConfigPath,
		ReaRPath:   s.Media.ReaRPath,
		SkipCheck:  s.Media.SkipCheck,
		OnProgress: sendProgress,
	})
	if err != nil {
		var notReady *bmr.NotReadyError
		if errors.As(err, &notReady) {
			return status.Error(codes.FailedPrecondition, err.Error())
		}
		return status.Error(codes.Internal, err.Error())
	}

	if err := streamISO(stream, media); err != nil {
		return err
	}

	slog.Info("recovery media built and streamed",
		"iso", media.ISOPath, "size_bytes", media.SizeBytes)
	return nil
}

// streamISO sends the built image, then a completion message carrying the size
// so the receiver can tell a truncated transfer from a finished one.
func streamISO(stream pb.AgentService_BuildRecoveryMediaServer, media bmr.Media) error {
	f, err := os.Open(media.ISOPath)
	if err != nil {
		return status.Errorf(codes.Internal, "agent: open recovery media: %v", err)
	}
	defer f.Close()

	buf := make([]byte, mediaChunkSize)
	var sent int64
	for {
		if err := stream.Context().Err(); err != nil {
			return status.FromContextError(err).Err()
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			if err := stream.Send(&pb.MediaChunk{
				Payload: &pb.MediaChunk_Data{Data: buf[:n]},
			}); err != nil {
				return fmt.Errorf("agent: send recovery media: %w", err)
			}
			sent += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return status.Errorf(codes.Internal, "agent: read recovery media: %v", readErr)
		}
	}

	return stream.Send(&pb.MediaChunk{
		Payload: &pb.MediaChunk_Complete{Complete: &pb.MediaComplete{
			SizeBytes:   uint64(sent),
			Filename:    filepath.Base(media.ISOPath),
			BuiltAtUnix: media.BuiltAt.Unix(),
		}},
	})
}
