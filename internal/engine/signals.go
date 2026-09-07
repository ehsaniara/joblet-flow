package engine

import "sync"

// signalHub buffers signals and correlates them with WaitSignal waiters, atomic
// under one lock. Ephemeral. Semantics: docs/PROTOCOL.md.
type signalHub struct {
	mu      sync.Mutex
	buffers map[string][][]byte      // key -> payloads awaiting a waiter
	waiters map[string][]chan []byte // key -> channels blocked in WaitSignal
}

func newSignalHub() *signalHub {
	return &signalHub{
		buffers: make(map[string][][]byte),
		waiters: make(map[string][]chan []byte),
	}
}

func signalKey(workflowID, name string) string {
	return workflowID + "\x00" + name
}

// send delivers a signal to the oldest waiter, or buffers it if none is waiting.
func (h *signalHub) send(workflowID, name string, payload []byte) {
	key := signalKey(workflowID, name)
	h.mu.Lock()
	defer h.mu.Unlock()
	if ws := h.waiters[key]; len(ws) > 0 {
		ch := ws[0]
		h.waiters[key] = ws[1:]
		ch <- payload // capacity-1 channel; never blocks
		return
	}
	h.buffers[key] = append(h.buffers[key], payload)
}

// wait returns a channel yielding the signal payload (buffered, or on delivery).
func (h *signalHub) wait(workflowID, name string) chan []byte {
	key := signalKey(workflowID, name)
	ch := make(chan []byte, 1)
	h.mu.Lock()
	defer h.mu.Unlock()
	if buf := h.buffers[key]; len(buf) > 0 {
		ch <- buf[0]
		h.buffers[key] = buf[1:]
		return ch
	}
	h.waiters[key] = append(h.waiters[key], ch)
	return ch
}

// cancel removes ch from the waiters for (workflowID, name).
func (h *signalHub) cancel(workflowID, name string, ch chan []byte) {
	key := signalKey(workflowID, name)
	h.mu.Lock()
	defer h.mu.Unlock()
	ws := h.waiters[key]
	for i, w := range ws {
		if w == ch {
			h.waiters[key] = append(ws[:i], ws[i+1:]...)
			return
		}
	}
}
