package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/hooperits/hitstor/internal/backend"
	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

// memoryBackend stores all data in memory. Useful for testing.
type memoryBackend struct {
	mu    sync.Mutex
	store map[string][]byte
}

var _ backend.Backend = (*memoryBackend)(nil)

func newMemoryBackend() *memoryBackend {
	return &memoryBackend{store: make(map[string][]byte)}
}

func (m *memoryBackend) storageKey(ft backend.FileType, id backend.ID) string {
	return fmt.Sprintf("%d:%s", ft, id.String())
}

func (m *memoryBackend) Save(_ context.Context, ft backend.FileType, id backend.ID, rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return fmt.Errorf("memory: read: %w", err)
	}
	m.mu.Lock()
	m.store[m.storageKey(ft, id)] = data
	m.mu.Unlock()
	return nil
}

func (m *memoryBackend) Load(_ context.Context, ft backend.FileType, id backend.ID) (io.ReadCloser, error) {
	m.mu.Lock()
	data, ok := m.store[m.storageKey(ft, id)]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("memory: not found: %s/%s", ft, id)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *memoryBackend) Stat(_ context.Context, ft backend.FileType, id backend.ID) (backend.FileInfo, error) {
	m.mu.Lock()
	data, ok := m.store[m.storageKey(ft, id)]
	m.mu.Unlock()
	if !ok {
		return backend.FileInfo{}, fmt.Errorf("memory: not found: %s/%s", ft, id)
	}
	return backend.FileInfo{Size: int64(len(data))}, nil
}

func (m *memoryBackend) Remove(_ context.Context, ft backend.FileType, id backend.ID) error {
	m.mu.Lock()
	delete(m.store, m.storageKey(ft, id))
	m.mu.Unlock()
	return nil
}

func (m *memoryBackend) List(_ context.Context, ft backend.FileType) ([]backend.ID, error) {
	prefix := fmt.Sprintf("%d:", ft)
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []backend.ID
	for k := range m.store {
		if strings.HasPrefix(k, prefix) {
			hexStr := strings.TrimPrefix(k, prefix)
			b, err := hex.DecodeString(hexStr)
			if err != nil || len(b) != 32 {
				continue
			}
			var id backend.ID
			copy(id[:], b)
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (m *memoryBackend) Close() error { return nil }

func main() {
	hitstorplugin.ServeStorageBackend(newMemoryBackend())
}
