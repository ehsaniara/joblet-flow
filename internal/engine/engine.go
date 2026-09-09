// Package engine is the joblet-flow orchestration core. See docs/ARCHITECTURE.md.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strconv"
	"sync"
	"time"

	pb "github.com/ehsaniara/joblet-proto/v2/gen/flow"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// defaultQueue receives workflow tasks that don't specify one.
const defaultQueue = "default"

// Engine implements gen.FlowServiceServer.
type Engine struct {
	pb.UnimplementedFlowServiceServer

	store  Store
	runner JobRunner
	log    *slog.Logger

	// pending correlates an in-flight worker activity to its blocked
	// RunWorkerActivity call. buffered holds an outcome that arrived before its
	// waiter registered (e.g. a worker reporting during a retry's backoff), so
	// it is delivered on the next register instead of being lost.
	mu       sync.Mutex
	pending  map[string]chan activityOutcome
	buffered map[string]activityOutcome

	signals *signalHub
}

// activityOutcome is the result an activity worker reports for a task.
type activityOutcome struct {
	result []byte
	err    string
}

// New builds an Engine over the given state store and activity job runner.
func New(store Store, runner JobRunner, log *slog.Logger) *Engine {
	if log == nil {
		log = slog.Default()
	}
	return &Engine{
		store:    store,
		runner:   runner,
		log:      log,
		pending:  make(map[string]chan activityOutcome),
		buffered: make(map[string]activityOutcome),
		signals:  newSignalHub(),
	}
}

// StartWorkflow begins a run and enqueues its workflow task; idempotent on workflow_id.
func (e *Engine) StartWorkflow(ctx context.Context, req *pb.StartWorkflowRequest) (*pb.StartWorkflowResponse, error) {
	if req.GetWorkflow() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow name is required")
	}

	id := req.GetWorkflowId()
	if id == "" {
		id = newID()
	}

	created := e.store.CreateRun(&Run{
		ID:       id,
		Workflow: req.GetWorkflow(),
		Input:    req.GetInput(),
		Status:   StatusRunning,
	})
	if created {
		e.store.Enqueue(defaultQueue, &pb.Task{
			WorkflowId: id,
			Type:       pb.TaskType_TASK_TYPE_WORKFLOW,
			Handler:    req.GetWorkflow(),
			Input:      req.GetInput(),
		})
		e.log.Info("workflow started", "workflow_id", id, "workflow", req.GetWorkflow())
	}
	return &pb.StartWorkflowResponse{WorkflowId: id}, nil
}

// pollDeadlineMargin lets an empty poll return before the caller's deadline fires.
const pollDeadlineMargin = 250 * time.Millisecond

// PollTask long-polls for the next task on a queue.
func (e *Engine) PollTask(ctx context.Context, req *pb.PollTaskRequest) (*pb.PollTaskResponse, error) {
	queue := req.GetQueue()
	if queue == "" {
		queue = defaultQueue
	}

	pollCtx := ctx
	if dl, ok := ctx.Deadline(); ok {
		stop := dl.Add(-pollDeadlineMargin)
		if !stop.After(time.Now()) {
			stop = time.Now().Add(10 * time.Millisecond)
		}
		var cancel context.CancelFunc
		pollCtx, cancel = context.WithDeadline(ctx, stop)
		defer cancel()
	}

	task, ok := e.store.Poll(pollCtx, queue)
	if !ok {
		return &pb.PollTaskResponse{Available: false}, nil
	}
	return &pb.PollTaskResponse{Task: task, Available: true}, nil
}

// CompleteTask (worker) records a workflow run's successful result.
func (e *Engine) CompleteTask(ctx context.Context, req *pb.CompleteTaskRequest) (*pb.CompleteTaskResponse, error) {
	if req.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id is required")
	}
	e.store.SetRunResult(req.GetWorkflowId(), StatusCompleted, req.GetResult(), "")
	e.log.Info("workflow completed", "workflow_id", req.GetWorkflowId())
	return &pb.CompleteTaskResponse{}, nil
}

// FailTask (worker) records a workflow run's terminal failure.
func (e *Engine) FailTask(ctx context.Context, req *pb.FailTaskRequest) (*pb.FailTaskResponse, error) {
	if req.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id is required")
	}
	e.store.SetRunResult(req.GetWorkflowId(), StatusFailed, nil, req.GetError())
	e.log.Info("workflow failed", "workflow_id", req.GetWorkflowId(), "error", req.GetError())
	return &pb.FailTaskResponse{}, nil
}

// RunActivity runs an activity as an isolated joblet job, blocking until done; memoized by step.
func (e *Engine) RunActivity(ctx context.Context, req *pb.RunActivityRequest) (*pb.RunActivityResponse, error) {
	if req.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id is required")
	}
	if req.GetJob() == nil {
		return nil, status.Error(codes.InvalidArgument, "job spec is required")
	}

	if memo, ok := e.store.GetActivity(req.GetWorkflowId(), req.GetStep()); ok {
		return memo, nil
	}

	resp, err := runWithRetry(ctx, e.runner, req.GetName(), req.GetJob(), req.GetRetry())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "activity %q dispatch failed: %v", req.GetName(), err)
	}

	e.store.PutActivity(req.GetWorkflowId(), req.GetStep(), resp)
	e.log.Info("activity finished", "workflow_id", req.GetWorkflowId(), "activity", req.GetName(),
		"step", req.GetStep(), "status", resp.GetStatus(), "job_uuid", resp.GetJobUuid())
	return resp, nil
}

// RunWorkerActivity runs a named activity on a worker, blocking until done; memoized by step.
func (e *Engine) RunWorkerActivity(ctx context.Context, req *pb.RunWorkerActivityRequest) (*pb.RunWorkerActivityResponse, error) {
	if req.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "activity name is required")
	}

	if memo, ok := e.store.GetWorkerActivity(req.GetWorkflowId(), req.GetStep()); ok {
		return &pb.RunWorkerActivityResponse{Result: memo}, nil
	}

	queue := req.GetQueue()
	if queue == "" {
		queue = defaultQueue
	}
	maxAttempts := int32(1)
	if rp := req.GetRetry(); rp != nil && rp.MaxAttempts > 1 {
		maxAttempts = rp.MaxAttempts
	}

	var lastErr string
	for attempt := int32(1); attempt <= maxAttempts; attempt++ {
		ch := e.registerPending(req.GetWorkflowId(), req.GetStep())
		e.store.Enqueue(queue, &pb.Task{
			WorkflowId: req.GetWorkflowId(),
			Type:       pb.TaskType_TASK_TYPE_ACTIVITY,
			Handler:    req.GetName(),
			Input:      req.GetInput(),
			Step:       req.GetStep(),
		})

		select {
		case out := <-ch:
			if out.err == "" {
				e.store.PutWorkerActivity(req.GetWorkflowId(), req.GetStep(), out.result)
				e.log.Info("worker activity finished", "workflow_id", req.GetWorkflowId(),
					"activity", req.GetName(), "step", req.GetStep())
				return &pb.RunWorkerActivityResponse{Result: out.result}, nil
			}
			lastErr = out.err
			if attempt < maxAttempts {
				select {
				case <-time.After(backoff(req.GetRetry(), attempt)):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
		case <-ctx.Done():
			e.clearPending(req.GetWorkflowId(), req.GetStep())
			return nil, ctx.Err()
		}
	}
	return &pb.RunWorkerActivityResponse{Error: lastErr}, nil
}

// CompleteActivity (activity worker) reports a completed activity task.
func (e *Engine) CompleteActivity(ctx context.Context, req *pb.CompleteActivityRequest) (*pb.CompleteActivityResponse, error) {
	if req.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id is required")
	}
	e.deliver(req.GetWorkflowId(), req.GetStep(), activityOutcome{result: req.GetResult()})
	return &pb.CompleteActivityResponse{}, nil
}

// FailActivity reports a failed activity task; the engine applies retry.
func (e *Engine) FailActivity(ctx context.Context, req *pb.FailActivityRequest) (*pb.FailActivityResponse, error) {
	if req.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id is required")
	}
	msg := req.GetError()
	if msg == "" {
		msg = "activity failed"
	}
	e.deliver(req.GetWorkflowId(), req.GetStep(), activityOutcome{err: msg})
	return &pb.FailActivityResponse{}, nil
}

func pendingKey(workflowID string, step int32) string {
	return workflowID + "\x00" + strconv.FormatInt(int64(step), 10)
}

func (e *Engine) registerPending(workflowID string, step int32) chan activityOutcome {
	ch := make(chan activityOutcome, 1)
	key := pendingKey(workflowID, step)
	e.mu.Lock()
	// An outcome that arrived before this register (e.g. during retry backoff)
	// was buffered; deliver it now rather than waiting for a fresh report.
	if out, ok := e.buffered[key]; ok {
		delete(e.buffered, key)
		ch <- out
	} else {
		e.pending[key] = ch
	}
	e.mu.Unlock()
	return ch
}

func (e *Engine) clearPending(workflowID string, step int32) {
	key := pendingKey(workflowID, step)
	e.mu.Lock()
	delete(e.pending, key)
	delete(e.buffered, key)
	e.mu.Unlock()
}

// deliver hands an outcome to the waiting RunWorkerActivity call, or buffers it
// if no waiter is currently registered so a report is never lost.
func (e *Engine) deliver(workflowID string, step int32, out activityOutcome) {
	key := pendingKey(workflowID, step)
	e.mu.Lock()
	ch, ok := e.pending[key]
	if ok {
		delete(e.pending, key)
	} else {
		e.buffered[key] = out
	}
	e.mu.Unlock()
	if ok {
		ch <- out
	}
}

// GetSideEffect returns a recorded side-effect result for a step, if one exists.
func (e *Engine) GetSideEffect(ctx context.Context, req *pb.GetSideEffectRequest) (*pb.GetSideEffectResponse, error) {
	data, found := e.store.GetSideEffect(req.GetWorkflowId(), req.GetStep())
	return &pb.GetSideEffectResponse{Data: data, Found: found}, nil
}

// RecordSideEffect stores a step's side-effect result.
func (e *Engine) RecordSideEffect(ctx context.Context, req *pb.RecordSideEffectRequest) (*pb.RecordSideEffectResponse, error) {
	e.store.PutSideEffect(req.GetWorkflowId(), req.GetStep(), req.GetData())
	return &pb.RecordSideEffectResponse{}, nil
}

// SignalWorkflow delivers a named signal (handed to a waiter or buffered).
func (e *Engine) SignalWorkflow(ctx context.Context, req *pb.SignalWorkflowRequest) (*pb.SignalWorkflowResponse, error) {
	if req.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id is required")
	}
	e.signals.send(req.GetWorkflowId(), req.GetName(), req.GetPayload())
	return &pb.SignalWorkflowResponse{}, nil
}

// WaitSignal blocks until a named signal arrives, memoized by step.
func (e *Engine) WaitSignal(ctx context.Context, req *pb.WaitSignalRequest) (*pb.WaitSignalResponse, error) {
	if req.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "signal name is required")
	}

	if memo, ok := e.store.GetSignalWait(req.GetWorkflowId(), req.GetStep()); ok {
		return &pb.WaitSignalResponse{Payload: memo}, nil
	}

	ch := e.signals.wait(req.GetWorkflowId(), req.GetName())
	select {
	case payload := <-ch:
		e.store.PutSignalWait(req.GetWorkflowId(), req.GetStep(), payload)
		return &pb.WaitSignalResponse{Payload: payload}, nil
	case <-ctx.Done():
		e.signals.cancel(req.GetWorkflowId(), req.GetName(), ch)
		// Recover a signal delivered in the cancellation race so it is not lost.
		select {
		case payload := <-ch:
			e.signals.send(req.GetWorkflowId(), req.GetName(), payload)
		default:
		}
		return nil, ctx.Err()
	}
}

// GetWorkflow (client) reads a run's current status and result.
func (e *Engine) GetWorkflow(ctx context.Context, req *pb.GetWorkflowRequest) (*pb.GetWorkflowResponse, error) {
	run, ok := e.store.GetRun(req.GetWorkflowId())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "workflow %q not found", req.GetWorkflowId())
	}
	return &pb.GetWorkflowResponse{Status: run.Status, Result: run.Result}, nil
}

// newID returns a random 128-bit hex identifier for workflow runs.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
