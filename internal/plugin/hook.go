package plugin

import (
	"context"
	"fmt"

	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
	"github.com/hooperits/hitstor/internal/hook"
)

// HookPlugin implements goplugin.GRPCPlugin for the hook type.
type HookPlugin struct {
	goplugin.Plugin
	// Impl is the concrete hook.Hook. Set by the plugin binary; nil on the host.
	Impl hook.Hook
}

func (p *HookPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterHookServer(s, &hookGRPCServer{impl: p.Impl})
	return nil
}

func (p *HookPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &HookGRPCClient{client: pb.NewHookClient(c)}, nil
}

// --- Plugin side: gRPC server wrapping hook.Hook ---

// hookGRPCServer wraps a hook.Hook to serve it as gRPC.
// Runs inside the plugin binary.
type hookGRPCServer struct {
	pb.UnimplementedHookServer
	impl hook.Hook
}

func (s *hookGRPCServer) Configure(ctx context.Context, req *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	if c, ok := s.impl.(Configurable); ok {
		if err := c.Configure(req.Settings); err != nil {
			return nil, err
		}
	}
	return &pb.ConfigureResponse{}, nil
}

func (s *hookGRPCServer) PreBackup(ctx context.Context, req *pb.HookRequest) (*pb.HookResponse, error) {
	evt := protoToHookEvent(req)
	if err := s.impl.PreBackup(ctx, evt); err != nil {
		return nil, err
	}
	return &pb.HookResponse{}, nil
}

func (s *hookGRPCServer) PostBackup(ctx context.Context, req *pb.HookRequest) (*pb.HookResponse, error) {
	evt := protoToHookEvent(req)
	if err := s.impl.PostBackup(ctx, evt); err != nil {
		return nil, err
	}
	return &pb.HookResponse{}, nil
}

func protoToHookEvent(req *pb.HookRequest) hook.HookEvent {
	return hook.HookEvent{
		Agent:      req.Agent,
		Hostname:   req.Hostname,
		Paths:      req.Paths,
		Excludes:   req.Excludes,
		Tags:       req.Tags,
		SnapshotID: req.SnapshotId,
		Details:    req.Details,
	}
}

// --- Host side: gRPC client implementing hook.Hook ---

// HookGRPCClient implements hook.Hook by making gRPC calls
// to a plugin subprocess.
type HookGRPCClient struct {
	client       pb.HookClient
	pluginClient *goplugin.Client
}

var _ hook.Hook = (*HookGRPCClient)(nil)

func hookEventToProto(event hook.HookEvent) *pb.HookRequest {
	return &pb.HookRequest{
		Agent:      event.Agent,
		Hostname:   event.Hostname,
		Paths:      event.Paths,
		Excludes:   event.Excludes,
		Tags:       event.Tags,
		SnapshotId: event.SnapshotID,
		Details:    event.Details,
	}
}

func (c *HookGRPCClient) PreBackup(ctx context.Context, event hook.HookEvent) error {
	_, err := c.client.PreBackup(ctx, hookEventToProto(event))
	if err != nil {
		return fmt.Errorf("plugin: pre_backup: %w", err)
	}
	return nil
}

func (c *HookGRPCClient) PostBackup(ctx context.Context, event hook.HookEvent) error {
	_, err := c.client.PostBackup(ctx, hookEventToProto(event))
	if err != nil {
		return fmt.Errorf("plugin: post_backup: %w", err)
	}
	return nil
}

func (c *HookGRPCClient) Close() error {
	if c.pluginClient != nil {
		c.pluginClient.Kill()
	}
	return nil
}
