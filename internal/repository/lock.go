package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
)

// LockStaleTimeout is how long a lock from another host may be held before considered stale.
const LockStaleTimeout = 30 * time.Minute

// ErrRepositoryLocked is returned when another process holds a valid lock.
var ErrRepositoryLocked = errors.New("repository is locked by another process")

// LockInfo describes an active repository lock.
type LockInfo struct {
	Hostname  string    `json:"hostname"`
	PID       int       `json:"pid"`
	UID       uint32    `json:"uid"`
	Exclusive bool      `json:"exclusive"`
	Time      time.Time `json:"time"`
	LockID    string    `json:"lock_id"`
}

// LockOptions controls repository lock acquisition.
type LockOptions struct {
	Exclusive  bool
	BreakStale bool
}

// Lock acquires an exclusive or shared repository lock.
func (r *Repository) Lock(exclusive bool) error {
	return r.LockWithOptions(LockOptions{Exclusive: exclusive})
}

// LockWithOptions acquires a lock with stale-lock handling.
func (r *Repository) LockWithOptions(opts LockOptions) error {
	r.lockMu.Lock()
	defer r.lockMu.Unlock()

	if r.lockID != nil {
		if opts.Exclusive {
			r.lockRefCount++
			return nil
		}
		return fmt.Errorf("repository: already locked by this process")
	}

	ctx := context.Background()
	existing, err := r.listLocks(ctx)
	if err != nil {
		return err
	}

	var stale []backend.ID
	for _, entry := range existing {
		if isLockStale(entry.lock) {
			if opts.BreakStale {
				stale = append(stale, entry.id)
				continue
			}
			return fmt.Errorf("%w: stale lock from %s pid %d since %s (use --break-lock)",
				ErrRepositoryLocked, entry.lock.Hostname, entry.lock.PID, entry.lock.Time.Format(time.RFC3339))
		}
		if opts.Exclusive || entry.lock.Exclusive {
			return fmt.Errorf("%w: held by %s pid %d since %s",
				ErrRepositoryLocked, entry.lock.Hostname, entry.lock.PID, entry.lock.Time.Format(time.RFC3339))
		}
	}

	for _, id := range stale {
		if err := r.be.Remove(ctx, backend.LockFile, id); err != nil {
			return fmt.Errorf("repository: remove stale lock: %w", err)
		}
	}

	hostname, _ := os.Hostname()
	lock := Lock{
		Time:      time.Now(),
		Hostname:  hostname,
		PID:       os.Getpid(),
		UID:       uint32(os.Getuid()),
		Exclusive: opts.Exclusive,
	}

	lockData, err := json.Marshal(lock)
	if err != nil {
		return fmt.Errorf("repository: marshal lock: %w", err)
	}

	lockID := ID(sha256.Sum256(lockData))
	if err := r.be.Save(ctx, backend.LockFile, lockID, bytes.NewReader(lockData)); err != nil {
		return fmt.Errorf("repository: save lock: %w", err)
	}

	r.lockID = &lockID
	r.lockRefCount = 1

	if fl, ok := r.be.(interface {
		AcquireFlock() (*os.File, error)
	}); ok {
		f, err := fl.AcquireFlock()
		if err != nil {
			_ = r.Unlock()
			return fmt.Errorf("repository: flock: %w", err)
		}
		if f != nil {
			r.flockFile = f
		}
	}

	return nil
}

type lockEntry struct {
	id   backend.ID
	lock Lock
}

func (r *Repository) listLocks(ctx context.Context) ([]lockEntry, error) {
	lockIDs, err := r.be.List(ctx, backend.LockFile)
	if err != nil {
		return nil, fmt.Errorf("repository: list locks: %w", err)
	}

	var entries []lockEntry
	for _, lockID := range lockIDs {
		rd, err := r.be.Load(ctx, backend.LockFile, lockID)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rd)
		rd.Close()
		if err != nil {
			continue
		}
		var lock Lock
		if err := json.Unmarshal(data, &lock); err != nil {
			continue
		}
		entries = append(entries, lockEntry{id: lockID, lock: lock})
	}
	return entries, nil
}

func isLockStale(lock Lock) bool {
	hostname, _ := os.Hostname()
	if lock.Hostname == hostname && processAlive(lock.PID) {
		return false
	}
	return time.Since(lock.Time) > LockStaleTimeout
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// ListLockInfo returns metadata about current repository locks.
func (r *Repository) ListLockInfo() ([]LockInfo, error) {
	ctx := context.Background()
	entries, err := r.listLocks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LockInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, LockInfo{
			Hostname:  e.lock.Hostname,
			PID:       e.lock.PID,
			UID:       e.lock.UID,
			Exclusive: e.lock.Exclusive,
			Time:      e.lock.Time,
			LockID:    e.id.String(),
		})
	}
	return out, nil
}

// ConfigExists reports whether the repository config file is present.
func ConfigExists(be backend.Backend) (bool, error) {
	ctx := context.Background()
	_, err := be.Stat(ctx, backend.ConfigFile, ID{})
	if err != nil {
		return false, nil
	}
	return true, nil
}
