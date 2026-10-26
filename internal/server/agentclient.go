package server

import (
	"context"
	"fmt"
	"io"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/tlsutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// AgentClient connects to an agent's gRPC management server.
type AgentClient struct {
	conn  *grpc.ClientConn
	agent pb.AgentServiceClient
}

// DialAgent connects to an agent's gRPC server at the given address without
// certificate pinning. Used for pre-registration probes (discovery), where no
// pin exists yet; the agent token provides authentication. Registered agents
// should be dialed with DialAgentPinned instead.
func DialAgent(addr string) (*AgentClient, error) {
	return DialAgentPinned(addr, "", nil)
}

// DialAgentPinned connects to an agent's gRPC server using trust-on-first-use
// certificate pinning. Agents use self-signed certificates, so chain
// verification is replaced by a fingerprint check: when pin is non-empty the
// agent's certificate must match it; when empty, the observed fingerprint is
// passed to record (if non-nil) so the caller can persist it.
func DialAgentPinned(addr, pin string, record func(fingerprint string)) (*AgentClient, error) {
	tlsCfg := tlsutil.PinnedClientTLS(pin, record)
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
	)
	if err != nil {
		return nil, fmt.Errorf("dial agent %s: %w", addr, err)
	}
	return &AgentClient{
		conn:  conn,
		agent: pb.NewAgentServiceClient(conn),
	}, nil
}

// DialAgentWith connects to an agent using caller-supplied dial options
// instead of the default pinned-TLS transport. It exists for callers that
// reach an agent over something other than a TCP socket with a self-signed
// certificate — an in-memory listener in tests, a tunnelled connection.
func DialAgentWith(addr string, opts ...grpc.DialOption) (*AgentClient, error) {
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial agent %s: %w", addr, err)
	}
	return &AgentClient{
		conn:  conn,
		agent: pb.NewAgentServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection.
func (c *AgentClient) Close() error {
	return c.conn.Close()
}

// Register sends the API key and server address to the agent.
func (c *AgentClient) Register(ctx context.Context, token, apiKey, serverAddr string, caCert []byte) (*pb.RegisterResponse, error) {
	return c.agent.Register(ctx, &pb.RegisterRequest{
		Token:        token,
		ApiKey:       apiKey,
		ServerAddr:   serverAddr,
		ServerCaCert: caCert,
	})
}

// ListDirectory queries a directory listing from the agent.
func (c *AgentClient) ListDirectory(ctx context.Context, token, path string) ([]*pb.DirEntry, error) {
	resp, err := c.agent.ListDirectory(ctx, &pb.ListDirectoryRequest{
		Token: token,
		Path:  path,
	})
	if err != nil {
		return nil, err
	}
	return resp.Entries, nil
}

// Configure sends backup paths and excludes to the agent.
func (c *AgentClient) Configure(ctx context.Context, token string, paths, excludes []string) error {
	resp, err := c.agent.Configure(ctx, &pb.ConfigureRequest{
		Token:       token,
		BackupPaths: paths,
		Excludes:    excludes,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("configure agent: %s", resp.Message)
	}
	return nil
}

// AgentInfo queries the agent's current status.
func (c *AgentClient) AgentInfo(ctx context.Context, token string) (*pb.AgentInfoResponse, error) {
	return c.agent.AgentInfo(ctx, &pb.AgentInfoRequest{Token: token})
}

// AgentInfoNoAuth queries agent metadata without token authentication.
// Used by network discovery to probe agents before registration.
func (c *AgentClient) AgentInfoNoAuth(ctx context.Context) (*pb.AgentInfoResponse, error) {
	return c.agent.AgentInfo(ctx, &pb.AgentInfoRequest{})
}

// RestoreData opens a client-streaming RPC for sending restore data to the agent.
// The caller sends RestoreDataRequest messages and closes with CloseAndRecv.
func (c *AgentClient) RestoreData(ctx context.Context) (pb.AgentService_RestoreDataClient, error) {
	return c.agent.RestoreData(ctx)
}

// WakeUp signals the agent to immediately check its schedule.
// Used when "Backup Now" is triggered from the dashboard.
func (c *AgentClient) WakeUp(ctx context.Context, token string) error {
	_, err := c.agent.WakeUp(ctx, &pb.WakeUpRequest{Token: token})
	return err
}

// FetchLogs returns recent log entries from the agent.
func (c *AgentClient) FetchLogs(ctx context.Context, token, since, level, query string, limit int32) (*pb.FetchLogsResponse, error) {
	return c.agent.FetchLogs(ctx, &pb.FetchLogsRequest{
		Token: token,
		Since: since,
		Level: level,
		Query: query,
		Limit: limit,
	})
}

// DiscoverDatabases tests connectivity from the agent and lists databases.
func (c *AgentClient) DiscoverDatabases(ctx context.Context, token string, req *pb.DiscoverDatabasesRequest) (*pb.DiscoverDatabasesResponse, error) {
	req.Token = token
	return c.agent.DiscoverDatabases(ctx, req)
}

// CheckRecoveryMedia asks the agent whether its host can build bare-metal
// recovery media.
func (c *AgentClient) CheckRecoveryMedia(ctx context.Context, token string) (*pb.CheckRecoveryMediaResponse, error) {
	return c.agent.CheckRecoveryMedia(ctx, &pb.CheckRecoveryMediaRequest{Token: token})
}

// BuildRecoveryMediaRequest describes one media build.
type BuildRecoveryMediaRequest struct {
	Token string
	// ServerAddr is prefilled into the rescue system's recovery prompt.
	ServerAddr string
	// AgentName is the registered name this media recovers, prefilled into
	// the same prompt.
	AgentName string
	// MigrationMode allows recovery onto dissimilar hardware or a VM.
	MigrationMode bool
	// Force builds even when the agent's readiness check reports blockers.
	Force bool

	// OnProgress receives ReaR's output line by line, and OnBytes the running
	// count of image bytes received. A build takes minutes and the image runs
	// to gigabytes; without both, a caller cannot tell progress from a hang.
	OnProgress func(line string)
	OnBytes    func(received int64)
}

// BuildRecoveryMedia runs a build on the agent and writes the streamed image
// into cache under the given host name.
//
// The image is never held in memory: it lands in the cache's temporary file as
// it arrives, and replaces the host's previous media only once the agent has
// signalled a complete transfer.
func (c *AgentClient) BuildRecoveryMedia(ctx context.Context, cache *MediaCache, host string, req BuildRecoveryMediaRequest) (MediaInfo, error) {
	// Claim the cache slot first. Opening the stream first would leave a
	// build running on the agent with nowhere to put its output if the slot
	// turns out to be taken.
	writer, err := cache.Writer(host)
	if err != nil {
		return MediaInfo{}, err
	}

	stream, err := c.agent.BuildRecoveryMedia(ctx, &pb.BuildRecoveryMediaRequest{
		Token:         req.Token,
		ServerAddr:    req.ServerAddr,
		AgentName:     req.AgentName,
		MigrationMode: req.MigrationMode,
		Force:         req.Force,
	})
	if err != nil {
		_ = writer.Abort()
		return MediaInfo{}, fmt.Errorf("build recovery media on %s: %w", host, err)
	}

	var complete *pb.MediaComplete
	for {
		msg, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			_ = writer.Abort()
			return MediaInfo{}, fmt.Errorf("build recovery media on %s: %w", host, recvErr)
		}

		switch p := msg.Payload.(type) {
		case *pb.MediaChunk_Progress:
			if req.OnProgress != nil {
				req.OnProgress(p.Progress)
			}
		case *pb.MediaChunk_Data:
			if _, err := writer.Write(p.Data); err != nil {
				_ = writer.Abort()
				return MediaInfo{}, fmt.Errorf("cache recovery media for %s: %w", host, err)
			}
			if req.OnBytes != nil {
				req.OnBytes(writer.Written())
			}
		case *pb.MediaChunk_Complete:
			complete = p.Complete
		}
	}

	// A stream that ends without a completion message ended early — a killed
	// agent, a dropped connection. Publishing what arrived would hand an
	// operator a truncated ISO that boots to nothing.
	if complete == nil {
		_ = writer.Abort()
		return MediaInfo{}, fmt.Errorf(
			"build recovery media on %s: stream ended before the image was complete", host)
	}
	if got, want := writer.Written(), int64(complete.SizeBytes); got != want {
		_ = writer.Abort()
		return MediaInfo{}, fmt.Errorf(
			"build recovery media on %s: received %d bytes, agent reported %d", host, got, want)
	}

	return writer.Commit()
}
