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

	protectChains(inScope, keep)

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

// protectChains keeps the backups that kept workload backups depend on.
//
// Workload snapshots carry "plugin:<id>" and "kind:<kind>" tags, and newer
// ones an "obj:<object>" tag per object they hold (a database, say), or
// "objects:none" when every object was left out. Snapshots without object
// tags (older ones) may hold any object of their host and plugin.
//
// Two rules:
//
//   - A snapshot the policy keeps also keeps, for every object of its host
//     and plugin, the newest snapshot holding that object at or before it:
//     what restores the object as of that point. Without this, keeping only
//     the newest log backups would forget the differentials of databases
//     that take no log backups (SIMPLE recovery). Only the policy's own
//     choices count, or every kept snapshot would keep an older one.
//   - A kept differential or log backup of an object keeps the earlier
//     backups of that object its kind needs (see chainNeeds), back to its
//     base: a full that is not copy-only ("copy-only" tag). A kept full,
//     copy-only or not, restores on its own. This rule runs until nothing
//     changes, since kept backups may hold other objects.
//
// inScope is sorted newest first.
func protectChains(inScope []SnapshotInfo, keep map[int]bool) {
	groups := make(map[string][]int) // host|plugin -> indices, newest first
	for i, s := range inScope {
		if p := tagValue(s.Tags, "plugin:"); p != "" {
			key := s.Hostname + "|" + p
			groups[key] = append(groups[key], i)
		}
	}
	mark := func(i int, changed *bool) {
		if !keep[i] {
			keep[i] = true
			*changed = true
		}
	}
	for _, idx := range groups {
		var objects []string // tagged objects of the group
		seen := map[string]bool{}
		for _, i := range idx {
			for _, o := range taggedObjects(inScope[i]) {
				if !seen[o] {
					seen[o] = true
					objects = append(objects, o)
				}
			}
		}
		var heads []int // positions the policy kept
		for pos, i := range idx {
			if keep[i] {
				heads = append(heads, pos)
			}
		}
		var unused bool
		for _, pos := range heads {
			for _, obj := range objects {
				for _, older := range idx[pos:] {
					if hasTag(inScope[older].Tags, "obj:"+obj) {
						mark(older, &unused)
						break
					}
				}
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, idx := range groups {
			for pos, i := range idx {
				if !keep[i] {
					continue
				}
				kind := tagValue(inScope[i].Tags, "kind:")
				if kind == "full" {
					continue
				}
				for _, obj := range objectsOf(inScope[i]) {
					for _, older := range idx[pos+1:] {
						o := inScope[older]
						if !holdsObject(o, obj) || !chainNeeds(kind, o) {
							continue
						}
						mark(older, &changed)
						// An older snapshot without object tags may not hold
						// this object: go on to the object's own base.
						if isChainBase(o) && (obj == "" || hasTag(o.Tags, "obj:"+obj)) {
							break
						}
					}
				}
			}
		}
	}
}

// chainNeeds reports whether a backup of kind needs the earlier backup o
// of the same object to restore: a differential needs only its base, a log
// the earlier logs and their base; other kinds keep everything back to the
// base.
func chainNeeds(kind string, o SnapshotInfo) bool {
	switch kind {
	case "differential":
		return isChainBase(o)
	case "log":
		return isChainBase(o) || tagValue(o.Tags, "kind:") == "log"
	}
	return true
}

// isChainBase reports whether s is a full workload backup that later ones
// can build on.
func isChainBase(s SnapshotInfo) bool {
	return tagValue(s.Tags, "kind:") == "full" && !hasTag(s.Tags, "copy-only")
}

// taggedObjects returns the objects s names in its tags.
func taggedObjects(s SnapshotInfo) []string {
	var objs []string
	for _, t := range s.Tags {
		if len(t) > 4 && t[:4] == "obj:" {
			objs = append(objs, t[4:])
		}
	}
	return objs
}

// objectsOf returns the objects s holds: none for "objects:none", "" for a
// snapshot that does not say (it may hold any object of its group).
func objectsOf(s SnapshotInfo) []string {
	if hasTag(s.Tags, "objects:none") {
		return nil
	}
	if objs := taggedObjects(s); len(objs) > 0 {
		return objs
	}
	return []string{""}
}

// holdsObject reports whether s may hold obj ("" matches every snapshot
// that may hold objects).
func holdsObject(s SnapshotInfo, obj string) bool {
	objs := objectsOf(s)
	if len(objs) == 0 {
		return false
	}
	return obj == "" || objs[0] == "" || hasTag(s.Tags, "obj:"+obj)
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func tagValue(tags []string, prefix string) string {
	for _, t := range tags {
		if len(t) > len(prefix) && t[:len(prefix)] == prefix {
			return t[len(prefix):]
		}
	}
	return ""
}
