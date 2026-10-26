package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// DefaultRecoveryTimeout bounds how long a recovery-mode agent waits for the
// restore it is participating in.
//
// The normal completion signal is the restore stream's own terminal state, so
// this only fires when that signal never arrives — a dropped connection, an
// operator who walked away. Twelve hours is generous enough for a large host
// over a slow link, and short enough that dead recovery hardware eventually
// gives an operator something to act on instead of hanging indefinitely.
const DefaultRecoveryTimeout = 12 * time.Hour

// ErrRecoveryTimeout reports that no restore reached a terminal state within
// the safety-net window.
var ErrRecoveryTimeout = errors.New("agent: timed out waiting for a restore to complete")

// RecoveryAnnouncement is what a rescue system tells the server when it calls
// home.
type RecoveryAnnouncement struct {
	// RecoveryToken is the single-use token an operator minted and typed in
	// at the rescue prompt.
	RecoveryToken string
	// AgentName is the registered agent whose backups this media recovers.
	AgentName string
	// AgentAddr is where the server should dial back. Normally just ":port",
	// leaving the server to fill in the host from the connection it receives:
	// a rescue system rarely knows which of its addresses the server can
	// reach.
	AgentAddr string
	// Hostname is what the rescue system calls itself, for the operator to
	// confirm against.
	Hostname string
	// TargetPath is where ReaR mounted the recreated root.
	TargetPath string
	// AgentToken authenticates the server's dial-back against this rescue
	// system's own listener.
	AgentToken string
}

// Announcer delivers a recovery announcement to the server. Defined here, at
// its point of use, so recovery logic can be exercised without a server.
type Announcer interface {
	// Announce returns the server's message for the operator, or an error
	// explaining why the recovery was refused.
	Announce(ctx context.Context, ann RecoveryAnnouncement) (string, error)
}

// RecoveryOptions configures one recovery-mode run.
type RecoveryOptions struct {
	// AgentName is the registered agent being recovered.
	AgentName string
	// Token is the operator's single-use recovery token.
	Token string
	// TargetPath is ReaR's mountpoint for the recreated root.
	TargetPath string
	// ListenPort is the port the rescue system's gRPC listener binds.
	ListenPort int
	// Announcer calls home. Required.
	Announcer Announcer
	// Timeout is the safety net. Zero uses DefaultRecoveryTimeout.
	Timeout time.Duration
}

// Recovery drives a rescue system through one bare-metal restore: it announces
// itself to the server, then waits for the restore the server pushes back.
//
// The completion signal is the restore stream's own terminal state rather than
// a heartbeat or a poll: this process is the one receiving the restore, so it
// already knows when the work is done.
type Recovery struct {
	opts RecoveryOptions

	// agentToken authenticates the server's dial-back. Generated here because
	// the original agent's token lives on a disk that may no longer exist,
	// and the server does not disclose it.
	agentToken string

	once     sync.Once
	finished chan RestoreOutcome
}

// NewRecovery validates options and prepares a recovery run.
func NewRecovery(opts RecoveryOptions) (*Recovery, error) {
	switch {
	case opts.AgentName == "":
		return nil, fmt.Errorf("agent: recovery: agent name is required")
	case opts.Token == "":
		return nil, fmt.Errorf("agent: recovery: recovery token is required")
	case opts.TargetPath == "":
		return nil, fmt.Errorf("agent: recovery: target path is required")
	case opts.Announcer == nil:
		return nil, fmt.Errorf("agent: recovery: no way to reach the server")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultRecoveryTimeout
	}

	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return nil, fmt.Errorf("agent: recovery: generate listener token: %w", err)
	}

	return &Recovery{
		opts:       opts,
		agentToken: hex.EncodeToString(buf[:]),
		finished:   make(chan RestoreOutcome, 1),
	}, nil
}

// AgentToken is the token this rescue system's listener will accept.
func (r *Recovery) AgentToken() string { return r.agentToken }

// restoreFinished records a completed restore. Safe to call more than once;
// only the first outcome counts, since a recovery ends at its first terminal
// state either way.
func (r *Recovery) restoreFinished(outcome RestoreOutcome) {
	r.once.Do(func() { r.finished <- outcome })
}

// RestoreFinished is the AgentServer.OnRestoreFinished callback for this run.
func (r *Recovery) RestoreFinished(outcome RestoreOutcome) {
	r.restoreFinished(outcome)
}

// announce tells the server where to find this rescue system.
func (r *Recovery) announce(ctx context.Context) error {
	hostname, _ := os.Hostname()

	msg, err := r.opts.Announcer.Announce(ctx, RecoveryAnnouncement{
		RecoveryToken: r.opts.Token,
		AgentName:     r.opts.AgentName,
		// Host deliberately omitted: the server fills it in from the
		// connection, which is by definition an address that reached it.
		AgentAddr:  fmt.Sprintf(":%d", r.opts.ListenPort),
		Hostname:   hostname,
		TargetPath: r.opts.TargetPath,
		AgentToken: r.agentToken,
	})
	if err != nil {
		return fmt.Errorf("agent: recovery refused by server: %w", err)
	}
	if msg != "" {
		slog.Info("recovery announced", "server_message", msg)
	}
	return nil
}

// wait blocks until the restore reaches a terminal state, the safety-net
// timeout fires, or ctx is cancelled.
func (r *Recovery) wait(ctx context.Context) (RestoreOutcome, error) {
	timer := time.NewTimer(r.opts.Timeout)
	defer timer.Stop()

	select {
	case outcome := <-r.finished:
		if outcome.Err != nil {
			return outcome, fmt.Errorf("agent: restore failed: %w", outcome.Err)
		}
		return outcome, nil
	case <-timer.C:
		return RestoreOutcome{}, fmt.Errorf("%w after %s — the server never pushed a restore, or the connection dropped",
			ErrRecoveryTimeout, r.opts.Timeout)
	case <-ctx.Done():
		return RestoreOutcome{}, ctx.Err()
	}
}

// Run announces this rescue system and waits for the restore.
//
// It is the whole of recovery mode: everything after the announce reuses the
// ordinary server→agent restore push unchanged.
func (r *Recovery) Run(ctx context.Context) (RestoreOutcome, error) {
	if err := r.announce(ctx); err != nil {
		return RestoreOutcome{}, err
	}
	return r.wait(ctx)
}
