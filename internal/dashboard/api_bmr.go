package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/server"
)

// mediaCheckTimeout bounds one readiness probe. The probe only looks for a few
// tools on the agent, so a slow answer means a sick host rather than a big one,
// and a page listing a whole fleet should not wait long to call a host down.
const mediaCheckTimeout = 10 * time.Second

// mediaProbeConcurrency bounds how many agents are probed for readiness at
// once, so listing a large fleet does not open a connection per host.
const mediaProbeConcurrency = 32

// maxBuildLogLines caps how much of ReaR's output is kept for the dashboard.
// Enough to diagnose a failure; bounded so a long build cannot grow the
// server's memory without limit.
const maxBuildLogLines = 2000

// mediaBuildTimeout bounds a whole media build. `rear mkrescue` legitimately
// runs for many minutes on a large host; the cap exists so a wedged build
// eventually releases the per-host lock rather than blocking rebuilds forever.
const mediaBuildTimeout = 2 * time.Hour

// bmrHostStatus is the per-host readiness row behind the Bare Metal tab.
type bmrHostStatus struct {
	Agent   string `json:"agent"`
	Addr    string `json:"addr,omitempty"`
	Managed bool   `json:"managed"`
	// Reachable reports whether the readiness probe reached the agent at all.
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`

	// Readiness, when the agent answered.
	Ready         bool     `json:"ready"`
	Platform      string   `json:"platform,omitempty"`
	ReaRInstalled bool     `json:"rear_installed"`
	ReaRVersion   string   `json:"rear_version,omitempty"`
	Blockers      []string `json:"blockers,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`

	// Cached media, when any has been built.
	HasMedia     bool      `json:"has_media"`
	MediaSize    int64     `json:"media_size_bytes,omitempty"`
	MediaBuiltAt time.Time `json:"media_built_at,omitempty"`
	MediaAgeDays int       `json:"media_age_days,omitempty"`
	// Building reports a build in flight, so the UI can disable the button.
	Building bool `json:"building"`
	// Recovering marks a host currently booted off recovery media, so the UI
	// does not present it as an ordinary agent.
	Recovering bool `json:"recovering,omitempty"`
}

// handleAPIBMRHosts lists every registered agent with its recovery-media
// readiness and what media the server holds for it.
//
// Readiness comes from the agents themselves, so an unreachable host reports
// as unreachable rather than dropping out of the list: "no media and I cannot
// ask why" is the answer an operator most needs to see.
func (d *Dashboard) handleAPIBMRHosts(w http.ResponseWriter, r *http.Request) {
	cache := d.mediaCache()
	agents := d.registry.ListAgents()

	out := make([]bmrHostStatus, len(agents))
	for i, a := range agents {
		out[i] = bmrHostStatus{
			Agent: a.Name, Addr: a.AgentAddr, Managed: a.Managed, Recovering: a.Recovering,
			Building: cache.Building(a.Name),
		}
		if media, err := cache.Get(a.Name); err == nil {
			out[i].HasMedia = true
			out[i].MediaSize = media.SizeBytes
			out[i].MediaBuiltAt = media.BuiltAt
			out[i].MediaAgeDays = int(time.Since(media.BuiltAt).Hours() / 24)
		}
	}

	// Each probe can sit out its full timeout against a host that is down, so
	// probing serially would make the page take minutes on a fleet with a few
	// dead machines — the fleet most in need of this view. Bounded so a large
	// fleet does not open hundreds of connections at once.
	var wg sync.WaitGroup
	sem := make(chan struct{}, mediaProbeConcurrency)
	for i := range agents {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			d.fillMediaReadiness(r.Context(), &out[i], agents[i])
		}(i)
	}
	wg.Wait()

	writeJSON(w, out)
}

// fillMediaReadiness asks one agent whether it can build recovery media.
func (d *Dashboard) fillMediaReadiness(ctx context.Context, status *bmrHostStatus, a server.AgentRecord) {
	if a.AgentAddr == "" {
		status.Error = "no agent address on file; register the agent from the Agents page"
		return
	}

	token, err := d.registry.DecryptToken(a.Name)
	if err != nil {
		status.Error = fmt.Sprintf("agent token unavailable: %v", err)
		return
	}
	client, err := d.dialManagedAgent(a.Name, a.AgentAddr)
	if err != nil {
		status.Error = fmt.Sprintf("connect to agent: %v", err)
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, mediaCheckTimeout)
	defer cancel()

	resp, err := client.CheckRecoveryMedia(ctx, token)
	if err != nil {
		status.Error = fmt.Sprintf("readiness check: %v", err)
		return
	}

	status.Reachable = true
	status.Ready = resp.Ready
	status.Platform = resp.Platform
	status.ReaRInstalled = resp.RearInstalled
	status.ReaRVersion = resp.RearVersion
	status.Blockers = resp.Blockers
	status.Warnings = resp.Warnings
}

// handleAPIBMRBuild starts a media build on one agent and returns an operation
// ID the caller follows over the existing operation SSE stream.
//
// The build runs detached from this request: it takes minutes, far longer than
// a browser will hold a POST open.
func (d *Dashboard) handleAPIBMRBuild(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("agent")

	agent := d.registry.GetAgent(name)
	if agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	if agent.AgentAddr == "" {
		http.Error(w, "agent has no address on file; register it from the Agents page",
			http.StatusBadRequest)
		return
	}

	var req struct {
		MigrationMode bool `json:"migration_mode"`
		Force         bool `json:"force"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	token, err := d.registry.DecryptToken(name)
	if err != nil {
		http.Error(w, fmt.Sprintf("decrypt token: %v", err), http.StatusInternalServerError)
		return
	}

	cache := d.mediaCache()
	if cache.Building(name) {
		http.Error(w, "recovery media for this host is already being built",
			http.StatusConflict)
		return
	}

	op := &operation{
		ID:     newOperationID(),
		Type:   "bmr-build",
		Status: "running",
		done:   make(chan struct{}),
	}
	d.opMu.Lock()
	d.operations[op.ID] = op
	d.opMu.Unlock()

	addr := agent.AgentAddr
	serverAddr := d.serverGRPCAddr

	go func() {
		defer close(op.done)

		op.addMessage(fmt.Sprintf("Building recovery media on %s (%s)", name, addr))
		op.addMessage("This runs ReaR on the protected host and takes several minutes.")

		ctx, cancel := context.WithTimeout(context.Background(), mediaBuildTimeout)
		defer cancel()

		client, err := d.dialManagedAgent(name, addr)
		if err != nil {
			op.fail(fmt.Sprintf("connect to agent: %v", err))
			return
		}
		defer client.Close()

		// `rear -v` is chatty and every forwarded line is retained for the
		// event stream, so a long build would grow without bound.
		var progressLines int
		forward := func(line string) {
			progressLines++
			switch {
			case progressLines < maxBuildLogLines:
				op.addMessage(line)
			case progressLines == maxBuildLogLines:
				op.addMessage("… further ReaR output suppressed; see the agent's logs for the full build log")
			}
		}

		// Report transfer in whole megabytes: a per-chunk message on a
		// multi-gigabyte image would flood the event stream.
		var lastMB int64
		media, err := client.BuildRecoveryMedia(ctx, cache, name, server.BuildRecoveryMediaRequest{
			Token:         token,
			ServerAddr:    serverAddr,
			AgentName:     name,
			MigrationMode: req.MigrationMode,
			Force:         req.Force,
			OnProgress:    forward,
			OnBytes: func(received int64) {
				if mb := received >> 20; mb > lastMB {
					lastMB = mb
					op.addMessage(fmt.Sprintf("Transferred %d MiB", mb))
				}
			},
		})
		if err != nil {
			op.fail(err.Error())
			slog.Warn("recovery media build failed", "agent", name, "error", err)
			return
		}

		op.addMessage(fmt.Sprintf("Cached %s (%s)", filepath.Base(media.Path), formatBytes(uint64(media.SizeBytes))))
		op.mu.Lock()
		op.Status = "completed"
		op.Result = media
		op.mu.Unlock()
		slog.Info("recovery media cached", "agent", name, "size_bytes", media.SizeBytes)
	}()

	writeJSON(w, map[string]string{"operation_id": op.ID})
}

// fail marks an operation failed with a message the operator can act on.
func (op *operation) fail(msg string) {
	op.addMessage(msg)
	op.mu.Lock()
	op.Status = "failed"
	op.Result = map[string]any{"error": msg}
	op.mu.Unlock()
}

// handleAPIBMRDownload serves a host's cached recovery image.
func (d *Dashboard) handleAPIBMRDownload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("agent")

	media, err := d.mediaCache().Get(name)
	if err != nil {
		if errors.Is(err, server.ErrNoMedia) {
			http.Error(w,
				"no recovery media has been built for this host yet", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f, err := os.Open(media.Path)
	if err != nil {
		http.Error(w, fmt.Sprintf("open recovery media: %v", err), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", downloadName(name)))
	// ServeContent handles range requests, so a dropped download of a
	// multi-gigabyte image can resume instead of starting over.
	http.ServeContent(w, r, downloadName(name), media.BuiltAt, f)
}

// downloadName is the filename an operator ends up with on disk.
func downloadName(agent string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, agent)
	return "hitstor-recovery-" + safe + ".iso"
}

// handleAPIBMRDeleteMedia discards a host's cached image.
func (d *Dashboard) handleAPIBMRDeleteMedia(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("agent")
	if err := d.mediaCache().Remove(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]bool{"deleted": true})
}

// recoveryTokenView is a minted token as the dashboard shows it. The
// plaintext appears exactly once, in the mint response.
type recoveryTokenView struct {
	ID        string    `json:"id"`
	Agent     string    `json:"agent"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	// Token is the plaintext, returned only from the mint call.
	Token string `json:"token,omitempty"`
}

// handleAPIBMRPrepare mints a recovery token for one host.
//
// This is the "Prepare bare-metal recovery" action: an operator runs it when a
// machine is already down, reads the token off the screen, and types it at the
// rescue console. It is deliberately explicit rather than something the media
// carries, so an ISO on a shelf is not a route into the repository.
func (d *Dashboard) handleAPIBMRPrepare(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("agent")

	if d.registry.GetAgent(name) == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	var req struct {
		// TTLHours overrides the default window for organizations wanting a
		// tighter one. Zero uses the default.
		TTLHours int `json:"ttl_hours"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	ttl := time.Duration(req.TTLHours) * time.Hour
	if req.TTLHours < 0 {
		http.Error(w, "ttl_hours cannot be negative", http.StatusBadRequest)
		return
	}

	token, err := d.registry.MintRecoveryToken(name, ttl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Find the record we just made, so the caller learns its id and expiry
	// without the plaintext being stored anywhere to look up later.
	var view recoveryTokenView
	for _, t := range d.registry.ListRecoveryTokens() {
		if t.Agent == name && (view.ID == "" || t.CreatedAt.After(view.CreatedAt)) {
			view = recoveryTokenView{
				ID: t.ID, Agent: t.Agent, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt,
			}
		}
	}
	view.Token = token

	slog.Warn("bare-metal recovery token issued", "agent", name, "expires_at", view.ExpiresAt)
	writeJSON(w, view)
}

// handleAPIBMRTokens lists the recovery tokens that can still be used.
func (d *Dashboard) handleAPIBMRTokens(w http.ResponseWriter, r *http.Request) {
	live := d.registry.ListRecoveryTokens()
	out := make([]recoveryTokenView, 0, len(live))
	for _, t := range live {
		out = append(out, recoveryTokenView{
			ID: t.ID, Agent: t.Agent, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt,
		})
	}
	writeJSON(w, out)
}

// handleAPIBMRRevokeToken kills an unused recovery token.
func (d *Dashboard) handleAPIBMRRevokeToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := d.registry.RevokeRecoveryToken(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]bool{"revoked": true})
}
