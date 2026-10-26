package archiver

import (
	"encoding/json"
	"testing"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/crypto"
	"github.com/hooperits/hitstor/internal/fs"
)

// testIDKey is a fixed HMAC key used across all tests for determinism.
var testIDKey = [32]byte{
	0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
	0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
	0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
}

// makeFileNode creates a file node with the given name and optional content IDs.
func makeFileNode(name string, contentIDs ...fs.ID) fs.Node {
	return fs.Node{
		Name:    name,
		Type:    fs.NodeTypeFile,
		Mode:    0644,
		UID:     1000,
		GID:     1000,
		MtimeNs: 1700000000000000000,
		AtimeNs: 1700000000000000000,
		CtimeNs: 1700000000000000000,
		Size:    1024,
		Inode:   100,
		Links:   1,
		Content: contentIDs,
	}
}

// makeDirNode creates a directory node with the given name and subtree ID.
func makeDirNode(name string, subtree fs.ID) fs.Node {
	return fs.Node{
		Name:    name,
		Type:    fs.NodeTypeDir,
		Mode:    0755,
		UID:     1000,
		GID:     1000,
		MtimeNs: 1700000000000000000,
		AtimeNs: 1700000000000000000,
		CtimeNs: 1700000000000000000,
		Inode:   200,
		Links:   2,
		Subtree: subtree,
	}
}

// makeSymlinkNode creates a symlink node with the given name and target.
func makeSymlinkNode(name, target string) fs.Node {
	return fs.Node{
		Name:       name,
		Type:       fs.NodeTypeSymlink,
		Mode:       0777,
		UID:        1000,
		GID:        1000,
		MtimeNs:    1700000000000000000,
		AtimeNs:    1700000000000000000,
		CtimeNs:    1700000000000000000,
		Inode:      300,
		Links:      1,
		LinkTarget: target,
	}
}

func TestTreeBuilder(t *testing.T) {
	// Define some content IDs for file nodes.
	chunkID1 := fs.ID{0xaa, 0xbb, 0xcc}
	chunkID2 := fs.ID{0xdd, 0xee, 0xff}

	// Define a subtree ID for directory nodes.
	subtreeID := fs.ID{0x11, 0x22, 0x33, 0x44, 0x55}

	tests := []struct {
		name  string
		nodes []fs.Node
	}{
		{
			name: "round-trip: mixed nodes serialize and deserialize correctly",
			nodes: []fs.Node{
				makeFileNode("readme.txt"),
				makeDirNode("subdir", subtreeID),
				makeSymlinkNode("link", "/tmp/target"),
			},
		},
		{
			name: "file nodes with Content blob IDs are preserved",
			nodes: []fs.Node{
				makeFileNode("data.bin", chunkID1, chunkID2),
				makeFileNode("config.yaml", chunkID1),
			},
		},
		{
			name: "directory nodes with Subtree blob IDs are preserved",
			nodes: []fs.Node{
				makeDirNode("photos", subtreeID),
				makeDirNode("docs", fs.ID{0x99, 0x88, 0x77}),
			},
		},
		{
			name:  "empty tree produces valid blob",
			nodes: []fs.Node{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tb := NewTreeBuilder(testIDKey)
			for _, n := range tc.nodes {
				tb.AddNode(n)
			}

			blobID, compressedData, uncompressedLen, err := tb.Finalize()
			if err != nil {
				t.Fatalf("Finalize() returned error: %v", err)
			}

			// Verify blobID is not zero.
			if blobID == (backend.ID{}) {
				t.Fatal("Finalize() returned zero blob ID")
			}

			// Verify compressed data is non-nil.
			if compressedData == nil {
				t.Fatal("Finalize() returned nil compressedData")
			}

			// Verify uncompressed length is positive (even empty tree has JSON bytes).
			if uncompressedLen <= 0 {
				t.Fatalf("Finalize() returned non-positive uncompressedLen: %d", uncompressedLen)
			}

			// Decompress and unmarshal to verify round-trip.
			decompressed, err := compress.Decompress(compressedData)
			if err != nil {
				t.Fatalf("Decompress() returned error: %v", err)
			}

			if len(decompressed) != uncompressedLen {
				t.Fatalf("decompressed length %d != uncompressedLen %d", len(decompressed), uncompressedLen)
			}

			var tree Tree
			if err := json.Unmarshal(decompressed, &tree); err != nil {
				t.Fatalf("json.Unmarshal() returned error: %v", err)
			}

			if len(tree.Nodes) != len(tc.nodes) {
				t.Fatalf("got %d nodes, want %d", len(tree.Nodes), len(tc.nodes))
			}

			// Build a map of expected nodes by name for order-independent comparison
			// (Finalize sorts by name).
			expectedByName := make(map[string]fs.Node)
			for _, n := range tc.nodes {
				expectedByName[n.Name] = n
			}

			for _, got := range tree.Nodes {
				expected, ok := expectedByName[got.Name]
				if !ok {
					t.Errorf("unexpected node %q in tree", got.Name)
					continue
				}

				if got.Type != expected.Type {
					t.Errorf("node %q: Type = %q, want %q", got.Name, got.Type, expected.Type)
				}
				if got.Mode != expected.Mode {
					t.Errorf("node %q: Mode = %o, want %o", got.Name, got.Mode, expected.Mode)
				}
				if got.UID != expected.UID {
					t.Errorf("node %q: UID = %d, want %d", got.Name, got.UID, expected.UID)
				}
				if got.GID != expected.GID {
					t.Errorf("node %q: GID = %d, want %d", got.Name, got.GID, expected.GID)
				}
				if got.Size != expected.Size {
					t.Errorf("node %q: Size = %d, want %d", got.Name, got.Size, expected.Size)
				}
				if got.LinkTarget != expected.LinkTarget {
					t.Errorf("node %q: LinkTarget = %q, want %q", got.Name, got.LinkTarget, expected.LinkTarget)
				}

				// Verify Content IDs are preserved.
				if len(got.Content) != len(expected.Content) {
					t.Errorf("node %q: len(Content) = %d, want %d", got.Name, len(got.Content), len(expected.Content))
				} else {
					for i := range got.Content {
						if got.Content[i] != expected.Content[i] {
							t.Errorf("node %q: Content[%d] = %x, want %x", got.Name, i, got.Content[i], expected.Content[i])
						}
					}
				}

				// Verify Subtree ID is preserved.
				if got.Subtree != expected.Subtree {
					t.Errorf("node %q: Subtree = %x, want %x", got.Name, got.Subtree, expected.Subtree)
				}
			}

			// Verify blob ID matches HMAC-SHA256 of the JSON bytes.
			expectedRawID := crypto.BlobID(testIDKey, decompressed)
			expectedBlobID := backend.ID(expectedRawID)
			if blobID != expectedBlobID {
				t.Errorf("blobID = %x, want %x", blobID, expectedBlobID)
			}
		})
	}
}

func TestTreeBuilderDeterministic(t *testing.T) {
	chunkID := fs.ID{0xaa, 0xbb, 0xcc}
	subtreeID := fs.ID{0x11, 0x22, 0x33}

	nodes := []fs.Node{
		makeFileNode("file.txt", chunkID),
		makeDirNode("subdir", subtreeID),
		makeSymlinkNode("link", "/tmp/target"),
	}

	// Build the tree twice with the same nodes.
	tb1 := NewTreeBuilder(testIDKey)
	tb2 := NewTreeBuilder(testIDKey)
	for _, n := range nodes {
		tb1.AddNode(n)
		tb2.AddNode(n)
	}

	id1, data1, len1, err1 := tb1.Finalize()
	if err1 != nil {
		t.Fatalf("first Finalize() returned error: %v", err1)
	}

	id2, data2, len2, err2 := tb2.Finalize()
	if err2 != nil {
		t.Fatalf("second Finalize() returned error: %v", err2)
	}

	if id1 != id2 {
		t.Errorf("blob IDs differ: %x vs %x", id1, id2)
	}

	if len1 != len2 {
		t.Errorf("uncompressed lengths differ: %d vs %d", len1, len2)
	}

	// Compressed data should also be identical (zstd is deterministic for same input).
	if len(data1) != len(data2) {
		t.Errorf("compressed data lengths differ: %d vs %d", len(data1), len(data2))
	}
	for i := range data1 {
		if data1[i] != data2[i] {
			t.Errorf("compressed data differs at byte %d", i)
			break
		}
	}
}

func TestTreeBuilderDeterministicNodeOrder(t *testing.T) {
	// Nodes added in different order should produce the same blob
	// because Finalize sorts by name.
	chunkID := fs.ID{0xaa, 0xbb, 0xcc}
	subtreeID := fs.ID{0x11, 0x22, 0x33}

	nodesOrder1 := []fs.Node{
		makeDirNode("zdir", subtreeID),
		makeFileNode("afile.txt", chunkID),
		makeSymlinkNode("mlink", "/tmp/target"),
	}

	nodesOrder2 := []fs.Node{
		makeSymlinkNode("mlink", "/tmp/target"),
		makeFileNode("afile.txt", chunkID),
		makeDirNode("zdir", subtreeID),
	}

	tb1 := NewTreeBuilder(testIDKey)
	for _, n := range nodesOrder1 {
		tb1.AddNode(n)
	}

	tb2 := NewTreeBuilder(testIDKey)
	for _, n := range nodesOrder2 {
		tb2.AddNode(n)
	}

	id1, _, _, err1 := tb1.Finalize()
	if err1 != nil {
		t.Fatalf("first Finalize() returned error: %v", err1)
	}

	id2, _, _, err2 := tb2.Finalize()
	if err2 != nil {
		t.Fatalf("second Finalize() returned error: %v", err2)
	}

	if id1 != id2 {
		t.Errorf("blob IDs differ for different node insertion order: %x vs %x", id1, id2)
	}
}

func TestTreeBuilderDifferentNodesProduceDifferentIDs(t *testing.T) {
	tests := []struct {
		name   string
		nodesA []fs.Node
		nodesB []fs.Node
	}{
		{
			name:   "different file names",
			nodesA: []fs.Node{makeFileNode("alpha.txt")},
			nodesB: []fs.Node{makeFileNode("beta.txt")},
		},
		{
			name:   "different number of nodes",
			nodesA: []fs.Node{makeFileNode("file1.txt")},
			nodesB: []fs.Node{makeFileNode("file1.txt"), makeFileNode("file2.txt")},
		},
		{
			name:   "different content IDs",
			nodesA: []fs.Node{makeFileNode("data.bin", fs.ID{0x01})},
			nodesB: []fs.Node{makeFileNode("data.bin", fs.ID{0x02})},
		},
		{
			name:   "different subtree IDs",
			nodesA: []fs.Node{makeDirNode("sub", fs.ID{0x01})},
			nodesB: []fs.Node{makeDirNode("sub", fs.ID{0x02})},
		},
		{
			name:   "file vs directory same name",
			nodesA: []fs.Node{makeFileNode("entry")},
			nodesB: []fs.Node{makeDirNode("entry", fs.ID{})},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tbA := NewTreeBuilder(testIDKey)
			for _, n := range tc.nodesA {
				tbA.AddNode(n)
			}

			tbB := NewTreeBuilder(testIDKey)
			for _, n := range tc.nodesB {
				tbB.AddNode(n)
			}

			idA, _, _, errA := tbA.Finalize()
			if errA != nil {
				t.Fatalf("Finalize() A returned error: %v", errA)
			}

			idB, _, _, errB := tbB.Finalize()
			if errB != nil {
				t.Fatalf("Finalize() B returned error: %v", errB)
			}

			if idA == idB {
				t.Errorf("blob IDs should differ but both are %x", idA)
			}
		})
	}
}
