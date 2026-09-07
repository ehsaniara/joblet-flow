# Development

## Layout

```
cmd/joblet-flow        engine entrypoint: config, joblet dial, gRPC server
internal/engine        FlowService impl, Store interface, in-memory store, dispatch
internal/jobletclient  activity job runner + mTLS dial to joblet
internal/config        environment configuration
scripts/pre-pr-check.sh   pre-PR pipeline
```

## Make targets

| Target        | Purpose                                |
|---------------|----------------------------------------|
| `make build`  | Build the engine → `bin/joblet-flow`   |
| `make run`    | Run the engine (mTLS to joblet)        |
| `make test`   | Unit tests                             |
| `make pre-pr` | Gate: fmt + vet + tidy + tests + build |
| `make clean`  | Remove `bin/`                          |

## Tests

**Unit** (`go test ./...`) - engine logic via a fake `JobRunner` (no joblet):
start/idempotency, poll (hit + clean timeout), activity memoization + retry
(job and worker activities), signals (buffer/block/replay), side effects,
complete/fail/get; plus mTLS config building from a generated cert.

**End-to-end** - there is no engine-repo e2e. Integration is exercised by
[`joblet-flow-sdk-python`](../../joblet-flow-sdk-python)'s e2e, which runs a real
worker against a real engine and a real joblet node and asserts workflow results.

## CLI

joblet-flow has no CLI of its own - **`rnx`** (the joblet CLI) is the client for
the ecosystem; applications use the language SDK. Flow-specific commands are
expected to be added to `rnx`.

## Adding a durable store

The engine depends on `engine.Store` (see `internal/engine/store.go`). To add
persistence, implement that interface (runs, task queues, activity/side-effect
memo) over SQLite/Bolt and construct the engine with it in `cmd/joblet-flow`.
No other engine code changes.

## Related repos

- [`joblet-proto`](../../joblet-proto) - the gRPC contract (`flow.proto`).
- [`joblet`](../../joblet) - the job execution platform activities run on.
- [`joblet-flow-sdk-python`](../../joblet-flow-sdk-python) - Python worker SDK.
- `joblet-flow-sdk-node` - Node worker SDK (planned).
