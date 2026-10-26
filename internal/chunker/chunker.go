// Package chunker implements FastCDC (Fast Content-Defined Chunking) using the
// Gear hash rolling hash variant. It splits an input stream into variable-size
// chunks whose boundaries are determined by the content, so that insertions or
// deletions only affect nearby chunk boundaries.
package chunker

import (
	"io"
	"math/bits"
	"math/rand"
)

// Default chunking parameters.
const (
	DefaultMin uint = 256 * 1024      // 256 KiB
	DefaultAvg uint = 1 * 1024 * 1024 // 1 MiB
	DefaultMax uint = 4 * 1024 * 1024 // 4 MiB
)

// Chunk represents a single chunk produced by the chunker.
// Note: Data is a view into the chunker's internal buffer and is only
// valid until the next call to Next(). If you need to retain the data,
// you must make a copy.
type Chunk struct {
	Offset uint64
	Length uint
	Data   []byte
}

// Chunker splits an io.Reader into content-defined chunks using the FastCDC
// algorithm with a Gear rolling hash.
type Chunker struct {
	rd            io.Reader
	min, avg, max uint
	buf           []byte
	blen          int // valid bytes in buf
	offset        uint64
	eof           bool // underlying reader exhausted
	consumed      int  // bytes consumed by the last returned chunk, shifted out on next Next()

	maskS uint64 // mask for range [min, avg)  -- more bits set, easier match
	maskL uint64 // mask for range [avg, max)  -- fewer bits set, harder match
}

// gearTable is a deterministic lookup table of 256 random uint64 values used
// by the Gear rolling hash.  It is generated once from a fixed seed so that
// chunk boundaries are reproducible across runs.
var gearTable [256]uint64

func init() {
	// Fixed seed for deterministic chunking.
	rng := rand.New(rand.NewSource(0xFA57CDC))
	for i := range gearTable {
		gearTable[i] = rng.Uint64()
	}
}

// mask creates a bitmask with the given number of '1' bits in the least
// significant positions.
func mask(nBits int) uint64 {
	if nBits >= 64 {
		return ^uint64(0)
	}
	if nBits <= 0 {
		return 0
	}
	return (1 << uint(nBits)) - 1
}

// New creates a new Chunker that reads from rd and produces chunks with the
// given minimum, average, and maximum sizes (in bytes).
func New(rd io.Reader, min, avg, max uint) *Chunker {
	// Number of bits corresponding to log2(avg).
	avgBits := bits.Len(avg) - 1

	// FastCDC normalization: maskS has more bits (easier to match near avg),
	// maskL has fewer bits (harder to match, lets chunks grow past avg).
	maskSBits := avgBits + 1
	maskLBits := avgBits - 1

	return &Chunker{
		rd:    rd,
		min:   min,
		avg:   avg,
		max:   max,
		buf:   make([]byte, max+4096),
		maskS: mask(maskSBits),
		maskL: mask(maskLBits),
	}
}

// fillBuf tries to ensure that buf contains at least c.max bytes of data
// (or all remaining data if the reader is exhausted).
func (c *Chunker) fillBuf() error {
	if c.eof {
		return nil
	}

	// Grow the backing slice if needed.
	need := int(c.max) + 4096
	if len(c.buf) < need {
		newBuf := make([]byte, need)
		copy(newBuf, c.buf[:c.blen])
		c.buf = newBuf
	}

	for c.blen < int(c.max) {
		readBuf := c.buf[c.blen:]
		n, err := c.rd.Read(readBuf)
		c.blen += n
		if err != nil {
			if err == io.EOF {
				c.eof = true
				return nil
			}
			return err
		}
	}
	return nil
}

// Next returns the next chunk from the input stream. When there is no more
// data, it returns a zero Chunk and io.EOF.
// The returned Chunk.Data is a view of the internal buffer and is only
// valid until the next call to Next().
func (c *Chunker) Next() (Chunk, error) {
	if c.consumed > 0 {
		// Shift remaining data to front of buffer.
		remaining := c.blen - c.consumed
		copy(c.buf, c.buf[c.consumed:c.blen])
		c.blen = remaining
		c.offset += uint64(c.consumed)
		c.consumed = 0
	}

	// Fill the buffer so we have enough data to determine a boundary.
	if err := c.fillBuf(); err != nil {
		return Chunk{}, err
	}

	if c.blen == 0 {
		return Chunk{}, io.EOF
	}

	data := c.buf[:c.blen]
	n := uint(len(data))

	// If the remaining data is at or below the minimum size, emit it all as
	// the final (possibly undersized) chunk.
	if n <= c.min {
		c.consumed = int(n)
		chunk := Chunk{
			Offset: c.offset,
			Length: n,
			Data:   data[:n],
		}
		return chunk, nil
	}

	// Determine chunk boundary using FastCDC gear hash.
	boundary := c.min
	var fp uint64 // gear hash fingerprint

	// Phase 1: positions [min, avg) -- use maskS (easier to match).
	limit := c.avg
	if limit > n {
		limit = n
	}
	if limit > c.min {
		subData := data[c.min:limit]
		for i, b := range subData {
			fp = (fp << 1) + gearTable[b]
			if fp&c.maskS == 0 {
				boundary = c.min + uint(i) + 1
				goto emit
			}
		}
	}

	// Phase 2: positions [avg, max) -- use maskL (harder to match).
	limit = c.max
	if limit > n {
		limit = n
	}
	if limit > c.avg {
		subData := data[c.avg:limit]
		for i, b := range subData {
			fp = (fp << 1) + gearTable[b]
			if fp&c.maskL == 0 {
				boundary = c.avg + uint(i) + 1
				goto emit
			}
		}
	}

	// No boundary found before max -- force cut at max (or end of data).
	if n < c.max {
		boundary = n
	} else {
		boundary = c.max
	}

emit:
	c.consumed = int(boundary)
	chunk := Chunk{
		Offset: c.offset,
		Length: boundary,
		Data:   data[:boundary],
	}

	return chunk, nil
}
