package repository

import (
	"crypto/sha256"
	"testing"
)

// testKey returns a deterministic 32-byte key derived from the given seed string.
func testKey(seed string) [32]byte {
	return sha256.Sum256([]byte(seed))
}

// testID returns a deterministic ID derived from the given seed string.
func testID(seed string) ID {
	return sha256.Sum256([]byte(seed))
}

func TestIndexEncodeDecodeRoundTrip(t *testing.T) {
	key := testKey("test-encryption-key")

	idx := &Index{
		Packs: []IndexPack{
			{
				PackID: testID("pack-1"),
				Blobs: []PackedBlob{
					{
						ID:                 testID("blob-1"),
						Type:               DataBlob,
						Offset:             0,
						Length:             1024,
						UncompressedLength: 2048,
					},
				},
			},
		},
	}

	encoded, err := EncodeIndex(key, idx)
	if err != nil {
		t.Fatalf("EncodeIndex failed: %v", err)
	}

	decoded, err := DecodeIndex(key, encoded)
	if err != nil {
		t.Fatalf("DecodeIndex failed: %v", err)
	}

	if len(decoded.Packs) != len(idx.Packs) {
		t.Fatalf("expected %d packs, got %d", len(idx.Packs), len(decoded.Packs))
	}

	pack := decoded.Packs[0]
	if pack.PackID != idx.Packs[0].PackID {
		t.Errorf("PackID mismatch: got %s, want %s", pack.PackID, idx.Packs[0].PackID)
	}

	if len(pack.Blobs) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(pack.Blobs))
	}

	blob := pack.Blobs[0]
	want := idx.Packs[0].Blobs[0]
	if blob.ID != want.ID {
		t.Errorf("Blob ID mismatch: got %s, want %s", blob.ID, want.ID)
	}
	if blob.Type != want.Type {
		t.Errorf("Blob Type mismatch: got %s, want %s", blob.Type, want.Type)
	}
	if blob.Offset != want.Offset {
		t.Errorf("Blob Offset mismatch: got %d, want %d", blob.Offset, want.Offset)
	}
	if blob.Length != want.Length {
		t.Errorf("Blob Length mismatch: got %d, want %d", blob.Length, want.Length)
	}
	if blob.UncompressedLength != want.UncompressedLength {
		t.Errorf("Blob UncompressedLength mismatch: got %d, want %d", blob.UncompressedLength, want.UncompressedLength)
	}
}

func TestIndexMultiplePacksRoundTrip(t *testing.T) {
	key := testKey("multi-pack-key")

	idx := &Index{
		Packs: []IndexPack{
			{
				PackID: testID("pack-alpha"),
				Blobs: []PackedBlob{
					{
						ID:                 testID("blob-a1"),
						Type:               DataBlob,
						Offset:             0,
						Length:             512,
						UncompressedLength: 1024,
					},
					{
						ID:                 testID("blob-a2"),
						Type:               TreeBlob,
						Offset:             512,
						Length:             256,
						UncompressedLength: 400,
					},
				},
			},
			{
				PackID: testID("pack-beta"),
				Blobs: []PackedBlob{
					{
						ID:                 testID("blob-b1"),
						Type:               DataBlob,
						Offset:             0,
						Length:             2048,
						UncompressedLength: 4096,
					},
					{
						ID:                 testID("blob-b2"),
						Type:               DataBlob,
						Offset:             2048,
						Length:             128,
						UncompressedLength: 128,
					},
					{
						ID:                 testID("blob-b3"),
						Type:               TreeBlob,
						Offset:             2176,
						Length:             64,
						UncompressedLength: 100,
					},
				},
			},
		},
	}

	encoded, err := EncodeIndex(key, idx)
	if err != nil {
		t.Fatalf("EncodeIndex failed: %v", err)
	}

	decoded, err := DecodeIndex(key, encoded)
	if err != nil {
		t.Fatalf("DecodeIndex failed: %v", err)
	}

	if len(decoded.Packs) != 2 {
		t.Fatalf("expected 2 packs, got %d", len(decoded.Packs))
	}

	// Verify pack 0
	if decoded.Packs[0].PackID != idx.Packs[0].PackID {
		t.Errorf("Pack[0] PackID mismatch")
	}
	if len(decoded.Packs[0].Blobs) != 2 {
		t.Fatalf("Pack[0]: expected 2 blobs, got %d", len(decoded.Packs[0].Blobs))
	}

	// Verify pack 1
	if decoded.Packs[1].PackID != idx.Packs[1].PackID {
		t.Errorf("Pack[1] PackID mismatch")
	}
	if len(decoded.Packs[1].Blobs) != 3 {
		t.Fatalf("Pack[1]: expected 3 blobs, got %d", len(decoded.Packs[1].Blobs))
	}

	// Spot-check specific blob fields across packs
	for pi, pack := range decoded.Packs {
		for bi, blob := range pack.Blobs {
			want := idx.Packs[pi].Blobs[bi]
			if blob.ID != want.ID {
				t.Errorf("Pack[%d].Blob[%d] ID mismatch", pi, bi)
			}
			if blob.Type != want.Type {
				t.Errorf("Pack[%d].Blob[%d] Type mismatch: got %s, want %s", pi, bi, blob.Type, want.Type)
			}
			if blob.Offset != want.Offset {
				t.Errorf("Pack[%d].Blob[%d] Offset mismatch: got %d, want %d", pi, bi, blob.Offset, want.Offset)
			}
			if blob.Length != want.Length {
				t.Errorf("Pack[%d].Blob[%d] Length mismatch: got %d, want %d", pi, bi, blob.Length, want.Length)
			}
			if blob.UncompressedLength != want.UncompressedLength {
				t.Errorf("Pack[%d].Blob[%d] UncompressedLength mismatch: got %d, want %d", pi, bi, blob.UncompressedLength, want.UncompressedLength)
			}
		}
	}
}

func TestIndexDecryptWithWrongKeyFails(t *testing.T) {
	correctKey := testKey("correct-key")
	wrongKey := testKey("wrong-key")

	idx := &Index{
		Packs: []IndexPack{
			{
				PackID: testID("pack-1"),
				Blobs: []PackedBlob{
					{
						ID:                 testID("blob-1"),
						Type:               DataBlob,
						Offset:             0,
						Length:             100,
						UncompressedLength: 200,
					},
				},
			},
		},
	}

	encoded, err := EncodeIndex(correctKey, idx)
	if err != nil {
		t.Fatalf("EncodeIndex failed: %v", err)
	}

	_, err = DecodeIndex(wrongKey, encoded)
	if err == nil {
		t.Fatal("DecodeIndex with wrong key should have failed, but succeeded")
	}
}
