package server

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"slices"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hooperits/hitstor/internal/plugin"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/workloadcat"
)

// knownPerKind caps the items of each kind GetWorkloadKnown returns per
// object, newest first. Every full is returned: a differential's base is
// the newest full that is not copy-only, which other jobs' copy-only fulls
// may follow by the dozen, and retention bounds them anyway. A log check
// needs the backup that ends the chain: the newest log, or a full.
var knownPerKind = map[string]int{"full": -1, "differential": 5, "log": 20}

// knownOtherKinds caps items of kinds a plugin defines beyond those.
const knownOtherKinds = 20

// GetWorkloadKnown implements BackupService.GetWorkloadKnown: the items
// the repositories hold for the job's selection, from the job's agent.
func (s *Server) GetWorkloadKnown(ctx context.Context, req *pb.GetWorkloadKnownRequest) (*pb.GetWorkloadKnownResponse, error) {
	agentName, ok := s.registry.VerifyKey(req.ApiKey)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "invalid API key")
	}
	job := s.registry.GetJob(req.JobId)
	if job == nil {
		return nil, status.Errorf(codes.NotFound, "backup job %q not found", req.JobId)
	}
	if job.AgentName != agentName {
		return nil, status.Error(codes.PermissionDenied, "job does not belong to this agent")
	}
	if job.JobType != JobTypeWorkload {
		return nil, status.Error(codes.InvalidArgument, "not a workload job")
	}
	items, err := s.knownWorkloadItems(*job)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list workload items: %v", err)
	}
	resp := &pb.GetWorkloadKnownResponse{}
	for _, it := range items {
		resp.Items = append(resp.Items, &pb.WorkloadKnownItem{Item: it.Manifest.Item, Metadata: it.Manifest.Metadata})
	}
	return resp, nil
}

// knownWorkloadItems returns the items of the job's plugin that snapshots
// of the agent's jobs hold for objects in the job's selection: every full,
// and the most recent others of each object.
func (s *Server) knownWorkloadItems(job BackupJob) ([]workloadcat.Item, error) {
	jobTags := map[string]bool{}
	for _, j := range s.registry.ListJobsByAgent(job.AgentName) {
		jobTags["job:"+j.ID] = true
	}
	byObject := map[string][]workloadcat.Item{}
	seen := map[string]bool{}
	// A snapshot that cannot be read is left out, with a warning. The
	// plugin then finds no base in it and asks for a full backup: a visible
	// failure. Failing the call instead would send the plugin back to its
	// msdb check, which cannot tell a backup HitStor no longer holds. The
	// catalog caches nothing from a failed read, so the next run retries.
	// Only the job's repository: retention keeps chains per repository, so
	// a base held elsewhere could be forgotten under this job's backups.
	var repos []*repository.Repository
	for _, mr := range s.repos.List() {
		repos = append(repos, mr.Repo)
	}
	workloadcat.Default.RetainRepos(repos)
	mr, err := s.repos.Get(job.RepoName)
	if err != nil {
		return nil, fmt.Errorf("repository %s: %w", job.RepoName, err)
	}
	for _, repo := range []*repository.Repository{mr.Repo} {
		ids, err := repo.ListSnapshots()
		if err != nil {
			slog.Warn("workload catalog: cannot list snapshots", "error", err)
			continue
		}
		workloadcat.Default.Retain(repo, ids)
		for _, id := range ids {
			snap, err := repo.LoadSnapshot(id)
			if err != nil {
				slog.Warn("workload catalog: unreadable snapshot", "snapshot", id.String()[:8], "error", err)
				continue
			}
			if !slices.Contains(snap.Tags, "plugin:"+job.Plugin) || !slices.ContainsFunc(snap.Tags, func(t string) bool { return jobTags[t] }) {
				continue
			}
			items, err := workloadcat.Default.SnapshotItems(repo, id, snap, job.Plugin)
			if err != nil {
				slog.Warn("workload catalog: unreadable snapshot", "snapshot", id.String()[:8], "error", err)
				continue
			}
			for _, it := range items {
				obj := it.Object()
				key := it.Manifest.Item + "|" + it.Manifest.SHA256
				if seen[key] || !SelectionCovers(job.Selection, obj) {
					continue
				}
				seen[key] = true
				byObject[obj] = append(byObject[obj], it)
			}
		}
	}
	var out []workloadcat.Item
	for _, items := range byObject {
		sort.Slice(items, func(i, j int) bool { return items[i].Manifest.CreatedAt.After(items[j].Manifest.CreatedAt) })
		taken := map[string]int{}
		for _, it := range items {
			kind := it.Manifest.Kind
			if kind == "" {
				kind = "full" // jobs made before kinds took fulls
			}
			limit, ok := knownPerKind[kind]
			if !ok {
				limit, kind = knownOtherKinds, "other"
			}
			if limit < 0 || taken[kind] < limit {
				taken[kind]++
				out = append(out, it)
			}
		}
	}
	return out, nil
}

// SelectionCovers reports whether a selection includes object: the object
// itself or a container of it (an instance holds its databases).
func SelectionCovers(selection []string, object string) bool {
	for _, s := range selection {
		if object == s || strings.HasPrefix(object, s+"/") {
			return true
		}
	}
	return false
}

// workloadObjectTags returns an "obj:<object>" tag for each object whose
// items a workload snapshot holds; nil for other snapshots. Files are stored
// as <plugin>/<object>/<item>, next to their manifests.
func workloadObjectTags(paths []string, files []*pb.FileNode) []string {
	if len(paths) != 1 || !strings.HasPrefix(paths[0], "workload:") {
		return nil
	}
	pluginID := strings.TrimPrefix(paths[0], "workload:")
	seen := map[string]bool{}
	var tags []string
	items := 0
	for _, f := range files {
		rel, ok := strings.CutPrefix(f.Path, pluginID+"/")
		if f.IsDir || !ok || strings.HasSuffix(rel, plugin.MetadataSuffix) {
			continue
		}
		items++
		obj := path.Dir(rel)
		if obj == "." {
			// An item outside any object: the snapshot may hold anything,
			// as an untagged one does.
			return nil
		}
		if !seen[obj] {
			seen[obj] = true
			tags = append(tags, "obj:"+obj)
		}
	}
	if items == 0 {
		// Every object was left out: say so, or retention would take the
		// snapshot for an older one that may hold any object.
		return []string{"objects:none"}
	}
	sort.Strings(tags)
	return tags
}

func outcomesFromProto(rs []*pb.WorkloadObjectResult) []ObjectOutcome {
	var out []ObjectOutcome
	for _, r := range rs {
		out = append(out, ObjectOutcome{Object: r.Object, Status: r.Status, Reason: r.Reason, Message: r.Message})
	}
	return out
}
