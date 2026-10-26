package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/archiver"
	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/fs"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/repository"
)

const packSizeThreshold = 128 * 1024 * 1024 // 128 MiB

// Session holds per-agent backup state for one active session.
type Session struct {
	ID               string
	AgentName        string
	Hostname         string
	Paths            []string
	Excludes         []string
	Tags             []string
	RepoName         string
	JobID            string // BackupJob ID (empty for legacy agents)
	ParentSnapshotID string // Hex snapshot ID of previous snapshot (incremental)
	BackupType       string // "full" or "incremental"

	repo       *repository.Repository
	repoLocked bool
	pw         *repository.PackWriter
	index      *repository.Index

	mu     sync.Mutex
	known  map[[32]byte]bool // blob IDs seen in this session
	newIDs map[[32]byte]bool // blob IDs newly added via AddChunk

	bytesAdded      uint64
	filesProcessed  uint64 // total blobs processed via AddChunk
	filesNew        uint64 // new blobs not seen before
	suspiciousFiles uint64
	startTime       time.Time
	lastActivity    time.Time
	phase           string // "uploading" or "finalizing"
}

// NewSession creates a new backup session for an agent.
func NewSession(repo *repository.Repository, agentName, hostname string, paths, excludes, tags []string) *Session {
	var idBytes [16]byte
	rand.Read(idBytes[:])

	now := time.Now()
	return &Session{
		ID:           hex.EncodeToString(idBytes[:]),
		AgentName:    agentName,
		Hostname:     hostname,
		Paths:        paths,
		Excludes:     excludes,
		Tags:         tags,
		repo:         repo,
		pw:           repository.NewPackWriter(repo.Key().EncryptKey),
		index:        &repository.Index{},
		known:        make(map[[32]byte]bool),
		newIDs:       make(map[[32]byte]bool),
		startTime:    now,
		lastActivity: now,
		phase:        "uploading",
	}
}

// IsKnown returns true if the blob ID was already seen in this session.
func (s *Session) IsKnown(blobID [32]byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.known[blobID]
}

// MarkKnown records a blob ID as already present (exists in repo index).
func (s *Session) MarkKnown(blobID [32]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.known[blobID] = true
}

// AddChunk encrypts a compressed chunk and adds it to the current pack.
// Flushes the pack at the 128 MiB threshold.
func (s *Session) AddChunk(blobID [32]byte, compressedData []byte, uncompressedLen int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.pw.AddBlob(backend.ID(blobID), repository.DataBlob, compressedData, uncompressedLen); err != nil {
		return fmt.Errorf("session: add blob: %w", err)
	}

	s.known[blobID] = true
	s.newIDs[blobID] = true
	s.bytesAdded += uint64(len(compressedData))
	s.filesProcessed++
	s.filesNew++
	s.lastActivity = time.Now()

	if s.pw.Size() >= packSizeThreshold {
		return s.flushPack()
	}
	return nil
}

// Progress returns the current session progress counters.
func (s *Session) Progress() (bytesAdded, filesProcessed, filesNew uint64, phase string, lastActivity time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bytesAdded, s.filesProcessed, s.filesNew, s.phase, s.lastActivity
}

// SetPhase updates the session phase (e.g., "uploading" -> "finalizing").
func (s *Session) SetPhase(phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = phase
}

// Finalize builds tree blobs from the agent's file metadata, creates a
// snapshot, saves the index, and returns the finalize response.
func (s *Session) Finalize(files []*pb.FileNode, systemProfile []byte) (*pb.FinalizeBackupResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build tree blobs from the flat file list.
	rootTreeID, stats, err := s.buildTrees(files)
	if err != nil {
		return nil, fmt.Errorf("session: build trees: %w", err)
	}

	// Store the agent's layout before flushing so its blob lands in a pack.
	// A profile that cannot be stored must not cost the backup.
	systemProfileID, err := s.saveSystemProfile(systemProfile)
	if err != nil {
		slog.Warn("session: system profile not stored", "session", s.ID, "error", err)
	}

	// Flush remaining pack data.
	if err := s.flushPack(); err != nil {
		return nil, fmt.Errorf("session: flush final pack: %w", err)
	}

	// Save the session index.
	if len(s.index.Packs) > 0 {
		if _, err := s.repo.SaveIndex(s.index); err != nil {
			return nil, fmt.Errorf("session: save index: %w", err)
		}
	}

	duration := time.Since(s.startTime).Seconds()

	s.suspiciousFiles = stats.suspiciousFiles

	// Create and save the snapshot.
	snap := &repository.Snapshot{
		Time:           time.Now(),
		Hostname:       s.Hostname,
		Paths:          s.Paths,
		Excludes:       s.Excludes,
		Tags:           s.Tags,
		Tree:           rootTreeID,
		Type:           s.BackupType,
		ParentSnapshot: s.ParentSnapshotID,
		SystemProfile:  systemProfileID,
		Summary: repository.SnapshotSummary{
			FilesNew:        stats.filesNew,
			FilesUnchanged:  stats.filesUnchanged,
			BytesAdded:      s.bytesAdded,
			BytesProcessed:  stats.bytesProcessed,
			TotalFiles:      stats.totalFiles,
			TotalSize:       stats.bytesProcessed,
			TotalDuration:   duration,
			SuspiciousFiles: stats.suspiciousFiles,
		},
	}

	snapID, err := s.repo.SaveSnapshot(snap)
	if err != nil {
		return nil, fmt.Errorf("session: save snapshot: %w", err)
	}

	if err := s.repo.MaybeConsolidateIndexes(); err != nil {
		return nil, fmt.Errorf("session: consolidate indexes: %w", err)
	}

	return &pb.FinalizeBackupResponse{
		SnapshotId:      hex.EncodeToString(snapID[:8]),
		FilesNew:        stats.filesNew,
		FilesUnchanged:  stats.filesUnchanged,
		BytesAdded:      s.bytesAdded,
		BytesProcessed:  stats.bytesProcessed,
		DurationSeconds: duration,
	}, nil
}

// treeStats tracks statistics during tree building.
type treeStats struct {
	filesNew        uint64
	filesUnchanged  uint64
	totalFiles      uint64
	bytesProcessed  uint64
	suspiciousFiles uint64
}

func isSuspiciousExtension(filePath string) bool {
	ext := strings.ToLower(path.Ext(filePath))
	switch ext {
	case ".locked", ".crypto", ".crypt", ".cryptolocker", ".wannacry", ".ryuk", ".locky", ".encrypted", ".lock", ".ransom":
		return true
	}
	return false
}

// buildTrees converts a flat list of FileNodes into a hierarchical tree of
// tree blobs. Returns the root tree blob ID and file statistics.
func (s *Session) buildTrees(files []*pb.FileNode) (backend.ID, treeStats, error) {
	idKey := s.repo.Key().IDKey
	var stats treeStats

	// Organize files by parent directory.
	dirChildren := make(map[string][]*pb.FileNode) // parent path -> children
	dirs := make(map[string]bool)                  // all directory paths

	// Ensure root directory exists.
	dirs[""] = true

	for _, f := range files {
		// Normalize Windows backslash paths to forward slashes so that
		// path.Dir/path.Base work correctly regardless of agent platform.
		f.Path = strings.ReplaceAll(f.Path, "\\", "/")
		p := strings.TrimSuffix(f.Path, "/")
		parent := path.Dir(p)
		if parent == "." {
			parent = ""
		}

		if f.IsDir {
			dirs[p] = true
			dirChildren[parent] = append(dirChildren[parent], f)
		} else {
			dirChildren[parent] = append(dirChildren[parent], f)
			stats.totalFiles++
			stats.bytesProcessed += uint64(f.Size)

			// Check if file has any new blobs.
			isNew := false
			for _, idBytes := range f.ContentIds {
				if len(idBytes) == 32 {
					var bid [32]byte
					copy(bid[:], idBytes)
					if s.newIDs[bid] {
						isNew = true
						break
					}
				}
			}
			if isNew {
				stats.filesNew++
				if isSuspiciousExtension(f.Path) {
					stats.suspiciousFiles++
				}
			} else {
				stats.filesUnchanged++
			}
		}
	}

	// Ensure all intermediate directories exist.
	for dir := range dirs {
		for d := dir; d != "" && d != "."; d = path.Dir(d) {
			if d == "." {
				break
			}
			if !dirs[path.Dir(d)] && path.Dir(d) != "." {
				dirs[path.Dir(d)] = true
			}
		}
	}

	// Sort directories by depth (deepest first) for bottom-up processing.
	dirList := make([]string, 0, len(dirs))
	for d := range dirs {
		dirList = append(dirList, d)
	}
	sort.Slice(dirList, func(i, j int) bool {
		di := strings.Count(dirList[i], "/")
		dj := strings.Count(dirList[j], "/")
		if dirList[i] == "" {
			di = -1
		}
		if dirList[j] == "" {
			dj = -1
		}
		return di > dj // deepest first
	})

	// Build tree blobs bottom-up.
	treeBlobIDs := make(map[string]backend.ID) // dir path -> tree blob ID

	for _, dirPath := range dirList {
		tb := archiver.NewTreeBuilder(idKey)

		children := dirChildren[dirPath]
		for _, child := range children {
			childName := path.Base(child.Path)
			if child.IsDir {
				childPath := strings.TrimSuffix(child.Path, "/")
				subtreeID, ok := treeBlobIDs[childPath]
				if !ok {
					// Empty directory — build an empty tree.
					emptyTB := archiver.NewTreeBuilder(idKey)
					emptyID, emptyData, emptyLen, err := emptyTB.Finalize()
					if err != nil {
						return backend.ID{}, stats, fmt.Errorf("build empty tree for %q: %w", childPath, err)
					}
					if err := s.pw.AddBlob(emptyID, repository.TreeBlob, emptyData, emptyLen); err != nil {
						return backend.ID{}, stats, fmt.Errorf("add empty tree blob: %w", err)
					}
					s.known[[32]byte(emptyID)] = true
					subtreeID = emptyID
					treeBlobIDs[childPath] = subtreeID
				}

				tb.AddNode(fs.Node{
					Name:    childName,
					Type:    fs.NodeTypeDir,
					Mode:    child.Mode,
					UID:     child.Uid,
					GID:     child.Gid,
					MtimeNs: child.MtimeNs,
					Subtree: fs.ID(subtreeID),
					Xattrs:  child.Xattrs,
				})
			} else if child.LinkTarget != "" {
				tb.AddNode(fs.Node{
					Name:       childName,
					Type:       fs.NodeTypeSymlink,
					Mode:       child.Mode,
					UID:        child.Uid,
					GID:        child.Gid,
					MtimeNs:    child.MtimeNs,
					LinkTarget: child.LinkTarget,
					Xattrs:     child.Xattrs,
				})
			} else {
				// Regular file.
				contentIDs := make([]fs.ID, 0, len(child.ContentIds))
				for _, idBytes := range child.ContentIds {
					if len(idBytes) == 32 {
						var cid fs.ID
						copy(cid[:], idBytes)
						contentIDs = append(contentIDs, cid)
					}
				}

				tb.AddNode(fs.Node{
					Name:    childName,
					Type:    fs.NodeTypeFile,
					Mode:    child.Mode,
					UID:     child.Uid,
					GID:     child.Gid,
					MtimeNs: child.MtimeNs,
					Size:    uint64(child.Size),
					Content: contentIDs,
					Xattrs:  child.Xattrs,
				})
			}
		}

		blobID, compressedData, uncompressedLen, err := tb.Finalize()
		if err != nil {
			return backend.ID{}, stats, fmt.Errorf("finalize tree %q: %w", dirPath, err)
		}

		if err := s.pw.AddBlob(blobID, repository.TreeBlob, compressedData, uncompressedLen); err != nil {
			return backend.ID{}, stats, fmt.Errorf("add tree blob %q: %w", dirPath, err)
		}
		s.known[[32]byte(blobID)] = true
		treeBlobIDs[dirPath] = blobID

		// Flush pack if needed.
		if s.pw.Size() >= packSizeThreshold {
			if err := s.flushPack(); err != nil {
				return backend.ID{}, stats, fmt.Errorf("flush pack during tree build: %w", err)
			}
		}
	}

	rootID, ok := treeBlobIDs[""]
	if !ok {
		return backend.ID{}, stats, fmt.Errorf("no root tree built")
	}

	return rootID, stats, nil
}

// extractPreviousFiles walks a snapshot's tree blobs recursively and extracts
// file metadata (path, mtime_ns, size, content_ids) for incremental detection.
func extractPreviousFiles(repo *repository.Repository, treeID backend.ID, prefix string) ([]*pb.PreviousFileEntry, error) {
	blob, packID := repo.LookupBlob(treeID)
	if blob == nil {
		return nil, fmt.Errorf("session: tree blob %s not found in index", treeID)
	}

	compressedData, err := repo.LoadBlob(*packID, *blob)
	if err != nil {
		return nil, fmt.Errorf("session: load tree blob %s: %w", treeID, err)
	}

	jsonData, err := compress.Decompress(compressedData)
	if err != nil {
		return nil, fmt.Errorf("session: decompress tree blob %s: %w", treeID, err)
	}

	var t struct {
		Nodes []fs.Node `json:"nodes"`
	}
	if err := json.Unmarshal(jsonData, &t); err != nil {
		return nil, fmt.Errorf("session: unmarshal tree blob %s: %w", treeID, err)
	}

	var entries []*pb.PreviousFileEntry
	for _, node := range t.Nodes {
		fullPath := node.Name
		if prefix != "" {
			fullPath = prefix + "/" + node.Name
		}

		switch node.Type {
		case fs.NodeTypeFile:
			contentIDs := make([][]byte, len(node.Content))
			for i, cid := range node.Content {
				b := make([]byte, 32)
				copy(b, cid[:])
				contentIDs[i] = b
			}
			entries = append(entries, &pb.PreviousFileEntry{
				Path:       fullPath,
				MtimeNs:    node.MtimeNs,
				Size:       int64(node.Size),
				ContentIds: contentIDs,
			})
		case fs.NodeTypeDir:
			if node.Subtree.IsZero() {
				continue
			}
			sub, err := extractPreviousFiles(repo, backend.ID(node.Subtree), fullPath)
			if err != nil {
				return nil, err
			}
			entries = append(entries, sub...)
		}
	}

	return entries, nil
}

// flushPack finalizes the current pack, saves it, and records it in the index.
func (s *Session) flushPack() error {
	if s.pw == nil || s.pw.Count() == 0 {
		return nil
	}

	packID, data, err := s.pw.Finalize()
	if err != nil {
		return fmt.Errorf("session: finalize pack: %w", err)
	}

	if _, err := s.repo.SavePack(data); err != nil {
		return fmt.Errorf("session: save pack: %w", err)
	}

	blobs, err := repository.ReadPackHeader(s.repo.Key().EncryptKey, data)
	if err != nil {
		return fmt.Errorf("session: read pack header: %w", err)
	}

	s.index.Packs = append(s.index.Packs, repository.IndexPack{
		PackID: packID,
		Blobs:  blobs,
	})

	// Start a new pack writer.
	s.pw = repository.NewPackWriter(s.repo.Key().EncryptKey)
	return nil
}

// saveSystemProfile stores the agent-supplied host layout as a data blob and
// returns the blob ID to record in the snapshot.
//
// The agent sends the profile already encoded as JSON, so the server records it
// verbatim rather than re-deriving it — the layout describes the agent's host,
// not the server's. Returns the zero ID when no profile was sent.
func (s *Session) saveSystemProfile(profileJSON []byte) (repository.ID, error) {
	if len(profileJSON) == 0 {
		return repository.ID{}, nil
	}

	// json.RawMessage keeps the agent's bytes intact through re-marshalling.
	blobID, compressedData, uncompressedLen, err := repository.EncodeSystemProfile(
		s.repo.Key().IDKey, json.RawMessage(profileJSON))
	if err != nil {
		return repository.ID{}, err
	}

	if s.known[[32]byte(blobID)] {
		return blobID, nil
	}
	if existing, _ := s.repo.LookupBlob(blobID); existing != nil {
		s.known[[32]byte(blobID)] = true
		return blobID, nil
	}

	if err := s.pw.AddBlob(blobID, repository.DataBlob, compressedData, uncompressedLen); err != nil {
		return repository.ID{}, fmt.Errorf("session: add system profile blob: %w", err)
	}
	s.known[[32]byte(blobID)] = true
	return blobID, nil
}
