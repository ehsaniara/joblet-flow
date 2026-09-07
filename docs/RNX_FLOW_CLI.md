# Design: `rnx flow` command group

How joblet-flow is exposed through **`rnx`** (the joblet CLI) instead of a
separate `flowctl`. This is a design to implement in the **joblet** repo (where
rnx lives); the FlowService contract lives in `joblet-proto`.

## Goal & scope

`rnx` is the single client for the ecosystem. Add a `flow` command group for the
**client-facing** `FlowService` RPCs - the operations an operator or app-starter
performs. Worker-facing RPCs (`PollTask`, `CompleteTask`/`FailTask`,
`RunActivity`, `RunWorkerActivity`, `CompleteActivity`/`FailActivity`,
`WaitSignal`, `GetSideEffect`/`RecordSideEffect`) are **not** exposed - those are
used only by SDK workers, never by a CLI.

| Command                       | RPC              | Purpose                    |
|-------------------------------|------------------|----------------------------|
| `rnx flow start <workflow>`   | `StartWorkflow`  | begin a run; prints its id |
| `rnx flow status <id>`        | `GetWorkflow`    | show status + result       |
| `rnx flow signal <id> <name>` | `SignalWorkflow` | deliver a signal           |

Deferred until the matching RPCs exist (see [new RPCs](#new-rpcs-deferred)):
`rnx flow list`, `rnx flow cancel`, `rnx flow describe` (history).

## Connection & config

The flow engine is a distinct endpoint. Add an optional `flow` address to each
node in `rnx-config.yml` (the same file rnx already loads):

```yaml
nodes:
  admin:
    isDefault: true  # used when no node name is specified
    address: "10.0.0.10:50051"   # joblet JobService (existing)
    flow:    "10.0.0.10:50055"   # joblet-flow FlowService (new, optional)
    cert: | ...
    key:  | ...
    ca:   | ...
```

- **Config change** (`pkg/config.Node`): add `Flow string \`yaml:"flow"\``.
  Backward-compatible; nodes without `flow` simply can't run `rnx flow`.
- **Transport, v1: plaintext.** The engine↔joblet link is mTLS, but the
  engine↔client (SDK/rnx) link is plaintext today, so `rnx flow` dials `flow`
  with `insecure` credentials. If `flow` is unset, fail with:
  `node "<name>" has no flow endpoint (add 'flow: host:50055' to rnx-config.yml)`.
- **Transport, later: mTLS.** When the engine serves TLS, reuse the node's
  `cert`/`key`/`ca` (add `flowCert`/`flowKey`/`flowCa` only if the flow engine
  uses a different CA). The client builder should branch on cert presence.

## Client wiring

Mirror `common.NewJobClient()` with a flow client. Two options; prefer the first:

1. **`pkg/client.FlowClient`** - a small wrapper like `JobClient`, holding a
   `pb.FlowServiceClient` and exposing `StartWorkflow`, `GetWorkflow`,
   `SignalWorkflow`. Built from a `*config.Node` (dials `node.Flow`).
2. `common.NewFlowClient()` in `internal/rnx/common` returning the raw
   `pb.FlowServiceClient` + conn.

```go
// common
func NewFlowClient() (*client.FlowClient, error) {
    node, err := NodeConfig.GetNode(NodeName)
    if err != nil { return nil, err }
    if node.Flow == "" { return nil, fmt.Errorf("node %q has no flow endpoint ...", NodeName) }
    return client.NewFlowClient(node)   // insecure v1; mTLS when available
}
```

## Package layout (in joblet repo)

```
internal/rnx/flow/
  flow_cmd.go   NewFlowCmd() - the `flow` parent command
  start.go      rnx flow start
  status.go     rnx flow status
  signal.go     rnx flow signal
pkg/client/flow.go   FlowClient wrapper (or extend pkg/client)
```

Wire it in `internal/rnx/cli/root.go`:
`rootCmd.AddCommand(flow.NewFlowCmd())`. It inherits the global `--config`,
`--node`, `--json` flags and the `PersistentPreRunE` config load.

## Command details

### `rnx flow start <workflow>`

Flags: `--id` (caller-supplied id, idempotent), `--input <json>` /
`--input-file <path>`, `--queue`.
Payloads are **opaque bytes**; rnx passes the JSON string through unchanged (SDKs
use JSON). Prints `workflow_id`; `--json` → `{"workflow_id":"..."}`.

```
$ rnx flow start greet --input '{"name":"world"}'
workflow_id: 8f2c...
```

### `rnx flow status <id>`

Prints status and (raw) result; `--json` emits `{workflow_id, status, result}`.

```
$ rnx flow status 8f2c
Workflow: 8f2c...
Status:   COMPLETED
Result:   {"greeting":"hello world"}
```

### `rnx flow signal <id> <name>`

Flags: `--payload <json>` / `--payload-file`. Prints nothing on success (or `ok`).

```
$ rnx flow signal 8f2c approve --payload '{"approved":true}'
```

## Output conventions

Honor the global `--json` flag (as the `job`/`monitor` groups do): human-readable
key/value by default, structured JSON when `--json`. Result/payload bytes are
emitted raw (they are already JSON from SDKs).

## New RPCs (deferred)

These commands need FlowService additions before implementation:

| Command                  | Needs RPC                           |
|--------------------------|-------------------------------------|
| `rnx flow list`          | `ListWorkflows` (filter by status)  |
| `rnx flow cancel <id>`   | `CancelWorkflow` / terminate        |
| `rnx flow describe <id>` | `GetWorkflowHistory` (events/steps) |

Add these to `joblet-proto/proto/flow/flow.proto` when the engine implements them
(they also benefit the SDKs).

## Compatibility

`FlowService` already ships in `joblet-proto` v2, so rnx (which imports the gen
package) gets the client stubs with no proto bump. The `Node.flow` field is
additive. Track versions per the proto's [COMPATIBILITY.md](../../joblet-proto/COMPATIBILITY.md).

## Implementation checklist

1. `pkg/config.Node`: add `Flow` field (+ optional flow certs later).
2. `pkg/client`: `FlowClient` wrapper (Start/Get/Signal), insecure dial to `node.Flow`.
3. `internal/rnx/common`: `NewFlowClient()`.
4. `internal/rnx/flow`: `flow_cmd.go` + `start.go` + `status.go` + `signal.go`.
5. Register in `internal/rnx/cli/root.go`.
6. Tests mirroring `internal/rnx/*/commands_test.go`; docs in joblet's
   `RNX_CLI_REFERENCE.md`.
