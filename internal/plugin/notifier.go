package plugin

import (
	"context"
	"fmt"

	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
	"github.com/hooperits/hitstor/internal/notifier"
)

// NotifierPlugin implements goplugin.GRPCPlugin for the notifier type.
type NotifierPlugin struct {
	goplugin.Plugin
	// Impl is the concrete notifier.Notifier. Set by the plugin binary; nil on the host.
	Impl notifier.Notifier
}

func (p *NotifierPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterNotifierServer(s, &notifierGRPCServer{impl: p.Impl})
	return nil
}

func (p *NotifierPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &NotifierGRPCClient{client: pb.NewNotifierClient(c)}, nil
}

// --- Plugin side: gRPC server wrapping notifier.Notifier ---

// notifierGRPCServer wraps a notifier.Notifier to serve it as gRPC.
// Runs inside the plugin binary.
type notifierGRPCServer struct {
	pb.UnimplementedNotifierServer
	impl notifier.Notifier
}

func (s *notifierGRPCServer) Configure(ctx context.Context, req *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	if c, ok := s.impl.(Configurable); ok {
		if err := c.Configure(req.Settings); err != nil {
			return nil, err
		}
	}
	return &pb.ConfigureResponse{}, nil
}

func (s *notifierGRPCServer) Notify(ctx context.Context, req *pb.NotifyRequest) (*pb.NotifyResponse, error) {
	details := req.Details
	// Convert map[string]string to map[string]any for the Event type.
	var detailsAny map[string]any
	if len(details) > 0 {
		detailsAny = make(map[string]any, len(details))
		for k, v := range details {
			detailsAny[k] = v
		}
	}
	evt := notifier.Event{
		EventType: req.EventType,
		Server:    req.Server,
		Timestamp: req.Timestamp,
		Agent:     req.Agent,
		Hostname:  req.Hostname,
		Message:   req.Message,
		Details:   detailsAny,
	}
	if err := s.impl.Notify(ctx, evt); err != nil {
		return nil, err
	}
	return &pb.NotifyResponse{}, nil
}

// --- Host side: gRPC client implementing notifier.Notifier ---

// NotifierGRPCClient implements notifier.Notifier by making gRPC calls
// to a plugin subprocess.
type NotifierGRPCClient struct {
	client       pb.NotifierClient
	pluginClient *goplugin.Client
}

var _ notifier.Notifier = (*NotifierGRPCClient)(nil)

func (c *NotifierGRPCClient) Notify(ctx context.Context, event notifier.Event) error {
	// Convert map[string]any to map[string]string for the proto.
	var details map[string]string
	if len(event.Details) > 0 {
		details = make(map[string]string, len(event.Details))
		for k, v := range event.Details {
			details[k] = fmt.Sprintf("%v", v)
		}
	}
	_, err := c.client.Notify(ctx, &pb.NotifyRequest{
		EventType: event.EventType,
		Server:    event.Server,
		Timestamp: event.Timestamp,
		Agent:     event.Agent,
		Hostname:  event.Hostname,
		Message:   event.Message,
		Details:   details,
	})
	if err != nil {
		return fmt.Errorf("plugin: notify: %w", err)
	}
	return nil
}

func (c *NotifierGRPCClient) Close() error {
	if c.pluginClient != nil {
		c.pluginClient.Kill()
	}
	return nil
}
