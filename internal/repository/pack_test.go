package repository

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/hooperits/hitstor/internal/crypto"
)

func TestPackWriterAddAndFinalize(t *testing.T) {
	key := testKey("test-encryption-key")
	pw := NewPackWriter(key)

	blob1Data := []byte("hello world")
	blob2Data := []byte("second blob data with more content")
	blob3Data := []byte("tree blob content here")

	if err := pw.AddBlob(testID("blob1"), DataBlob, blob1Data, len(blob1Data)); err != nil {
		t.Fatalf("AddBlob blob1: %v", err)
	}
	if err := pw.AddBlob(testID("blob2"), DataBlob, blob2Data, len(blob2Data)); err != nil {
		t.Fatalf("AddBlob blob2: %v", err)
	}
	if err := pw.AddBlob(testID("blob3"), TreeBlob, blob3Data, len(blob3Data)); err != nil {
		t.Fatalf("AddBlob blob3: %v", err)
	}

	if pw.Count() != 3 {
		t.Fatalf("expected Count()=3, got %d", pw.Count())
	}
	if pw.Size() == 0 {
		t.Fatal("expected Size() > 0 after adding blobs")
	}

	packID, data, err := pw.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("expected non-empty pack data")
	}

	// Verify pack ID is the SHA-256 of the complete data.
	expectedID := sha256.Sum256(data)
	if packID != expectedID {
		t.Fatalf("pack ID mismatch: got %x, want %x", packID, expectedID)
	}
}

func TestPackReaderReadBack(t *testing.T) {
	key := testKey("test-encryption-key")
	pw := NewPackWriter(key)

	testBlobs := []struct {
		id       ID
		blobType BlobType
		data     []byte
	}{
		{testID("data1"), DataBlob, []byte("first data blob")},
		{testID("data2"), DataBlob, []byte("second data blob with longer content")},
		{testID("tree1"), TreeBlob, []byte("tree blob content")},
	}

	for _, tb := range testBlobs {
		if err := pw.AddBlob(tb.id, tb.blobType, tb.data, len(tb.data)); err != nil {
			t.Fatalf("AddBlob %x: %v", tb.id[:4], err)
		}
	}

	_, data, err := pw.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Read header back.
	blobs, err := ReadPackHeader(key, data)
	if err != nil {
		t.Fatalf("ReadPackHeader: %v", err)
	}

	if len(blobs) != len(testBlobs) {
		t.Fatalf("expected %d blobs in header, got %d", len(testBlobs), len(blobs))
	}

	// Read each blob and verify its content.
	for i, blob := range blobs {
		plaintext, err := ReadPackBlob(key, data, blob)
		if err != nil {
			t.Fatalf("ReadPackBlob[%d]: %v", i, err)
		}

		if string(plaintext) != string(testBlobs[i].data) {
			t.Errorf("blob[%d] data mismatch: got %q, want %q", i, plaintext, testBlobs[i].data)
		}

		if blob.ID != testBlobs[i].id {
			t.Errorf("blob[%d] ID mismatch: got %x, want %x", i, blob.ID, testBlobs[i].id)
		}

		if blob.Type != testBlobs[i].blobType {
			t.Errorf("blob[%d] Type mismatch: got %q, want %q", i, blob.Type, testBlobs[i].blobType)
		}

		if blob.UncompressedLength != len(testBlobs[i].data) {
			t.Errorf("blob[%d] UncompressedLength mismatch: got %d, want %d",
				i, blob.UncompressedLength, len(testBlobs[i].data))
		}
	}
}

func TestPackHeaderEncryptionDecryption(t *testing.T) {
	key := testKey("pack-header-key")
	pw := NewPackWriter(key)

	if err := pw.AddBlob(testID("b1"), DataBlob, []byte("data1"), 5); err != nil {
		t.Fatalf("AddBlob: %v", err)
	}
	if err := pw.AddBlob(testID("b2"), TreeBlob, []byte("data2"), 5); err != nil {
		t.Fatalf("AddBlob: %v", err)
	}

	_, data, err := pw.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Decrypt header with correct key should succeed.
	blobs, err := ReadPackHeader(key, data)
	if err != nil {
		t.Fatalf("ReadPackHeader with correct key: %v", err)
	}
	if len(blobs) != 2 {
		t.Fatalf("expected 2 blobs, got %d", len(blobs))
	}

	// Decrypt header with wrong key should fail.
	wrongKey := testKey("wrong-key")
	_, err = ReadPackHeader(wrongKey, data)
	if err == nil {
		t.Fatal("expected error when decrypting header with wrong key")
	}
}

func TestPackHeaderLengthMatchesEncryptedSize(t *testing.T) {
	key := testKey("header-length-key")
	pw := NewPackWriter(key)

	if err := pw.AddBlob(testID("b1"), DataBlob, []byte("some data"), 9); err != nil {
		t.Fatalf("AddBlob: %v", err)
	}
	if err := pw.AddBlob(testID("b2"), DataBlob, []byte("more data"), 9); err != nil {
		t.Fatalf("AddBlob: %v", err)
	}

	_, data, err := pw.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Read the last 4 bytes as big-endian uint32.
	storedHeaderLen := binary.BigEndian.Uint32(data[len(data)-4:])

	// Extract the encrypted header using the stored length.
	headerStart := len(data) - 4 - int(storedHeaderLen)
	encryptedHeader := data[headerStart : len(data)-4]

	// The stored header length must equal the actual encrypted header size.
	if uint32(len(encryptedHeader)) != storedHeaderLen {
		t.Fatalf("header length mismatch: stored %d, actual encrypted header size %d",
			storedHeaderLen, len(encryptedHeader))
	}

	// Verify we can decrypt it.
	plaintext, err := crypto.Decrypt(key, encryptedHeader)
	if err != nil {
		t.Fatalf("failed to decrypt extracted header: %v", err)
	}
	if len(plaintext) == 0 {
		t.Fatal("decrypted header is empty")
	}
}

func TestPackIDIsSHA256OfContent(t *testing.T) {
	key := testKey("pack-id-key")
	pw := NewPackWriter(key)

	if err := pw.AddBlob(testID("x"), DataBlob, []byte("payload"), 7); err != nil {
		t.Fatalf("AddBlob: %v", err)
	}

	packID, data, err := pw.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Compute SHA-256 independently.
	computed := sha256.Sum256(data)
	if packID != computed {
		t.Fatalf("pack ID does not match SHA-256 of content: got %x, want %x", packID, computed)
	}
}

func TestPackEmptyPack(t *testing.T) {
	key := testKey("empty-pack-key")
	pw := NewPackWriter(key)

	if pw.Count() != 0 {
		t.Fatalf("expected Count()=0, got %d", pw.Count())
	}
	if pw.Size() != 0 {
		t.Fatalf("expected Size()=0, got %d", pw.Size())
	}

	packID, data, err := pw.Finalize()
	if err != nil {
		t.Fatalf("Finalize empty pack: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("expected non-empty data even for empty pack (header + length)")
	}

	// Verify pack ID.
	expectedID := sha256.Sum256(data)
	if packID != expectedID {
		t.Fatalf("pack ID mismatch for empty pack")
	}

	// Read header should return empty slice.
	blobs, err := ReadPackHeader(key, data)
	if err != nil {
		t.Fatalf("ReadPackHeader for empty pack: %v", err)
	}
	if len(blobs) != 0 {
		t.Fatalf("expected 0 blobs in empty pack header, got %d", len(blobs))
	}
}

func TestPackBlobOffsetAndLength(t *testing.T) {
	key := testKey("offset-length-key")
	pw := NewPackWriter(key)

	data1 := []byte("short")
	data2 := []byte("a much longer blob with significantly more content inside")

	if err := pw.AddBlob(testID("s"), DataBlob, data1, len(data1)); err != nil {
		t.Fatalf("AddBlob: %v", err)
	}
	if err := pw.AddBlob(testID("l"), DataBlob, data2, len(data2)); err != nil {
		t.Fatalf("AddBlob: %v", err)
	}

	_, packData, err := pw.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	blobs, err := ReadPackHeader(key, packData)
	if err != nil {
		t.Fatalf("ReadPackHeader: %v", err)
	}

	// First blob should start at offset 0.
	if blobs[0].Offset != 0 {
		t.Errorf("first blob offset: got %d, want 0", blobs[0].Offset)
	}

	// Second blob should start right after the first blob ends.
	if blobs[1].Offset != blobs[0].Length {
		t.Errorf("second blob offset: got %d, want %d (first blob length)",
			blobs[1].Offset, blobs[0].Length)
	}

	// Both blobs should be readable.
	plain1, err := ReadPackBlob(key, packData, blobs[0])
	if err != nil {
		t.Fatalf("ReadPackBlob[0]: %v", err)
	}
	if string(plain1) != string(data1) {
		t.Errorf("blob[0] data: got %q, want %q", plain1, data1)
	}

	plain2, err := ReadPackBlob(key, packData, blobs[1])
	if err != nil {
		t.Fatalf("ReadPackBlob[1]: %v", err)
	}
	if string(plain2) != string(data2) {
		t.Errorf("blob[1] data: got %q, want %q", plain2, data2)
	}
}
