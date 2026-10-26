package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/logstore"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/tlsutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// AgentServer implements the AgentService gRPC server that runs on the agent.
// It accepts connections from the HitStor server for registration, filesystem
// browsing, and configuration.
type AgentServer struct {
	pb.UnimplementedAgentServiceServer

	token      string // 64-char hex token for authentication
	envPath    string // path to agent.env (for writing registration config)
	configPath string // path to agent-config.yaml
	version    string

	mu     sync.Mutex
	server *grpc.Server

	// OnRegister is called after a successful Register RPC with the server
	// address, API key, and CA cert path. The daemon uses this to start the backup loop.
	OnRegister func(serverAddr, apiKey, caCertPath string)

	// WakeUpCh is signaled when the server calls WakeUp, so the daemon loop
	// can immediately check its schedule instead of waiting for the next poll.
	WakeUpCh chan struct{}

	// Media overrides where bare-metal recovery media is built. The zero
	// value uses the packaged locations.
	Media MediaOptions

	// OnRestoreFinished is called when a RestoreData stream reaches a
	// terminal state. Recovery mode uses it as its completion signal: the
	// process serving the restore already knows when the restore is done, so
	// it needs no separate heartbeat or poll channel.
	OnRestoreFinished func(RestoreOutcome)
}

// RestoreOutcome summarizes one completed RestoreData stream.
type RestoreOutcome struct {
	FilesRestored uint64
	BytesWritten  uint64
	// Errors are non-fatal problems (a permission denied on one file, say).
	// A restore can finish with some, and whether that is acceptable is the
	// caller's judgement, not the stream's.
	Errors []string
	// Err is set when the stream itself failed, as opposed to individual
	// files inside a stream that otherwise completed.
	Err error
}

// AgentServerOptions configures the agent-side gRPC server.
type AgentServerOptions struct {
	Token      string // 64-char hex agent token
	EnvPath    string // path to agent.env
	ConfigPath string // path to agent-config.yaml
	Version    string // agent binary version
}

// NewAgentServer creates a new agent-side gRPC server.
func NewAgentServer(opts AgentServerOptions) *AgentServer {
	return &AgentServer{
		token:      opts.Token,
		envPath:    opts.EnvPath,
		configPath: opts.ConfigPath,
		version:    opts.Version,
		WakeUpCh:   make(chan struct{}, 1),
	}
}

// Start listens on the given port with self-signed TLS.
func (s *AgentServer) Start(port int) error {
	tlsCfg, _, _, err := tlsutil.GenerateSelfSigned("hitstor-agent")
	if err != nil {
		return fmt.Errorf("agent server: generate TLS: %w", err)
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("agent server: listen on port %d: %w", port, err)
	}

	s.server = grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsCfg)))
	pb.RegisterAgentServiceServer(s.server, s)

	slog.Info("agent gRPC server listening", "port", port)
	return s.server.Serve(lis)
}

// Stop gracefully shuts down the gRPC server.
func (s *AgentServer) Stop() {
	if s.server != nil {
		s.server.GracefulStop()
	}
}

// verifyToken checks the token in a request. Returns an error if invalid.
func (s *AgentServer) verifyToken(provided string) error {
	if !VerifyToken(s.token, provided) {
		return fmt.Errorf("invalid agent token")
	}
	return nil
}

// Register implements AgentService.Register. The server sends an API key and
// its address; the agent stores them and starts pushing backups.
func (s *AgentServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if err := s.verifyToken(req.Token); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Write server address and API key to agent.env.
	envContent := fmt.Sprintf("HITSTOR_AGENT_SERVER=%s\nHITSTOR_AGENT_API_KEY=%s\n", req.ServerAddr, req.ApiKey)

	// Write server CA cert if provided, and include its path in agent.env.
	var caCertPath string
	if len(req.ServerCaCert) > 0 {
		caCertPath = filepath.Join(filepath.Dir(s.envPath), "server-ca.crt")
		if err := os.MkdirAll(filepath.Dir(s.envPath), 0755); err != nil {
			return nil, fmt.Errorf("create env dir: %w", err)
		}
		if err := os.WriteFile(caCertPath, req.ServerCaCert, 0644); err != nil {
			slog.Warn("failed to write server CA cert", "error", err)
			caCertPath = ""
		} else {
			envContent += fmt.Sprintf("HITSTOR_AGENT_CA_CERT=%s\n", caCertPath)
		}
	}

	if err := os.MkdirAll(filepath.Dir(s.envPath), 0755); err != nil {
		return nil, fmt.Errorf("create env dir: %w", err)
	}
	if err := os.WriteFile(s.envPath, []byte(envContent), 0600); err != nil {
		return nil, fmt.Errorf("write agent.env: %w", err)
	}

	slog.Info("agent registered with server", "server", req.ServerAddr)

	// Notify daemon to start backup loop.
	if s.OnRegister != nil {
		go s.OnRegister(req.ServerAddr, req.ApiKey, caCertPath)
	}

	hostname, _ := os.Hostname()
	return &pb.RegisterResponse{
		Hostname: hostname,
		Version:  s.version,
		Os:       runtime.GOOS,
		Arch:     runtime.GOARCH,
	}, nil
}

// ListDirectory implements AgentService.ListDirectory.
func (s *AgentServer) ListDirectory(ctx context.Context, req *pb.ListDirectoryRequest) (*pb.ListDirectoryResponse, error) {
	if err := s.verifyToken(req.Token); err != nil {
		return nil, err
	}

	path := req.Path
	if path == "" {
		// Return root-level directories.
		return s.listRootDirs()
	}

	// Clean and validate the path.
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("path must be absolute")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var result []*pb.DirEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}

		absPath := filepath.Join(path, e.Name())
		entry := &pb.DirEntry{
			Name:     e.Name(),
			Path:     absPath,
			IsDir:    e.IsDir(),
			Size:     info.Size(),
			Mode:     uint32(info.Mode()),
			Readable: isReadable(absPath),
		}

		if e.IsDir() {
			entry.HasChildren = hasSubdirs(absPath)
		}

		result = append(result, entry)
	}

	// Sort: directories first, then files, alphabetical within each group.
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return &pb.ListDirectoryResponse{Entries: result}, nil
}

// Configure implements AgentService.Configure.
func (s *AgentServer) Configure(ctx context.Context, req *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	if err := s.verifyToken(req.Token); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Load current config, update paths and excludes, save.
	cfg, _ := config.Load(s.configPath)
	cfg.AgentPaths = req.BackupPaths
	cfg.AgentExcludes = req.Excludes
	cfg.Path = s.configPath

	if err := config.Save(cfg); err != nil {
		return &pb.ConfigureResponse{
			Success: false,
			Message: fmt.Sprintf("save config: %v", err),
		}, nil
	}

	slog.Info("agent backup paths configured", "paths", req.BackupPaths)
	return &pb.ConfigureResponse{
		Success: true,
		Message: "configuration updated",
	}, nil
}

// AgentInfo implements AgentService.AgentInfo.
// Token verification is skipped when token is empty, allowing unauthenticated
// discovery probes to retrieve agent metadata (hostname, OS, version).
func (s *AgentServer) AgentInfo(ctx context.Context, req *pb.AgentInfoRequest) (*pb.AgentInfoResponse, error) {
	if req.Token != "" {
		if err := s.verifyToken(req.Token); err != nil {
			return nil, err
		}
	}

	hostname, _ := os.Hostname()

	cfg, _ := config.Load(s.configPath)
	registered := cfg.AgentServer != ""

	return &pb.AgentInfoResponse{
		Hostname:    hostname,
		Version:     s.version,
		Os:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		BackupPaths: cfg.AgentPaths,
		Excludes:    cfg.AgentExcludes,
		Registered:  registered,
		ServerAddr:  cfg.AgentServer,
	}, nil
}

// WakeUp implements AgentService.WakeUp. Signals the daemon loop to check
// its schedule immediately instead of waiting for the next poll interval.
func (s *AgentServer) WakeUp(ctx context.Context, req *pb.WakeUpRequest) (*pb.WakeUpResponse, error) {
	if err := s.verifyToken(req.Token); err != nil {
		return nil, err
	}
	select {
	case s.WakeUpCh <- struct{}{}:
		slog.Info("wakeup signal received from server")
	default:
		// Already pending, skip.
	}
	return &pb.WakeUpResponse{Acknowledged: true}, nil
}

// FetchLogs implements AgentService.FetchLogs.
func (s *AgentServer) FetchLogs(ctx context.Context, req *pb.FetchLogsRequest) (*pb.FetchLogsResponse, error) {
	if err := s.verifyToken(req.Token); err != nil {
		return nil, err
	}
	_ = ctx
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 500
	}
	if limit > 1000 {
		limit = 1000
	}
	entries := logstore.Query(logstore.Filter{
		Since: parseAgentLogSince(req.Since),
		Level: strings.TrimSpace(req.Level),
		Query: strings.TrimSpace(req.Query),
		Limit: limit,
	})
	out := make([]*pb.LogEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, &pb.LogEntry{
			Time:      e.Time.UTC().Format(time.RFC3339Nano),
			Level:     e.Level,
			Source:    e.Source,
			Component: e.Component,
			Message:   e.Message,
			Fields:    e.Fields,
		})
	}
	return &pb.FetchLogsResponse{Entries: out}, nil
}

func parseAgentLogSince(s string) time.Time {
	switch strings.TrimSpace(s) {
	case "15m":
		return time.Now().UTC().Add(-15 * time.Minute)
	case "1h":
		return time.Now().UTC().Add(-1 * time.Hour)
	case "24h", "1d":
		return time.Now().UTC().Add(-24 * time.Hour)
	case "7d":
		return time.Now().UTC().Add(-7 * 24 * time.Hour)
	default:
		return time.Time{}
	}
}

// RestoreData implements AgentService.RestoreData. The server streams file
// data to the agent; the agent writes files to disk and responds with a summary.
func (s *AgentServer) RestoreData(stream pb.AgentService_RestoreDataServer) error {
	// First message must be RestoreBegin.
	first, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "agent: restore: receive begin: %v", err)
	}
	begin := first.GetBegin()
	if begin == nil {
		return status.Errorf(codes.InvalidArgument, "agent: restore: first message must be RestoreBegin")
	}
	if err := s.verifyToken(begin.Token); err != nil {
		return status.Errorf(codes.Unauthenticated, "agent: restore: %v", err)
	}

	targetPath := begin.TargetPath
	slog.Info("restore started",
		"snapshot", begin.SnapshotId,
		"target", targetPath,
		"files_total", begin.FilesTotal,
		"bytes_total", begin.BytesTotal,
	)

	type openFile struct {
		file    *os.File
		path    string
		mode    os.FileMode
		uid     uint32
		gid     uint32
		mtimeNs int64
		atimeNs int64
	}

	var (
		filesRestored uint64
		bytesWritten  uint64
		restoreErrors []string
		current       *openFile
		dirs          []dirTimestamp
	)

	// finishFile closes the current file and applies metadata.
	finishFile := func() {
		if current == nil {
			return
		}
		current.file.Close()
		if err := os.Chmod(current.path, current.mode); err != nil {
			slog.Warn("restore: chmod file", "path", current.path, "error", err)
		}
		setOwnership(current.path, current.uid, current.gid)
		mtime := time.Unix(0, current.mtimeNs)
		atime := time.Unix(0, current.atimeNs)
		if err := os.Chtimes(current.path, atime, mtime); err != nil {
			slog.Warn("restore: chtimes file", "path", current.path, "error", err)
		}
		filesRestored++
		current = nil
	}

	defer func() {
		if current != nil {
			current.file.Close()
		}
	}()

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			// A stream that dies partway is a failed restore, not a pause.
			// Recovery mode must hear about it: a half-restored disk that
			// then gets a bootloader is worse than an obvious failure.
			streamErr := status.Errorf(codes.Internal, "agent: restore: receive: %v", err)
			s.notifyRestoreFinished(RestoreOutcome{
				FilesRestored: filesRestored,
				BytesWritten:  bytesWritten,
				Errors:        restoreErrors,
				Err:           streamErr,
			})
			return streamErr
		}

		switch p := msg.Payload.(type) {
		case *pb.RestoreDataRequest_Dir:
			finishFile()
			dirPath := p.Dir.Path
			if targetPath != "" {
				dirPath = filepath.Join(targetPath, dirPath)
			}
			if err := os.MkdirAll(dirPath, os.FileMode(p.Dir.Mode)); err != nil {
				restoreErrors = append(restoreErrors, fmt.Sprintf("mkdir %s: %v", dirPath, err))
				continue
			}
			setOwnership(dirPath, p.Dir.Uid, p.Dir.Gid)
			dirs = append(dirs, dirTimestamp{
				path:    dirPath,
				mode:    os.FileMode(p.Dir.Mode),
				mtimeNs: p.Dir.MtimeNs,
				atimeNs: p.Dir.AtimeNs,
			})

		case *pb.RestoreDataRequest_FileHeader:
			finishFile()
			filePath := p.FileHeader.Path
			if targetPath != "" {
				filePath = filepath.Join(targetPath, filePath)
			}
			if err := os.MkdirAll(filepath.Dir(filePath), 0700); err != nil {
				restoreErrors = append(restoreErrors, fmt.Sprintf("create parent dir %s: %v", filePath, err))
				continue
			}
			f, err := os.Create(filePath)
			if err != nil {
				restoreErrors = append(restoreErrors, fmt.Sprintf("create file %s: %v", filePath, err))
				continue
			}
			current = &openFile{
				file:    f,
				path:    filePath,
				mode:    os.FileMode(p.FileHeader.Mode),
				uid:     p.FileHeader.Uid,
				gid:     p.FileHeader.Gid,
				mtimeNs: p.FileHeader.MtimeNs,
				atimeNs: p.FileHeader.AtimeNs,
			}

		case *pb.RestoreDataRequest_FileChunk:
			if current == nil {
				continue
			}
			n, err := current.file.Write(p.FileChunk.Data)
			bytesWritten += uint64(n)
			if err != nil {
				restoreErrors = append(restoreErrors, fmt.Sprintf("write %s: %v", current.path, err))
				current.file.Close()
				current = nil
				continue
			}

		case *pb.RestoreDataRequest_Symlink:
			finishFile()
			linkPath := p.Symlink.Path
			if targetPath != "" {
				linkPath = filepath.Join(targetPath, linkPath)
			}
			if err := os.MkdirAll(filepath.Dir(linkPath), 0700); err != nil {
				restoreErrors = append(restoreErrors, fmt.Sprintf("create parent dir %s: %v", linkPath, err))
				continue
			}
			os.Remove(linkPath)
			if err := os.Symlink(p.Symlink.Target, linkPath); err != nil {
				restoreErrors = append(restoreErrors, fmt.Sprintf("symlink %s -> %s: %v", linkPath, p.Symlink.Target, err))
				continue
			}
			setOwnership(linkPath, p.Symlink.Uid, p.Symlink.Gid)

		case *pb.RestoreDataRequest_End:
			finishFile()
			goto done
		}
	}

done:
	finishFile()

	// Set directory timestamps in reverse order (deepest first).
	for i := len(dirs) - 1; i >= 0; i-- {
		d := dirs[i]
		mtime := time.Unix(0, d.mtimeNs)
		atime := time.Unix(0, d.atimeNs)
		if err := os.Chtimes(d.path, atime, mtime); err != nil {
			slog.Warn("restore: chtimes dir", "path", d.path, "error", err)
		}
		if err := os.Chmod(d.path, d.mode); err != nil {
			slog.Warn("restore: chmod dir", "path", d.path, "error", err)
		}
	}

	slog.Info("restore complete", "files", filesRestored, "bytes", bytesWritten, "errors", len(restoreErrors))

	sendErr := stream.SendAndClose(&pb.RestoreDataResponse{
		FilesRestored: filesRestored,
		BytesWritten:  bytesWritten,
		Errors:        restoreErrors,
	})
	s.notifyRestoreFinished(RestoreOutcome{
		FilesRestored: filesRestored,
		BytesWritten:  bytesWritten,
		Errors:        restoreErrors,
		Err:           sendErr,
	})
	return sendErr
}

// notifyRestoreFinished reports a completed restore to whoever is waiting on
// it, if anyone is.
func (s *AgentServer) notifyRestoreFinished(outcome RestoreOutcome) {
	if s.OnRestoreFinished != nil {
		s.OnRestoreFinished(outcome)
	}
}

// dirTimestamp stores directory metadata for deferred timestamp setting.
type dirTimestamp struct {
	path    string
	mode    os.FileMode
	mtimeNs int64
	atimeNs int64
}

// DiscoverDatabases is retained on the wire for older clients but is no longer
// supported: HitStor is filesystem-only.
func (s *AgentServer) DiscoverDatabases(ctx context.Context, req *pb.DiscoverDatabasesRequest) (*pb.DiscoverDatabasesResponse, error) {
	if err := s.verifyToken(req.Token); err != nil {
		return nil, err
	}
	return &pb.DiscoverDatabasesResponse{
		Ok:      false,
		Message: "database discovery was removed: HitStor supports filesystem backup only",
	}, nil
}

// isReadable checks if the current process can read a path.
func isReadable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// hasSubdirs checks if a directory contains at least one subdirectory.
func hasSubdirs(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			return true
		}
	}
	return false
}
