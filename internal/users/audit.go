package users

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEvent is one line of the audit log: who did what, from where, and
// whether it worked. Each line carries the hash of the previous one, so an
// edited or removed line breaks the chain.
type AuditEvent struct {
	Time   time.Time `json:"time"`
	User   string    `json:"user,omitempty"`
	IP     string    `json:"ip,omitempty"`
	Action string    `json:"action"`
	Target string    `json:"target,omitempty"`
	OK     bool      `json:"ok"`
	Detail string    `json:"detail,omitempty"`
	Prev   string    `json:"prev"`
}

// AuditLog appends events to a file that is only ever appended to.
type AuditLog struct {
	mu   sync.Mutex
	path string
	last string // hash of the last line
	size int64  // file size after the last line; another process appended if it differs
}

// OpenAuditLog opens (or creates) the audit log in dir.
func OpenAuditLog(dir string) (*AuditLog, error) {
	a := &AuditLog{path: filepath.Join(dir, "audit.log")}
	if err := a.scan(); err != nil {
		return nil, err
	}
	return a, nil
}

// scan finds the hash of the last line.
func (a *AuditLog) scan() error {
	a.last, a.size = "", 0
	f, err := os.Open(a.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		a.last = lineHash(sc.Bytes())
		a.size += int64(len(sc.Bytes())) + 1
	}
	return sc.Err()
}

func lineHash(line []byte) string {
	h := sha256.Sum256(line)
	return hex.EncodeToString(h[:])
}

// Append writes e, chained to the previous line.
func (a *AuditLog) Append(e AuditEvent) error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	// The CLI on the server console appends too: chain to its lines.
	if fi, err := os.Stat(a.path); err == nil && fi.Size() != a.size {
		if err := a.scan(); err != nil {
			return err
		}
	}
	e.Prev = a.last
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	a.last = lineHash(line)
	a.size += int64(len(line)) + 1
	return nil
}

// Recent returns up to n events, newest first, and whether the whole chain
// is intact (brokenAt is the 1-based line where it breaks, or 0).
func (a *AuditLog) Recent(n int) (events []AuditEvent, brokenAt int, err error) {
	events, _, brokenAt, err = a.Query(nil, 0, n)
	return events, brokenAt, err
}

// Query returns the events match accepts (all, if nil), newest first,
// skipping offset and returning at most limit, with how many matched in
// all. The whole chain is checked: brokenAt is the 1-based line where it
// breaks, or 0.
func (a *AuditLog) Query(match func(AuditEvent) bool, offset, limit int) (page []AuditEvent, total, brokenAt int, err error) {
	if a == nil {
		return nil, 0, 0, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.Open(a.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, 0, nil
	}
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	var matched []AuditEvent
	prev, line := "", 0
	for sc.Scan() {
		line++
		var e AuditEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, 0, 0, fmt.Errorf("audit log line %d: %w", line, err)
		}
		if e.Prev != prev && brokenAt == 0 {
			brokenAt = line
		}
		prev = lineHash(sc.Bytes())
		if match == nil || match(e) {
			matched = append(matched, e)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, 0, 0, err
	}
	total = len(matched)
	// Newest first.
	for i := total - 1 - offset; i >= 0 && len(page) < limit; i-- {
		page = append(page, matched[i])
	}
	return page, total, brokenAt, nil
}
