package runlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendAndRange(t *testing.T) {
	dir := t.TempDir()
	l := Open(dir, [32]byte{3})
	base := time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC)
	for i, res := range []string{ResultOK, ResultFailed, ResultPartial, ResultOK} {
		r := Run{Time: base.Add(time.Duration(i) * time.Hour), Agent: "sql01", JobName: "SQL", Result: res, Kind: "log"}
		if res == ResultFailed {
			r.Error = "VDI timeout"
		}
		if err := l.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	// Two months, one file each.
	for _, m := range []string{"runs-2026-09.log", "runs-2026-10.log"} {
		raw, err := os.ReadFile(filepath.Join(dir, m))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "sql01") {
			t.Fatal("runs stored in clear text")
		}
	}
	runs, err := l.Range(base.Add(30*time.Minute), base.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].Result != ResultFailed || runs[0].Error != "VDI timeout" || runs[1].Result != ResultPartial {
		t.Fatalf("range: %+v", runs)
	}
	if got := l.Since(); !got.Equal(base) {
		t.Fatalf("since = %v", got)
	}
	// Another key reads nothing; a damaged line is skipped.
	if runs, _ := Open(dir, [32]byte{4}).Range(base, base.Add(24*time.Hour)); len(runs) != 0 {
		t.Fatal("read with another key")
	}
	f, _ := os.OpenFile(filepath.Join(dir, "runs-2026-10.log"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("not base64!\n")
	f.Close()
	if runs, _ := l.Range(base, base.Add(24*time.Hour)); len(runs) != 4 {
		t.Fatalf("damaged line: %d runs", len(runs))
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	l := Open(dir, [32]byte{3})
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }
	old := filepath.Join(dir, "runs-2025-08.log")
	keep := filepath.Join(dir, "runs-2025-10.log")
	os.WriteFile(old, nil, 0o600)
	os.WriteFile(keep, nil, 0o600)
	if err := l.Append(Run{Agent: "a", Result: ResultOK}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("a month past retention was kept")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("a month within retention was deleted")
	}
}
