package store

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	storepb "github.com/ehsaniara/joblet-flow/internal/store/proto/gen"
)

func TestPubSub_FanOutToMultipleSubscribers(t *testing.T) {
	ps := NewPubSub(8)
	defer ps.Close()
	a, unsubA := ps.Subscribe()
	b, unsubB := ps.Subscribe()
	defer unsubA()
	defer unsubB()

	ps.Publish(&storepb.StoreEvent{WorkflowId: "wf-1"})

	for _, ch := range []<-chan *storepb.StoreEvent{a, b} {
		select {
		case ev := <-ch:
			if ev.GetWorkflowId() != "wf-1" {
				t.Fatalf("got %q, want wf-1", ev.GetWorkflowId())
			}
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive the event")
		}
	}
}

func TestPubSub_SlowSubscriberDoesNotBlockPublisher(t *testing.T) {
	ps := NewPubSub(1) // tiny buffer
	defer ps.Close()
	_, unsub := ps.Subscribe() // never drained
	defer unsub()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			ps.Publish(&storepb.StoreEvent{WorkflowId: "x"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publisher blocked on a slow subscriber")
	}
}

func TestPubSub_UnsubscribeStopsDelivery(t *testing.T) {
	ps := NewPubSub(4)
	defer ps.Close()
	ch, unsub := ps.Subscribe()
	unsub()
	ps.Publish(&storepb.StoreEvent{WorkflowId: "x"})
	if _, ok := <-ch; ok {
		t.Fatal("expected closed channel after unsubscribe")
	}
}

func TestLocalBackend_AppendsFramedEvents(t *testing.T) {
	dir := t.TempDir()
	b, err := NewLocalBackend(dir)
	if err != nil {
		t.Fatalf("NewLocalBackend: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := b.Apply(context.Background(), &storepb.StoreEvent{
			Type: storepb.EventType_EVENT_TYPE_CREATE_RUN, WorkflowId: "wf-1",
		}); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// The log holds exactly the three framed events.
	f, err := os.Open(filepath.Join(dir, "events.log"))
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()
	for i := 0; i < 3; i++ {
		if _, err := readEvent(f); err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
	}
	if _, err := readEvent(f); err == nil {
		t.Fatal("expected exactly 3 events")
	}
}

// TestIPCRoundTrip ships events over a real Unix socket into a LocalBackend,
// end to end: writeEvent -> ServeConn -> Apply.
func TestIPCRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "s.sock")
	backend, err := NewLocalBackend(dir)
	if err != nil {
		t.Fatalf("backend: %v", err)
	}
	defer backend.Close()

	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lis.Close()

	var applied int
	var mu sync.Mutex
	served := make(chan struct{})
	go func() {
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		_ = ServeConn(conn, backend, func(ev *storepb.StoreEvent) error {
			mu.Lock()
			applied++
			mu.Unlock()
			return backend.Apply(context.Background(), ev)
		})
		close(served)
	}()

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := writeEvent(conn, &storepb.StoreEvent{WorkflowId: "wf-1", Step: int32(i)}); err != nil {
			t.Fatalf("writeEvent: %v", err)
		}
	}
	conn.Close()

	select {
	case <-served:
	case <-time.After(2 * time.Second):
		t.Fatal("ServeConn did not finish")
	}
	mu.Lock()
	defer mu.Unlock()
	if applied != 5 {
		t.Fatalf("applied %d events, want 5", applied)
	}
}
