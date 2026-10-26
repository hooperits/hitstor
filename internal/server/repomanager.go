package server

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/backend/local"
	s3backend "github.com/hooperits/hitstor/internal/backend/s3"
	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/crypto"
	"github.com/hooperits/hitstor/internal/repository"
)

// ManagedRepo is the runtime representation of an open repository.
type ManagedRepo struct {
	Name            string
	Repo            *repository.Repository
	Backend         backend.Backend
	Config          config.RepoConfig
	MaintenanceLock *MaintenanceLock
}

// RepoManager manages multiple open repositories.
type RepoManager struct {
	mu             sync.RWMutex
	repos          map[string]*ManagedRepo
	encryptKey     [32]byte // first repo's master key, used for S3 credential decryption
	hasKey         bool
	syncWrites     bool
	minFreeReserve uint64
}

// SetStorageOptions configures local backend durability and free-space checks for opened repos.
func (rm *RepoManager) SetStorageOptions(syncWrites bool, minFreeReserve uint64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.syncWrites = syncWrites || os.Getenv("HITSTOR_SYNC_WRITES") == "1"
	rm.minFreeReserve = minFreeReserve
}

// NewRepoManager creates an empty RepoManager.
func NewRepoManager() *RepoManager {
	return &RepoManager{
		repos: make(map[string]*ManagedRepo),
	}
}

// Open opens a repository from its config and registers it.
// For S3 repos, a local repo must be opened first to provide the encryption key.
func (rm *RepoManager) Open(cfg config.RepoConfig) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, ok := rm.repos[cfg.Name]; ok {
		return fmt.Errorf("server: repo %q already open", cfg.Name)
	}

	password, err := os.ReadFile(cfg.PasswordFile)
	if err != nil {
		return fmt.Errorf("server: read password for repo %q: %w", cfg.Name, err)
	}

	be, err := rm.openBackend(cfg)
	if err != nil {
		return fmt.Errorf("server: open backend for repo %q: %w", cfg.Name, err)
	}

	repo, err := repository.Open(be, strings.TrimSpace(string(password)))
	if err != nil {
		be.Close()
		return fmt.Errorf("server: open repo %q: %w", cfg.Name, err)
	}
	rm.applyRepoSettings(repo, be)

	// Set encryption key from first opened repo.
	if !rm.hasKey {
		rm.encryptKey = repo.Key().EncryptKey
		rm.hasKey = true
	}

	rm.repos[cfg.Name] = &ManagedRepo{
		Name:            cfg.Name,
		Repo:            repo,
		Backend:         be,
		Config:          cfg,
		MaintenanceLock: NewMaintenanceLock(),
	}

	slog.Info("repository opened", "name", cfg.Name, "type", cfg.Type)
	return nil
}

// Close unlocks and closes a repository, removing it from management.
func (rm *RepoManager) Close(name string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	mr, ok := rm.repos[name]
	if !ok {
		return fmt.Errorf("server: repo %q not found", name)
	}

	mr.Repo.Unlock()
	mr.Backend.Close()
	delete(rm.repos, name)

	slog.Info("repository closed", "name", name)
	return nil
}

// Get returns a managed repo by name.
func (rm *RepoManager) Get(name string) (*ManagedRepo, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	mr, ok := rm.repos[name]
	if !ok {
		return nil, fmt.Errorf("server: repo %q not found", name)
	}
	return mr, nil
}

// List returns all managed repos sorted by name.
func (rm *RepoManager) List() []*ManagedRepo {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]*ManagedRepo, 0, len(rm.repos))
	for _, mr := range rm.repos {
		result = append(result, mr)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// CloseAll closes all managed repositories.
func (rm *RepoManager) CloseAll() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for name, mr := range rm.repos {
		mr.Repo.Unlock()
		mr.Backend.Close()
		slog.Info("repository closed", "name", name)
	}
	rm.repos = make(map[string]*ManagedRepo)
}

// AddDirect registers an already-opened repo and backend (for legacy single-repo mode).
func (rm *RepoManager) AddDirect(name string, repo *repository.Repository, be backend.Backend) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.hasKey {
		rm.encryptKey = repo.Key().EncryptKey
		rm.hasKey = true
	}

	rm.applyRepoSettings(repo, be)

	rm.repos[name] = &ManagedRepo{
		Name:            name,
		Repo:            repo,
		Backend:         be,
		MaintenanceLock: NewMaintenanceLock(),
	}
	slog.Info("repository registered", "name", name)
}

// EncryptKey returns the first repo's encryption key for S3 credential encryption.
// Returns a zero key and false if no repos have been opened yet.
func (rm *RepoManager) EncryptKey() ([32]byte, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.encryptKey, rm.hasKey
}

// openBackend creates a backend from the repo config.
func (rm *RepoManager) openBackend(cfg config.RepoConfig) (backend.Backend, error) {
	switch cfg.Type {
	case "local":
		return local.NewWithOptions(cfg.Path, local.Options{SyncWrites: rm.syncWrites})
	case "s3":
		if !rm.hasKey {
			return nil, fmt.Errorf("cannot open S3 repo: no encryption key (open a local repo first)")
		}
		accessKey, err := decryptCredential(rm.encryptKey, cfg.AccessKeyEncrypted)
		if err != nil {
			return nil, fmt.Errorf("decrypt access key: %w", err)
		}
		secretKey, err := decryptCredential(rm.encryptKey, cfg.SecretKeyEncrypted)
		if err != nil {
			return nil, fmt.Errorf("decrypt secret key: %w", err)
		}
		region := cfg.Region
		if region == "" {
			region = "us-east-1"
		}
		return s3backend.New(cfg.Endpoint, cfg.Bucket, accessKey, secretKey, region)
	default:
		return nil, fmt.Errorf("unknown repo type %q", cfg.Type)
	}
}

func (rm *RepoManager) applyRepoSettings(repo *repository.Repository, be backend.Backend) {
	if rm.minFreeReserve > 0 {
		repo.SetOptions(repository.RepoOptions{
			MinFreeReserveBytes: rm.minFreeReserve,
		})
	}
	if lb, ok := be.(*local.Local); ok {
		lb.SetSyncWrites(rm.syncWrites)
	}
}

// decryptCredential decodes a base64-encoded AES-256-GCM ciphertext and decrypts it.
func decryptCredential(key [32]byte, encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	plaintext, err := crypto.Decrypt(key, ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}
