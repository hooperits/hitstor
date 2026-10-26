package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/crypto"
)

// IsFilesystemJobType reports whether jobType is a supported filesystem job.
// Empty job type is treated as files for backward compatibility.
func IsFilesystemJobType(jobType string) bool {
	return jobType == "" || jobType == "files"
}

// SentinelAlert records a ransomware threat event.
type SentinelAlert struct {
	ID             string    `json:"id"`
	AgentName      string    `json:"agent_name"`
	Hostname       string    `json:"hostname"`
	Time           time.Time `json:"time"`
	SnapshotID     string    `json:"snapshot_id"`
	ThreatDetected bool      `json:"threat_detected"`
	Confidence     float64   `json:"confidence"`
	Reason         string    `json:"reason"`
	Status         string    `json:"status"` // "active" or "resolved"
}

// AgentRecord holds metadata about a registered agent.
type AgentRecord struct {
	Name      string    `json:"name"`
	KeyHash   [32]byte  `json:"key_hash"`
	CreatedAt time.Time `json:"created_at"`

	// Agent management fields (dashboard-registered agents).
	AgentAddr      string `json:"agent_addr,omitempty"`      // host:port of agent's gRPC listen address
	Managed        bool   `json:"managed,omitempty"`         // true if registered via dashboard
	TokenEncrypted string `json:"token_encrypted,omitempty"` // AES-256-GCM encrypted agent token
	Suspended      bool   `json:"suspended,omitempty"`       // true if suspended by Sentinel
	TLSFingerprint string `json:"tls_fingerprint,omitempty"` // pinned SHA-256 of the agent's TLS cert (trust-on-first-use)

	// Recovering marks a host currently booted off bare-metal recovery media
	// rather than its own OS. AgentAddr then points at the rescue system, not
	// the real machine, and the flag clears when the restore reaches a
	// terminal state or the restored OS registers normally.
	Recovering bool `json:"recovering,omitempty"`

	// RecoveryTokenEncrypted is the rescue system's own agent token, valid
	// only while Recovering. The rescue agent generates it and reports it
	// when announcing, so a bare-metal recovery discloses no existing
	// credential and leaves the real agent's token untouched for when the
	// restored OS comes back up.
	RecoveryTokenEncrypted string `json:"recovery_token_encrypted,omitempty"`

	// Deprecated: schedule/repo/backup fields moved to BackupJob.
	// Kept for backward-compatible JSON parsing during migration.
	LastBackup      *time.Time `json:"last_backup,omitempty"`
	ScheduleEnabled bool       `json:"schedule_enabled,omitempty"`
	ScheduleHours   int        `json:"schedule_hours,omitempty"`
	RepoName        string     `json:"repo_name,omitempty"`
}

// BackupJob defines a scheduled backup targeting one agent and one repository.
// Multiple jobs can target the same agent with different paths and schedules.
type BackupJob struct {
	ID              string     `json:"id"`         // 16-char hex, unique
	Name            string     `json:"name"`       // human label
	AgentName       string     `json:"agent_name"` // references AgentRecord.Name
	RepoName        string     `json:"repo_name"`  // target repository
	Paths           []string   `json:"paths"`      // absolute paths on agent
	Excludes        []string   `json:"excludes,omitempty"`
	ScheduleEnabled bool       `json:"schedule_enabled,omitempty"`
	ScheduleHours   int        `json:"schedule_hours,omitempty"`
	CronExpression  string     `json:"cron_expression,omitempty"`
	Timezone        string     `json:"timezone,omitempty"`
	LastBackup      *time.Time `json:"last_backup,omitempty"`
	RunNow          bool       `json:"run_now,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	FullRescanDays  int        `json:"full_rescan_days,omitempty"`
	LastFullBackup  *time.Time `json:"last_full_backup,omitempty"`
	ForceFullScan   bool       `json:"force_full_scan,omitempty"`
	UseVSS          bool       `json:"use_vss,omitempty"`

	// Restore verification.
	// VerifyAfterBackup defaults to true when the field is omitted (see
	// jobWantsVerify). Status is updated asynchronously after each backup.
	VerifyAfterBackup *bool      `json:"verify_after_backup,omitempty"`
	LastVerifyAt      *time.Time `json:"last_verify_at,omitempty"`
	LastVerifyStatus  string     `json:"last_verify_status,omitempty"` // ok|failed|error|skipped
	LastVerifySnap    string     `json:"last_verify_snapshot,omitempty"`
	LastVerifyDetail  string     `json:"last_verify_detail,omitempty"`
	LastVerifyFiles   int        `json:"last_verify_files,omitempty"`
	LastVerifyBytes   uint64     `json:"last_verify_bytes,omitempty"`

	// JobType is "files" (or empty) for every job; HitStor is filesystem-only.
	JobType string `json:"job_type,omitempty"`
}

// registryData is the versioned envelope stored in agents.json.
type registryData struct {
	Agents         []AgentRecord   `json:"agents"`
	Jobs           []BackupJob     `json:"jobs,omitempty"`
	Alerts         []SentinelAlert `json:"alerts,omitempty"`
	RecoveryTokens []RecoveryToken `json:"recovery_tokens,omitempty"`
}

// AgentRegistry manages API keys for remote backup agents and backup jobs.
// Data is stored encrypted in the repository directory.
//
// All public methods are safe for concurrent use. mu guards agents + jobs;
// save() assumes the caller holds the write lock and is invoked from every
// mutator before returning.
type AgentRegistry struct {
	mu     sync.RWMutex
	agents []AgentRecord
	jobs   []BackupJob
	alerts []SentinelAlert
	// recoveryTokens authorize bare-metal recoveries. Persisted so a token
	// minted before an outage survives a server restart during one, and so
	// single-use cannot be reset by bouncing the server.
	recoveryTokens []RecoveryToken
	encryptKey     [32]byte
	path           string // path to agents.json file
}

// NewEmptyRegistry creates an empty in-memory agent registry with no backing
// store. Used when the server starts in setup mode with no repositories.
func NewEmptyRegistry() *AgentRegistry {
	return &AgentRegistry{}
}

// HasBacking reports whether the registry has a backing file path set.
func (r *AgentRegistry) HasBacking() bool {
	return r.path != ""
}

// InitBacking sets the encryption key and file path on an empty registry so
// it can persist to disk. This is called when the first repository is created
// from the dashboard while the server was started in setup mode.
func (r *AgentRegistry) InitBacking(encryptKey [32]byte, repoPath string) {
	r.encryptKey = encryptKey
	r.path = filepath.Join(repoPath, "agents.json")
}

// LoadAgentRegistry reads and decrypts the agent registry from disk.
// Returns an empty registry if the file does not exist.
// Supports both the new registryData format and the legacy []AgentRecord format.
func LoadAgentRegistry(encryptKey [32]byte, repoPath string) (*AgentRegistry, error) {
	r := &AgentRegistry{
		encryptKey: encryptKey,
		path:       filepath.Join(repoPath, "agents.json"),
	}

	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, fmt.Errorf("server: read agent registry: %w", err)
	}

	plaintext, err := crypto.Decrypt(encryptKey, data)
	if err != nil {
		return nil, fmt.Errorf("server: decrypt agent registry: %w", err)
	}

	// Determine format: new format starts with '{' (JSON object).
	trimmed := strings.TrimSpace(string(plaintext))
	if strings.HasPrefix(trimmed, "{") {
		var rd registryData
		if err := json.Unmarshal(plaintext, &rd); err != nil {
			return nil, fmt.Errorf("server: unmarshal agent registry: %w", err)
		}
		r.agents = rd.Agents
		r.jobs = rd.Jobs
		r.alerts = rd.Alerts
		r.recoveryTokens = rd.RecoveryTokens
		r.orphanNonFilesystemJobs()
		return r, nil
	}

	// Fall back to legacy format (plain []AgentRecord array).
	if err := json.Unmarshal(plaintext, &r.agents); err != nil {
		return nil, fmt.Errorf("server: unmarshal agent registry: %w", err)
	}

	// Migrate: create BackupJob records from agents with schedule/repo set.
	r.migrateAgentSchedules()
	r.orphanNonFilesystemJobs()

	return r, nil
}

// orphanNonFilesystemJobs disables schedules for job types removed from product
// scope (M365, database, cloud VM, Google Workspace). Records remain so
// operators can inspect and delete them; they will not run.
func (r *AgentRegistry) orphanNonFilesystemJobs() {
	for i := range r.jobs {
		j := &r.jobs[i]
		if IsFilesystemJobType(j.JobType) {
			continue
		}
		if j.ScheduleEnabled || j.RunNow {
			slog.Warn("disabling non-filesystem backup job (product focus: filesystem only)",
				"job_id", j.ID,
				"job_name", j.Name,
				"job_type", j.JobType,
			)
		}
		j.ScheduleEnabled = false
		j.RunNow = false
	}
}

// migrateAgentSchedules moves deprecated schedule/repo fields from
// AgentRecord into BackupJob records and saves the new format.
func (r *AgentRegistry) migrateAgentSchedules() {
	var migrated bool
	for i := range r.agents {
		a := &r.agents[i]
		if a.RepoName == "" && !a.ScheduleEnabled {
			continue
		}

		var idBytes [8]byte
		rand.Read(idBytes[:])

		r.jobs = append(r.jobs, BackupJob{
			ID:              hex.EncodeToString(idBytes[:]),
			Name:            a.Name + "-backup",
			AgentName:       a.Name,
			RepoName:        a.RepoName,
			ScheduleEnabled: a.ScheduleEnabled,
			ScheduleHours:   a.ScheduleHours,
			LastBackup:      a.LastBackup,
			CreatedAt:       a.CreatedAt,
		})

		// Clear deprecated fields.
		a.RepoName = ""
		a.ScheduleEnabled = false
		a.ScheduleHours = 0
		a.LastBackup = nil
		migrated = true
	}

	if migrated {
		r.save() // persist the new format
	}
}

// save encrypts and writes the registry to disk atomically.
func (r *AgentRegistry) save() error {
	if r.path == "" {
		return fmt.Errorf("server: agent registry has no backing store — create a repository first")
	}
	rd := registryData{
		Agents:         r.agents,
		Jobs:           r.jobs,
		Alerts:         r.alerts,
		RecoveryTokens: r.recoveryTokens,
	}
	plaintext, err := json.Marshal(rd)
	if err != nil {
		return fmt.Errorf("server: marshal agent registry: %w", err)
	}

	ciphertext, err := crypto.Encrypt(r.encryptKey, plaintext)
	if err != nil {
		return fmt.Errorf("server: encrypt agent registry: %w", err)
	}

	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, ciphertext, 0600); err != nil {
		return fmt.Errorf("server: write agent registry: %w", err)
	}
	if err := os.Rename(tmp, r.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("server: rename agent registry: %w", err)
	}

	return nil
}

// AddAgent registers a new agent and returns the API key (hex-encoded).
// The key is displayed once and stored as an HMAC-SHA256 hash.
func (r *AgentRegistry) AddAgent(name string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.agents {
		if a.Name == name {
			return "", fmt.Errorf("server: agent %q already registered", name)
		}
	}

	// Generate 32 random bytes as the API key.
	var keyBytes [32]byte
	if _, err := rand.Read(keyBytes[:]); err != nil {
		return "", fmt.Errorf("server: generate API key: %w", err)
	}

	apiKey := hex.EncodeToString(keyBytes[:])
	keyHash := hashAPIKey(keyBytes[:])

	r.agents = append(r.agents, AgentRecord{
		Name:      name,
		KeyHash:   keyHash,
		CreatedAt: time.Now(),
	})

	if err := r.save(); err != nil {
		// Roll back the in-memory addition.
		r.agents = r.agents[:len(r.agents)-1]
		return "", err
	}

	return apiKey, nil
}

// RemoveAgent removes an agent by name.
// If deleteJobs is true, also removes any associated backup jobs.
// Returns an error if not found.
func (r *AgentRegistry) RemoveAgent(name string, deleteJobs bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	found := false
	for i, a := range r.agents {
		if a.Name == name {
			r.agents = append(r.agents[:i], r.agents[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("server: agent %q not found", name)
	}

	if deleteJobs {
		// Remove all backup jobs targeting this agent.
		filtered := r.jobs[:0]
		for _, j := range r.jobs {
			if j.AgentName != name {
				filtered = append(filtered, j)
			}
		}
		r.jobs = filtered
	}

	return r.save()
}

// ListAgents returns all registered agent records.
func (r *AgentRegistry) ListAgents() []AgentRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]AgentRecord, len(r.agents))
	copy(result, r.agents)
	return result
}

// VerifyKey checks an API key (hex-encoded) against the registry.
// Returns the agent name if valid, or empty string and false if not.
func (r *AgentRegistry) VerifyKey(apiKeyHex string) (string, bool) {
	keyBytes, err := hex.DecodeString(apiKeyHex)
	if err != nil || len(keyBytes) != 32 {
		return "", false
	}

	keyHash := hashAPIKey(keyBytes)
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, a := range r.agents {
		if a.KeyHash == keyHash {
			if a.Suspended {
				return "", false
			}
			return a.Name, true
		}
	}
	return "", false
}

// UpdateLastBackup sets the last backup time for the named agent.
func (r *AgentRegistry) UpdateLastBackup(name string, t time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.agents {
		if r.agents[i].Name == name {
			r.agents[i].LastBackup = &t
			return r.save()
		}
	}
	return fmt.Errorf("server: agent %q not found", name)
}

// UpdateSchedule sets the backup schedule for the named agent.
func (r *AgentRegistry) UpdateSchedule(name string, enabled bool, hours int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.agents {
		if r.agents[i].Name == name {
			r.agents[i].ScheduleEnabled = enabled
			r.agents[i].ScheduleHours = hours
			return r.save()
		}
	}
	return fmt.Errorf("server: agent %q not found", name)
}

// UpdateRepoName sets the target repository for the named agent.
func (r *AgentRegistry) UpdateRepoName(name, repoName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.agents {
		if r.agents[i].Name == name {
			r.agents[i].RepoName = repoName
			return r.save()
		}
	}
	return fmt.Errorf("server: agent %q not found", name)
}

// SetAgentFingerprint pins the TLS certificate fingerprint for the named
// agent, recorded on first successful connection (trust-on-first-use).
func (r *AgentRegistry) SetAgentFingerprint(name, fingerprint string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.agents {
		if r.agents[i].Name == name {
			r.agents[i].TLSFingerprint = fingerprint
			return r.save()
		}
	}
	return fmt.Errorf("server: agent %q not found", name)
}

// GetAgent returns the agent record for the given name, or nil if not found.
func (r *AgentRegistry) GetAgent(name string) *AgentRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getAgentLocked(name)
}

// getAgentLocked is the lock-free inner lookup. Callers must hold r.mu at
// least for reading.
func (r *AgentRegistry) getAgentLocked(name string) *AgentRecord {
	for i := range r.agents {
		if r.agents[i].Name == name {
			a := r.agents[i]
			return &a
		}
	}
	return nil
}

// mutableAgentLocked returns a pointer into the agents slice, so callers can
// change a record in place. getAgentLocked deliberately returns a copy;
// mutators need the original. Callers must hold r.mu for writing.
func (r *AgentRegistry) mutableAgentLocked(name string) *AgentRecord {
	for i := range r.agents {
		if r.agents[i].Name == name {
			return &r.agents[i]
		}
	}
	return nil
}

// AddManagedAgent registers a dashboard-managed agent with its connection
// info and encrypted token. Generates an API key and returns it.
func (r *AgentRegistry) AddManagedAgent(name, agentAddr, token string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.agents {
		if a.Name == name {
			return "", fmt.Errorf("server: agent %q already registered", name)
		}
	}

	// Encrypt the agent token for storage.
	tokenEncrypted, err := config.EncryptCredential(r.encryptKey, token)
	if err != nil {
		return "", fmt.Errorf("server: encrypt agent token: %w", err)
	}

	// Generate API key.
	var keyBytes [32]byte
	if _, err := rand.Read(keyBytes[:]); err != nil {
		return "", fmt.Errorf("server: generate API key: %w", err)
	}

	apiKey := hex.EncodeToString(keyBytes[:])
	keyHash := hashAPIKey(keyBytes[:])

	r.agents = append(r.agents, AgentRecord{
		Name:           name,
		KeyHash:        keyHash,
		CreatedAt:      time.Now(),
		AgentAddr:      agentAddr,
		Managed:        true,
		TokenEncrypted: tokenEncrypted,
	})

	if err := r.save(); err != nil {
		r.agents = r.agents[:len(r.agents)-1]
		return "", err
	}

	return apiKey, nil
}

// ReRegisterAgent updates an existing agent's token and generates a new API key.
// Used when an agent is reinstalled and needs fresh credentials.
func (r *AgentRegistry) ReRegisterAgent(name, token string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.agents {
		if r.agents[i].Name == name {
			tokenEncrypted, err := config.EncryptCredential(r.encryptKey, token)
			if err != nil {
				return "", fmt.Errorf("server: encrypt agent token: %w", err)
			}
			var keyBytes [32]byte
			if _, err := rand.Read(keyBytes[:]); err != nil {
				return "", fmt.Errorf("server: generate API key: %w", err)
			}
			apiKey := hex.EncodeToString(keyBytes[:])
			r.agents[i].KeyHash = hashAPIKey(keyBytes[:])
			r.agents[i].TokenEncrypted = tokenEncrypted
			// A re-registered agent was reinstalled and presents a new
			// certificate; drop the stale pin so TOFU re-records it.
			r.agents[i].TLSFingerprint = ""
			// A real agent registering from a restored OS supersedes any
			// rescue system that was standing in for it. Leaving the flag set
			// would keep pointing restores at a machine that has rebooted.
			r.agents[i].Recovering = false
			r.agents[i].RecoveryTokenEncrypted = ""
			if err := r.save(); err != nil {
				return "", err
			}
			return apiKey, nil
		}
	}
	return "", fmt.Errorf("server: agent %q not found", name)
}

// DecryptToken decrypts the stored agent token for a managed agent.
func (r *AgentRegistry) DecryptToken(name string) (string, error) {
	r.mu.RLock()
	agent := r.getAgentLocked(name)
	r.mu.RUnlock()
	if agent == nil {
		return "", fmt.Errorf("server: agent %q not found", name)
	}
	// A host booted off recovery media answers on a token it generated
	// itself; the original agent's token is on a disk that may be gone.
	if agent.Recovering && agent.RecoveryTokenEncrypted != "" {
		return config.DecryptCredential(r.encryptKey, agent.RecoveryTokenEncrypted)
	}
	if !agent.Managed || agent.TokenEncrypted == "" {
		return "", fmt.Errorf("server: agent %q is not a managed agent", name)
	}
	return config.DecryptCredential(r.encryptKey, agent.TokenEncrypted)
}

// EncryptKey returns the registry's encryption key and whether it's available.
func (r *AgentRegistry) EncryptKey() ([32]byte, bool) {
	var zero [32]byte
	if r.encryptKey == zero {
		return zero, false
	}
	return r.encryptKey, true
}

// --- BackupJob CRUD ---

// AddJob registers a new backup job and persists to disk.
func (r *AgentRegistry) AddJob(job BackupJob) error {
	if !IsFilesystemJobType(job.JobType) {
		return fmt.Errorf("server: unsupported job type %q: HitStor supports filesystem backup jobs only", job.JobType)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range r.jobs {
		if j.ID == job.ID {
			return fmt.Errorf("server: job %q already exists", job.ID)
		}
	}
	r.jobs = append(r.jobs, job)
	if err := r.save(); err != nil {
		r.jobs = r.jobs[:len(r.jobs)-1]
		return err
	}
	return nil
}

// UpdateJob replaces a backup job by ID and persists.
func (r *AgentRegistry) UpdateJob(job BackupJob) error {
	if !IsFilesystemJobType(job.JobType) {
		return fmt.Errorf("server: unsupported job type %q: HitStor supports filesystem backup jobs only", job.JobType)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.jobs {
		if r.jobs[i].ID == job.ID {
			r.jobs[i] = job
			return r.save()
		}
	}
	return fmt.Errorf("server: job %q not found", job.ID)
}

// RemoveJob removes a backup job by ID and persists.
func (r *AgentRegistry) RemoveJob(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, j := range r.jobs {
		if j.ID == id {
			r.jobs = append(r.jobs[:i], r.jobs[i+1:]...)
			return r.save()
		}
	}
	return fmt.Errorf("server: job %q not found", id)
}

// GetJob returns the backup job for the given ID, or nil if not found.
func (r *AgentRegistry) GetJob(id string) *BackupJob {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.jobs {
		if r.jobs[i].ID == id {
			j := r.jobs[i]
			return &j
		}
	}
	return nil
}

// ListJobs returns all backup jobs.
func (r *AgentRegistry) ListJobs() []BackupJob {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]BackupJob, len(r.jobs))
	copy(result, r.jobs)
	return result
}

// ListJobsByAgent returns all backup jobs for the named agent.
func (r *AgentRegistry) ListJobsByAgent(agentName string) []BackupJob {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []BackupJob
	for _, j := range r.jobs {
		if j.AgentName == agentName {
			result = append(result, j)
		}
	}
	return result
}

// ListJobsByRepo returns all backup jobs targeting the named repository.
func (r *AgentRegistry) ListJobsByRepo(repoName string) []BackupJob {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []BackupJob
	for _, j := range r.jobs {
		if j.RepoName == repoName {
			result = append(result, j)
		}
	}
	return result
}

// ListOrphanJobs returns jobs whose AgentName does not match any currently
// registered agent. Useful after remove + re-add scenarios.
func (r *AgentRegistry) ListOrphanJobs() []BackupJob {
	r.mu.RLock()
	defer r.mu.RUnlock()

	active := make(map[string]bool, len(r.agents))
	for _, a := range r.agents {
		active[a.Name] = true
	}

	var result []BackupJob
	for _, j := range r.jobs {
		if !active[j.AgentName] {
			result = append(result, j)
		}
	}
	return result
}

// ReattachJobs updates the AgentName on the specified job IDs to targetAgentName
// (after verifying the target agent exists). This helps recover linkage after
// a remove + same-name re-add.
func (r *AgentRegistry) ReattachJobs(targetAgentName string, jobIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.getAgentLocked(targetAgentName) == nil {
		return fmt.Errorf("server: target agent %q not found", targetAgentName)
	}

	idSet := make(map[string]bool, len(jobIDs))
	for _, id := range jobIDs {
		idSet[id] = true
	}

	changed := false
	for i := range r.jobs {
		if idSet[r.jobs[i].ID] {
			r.jobs[i].AgentName = targetAgentName
			changed = true
		}
	}

	if changed {
		return r.save()
	}
	return nil
}

// UpdateJobLastBackup sets the last backup time for the given job ID.
func (r *AgentRegistry) UpdateJobLastBackup(id string, t time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.jobs {
		if r.jobs[i].ID == id {
			r.jobs[i].LastBackup = &t
			r.jobs[i].RunNow = false        // Clear manual trigger after backup completes.
			r.jobs[i].ForceFullScan = false // Clear force-full after backup completes.
			return r.save()
		}
	}
	return fmt.Errorf("server: job %q not found", id)
}

// UpdateJobLastFullBackup sets the last full backup time for the given job ID.
func (r *AgentRegistry) UpdateJobLastFullBackup(id string, t time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.jobs {
		if r.jobs[i].ID == id {
			r.jobs[i].LastFullBackup = &t
			return r.save()
		}
	}
	return fmt.Errorf("server: job %q not found", id)
}

// JobWantsVerify reports whether post-backup restore verification is enabled.
// nil VerifyAfterBackup means default-on (filesystem excellence).
func JobWantsVerify(j BackupJob) bool {
	if j.VerifyAfterBackup == nil {
		return true
	}
	return *j.VerifyAfterBackup
}

// JobVerifyResult is persisted after an automated test-restore.
type JobVerifyResult struct {
	At       time.Time
	Status   string
	Snapshot string
	Detail   string
	Files    int
	Bytes    uint64
}

// UpdateJobVerifyResult records the last restore-verification outcome for a job.
func (r *AgentRegistry) UpdateJobVerifyResult(id string, result JobVerifyResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.jobs {
		if r.jobs[i].ID == id {
			t := result.At
			r.jobs[i].LastVerifyAt = &t
			r.jobs[i].LastVerifyStatus = result.Status
			r.jobs[i].LastVerifySnap = result.Snapshot
			r.jobs[i].LastVerifyDetail = result.Detail
			r.jobs[i].LastVerifyFiles = result.Files
			r.jobs[i].LastVerifyBytes = result.Bytes
			return r.save()
		}
	}
	return fmt.Errorf("server: job %q not found", id)
}

// SetJobRunNow marks or clears the manual "Backup Now" flag on a job.
func (r *AgentRegistry) SetJobRunNow(id string, runNow bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.jobs {
		if r.jobs[i].ID == id {
			r.jobs[i].RunNow = runNow
			return r.save()
		}
	}
	return fmt.Errorf("server: job %q not found", id)
}

func (r *AgentRegistry) SetJobForceFullScan(id string, force bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.jobs {
		if r.jobs[i].ID == id {
			r.jobs[i].ForceFullScan = force
			return r.save()
		}
	}
	return fmt.Errorf("server: job %q not found", id)
}

// hashAPIKey computes HMAC-SHA256(key=encryptionKey, data=apiKey) to avoid
// storing raw API keys. We use a fixed key derived from the key bytes
// themselves (SHA-256 of the raw key) for simplicity.
func hashAPIKey(apiKey []byte) [32]byte {
	mac := hmac.New(sha256.New, apiKey)
	mac.Write([]byte("hitstor-agent-key"))
	var result [32]byte
	copy(result[:], mac.Sum(nil))
	return result
}

// SuspendAgent marks the named agent as suspended.
func (r *AgentRegistry) SuspendAgent(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.agents {
		if r.agents[i].Name == name {
			r.agents[i].Suspended = true
			return r.save()
		}
	}
	return fmt.Errorf("server: agent %q not found", name)
}

// UnsuspendAgent clears the suspended flag for the named agent.
func (r *AgentRegistry) UnsuspendAgent(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.agents {
		if r.agents[i].Name == name {
			r.agents[i].Suspended = false
			return r.save()
		}
	}
	return fmt.Errorf("server: agent %q not found", name)
}

// AddAlert logs a threat alert to the registry history.
func (r *AgentRegistry) AddAlert(alert SentinelAlert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.alerts = append(r.alerts, alert)
	return r.save()
}

// ListAlerts returns all sentinel alerts.
func (r *AgentRegistry) ListAlerts() []SentinelAlert {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]SentinelAlert, len(r.alerts))
	copy(result, r.alerts)
	return result
}

// ResolveAlert marks an alert as resolved.
func (r *AgentRegistry) ResolveAlert(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.alerts {
		if r.alerts[i].ID == id {
			r.alerts[i].Status = "resolved"
			return r.save()
		}
	}
	return fmt.Errorf("server: alert %q not found", id)
}
