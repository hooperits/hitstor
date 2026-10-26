package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// MaintenanceLock coordinates exclusive access during maintenance operations
// (integrity check and prune). Only one maintenance operation can run per
// repository at a time.
type MaintenanceLock struct {
	mu          sync.Mutex
	locked      bool
	reason      string
	startTime   time.Time
	operationID string
}

// NewMaintenanceLock creates a new unlocked maintenance lock.
func NewMaintenanceLock() *MaintenanceLock {
	return &MaintenanceLock{}
}

// Acquire attempts to take the lock for the given reason ("check" or "prune").
// Returns an error if the lock is already held.
func (ml *MaintenanceLock) Acquire(reason string) (string, error) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	if ml.locked {
		return "", fmt.Errorf("server: maintenance lock held for %q since %s",
			ml.reason, ml.startTime.Format(time.RFC3339))
	}

	var idBytes [8]byte
	rand.Read(idBytes[:])
	opID := hex.EncodeToString(idBytes[:])

	ml.locked = true
	ml.reason = reason
	ml.startTime = time.Now()
	ml.operationID = opID
	return opID, nil
}

// Release releases the maintenance lock.
func (ml *MaintenanceLock) Release() {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	ml.locked = false
	ml.reason = ""
	ml.startTime = time.Time{}
	ml.operationID = ""
}

// IsLocked returns whether the lock is held and the reason.
func (ml *MaintenanceLock) IsLocked() (bool, string) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	return ml.locked, ml.reason
}

// OperationID returns the current operation ID, or empty if unlocked.
func (ml *MaintenanceLock) OperationID() string {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	return ml.operationID
}

// StartTime returns when the lock was acquired.
func (ml *MaintenanceLock) StartTime() time.Time {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	return ml.startTime
}
