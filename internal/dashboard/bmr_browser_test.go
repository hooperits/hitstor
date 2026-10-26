package dashboard

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestBareMetalTabRendersInABrowser drives the real page with headless
// Chromium, so a JavaScript error that no Go test can see does not ship a tab
// that silently stays empty.
func TestBareMetalTabRendersInABrowser(t *testing.T) {
	chrome := os.Getenv("HITSTOR_TEST_CHROME")
	if chrome == "" {
		t.Skip("set HITSTOR_TEST_CHROME to a Chromium binary to run the browser check")
	}

	d := bmrTestDashboard(t)
	if _, err := d.registry.AddAgent("web01"); err != nil {
		t.Fatal(err)
	}
	seedMedia(t, d, "web01", "iso-bytes")

	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// The hash opens the tab, so the dump shows the panel as an operator
	// clicking through would see it.
	cmd := exec.CommandContext(ctx, chrome,
		"--headless", "--disable-gpu", "--no-sandbox",
		"--virtual-time-budget=8000",
		"--dump-dom", srv.URL+"/disaster#baremetal",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("chromium: %v\n%s", err, out)
	}
	dom := string(out)

	if !strings.Contains(dom, "id=\"dr-panel-baremetal\"") {
		t.Fatalf("bare metal panel absent from the rendered DOM:\n%s", truncate(dom))
	}
	// The host row only exists if the script ran, fetched /api/bmr/hosts and
	// rendered the response.
	if !strings.Contains(dom, "web01") {
		t.Errorf("the host table never populated; the tab's script did not run:\n%s", truncate(dom))
	}
	if !strings.Contains(dom, "Download") {
		t.Errorf("no download action rendered for a host that has cached media:\n%s", truncate(dom))
	}
	if strings.Contains(dom, "Loading…") {
		t.Error("the host table is still showing its loading placeholder")
	}
}

func truncate(s string) string {
	if len(s) > 4000 {
		return s[:4000] + "\n…(truncated)"
	}
	return s
}
