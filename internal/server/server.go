package server

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hooperits/hitstor/internal/hook"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/runlog"
	"github.com/hooperits/hitstor/internal/sentinel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Options configures the backup server.
type Options struct {
	Port           int
	TLSCert        string // path to TLS certificate file
	TLSKey         string // path to TLS private key file
	Version        string // server version string
	SentinelConfig sentinel.Config
}

// Server is the gRPC backup server that receives backups from remote agents.
type Server struct {
	pb.UnimplementedBackupServiceServer

	repos    *RepoManager
	registry *AgentRegistry
	opts     Options

	mu       sync.Mutex
	sessions map[string]*Session

	// Recently completed sessions (kept for 30s so SSE clients can display completion).
	completedMu       sync.Mutex
	completedSessions map[string]completedSession

	// Prometheus counters (cumulative, never reset).
	backupsCompleted atomic.Uint64
	backupsFailed    atomic.Uint64
	bytesReceived    atomic.Uint64
	chunksReceived   atomic.Uint64

	grpcServer *grpc.Server
	tlsConfig  *tls.Config

	// handlersMu guards the optional callbacks below, which may be swapped
	// while RPC goroutines fire notifications.
	handlersMu sync.RWMutex

	// runs records finished backup runs (optional; see SetRunLog).
	runs *runlog.Log

	// onBackupFailed is called when a backup fails (optional).
	onBackupFailed func(agentName, hostname, reason string, details map[string]any)

	// onThreatDetected is called when a ransomware threat is detected (optional).
	onThreatDetected func(agentName, hostname, reason string, confidence float64, details map[string]any)

	// hooks are pre/post-backup lifecycle plugins (optional).
	hooks []hook.Hook

	sentinel *sentinel.Sentinel
	// sentinelFailed is set after a failed check is reported, so repeated
	// failures (no credentials) are not logged as errors after every backup.
	sentinelFailed atomic.Bool

	// background tracks post-backup work (sentinel check, restore
	// verification) so Stop can let it finish before returning.
	background sync.WaitGroup
}

// New creates a new backup server.
func New(repos *RepoManager, registry *AgentRegistry, opts Options) *Server {
	return &Server{
		repos:             repos,
		registry:          registry,
		opts:              opts,
		sessions:          make(map[string]*Session),
		completedSessions: make(map[string]completedSession),
		sentinel:          sentinel.New(opts.SentinelConfig),
	}
}

// Start begins listening for gRPC connections on the configured port.
func (s *Server) Start() error {
	if s.tlsConfig == nil {
		if err := s.LoadTLS(); err != nil {
			return err
		}
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.opts.Port))
	if err != nil {
		return fmt.Errorf("server: listen on port %d: %w", s.opts.Port, err)
	}

	creds := credentials.NewTLS(s.tlsConfig)
	const maxMsgSize = 64 * 1024 * 1024 // 64 MB – accommodates BeginSessionResponse with previous_files for incremental backups
	s.grpcServer = grpc.NewServer(
		grpc.Creds(creds),
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
	)
	pb.RegisterBackupServiceServer(s.grpcServer, s)

	slog.Info("server listening", "port", s.opts.Port)
	return s.grpcServer.Serve(lis)
}

// StartWithListener begins serving on the provided listener (for testing).
func (s *Server) StartWithListener(lis net.Listener) error {
	s.grpcServer = grpc.NewServer()
	pb.RegisterBackupServiceServer(s.grpcServer, s)
	return s.grpcServer.Serve(lis)
}

// InitGRPCServer initializes the gRPC server instance without starting it.
func (s *Server) InitGRPCServer() *grpc.Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.grpcServer == nil {
		const maxMsgSize = 64 * 1024 * 1024
		s.grpcServer = grpc.NewServer(
			grpc.MaxRecvMsgSize(maxMsgSize),
			grpc.MaxSendMsgSize(maxMsgSize),
		)
		pb.RegisterBackupServiceServer(s.grpcServer, s)
	}
	return s.grpcServer
}

// SetBackupFailedHandler sets the callback invoked on backup failures.
func (s *Server) SetBackupFailedHandler(fn func(agentName, hostname, reason string, details map[string]any)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.onBackupFailed = fn
}

// SetHooks sets the pre/post-backup hook plugins.
func (s *Server) SetHooks(hooks []hook.Hook) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.hooks = hooks
}

// getHooks returns the current hook plugins for RPC-side iteration.
func (s *Server) getHooks() []hook.Hook {
	s.handlersMu.RLock()
	defer s.handlersMu.RUnlock()
	return s.hooks
}

func (s *Server) notifyBackupFailed(agentName, hostname, reason string, details map[string]any) {
	s.handlersMu.RLock()
	fn := s.onBackupFailed
	s.handlersMu.RUnlock()
	if fn != nil {
		go fn(agentName, hostname, reason, details)
	}
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() {
	if s.grpcServer != nil {
		slog.Info("server shutting down")
		s.grpcServer.GracefulStop()
	}
	s.waitBackground(backgroundStopTimeout)
}

// backgroundStopTimeout bounds how long Stop waits for post-backup work, so a
// long restore verification cannot hold up a service shutdown.
const backgroundStopTimeout = 15 * time.Second

// goBackground runs fn in a goroutine that Stop waits for.
func (s *Server) goBackground(fn func()) {
	s.background.Add(1)
	go func() {
		defer s.background.Done()
		fn()
	}()
}

// waitBackground waits up to timeout for post-backup work to finish.
func (s *Server) waitBackground(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		s.background.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		slog.Warn("server stopped before post-backup checks finished", "waited", timeout)
	}
}

// repoBasePath attempts to get the filesystem base path of the first repository.
// Returns empty string if no repos or the backend is not local.
func (s *Server) repoBasePath() string {
	repos := s.repos.List()
	if len(repos) == 0 {
		return ""
	}
	type basePather interface {
		BasePath() string
	}
	if bp, ok := repos[0].Backend.(basePather); ok {
		return bp.BasePath()
	}
	return ""
}
