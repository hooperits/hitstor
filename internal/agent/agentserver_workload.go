package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hooperits/hitstor/internal/plugin"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
)

// WorkloadPlugin is what the agent server needs from a launched workload
// plugin. *plugin.WorkloadGRPCClient implements it.
type WorkloadPlugin interface {
	Browse(ctx context.Context, path string) ([]plugin.BrowseEntry, error)
	PlanRestore(ctx context.Context, req plugin.PlanRestoreRequest) ([]string, error)
	Restore(ctx context.Context, req plugin.RestoreRequest, items plugin.RestoreItems) (string, error)
}

// WorkloadOpener launches an installed plugin, configured with settings.
// The returned function releases it.
type WorkloadOpener func(id string, settings map[string]string) (WorkloadPlugin, func(), error)

func (s *AgentServer) openWorkload(id string, settings map[string]string) (WorkloadPlugin, func(), error) {
	if s.OpenWorkload == nil {
		return nil, nil, status.Error(codes.Unavailable, "agent: workload plugins are not available on this agent")
	}
	p, release, err := s.OpenWorkload(id, settings)
	if err != nil {
		return nil, nil, status.Errorf(codes.FailedPrecondition, "agent: plugin %s: %v", id, err)
	}
	return p, release, nil
}

func fromNanos(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

// BrowseWorkload implements AgentService.BrowseWorkload.
func (s *AgentServer) BrowseWorkload(ctx context.Context, req *pb.BrowseWorkloadRequest) (*pb.BrowseWorkloadResponse, error) {
	if err := s.verifyToken(req.Token); err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	p, release, err := s.openWorkload(req.Plugin, req.Settings)
	if err != nil {
		return nil, err
	}
	defer release()
	entries, err := p.Browse(ctx, req.Path)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "agent: browse %s: %v", req.Plugin, err)
	}
	resp := &pb.BrowseWorkloadResponse{}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, &pb.WorkloadObject{Path: e.Path, Name: e.Name, Kind: e.Kind, Selectable: e.Selectable, HasChildren: e.HasChildren})
	}
	return resp, nil
}

// PlanWorkloadRestore implements AgentService.PlanWorkloadRestore.
func (s *AgentServer) PlanWorkloadRestore(ctx context.Context, req *pb.PlanWorkloadRestoreRequest) (*pb.PlanWorkloadRestoreResponse, error) {
	if err := s.verifyToken(req.Token); err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	p, release, err := s.openWorkload(req.Plugin, req.Settings)
	if err != nil {
		return nil, err
	}
	defer release()
	pr := plugin.PlanRestoreRequest{Object: req.Object, Target: fromNanos(req.TargetUnixNanos), Options: req.Options}
	for _, c := range req.Candidates {
		pr.Candidates = append(pr.Candidates, plugin.RestoreCandidate{
			Ref: c.Ref, Item: c.Item, Kind: c.Kind, CreatedAt: fromNanos(c.CreatedUnixNanos), Metadata: c.Metadata,
		})
	}
	refs, err := p.PlanRestore(ctx, pr)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "agent: plan restore with %s: %v", req.Plugin, err)
	}
	return &pb.PlanWorkloadRestoreResponse{Refs: refs}, nil
}

// RestoreWorkload implements AgentService.RestoreWorkload: items streamed by
// the server are piped into the plugin as they arrive.
func (s *AgentServer) RestoreWorkload(stream pb.AgentService_RestoreWorkloadServer) error {
	first, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "agent: workload restore: receive begin: %v", err)
	}
	begin := first.GetBegin()
	if begin == nil {
		return status.Error(codes.InvalidArgument, "agent: workload restore: first message must be begin")
	}
	if err := s.verifyToken(begin.Token); err != nil {
		return status.Error(codes.Unauthenticated, err.Error())
	}
	p, release, err := s.openWorkload(begin.Plugin, begin.Settings)
	if err != nil {
		return err
	}
	defer release()

	slog.Info("workload restore started", "plugin", begin.Plugin, "object", begin.Object, "items", begin.ItemCount)
	start := time.Now()
	msg, err := p.Restore(stream.Context(), plugin.RestoreRequest{
		Object: begin.Object, Target: fromNanos(begin.TargetUnixNanos), Options: begin.Options, ItemCount: int(begin.ItemCount),
	}, &streamedItems{recv: stream.Recv})
	if err != nil {
		slog.Error("workload restore failed", "plugin", begin.Plugin, "object", begin.Object, "error", err)
		return status.Errorf(codes.Internal, "agent: workload restore: %v", err)
	}
	slog.Info("workload restore complete", "plugin", begin.Plugin, "object", begin.Object, "message", msg, "duration", time.Since(start))
	return stream.SendAndClose(&pb.WorkloadRestoreResponse{Message: msg})
}

// streamedItems turns the server's restore stream into plugin.RestoreItems.
type streamedItems struct {
	recv func() (*pb.WorkloadRestoreMessage, error)
	cur  *verifiedItemReader
	done bool
}

func (it *streamedItems) Next() (*plugin.RestoreItem, error) {
	if it.done {
		return nil, io.EOF
	}
	if it.cur != nil {
		if _, err := io.Copy(io.Discard, it.cur); err != nil {
			return nil, err
		}
		it.cur = nil
	}
	msg, err := it.recv()
	if err == io.EOF {
		it.done = true
		return nil, io.EOF
	}
	if err != nil {
		return nil, err
	}
	start := msg.GetStart()
	if start == nil {
		return nil, fmt.Errorf("agent: workload restore: expected item start, got %T", msg.Payload)
	}
	it.cur = &verifiedItemReader{recv: it.recv, item: start.Item, wantSize: start.Size, wantSHA: start.Sha256, h: sha256.New()}
	return &plugin.RestoreItem{Ref: start.Ref, Item: start.Item, Metadata: start.Metadata, Data: it.cur}, nil
}

// verifiedItemReader yields one item's data and, at its end, fails unless
// the size and SHA-256 match the item's manifest. The check runs before the
// plugin sees EOF, so a corrupted item never looks complete.
type verifiedItemReader struct {
	recv     func() (*pb.WorkloadRestoreMessage, error)
	item     string
	wantSize int64
	wantSHA  string
	h        hash.Hash
	size     int64
	buf      []byte
	done     bool
}

func (r *verifiedItemReader) Read(p []byte) (int, error) {
	for len(r.buf) == 0 {
		if r.done {
			return 0, io.EOF
		}
		msg, err := r.recv()
		if err == io.EOF {
			return 0, fmt.Errorf("agent: workload restore: stream ended inside item %s", r.item)
		}
		if err != nil {
			return 0, err
		}
		switch m := msg.Payload.(type) {
		case *pb.WorkloadRestoreMessage_Data:
			r.buf = m.Data
			r.h.Write(m.Data)
			r.size += int64(len(m.Data))
		case *pb.WorkloadRestoreMessage_End:
			if r.wantSHA != "" {
				if got := hex.EncodeToString(r.h.Sum(nil)); got != r.wantSHA || r.size != r.wantSize {
					return 0, fmt.Errorf("agent: workload restore: item %s is corrupt (size %d, sha256 %s; manifest says %d, %s)",
						r.item, r.size, got, r.wantSize, r.wantSHA)
				}
			}
			r.done = true
		default:
			return 0, fmt.Errorf("agent: workload restore: unexpected %T inside an item", msg.Payload)
		}
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}
