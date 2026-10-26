// Package archiver builds tree blobs from filesystem nodes for the HitStor
// encrypted backup tool.
package archiver

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/crypto"
	"github.com/hooperits/hitstor/internal/fs"
)

// Tree represents a directory's contents as a list of Nodes.
type Tree struct {
	Nodes []fs.Node `json:"nodes"`
}

// TreeBuilder accumulates nodes for a directory and produces a tree blob.
type TreeBuilder struct {
	nodes []fs.Node
	idKey [32]byte
}

// NewTreeBuilder creates a new TreeBuilder with the given HMAC key for blob ID
// computation.
func NewTreeBuilder(idKey [32]byte) *TreeBuilder {
	return &TreeBuilder{
		idKey: idKey,
	}
}

// AddNode adds a filesystem node to the tree. For file nodes, their Content
// field should already contain the chunk blob IDs. For directory nodes, their
// Subtree field should already contain the subtree blob ID.
func (tb *TreeBuilder) AddNode(node fs.Node) {
	tb.nodes = append(tb.nodes, node)
}

// Finalize serializes the tree nodes to JSON, computes the blob ID as
// HMAC-SHA256(idKey, jsonBytes), compresses with zstd, and returns:
//   - blobID: the tree blob's content-addressable ID
//   - compressedData: zstd-compressed JSON (ready for encryption)
//   - uncompressedLen: length of the original JSON bytes
func (tb *TreeBuilder) Finalize() (blobID backend.ID, compressedData []byte, uncompressedLen int, err error) {
	// Sort nodes by name for deterministic output.
	sorted := make([]fs.Node, len(tb.nodes))
	copy(sorted, tb.nodes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	tree := Tree{Nodes: sorted}

	jsonBytes, err := json.Marshal(tree)
	if err != nil {
		return backend.ID{}, nil, 0, fmt.Errorf("archiver: marshal tree: %w", err)
	}

	// Compute blob ID as HMAC-SHA256(idKey, jsonBytes).
	rawID := crypto.BlobID(tb.idKey, jsonBytes)
	blobID = backend.ID(rawID)

	compressedData, err = compress.Compress(jsonBytes)
	if err != nil {
		return backend.ID{}, nil, 0, fmt.Errorf("archiver: compress tree: %w", err)
	}

	return blobID, compressedData, len(jsonBytes), nil
}
