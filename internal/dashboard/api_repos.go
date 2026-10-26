package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"strings"

	"github.com/hooperits/hitstor/internal/backend"
	localbackend "github.com/hooperits/hitstor/internal/backend/local"
	s3backend "github.com/hooperits/hitstor/internal/backend/s3"
	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
)

// repoMetricsJSON is the JSON response for GET /api/repos.
type repoMetricsJSON struct {
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	Location          string  `json:"location"`
	Snapshots         int     `json:"snapshots"`
	Packs             int     `json:"packs"`
	Indexes           int     `json:"indexes"`
	TotalBytes        uint64  `json:"total_bytes"`
	DiskTotal         uint64  `json:"disk_total"`
	DiskFree          uint64  `json:"disk_free"`
	DiskUsedPercent   float64 `json:"disk_used_percent"`
	DedupRatio        float64 `json:"dedup_ratio"`
	CompressionRatio  float64 `json:"compression_ratio"`
	AlertLevel        string  `json:"alert_level"`
	IsLocked          bool    `json:"is_locked"`
	LockReason        string  `json:"lock_reason"`
	BackupJobCount    int     `json:"backup_job_count"`
	MetricsLoaded     bool    `json:"metrics_loaded"`
	LastSnapshotTime  string  `json:"last_snapshot_time,omitempty"`
	LastSnapshotAgent string  `json:"last_snapshot_agent,omitempty"`
	HealthStatus      string  `json:"health_status,omitempty"`
}

func (d *Dashboard) computeStorageMetrics(mr *server.ManagedRepo, agentJobCounts map[string]int) repoMetricsJSON {
	return d.getRepoMetrics(mr, agentJobCounts, false)
}

func (d *Dashboard) repoJobCounts() map[string]int {
	agentJobCounts := make(map[string]int)
	for _, j := range d.registry.ListJobs() {
		agentJobCounts[j.RepoName]++
	}
	return agentJobCounts
}

// handleAPIRepos returns per-repo storage metrics.
// Use ?summary=1 for a lightweight response (no pack stats or dedup ratios).
func (d *Dashboard) handleAPIRepos(w http.ResponseWriter, r *http.Request) {
	repos := d.repos.List()
	summary := r.URL.Query().Get("summary") == "1" || r.URL.Query().Get("summary") == "true"
	agentJobCounts := d.repoJobCounts()

	result := make([]repoMetricsJSON, 0, len(repos))
	for _, mr := range repos {
		result = append(result, d.getRepoMetrics(mr, agentJobCounts, summary))
		if summary {
			go d.warmRepoMetricsCache(mr, agentJobCounts)
		}
	}

	writeJSON(w, result)
}

// handleAPIRepo returns full metrics for a single repository.
func (d *Dashboard) handleAPIRepo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	mr, err := d.repos.Get(name)
	if err != nil {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}
	writeJSON(w, d.getRepoMetrics(mr, d.repoJobCounts(), false))
}

func (d *Dashboard) localBackendOptions() localbackend.Options {
	opts := localbackend.Options{}
	if d.cfg != nil {
		opts.SyncWrites = d.cfg.SyncWrites || os.Getenv("HITSTOR_SYNC_WRITES") == "1"
	}
	return opts
}

// handleAPIRepoHealth returns operational health metadata for a repository.
func (d *Dashboard) handleAPIRepoHealth(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	mr, err := d.repos.Get(name)
	if err != nil {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}
	report, err := mr.Repo.Health()
	if err != nil {
		http.Error(w, fmt.Sprintf("health: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, report)
}

type repoSnapshotsResponse struct {
	RepoName string               `json:"repo_name"`
	Hosts    []string             `json:"hosts"`
	Agents   []snapshotAgentGroup `json:"agents"`
}

// handleAPIRepoSnapshots returns snapshots for one repo, grouped by agent.
func (d *Dashboard) handleAPIRepoSnapshots(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	mr, err := d.repos.Get(name)
	if err != nil {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}

	hostFilter := r.URL.Query().Get("host")
	summaries, err := d.getSnapshotSummaries(mr)
	if err != nil {
		http.Error(w, "list snapshots: "+err.Error(), http.StatusInternalServerError)
		return
	}

	hosts := make(map[string]bool)
	var allSnaps []snapshotJSON
	for _, cs := range summaries {
		hosts[cs.Hostname] = true
		if hostFilter != "" && cs.Hostname != hostFilter {
			continue
		}
		snapType := cs.Type
		if snapType == "" {
			snapType = "full"
		}
		allSnaps = append(allSnaps, snapshotJSON{
			ID:             cs.ID,
			Time:           cs.Time.Format(time.RFC3339),
			Hostname:       cs.Hostname,
			Tags:           cs.Tags,
			Paths:          cs.Paths,
			FilesNew:       cs.Summary.FilesNew,
			TotalFiles:     cs.Summary.TotalFiles,
			TotalSize:      cs.Summary.TotalSize,
			Duration:       cs.Summary.TotalDuration,
			Type:           snapType,
			ParentSnapshot: cs.ParentSnapshot,
			RepoName:       mr.Name,
			BackupType:     d.snapshotBackupType(cs),
		})
	}

	grouped := make(map[string][]snapshotJSON)
	for _, s := range allSnaps {
		grouped[s.Hostname] = append(grouped[s.Hostname], s)
	}

	var agents []snapshotAgentGroup
	for agent, snaps := range grouped {
		sort.Slice(snaps, func(i, j int) bool {
			return snaps[i].Time > snaps[j].Time
		})
		agents = append(agents, snapshotAgentGroup{Agent: agent, Snapshots: snaps})
	}
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Agent < agents[j].Agent
	})

	var hostList []string
	for h := range hosts {
		hostList = append(hostList, h)
	}
	sort.Strings(hostList)

	writeJSON(w, repoSnapshotsResponse{
		RepoName: name,
		Hosts:    hostList,
		Agents:   agents,
	})
}

// createRepoRequest is the JSON body for POST /api/repos.
type createRepoRequest struct {
	Name               string `json:"name"`
	Type               string `json:"type"`
	Mode               string `json:"mode,omitempty"` // "create" or "connect"
	Path               string `json:"path,omitempty"`
	Password           string `json:"password"`
	Endpoint           string `json:"endpoint,omitempty"`
	Bucket             string `json:"bucket,omitempty"`
	Region             string `json:"region,omitempty"`
	AccessKey          string `json:"access_key,omitempty"`
	SecretKey          string `json:"secret_key,omitempty"`
	ConnectionVerified bool   `json:"connection_verified,omitempty"`
}

// handleAPICreateRepo creates and initializes a new repository.
func (d *Dashboard) handleAPICreateRepo(w http.ResponseWriter, r *http.Request) {
	var req createRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Password == "" {
		http.Error(w, "name and password are required", http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		req.Type = "local"
	}

	// Check for duplicate name.
	if _, err := d.repos.Get(req.Name); err == nil {
		http.Error(w, "repository name already exists", http.StatusConflict)
		return
	}

	// Build RepoConfig.
	rc := config.RepoConfig{
		Name: req.Name,
		Type: req.Type,
	}

	// Create password file in config directory.
	configDir := filepath.Dir(d.cfg.Path)
	pwFile := filepath.Join(configDir, req.Name+".password")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		http.Error(w, fmt.Sprintf("create config dir: %v", err), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(pwFile, []byte(req.Password), 0600); err != nil {
		http.Error(w, fmt.Sprintf("write password file: %v", err), http.StatusInternalServerError)
		return
	}
	rc.PasswordFile = pwFile

	var be backend.Backend
	switch req.Type {
	case "local":
		if req.Path == "" || !filepath.IsAbs(req.Path) {
			os.Remove(pwFile)
			http.Error(w, "absolute path is required for local repos", http.StatusBadRequest)
			return
		}
		rc.Path = req.Path
		lb, err := localbackend.NewWithOptions(req.Path, d.localBackendOptions())
		if err != nil {
			os.Remove(pwFile)
			http.Error(w, fmt.Sprintf("create backend: %v", err), http.StatusInternalServerError)
			return
		}
		be = lb
	case "s3":
		if req.Endpoint == "" || req.Bucket == "" || req.AccessKey == "" || req.SecretKey == "" {
			os.Remove(pwFile)
			http.Error(w, "endpoint, bucket, access_key, and secret_key are required for S3", http.StatusBadRequest)
			return
		}
		if !req.ConnectionVerified {
			os.Remove(pwFile)
			http.Error(w, "S3 connection must be tested before saving", http.StatusBadRequest)
			return
		}
		rc.Endpoint = req.Endpoint
		rc.Bucket = req.Bucket
		rc.Region = req.Region
		if rc.Region == "" {
			rc.Region = "us-east-1"
		}
		// Encrypt S3 credentials.
		encKey, hasKey := d.repos.EncryptKey()
		if !hasKey {
			os.Remove(pwFile)
			http.Error(w, "no encryption key available (open a local repo first)", http.StatusBadRequest)
			return
		}
		var err error
		rc.AccessKeyEncrypted, err = config.EncryptCredential(encKey, req.AccessKey)
		if err != nil {
			os.Remove(pwFile)
			http.Error(w, fmt.Sprintf("encrypt access key: %v", err), http.StatusInternalServerError)
			return
		}
		rc.SecretKeyEncrypted, err = config.EncryptCredential(encKey, req.SecretKey)
		if err != nil {
			os.Remove(pwFile)
			http.Error(w, fmt.Sprintf("encrypt secret key: %v", err), http.StatusInternalServerError)
			return
		}
		sb, err := s3backend.New(req.Endpoint, req.Bucket, req.AccessKey, req.SecretKey, rc.Region)
		if err != nil {
			os.Remove(pwFile)
			http.Error(w, fmt.Sprintf("create S3 backend: %v", err), http.StatusInternalServerError)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := sb.TestConnection(ctx); err != nil {
			sb.Close()
			os.Remove(pwFile)
			http.Error(w, fmt.Sprintf("S3 connection failed: %v", err), http.StatusBadRequest)
			return
		}
		be = sb
	default:
		os.Remove(pwFile)
		http.Error(w, "type must be \"local\" or \"s3\"", http.StatusBadRequest)
		return
	}

	// Check if the backend already contains a repository.
	var repo *repository.Repository
	var connected bool
	var err error
	mode := req.Mode
	if mode == "" {
		mode = "create"
	}

	exists, err := backendHasRepo(r.Context(), be, req.Type, req.Path)
	if err != nil {
		be.Close()
		os.Remove(pwFile)
		http.Error(w, fmt.Sprintf("check repository: %v", err), http.StatusInternalServerError)
		return
	}
	if mode == "create" && exists {
		be.Close()
		os.Remove(pwFile)
		http.Error(w, "repository already exists at this location; use connect mode", http.StatusConflict)
		return
	}
	if mode == "connect" && !exists {
		be.Close()
		os.Remove(pwFile)
		http.Error(w, "no repository found at this location", http.StatusBadRequest)
		return
	}

	if exists {
		repo, err = repository.Open(be, req.Password)
		if err != nil {
			be.Close()
			os.Remove(pwFile)
			http.Error(w, fmt.Sprintf("open existing repository: %v", err), http.StatusBadRequest)
			return
		}
		connected = true
	} else {
		repo, err = repository.Init(be, req.Password)
		if err != nil {
			be.Close()
			os.Remove(pwFile)
			http.Error(w, fmt.Sprintf("init repository: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Register in RepoManager.
	d.repos.AddDirect(req.Name, repo, be)

	// If the agent registry has no backing store (server started in setup
	// mode with no repositories), initialize it now with this repo's key
	// and path so agents can be registered from the dashboard.
	if !d.registry.HasBacking() {
		if bp, ok := be.(interface{ BasePath() string }); ok {
			d.registry.InitBacking(repo.Key().EncryptKey, bp.BasePath())
		}
	}

	// Persist to config.
	if d.cfg != nil {
		d.cfg.Repositories = append(d.cfg.Repositories, rc)
		if err := config.Save(*d.cfg); err != nil {
			http.Error(w, fmt.Sprintf("save config: %v", err), http.StatusInternalServerError)
			return
		}
	}

	location := rc.Path
	if req.Type == "s3" {
		location = fmt.Sprintf("s3://%s@%s", rc.Bucket, rc.Endpoint)
	}

	action := "created"
	msg := "Repository created and initialized"
	if connected {
		action = "connected"
		msg = "Existing repository connected"
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"action":   action,
		"name":     req.Name,
		"type":     req.Type,
		"location": location,
		"message":  msg,
	})
}

// handleAPIDeleteRepo disconnects a repository.
func (d *Dashboard) handleAPIDeleteRepo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// Check if any backup jobs target this repo.
	repoJobs := d.registry.ListJobsByRepo(name)
	if len(repoJobs) > 0 {
		jobNames := make([]string, 0, len(repoJobs))
		for _, j := range repoJobs {
			jobNames = append(jobNames, j.Name)
		}
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{
			"error":       "Cannot disconnect: repository has active backup jobs",
			"backup_jobs": jobNames,
		})
		return
	}

	if err := d.repos.Close(name); err != nil {
		http.Error(w, fmt.Sprintf("close repo: %v", err), http.StatusNotFound)
		return
	}

	// Remove from config.
	if d.cfg != nil {
		filtered := make([]config.RepoConfig, 0, len(d.cfg.Repositories))
		for _, rc := range d.cfg.Repositories {
			if rc.Name != name {
				filtered = append(filtered, rc)
			}
		}
		d.cfg.Repositories = filtered
		if err := config.Save(*d.cfg); err != nil {
			http.Error(w, fmt.Sprintf("save config: %v", err), http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, map[string]any{
		"message": fmt.Sprintf("Repository '%s' disconnected", name),
	})
}

// handleAPIDiskSpace returns disk space at a given path.
func (d *Dashboard) handleAPIDiskSpace(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path parameter is required", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		http.Error(w, "path does not exist or is not a directory", http.StatusBadRequest)
		return
	}

	totalBytes, freeBytes, err := getDiskSpace(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("stat filesystem: %v", err), http.StatusInternalServerError)
		return
	}

	var usedPercent float64
	if totalBytes > 0 {
		usedPercent = float64(totalBytes-freeBytes) / float64(totalBytes) * 100
	}

	writeJSON(w, map[string]any{
		"path":         path,
		"total_bytes":  totalBytes,
		"free_bytes":   freeBytes,
		"used_percent": usedPercent,
		"valid":        true,
	})
}

// growthPoint is a single data point for the growth chart.
type growthPoint struct {
	Date       string `json:"date"`
	TotalBytes uint64 `json:"total_bytes"`
	Snapshots  int    `json:"snapshots"`
}

// handleAPIGrowth returns storage growth data for a repo.
func (d *Dashboard) handleAPIGrowth(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	mr, err := d.repos.Get(name)
	if err != nil {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}

	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = "90d"
	}
	granularity := r.URL.Query().Get("granularity")
	if granularity == "" {
		granularity = "daily"
	}

	summaries, err := d.getSnapshotSummaries(mr)
	if err != nil {
		http.Error(w, "list snapshots: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, growthFromSummaries(summaries, parseGrowthRange(rangeStr), granularity))
}

// handleAPIReposGrowthBatch returns growth data for multiple repos in one request.
func (d *Dashboard) handleAPIReposGrowthBatch(w http.ResponseWriter, r *http.Request) {
	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = "30d"
	}
	granularity := r.URL.Query().Get("granularity")
	if granularity == "" {
		granularity = "daily"
	}
	cutoff := parseGrowthRange(rangeStr)

	names := r.URL.Query()["repos"]
	if len(names) == 1 && strings.Contains(names[0], ",") {
		names = strings.Split(names[0], ",")
	}

	result := make(map[string][]growthPoint, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		mr, err := d.repos.Get(name)
		if err != nil {
			continue
		}
		summaries, err := d.getSnapshotSummaries(mr)
		if err != nil {
			continue
		}
		result[name] = growthFromSummaries(summaries, cutoff, granularity)
	}

	writeJSON(w, result)
}
