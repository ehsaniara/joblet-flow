// Package store carries flow's durable-state plumbing: a stdlib pub/sub for
// state-change events, the Backend interface a sink implements, and the
// flow-core <-> flow-store IPC. Core flow depends only on the standard library
// and the generated protobuf; any backend needing third-party libraries runs
// in the flow-store subprocess.
package store

import (
	"sync"

	storepb "github.com/ehsaniara/joblet-flow/internal/store/proto/gen"
)

// PubSub fans state-change events out to every subscriber. The publisher never
// blocks on a slow subscriber: a full subscriber buffer drops the event for
// that subscriber only (the authoritative state stays in the engine's memory;
// a dropped event only costs that sink some durability, recovered on the next
// full checkpoint). Subscribers consume at their own pace.
type PubSub struct {
	mu   sync.RWMutex
	subs map[int]chan *storepb.StoreEvent
	next int
	size int
}

// NewPubSub returns a PubSub whose subscriber channels buffer size events.
func NewPubSub(size int) *PubSub {
	if size <= 0 {
		size = 1024
	}
	return &PubSub{subs: make(map[int]chan *storepb.StoreEvent), size: size}
}

// Subscribe returns a receive channel and an unsubscribe function.
func (p *PubSub) Subscribe() (<-chan *storepb.StoreEvent, func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := p.next
	p.next++
	ch := make(chan *storepb.StoreEvent, p.size)
	p.subs[id] = ch
	return ch, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if c, ok := p.subs[id]; ok {
			delete(p.subs, id)
			close(c)
		}
	}
}

// Publish delivers ev to every subscriber, skipping any whose buffer is full.
func (p *PubSub) Publish(ev *storepb.StoreEvent) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, ch := range p.subs {
		select {
		case ch <- ev:
		default: // slow subscriber; skip rather than block the engine
		}
	}
}

// Close removes and closes all subscriber channels.
func (p *PubSub) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, ch := range p.subs {
		delete(p.subs, id)
		close(ch)
	}
}
