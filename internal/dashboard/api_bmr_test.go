package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// bmrTestDashboard returns a dashboard whose recovery-media cache is isolated
// to a temp dir, so tests never touch a real server's images.
func bmrTestDashboard(t *testing.T) *Dashboard {
	t.Helper()
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	t.Cleanup(cleanup)
	dash.SetMediaCacheDir(t.TempDir())
	return dash
}

// seedMedia writes a cached image for a host.
func seedMedia(t *testing.T, d *Dashboard, host, body string) {
	t.Helper()
	w, err := d.mediaCache().Writer(host)
	if err != nil {
		t.Fatalf("cache writer: %v", err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// bmrRequest routes one request through the dashboard mux with auth disabled.
func bmrRequest(t *testing.T, d *Dashboard, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, req)
	return rec
}

func TestBMRDownloadIsA404BeforeAnyBuild(t *testing.T) {
	d := bmrTestDashboard(t)

	rec := bmrRequest(t, d, http.MethodGet, "/api/bmr/media/web01/download")

	// "Not built yet" is an ordinary state, not a server fault: most hosts
	// never have media built.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404\nbody: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no recovery media") {
		t.Errorf("body does not say why: %s", rec.Body.String())
	}
}

func TestBMRDownloadServesTheCachedImage(t *testing.T) {
	d := bmrTestDashboard(t)
	seedMedia(t, d, "web01", "iso-bytes")

	rec := bmrRequest(t, d, http.MethodGet, "/api/bmr/media/web01/download")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	if string(body) != "iso-bytes" {
		t.Errorf("body = %q, want the cached image", body)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "hitstor-recovery-web01.iso") {
		t.Errorf("Content-Disposition = %q, want a named .iso attachment", got)
	}
	// Range support matters: these images run to gigabytes and a dropped
	// download should resume rather than start over.
	if got := rec.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Errorf("Accept-Ranges = %q, want %q", got, "bytes")
	}
}

func TestBMRDownloadRefusesPathTraversal(t *testing.T) {
	d := bmrTestDashboard(t)

	// Nothing outside the cache directory may be served, whatever the
	// registry or a crafted URL says the host is called.
	for _, target := range []string{
		"/api/bmr/media/..%2f..%2fetc%2fpasswd/download",
		"/api/bmr/media/.../download",
		"/api/bmr/media/..%5cwindows/download",
	} {
		t.Run(target, func(t *testing.T) {
			rec := bmrRequest(t, d, http.MethodGet, target)
			if rec.Code == http.StatusOK {
				t.Fatalf("served something for %q:\n%s", target, rec.Body.String())
			}
		})
	}
}

func TestBMRDeleteMediaIsIdempotent(t *testing.T) {
	d := bmrTestDashboard(t)
	seedMedia(t, d, "web01", "iso-bytes")

	for i := 0; i < 2; i++ {
		rec := bmrRequest(t, d, http.MethodDelete, "/api/bmr/media/web01")
		if rec.Code != http.StatusOK {
			t.Fatalf("delete %d: status = %d, want 200\nbody: %s", i+1, rec.Code, rec.Body.String())
		}
	}
	if rec := bmrRequest(t, d, http.MethodGet, "/api/bmr/media/web01/download"); rec.Code != http.StatusNotFound {
		t.Errorf("status after delete = %d, want 404", rec.Code)
	}
}

func TestBMRHostsListsRegisteredAgentsWithTheirMedia(t *testing.T) {
	d := bmrTestDashboard(t)
	if _, err := d.registry.AddAgent("web01"); err != nil {
		t.Fatalf("add agent: %v", err)
	}
	seedMedia(t, d, "web01", "iso-bytes")

	rec := bmrRequest(t, d, http.MethodGet, "/api/bmr/hosts")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
	}

	var hosts []bmrHostStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &hosts); err != nil {
		t.Fatalf("decode: %v\nbody: %s", err, rec.Body.String())
	}
	if len(hosts) != 1 {
		t.Fatalf("got %d hosts, want 1: %+v", len(hosts), hosts)
	}

	got := hosts[0]
	if got.Agent != "web01" {
		t.Errorf("Agent = %q, want %q", got.Agent, "web01")
	}
	if !got.HasMedia || got.MediaSize != int64(len("iso-bytes")) {
		t.Errorf("media not reported: %+v", got)
	}
	// The agent has no address, so readiness is unknown — and the row must
	// still appear, saying so. Dropping unreachable hosts hides exactly the
	// ones an operator needs to fix.
	if got.Reachable {
		t.Error("an agent with no address reported as reachable")
	}
	if got.Error == "" {
		t.Error("no explanation for the unknown readiness")
	}
}

func TestBMRBuildRejectsAnAgentWithNoAddress(t *testing.T) {
	d := bmrTestDashboard(t)
	if _, err := d.registry.AddAgent("web01"); err != nil {
		t.Fatalf("add agent: %v", err)
	}

	rec := bmrRequest(t, d, http.MethodPost, "/api/bmr/media/web01/build")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400\nbody: %s", rec.Code, rec.Body.String())
	}

	rec = bmrRequest(t, d, http.MethodPost, "/api/bmr/media/nope/build")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status for an unknown agent = %d, want 404", rec.Code)
	}
}

func TestBMRPrepareMintsATokenShownExactlyOnce(t *testing.T) {
	d := bmrTestDashboard(t)
	if _, err := d.registry.AddAgent("web01"); err != nil {
		t.Fatal(err)
	}

	rec := bmrRequest(t, d, http.MethodPost, "/api/bmr/prepare/web01")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
	}

	var minted recoveryTokenView
	if err := json.Unmarshal(rec.Body.Bytes(), &minted); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if minted.Token == "" {
		t.Fatal("no token returned; the operator has nothing to type at the console")
	}
	if minted.ID == "" || minted.ExpiresAt.IsZero() {
		t.Errorf("token record incomplete: %+v", minted)
	}

	// The listing exists so an operator can see and revoke what is
	// outstanding — but never read a token back out of it.
	listed := bmrRequest(t, d, http.MethodGet, "/api/bmr/tokens")
	var tokens []recoveryTokenView
	if err := json.Unmarshal(listed.Body.Bytes(), &tokens); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("got %d tokens, want 1", len(tokens))
	}
	if tokens[0].Token != "" {
		t.Error("the token's plaintext is readable from the listing")
	}
	if strings.Contains(listed.Body.String(), minted.Token) {
		t.Error("the token's plaintext appears in the listing response")
	}
}

func TestBMRPrepareRejectsAnUnknownAgent(t *testing.T) {
	d := bmrTestDashboard(t)

	rec := bmrRequest(t, d, http.MethodPost, "/api/bmr/prepare/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404\nbody: %s", rec.Code, rec.Body.String())
	}
}

func TestBMRRevokeTokenRemovesItFromTheListing(t *testing.T) {
	d := bmrTestDashboard(t)
	if _, err := d.registry.AddAgent("web01"); err != nil {
		t.Fatal(err)
	}

	rec := bmrRequest(t, d, http.MethodPost, "/api/bmr/prepare/web01")
	var minted recoveryTokenView
	if err := json.Unmarshal(rec.Body.Bytes(), &minted); err != nil {
		t.Fatal(err)
	}

	if got := bmrRequest(t, d, http.MethodDelete, "/api/bmr/tokens/"+minted.ID); got.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, want 200\nbody: %s", got.Code, got.Body.String())
	}

	listed := bmrRequest(t, d, http.MethodGet, "/api/bmr/tokens")
	var tokens []recoveryTokenView
	json.Unmarshal(listed.Body.Bytes(), &tokens)
	if len(tokens) != 0 {
		t.Errorf("%d tokens after revoke, want 0", len(tokens))
	}

	// Revoking one that is already gone is a 404, not a silent success: the
	// operator asked about a specific token and should learn it is unknown.
	if got := bmrRequest(t, d, http.MethodDelete, "/api/bmr/tokens/"+minted.ID); got.Code != http.StatusNotFound {
		t.Errorf("second revoke status = %d, want 404", got.Code)
	}
}

func TestBMRHostsProbesAgentsConcurrently(t *testing.T) {
	d := bmrTestDashboard(t)

	// Point every agent at an address that black-holes connections, so each
	// probe sits out its timeout. Probed serially these would take minutes —
	// and a fleet with dead machines is exactly the fleet that needs this
	// page to load.
	const agentCount = 24
	for i := 0; i < agentCount; i++ {
		name := fmt.Sprintf("host%02d", i)
		if _, err := d.registry.AddManagedAgent(name, "192.0.2.1:8422", "token"); err != nil {
			t.Fatalf("add agent: %v", err)
		}
	}

	// A short request deadline: the handler must return within it rather than
	// serializing agentCount probes behind one another.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/bmr/hosts", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	start := time.Now()
	d.Handler().ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
	}
	var hosts []bmrHostStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &hosts); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(hosts) != agentCount {
		t.Fatalf("got %d hosts, want %d", len(hosts), agentCount)
	}
	// Well under agentCount × the per-probe timeout, which is the point.
	if elapsed > 15*time.Second {
		t.Errorf("listing took %v for %d unreachable agents; probes are not overlapping",
			elapsed, agentCount)
	}
	// Every row must still carry a verdict, not just the ones that finished.
	for _, h := range hosts {
		if h.Reachable {
			t.Errorf("%s reported reachable at a black-holed address", h.Agent)
		}
		if h.Error == "" {
			t.Errorf("%s has no explanation for its unknown readiness", h.Agent)
		}
	}
}
