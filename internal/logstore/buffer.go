package logstore

import (
	"sort"
	"sync"
)

type ringBuffer struct {
	mu       sync.RWMutex
	entries  []Entry
	capacity int
}

func newRingBuffer(capacity int) *ringBuffer {
	if capacity < 500 {
		capacity = 500
	}
	return &ringBuffer{capacity: capacity}
}

func (b *ringBuffer) resize(capacity int) {
	if capacity < 500 {
		capacity = 500
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.capacity = capacity
	if len(b.entries) > capacity {
		b.entries = b.entries[len(b.entries)-capacity:]
	}
}

func (b *ringBuffer) append(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) >= b.capacity {
		copy(b.entries, b.entries[1:])
		b.entries[len(b.entries)-1] = e
		return
	}
	b.entries = append(b.entries, e)
}

func (b *ringBuffer) query(f Filter) []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	limit := f.Limit
	if limit <= 0 {
		limit = 500
	}
	out := make([]Entry, 0, limit)
	for i := len(b.entries) - 1; i >= 0; i-- {
		if !b.entries[i].Matches(f) {
			continue
		}
		out = append(out, b.entries[i])
		if len(out) >= limit {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Time.Before(out[j].Time)
	})
	return out
}
