// Package retention implements time-based snapshot retention policies.
// It is a pure algorithm package with no I/O dependencies.
package retention

import (
	"fmt"
	"sort"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
)

// Policy defines which snapshots to keep based on various retention rules.
// A snapshot is kept if ANY rule matches it (union semantics).
type Policy struct {
	KeepLast    int
	KeepHourly  int
	KeepDaily   int
	KeepWeekly  int
	KeepMonthly int
	KeepYearly  int
	KeepTags    []string
}

// IsEmpty returns true if no retention rules are defined.
func (p Policy) IsEmpty() bool {
	return p.KeepLast == 0 &&
		p.KeepHourly == 0 &&
		p.KeepDaily == 0 &&
		p.KeepWeekly == 0 &&
		p.KeepMonthly == 0 &&
		p.KeepYearly == 0 &&
		len(p.KeepTags) == 0
}

// SnapshotInfo holds the metadata needed for retention decisions.
type SnapshotInfo struct {
	ID       backend.ID
	Time     time.Time
	Hostname string
	Paths    []string
	Tags     []string
}

// Result contains the keep and forget lists after applying a retention policy.
type Result struct {
	Keep   []SnapshotInfo
	Forget []SnapshotInfo
}

// Apply evaluates the retention policy against the given snapshots and returns
// which to keep and which to forget. filterHost and filterPath restrict policy
// evaluation to matching snapshots; non-matching snapshots are always kept.
func Apply(snapshots []SnapshotInfo, policy Policy, filterHost string, filterPath string) (Result, error) {
	if policy.IsEmpty() {
		return Result{}, fmt.Errorf("retention: empty policy; at least one --keep-* flag is required")
	}

	// Partition into in-scope and out-of-scope.
	var inScope, outOfScope []SnapshotInfo
	for _, s := range snapshots {
		if matchesFilter(s, filterHost, filterPath) {
			inScope = append(inScope, s)
		} else {
			outOfScope = append(outOfScope, s)
		}
	}

	// Sort in-scope newest-first.
	sort.Slice(inScope, func(i, j int) bool {
		return inScope[i].Time.After(inScope[j].Time)
	})

	// Track which snapshots are kept (by index in inScope).
	keep := make(map[int]bool)

	// Apply keep-tag: unconditionally keep snapshots matching any tag.
	if len(policy.KeepTags) > 0 {
		tagSet := make(map[string]bool, len(policy.KeepTags))
		for _, t := range policy.KeepTags {
			tagSet[t] = true
		}
		for i, s := range inScope {
			for _, t := range s.Tags {
				if tagSet[t] {
					keep[i] = true
					break
				}
			}
		}
	}

	// Apply keep-last.
	if policy.KeepLast > 0 {
		for i := 0; i < len(inScope) && i < policy.KeepLast; i++ {
			keep[i] = true
		}
	}

	// Apply time-bucket rules.
	applyTimeBucket(inScope, keep, policy.KeepHourly, bucketHourly)
	applyTimeBucket(inScope, keep, policy.KeepDaily, bucketDaily)
	applyTimeBucket(inScope, keep, policy.KeepWeekly, bucketWeekly)
	applyTimeBucket(inScope, keep, policy.KeepMonthly, bucketMonthly)
	applyTimeBucket(inScope, keep, policy.KeepYearly, bucketYearly)

	// Build result.
	var result Result

	// Out-of-scope snapshots are always kept.
	result.Keep = append(result.Keep, outOfScope...)

	for i, s := range inScope {
		if keep[i] {
			result.Keep = append(result.Keep, s)
		} else {
			result.Forget = append(result.Forget, s)
		}
	}

	return result, nil
}

// matchesFilter returns true if the snapshot matches the host and path filters.
// Empty filters match everything.
func matchesFilter(s SnapshotInfo, filterHost string, filterPath string) bool {
	if filterHost != "" && s.Hostname != filterHost {
		return false
	}
	if filterPath != "" {
		found := false
		for _, p := range s.Paths {
			if p == filterPath {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// applyTimeBucket applies a time-bucket retention rule. It walks snapshots
// (sorted newest-first) and keeps the first snapshot in each unique bucket
// up to n buckets.
func applyTimeBucket(snapshots []SnapshotInfo, keep map[int]bool, n int, bucketFn func(time.Time) string) {
	if n <= 0 {
		return
	}

	seen := make(map[string]bool)
	for i, s := range snapshots {
		key := bucketFn(s.Time)
		if !seen[key] {
			seen[key] = true
			keep[i] = true
			if len(seen) >= n {
				return
			}
		}
	}
}

// Bucket key functions. Each returns a string that groups snapshots into
// the same time bucket.

func bucketHourly(t time.Time) string {
	return t.Format("2006-01-02-15")
}

func bucketDaily(t time.Time) string {
	return t.Format("2006-01-02")
}

func bucketWeekly(t time.Time) string {
	year, week := t.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

func bucketMonthly(t time.Time) string {
	return t.Format("2006-01")
}

func bucketYearly(t time.Time) string {
	return t.Format("2006")
}
