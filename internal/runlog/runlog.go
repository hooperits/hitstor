// Package runlog keeps the history of backup runs: each run that ends,
// well, partly or with an error, is one record. Jobs keep only their latest
// outcome; reports need the whole period, failures included.
//
// Records go to one file per month (runs-2026-10.log) next to the agent
// registry, one encrypted record per line, so appending never rewrites the
// file. Months older than Retention are deleted.
package runlog

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/crypto"
)

// Retention is how long runs are kept: a year and the month before, so a
// yearly report can compare with the same month a year ago.
const Retention = 13 * 30 * 24 * time.Hour

// Results of a run.
const (
	ResultOK      = "ok"
	ResultPartial = "partial" // stored, but some objects failed
	ResultFailed  = "failed"
)

// Run is one finished backup run.
type Run struct {
	Time    time.Time `json:"time"` // when it ended
	JobID   string    `json:"job_id,omitempty"`
	JobName string    `json:"job_name,omitempty"`
	Agent   string    `json:"agent"`
	Repo    string    `json:"repo,omitempty"`
	Plugin  string    `json:"plugin,omitempty"` // workload plugin; empty for file jobs
	Kind    string    `json:"kind,omitempty"`   // full, differential, log...; empty for file jobs
	Result  string    `json:"result"`
	Error   string    `json:"error,omitempty"`

	SnapshotID     string  `json:"snapshot_id,omitempty"`
	FilesNew       uint64  `json:"files_new,omitempty"`
	BytesProcessed uint64  `json:"bytes_processed,omitempty"`
	BytesAdded     uint64  `json:"bytes_added,omitempty"`
	Duration       float64 `json:"duration,omitempty"` // seconds

	// FailedObjects are the objects that failed in a partial or failed
	// run, as "object: reason".
	FailedObjects []string `json:"failed_objects,omitempty"`
}

// Log is the run history in a directory.
type Log struct {
	dir string
	key [32]byte
	now func() time.Time // tests

	mu     sync.Mutex
	pruned string // month last pruned at
}

// Open returns the history kept in dir, encrypted with key. The first
// time, it notes when the history starts.
func Open(dir string, key [32]byte) *Log {
	l := &Log{dir: dir, key: key, now: time.Now}
	marker := filepath.Join(dir, "runs-since")
	if _, err := os.Stat(marker); errors.Is(err, os.ErrNotExist) {
		os.WriteFile(marker, []byte(l.now().UTC().Format(time.RFC3339)+"\n"), 0o600)
	}
	return l
}

func monthFile(t time.Time) string { return "runs-" + t.UTC().Format("2006-01") + ".log" }

// Append records a run.
func (l *Log) Append(r Run) error {
	if r.Time.IsZero() {
		r.Time = l.now()
	}
	r.Time = r.Time.UTC()
	if len(r.Error) > 2000 {
		r.Error = r.Error[:2000]
	}
	plain, err := json.Marshal(r)
	if err != nil {
		return err
	}
	enc, err := crypto.Encrypt(l.key, plain)
	if err != nil {
		return err
	}
	line := base64.StdEncoding.EncodeToString(enc) + "\n"

	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(l.dir, monthFile(r.Time)), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(line); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if m := monthFile(l.now()); m != l.pruned {
		l.prune()
		l.pruned = m
	}
	return nil
}

// prune deletes the months older than Retention.
func (l *Log) prune() {
	oldest := monthFile(l.now().Add(-Retention))
	files, _ := filepath.Glob(filepath.Join(l.dir, "runs-*.log"))
	for _, f := range files {
		if filepath.Base(f) < oldest {
			os.Remove(f)
		}
	}
}

// Since returns when the history starts: when it was first opened, or the
// oldest run kept if older. Zero when unknown.
func (l *Log) Since() time.Time {
	var since time.Time
	if raw, err := os.ReadFile(filepath.Join(l.dir, "runs-since")); err == nil {
		since, _ = time.Parse(time.RFC3339, strings.TrimSpace(string(raw)))
	}
	if first := l.oldest(); !first.IsZero() && (since.IsZero() || first.Before(since)) {
		since = first
	}
	return since
}

func (l *Log) oldest() time.Time {
	files, _ := filepath.Glob(filepath.Join(l.dir, "runs-*.log"))
	sort.Strings(files)
	for _, f := range files {
		runs, err := l.read(f)
		if err == nil && len(runs) > 0 {
			return runs[0].Time
		}
	}
	return time.Time{}
}

// Range returns the runs that ended in [from, to), oldest first. A
// damaged line is skipped, not fatal: one bad write must not hide a year.
func (l *Log) Range(from, to time.Time) ([]Run, error) {
	var out []Run
	for m := time.Date(from.UTC().Year(), from.UTC().Month(), 1, 0, 0, 0, 0, time.UTC); m.Before(to); m = m.AddDate(0, 1, 0) {
		runs, err := l.read(filepath.Join(l.dir, monthFile(m)))
		if err != nil {
			return nil, err
		}
		for _, r := range runs {
			if !r.Time.Before(from) && r.Time.Before(to) {
				out = append(out, r)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}

func (l *Log) read(path string) ([]Run, error) {
	l.mu.Lock()
	data, err := os.ReadFile(path)
	l.mu.Unlock()
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("runlog: %w", err)
	}
	var out []Run
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		enc, err := base64.StdEncoding.DecodeString(line)
		if err != nil {
			continue
		}
		plain, err := crypto.Decrypt(l.key, enc)
		if err != nil {
			continue
		}
		var r Run
		if json.Unmarshal(plain, &r) == nil {
			out = append(out, r)
		}
	}
	return out, nil
}
