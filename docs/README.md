# joblet-flow Documentation

- **[ARCHITECTURE](ARCHITECTURE.md)** - system design: components, workflow
  lifecycle, the execution model (loops, DAGs, cycles), determinism & replay,
  activity dispatch, retry, state, mTLS, deployment, limitations, roadmap.
- **[PROTOCOL](PROTOCOL.md)** - `FlowService` RPC reference and messages.
- **[CONFIGURATION](CONFIGURATION.md)** - environment variables and mTLS to
  joblet.
- **[DEVELOPMENT](DEVELOPMENT.md)** - layout, make targets, tests, CLI (rnx),
  adding a durable store.
- **[RNX_FLOW_CLI](RNX_FLOW_CLI.md)** - design for the `rnx flow` command group
  (to implement in joblet-rnx, where rnx lives).
- **[COMPATIBILITY](../COMPATIBILITY.md)** - proto and joblet version
  compatibility.

New here? Start with the [top-level README](../README.md) for the overview and
quick start, then read [ARCHITECTURE](ARCHITECTURE.md).
