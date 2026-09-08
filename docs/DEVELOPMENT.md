# Development

## Layout

```
cmd/joblet-flow           engine entrypoint: config, joblet dial, gRPC server
internal/engine           FlowService impl, Store interface, in-memory store, dispatch
internal/jobletclient     activity job runner + mTLS dial to joblet
internal/config           environment configuration
debian/                   .deb control files (postinst enables the service)
scripts/build-deb.sh      package the engine (per-arch, verified binaries)
scripts/get-joblet.sh     download the latest released joblet .deb
scripts/joblet-flow.service  systemd unit (loopback listener)
scripts/pre-pr-check.sh   pre-PR pipeline
tests/e2e/                clean-room e2e suites and the test driver
```

## Make targets

| Target        | Purpose                                              |
|---------------|------------------------------------------------------|
| `make build`  | Build the engine → `bin/joblet-flow`                 |
| `make run`    | Run the engine (mTLS to joblet)                      |
| `make test`   | Unit tests, cache disabled                           |
| `make deb`    | Build the `.deb` from the working tree               |
| `make e2e`    | Clean-room e2e against the installed service (sudo)  |
| `make pre-pr` | Gate: unit tests + e2e, structurally identical to joblet's |
| `make clean`  | Remove build artifacts                               |

## Tests

**Unit** (`make test`) - engine logic via a fake `JobRunner` (no joblet):
start/idempotency, poll (hit + clean timeout), activity memoization + retry
(job and worker activities), signals (buffer/block/replay), side effects,
complete/fail/get; plus mTLS config building from a generated cert.

**End-to-end** (`make e2e`, needs sudo) - clean-room on this host:

1. Uninstall joblet-flow AND joblet completely (bridge included)
2. Install the latest released joblet from GitHub
3. Build and install joblet-flow from the working tree as a `.deb`
4. Run every `tests/e2e/tests/*.sh` fail-fast against the installed service

Suites drive `tests/e2e/driver` (a test-only binary speaking the `FlowService`
contract, playing both the client and worker roles) and cross-check activity
jobs from joblet's side via the installed rnx. A running joblet on the same
host is required; the runner installs one.

SDK-level integration (a real language worker against the engine) lives in
each SDK repo's own e2e.

## CLI

joblet-flow has no CLI of its own - applications use the language SDKs, and
the operator surface is designed as an `rnx flow` command group
([RNX_FLOW_CLI.md](RNX_FLOW_CLI.md), to implement in joblet-rnx).

## Adding a durable store

The engine depends on `engine.Store` (see `internal/engine/store.go`). To add
persistence, implement that interface (runs, task queues, activity/side-effect
memo) over SQLite/Bolt and construct the engine with it in `cmd/joblet-flow`.
No other engine code changes.

## Related repos

- [`joblet-proto`](../../joblet-proto) - the gRPC contract (`flow.proto`).
- [`joblet`](../../joblet) - the job execution platform activities run on.
- [`joblet-flow-sdk-python`](../../joblet-flow-sdk-python) - Python SDK (client + worker).
- [`joblet-flow-sdk-node`](../../joblet-flow-sdk-node) - Node SDK (client + worker).
