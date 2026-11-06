package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hooperits/hitstor/internal/pluginmgr"
	"github.com/hooperits/hitstor/internal/pluginpkg"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
)

// maxPluginPackage bounds a pushed package: the uncompressed limit plus
// generous room for zip overhead.
const maxPluginPackage = pluginpkg.MaxTotalSize + 64<<20

// PluginInstaller installs and removes plugins for the server. The daemon
// wires it to the agent's plugin manager, so remote installs get the same
// signature and compatibility checks as local ones.
type PluginInstaller interface {
	Install(ctx context.Context, pkgPath string, opts pluginmgr.InstallOptions) (*pluginmgr.InstalledPlugin, error)
	Remove(id string) error
}

// InstallPlugin implements AgentService.InstallPlugin.
func (s *AgentServer) InstallPlugin(stream pb.AgentService_InstallPluginServer) error {
	first, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "agent: install plugin: receive begin: %v", err)
	}
	begin := first.GetBegin()
	if begin == nil {
		return status.Error(codes.InvalidArgument, "agent: install plugin: first message must be begin")
	}
	if err := s.verifyToken(begin.Token); err != nil {
		return status.Error(codes.Unauthenticated, err.Error())
	}
	if s.Plugins == nil {
		return status.Error(codes.Unavailable, "agent: plugin installation is not available on this agent")
	}
	if begin.Size <= 0 || begin.Size > maxPluginPackage {
		return status.Errorf(codes.InvalidArgument, "agent: install plugin: invalid package size %d", begin.Size)
	}

	tmp, err := os.CreateTemp("", "hitstor-plugin-*.hpk")
	if err != nil {
		return status.Errorf(codes.Internal, "agent: install plugin: %v", err)
	}
	defer os.Remove(tmp.Name())
	var got int64
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			tmp.Close()
			return status.Errorf(codes.Internal, "agent: install plugin: receive: %v", err)
		}
		data := msg.GetData()
		got += int64(len(data))
		if got > begin.Size {
			tmp.Close()
			return status.Error(codes.InvalidArgument, "agent: install plugin: more data than announced")
		}
		if _, err := tmp.Write(data); err != nil {
			tmp.Close()
			return status.Errorf(codes.Internal, "agent: install plugin: %v", err)
		}
	}
	if err := tmp.Close(); err != nil {
		return status.Errorf(codes.Internal, "agent: install plugin: %v", err)
	}
	if got != begin.Size {
		return status.Errorf(codes.InvalidArgument, "agent: install plugin: received %d of %d bytes", got, begin.Size)
	}

	p, err := s.Plugins.Install(stream.Context(), tmp.Name(), pluginmgr.InstallOptions{Upgrade: begin.Upgrade, AllowDowngrade: begin.AllowDowngrade})
	if err != nil {
		slog.Warn("remote plugin install refused", "package", begin.FileName, "error", err)
		return status.Errorf(codes.FailedPrecondition, "agent: %v", err)
	}
	slog.Info("plugin installed by server", "plugin", p.ID, "version", p.Version, "package", begin.FileName)
	return stream.SendAndClose(&pb.InstallPluginResponse{Id: p.ID, Name: p.Name, Version: p.Version, Status: p.Status, Signer: p.Signer})
}

// RemovePlugin implements AgentService.RemovePlugin.
func (s *AgentServer) RemovePlugin(_ context.Context, req *pb.RemovePluginRequest) (*pb.RemovePluginResponse, error) {
	if err := s.verifyToken(req.Token); err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	if s.Plugins == nil {
		return nil, status.Error(codes.Unavailable, "agent: plugin installation is not available on this agent")
	}
	if err := s.Plugins.Remove(req.Id); err != nil {
		return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("agent: %v", err))
	}
	slog.Info("plugin removed by server", "plugin", req.Id)
	return &pb.RemovePluginResponse{}, nil
}
