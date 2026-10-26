package server

import (
	"sync"
	"testing"
)

func TestMaintenanceLockAcquireRelease(t *testing.T) {
	ml := NewMaintenanceLock()

	locked, reason := ml.IsLocked()
	if locked {
		t.Error("expected unlocked initially")
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}

	opID, err := ml.Acquire("check")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if opID == "" {
		t.Error("expected non-empty operation ID")
	}

	locked, reason = ml.IsLocked()
	if !locked {
		t.Error("expected locked after Acquire")
	}
	if reason != "check" {
		t.Errorf("reason = %q, want %q", reason, "check")
	}
	if ml.OperationID() != opID {
		t.Errorf("OperationID = %q, want %q", ml.OperationID(), opID)
	}
	if ml.StartTime().IsZero() {
		t.Error("expected non-zero start time")
	}

	ml.Release()

	locked, reason = ml.IsLocked()
	if locked {
		t.Error("expected unlocked after Release")
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
	if ml.OperationID() != "" {
		t.Errorf("OperationID = %q, want empty", ml.OperationID())
	}
}

func TestMaintenanceLockDoubleAcquire(t *testing.T) {
	ml := NewMaintenanceLock()

	if _, err := ml.Acquire("check"); err != nil {
		t.Fatal(err)
	}

	_, err := ml.Acquire("prune")
	if err == nil {
		t.Fatal("expected error for double Acquire")
	}
}

func TestMaintenanceLockAcquireAfterRelease(t *testing.T) {
	ml := NewMaintenanceLock()

	if _, err := ml.Acquire("check"); err != nil {
		t.Fatal(err)
	}
	ml.Release()

	opID, err := ml.Acquire("prune")
	if err != nil {
		t.Fatalf("re-Acquire after Release: %v", err)
	}
	if opID == "" {
		t.Error("expected non-empty operation ID")
	}

	locked, reason := ml.IsLocked()
	if !locked {
		t.Error("expected locked")
	}
	if reason != "prune" {
		t.Errorf("reason = %q, want %q", reason, "prune")
	}
}

func TestMaintenanceLockUniqueOperationIDs(t *testing.T) {
	ml := NewMaintenanceLock()

	opID1, err := ml.Acquire("check")
	if err != nil {
		t.Fatal(err)
	}
	ml.Release()

	opID2, err := ml.Acquire("prune")
	if err != nil {
		t.Fatal(err)
	}
	ml.Release()

	if opID1 == opID2 {
		t.Error("expected unique operation IDs across acquires")
	}
}

func TestMaintenanceLockConcurrentAcquire(t *testing.T) {
	ml := NewMaintenanceLock()

	const goroutines = 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if _, err := ml.Acquire("check"); err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("successful acquires = %d, want exactly 1", successCount)
	}
}

func TestMaintenanceLockReleaseIdempotent(t *testing.T) {
	ml := NewMaintenanceLock()

	// Release on unlocked lock should not panic.
	ml.Release()
	ml.Release()

	locked, _ := ml.IsLocked()
	if locked {
		t.Error("expected unlocked")
	}
}
