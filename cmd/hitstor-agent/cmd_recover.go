package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/hooperits/hitstor/internal/agent"
	"github.com/hooperits/hitstor/internal/bmr"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/spf13/cobra"
)

var (
	recoverTokenFlag   string
	recoverAgentFlag   string
	recoverTargetFlag  string
	recoverPortFlag    int
	recoverTimeoutFlag time.Duration
)

func newRecoverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recover",
		Short: "Run in bare-metal recovery mode (invoked from rescue media)",
		Long: `Recovery mode is what the rescue system runs after ReaR has recreated this
machine's disk layout and mounted the new root.

It is not a normal agent: it starts no schedule, writes no configuration, and
exits as soon as the restore it is here for reaches a terminal state.

Every other restore in HitStor is started by the server dialling an agent it
already knows the address of. A machine that just booted off rescue media has
whatever address DHCP handed it, which nobody has told the server about — so
this calls home instead, presenting a single-use recovery token, and the
server dials back over the ordinary restore path.

ReaR installs a bootloader only if this command exits 0.`,
		RunE: runRecover,
	}

	cmd.Flags().StringVar(&recoverTokenFlag, "token", "",
		"single-use recovery token, issued from the dashboard")
	cmd.Flags().StringVar(&recoverAgentFlag, "agent", "",
		"registered agent name being recovered (default: this host's name)")
	cmd.Flags().StringVar(&recoverTargetFlag, "target", bmr.RecoveryRoot,
		"directory the recreated root is mounted on")
	cmd.Flags().IntVar(&recoverPortFlag, "listen-port", defaultListenPort,
		"gRPC port for the server to push the restore to")
	cmd.Flags().DurationVar(&recoverTimeoutFlag, "timeout", agent.DefaultRecoveryTimeout,
		"give up if no restore completes within this window")

	return cmd
}

func runRecover(cmd *cobra.Command, args []string) error {
	if err := requireServer(); err != nil {
		return err
	}
	if recoverTokenFlag == "" {
		return fmt.Errorf("recovery token required: use --token <token> " +
			"(issue one from Disaster Recovery -> Bare Metal in the dashboard)")
	}

	agentName := recoverAgentFlag
	if agentName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("cannot determine this host's name; pass --agent explicitly: %w", err)
		}
		agentName = hostname
	}

	// Refuse to restore over the rescue system's own ramdisk. Without ReaR's
	// mountpoint the restore would write into memory and vanish at reboot,
	// which looks like success right up until the machine boots to nothing.
	if err := checkRecoveryTarget(recoverTargetFlag); err != nil {
		return err
	}

	client, err := agent.NewClient(agent.ClientOptions{
		ServerAddr: serverFlag,
		CACertPath: caCertFlag,
		PinPath:    agent.DefaultServerPinPath,
	})
	if err != nil {
		return fmt.Errorf("connect to server %s: %w", serverFlag, err)
	}
	defer client.Close()

	rec, err := agent.NewRecovery(agent.RecoveryOptions{
		AgentName:  agentName,
		Token:      recoverTokenFlag,
		TargetPath: recoverTargetFlag,
		ListenPort: recoverPortFlag,
		Timeout:    recoverTimeoutFlag,
		Announcer:  &serverAnnouncer{client: client},
	})
	if err != nil {
		return err
	}

	// The listener authenticates on a token this process just generated: the
	// original agent's token is on a disk that may no longer exist.
	agentSrv := agent.NewAgentServer(agent.AgentServerOptions{
		Token:   rec.AgentToken(),
		Version: Version,
	})
	agentSrv.OnRestoreFinished = rec.RestoreFinished

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, shutdownSignals...)
	go func() {
		select {
		case <-sigCh:
			slog.Warn("recovery interrupted")
			cancel()
		case <-ctx.Done():
		}
	}()

	listenErr := make(chan error, 1)
	go func() { listenErr <- agentSrv.Start(recoverPortFlag) }()
	defer agentSrv.Stop()

	// A listener that never came up would leave the server dialling into
	// nothing, so fail here rather than after the announce succeeds.
	select {
	case err := <-listenErr:
		return fmt.Errorf("recovery listener on port %d: %w", recoverPortFlag, err)
	case <-time.After(500 * time.Millisecond):
	}

	fmt.Fprintf(cmd.OutOrStdout(),
		"Announcing %s to %s for bare-metal recovery into %s...\n",
		agentName, serverFlag, recoverTargetFlag)

	outcome, err := rec.Run(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nRestored %d files (%d bytes) into %s\n",
		outcome.FilesRestored, outcome.BytesWritten, recoverTargetFlag)
	for _, warning := range outcome.Errors {
		fmt.Fprintf(cmd.ErrOrStderr(), "  warning: %s\n", warning)
	}
	if len(outcome.Errors) > 0 {
		// Individual files can legitimately fail without invalidating the
		// restore, so this is reported rather than fatal — but it must be
		// visible before ReaR moves on and installs a bootloader.
		fmt.Fprintf(cmd.ErrOrStderr(),
			"\n%d file(s) could not be restored. Review the warnings above before rebooting.\n",
			len(outcome.Errors))
	}
	return nil
}

// checkRecoveryTarget verifies ReaR really mounted the recreated root where we
// are about to write.
func checkRecoveryTarget(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("restore target %s is not available; "+
			"ReaR should have mounted the recreated root there: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("restore target %s is not a directory", path)
	}
	return nil
}

// serverAnnouncer adapts the agent's gRPC client to the Announcer interface.
type serverAnnouncer struct {
	client *agent.Client
}

func (a *serverAnnouncer) Announce(ctx context.Context, ann agent.RecoveryAnnouncement) (string, error) {
	resp, err := a.client.AnnounceForRecovery(ctx, &pb.AnnounceForRecoveryRequest{
		RecoveryToken: ann.RecoveryToken,
		AgentName:     ann.AgentName,
		AgentAddr:     ann.AgentAddr,
		Hostname:      ann.Hostname,
		TargetPath:    ann.TargetPath,
		AgentToken:    ann.AgentToken,
	})
	if err != nil {
		return "", err
	}
	return resp.Message, nil
}
