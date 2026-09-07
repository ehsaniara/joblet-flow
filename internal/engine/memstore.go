package engine

import (
	"context"
	"sync"

	pb "github.com/ehsaniara/joblet-proto/v2/gen/flow"
)

// memStore is an in-memory Store for development and tests (state lost on restart).
type memStore struct {
	mu     sync.Mutex
	runs   map[string]*Run
	acts   map[string]map[int32]*pb.RunActivityResponse
	wacts  map[string]map[int32][]byte
	sides  map[string]map[int32][]byte
	swaits map[string]map[int32][]byte
	queues map[string]chan *pb.Task
}

// NewMemStore returns an in-memory Store implementation.
func NewMemStore() Store {
	return &memStore{
		runs:   make(map[string]*Run),
		acts:   make(map[string]map[int32]*pb.RunActivityResponse),
		wacts:  make(map[string]map[int32][]byte),
		sides:  make(map[string]map[int32][]byte),
		swaits: make(map[string]map[int32][]byte),
		queues: make(map[string]chan *pb.Task),
	}
}

func (s *memStore) CreateRun(run *Run) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[run.ID]; exists {
		return false
	}
	s.runs[run.ID] = run
	return true
}

func (s *memStore) GetRun(id string) (*Run, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return nil, false
	}
	cp := *run // copy so callers can't mutate stored state under our lock
	return &cp, true
}

func (s *memStore) SetRunResult(id, status string, result []byte, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return
	}
	run.Status = status
	run.Result = result
	run.Error = errMsg
}

func (s *memStore) queueChan(queue string) chan *pb.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.queues[queue]
	if !ok {
		ch = make(chan *pb.Task, 1024)
		s.queues[queue] = ch
	}
	return ch
}

func (s *memStore) Enqueue(queue string, task *pb.Task) {
	ch := s.queueChan(queue)
	select {
	case ch <- task:
	default:
		// Buffer full: hand off to a goroutine so the caller never blocks.
		go func() { ch <- task }()
	}
}

func (s *memStore) Poll(ctx context.Context, queue string) (*pb.Task, bool) {
	ch := s.queueChan(queue)
	select {
	case task := <-ch:
		return task, true
	case <-ctx.Done():
		return nil, false
	}
}

func (s *memStore) GetActivity(workflowID string, step int32) (*pb.RunActivityResponse, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	resp, ok := s.acts[workflowID][step]
	return resp, ok
}

func (s *memStore) PutActivity(workflowID string, step int32, resp *pb.RunActivityResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.acts[workflowID] == nil {
		s.acts[workflowID] = make(map[int32]*pb.RunActivityResponse)
	}
	s.acts[workflowID][step] = resp
}

func (s *memStore) GetWorkerActivity(workflowID string, step int32) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, ok := s.wacts[workflowID][step]
	return result, ok
}

func (s *memStore) PutWorkerActivity(workflowID string, step int32, result []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wacts[workflowID] == nil {
		s.wacts[workflowID] = make(map[int32][]byte)
	}
	s.wacts[workflowID][step] = result
}

func (s *memStore) GetSideEffect(workflowID string, step int32) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.sides[workflowID][step]
	return data, ok
}

func (s *memStore) PutSideEffect(workflowID string, step int32, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sides[workflowID] == nil {
		s.sides[workflowID] = make(map[int32][]byte)
	}
	s.sides[workflowID][step] = data
}

func (s *memStore) GetSignalWait(workflowID string, step int32) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, ok := s.swaits[workflowID][step]
	return payload, ok
}

func (s *memStore) PutSignalWait(workflowID string, step int32, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.swaits[workflowID] == nil {
		s.swaits[workflowID] = make(map[int32][]byte)
	}
	s.swaits[workflowID][step] = payload
}
