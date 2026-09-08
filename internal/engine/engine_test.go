package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/ehsaniara/joblet-proto/v2/gen/flow"
	"google.golang.org/protobuf/proto"
)

// fakeRunner is an in-memory JobRunner: it returns scripted responses and
// counts invocations, so engine behavior can be tested without a live joblet.
type fakeRunner struct {
	calls   int32
	fails   int32 // return FAILED for the first N calls, then COMPLETED
	resp    *pb.RunActivityResponse
	lastErr error
}

func (f *fakeRunner) Run(ctx context.Context, name string, spec *pb.FlowJobSpec) (*pb.RunActivityResponse, error) {
	n := atomic.AddInt32(&f.calls, 1)
	if f.lastErr != nil {
		return nil, f.lastErr
	}
	status := StatusCompleted
	if n <= f.fails {
		status = StatusFailed
	}
	if f.resp != nil {
		cp := proto.Clone(f.resp).(*pb.RunActivityResponse)
		cp.Status = status
		return cp, nil
	}
	return &pb.RunActivityResponse{Status: status, ExitCode: 0, JobUuid: "job-x"}, nil
}

func newEngine(r JobRunner) *Engine { return New(NewMemStore(), r, nil) }

func TestStartWorkflow_EnqueuesAndIsIdempotent(t *testing.T) {
	e := newEngine(&fakeRunner{})
	ctx := context.Background()

	start, err := e.StartWorkflow(ctx, &pb.StartWorkflowRequest{Workflow: "greet", WorkflowId: "wf-1", Input: []byte("hi")})
	if err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}
	if start.GetWorkflowId() != "wf-1" {
		t.Fatalf("workflow_id = %q, want wf-1", start.GetWorkflowId())
	}

	// A worker polls and receives the enqueued workflow task.
	poll, err := e.PollTask(ctx, &pb.PollTaskRequest{})
	if err != nil {
		t.Fatalf("PollTask: %v", err)
	}
	if !poll.GetAvailable() || poll.GetTask().GetHandler() != "greet" {
		t.Fatalf("poll = %+v, want available greet task", poll)
	}
	if poll.GetTask().GetType() != pb.TaskType_TASK_TYPE_WORKFLOW {
		t.Fatalf("task type = %v, want WORKFLOW", poll.GetTask().GetType())
	}

	// Re-starting the same id must not enqueue a second task.
	if _, err := e.StartWorkflow(ctx, &pb.StartWorkflowRequest{Workflow: "greet", WorkflowId: "wf-1"}); err != nil {
		t.Fatalf("idempotent StartWorkflow: %v", err)
	}
	pctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if poll, _ := e.PollTask(pctx, &pb.PollTaskRequest{}); poll.GetAvailable() {
		t.Fatalf("expected no second task for idempotent start")
	}
}

func TestPollTask_EmptyQueueReturnsCleanMiss(t *testing.T) {
	e := newEngine(&fakeRunner{})
	// A deadline'd poll on an empty queue must return a clean available=false,
	// never a context error, thanks to the server-side deadline margin.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	resp, err := e.PollTask(ctx, &pb.PollTaskRequest{})
	if err != nil {
		t.Fatalf("PollTask returned error, want clean miss: %v", err)
	}
	if resp.GetAvailable() {
		t.Fatalf("available=true on empty queue")
	}
}

func TestRunActivity_MemoizesByStep(t *testing.T) {
	fr := &fakeRunner{}
	e := newEngine(fr)
	ctx := context.Background()

	req := &pb.RunActivityRequest{
		WorkflowId: "wf-1",
		Name:       "fetch",
		Step:       0,
		Job:        &pb.FlowJobSpec{Command: "echo", Args: []string{"hi"}},
	}
	first, err := e.RunActivity(ctx, req)
	if err != nil {
		t.Fatalf("RunActivity: %v", err)
	}
	if first.GetStatus() != StatusCompleted {
		t.Fatalf("status = %q, want COMPLETED", first.GetStatus())
	}

	// Replaying the same step returns the memo without dispatching again.
	if _, err := e.RunActivity(ctx, req); err != nil {
		t.Fatalf("replay RunActivity: %v", err)
	}
	if got := atomic.LoadInt32(&fr.calls); got != 1 {
		t.Fatalf("runner called %d times, want 1 (memoized)", got)
	}
}

func TestRunActivity_RetriesUntilCompleted(t *testing.T) {
	fr := &fakeRunner{fails: 2} // fail twice, succeed on the third attempt
	e := newEngine(fr)

	resp, err := e.RunActivity(context.Background(), &pb.RunActivityRequest{
		WorkflowId: "wf-1",
		Name:       "flaky",
		Job:        &pb.FlowJobSpec{Command: "true"},
		Retry:      &pb.RetryPolicy{MaxAttempts: 3, Backoff: "fixed", BaseMs: 1},
	})
	if err != nil {
		t.Fatalf("RunActivity: %v", err)
	}
	if resp.GetStatus() != StatusCompleted {
		t.Fatalf("status = %q, want COMPLETED", resp.GetStatus())
	}
	if got := atomic.LoadInt32(&fr.calls); got != 3 {
		t.Fatalf("runner called %d times, want 3", got)
	}
}

// pollActivity long-polls one task off the default queue.
func pollActivity(t *testing.T, e *Engine, timeout time.Duration) *pb.Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	resp, err := e.PollTask(ctx, &pb.PollTaskRequest{})
	if err != nil || !resp.GetAvailable() {
		t.Fatalf("PollTask: available=%v err=%v", resp.GetAvailable(), err)
	}
	return resp.GetTask()
}

func TestRunWorkerActivity_DispatchCompleteMemoize(t *testing.T) {
	e := newEngine(&fakeRunner{})
	type res struct {
		resp *pb.RunWorkerActivityResponse
		err  error
	}
	done := make(chan res, 1)
	go func() {
		r, err := e.RunWorkerActivity(context.Background(), &pb.RunWorkerActivityRequest{
			WorkflowId: "wf-1", Name: "summarize", Input: []byte("in"), Step: 0,
		})
		done <- res{r, err}
	}()

	task := pollActivity(t, e, 2*time.Second)
	if task.GetType() != pb.TaskType_TASK_TYPE_ACTIVITY || task.GetHandler() != "summarize" || task.GetStep() != 0 {
		t.Fatalf("unexpected activity task: %+v", task)
	}
	if string(task.GetInput()) != "in" {
		t.Fatalf("task input = %q, want in", task.GetInput())
	}

	if _, err := e.CompleteActivity(context.Background(), &pb.CompleteActivityRequest{WorkflowId: "wf-1", Step: 0, Result: []byte("out")}); err != nil {

		t.Fatalf("setup: %v", err)

	}

	got := <-done
	if got.err != nil {
		t.Fatalf("RunWorkerActivity: %v", got.err)
	}
	if string(got.resp.GetResult()) != "out" {
		t.Fatalf("result = %q, want out", got.resp.GetResult())
	}

	// Replay of the same step is memoized: returns immediately, enqueues nothing.
	r2, err := e.RunWorkerActivity(context.Background(), &pb.RunWorkerActivityRequest{WorkflowId: "wf-1", Name: "summarize", Step: 0})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if string(r2.GetResult()) != "out" {
		t.Fatalf("memo result = %q, want out", r2.GetResult())
	}
	pctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if p, _ := e.PollTask(pctx, &pb.PollTaskRequest{}); p.GetAvailable() {
		t.Fatalf("unexpected task after memoized replay")
	}
}

func TestRunWorkerActivity_RetriesThenCompletes(t *testing.T) {
	e := newEngine(&fakeRunner{})
	done := make(chan *pb.RunWorkerActivityResponse, 1)
	go func() {
		r, _ := e.RunWorkerActivity(context.Background(), &pb.RunWorkerActivityRequest{
			WorkflowId: "wf-1", Name: "flaky", Step: 0,
			Retry: &pb.RetryPolicy{MaxAttempts: 2, Backoff: "fixed", BaseMs: 1},
		})
		done <- r
	}()

	pollActivity(t, e, 2*time.Second) // attempt 1
	if _, err := e.FailActivity(context.Background(), &pb.FailActivityRequest{WorkflowId: "wf-1", Step: 0, Error: "boom"}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	pollActivity(t, e, 2*time.Second) // attempt 2 (re-dispatched)
	if _, err := e.CompleteActivity(context.Background(), &pb.CompleteActivityRequest{WorkflowId: "wf-1", Step: 0, Result: []byte("ok")}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	r := <-done
	if string(r.GetResult()) != "ok" || r.GetError() != "" {
		t.Fatalf("resp = %+v, want result ok / no error", r)
	}
}

func TestRunWorkerActivity_FailsAfterRetries(t *testing.T) {
	e := newEngine(&fakeRunner{})
	done := make(chan *pb.RunWorkerActivityResponse, 1)
	go func() {
		r, _ := e.RunWorkerActivity(context.Background(), &pb.RunWorkerActivityRequest{
			WorkflowId: "wf-1", Name: "always", Step: 0,
			Retry: &pb.RetryPolicy{MaxAttempts: 2, Backoff: "fixed", BaseMs: 1},
		})
		done <- r
	}()

	pollActivity(t, e, 2*time.Second)
	if _, err := e.FailActivity(context.Background(), &pb.FailActivityRequest{WorkflowId: "wf-1", Step: 0, Error: "boom"}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	pollActivity(t, e, 2*time.Second)
	if _, err := e.FailActivity(context.Background(), &pb.FailActivityRequest{WorkflowId: "wf-1", Step: 0, Error: "boom-final"}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	r := <-done
	if r.GetError() == "" {
		t.Fatalf("expected error after exhausting retries, got %+v", r)
	}
}

func TestWaitSignal_BufferedBeforeWait(t *testing.T) {
	e := newEngine(&fakeRunner{})
	ctx := context.Background()

	// Signal arrives before the workflow waits - it must be buffered.
	if _, err := e.SignalWorkflow(ctx, &pb.SignalWorkflowRequest{WorkflowId: "wf-1", Name: "approve", Payload: []byte("yes")}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	resp, err := e.WaitSignal(ctx, &pb.WaitSignalRequest{WorkflowId: "wf-1", Name: "approve", Step: 0})
	if err != nil {
		t.Fatalf("WaitSignal: %v", err)
	}
	if string(resp.GetPayload()) != "yes" {
		t.Fatalf("payload = %q, want yes", resp.GetPayload())
	}
}

func TestWaitSignal_BlocksThenDelivered(t *testing.T) {
	e := newEngine(&fakeRunner{})
	done := make(chan []byte, 1)
	go func() {
		resp, _ := e.WaitSignal(context.Background(), &pb.WaitSignalRequest{WorkflowId: "wf-1", Name: "approve", Step: 0})
		done <- resp.GetPayload()
	}()

	// Delivered whether it buffers (waiter not yet registered) or hands off directly.
	if _, err := e.SignalWorkflow(context.Background(), &pb.SignalWorkflowRequest{WorkflowId: "wf-1", Name: "approve", Payload: []byte("go")}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	select {
	case p := <-done:
		if string(p) != "go" {
			t.Fatalf("payload = %q, want go", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("WaitSignal did not return")
	}
}

func TestWaitSignal_MemoizesByStep(t *testing.T) {
	e := newEngine(&fakeRunner{})
	ctx := context.Background()

	if _, err := e.SignalWorkflow(ctx, &pb.SignalWorkflowRequest{WorkflowId: "wf-1", Name: "s", Payload: []byte("first")}); err != nil {

		t.Fatalf("setup: %v", err)

	}
	r1, _ := e.WaitSignal(ctx, &pb.WaitSignalRequest{WorkflowId: "wf-1", Name: "s", Step: 0})
	if string(r1.GetPayload()) != "first" {
		t.Fatalf("r1 = %q, want first", r1.GetPayload())
	}

	// A second signal must not be consumed by a replay of step 0 (memoized).
	if _, err := e.SignalWorkflow(ctx, &pb.SignalWorkflowRequest{WorkflowId: "wf-1", Name: "s", Payload: []byte("second")}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	r2, _ := e.WaitSignal(ctx, &pb.WaitSignalRequest{WorkflowId: "wf-1", Name: "s", Step: 0})
	if string(r2.GetPayload()) != "first" {
		t.Fatalf("replay r2 = %q, want first (memoized)", r2.GetPayload())
	}
	// A wait at a new step consumes the buffered second signal.
	r3, _ := e.WaitSignal(ctx, &pb.WaitSignalRequest{WorkflowId: "wf-1", Name: "s", Step: 1})
	if string(r3.GetPayload()) != "second" {
		t.Fatalf("r3 = %q, want second", r3.GetPayload())
	}
}

func TestSideEffect_RecordThenGet(t *testing.T) {
	e := newEngine(&fakeRunner{})
	ctx := context.Background()

	miss, _ := e.GetSideEffect(ctx, &pb.GetSideEffectRequest{WorkflowId: "wf-1", Step: 5})
	if miss.GetFound() {
		t.Fatalf("expected side-effect miss before record")
	}

	if _, err := e.RecordSideEffect(ctx, &pb.RecordSideEffectRequest{WorkflowId: "wf-1", Step: 5, Data: []byte("llm-reply")}); err != nil {
		t.Fatalf("RecordSideEffect: %v", err)
	}
	hit, _ := e.GetSideEffect(ctx, &pb.GetSideEffectRequest{WorkflowId: "wf-1", Step: 5})
	if !hit.GetFound() || string(hit.GetData()) != "llm-reply" {
		t.Fatalf("side-effect = %+v, want found llm-reply", hit)
	}
}

func TestCompleteAndGetWorkflow(t *testing.T) {
	e := newEngine(&fakeRunner{})
	ctx := context.Background()

	if _, err := e.StartWorkflow(ctx, &pb.StartWorkflowRequest{Workflow: "greet", WorkflowId: "wf-1"}); err != nil {

		t.Fatalf("setup: %v", err)

	}
	if _, err := e.CompleteTask(ctx, &pb.CompleteTaskRequest{WorkflowId: "wf-1", Result: []byte("done")}); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	got, err := e.GetWorkflow(ctx, &pb.GetWorkflowRequest{WorkflowId: "wf-1"})
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if got.GetStatus() != StatusCompleted || string(got.GetResult()) != "done" {
		t.Fatalf("workflow = %+v, want COMPLETED done", got)
	}

	if _, err := e.GetWorkflow(ctx, &pb.GetWorkflowRequest{WorkflowId: "missing"}); err == nil {
		t.Fatalf("expected NotFound for unknown workflow")
	}
}
