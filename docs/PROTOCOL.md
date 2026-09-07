# FlowService Protocol Reference

The engine implements `FlowService` from
[`joblet-proto/proto/flow/flow.proto`](../../joblet-proto/proto/flow/flow.proto)
(package `jobletflow`). Every SDK generates its client from that file. All
payload fields (`input`, `result`, side-effect `data`, signal `payload`) are
**opaque bytes** the engine never interprets; SDKs agree on a codec (JSON in v1).

## RPCs

| RPC                                 | Caller          | Summary                                                        |
|-------------------------------------|-----------------|----------------------------------------------------------------|
| `StartWorkflow`                     | client          | Begin a run; idempotent on `workflow_id`.                      |
| `PollTask`                          | worker          | Long-poll for the next task on a queue.                        |
| `CompleteTask`                      | worker          | Record a run's successful result.                              |
| `FailTask`                          | worker          | Record a run's terminal failure.                               |
| `RunActivity`                       | handler         | Run an activity as an isolated joblet job; memoized by `step`. |
| `RunWorkerActivity`                 | handler         | Run a named activity on a worker; memoized by `step`.          |
| `CompleteActivity` / `FailActivity` | activity worker | Report a worker-activity result.                               |
| `GetSideEffect`                     | handler         | Read a recorded side-effect result.                            |
| `RecordSideEffect`                  | handler         | Store a side-effect result.                                    |
| `SignalWorkflow`                    | client          | Deliver an external signal to a workflow.                      |
| `WaitSignal`                        | handler         | Block until a named signal arrives; memoized by `step`.        |
| `GetWorkflow`                       | client          | Read status/result of a run.                                   |

### StartWorkflow

`StartWorkflowRequest{ workflow, workflow_id, input }` →
`StartWorkflowResponse{ workflow_id }`

Creates a run and enqueues a `WORKFLOW` task on the default queue. If
`workflow_id` is empty the engine generates one. A repeat call with an existing
`workflow_id` returns the same id **without** enqueuing again (idempotency).
Errors: `InvalidArgument` if `workflow` is empty.

### PollTask

`PollTaskRequest{ queue }` → `PollTaskResponse{ task, available }`

Long-polls. Blocks until a task is available or shortly before the request
deadline, then returns `available=false`. Empty `queue` means `default`.

### CompleteTask / FailTask

`CompleteTaskRequest{ workflow_id, handler, result }` → `{}`
`FailTaskRequest{ workflow_id, handler, error }` → `{}`

Move a run to `COMPLETED` (with `result`) or `FAILED` (with `error`). Errors:
`InvalidArgument` if `workflow_id` is empty.

### RunActivity

`RunActivityRequest{ workflow_id, name, job, step, retry }` →
`RunActivityResponse{ exit_code, stdout, stderr, status, job_uuid }`

Dispatches `job` (a `FlowJobSpec`) as a joblet job and blocks until terminal.
Memoized by `(workflow_id, step)`: a repeat of the same step returns the recorded
result without re-dispatching. Applies `retry` on non-`COMPLETED` outcomes.
Errors: `InvalidArgument` (empty `workflow_id`, nil `job`); `Internal` on a
dispatch/transport failure with no result.

### RunWorkerActivity + CompleteActivity / FailActivity

`RunWorkerActivityRequest{ workflow_id, name, input, step, queue, retry }` →
`RunWorkerActivityResponse{ result, error }`

Runs a named activity on an **activity worker** and blocks until it finishes -
the counterpart to `RunActivity` (isolated joblet job) for lightweight worker-
side work such as LLM calls. The engine enqueues a `TASK_TYPE_ACTIVITY` task,
waits for the worker's report, applies `retry`, and memoizes by
`(workflow_id, step)`. `error` is non-empty if the activity ultimately failed.

The activity worker polls the task via `PollTask` (getting a `Task` with
`type = TASK_TYPE_ACTIVITY`, `handler`, `input`, `step`), runs the registered
handler, and reports:

- `CompleteActivityRequest{ workflow_id, step, result }` → `{}`
- `FailActivityRequest{ workflow_id, step, error }` → `{}` (engine retries)

Because the workflow handler blocks while the activity runs, a worker must poll
and execute tasks **concurrently** (the Python SDK uses a thread pool) so it does
not starve the activity it is waiting on.

### GetSideEffect / RecordSideEffect

`GetSideEffectRequest{ workflow_id, step }` → `{ data, found }`
`RecordSideEffectRequest{ workflow_id, step, data }` → `{}`

Read/store an opaque side-effect result keyed by `(workflow_id, step)`. Workers
call `GetSideEffect` on replay before re-executing a non-deterministic step.

### SignalWorkflow / WaitSignal

`SignalWorkflowRequest{ workflow_id, name, payload }` → `{}`
`WaitSignalRequest{ workflow_id, name, step }` → `WaitSignalResponse{ payload }`

`SignalWorkflow` (client) delivers a named signal. `WaitSignal` (handler) blocks
until a signal of that name arrives, then returns its payload - the human-in-the-
loop primitive. Ordering is handled either way: a signal that arrives before the
wait is **buffered** (FIFO per workflow+name); a wait that arrives first blocks.
Memoized by `step`, so a replay returns the recorded payload instead of consuming
another signal.

### GetWorkflow

`GetWorkflowRequest{ workflow_id }` → `GetWorkflowResponse{ status, result }`

Returns the run's current `status` and `result`. Errors: `NotFound` if unknown.

## Messages

**Task** `{ workflow_id, type, handler, input, job }` - `type` is `TaskType`
(`TASK_TYPE_WORKFLOW` | `TASK_TYPE_ACTIVITY`); `job` is set for activity tasks.

**FlowJobSpec** `{ runtime, command, args, env, resources, node, volumes }` - the
isolated joblet job backing an activity. `volumes` are joblet volumes mounted at
`/volumes/<name>` (so activities can reach a codebase or persist artifacts).
`node` selects a target joblet (not yet honored).

**FlowResources** `{ max_cpu (percent), max_memory (MB), gpu_count }` - joblet
cgroup limits applied to the activity's job.

**RetryPolicy** `{ max_attempts, backoff, base_ms }` - `max_attempts` 0/1 = no
retry; `backoff` `"exponential"` (default) | `"fixed"`; `base_ms` default 100.

## Status values

Workflow run status (via `GetWorkflow`): `RUNNING`, `COMPLETED`, `FAILED`.

Activity `status` mirrors joblet's terminal job status: `COMPLETED` (success),
`FAILED`, `STOPPED`, `TIMEOUT`.
