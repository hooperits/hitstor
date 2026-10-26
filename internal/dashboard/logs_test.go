package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hooperits/hitstor/internal/logstore"
)

func TestHandleAPILogs(t *testing.T) {
	logstore.ResetForTest()
	t.Cleanup(logstore.ResetForTest)
	logstore.Append(logstore.Entry{
		Source:  "server",
		Level:   "info",
		Message: "test log entry",
	})

	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/logs?source=server&limit=10", nil)
	rec := httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp logsAPIResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Entries) == 0 {
		t.Fatal("expected at least one log entry")
	}
	found := false
	for _, e := range resp.Entries {
		if e.Message == "test log entry" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("entries = %+v, want test log entry", resp.Entries)
	}
	if resp.Summary.Total == 0 {
		t.Fatal("expected summary.total > 0")
	}
	if resp.SourcesAvailable.Server != true {
		t.Fatal("expected server source available")
	}
}
