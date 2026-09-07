package engine

import (
	"context"

	pb "github.com/ehsaniara/joblet-proto/v2/gen/flow"
)

// Coarse workflow lifecycle states surfaced through GetWorkflow.
const (
	StatusRunning   = "RUNNING"
	StatusCompleted = "COMPLETED"
	StatusFailed    = "FAILED"
)

// Run is the durable state of one workflow execution (Input/Result are opaque bytes).
type Run struct {
	ID       string
	Workflow string
	Input    []byte
	Status   string
	Result   []byte
	Error    string
}

// Store holds engine state: runs, task queues, and step-indexed memos. A durable
// implementation swaps in behind this interface. See docs/ARCHITECTURE.md.
type Store interface {
	// CreateRun records a new run; returns false if the id already exists.
	CreateRun(run *Run) (created bool)
	GetRun(id string) (*Run, bool)
	// SetRunResult moves a run to a terminal state.
	SetRunResult(id, status string, result []byte, errMsg string)

	// Enqueue adds a task; Poll blocks until one is available or ctx is done.
	Enqueue(queue string, task *pb.Task)
	Poll(ctx context.Context, queue string) (*pb.Task, bool)

	// Job-activity results memoized by step.
	GetActivity(workflowID string, step int32) (*pb.RunActivityResponse, bool)
	PutActivity(workflowID string, step int32, resp *pb.RunActivityResponse)

	// Worker-activity results memoized by step.
	GetWorkerActivity(workflowID string, step int32) ([]byte, bool)
	PutWorkerActivity(workflowID string, step int32, result []byte)

	// Side-effect results memoized by step.
	GetSideEffect(workflowID string, step int32) ([]byte, bool)
	PutSideEffect(workflowID string, step int32, data []byte)

	// Signal-wait payloads memoized by step.
	GetSignalWait(workflowID string, step int32) ([]byte, bool)
	PutSignalWait(workflowID string, step int32, payload []byte)
}
