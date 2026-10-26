package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// AnnounceForRecovery implements BackupService.AnnounceForRecovery: it admits
// a host booted off bare-metal recovery media.
//
// Authentication is the single-use recovery token alone, not an API key. The
// rescue media deliberately carries no credentials, and the original host's
// key is on a disk that may no longer exist.
func (s *Server) AnnounceForRecovery(ctx context.Context, req *pb.AnnounceForRecoveryRequest) (*pb.AnnounceForRecoveryResponse, error) {
	agentName := strings.TrimSpace(req.AgentName)
	if agentName == "" {
		return nil, status.Error(codes.InvalidArgument, "agent name is required")
	}

	addr, err := resolveRescueAddr(ctx, req.AgentAddr)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	name, err := s.registry.BeginRecoveryAnnounce(RecoveryAnnounce{
		Token:      req.RecoveryToken,
		Agent:      agentName,
		Addr:       addr,
		AgentToken: req.AgentToken,
	})
	if err != nil {
		// Token failures are the expected way this call is refused, and the
		// reason (expired, spent, wrong host) is what the operator at the
		// console needs to read.
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	slog.Warn("bare-metal recovery host announced",
		"agent", name,
		"addr", addr,
		"hostname", req.Hostname,
		"target", req.TargetPath,
	)

	return &pb.AnnounceForRecoveryResponse{
		AgentName: name,
		Message: fmt.Sprintf(
			"Registered %s for bare-metal recovery at %s. "+
				"Start the restore from the dashboard: Disaster Recovery → Bare Metal.",
			name, addr),
	}, nil
}

// resolveRescueAddr works out where the server should dial the rescue host
// back.
//
// A rescue system generally knows its own port but not the address the server
// sees it as — NAT, multiple interfaces, an address DHCP assigned seconds ago.
// So a bare ":port" is completed from the connection's own peer address, which
// is by definition an address that reached us.
func resolveRescueAddr(ctx context.Context, announced string) (string, error) {
	announced = strings.TrimSpace(announced)
	if announced == "" {
		return "", fmt.Errorf("agent address is required so the server can reach the rescue host")
	}

	host, port, err := net.SplitHostPort(announced)
	if err != nil {
		return "", fmt.Errorf("agent address %q must be host:port", announced)
	}
	if port == "" {
		return "", fmt.Errorf("agent address %q must include a port", announced)
	}
	if host != "" && host != "0.0.0.0" && host != "::" {
		return announced, nil
	}

	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf(
			"agent address %q has no host and the connection's own address is unknown", announced)
	}
	peerHost, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		return "", fmt.Errorf("cannot determine the rescue host's address: %w", err)
	}
	return net.JoinHostPort(peerHost, port), nil
}
