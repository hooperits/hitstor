package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/fs"
	"github.com/hooperits/hitstor/internal/plugin"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
)

// workloadPoint is one item a workload plugin produced, found in a snapshot.
type workloadPoint struct {
	Ref        string          `json:"ref"`
	SnapshotID string          `json:"snapshot_id"`
	Item       string          `json:"item"`
	Kind       string          `json:"kind,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	Size       int64           `json:"size"`
	SHA256     string          `json:"sha256"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`

	repo    *repository.Repository
	content []fs.ID
}

// readNodeContent calls fn with each decompressed chunk of a file node.
func readNodeContent(repo *repository.Repository, content []fs.ID, fn func([]byte) error) error {
	for _, id := range content {
		blob, packID := repo.LookupBlob(repository.ID(id))
		if blob == nil {
			return fmt.Errorf("blob %s not found in index", backend.ID(id))
		}
		compressed, err := repo.LoadBlob(*packID, *blob)
		if err != nil {
			return fmt.Errorf("load blob %s: %w", backend.ID(id), err)
		}
		data, err := compress.Decompress(compressed)
		if err != nil {
			return fmt.Errorf("decompress blob %s: %w", backend.ID(id), err)
		}
		if err := fn(data); err != nil {
			return err
		}
	}
	return nil
}

// findTreeDir descends from root along slash-separated dir, returning nil
// when any component is missing.
func findTreeDir(repo *repository.Repository, root *restoreTree, dir string) *restoreTree {
	cur := root
	for _, name := range strings.Split(dir, "/") {
		var next *restoreTree
		for _, n := range cur.Nodes {
			if n.Name == name && n.Type == fs.NodeTypeDir {
				t, err := loadRestoreTree(repo, backend.ID(n.Subtree))
				if err != nil {
					return nil
				}
				next = t
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

// collectItemManifests walks t (the directory at itemDir inside the plugin's
// snapshot directory) and returns a point for every item with a manifest.
func collectItemManifests(repo *repository.Repository, snapHex string, t *restoreTree, itemDir string, out *[]workloadPoint) {
	files := make(map[string]fs.Node)
	for _, n := range t.Nodes {
		switch n.Type {
		case fs.NodeTypeFile:
			files[n.Name] = n
		case fs.NodeTypeDir:
			if sub, err := loadRestoreTree(repo, backend.ID(n.Subtree)); err == nil {
				collectItemManifests(repo, snapHex, sub, path.Join(itemDir, n.Name), out)
			}
		}
	}
	for name, n := range files {
		itemName, ok := strings.CutSuffix(name, plugin.MetadataSuffix)
		if !ok {
			continue
		}
		item, ok := files[itemName]
		if !ok {
			continue
		}
		var raw []byte
		if err := readNodeContent(repo, n.Content, func(b []byte) error { raw = append(raw, b...); return nil }); err != nil {
			slog.Warn("workload restore: unreadable item manifest", "snapshot", snapHex, "item", path.Join(itemDir, name), "error", err)
			continue
		}
		var m plugin.ItemManifest
		if err := json.Unmarshal(raw, &m); err != nil {
			slog.Warn("workload restore: invalid item manifest", "snapshot", snapHex, "item", path.Join(itemDir, name), "error", err)
			continue
		}
		*out = append(*out, workloadPoint{
			Ref:        snapHex[:16] + ":" + m.Item,
			SnapshotID: snapHex,
			Item:       m.Item,
			Kind:       m.Kind,
			CreatedAt:  m.CreatedAt,
			Size:       m.Size,
			SHA256:     m.SHA256,
			Metadata:   m.Metadata,
			repo:       repo,
			content:    item.Content,
		})
	}
}

// workloadSnapshotMatches reports whether a workload snapshot belongs to
// sourceAgent: through one of the agent's jobs, or by hostname/tag.
func (d *Dashboard) workloadSnapshotMatches(snap *repository.Snapshot, sourceAgent string, jobIDs map[string]bool) bool {
	for _, tag := range snap.Tags {
		if id, ok := strings.CutPrefix(tag, "job:"); ok && jobIDs[id] {
			return true
		}
	}
	return d.snapshotMatchesAgent(cachedSnapshotSummary{Hostname: snap.Hostname, Tags: snap.Tags}, sourceAgent)
}

// workloadPoints returns, oldest first, every item that the plugin produced
// for object on sourceAgent, across all repositories.
func (d *Dashboard) workloadPoints(sourceAgent, pluginID, object string) ([]workloadPoint, error) {
	return d.workloadItems(sourceAgent, pluginID, object, "")
}

// workloadItems is workloadPoints for every object when object is empty,
// and for one snapshot (an ID prefix) when snapshot is set.
func (d *Dashboard) workloadItems(sourceAgent, pluginID, object, snapshot string) ([]workloadPoint, error) {
	jobIDs := make(map[string]bool)
	for _, j := range d.registry.ListJobsByAgent(sourceAgent) {
		jobIDs[j.ID] = true
	}
	var repos []*repository.Repository
	if d.repos != nil {
		for _, mr := range d.repos.List() {
			repos = append(repos, mr.Repo)
		}
	} else if d.repo != nil {
		repos = append(repos, d.repo)
	}

	var points []workloadPoint
	seen := make(map[string]bool)
	for _, repo := range repos {
		ids, err := repo.ListSnapshots()
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if snapshot != "" && !strings.HasPrefix(id.String(), snapshot) {
				continue
			}
			snap, err := repo.LoadSnapshot(id)
			if err != nil || !slices.Contains(snap.Tags, "plugin:"+pluginID) || !d.workloadSnapshotMatches(snap, sourceAgent, jobIDs) {
				continue
			}
			root, err := loadRestoreTree(repo, backend.ID(snap.Tree))
			if err != nil {
				continue
			}
			dir := findTreeDir(repo, root, strings.TrimSuffix(pluginID+"/"+object, "/"))
			if dir == nil {
				continue
			}
			var found []workloadPoint
			collectItemManifests(repo, id.String(), dir, object, &found)
			for _, p := range found {
				// An item is the same backup wherever it is stored.
				if key := p.Item + "|" + p.SHA256; !seen[key] {
					seen[key] = true
					points = append(points, p)
				}
			}
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].CreatedAt.Before(points[j].CreatedAt) })
	return points, nil
}

// workloadSettings returns the plugin settings of the first workload job for
// pluginID on agentName, so browse and restore connect the way backups do.
func (d *Dashboard) workloadSettings(agentName, pluginID string) map[string]string {
	for _, j := range d.registry.ListJobsByAgent(agentName) {
		if server.IsWorkloadJobType(j.JobType) && j.Plugin == pluginID {
			return j.PluginSettings
		}
	}
	return nil
}

// WorkloadRestoreSession tracks one restore through a workload plugin.
type WorkloadRestoreSession struct {
	ID          string            `json:"id"`
	AgentName   string            `json:"agent_name"`
	SourceAgent string            `json:"source_agent"`
	Plugin      string            `json:"plugin"`
	Object      string            `json:"object"`
	Target      *time.Time        `json:"target,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
	StartTime   time.Time         `json:"start_time"`

	mu        sync.Mutex
	status    string // planning, restoring, completed, failed
	plan      []string
	message   string
	err       string
	endTime   time.Time
	bytesSent atomic.Int64
	bytesTot  int64
	settings  map[string]string
	language  string // for the plugin's preview messages
}

type workloadRestoreJSON struct {
	ID          string            `json:"id"`
	AgentName   string            `json:"agent_name"`
	SourceAgent string            `json:"source_agent"`
	Plugin      string            `json:"plugin"`
	Object      string            `json:"object"`
	Target      *time.Time        `json:"target,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
	Status      string            `json:"status"`
	Plan        []string          `json:"plan,omitempty"`
	BytesTotal  int64             `json:"bytes_total"`
	BytesSent   int64             `json:"bytes_sent"`
	Message     string            `json:"message,omitempty"`
	Error       string            `json:"error,omitempty"`
	StartTime   time.Time         `json:"start_time"`
	EndTime     *time.Time        `json:"end_time,omitempty"`
}

func (s *WorkloadRestoreSession) snapshot() workloadRestoreJSON {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := workloadRestoreJSON{
		ID: s.ID, AgentName: s.AgentName, SourceAgent: s.SourceAgent, Plugin: s.Plugin, Object: s.Object,
		Target: s.Target, Options: s.Options, Status: s.status, Plan: s.plan,
		BytesTotal: s.bytesTot, BytesSent: s.bytesSent.Load(), Message: s.message, Error: s.err, StartTime: s.StartTime,
	}
	if !s.endTime.IsZero() {
		t := s.endTime
		out.EndTime = &t
	}
	return out
}

func (s *WorkloadRestoreSession) setStatus(status string) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
}

func (s *WorkloadRestoreSession) fail(err error) {
	slog.Error("workload restore failed", "session", s.ID, "agent", s.AgentName, "plugin", s.Plugin, "object", s.Object, "error", err)
	s.mu.Lock()
	s.status, s.err, s.endTime = "failed", err.Error(), time.Now()
	s.mu.Unlock()
}

func newWorkloadRestoreID() string {
	var b [8]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// runWorkloadRestore plans the restore on the target agent and streams the
// planned items to it.
func (d *Dashboard) runWorkloadRestore(ctx context.Context, sess *WorkloadRestoreSession, points []workloadPoint) {
	agentRec := d.registry.GetAgent(sess.AgentName)
	if agentRec == nil || agentRec.AgentAddr == "" {
		sess.fail(fmt.Errorf("agent %q has no gRPC address", sess.AgentName))
		return
	}
	token, err := d.registry.DecryptToken(sess.AgentName)
	if err != nil {
		sess.fail(fmt.Errorf("decrypt agent token: %w", err))
		return
	}
	client, err := d.dialManagedAgent(sess.AgentName, agentRec.AgentAddr)
	if err != nil {
		sess.fail(fmt.Errorf("dial agent: %w", err))
		return
	}
	defer client.Close()

	var target int64
	if sess.Target != nil {
		target = sess.Target.UnixNano()
	}
	_, planned, err := d.planWorkloadRestore(ctx, client, token, sess, points)
	if err != nil {
		sess.fail(err)
		return
	}
	var total int64
	for _, p := range planned {
		total += p.Size
	}
	sess.mu.Lock()
	for _, p := range planned {
		sess.plan = append(sess.plan, p.Item)
	}
	sess.bytesTot = total
	sess.status = "restoring"
	sess.mu.Unlock()

	stream, err := client.RestoreWorkload(ctx)
	if err != nil {
		sess.fail(fmt.Errorf("open restore stream: %w", err))
		return
	}
	sendErr := d.sendWorkloadItems(stream, sess, token, target, planned)
	resp, err := stream.CloseAndRecv()
	// A local failure (an unreadable repository blob) explains the outcome
	// better than the agent's view of a stream that stopped mid-item.
	if sendErr != nil && !errors.Is(sendErr, errStreamClosed) {
		sess.fail(sendErr)
		return
	}
	if err != nil {
		sess.fail(err)
		return
	}
	sess.mu.Lock()
	sess.status, sess.message, sess.endTime = "completed", resp.Message, time.Now()
	sess.mu.Unlock()
	slog.Info("workload restore complete", "session", sess.ID, "agent", sess.AgentName, "plugin", sess.Plugin,
		"object", sess.Object, "items", len(planned), "message", resp.Message)
}

var errStreamClosed = errors.New("agent closed the restore stream")

func (d *Dashboard) sendWorkloadItems(stream pb.AgentService_RestoreWorkloadClient, sess *WorkloadRestoreSession, token string, target int64, planned []workloadPoint) error {
	send := func(m *pb.WorkloadRestoreMessage) error {
		if err := stream.Send(m); err != nil {
			// The agent's own error arrives through CloseAndRecv.
			return errStreamClosed
		}
		return nil
	}
	if err := send(&pb.WorkloadRestoreMessage{Payload: &pb.WorkloadRestoreMessage_Begin{Begin: &pb.WorkloadRestoreBegin{
		Token: token, Plugin: sess.Plugin, Settings: sess.settings, Object: sess.Object,
		TargetUnixNanos: target, Options: sess.Options, ItemCount: int32(len(planned)),
	}}}); err != nil {
		return err
	}
	for _, p := range planned {
		if err := send(&pb.WorkloadRestoreMessage{Payload: &pb.WorkloadRestoreMessage_Start{Start: &pb.WorkloadItemStart{
			Ref: p.Ref, Item: p.Item, Metadata: p.Metadata, Size: p.Size, Sha256: p.SHA256,
		}}}); err != nil {
			return err
		}
		err := readNodeContent(p.repo, p.content, func(data []byte) error {
			for len(data) > 0 {
				n := min(len(data), restoreChunkSize)
				if err := send(&pb.WorkloadRestoreMessage{Payload: &pb.WorkloadRestoreMessage_Data{Data: data[:n]}}); err != nil {
					return err
				}
				sess.bytesSent.Add(int64(n))
				data = data[n:]
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("stream item %s: %w", p.Item, err)
		}
		if err := send(&pb.WorkloadRestoreMessage{Payload: &pb.WorkloadRestoreMessage_End{End: &pb.WorkloadItemEnd{}}}); err != nil {
			return err
		}
	}
	return nil
}

// planWorkloadRestore asks the agent which of points restore sess's object,
// and what the restore will do. It returns the planned points in order.
func (d *Dashboard) planWorkloadRestore(ctx context.Context, client *server.AgentClient, token string, sess *WorkloadRestoreSession, points []workloadPoint) (*pb.PlanWorkloadRestoreResponse, []workloadPoint, error) {
	var target int64
	if sess.Target != nil {
		target = sess.Target.UnixNano()
	}
	byRef := make(map[string]workloadPoint, len(points))
	planReq := &pb.PlanWorkloadRestoreRequest{
		Token: token, Plugin: sess.Plugin, Settings: sess.settings, Object: sess.Object,
		TargetUnixNanos: target, Options: sess.Options, Language: sess.language,
	}
	for _, p := range points {
		byRef[p.Ref] = p
		planReq.Candidates = append(planReq.Candidates, &pb.WorkloadCandidate{
			Ref: p.Ref, Item: p.Item, Kind: p.Kind, CreatedUnixNanos: p.CreatedAt.UnixNano(), Metadata: p.Metadata,
		})
	}
	plan, err := client.PlanWorkloadRestore(ctx, planReq)
	if err != nil {
		return nil, nil, fmt.Errorf("plan restore: %w", err)
	}
	planned := make([]workloadPoint, 0, len(plan.Refs))
	for _, ref := range plan.Refs {
		p, ok := byRef[ref]
		if !ok {
			return nil, nil, fmt.Errorf("plan references unknown item %q", ref)
		}
		planned = append(planned, p)
	}
	return plan, planned, nil
}
