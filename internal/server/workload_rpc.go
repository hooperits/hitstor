package server

import (
	"context"
	"path"
	"slices"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hooperits/hitstor/internal/plugin"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/workloadcat"
)

// knownRecentPerObject caps the non-full items GetWorkloadKnown returns per
// object: a chain check needs the backup that ends the chain, which is
// recent. Fulls are all returned, since a differential's base may be old.
const knownRecentPerObject = 50

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
	for _, mr := range s.repos.List() {
		ids, err := mr.Repo.ListSnapshots()
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			snap, err := mr.Repo.LoadSnapshot(id)
			if err != nil || !slices.Contains(snap.Tags, "plugin:"+job.Plugin) || !slices.ContainsFunc(snap.Tags, func(t string) bool { return jobTags[t] }) {
				continue
			}
			items, err := workloadcat.Default.SnapshotItems(mr.Repo, id, snap, job.Plugin)
			if err != nil {
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
		recent := 0
		for _, it := range items {
			if it.Manifest.Kind != "full" {
				if recent >= knownRecentPerObject {
					continue
				}
				recent++
			}
			out = append(out, it)
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
	for _, f := range files {
		rel, ok := strings.CutPrefix(f.Path, pluginID+"/")
		if f.IsDir || !ok || strings.HasSuffix(rel, plugin.MetadataSuffix) {
			continue
		}
		if obj := path.Dir(rel); obj != "." && !seen[obj] {
			seen[obj] = true
			tags = append(tags, "obj:"+obj)
		}
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
