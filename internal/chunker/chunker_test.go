package chunker

import (
	"bytes"
	"io"
	"math/rand"
	"testing"
)

// Default parameters used across all tests.
const (
	testMin uint = 256 * 1024      // 256 KiB
	testAvg uint = 1 * 1024 * 1024 // 1 MiB
	testMax uint = 4 * 1024 * 1024 // 4 MiB
)

// chunkAll reads every chunk from the reader and returns them.
func chunkAll(t *testing.T, data []byte) []Chunk {
	t.Helper()
	c := New(bytes.NewReader(data), testMin, testAvg, testMax)
	var chunks []Chunk
	for {
		ch, err := c.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Clone chunk data because chunker will return a temporary view.
		cloned := make([]byte, len(ch.Data))
		copy(cloned, ch.Data)
		ch.Data = cloned
		chunks = append(chunks, ch)
	}
	return chunks
}

func TestDeterministicBoundaries(t *testing.T) {
	// Same input must always produce the exact same chunk offsets and lengths.
	rng := rand.New(rand.NewSource(42))
	data := make([]byte, 10*1024*1024) // 10 MiB
	rng.Read(data)

	runs := make([][]Chunk, 3)
	for i := range runs {
		runs[i] = chunkAll(t, data)
	}

	if len(runs[0]) == 0 {
		t.Fatal("expected at least one chunk")
	}

	for r := 1; r < len(runs); r++ {
		if len(runs[r]) != len(runs[0]) {
			t.Fatalf("run %d produced %d chunks, run 0 produced %d", r, len(runs[r]), len(runs[0]))
		}
		for i := range runs[0] {
			if runs[r][i].Offset != runs[0][i].Offset {
				t.Errorf("run %d chunk %d offset %d != %d", r, i, runs[r][i].Offset, runs[0][i].Offset)
			}
			if runs[r][i].Length != runs[0][i].Length {
				t.Errorf("run %d chunk %d length %d != %d", r, i, runs[r][i].Length, runs[0][i].Length)
			}
		}
	}
}

func TestChunkSizeBounds(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	data := make([]byte, 10*1024*1024) // 10 MiB
	rng.Read(data)

	chunks := chunkAll(t, data)
	if len(chunks) < 2 {
		t.Fatal("expected multiple chunks for 10 MiB input")
	}

	for i, ch := range chunks {
		isLast := i == len(chunks)-1
		if ch.Length > testMax {
			t.Errorf("chunk %d: length %d exceeds max %d", i, ch.Length, testMax)
		}
		if !isLast && ch.Length < testMin {
			t.Errorf("chunk %d (not last): length %d below min %d", i, ch.Length, testMin)
		}
	}
}

func TestAverageChunkSize(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	size := 10 * 1024 * 1024
	data := make([]byte, size)
	rng.Read(data)

	chunks := chunkAll(t, data)
	if len(chunks) == 0 {
		t.Fatal("no chunks produced")
	}

	var totalLen uint64
	for _, ch := range chunks {
		totalLen += uint64(ch.Length)
	}
	avg := float64(totalLen) / float64(len(chunks))

	lower := float64(testAvg) * 0.5
	upper := float64(testAvg) * 1.5
	if avg < lower || avg > upper {
		t.Errorf("average chunk size %.0f not within 50%% of target %d (range [%.0f, %.0f])",
			avg, testAvg, lower, upper)
	}
	t.Logf("produced %d chunks, average size %.0f bytes (target %d)", len(chunks), avg, testAvg)
}

func TestIdenticalContentProducesIdenticalChunks(t *testing.T) {
	rng := rand.New(rand.NewSource(123))
	data := make([]byte, 5*1024*1024) // 5 MiB
	rng.Read(data)

	chunks1 := chunkAll(t, data)
	chunks2 := chunkAll(t, data)

	if len(chunks1) != len(chunks2) {
		t.Fatalf("chunk count mismatch: %d vs %d", len(chunks1), len(chunks2))
	}
	for i := range chunks1 {
		if chunks1[i].Offset != chunks2[i].Offset {
			t.Errorf("chunk %d offset mismatch: %d vs %d", i, chunks1[i].Offset, chunks2[i].Offset)
		}
		if chunks1[i].Length != chunks2[i].Length {
			t.Errorf("chunk %d length mismatch: %d vs %d", i, chunks1[i].Length, chunks2[i].Length)
		}
		if !bytes.Equal(chunks1[i].Data, chunks2[i].Data) {
			t.Errorf("chunk %d data mismatch", i)
		}
	}
}

func TestInsertedByteShiftsOnlyNearbyBoundaries(t *testing.T) {
	rng := rand.New(rand.NewSource(2024))
	original := make([]byte, 1*1024*1024) // 1 MiB
	rng.Read(original)

	// Insert a single byte at position 500000.
	insertPos := 500000
	modified := make([]byte, len(original)+1)
	copy(modified[:insertPos], original[:insertPos])
	modified[insertPos] = 0xFF
	copy(modified[insertPos+1:], original[insertPos:])

	origChunks := chunkAll(t, original)
	modChunks := chunkAll(t, modified)

	// Build a set of original chunk boundaries (end offsets).
	origBoundaries := make(map[uint64]bool)
	for _, ch := range origChunks {
		origBoundaries[ch.Offset+uint64(ch.Length)] = true
	}

	// Count how many modified boundaries match an original boundary (exactly
	// or shifted by 1 to account for the insertion).
	matched := 0
	for _, ch := range modChunks {
		end := ch.Offset + uint64(ch.Length)
		if origBoundaries[end] || origBoundaries[end-1] || origBoundaries[end+1] {
			matched++
		}
	}

	// At least 50% of the original boundaries should survive the insertion.
	threshold := len(origChunks) / 2
	if matched < threshold {
		t.Errorf("only %d/%d chunk boundaries survived a 1-byte insertion (need at least %d)",
			matched, len(origChunks), threshold)
	}
	t.Logf("original chunks: %d, modified chunks: %d, matched boundaries: %d",
		len(origChunks), len(modChunks), matched)
}

func TestDataIntegrity(t *testing.T) {
	// Verify that concatenating all chunk data reproduces the original input.
	rng := rand.New(rand.NewSource(555))
	data := make([]byte, 8*1024*1024) // 8 MiB
	rng.Read(data)

	chunks := chunkAll(t, data)

	var reconstructed []byte
	for _, ch := range chunks {
		reconstructed = append(reconstructed, ch.Data...)
	}

	if !bytes.Equal(data, reconstructed) {
		t.Fatalf("reconstructed data (%d bytes) does not match original (%d bytes)",
			len(reconstructed), len(data))
	}
}

func TestOffsetContinuity(t *testing.T) {
	// Each chunk's offset must equal the previous chunk's offset + length.
	rng := rand.New(rand.NewSource(777))
	data := make([]byte, 6*1024*1024)
	rng.Read(data)

	chunks := chunkAll(t, data)
	if len(chunks) == 0 {
		t.Fatal("no chunks produced")
	}

	if chunks[0].Offset != 0 {
		t.Errorf("first chunk offset should be 0, got %d", chunks[0].Offset)
	}

	for i := 1; i < len(chunks); i++ {
		expected := chunks[i-1].Offset + uint64(chunks[i-1].Length)
		if chunks[i].Offset != expected {
			t.Errorf("chunk %d offset %d != expected %d", i, chunks[i].Offset, expected)
		}
	}
}

func TestEmptyInput(t *testing.T) {
	c := New(bytes.NewReader(nil), testMin, testAvg, testMax)
	_, err := c.Next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF for empty input, got %v", err)
	}
}

func TestSmallInput(t *testing.T) {
	// Input smaller than min should produce a single chunk.
	data := []byte("hello world, this is a small input")
	chunks := chunkAll(t, data)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for small input, got %d", len(chunks))
	}
	if chunks[0].Length != uint(len(data)) {
		t.Errorf("chunk length %d != input length %d", chunks[0].Length, len(data))
	}
	if !bytes.Equal(chunks[0].Data, data) {
		t.Error("chunk data does not match input")
	}
}

func BenchmarkChunker(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	// 32 MiB of random data
	data := make([]byte, 32*1024*1024)
	rng.Read(data)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := New(bytes.NewReader(data), testMin, testAvg, testMax)
		for {
			_, err := c.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatalf("unexpected error: %v", err)
			}
		}
	}
}
