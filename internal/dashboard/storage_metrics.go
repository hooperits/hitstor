package dashboard

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
)

const (
	metricsCacheTTL     = 60 * time.Second
	snapshotLoadWorkers = 8
)

type cachedSnapshotSummary struct {
	ID             string
	Time           time.Time
	Hostname       string
	Tags           []string
	Paths          []string
	Type           string
	ParentSnapshot string
	Summary        repository.SnapshotSummary
}

type metricsCacheEntry struct {
	metrics  repoMetricsJSON
	cachedAt time.Time
}

func (d *Dashboard) invalidateRepoMetricsCache(repoName string) {
	d.metricsCacheMu.Lock()
	delete(d.metricsCache, repoName)
	d.metricsCacheMu.Unlock()
}

func (d *Dashboard) invalidateSnapshotCache(repoName string) {
	d.snapshotCacheMu.Lock()
	delete(d.snapshotCache, repoName)
	d.snapshotCacheMu.Unlock()
	d.invalidateRepoMetricsCache(repoName)
}

func (d *Dashboard) latestSnapshotInfo(mr *server.ManagedRepo) (timeStr, agent string) {
	summaries, err := d.getSnapshotSummaries(mr)
	if err != nil || len(summaries) == 0 {
		return "", ""
	}
	var latest cachedSnapshotSummary
	for _, cs := range summaries {
		if latest.Time.IsZero() || cs.Time.After(latest.Time) {
			latest = cs
		}
	}
	if latest.Time.IsZero() {
		return "", ""
	}
	return latest.Time.Format(time.RFC3339), latest.Hostname
}

// snapshotBackupType classifies a snapshot for restore filtering.
// All snapshots are filesystem backups.
func (d *Dashboard) snapshotBackupType(cs cachedSnapshotSummary) string {
	return "files"
}

// snapshotMatchesAgent reports whether a snapshot belongs to the given registered agent.
func (d *Dashboard) snapshotMatchesAgent(cs cachedSnapshotSummary, agentName string) bool {
	if strings.EqualFold(cs.Hostname, agentName) {
		return true
	}
	for _, tag := range cs.Tags {
		if strings.EqualFold(tag, agentName) {
			return true
		}
	}
	if d.registry != nil {
		for _, job := range d.registry.ListJobsByAgent(agentName) {
			if strings.EqualFold(job.Name, cs.Hostname) {
				return true
			}
			for _, tag := range cs.Tags {
				if strings.EqualFold(tag, job.Name) {
					return true
				}
			}
		}
	}
	return false
}

func repoHealthStatus(mr *server.ManagedRepo) string {
	report, err := mr.Repo.Health()
	if err != nil {
		return "error"
	}
	if report.LockCount > 0 {
		return "warning"
	}
	return "ok"
}

func (d *Dashboard) getSnapshotSummaries(mr *server.ManagedRepo) ([]cachedSnapshotSummary, error) {
	snapIDs, err := mr.Repo.ListSnapshots()
	if err != nil {
		return nil, err
	}

	current := make(map[string]bool, len(snapIDs))
	for _, id := range snapIDs {
		current[id.String()] = true
	}

	d.snapshotCacheMu.Lock()
	if d.snapshotCache == nil {
		d.snapshotCache = make(map[string]map[string]*cachedSnapshotSummary)
	}
	repoCache := d.snapshotCache[mr.Name]
	if repoCache == nil {
		repoCache = make(map[string]*cachedSnapshotSummary)
		d.snapshotCache[mr.Name] = repoCache
	}

	var missing []repository.ID
	for idStr := range repoCache {
		if !current[idStr] {
			delete(repoCache, idStr)
		}
	}
	for _, id := range snapIDs {
		if _, ok := repoCache[id.String()]; !ok {
			missing = append(missing, id)
		}
	}
	d.snapshotCacheMu.Unlock()

	if len(missing) > 0 {
		d.loadSnapshotsParallel(mr, missing)
	}

	result := make([]cachedSnapshotSummary, 0, len(snapIDs))
	d.snapshotCacheMu.Lock()
	repoCache = d.snapshotCache[mr.Name]
	for _, id := range snapIDs {
		if cs, ok := repoCache[id.String()]; ok {
			result = append(result, *cs)
		}
	}
	d.snapshotCacheMu.Unlock()

	return result, nil
}

func (d *Dashboard) loadSnapshotsParallel(mr *server.ManagedRepo, ids []repository.ID) {
	type loaded struct {
		idStr string
		cs    *cachedSnapshotSummary
	}

	jobs := make(chan repository.ID, len(ids))
	results := make(chan loaded, len(ids))

	workers := snapshotLoadWorkers
	if workers > len(ids) {
		workers = len(ids)
	}
	if workers == 0 {
		return
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				snap, err := mr.Repo.LoadSnapshot(id)
				if err != nil {
					continue
				}
				snapType := snap.Type
				if snapType == "" {
					snapType = "full"
				}
				parentSnap := snap.ParentSnapshot
				if len(parentSnap) > 8 {
					parentSnap = parentSnap[:8]
				}
				results <- loaded{
					idStr: id.String(),
					cs: &cachedSnapshotSummary{
						ID:             id.String(),
						Time:           snap.Time,
						Hostname:       snap.Hostname,
						Tags:           snap.Tags,
						Paths:          snap.Paths,
						Type:           snapType,
						ParentSnapshot: parentSnap,
						Summary:        snap.Summary,
					},
				}
			}
		}()
	}

	for _, id := range ids {
		jobs <- id
	}
	close(jobs)
	wg.Wait()
	close(results)

	d.snapshotCacheMu.Lock()
	if d.snapshotCache == nil {
		d.snapshotCache = make(map[string]map[string]*cachedSnapshotSummary)
	}
	if d.snapshotCache[mr.Name] == nil {
		d.snapshotCache[mr.Name] = make(map[string]*cachedSnapshotSummary)
	}
	for item := range results {
		d.snapshotCache[mr.Name][item.idStr] = item.cs
	}
	d.snapshotCacheMu.Unlock()
}

func (d *Dashboard) computeStorageMetricsSummary(mr *server.ManagedRepo, agentJobCounts map[string]int) repoMetricsJSON {
	ctx := context.Background()
	snapIDs, _ := mr.Repo.ListSnapshots()
	packIDs, _ := mr.Backend.List(ctx, backend.DataFile)
	indexIDs, _ := mr.Backend.List(ctx, backend.IndexFile)

	var diskTotal, diskFree uint64
	var diskUsedPercent float64
	if du, ok := mr.Backend.(interface {
		DiskUsage() (uint64, uint64, error)
	}); ok {
		if t, f, err := du.DiskUsage(); err == nil {
			diskTotal = t
			diskFree = f
			if diskTotal > 0 {
				diskUsedPercent = float64(diskTotal-diskFree) / float64(diskTotal) * 100
			}
		}
	}

	alertLevel := "ok"
	if diskTotal > 0 {
		if diskUsedPercent >= 90 {
			alertLevel = "critical"
		} else if diskUsedPercent >= 80 {
			alertLevel = "warning"
		}
	}

	locked, lockReason := mr.MaintenanceLock.IsLocked()

	repoType := mr.Config.Type
	if repoType == "" {
		repoType = "local"
	}
	location := mr.Config.Path
	if repoType == "s3" {
		location = fmt.Sprintf("s3://%s@%s", mr.Config.Bucket, mr.Config.Endpoint)
	}
	if location == "" {
		if bp, ok := mr.Backend.(interface{ BasePath() string }); ok {
			location = bp.BasePath()
		}
	}

	return repoMetricsJSON{
		Name:            mr.Name,
		Type:            repoType,
		Location:        location,
		Snapshots:       len(snapIDs),
		Packs:           len(packIDs),
		Indexes:         len(indexIDs),
		DiskTotal:       diskTotal,
		DiskFree:        diskFree,
		DiskUsedPercent: diskUsedPercent,
		AlertLevel:      alertLevel,
		IsLocked:        locked,
		LockReason:      lockReason,
		BackupJobCount:  agentJobCounts[mr.Name],
		MetricsLoaded:   false,
	}
}

func (d *Dashboard) computeStorageMetricsFull(mr *server.ManagedRepo, agentJobCounts map[string]int) repoMetricsJSON {
	ctx := context.Background()

	snapIDs, _ := mr.Repo.ListSnapshots()
	packIDs, _ := mr.Backend.List(ctx, backend.DataFile)
	indexIDs, _ := mr.Backend.List(ctx, backend.IndexFile)

	var totalBytes uint64
	totalBytes += d.getFileSizes(ctx, mr.Backend, mr.Name, backend.DataFile, packIDs)
	totalBytes += d.getFileSizes(ctx, mr.Backend, mr.Name, backend.IndexFile, indexIDs)
	totalBytes += d.getFileSizes(ctx, mr.Backend, mr.Name, backend.SnapshotFile, snapIDs)

	summaries, _ := d.getSnapshotSummaries(mr)
	var sumBytesProcessed, sumBytesAdded uint64
	for _, cs := range summaries {
		sumBytesProcessed += cs.Summary.BytesProcessed
		sumBytesAdded += cs.Summary.BytesAdded
	}

	var dedupRatio, compressionRatio float64
	if sumBytesAdded > 0 {
		dedupRatio = float64(sumBytesProcessed) / float64(sumBytesAdded)
	}
	packBytes := totalBytes
	if packBytes > 0 && sumBytesAdded > 0 {
		compressionRatio = float64(sumBytesAdded) / float64(packBytes)
	}

	var diskTotal, diskFree uint64
	var diskUsedPercent float64
	if du, ok := mr.Backend.(interface {
		DiskUsage() (uint64, uint64, error)
	}); ok {
		if t, f, err := du.DiskUsage(); err == nil {
			diskTotal = t
			diskFree = f
			if diskTotal > 0 {
				diskUsedPercent = float64(diskTotal-diskFree) / float64(diskTotal) * 100
			}
		}
	}

	alertLevel := "ok"
	if diskTotal > 0 {
		if diskUsedPercent >= 90 {
			alertLevel = "critical"
		} else if diskUsedPercent >= 80 {
			alertLevel = "warning"
		}
	}

	locked, lockReason := mr.MaintenanceLock.IsLocked()

	repoType := mr.Config.Type
	if repoType == "" {
		repoType = "local"
	}
	location := mr.Config.Path
	if repoType == "s3" {
		location = fmt.Sprintf("s3://%s@%s", mr.Config.Bucket, mr.Config.Endpoint)
	}
	if location == "" {
		if bp, ok := mr.Backend.(interface{ BasePath() string }); ok {
			location = bp.BasePath()
		}
	}

	lastSnapTime, lastSnapAgent := d.latestSnapshotInfo(mr)

	return repoMetricsJSON{
		Name:              mr.Name,
		Type:              repoType,
		Location:          location,
		Snapshots:         len(snapIDs),
		Packs:             len(packIDs),
		Indexes:           len(indexIDs),
		TotalBytes:        totalBytes,
		DiskTotal:         diskTotal,
		DiskFree:          diskFree,
		DiskUsedPercent:   diskUsedPercent,
		DedupRatio:        dedupRatio,
		CompressionRatio:  compressionRatio,
		AlertLevel:        alertLevel,
		IsLocked:          locked,
		LockReason:        lockReason,
		BackupJobCount:    agentJobCounts[mr.Name],
		MetricsLoaded:     true,
		LastSnapshotTime:  lastSnapTime,
		LastSnapshotAgent: lastSnapAgent,
		HealthStatus:      repoHealthStatus(mr),
	}
}

func (d *Dashboard) cachedRepoMetrics(repoName string) (repoMetricsJSON, bool) {
	d.metricsCacheMu.Lock()
	defer d.metricsCacheMu.Unlock()
	if d.metricsCache == nil {
		return repoMetricsJSON{}, false
	}
	entry, ok := d.metricsCache[repoName]
	if !ok || time.Since(entry.cachedAt) >= metricsCacheTTL {
		return repoMetricsJSON{}, false
	}
	return entry.metrics, true
}

func (d *Dashboard) mergeCachedFullMetrics(summary repoMetricsJSON) repoMetricsJSON {
	if full, ok := d.cachedRepoMetrics(summary.Name); ok {
		summary.TotalBytes = full.TotalBytes
		summary.DedupRatio = full.DedupRatio
		summary.CompressionRatio = full.CompressionRatio
		summary.MetricsLoaded = true
		if summary.LastSnapshotTime == "" {
			summary.LastSnapshotTime = full.LastSnapshotTime
		}
		if summary.LastSnapshotAgent == "" {
			summary.LastSnapshotAgent = full.LastSnapshotAgent
		}
		if summary.HealthStatus == "" {
			summary.HealthStatus = full.HealthStatus
		}
	}
	return summary
}

func (d *Dashboard) warmRepoMetricsCache(mr *server.ManagedRepo, agentJobCounts map[string]int) {
	if _, ok := d.cachedRepoMetrics(mr.Name); ok {
		return
	}
	_ = d.getRepoMetrics(mr, agentJobCounts, false)
}

func (d *Dashboard) getRepoMetrics(mr *server.ManagedRepo, agentJobCounts map[string]int, summary bool) repoMetricsJSON {
	if summary {
		return d.mergeCachedFullMetrics(d.computeStorageMetricsSummary(mr, agentJobCounts))
	}

	if metrics, ok := d.cachedRepoMetrics(mr.Name); ok {
		return metrics
	}

	metrics := d.computeStorageMetricsFull(mr, agentJobCounts)

	d.metricsCacheMu.Lock()
	if d.metricsCache == nil {
		d.metricsCache = make(map[string]metricsCacheEntry)
	}
	d.metricsCache[mr.Name] = metricsCacheEntry{metrics: metrics, cachedAt: time.Now()}
	d.metricsCacheMu.Unlock()

	return metrics
}

func growthFromSummaries(summaries []cachedSnapshotSummary, cutoff time.Time, granularity string) []growthPoint {
	type dayData struct {
		totalBytes uint64
		count      int
	}
	byDate := make(map[string]*dayData)

	for _, cs := range summaries {
		if !cutoff.IsZero() && cs.Time.Before(cutoff) {
			continue
		}
		var dateKey string
		if granularity == "weekly" {
			year, week := cs.Time.ISOWeek()
			dateKey = fmt.Sprintf("%d-W%02d", year, week)
		} else {
			dateKey = cs.Time.Format("2006-01-02")
		}
		dd, ok := byDate[dateKey]
		if !ok {
			dd = &dayData{}
			byDate[dateKey] = dd
		}
		dd.totalBytes += cs.Summary.TotalSize
		dd.count++
	}

	dates := make([]string, 0, len(byDate))
	for k := range byDate {
		dates = append(dates, k)
	}
	sort.Strings(dates)

	result := make([]growthPoint, 0, len(dates))
	for _, date := range dates {
		dd := byDate[date]
		result = append(result, growthPoint{
			Date:       date,
			TotalBytes: dd.totalBytes,
			Snapshots:  dd.count,
		})
	}
	return result
}

func parseGrowthRange(rangeStr string) time.Time {
	now := time.Now()
	switch rangeStr {
	case "30d":
		return now.AddDate(0, 0, -30)
	case "90d":
		return now.AddDate(0, 0, -90)
	case "6m":
		return now.AddDate(0, -6, 0)
	case "1y":
		return now.AddDate(-1, 0, 0)
	case "all":
		return time.Time{}
	default:
		return now.AddDate(0, 0, -90)
	}
}
