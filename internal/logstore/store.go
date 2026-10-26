package logstore

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"
)

var (
	globalMu sync.RWMutex
	global   = &Store{cfg: DefaultConfig(), ring: newRingBuffer(DefaultConfig().MaxEntries)}
)

// Store holds the in-memory ring and optional rotating file writer.
type Store struct {
	cfg  Config
	ring *ringBuffer
	file *rotatingFile
}

// ReloadConfig applies new retention settings at runtime.
func ReloadConfig(cfg Config) error {
	cfg = cfg.Normalize()
	globalMu.Lock()
	defer globalMu.Unlock()
	if global.file != nil {
		_ = global.file.close()
		global.file = nil
	}
	global.cfg = cfg
	global.ring.resize(cfg.MaxEntries)
	if cfg.FileEnabled {
		f, err := newRotatingFile(cfg)
		if err != nil {
			return err
		}
		global.file = f
	}
	return nil
}

// Config returns the active configuration.
func ConfigSnapshot() Config {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return global.cfg
}

// Append records a log entry (server/m365/system ingestion).
func Append(e Entry) {
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if e.Source == "" {
		e.Source = "server"
	}
	globalMu.RLock()
	s := global
	globalMu.RUnlock()

	s.ring.append(e)
	if s.file != nil {
		b, _ := json.Marshal(e)
		_ = s.file.writeLine(b)
	}
}

// Query returns matching entries newest-first then sorted ascending by time.
func Query(f Filter) []Entry {
	globalMu.RLock()
	s := global
	globalMu.RUnlock()
	return s.ring.query(f)
}

// TeeHandler wraps a base slog handler and mirrors records into the store.
type TeeHandler struct {
	base   slog.Handler
	source string
}

// NewTeeHandler creates a handler that writes to base and logstore.
func NewTeeHandler(base slog.Handler, source string) *TeeHandler {
	if source == "" {
		source = "server"
	}
	return &TeeHandler{base: base, source: source}
}

func (h *TeeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *TeeHandler) Handle(ctx context.Context, r slog.Record) error {
	fields := make(map[string]string)
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = attrString(a)
		return true
	})
	component := fields["component"]
	delete(fields, "component")
	entry := Entry{
		Time:      r.Time.UTC(),
		Level:     strings.ToLower(r.Level.String()),
		Source:    h.source,
		Component: component,
		Message:   r.Message,
		Fields:    fields,
	}
	if !IsNoise(entry) {
		Append(entry)
	}
	return h.base.Handle(ctx, r)
}

func (h *TeeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TeeHandler{base: h.base.WithAttrs(attrs), source: h.source}
}

func (h *TeeHandler) WithGroup(name string) slog.Handler {
	return &TeeHandler{base: h.base.WithGroup(name), source: h.source}
}

func attrString(a slog.Attr) string {
	if a.Equal(slog.Attr{}) {
		return ""
	}
	if a.Value.Kind() == slog.KindString {
		return a.Value.String()
	}
	return a.Value.String()
}

// ResetForTest clears the global store (tests only).
func ResetForTest() {
	globalMu.Lock()
	defer globalMu.Unlock()
	if global.file != nil {
		_ = global.file.close()
	}
	cfg := DefaultConfig()
	global = &Store{cfg: cfg, ring: newRingBuffer(cfg.MaxEntries)}
}

// InitFromConfig initializes global store from config at process start.
func InitFromConfig(cfg Config) error {
	return ReloadConfig(cfg.Normalize())
}
