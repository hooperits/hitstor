package repository

import (
	"fmt"

	localbackend "github.com/hooperits/hitstor/internal/backend/local"
)

// HealthReport summarizes repository operational health.
type HealthReport struct {
	IndexCount      int        `json:"index_count"`
	SnapshotCount   int        `json:"snapshot_count"`
	LockCount       int        `json:"lock_count"`
	Locks           []LockInfo `json:"locks,omitempty"`
	FreeBytes       uint64     `json:"free_bytes,omitempty"`
	TotalBytes      uint64     `json:"total_bytes,omitempty"`
	LatestIndexID   string     `json:"latest_index_id,omitempty"`
	DeltaIndexCount int        `json:"delta_index_count,omitempty"`
	MinFreeReserve  uint64     `json:"min_free_reserve_bytes"`
	SyncWrites      bool       `json:"sync_writes"`
}

// Health returns a snapshot of repository metadata for monitoring.
func (r *Repository) Health() (HealthReport, error) {
	var report HealthReport

	indexIDs, err := r.ListIndexes()
	if err != nil {
		return report, err
	}
	report.IndexCount = len(indexIDs)

	snapIDs, err := r.ListSnapshots()
	if err != nil {
		return report, err
	}
	report.SnapshotCount = len(snapIDs)

	locks, err := r.ListLockInfo()
	if err != nil {
		return report, err
	}
	report.Locks = locks
	report.LockCount = len(locks)

	report.MinFreeReserve = r.minFreeRequired()

	if lb, ok := r.be.(*localbackend.Local); ok {
		report.SyncWrites = lb.SyncWritesEnabled()
		total, free, err := lb.DiskUsage()
		if err == nil {
			report.TotalBytes = total
			report.FreeBytes = free
		}
	}

	if latestID, hasLatest, err := ReadLatestIndexID(r.be); err == nil && hasLatest {
		report.LatestIndexID = latestID.String()
		if report.IndexCount > 0 {
			report.DeltaIndexCount = report.IndexCount - 1
		}
	}

	return report, nil
}

// MinFreeReserveBytes is the default minimum free space required before writing packs.
const MinFreeReserveBytes = 1 * 1024 * 1024 * 1024

// CheckFreeSpace returns an error if the local filesystem has less than minFree bytes available.
func (r *Repository) CheckFreeSpace(minFree uint64) error {
	lb, ok := r.be.(*localbackend.Local)
	if !ok {
		return nil
	}
	_, free, err := lb.DiskUsage()
	if err != nil {
		return err
	}
	if free < minFree {
		return fmt.Errorf("repository: insufficient disk space (%d bytes free, need %d)", free, minFree)
	}
	return nil
}
