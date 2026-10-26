package logstore

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRingBufferRespectsCapacity(t *testing.T) {
	ResetForTest()
	_ = ReloadConfig(Config{MaxEntries: 500, FileEnabled: false})

	for i := 0; i < 502; i++ {
		Append(Entry{Message: "m", Fields: map[string]string{"i": string(rune('0' + (i % 10)))}})
	}
	entries := Query(Filter{Limit: 600})
	if len(entries) != 500 {
		t.Fatalf("len = %d, want 500", len(entries))
	}
}

func TestRotatingFileCreatesBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.log")
	cfg := Config{
		MaxEntries:     1000,
		FileEnabled:    true,
		FilePath:       path,
		RotateMaxMB:    1,
		RotateMaxFiles: 2,
	}
	ResetForTest()
	if err := ReloadConfig(cfg); err != nil {
		t.Fatal(err)
	}
	line := bytes.Repeat([]byte("x"), 600*1024)
	for i := 0; i < 3; i++ {
		Append(Entry{Message: string(line), Source: "server"})
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected .1 backup: %v", err)
	}
}

func TestExportJSONLAndText(t *testing.T) {
	ResetForTest()
	Append(Entry{
		Time:    time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
		Level:   "warn",
		Source:  "m365",
		Message: "provision failed",
	})
	entries := Query(Filter{Limit: 10})
	var jbuf, tbuf bytes.Buffer
	if err := Export(&jbuf, entries, FormatJSONL); err != nil {
		t.Fatal(err)
	}
	if err := Export(&tbuf, entries, FormatText); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(jbuf.Bytes()[:len(jbuf.Bytes())-1]) {
		t.Fatalf("invalid jsonl: %s", jbuf.String())
	}
	if !bytes.Contains(tbuf.Bytes(), []byte("[WARN]")) {
		t.Fatalf("text = %s", tbuf.String())
	}
}

func TestTeeHandlerWritesToStore(t *testing.T) {
	ResetForTest()
	_ = ReloadConfig(Config{MaxEntries: 100, FileEnabled: false})
	base := slog.NewTextHandler(ioDiscard{}, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(NewTeeHandler(base, "server")))
	slog.Warn("hello tee")
	entries := Query(Filter{Query: "tee", Limit: 5})
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
