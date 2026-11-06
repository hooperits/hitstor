package server

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/verifyrestore"
)

// runRestoreVerify performs an asynchronous sample restore-verification after
// a successful backup finalize. Failures are recorded on the job and logged;
// they never fail the backup RPC itself.
func (s *Server) runRestoreVerify(jobID, repoName, snapshotIDHex string) {
	if jobID == "" || snapshotIDHex == "" {
		return
	}
	job := s.registry.GetJob(jobID)
	if job == nil {
		return
	}
	if !JobWantsVerify(*job) {
		return
	}

	repo := s.resolveRepo(repoName)
	if repo == nil {
		slog.Warn("restore verify skipped: no repository", "job", jobID, "repo", repoName)
		_ = s.registry.UpdateJobVerifyResult(jobID, JobVerifyResult{
			At:       time.Now(),
			Status:   verifyrestore.StatusSkipped,
			Snapshot: snapshotIDHex,
			Detail:   "no repository available for verification",
		})
		return
	}

	snapID, err := resolveSnapshotID(repo, snapshotIDHex)
	if err != nil {
		_ = s.registry.UpdateJobVerifyResult(jobID, JobVerifyResult{
			At:       time.Now(),
			Status:   verifyrestore.StatusError,
			Snapshot: snapshotIDHex,
			Detail:   err.Error(),
		})
		return
	}
	snapshotIDHex = snapID.String()

	slog.Info("restore verification started",
		"job", jobID,
		"snapshot", snapshotIDHex[:12],
		"repo", repoName,
	)

	res := verifyrestore.Verify(repo, snapID, verifyrestore.Options{
		MaxFiles: verifyrestore.DefaultMaxFiles,
		MaxBytes: verifyrestore.DefaultMaxBytes,
		Parallel: 4,
	})

	result := JobVerifyResult{
		At:       time.Now(),
		Snapshot: snapshotIDHex,
		Files:    res.FilesChecked,
		Bytes:    res.BytesChecked,
	}
	switch {
	case res.Error != "":
		result.Status = verifyrestore.StatusError
		result.Detail = res.Error
	case !res.OK:
		result.Status = verifyrestore.StatusFailed
		if len(res.Mismatches) > 0 {
			result.Detail = fmt.Sprintf("%d mismatch(es); first: %s: %s",
				len(res.Mismatches), res.Mismatches[0].Path, res.Mismatches[0].Reason)
		} else {
			result.Detail = "verification failed"
		}
	default:
		result.Status = verifyrestore.StatusOK
		if res.Sampled {
			result.Detail = fmt.Sprintf("sampled %d files, %d bytes in %s",
				res.FilesChecked, res.BytesChecked, res.Duration.Round(time.Millisecond))
		} else {
			result.Detail = fmt.Sprintf("%d files, %d bytes in %s",
				res.FilesChecked, res.BytesChecked, res.Duration.Round(time.Millisecond))
		}
	}

	if err := s.registry.UpdateJobVerifyResult(jobID, result); err != nil {
		slog.Warn("failed to persist verify result", "job", jobID, "error", err)
	}

	if result.Status == verifyrestore.StatusOK {
		slog.Info("restore verification ok",
			"job", jobID,
			"snapshot", snapshotIDHex[:12],
			"files", result.Files,
			"bytes", result.Bytes,
			"detail", result.Detail,
		)
	} else {
		slog.Warn("restore verification failed",
			"job", jobID,
			"snapshot", snapshotIDHex[:12],
			"status", result.Status,
			"detail", result.Detail,
		)
	}
}

func (s *Server) resolveRepo(name string) *repository.Repository {
	if name != "" {
		if mr, err := s.repos.Get(name); err == nil && mr != nil {
			return mr.Repo
		}
	}
	list := s.repos.List()
	if len(list) > 0 {
		return list[0].Repo
	}
	return nil
}

// resolveSnapshotID turns a snapshot ID, full or the short prefix that
// FinalizeBackup reports, into the full ID of a snapshot in repo.
func resolveSnapshotID(repo *repository.Repository, idHex string) (backend.ID, error) {
	var id backend.ID
	raw, err := hex.DecodeString(idHex)
	if err != nil || len(raw) == 0 || len(raw) > len(id) {
		return id, fmt.Errorf("invalid snapshot id %q", idHex)
	}
	if len(raw) == len(id) {
		copy(id[:], raw)
		return id, nil
	}
	ids, err := repo.ListSnapshots()
	if err != nil {
		return id, fmt.Errorf("list snapshots: %w", err)
	}
	found := 0
	for _, sid := range ids {
		if strings.HasPrefix(sid.String(), idHex) {
			id = sid
			found++
		}
	}
	switch found {
	case 0:
		return id, fmt.Errorf("snapshot %s not found", idHex)
	case 1:
		return id, nil
	}
	return backend.ID{}, fmt.Errorf("snapshot id %s is ambiguous", idHex)
}
