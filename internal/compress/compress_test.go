package compress

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestCompressDecompress(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "short string round-trips correctly",
			input: []byte("hello, hitstor backup engine!"),
		},
		{
			name:  "empty input compresses and decompresses correctly",
			input: []byte{},
		},
		{
			name:  "binary data round-trips correctly",
			input: []byte{0x00, 0xff, 0x0a, 0x0d, 0x80, 0x7f},
		},
		{
			name:  "single byte round-trips correctly",
			input: []byte{0x42},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			compressed, err := Compress(tc.input)
			if err != nil {
				t.Fatalf("Compress() error: %v", err)
			}

			decompressed, err := Decompress(compressed)
			if err != nil {
				t.Fatalf("Decompress() error: %v", err)
			}

			if !bytes.Equal(decompressed, tc.input) {
				t.Errorf("round-trip mismatch: got %v, want %v", decompressed, tc.input)
			}
		})
	}
}

func TestCompressedSizeSmallerForCompressibleData(t *testing.T) {
	// Repeated bytes are highly compressible.
	input := bytes.Repeat([]byte("AAAAAAAAAA"), 10000) // 100KB of repeated 'A'

	compressed, err := Compress(input)
	if err != nil {
		t.Fatalf("Compress() error: %v", err)
	}

	if len(compressed) >= len(input) {
		t.Errorf("compressed size (%d) should be smaller than original size (%d) for compressible data",
			len(compressed), len(input))
	}

	// Verify round-trip still works.
	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress() error: %v", err)
	}

	if !bytes.Equal(decompressed, input) {
		t.Error("round-trip mismatch for compressible data")
	}
}

func TestLargeRandomDataRoundTrip(t *testing.T) {
	// 1MB of random data.
	input := make([]byte, 1<<20)
	if _, err := rand.Read(input); err != nil {
		t.Fatalf("failed to generate random data: %v", err)
	}

	compressed, err := Compress(input)
	if err != nil {
		t.Fatalf("Compress() error: %v", err)
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress() error: %v", err)
	}

	if !bytes.Equal(decompressed, input) {
		t.Error("round-trip mismatch for 1MB random data")
	}
}

func TestDecompressInvalidData(t *testing.T) {
	_, err := Decompress([]byte("this is not valid zstd data"))
	if err == nil {
		t.Error("Decompress() should return error for invalid input")
	}
}
