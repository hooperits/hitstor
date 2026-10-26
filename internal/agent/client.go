package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// Client wraps the gRPC connection to a HitStor server.
type Client struct {
	conn   *grpc.ClientConn
	backup pb.BackupServiceClient
}

// ClientOptions configures the gRPC connection.
type ClientOptions struct {
	ServerAddr string // host:port
	CACertPath string // optional: path to CA cert for self-signed TLS
	PinPath    string // optional: where the server certificate fingerprint is pinned when CACertPath is empty
}

// NewClient dials the server and returns a connected client.
func NewClient(opts ClientOptions) (*Client, error) {
	var (
		tlsCfg *tls.Config
		err    error
	)

	if opts.CACertPath != "" {
		// User provided a CA cert — use strict verification.
		pem, err := os.ReadFile(opts.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("agent: read CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("agent: failed to parse CA certificate")
		}
		tlsCfg = &tls.Config{RootCAs: pool}
	} else {
		// No CA cert — pin the server's certificate on first use so a
		// man-in-the-middle cannot harvest the agent's API key later.
		// Same pattern as server→agent (agentclient.go).
		tlsCfg, err = serverPinTLS(opts.ServerAddr, opts.PinPath)
		if err != nil {
			return nil, err
		}
	}

	const maxMsgSize = 64 * 1024 * 1024 // 64 MB – matches server limit for incremental backup metadata
	conn, err := grpc.NewClient(
		opts.ServerAddr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxMsgSize),
			grpc.MaxCallSendMsgSize(maxMsgSize),
		),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("agent: dial server: %w", err)
	}

	return &Client{
		conn:   conn,
		backup: pb.NewBackupServiceClient(conn),
	}, nil
}

// NewClientWithConn creates a client from an existing gRPC connection.
// Used for testing with bufconn.
func NewClientWithConn(conn *grpc.ClientConn) *Client {
	return &Client{
		conn:   conn,
		backup: pb.NewBackupServiceClient(conn),
	}
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Ping calls the Ping RPC to check server health.
func (c *Client) Ping(ctx context.Context) (*pb.PingResponse, error) {
	return c.backup.Ping(ctx, &pb.PingRequest{})
}

// BeginSession starts a backup session with the server.
func (c *Client) BeginSession(ctx context.Context, apiKey, hostname string, paths, excludes, tags []string) (*pb.BeginSessionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.backup.BeginSession(ctx, &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: hostname,
		Paths:    paths,
		Excludes: excludes,
		Tags:     tags,
	})
}

const checkBlobsBatchSize = 1000

// CheckBlobs queries the server for which blob IDs are needed.
// It handles batching automatically (1000 IDs per call).
// Returns a set of blob IDs that the server needs (not already stored).
func (c *Client) CheckBlobs(ctx context.Context, sessionID string, blobIDs [][32]byte) (map[[32]byte]bool, error) {
	needed := make(map[[32]byte]bool)

	for i := 0; i < len(blobIDs); i += checkBlobsBatchSize {
		end := i + checkBlobsBatchSize
		if end > len(blobIDs) {
			end = len(blobIDs)
		}
		batch := blobIDs[i:end]

		// Convert to [][]byte for protobuf.
		pbIDs := make([][]byte, len(batch))
		for j, id := range batch {
			idCopy := id
			pbIDs[j] = idCopy[:]
		}

		batchCtx, batchCancel := context.WithTimeout(ctx, 2*time.Minute)
		resp, err := c.backup.CheckBlobs(batchCtx, &pb.CheckBlobsRequest{
			SessionId: sessionID,
			BlobIds:   pbIDs,
		})
		batchCancel()
		if err != nil {
			return nil, fmt.Errorf("agent: check blobs: %w", err)
		}

		for _, raw := range resp.NeededIds {
			if len(raw) == 32 {
				var id [32]byte
				copy(id[:], raw)
				needed[id] = true
			}
		}
	}

	return needed, nil
}

// PreviousFile holds metadata from a previous snapshot for incremental detection.
type PreviousFile struct {
	Path       string
	MtimeNs    int64
	Size       int64
	ContentIDs [][32]byte
}

// ParsePreviousFiles converts proto PreviousFileEntry messages to PreviousFile structs.
func ParsePreviousFiles(resp *pb.BeginSessionResponse) []PreviousFile {
	if len(resp.PreviousFiles) == 0 {
		return nil
	}
	files := make([]PreviousFile, len(resp.PreviousFiles))
	for i, pf := range resp.PreviousFiles {
		contentIDs := make([][32]byte, len(pf.ContentIds))
		for j, raw := range pf.ContentIds {
			if len(raw) == 32 {
				copy(contentIDs[j][:], raw)
			}
		}
		files[i] = PreviousFile{
			Path:       pf.Path,
			MtimeNs:    pf.MtimeNs,
			Size:       pf.Size,
			ContentIDs: contentIDs,
		}
	}
	return files
}

// ChunkData holds a compressed chunk ready for upload.
type ChunkData struct {
	BlobID          [32]byte
	CompressedData  []byte
	UncompressedLen uint32
}

// UploadChunks streams compressed chunks to the server.
func (c *Client) UploadChunks(ctx context.Context, sessionID string, chunks []ChunkData) (*pb.UploadChunksResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	stream, err := c.backup.UploadChunks(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: open upload stream: %w", err)
	}

	for _, chunk := range chunks {
		if err := stream.Send(&pb.UploadChunkMessage{
			SessionId:          sessionID,
			BlobId:             chunk.BlobID[:],
			CompressedData:     chunk.CompressedData,
			UncompressedLength: chunk.UncompressedLen,
		}); err != nil {
			return nil, fmt.Errorf("agent: upload chunk: %w", err)
		}
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return nil, fmt.Errorf("agent: close upload stream: %w", err)
	}

	return resp, nil
}

// JobSchedule holds info about a single backup job from the server.
type JobSchedule struct {
	JobID              string
	Name               string
	Paths              []string
	Excludes           []string
	RepoName           string
	Enabled            bool
	IntervalHours      int
	NextRunUnix        int64
	RunNow             bool
	FullRescanDays     int
	LastFullBackupUnix int64
	CronExpression     string
	Timezone           string
	UseVSS             bool
	JobType            string
}

// ScheduleInfo holds the schedule returned by the server.
type ScheduleInfo struct {
	Enabled       bool
	IntervalHours int
	NextRunUnix   int64
	Jobs          []JobSchedule
}

// GetSchedule queries the server for this agent's backup schedule.
func (c *Client) GetSchedule(ctx context.Context, apiKey string) (*ScheduleInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := c.backup.GetSchedule(ctx, &pb.GetScheduleRequest{
		ApiKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("agent: get schedule: %w", err)
	}

	info := &ScheduleInfo{
		Enabled:       resp.Enabled,
		IntervalHours: int(resp.IntervalHours),
		NextRunUnix:   resp.NextRunUnix,
	}

	for _, j := range resp.Jobs {
		info.Jobs = append(info.Jobs, JobSchedule{
			JobID:              j.JobId,
			Name:               j.Name,
			Paths:              j.Paths,
			Excludes:           j.Excludes,
			RepoName:           j.RepoName,
			Enabled:            j.Enabled,
			IntervalHours:      int(j.IntervalHours),
			NextRunUnix:        j.NextRunUnix,
			RunNow:             j.RunNow,
			FullRescanDays:     int(j.FullRescanDays),
			LastFullBackupUnix: j.LastFullBackupUnix,
			CronExpression:     j.CronExpression,
			Timezone:           j.Timezone,
			UseVSS:             j.UseVss,
			JobType:            j.JobType,
		})
	}

	return info, nil
}

// BeginSessionWithJob starts a backup session linked to a specific backup job.
func (c *Client) BeginSessionWithJob(ctx context.Context, apiKey, hostname string, paths, excludes, tags []string, jobID string) (*pb.BeginSessionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.backup.BeginSession(ctx, &pb.BeginSessionRequest{
		ApiKey:   apiKey,
		Hostname: hostname,
		Paths:    paths,
		Excludes: excludes,
		Tags:     tags,
		JobId:    jobID,
	})
}

// FinalizeBackup sends the file tree and completes the backup session.
//
// systemProfile carries the host's disk and boot layout as JSON for bare-metal
// restore, and is nil when the job does not capture it.
func (c *Client) FinalizeBackup(ctx context.Context, sessionID string, files []*pb.FileNode, systemProfile []byte) (*pb.FinalizeBackupResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return c.backup.FinalizeBackup(ctx, &pb.FinalizeBackupRequest{
		SessionId:     sessionID,
		Files:         files,
		SystemProfile: systemProfile,
	})
}

// AnnounceForRecovery tells the server that this host has booted off
// bare-metal recovery media and is reachable at addr.
//
// Unlike every other call here it authenticates with a single-use recovery
// token rather than an API key: the rescue media carries no credentials, and
// the original host's key lives on a disk that may no longer exist.
func (c *Client) AnnounceForRecovery(ctx context.Context, req *pb.AnnounceForRecoveryRequest) (*pb.AnnounceForRecoveryResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.backup.AnnounceForRecovery(ctx, req)
}
