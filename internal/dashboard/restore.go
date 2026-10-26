package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/fs"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"

	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/restorer"
	"github.com/hooperits/hitstor/internal/server"
)

// RestoreSession tracks an active or completed restore operation.
type RestoreSession struct {
	ID            string    `json:"id"`
	SnapshotID    string    `json:"snapshot_id"`
	AgentName     string    `json:"agent_name"`
	TargetPath    string    `json:"target_path"`
	Includes      []string  `json:"includes,omitempty"`
	Status        string    `json:"status"` // "running", "completed", "failed"
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time,omitempty"`
	FilesTotal    uint64    `json:"files_total"`
	FilesRestored atomic.Uint64
	BytesTotal    uint64 `json:"bytes_total"`
	BytesWritten  atomic.Uint64
	CurrentFile   string   `json:"current_file"`
	Errors        []string `json:"errors,omitempty"`
	mu            sync.Mutex
}

// Progress returns a snapshot of the current restore progress.
func (s *RestoreSession) Progress() restoreProgress {
	s.mu.Lock()
	currentFile := s.CurrentFile
	errs := make([]string, len(s.Errors))
	copy(errs, s.Errors)
	status := s.Status
	endTime := s.EndTime
	s.mu.Unlock()

	mode := "agent"
	if s.AgentName == "" && s.TargetPath != "" {
		mode = "server"
	}

	return restoreProgress{
		ID:            s.ID,
		SnapshotID:    s.SnapshotID,
		AgentName:     s.AgentName,
		Mode:          mode,
		Status:        status,
		FilesRestored: s.FilesRestored.Load(),
		BytesWritten:  s.BytesWritten.Load(),
		FilesTotal:    s.FilesTotal,
		BytesTotal:    s.BytesTotal,
		CurrentFile:   currentFile,
		Elapsed:       time.Since(s.StartTime).Seconds(),
		Errors:        errs,
		EndTime:       endTime,
	}
}

type restoreProgress struct {
	ID            string    `json:"id"`
	SnapshotID    string    `json:"snapshot_id"`
	AgentName     string    `json:"agent_name"`
	Mode          string    `json:"mode,omitempty"`
	Status        string    `json:"status"`
	FilesRestored uint64    `json:"files_restored"`
	BytesWritten  uint64    `json:"bytes_written"`
	FilesTotal    uint64    `json:"files_total"`
	BytesTotal    uint64    `json:"bytes_total"`
	CurrentFile   string    `json:"current_file"`
	Elapsed       float64   `json:"elapsed"`
	Errors        []string  `json:"errors,omitempty"`
	EndTime       time.Time `json:"end_time,omitempty"`
}

// RestoreManager manages restore sessions.
type RestoreManager struct {
	mu       sync.Mutex
	sessions map[string]*RestoreSession
}

// NewRestoreManager creates a new RestoreManager.
func NewRestoreManager() *RestoreManager {
	return &RestoreManager{
		sessions: make(map[string]*RestoreSession),
	}
}

// StartRestore creates a new restore session and returns it.
func (m *RestoreManager) StartRestore(snapshotID, agentName, targetPath string, includes []string) *RestoreSession {
	id := generateRestoreID()
	sess := &RestoreSession{
		ID:         id,
		SnapshotID: snapshotID,
		AgentName:  agentName,
		TargetPath: targetPath,
		Includes:   includes,
		Status:     "running",
		StartTime:  time.Now(),
	}
	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()
	return sess
}

// GetSession returns a restore session by ID.
func (m *RestoreManager) GetSession(id string) *RestoreSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// ListSessions returns all restore sessions.
func (m *RestoreManager) ListSessions() []*RestoreSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*RestoreSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

// HasActiveRestore returns true if there's an active restore for the given agent.
func (m *RestoreManager) HasActiveRestore(agentName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.AgentName == agentName && s.Status == "running" {
			return true
		}
	}
	return false
}

func generateRestoreID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// tree is a local copy of archiver.Tree to avoid importing the archiver package.
type restoreTree struct {
	Nodes []fs.Node `json:"nodes"`
}

// RunServerRestore extracts selected snapshot paths to a directory on the server host.
func RunServerRestore(sess *RestoreSession, repo *repository.Repository) {
	RunServerRestoreWithHook(sess, repo, nil)
}

// RunServerRestoreWithHook runs a server-side restore and optionally invokes afterHook
// after files are extracted but before the session is marked completed.
// Hook errors are logged; the restore still completes if extraction succeeded.
func RunServerRestoreWithHook(sess *RestoreSession, repo *repository.Repository, afterHook func() error) {
	slog.Info("server restore: starting",
		"session", sess.ID,
		"snapshot", sess.SnapshotID,
		"target", sess.TargetPath,
	)

	snapID, err := parseSnapshotID(sess.SnapshotID)
	if err != nil {
		failRestore(sess, fmt.Sprintf("parse snapshot ID: %v", err))
		return
	}

	r := restorer.New(repo)
	r.SetProgress(func(path string, size uint64) {
		sess.mu.Lock()
		sess.CurrentFile = path
		sess.mu.Unlock()
		sess.FilesRestored.Add(1)
		sess.BytesWritten.Add(size)
	})
	r.SetTreeWalkComplete(func(files int, totalBytes uint64) {
		sess.FilesTotal = uint64(files)
		sess.BytesTotal = totalBytes
	})

	if err := r.Restore(snapID, sess.TargetPath, sess.Includes); err != nil {
		failRestore(sess, err.Error())
		return
	}

	if afterHook != nil {
		if err := afterHook(); err != nil {
			slog.Warn("server restore: post-restore hook failed", "session", sess.ID, "error", err)
		}
	}

	sess.mu.Lock()
	sess.Status = "completed"
	sess.EndTime = time.Now()
	sess.mu.Unlock()

	slog.Info("server restore: complete",
		"session", sess.ID,
		"files", sess.FilesRestored.Load(),
		"bytes", sess.BytesWritten.Load(),
		"duration", time.Since(sess.StartTime),
	)
}

// RunRestore executes the restore operation: loads snapshot from repo, streams
// data to agent via gRPC, and updates the session with progress.
func RunRestore(sess *RestoreSession, repo *repository.Repository, registry *server.AgentRegistry) {
	slog.Info("restore: starting",
		"session", sess.ID,
		"snapshot", sess.SnapshotID,
		"agent", sess.AgentName,
		"target", sess.TargetPath,
	)

	snapID, err := parseSnapshotID(sess.SnapshotID)
	if err != nil {
		failRestore(sess, fmt.Sprintf("parse snapshot ID: %v", err))
		return
	}

	snap, err := repo.LoadSnapshot(snapID)
	if err != nil {
		failRestore(sess, fmt.Sprintf("load snapshot: %v", err))
		return
	}

	// Phase 1: Walk tree to count files and bytes.
	rootTree, err := loadRestoreTree(repo, backend.ID(snap.Tree))
	if err != nil {
		failRestore(sess, fmt.Sprintf("load root tree: %v", err))
		return
	}

	var filesTotal uint64
	var bytesTotal uint64
	countFiles(repo, rootTree, &filesTotal, &bytesTotal, sess.Includes, "")
	sess.FilesTotal = filesTotal
	sess.BytesTotal = bytesTotal

	// Look up agent address and token.
	agent := registry.GetAgent(sess.AgentName)
	if agent == nil {
		failRestore(sess, fmt.Sprintf("agent %q not found", sess.AgentName))
		return
	}
	if agent.AgentAddr == "" {
		failRestore(sess, fmt.Sprintf("agent %q has no gRPC address", sess.AgentName))
		return
	}
	token, err := registry.DecryptToken(sess.AgentName)
	if err != nil {
		failRestore(sess, fmt.Sprintf("decrypt agent token: %v", err))
		return
	}

	// Dial agent.
	agentClient, err := dialPinnedAgent(registry, sess.AgentName, agent.AgentAddr)
	if err != nil {
		failRestore(sess, fmt.Sprintf("dial agent: %v", err))
		return
	}
	defer agentClient.Close()

	ctx := context.Background()
	stream, err := agentClient.RestoreData(ctx)
	if err != nil {
		failRestore(sess, fmt.Sprintf("open restore stream: %v", err))
		return
	}

	// Send RestoreBegin.
	if err := stream.Send(&pb.RestoreDataRequest{
		Payload: &pb.RestoreDataRequest_Begin{
			Begin: &pb.RestoreBegin{
				Token:      token,
				SnapshotId: sess.SnapshotID,
				TargetPath: sess.TargetPath,
				FilesTotal: filesTotal,
				BytesTotal: bytesTotal,
			},
		},
	}); err != nil {
		failRestore(sess, fmt.Sprintf("send begin: %v", err))
		return
	}

	// Phase 2: Walk tree again, streaming data to agent.
	if err := streamTree(repo, stream, sess, rootTree, "", sess.Includes, ""); err != nil {
		failRestore(sess, fmt.Sprintf("stream data: %v", err))
		return
	}

	// Send RestoreEnd.
	if err := stream.Send(&pb.RestoreDataRequest{
		Payload: &pb.RestoreDataRequest_End{End: &pb.RestoreEnd{}},
	}); err != nil {
		failRestore(sess, fmt.Sprintf("send end: %v", err))
		return
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		failRestore(sess, fmt.Sprintf("close stream: %v", err))
		return
	}

	sess.mu.Lock()
	sess.Status = "completed"
	sess.EndTime = time.Now()
	sess.FilesRestored.Store(resp.FilesRestored)
	sess.BytesWritten.Store(resp.BytesWritten)
	if len(resp.Errors) > 0 {
		sess.Errors = append(sess.Errors, resp.Errors...)
	}
	sess.mu.Unlock()

	slog.Info("restore: complete",
		"session", sess.ID,
		"files", resp.FilesRestored,
		"bytes", resp.BytesWritten,
		"errors", len(resp.Errors),
		"duration", time.Since(sess.StartTime),
	)
}

func failRestore(sess *RestoreSession, msg string) {
	slog.Error("restore: failed", "session", sess.ID, "error", msg)
	sess.mu.Lock()
	sess.Status = "failed"
	sess.EndTime = time.Now()
	sess.Errors = append(sess.Errors, msg)
	sess.mu.Unlock()
}

func parseSnapshotID(idStr string) (backend.ID, error) {
	b, err := hex.DecodeString(idStr)
	if err != nil || len(b) != 32 {
		return backend.ID{}, fmt.Errorf("invalid snapshot ID %q", idStr)
	}
	var id backend.ID
	copy(id[:], b)
	return id, nil
}

func loadRestoreTree(repo *repository.Repository, treeID backend.ID) (*restoreTree, error) {
	blob, packID := repo.LookupBlob(repository.ID(treeID))
	if blob == nil {
		return nil, fmt.Errorf("tree blob %s not found in index", treeID)
	}
	compressedData, err := repo.LoadBlob(*packID, *blob)
	if err != nil {
		return nil, fmt.Errorf("load tree blob %s: %w", treeID, err)
	}
	jsonData, err := compress.Decompress(compressedData)
	if err != nil {
		return nil, fmt.Errorf("decompress tree blob %s: %w", treeID, err)
	}
	var t restoreTree
	if err := json.Unmarshal(jsonData, &t); err != nil {
		return nil, fmt.Errorf("unmarshal tree blob %s: %w", treeID, err)
	}
	return &t, nil
}

// countFiles recursively counts files and bytes in a tree, respecting includes.
func countFiles(repo *repository.Repository, t *restoreTree, files *uint64, bytes *uint64, includes []string, relPrefix string) {
	for _, node := range t.Nodes {
		relPath := node.Name
		if relPrefix != "" {
			relPath = relPrefix + "/" + node.Name
		}
		if !shouldProcess(relPath, includes) {
			continue
		}
		switch node.Type {
		case fs.NodeTypeFile:
			*files++
			*bytes += node.Size
		case fs.NodeTypeDir:
			subtreeID := backend.ID(node.Subtree)
			sub, err := loadRestoreTree(repo, subtreeID)
			if err != nil {
				continue
			}
			countFiles(repo, sub, files, bytes, includes, relPath)
		}
	}
}

const restoreChunkSize = 1024 * 1024 // 1MB

// streamTree recursively walks a tree and streams data to the agent.
func streamTree(repo *repository.Repository, stream pb.AgentService_RestoreDataClient, sess *RestoreSession, t *restoreTree, basePath string, includes []string, relPrefix string) error {
	for _, node := range t.Nodes {
		nodePath := path.Join(basePath, node.Name)
		relPath := node.Name
		if relPrefix != "" {
			relPath = relPrefix + "/" + node.Name
		}

		if !shouldProcess(relPath, includes) {
			continue
		}

		switch node.Type {
		case fs.NodeTypeDir:
			if err := stream.Send(&pb.RestoreDataRequest{
				Payload: &pb.RestoreDataRequest_Dir{Dir: &pb.RestoreDir{
					Path:    nodePath,
					Mode:    node.Mode,
					Uid:     node.UID,
					Gid:     node.GID,
					MtimeNs: node.MtimeNs,
					AtimeNs: node.AtimeNs,
				}},
			}); err != nil {
				return fmt.Errorf("send dir %s: %w", nodePath, err)
			}

			subtreeID := backend.ID(node.Subtree)
			sub, err := loadRestoreTree(repo, subtreeID)
			if err != nil {
				return fmt.Errorf("load subtree %s: %w", nodePath, err)
			}
			if err := streamTree(repo, stream, sess, sub, nodePath, includes, relPath); err != nil {
				return err
			}

		case fs.NodeTypeFile:
			sess.mu.Lock()
			sess.CurrentFile = nodePath
			sess.mu.Unlock()

			if err := stream.Send(&pb.RestoreDataRequest{
				Payload: &pb.RestoreDataRequest_FileHeader{FileHeader: &pb.RestoreFileHeader{
					Path:    nodePath,
					Mode:    node.Mode,
					Uid:     node.UID,
					Gid:     node.GID,
					Size:    node.Size,
					MtimeNs: node.MtimeNs,
					AtimeNs: node.AtimeNs,
				}},
			}); err != nil {
				return fmt.Errorf("send file header %s: %w", nodePath, err)
			}

			// Load and stream file content blobs.
			for _, contentID := range node.Content {
				blobID := backend.ID(contentID)
				blob, packID := repo.LookupBlob(repository.ID(blobID))
				if blob == nil {
					return fmt.Errorf("content blob %s not found", blobID)
				}
				compressedData, err := repo.LoadBlob(*packID, *blob)
				if err != nil {
					return fmt.Errorf("load blob %s: %w", blobID, err)
				}
				data, err := compress.Decompress(compressedData)
				if err != nil {
					return fmt.Errorf("decompress blob %s: %w", blobID, err)
				}

				// Send data in chunks.
				for off := 0; off < len(data); off += restoreChunkSize {
					end := off + restoreChunkSize
					if end > len(data) {
						end = len(data)
					}
					if err := stream.Send(&pb.RestoreDataRequest{
						Payload: &pb.RestoreDataRequest_FileChunk{FileChunk: &pb.RestoreFileChunk{
							Data: data[off:end],
						}},
					}); err != nil {
						return fmt.Errorf("send chunk for %s: %w", nodePath, err)
					}
					sess.BytesWritten.Add(uint64(end - off))
				}
			}
			sess.FilesRestored.Add(1)

		case fs.NodeTypeSymlink:
			if err := stream.Send(&pb.RestoreDataRequest{
				Payload: &pb.RestoreDataRequest_Symlink{Symlink: &pb.RestoreSymlink{
					Path:   nodePath,
					Target: node.LinkTarget,
					Uid:    node.UID,
					Gid:    node.GID,
				}},
			}); err != nil {
				return fmt.Errorf("send symlink %s: %w", nodePath, err)
			}
		}
	}
	return nil
}

// shouldProcess determines whether a node at relPath should be processed
// given the include filter list. Same logic as restorer.shouldProcess.
func shouldProcess(relPath string, includes []string) bool {
	if len(includes) == 0 {
		return true
	}
	for _, inc := range includes {
		if relPath == inc {
			return true
		}
		if strings.HasPrefix(relPath, inc+"/") {
			return true
		}
		if strings.HasPrefix(inc, relPath+"/") {
			return true
		}
	}
	return false
}
