package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
)

// WorkloadProtocolVersion is the workload protocol revision this host speaks.
const WorkloadProtocolVersion = 1

// WorkloadMetadata identifies a workload plugin and what it can do.
type WorkloadMetadata struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Version         string      `json:"version"`
	ProtocolVersion int32       `json:"protocol_version"`
	Description     string      `json:"description,omitempty"`
	Capabilities    []string    `json:"capabilities,omitempty"`
	Fields          []FieldSpec `json:"fields,omitempty"`
}

// Field scopes.
const (
	FieldScopeSettings = "settings"
	FieldScopeBackup   = "backup"
	FieldScopeRestore  = "restore"
)

// FieldSpec declares one form field the dashboard renders for a plugin.
// Label and Help are keyed by language code; "en" is the fallback.
type FieldSpec struct {
	Key      string            `json:"key"`
	Scope    string            `json:"scope"`
	Type     string            `json:"type"` // string, bool, number, choice
	Label    map[string]string `json:"label"`
	Help     map[string]string `json:"help,omitempty"`
	Default  string            `json:"default,omitempty"`
	Choices  []string          `json:"choices,omitempty"`
	Required bool              `json:"required,omitempty"`
}

func fieldsToProto(fs []FieldSpec) []*pb.FieldSpec {
	out := make([]*pb.FieldSpec, 0, len(fs))
	for _, f := range fs {
		out = append(out, &pb.FieldSpec{Key: f.Key, Scope: f.Scope, Type: f.Type, Label: f.Label, Help: f.Help,
			Default: f.Default, Choices: f.Choices, Required: f.Required})
	}
	return out
}

func fieldsFromProto(fs []*pb.FieldSpec) []FieldSpec {
	out := make([]FieldSpec, 0, len(fs))
	for _, f := range fs {
		out = append(out, FieldSpec{Key: f.Key, Scope: f.Scope, Type: f.Type, Label: f.Label, Help: f.Help,
			Default: f.Default, Choices: f.Choices, Required: f.Required})
	}
	return out
}

// Workload is implemented by workload plugin binaries. Plugins may also
// implement Configurable to receive settings.
type Workload interface {
	Metadata(ctx context.Context) (WorkloadMetadata, error)
}

// WorkloadPlugin implements goplugin.GRPCPlugin for the workload type.
type WorkloadPlugin struct {
	goplugin.Plugin
	// Impl is the concrete Workload. Set by the plugin binary; nil on the host.
	Impl Workload
}

func (p *WorkloadPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterWorkloadServer(s, &workloadGRPCServer{impl: p.Impl})
	return nil
}

func (p *WorkloadPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &WorkloadGRPCClient{client: pb.NewWorkloadClient(c)}, nil
}

// --- Plugin side: gRPC server wrapping Workload ---

// workloadGRPCServer wraps a Workload to serve it as gRPC.
// Runs inside the plugin binary.
type workloadGRPCServer struct {
	pb.UnimplementedWorkloadServer
	impl Workload
}

func (s *workloadGRPCServer) GetMetadata(ctx context.Context, _ *pb.GetMetadataRequest) (*pb.WorkloadMetadata, error) {
	m, err := s.impl.Metadata(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.WorkloadMetadata{
		Id:              m.ID,
		Name:            m.Name,
		Version:         m.Version,
		ProtocolVersion: m.ProtocolVersion,
		Description:     m.Description,
		Capabilities:    m.Capabilities,
		Fields:          fieldsToProto(m.Fields),
	}, nil
}

func (s *workloadGRPCServer) Configure(ctx context.Context, req *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	if c, ok := s.impl.(Configurable); ok {
		if err := c.Configure(req.Settings); err != nil {
			return nil, err
		}
	}
	return &pb.ConfigureResponse{}, nil
}

// --- Host side: gRPC client ---

// WorkloadGRPCClient talks to a workload plugin subprocess.
type WorkloadGRPCClient struct {
	client       pb.WorkloadClient
	pluginClient *goplugin.Client
}

// Metadata asks the plugin to describe itself.
func (c *WorkloadGRPCClient) Metadata(ctx context.Context) (WorkloadMetadata, error) {
	m, err := c.client.GetMetadata(ctx, &pb.GetMetadataRequest{})
	if err != nil {
		return WorkloadMetadata{}, fmt.Errorf("plugin: get metadata: %w", err)
	}
	return WorkloadMetadata{
		ID:              m.Id,
		Name:            m.Name,
		Version:         m.Version,
		ProtocolVersion: m.ProtocolVersion,
		Description:     m.Description,
		Capabilities:    m.Capabilities,
		Fields:          fieldsFromProto(m.Fields),
	}, nil
}

// Configure passes settings to the plugin.
func (c *WorkloadGRPCClient) Configure(ctx context.Context, settings map[string]string) error {
	if _, err := c.client.Configure(ctx, &pb.ConfigureRequest{Settings: settings}); err != nil {
		return fmt.Errorf("plugin: configure: %w", err)
	}
	return nil
}

// Close kills the plugin subprocess.
func (c *WorkloadGRPCClient) Close() error {
	if c.pluginClient != nil {
		c.pluginClient.Kill()
	}
	return nil
}

// OpenWorkload launches the workload plugin binary at binaryPath.
//
// Unlike the other plugin types, workload plugins are not discovered on
// $PATH: the agent's plugin manager resolves the binary of the installed,
// verified package and passes its absolute path here.
//
// The returned client must be closed by the caller, which kills the plugin
// subprocess.
func OpenWorkload(binaryPath string) (*WorkloadGRPCClient, error) {
	slog.Info("launching workload plugin", "path", binaryPath)

	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			WorkloadPluginName: &WorkloadPlugin{},
		},
		Cmd:              exec.Command(binaryPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		// go-plugin's default logger traces every handshake step to stderr,
		// which would bury CLI output. Keep only real problems.
		Logger: hclog.New(&hclog.LoggerOptions{Name: "plugin", Output: os.Stderr, Level: hclog.Warn}),
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("plugin: connect to %s: %w", binaryPath, err)
	}

	raw, err := rpcClient.Dispense(WorkloadPluginName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("plugin: dispense %s: %w", binaryPath, err)
	}

	grpcClient, ok := raw.(*WorkloadGRPCClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin: %s returned unexpected type %T", binaryPath, raw)
	}
	grpcClient.pluginClient = client
	return grpcClient, nil
}
