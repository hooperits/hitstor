package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// recordingAnnouncer stands in for the server during recovery-mode tests.
//
// The announce happens on the recovery goroutine while a test reads what it
// captured, so the record is guarded.
type recordingAnnouncer struct {
	mu  sync.Mutex
	got RecoveryAnnouncement
	err error
}

func (a *recordingAnnouncer) Announce(_ context.Context, ann RecoveryAnnouncement) (string, error) {
	a.mu.Lock()
	a.got = ann
	err := a.err
	a.mu.Unlock()

	if err != nil {
		return "", err
	}
	return "Registered for recovery.", nil
}

// announcement returns what the rescue system reported, if it has yet.
func (a *recordingAnnouncer) announcement() RecoveryAnnouncement {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.got
}

func recoveryTestOptions(ann Announcer) RecoveryOptions {
	return RecoveryOptions{
		AgentName:  "web01",
		Token:      "recovery-token",
		TargetPath: "/mnt/local",
		ListenPort: 8422,
		Announcer:  ann,
		Timeout:    time.Minute,
	}
}

func TestRecoveryAnnouncesItselfWithAFreshAgentToken(t *testing.T) {
	ann := &recordingAnnouncer{}
	r, err := NewRecovery(recoveryTestOptions(ann))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	if err := r.announce(context.Background()); err != nil {
		t.Fatalf("announce: %v", err)
	}

	if ann.announcement().AgentName != "web01" {
		t.Errorf("AgentName = %q, want %q", ann.announcement().AgentName, "web01")
	}
	if ann.announcement().RecoveryToken != "recovery-token" {
		t.Errorf("RecoveryToken = %q, want the operator's token", ann.announcement().RecoveryToken)
	}
	if ann.announcement().TargetPath != "/mnt/local" {
		t.Errorf("TargetPath = %q, want ReaR's mountpoint", ann.announcement().TargetPath)
	}
	// The rescue system cannot know the original agent's token, so it makes
	// its own and tells the server what to authenticate against.
	if len(ann.announcement().AgentToken) < 32 {
		t.Errorf("AgentToken = %q; too short to be a credential", ann.announcement().AgentToken)
	}
	if ann.announcement().AgentToken == ann.announcement().RecoveryToken {
		t.Error("the rescue agent reused the recovery token as its own listener token")
	}
	if ann.announcement().Hostname == "" {
		t.Error("no hostname reported for the operator to confirm against")
	}
}

func TestRecoveryAnnouncesTheListenPortSoTheServerCanDialBack(t *testing.T) {
	ann := &recordingAnnouncer{}
	opts := recoveryTestOptions(ann)
	opts.ListenPort = 9999
	r, err := NewRecovery(opts)
	if err != nil {
		t.Fatal(err)
	}

	if err := r.announce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// The rescue system does not know which of its addresses the server can
	// reach, so it reports the port and lets the server fill in the host from
	// the connection that just arrived.
	if ann.announcement().AgentAddr != ":9999" {
		t.Errorf("AgentAddr = %q, want %q", ann.announcement().AgentAddr, ":9999")
	}
}

func TestRecoveryRefusesIncompleteOptions(t *testing.T) {
	tests := map[string]func(*RecoveryOptions){
		"no agent name":  func(o *RecoveryOptions) { o.AgentName = "" },
		"no token":       func(o *RecoveryOptions) { o.Token = "" },
		"no target path": func(o *RecoveryOptions) { o.TargetPath = "" },
		"no announcer":   func(o *RecoveryOptions) { o.Announcer = nil },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			opts := recoveryTestOptions(&recordingAnnouncer{})
			mutate(&opts)
			if _, err := NewRecovery(opts); err == nil {
				t.Fatal("expected an error, got none")
			}
		})
	}
}

func TestRecoveryWaitReturnsWhenTheRestoreFinishes(t *testing.T) {
	r, err := NewRecovery(recoveryTestOptions(&recordingAnnouncer{}))
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		r.restoreFinished(RestoreOutcome{FilesRestored: 42, BytesWritten: 1024})
	}()

	outcome, err := r.wait(context.Background())
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if outcome.FilesRestored != 42 {
		t.Errorf("FilesRestored = %d, want 42", outcome.FilesRestored)
	}
}

func TestRecoveryWaitFailsOnAFailedRestore(t *testing.T) {
	r, err := NewRecovery(recoveryTestOptions(&recordingAnnouncer{}))
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		r.restoreFinished(RestoreOutcome{Err: errors.New("connection reset")})
	}()

	// ReaR installs a bootloader only if this succeeds. A half-restored disk
	// that boots is worse than one that visibly did not restore.
	if _, err := r.wait(context.Background()); err == nil {
		t.Fatal("a failed restore was reported as success")
	}
}

func TestRecoveryWaitTimesOutRatherThanHangingForever(t *testing.T) {
	opts := recoveryTestOptions(&recordingAnnouncer{})
	opts.Timeout = 20 * time.Millisecond
	r, err := NewRecovery(opts)
	if err != nil {
		t.Fatal(err)
	}

	// Unattended recovery hardware must not sit waiting on a restore that
	// will never arrive, with no operator recourse.
	start := time.Now()
	_, err = r.wait(context.Background())
	if err == nil {
		t.Fatal("wait returned success after the timeout elapsed")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("wait took %v to give up; the safety net did not fire", elapsed)
	}
	if !errors.Is(err, ErrRecoveryTimeout) {
		t.Errorf("err = %v, want ErrRecoveryTimeout so the exit reason is clear", err)
	}
}

func TestRecoveryDefaultTimeoutIsBoundedButGenerous(t *testing.T) {
	opts := recoveryTestOptions(&recordingAnnouncer{})
	opts.Timeout = 0
	r, err := NewRecovery(opts)
	if err != nil {
		t.Fatal(err)
	}

	// Long enough to restore a large host over a slow link; short enough that
	// a dead recovery does not tie up hardware indefinitely.
	if r.opts.Timeout < time.Hour {
		t.Errorf("default timeout %v is too short for a real restore", r.opts.Timeout)
	}
	if r.opts.Timeout > 48*time.Hour {
		t.Errorf("default timeout %v is not a safety net", r.opts.Timeout)
	}
}

func TestRecoverySurfacesAnAnnounceRejection(t *testing.T) {
	ann := &recordingAnnouncer{err: errors.New("recovery token expired")}
	r, err := NewRecovery(recoveryTestOptions(ann))
	if err != nil {
		t.Fatal(err)
	}

	err = r.announce(context.Background())
	if err == nil {
		t.Fatal("a rejected announce was reported as success")
	}
	// The operator is at a console with no dashboard; the reason has to reach
	// them here.
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error does not carry the server's reason: %v", err)
	}
}
