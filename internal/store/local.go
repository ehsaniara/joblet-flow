package store

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"google.golang.org/protobuf/proto"

	storepb "github.com/ehsaniara/joblet-flow/internal/store/proto/gen"
)

// LocalBackend persists events to an append-only log on local disk. It uses
// only the standard library and the generated protobuf, so it can run in-core
// or in the flow-store subprocess. The log is the event stream itself, which a
// later recovery path replays to rebuild state.
type LocalBackend struct {
	mu sync.Mutex
	f  *os.File
}

// NewLocalBackend opens (creating as needed) the append-only event log at
// dir/events.log.
func NewLocalBackend(dir string) (*LocalBackend, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}
	return &LocalBackend{f: f}, nil
}

// Apply appends the framed event and flushes it to disk.
func (b *LocalBackend) Apply(_ context.Context, ev *storepb.StoreEvent) error {
	data, err := proto.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(data)))

	b.mu.Lock()
	defer b.mu.Unlock()
	if _, err := b.f.Write(hdr[:]); err != nil {
		return err
	}
	if _, err := b.f.Write(data); err != nil {
		return err
	}
	return b.f.Sync()
}

// Close closes the event log.
func (b *LocalBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.f.Close()
}
