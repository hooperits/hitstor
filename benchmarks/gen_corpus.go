// Command gen_corpus builds a deterministic filesystem tree for backup benchmarks.
//
// Usage:
//
//	go run ./benchmarks/gen_corpus.go -out /path/to/corpus -profile small
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
)

type profile struct {
	name       string
	files      int
	avgSize    int
	dirs       int
	duplicates float64 // fraction of files that reuse a previous payload (dedup stress)
}

var profiles = map[string]profile{
	"tiny": {
		name: "tiny", files: 50, avgSize: 8 * 1024, dirs: 5, duplicates: 0.2,
	},
	"small": {
		name: "small", files: 1_000, avgSize: 100 * 1024, dirs: 20, duplicates: 0.25,
	},
	"medium": {
		name: "medium", files: 5_000, avgSize: 200 * 1024, dirs: 50, duplicates: 0.3,
	},
	"large": {
		name: "large", files: 20_000, avgSize: 256 * 1024, dirs: 100, duplicates: 0.35,
	},
}

func main() {
	out := flag.String("out", "", "output corpus directory (required)")
	profName := flag.String("profile", "small", "corpus profile: tiny|small|medium|large")
	seed := flag.Int64("seed", 42, "PRNG seed for reproducibility")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}
	p, ok := profiles[*profName]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown profile %q\n", *profName)
		os.Exit(2)
	}

	if err := os.RemoveAll(*out); err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}

	rng := rand.New(rand.NewSource(*seed))
	// Pre-build directory layout.
	dirNames := make([]string, p.dirs)
	for i := 0; i < p.dirs; i++ {
		dirNames[i] = filepath.Join(*out, fmt.Sprintf("d%02d", i))
		if err := os.MkdirAll(dirNames[i], 0o755); err != nil {
			fatal(err)
		}
	}

	var payloads [][]byte
	var totalBytes int64
	for i := 0; i < p.files; i++ {
		dir := dirNames[rng.Intn(len(dirNames))]
		name := filepath.Join(dir, fmt.Sprintf("f%05d.bin", i))

		var data []byte
		if len(payloads) > 0 && rng.Float64() < p.duplicates {
			data = payloads[rng.Intn(len(payloads))]
		} else {
			// Variable size around avg (50%–150%).
			size := p.avgSize/2 + rng.Intn(p.avgSize+1)
			data = make([]byte, size)
			// Fill with deterministic pseudo-random bytes derived from seed+i.
			fillDeterministic(data, *seed, int64(i))
			// Keep a few unique payloads for later duplicate reuse (cap memory).
			if len(payloads) < 64 {
				cp := make([]byte, len(data))
				copy(cp, data)
				payloads = append(payloads, cp)
			}
		}

		if err := os.WriteFile(name, data, 0o644); err != nil {
			fatal(err)
		}
		totalBytes += int64(len(data))
	}

	// Manifest for verification / publication.
	manifest := fmt.Sprintf("profile=%s\nseed=%d\nfiles=%d\nbytes=%d\ndirs=%d\nduplicates=%.2f\n",
		p.name, *seed, p.files, totalBytes, p.dirs, p.duplicates)
	if err := os.WriteFile(filepath.Join(*out, "CORPUS.txt"), []byte(manifest), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("corpus ready: %s\n%s", *out, manifest)
}

func fillDeterministic(buf []byte, seed, i int64) {
	var counter uint64
	for off := 0; off < len(buf); {
		h := sha256.New()
		var hdr [16]byte
		binary.LittleEndian.PutUint64(hdr[0:8], uint64(seed))
		binary.LittleEndian.PutUint64(hdr[8:16], uint64(i)^counter)
		_, _ = h.Write(hdr[:])
		sum := h.Sum(nil)
		n := copy(buf[off:], sum)
		off += n
		counter++
	}
	// Avoid all-zero compressible regions looking identical across tools unfairly:
	// already hashed content is fine for CDC tools.
	_ = io.Discard
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
