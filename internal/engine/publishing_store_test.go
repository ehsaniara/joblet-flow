package engine

import (
	"context"
	"testing"

	pb "github.com/ehsaniara/joblet-proto/v2/gen/flow"

	"github.com/ehsaniara/joblet-flow/internal/store"
	storepb "github.com/ehsaniara/joblet-flow/internal/store/proto/gen"
)

// drain collects events a subscriber receives until n arrive or the channel is
// exhausted after a mutation burst.
func TestPublishingStore_MutationsPublish_ReadsPassThrough(t *testing.T) {
	ps := store.NewPubSub(64)
	defer ps.Close()
	ch, unsub := ps.Subscribe()
	defer unsub()

	st := NewPublishingStore(NewMemStore(), ps)

	// A create + a memo write; both must publish and both must be readable back
	// from the in-memory authoritative state.
	if !st.CreateRun(&Run{ID: "wf-1", Workflow: "demo", Status: StatusRunning}) {
		t.Fatal("CreateRun returned false")
	}
	st.PutSideEffect("wf-1", 0, []byte("effect"))

	if run, ok := st.GetRun("wf-1"); !ok || run.Workflow != "demo" {
		t.Fatalf("read-through failed: %+v ok=%v", run, ok)
	}
	if got, ok := st.GetSideEffect("wf-1", 0); !ok || string(got) != "effect" {
		t.Fatalf("memo read-through failed: %q ok=%v", got, ok)
	}

	got := map[storepb.EventType]bool{}
	for i := 0; i < 2; i++ {
		select {
		case ev := <-ch:
			got[ev.GetType()] = true
		default:
			t.Fatalf("expected 2 published events, missing after %d", i)
		}
	}
	if !got[storepb.EventType_EVENT_TYPE_CREATE_RUN] || !got[storepb.EventType_EVENT_TYPE_PUT_SIDE_EFFECT] {
		t.Fatalf("missing event types, got %v", got)
	}
}

func TestPublishingStore_IdempotentCreateDoesNotRepublish(t *testing.T) {
	ps := store.NewPubSub(64)
	defer ps.Close()
	ch, unsub := ps.Subscribe()
	defer unsub()
	st := NewPublishingStore(NewMemStore(), ps)

	st.CreateRun(&Run{ID: "wf-1", Workflow: "demo"})
	st.CreateRun(&Run{ID: "wf-1", Workflow: "demo"}) // duplicate: no publish

	<-ch // the first create
	select {
	case ev := <-ch:
		t.Fatalf("duplicate create republished: %+v", ev)
	default:
	}
}

// enqueue/poll still work through the decorator (Enqueue is overridden, Poll
// passes through to the embedded store).
func TestPublishingStore_EnqueuePollThrough(t *testing.T) {
	ps := store.NewPubSub(64)
	defer ps.Close()
	st := NewPublishingStore(NewMemStore(), ps)
	st.Enqueue("default", &pb.Task{WorkflowId: "wf-1", Handler: "demo"})
	task, ok := st.Poll(context.Background(), "default")
	if !ok || task.GetWorkflowId() != "wf-1" {
		t.Fatalf("poll-through failed: %+v ok=%v", task, ok)
	}
}
