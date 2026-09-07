# joblet-flow

The **joblet-flow engine** - a language-agnostic durable workflow orchestrator.
It owns control flow; SDK workers (`joblet-flow-sdk-python`, `-node`, …) run the
actual workflow and activity code. An **activity is an isolated joblet job**: the
engine dispatches it through joblet's `JobService` and records its result.

The gRPC contract is [`joblet-proto/proto/flow/flow.proto`](../joblet-proto/proto/flow/flow.proto)
(`FlowService`). Every SDK generates its client from that file.

📖 **Docs:** [Architecture / system design](docs/ARCHITECTURE.md) ·
[Protocol reference](docs/PROTOCOL.md) · [Configuration](docs/CONFIGURATION.md) ·
[Development](docs/DEVELOPMENT.md)

## What the engine does (and doesn't)

**Owns - logic and orchestration only:**
- Durable workflow state keyed by `workflow_id`
- Task queues that workers long-poll (`PollTask`)
- Dispatch of activities as joblet jobs (`RunActivity` → joblet `RunJob`, wait
  for terminal status, capture logs)
- Step-indexed memoization of activity + side-effect results → deterministic
  replay and idempotency
- Retry policy, signals, `workflow_id` idempotency

**Never touches:**
- Workflow / activity handler code - that runs in the SDK worker
- Serialization - all inputs, results, and side-effect payloads are opaque
  `bytes`; SDKs agree on a codec (JSON for v1)
- Any language toolchain

## Architecture

```
 Client            joblet-flow engine (this repo)        SDK worker
 ------            ------------------------------        ----------
 StartWorkflow ───▶ enqueue workflow task
                                            ◀─ PollTask ─ long-poll
                    hand out task ──────────────────────▶ run workflow handler
                                            ◀─ RunActivity(step, jobspec)
                    joblet RunJob ─▶ isolated job
                    wait terminal, capture logs
                                   ── result ───────────▶ handler continues
                                            ◀─ CompleteTask / FailTask
 GetWorkflow  ◀──── status + result
```

## FlowService RPCs

| RPC | Caller | Purpose |
|-----|--------|---------|
| `StartWorkflow` | client | begin a run (idempotent on `workflow_id`) |
| `PollTask` | worker | long-poll for the next task |
| `CompleteTask` / `FailTask` | worker | report the run's terminal result |
| `RunActivity` | handler | run an activity as a joblet job, memoized by `step` |
| `GetSideEffect` / `RecordSideEffect` | handler | durable non-deterministic steps |
| `SignalWorkflow` | client | deliver an external signal |
| `GetWorkflow` | client | read status / result |

## Layout

```
cmd/joblet-flow        entrypoint: config, joblet dial, gRPC server
internal/engine        FlowService implementation, Store interface, in-memory store
internal/jobletclient  activity job runner over joblet's JobService
internal/config        environment configuration
```

## Build & run

```bash
make test         # unit tests (no live joblet needed)
make build        # -> bin/joblet-flow
make pre-pr       # fmt + vet + tidy + tests + build

# mTLS to joblet (default): reads a node from rnx-config.yml
FLOW_LISTEN_ADDR=:50055 JOBLET_CONFIG=~/.rnx/rnx-config.yml JOBLET_NODE=default make run

# insecure to joblet (dev only)
JOBLET_INSECURE=1 JOBLET_ADDR=localhost:50051 make run
```

### Connecting to joblet

The engine dials joblet over **mTLS by default**, loading a node's embedded
`cert`/`key`/`ca` from an `rnx-config.yml` (the same file rnx uses), with
`ServerName: joblet` and TLS 1.3.

| Env | Default | Purpose |
|-----|---------|---------|
| `FLOW_LISTEN_ADDR` | `:50055` | FlowService listen address |
| `JOBLET_CONFIG` | *(standard search paths)* | explicit `rnx-config.yml` |
| `JOBLET_NODE` | *(isDefault node)* | node entry to dial; empty uses the node marked `isDefault: true` |
| `JOBLET_INSECURE` | *(off)* | `1` = plaintext dial (dev/e2e), uses `JOBLET_ADDR` |
| `JOBLET_ADDR` | `localhost:50051` | target in insecure mode |

## CLI & end-to-end tests

There is no separate CLI for joblet-flow - **`rnx`** (the joblet CLI) is the
client for the whole ecosystem. Applications use the language SDK
([`joblet-flow-sdk-python`](../joblet-flow-sdk-python)).

End-to-end integration (a real worker + engine + real joblet node) is exercised
by the **SDK's e2e** (`joblet-flow-sdk-python/tests/e2e`), which runs actual
workflows and asserts real results. The engine itself is covered here by unit
tests (`make test`).

Suites: `01_lifecycle` (orchestration) and `02_activity` (dispatches a real
`echo` job to joblet and asserts its stdout/exit + step memoization). The
activity suite skips only on the `JOBLET_INSECURE` dev path - like the joblet
GPU suites skip without hardware.

## Status

Walking skeleton. The full FlowService is implemented over an **in-memory**
`Store` (state is lost on restart). Done: all 8 RPCs, step memoization, retry,
long-poll, and **mTLS activity dispatch to a real joblet** (verified e2e). Not
yet done:

- **Durable store** (SQLite/Bolt) behind the existing `Store` interface - 
  required for real durability and crash recovery
- **Signal delivery** into a replaying workflow, and multi-node activity routing
  (`FlowJobSpec.node`)
- **At-least-once task delivery** - the in-memory queue can drop a task if a
  worker's poll is cancelled at the instant a task is handed out (needs
  lease/ack); fine for the skeleton
- The **Python** and **Node** SDK workers
