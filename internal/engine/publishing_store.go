package engine

import (
	pb "github.com/ehsaniara/joblet-proto/v2/gen/flow"
	"google.golang.org/protobuf/proto"

	"github.com/ehsaniara/joblet-flow/internal/store"
	storepb "github.com/ehsaniara/joblet-flow/internal/store/proto/gen"
)

// publishingStore decorates a Store: reads pass straight through to the
// in-memory authoritative state, and every mutation is also published as a
// StoreEvent for durable sinks. Publishing never blocks or fails the mutation.
type publishingStore struct {
	Store
	ps *store.PubSub
}

// NewPublishingStore wraps inner so its mutations publish to ps.
func NewPublishingStore(inner Store, ps *store.PubSub) Store {
	return &publishingStore{Store: inner, ps: ps}
}

func (s *publishingStore) CreateRun(run *Run) bool {
	created := s.Store.CreateRun(run)
	if created {
		s.ps.Publish(&storepb.StoreEvent{
			Type:       storepb.EventType_EVENT_TYPE_CREATE_RUN,
			WorkflowId: run.ID,
			Workflow:   run.Workflow,
			Status:     run.Status,
			Payload:    run.Input,
		})
	}
	return created
}

func (s *publishingStore) SetRunResult(id, status string, result []byte, errMsg string) {
	s.Store.SetRunResult(id, status, result, errMsg)
	s.ps.Publish(&storepb.StoreEvent{
		Type:       storepb.EventType_EVENT_TYPE_SET_RESULT,
		WorkflowId: id,
		Status:     status,
		Error:      errMsg,
		Payload:    result,
	})
}

func (s *publishingStore) Enqueue(queue string, task *pb.Task) {
	s.Store.Enqueue(queue, task)
	if data, err := proto.Marshal(task); err == nil {
		s.ps.Publish(&storepb.StoreEvent{
			Type:    storepb.EventType_EVENT_TYPE_ENQUEUE,
			Queue:   queue,
			Payload: data,
		})
	}
}

func (s *publishingStore) PutActivity(workflowID string, step int32, resp *pb.RunActivityResponse) {
	s.Store.PutActivity(workflowID, step, resp)
	if data, err := proto.Marshal(resp); err == nil {
		s.ps.Publish(&storepb.StoreEvent{
			Type:       storepb.EventType_EVENT_TYPE_PUT_ACTIVITY,
			WorkflowId: workflowID,
			Step:       step,
			Payload:    data,
		})
	}
}

func (s *publishingStore) PutWorkerActivity(workflowID string, step int32, result []byte) {
	s.Store.PutWorkerActivity(workflowID, step, result)
	s.ps.Publish(&storepb.StoreEvent{
		Type:       storepb.EventType_EVENT_TYPE_PUT_WORKER_ACTIVITY,
		WorkflowId: workflowID,
		Step:       step,
		Payload:    result,
	})
}

func (s *publishingStore) PutSideEffect(workflowID string, step int32, data []byte) {
	s.Store.PutSideEffect(workflowID, step, data)
	s.ps.Publish(&storepb.StoreEvent{
		Type:       storepb.EventType_EVENT_TYPE_PUT_SIDE_EFFECT,
		WorkflowId: workflowID,
		Step:       step,
		Payload:    data,
	})
}

func (s *publishingStore) PutSignalWait(workflowID string, step int32, payload []byte) {
	s.Store.PutSignalWait(workflowID, step, payload)
	s.ps.Publish(&storepb.StoreEvent{
		Type:       storepb.EventType_EVENT_TYPE_PUT_SIGNAL_WAIT,
		WorkflowId: workflowID,
		Step:       step,
		Payload:    payload,
	})
}

// compile-time guard: publishingStore must satisfy Store.
var _ Store = (*publishingStore)(nil)
