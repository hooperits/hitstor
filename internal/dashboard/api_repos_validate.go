package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	localbackend "github.com/hooperits/hitstor/internal/backend/local"
	s3backend "github.com/hooperits/hitstor/internal/backend/s3"
)

// testS3Request is the JSON body for POST /api/repos/test-s3.
type testS3Request struct {
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
}

// handleAPITestS3 tests S3 connectivity without creating a repo.
func (d *Dashboard) handleAPITestS3(w http.ResponseWriter, r *http.Request) {
	var req testS3Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Endpoint == "" || req.Bucket == "" || req.AccessKey == "" || req.SecretKey == "" {
		http.Error(w, "endpoint, bucket, access_key, and secret_key are required", http.StatusBadRequest)
		return
	}

	region := req.Region
	if region == "" {
		region = "us-east-1"
	}

	be, err := s3backend.New(req.Endpoint, req.Bucket, req.AccessKey, req.SecretKey, region)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"success": false, "message": fmt.Sprintf("create client: %v", err)})
		return
	}
	defer be.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := be.TestConnection(ctx); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"success": false, "message": fmt.Sprintf("connection failed: %v", err)})
		return
	}

	writeJSON(w, map[string]any{"success": true, "message": "Connection successful. Bucket accessible."})
}

// detectRepoRequest is the JSON body for POST /api/repos/detect.
type detectRepoRequest struct {
	Path string `json:"path"`
}

// handleAPIDetectRepo checks if a path contains an existing repository.
func (d *Dashboard) handleAPIDetectRepo(w http.ResponseWriter, r *http.Request) {
	var req detectRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	// Check if the config file exists in the path.
	configPath := filepath.Join(req.Path, "config")
	if _, err := os.Stat(configPath); err != nil {
		writeJSON(w, map[string]any{"exists": false})
		return
	}

	// Try to open the repo (without password — just check structure).
	be, err := localbackend.New(req.Path)
	if err != nil {
		writeJSON(w, map[string]any{"exists": false})
		return
	}

	// Count snapshots by listing snapshot directory.
	snapIDs, _ := be.List(context.Background(), backend.SnapshotFile)
	be.Close()

	writeJSON(w, map[string]any{
		"exists":    true,
		"snapshots": len(snapIDs),
	})
}

// validatePathRequest is the JSON body for POST /api/repos/validate-path.
type validatePathRequest struct {
	Path string `json:"path"`
	Mode string `json:"mode"` // "create" or "connect"
}

// handleAPIValidatePath checks whether a local path is suitable for create/connect.
func (d *Dashboard) handleAPIValidatePath(w http.ResponseWriter, r *http.Request) {
	var req validatePathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	if !filepath.IsAbs(req.Path) {
		writeJSON(w, map[string]any{
			"valid":    false,
			"writable": false,
			"exists":   false,
			"error":    "absolute path is required",
		})
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = "create"
	}

	configPath := filepath.Join(req.Path, "config")
	_, configErr := os.Stat(configPath)
	repoExists := configErr == nil

	writable, writeErr := pathWritable(req.Path)
	var warnings []string
	spacePath := req.Path
	if info, err := os.Stat(req.Path); err != nil || !info.IsDir() {
		spacePath = filepath.Dir(req.Path)
	}
	if total, free, err := getDiskSpace(spacePath); err == nil && total > 0 {
		freePct := float64(free) / float64(total) * 100
		if free < 10*1024*1024*1024 {
			warnings = append(warnings, "Less than 10 GB free on this filesystem")
		}
		if freePct < 15 {
			warnings = append(warnings, fmt.Sprintf("Only %.0f%% disk space remaining", freePct))
		}
	}

	valid := true
	var errMsg string
	switch mode {
	case "connect":
		if !repoExists {
			valid = false
			errMsg = "no repository found at this path"
		}
	case "create":
		if repoExists {
			valid = false
			errMsg = "repository already exists; use connect mode"
		}
	default:
		http.Error(w, "mode must be \"create\" or \"connect\"", http.StatusBadRequest)
		return
	}
	if valid && !writable {
		valid = false
		errMsg = writeErr
	}

	resp := map[string]any{
		"valid":    valid,
		"writable": writable,
		"exists":   repoExists,
		"warnings": warnings,
	}
	if errMsg != "" {
		resp["error"] = errMsg
	}
	writeJSON(w, resp)
}

func pathWritable(path string) (bool, string) {
	info, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, fmt.Sprintf("stat path: %v", err)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false, "invalid path"
		}
		parentInfo, perr := os.Stat(parent)
		if perr != nil {
			return false, "parent directory does not exist"
		}
		if !parentInfo.IsDir() {
			return false, "parent is not a directory"
		}
		testFile := filepath.Join(parent, ".hitstor-write-test")
		if err := os.WriteFile(testFile, []byte("x"), 0600); err != nil {
			return false, "parent directory is not writable"
		}
		os.Remove(testFile)
		return true, ""
	}
	if !info.IsDir() {
		return false, "path is not a directory"
	}
	testFile := filepath.Join(path, ".hitstor-write-test")
	if err := os.WriteFile(testFile, []byte("x"), 0600); err != nil {
		return false, "directory is not writable"
	}
	os.Remove(testFile)
	return true, ""
}

func backendHasRepo(ctx context.Context, be backend.Backend, repoType, path string) (bool, error) {
	switch repoType {
	case "local":
		configPath := filepath.Join(path, "config")
		_, err := os.Stat(configPath)
		return err == nil, nil
	case "s3":
		sb, ok := be.(*s3backend.S3Backend)
		if !ok {
			return false, fmt.Errorf("expected S3 backend")
		}
		return sb.HasRepoConfig(ctx)
	default:
		return false, fmt.Errorf("unsupported repo type %q", repoType)
	}
}

// handleAPIDetectS3 checks if an S3 bucket contains an existing repository.
func (d *Dashboard) handleAPIDetectS3(w http.ResponseWriter, r *http.Request) {
	var req testS3Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Endpoint == "" || req.Bucket == "" || req.AccessKey == "" || req.SecretKey == "" {
		http.Error(w, "endpoint, bucket, access_key, and secret_key are required", http.StatusBadRequest)
		return
	}
	region := req.Region
	if region == "" {
		region = "us-east-1"
	}

	be, err := s3backend.New(req.Endpoint, req.Bucket, req.AccessKey, req.SecretKey, region)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"exists": false, "error": fmt.Sprintf("create client: %v", err)})
		return
	}
	defer be.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := be.TestConnection(ctx); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"exists": false, "error": fmt.Sprintf("connection failed: %v", err)})
		return
	}

	exists, err := be.HasRepoConfig(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("check config: %v", err), http.StatusInternalServerError)
		return
	}
	if !exists {
		writeJSON(w, map[string]any{"exists": false})
		return
	}

	snapIDs, _ := be.List(ctx, backend.SnapshotFile)
	writeJSON(w, map[string]any{
		"exists":    true,
		"snapshots": len(snapIDs),
	})
}
