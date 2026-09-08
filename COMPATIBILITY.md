# Compatibility

joblet-flow speaks the `joblet-proto` gRPC contract on both sides: it serves
`FlowService` (from `flow.proto`) to SDK workers and clients, and it is a
`JobService` client of the joblet server. The **proto major** is the
compatibility boundary, and release versions are tracked by **git tag**; treat
the tag as authoritative.

## joblet-flow ↔ proto

`flow.proto` is first published in joblet-proto **v2.6.0**; joblet-flow
requires that tag or later.

| joblet-flow           | joblet-proto |
|-----------------------|--------------|
| **v0.x** (unreleased) | v2.6.0+      |

## joblet-flow ↔ joblet server

The engine talks to joblet only through the `JobService` contract, so any
joblet release on proto v2.x works (joblet server v5.0.2+). The e2e suite
verifies each working tree against the **latest released joblet** downloaded
from GitHub, on the same host.

joblet-flow installs and versions independently of joblet: its package declares
no dpkg dependency, and joblet has no knowledge of joblet-flow. The engine
requires a joblet install on the same host at runtime (it reads mTLS
credentials from the joblet install's `rnx-config.yml`).

## joblet-flow ↔ SDKs

SDK workers and clients (`joblet-flow-sdk-python`, `joblet-flow-sdk-node`)
generate their `FlowService` stubs from the same `flow.proto`; they follow the
same proto v2.6.0+ requirement.

The authoritative cross-project matrix lives in
[joblet-proto/COMPATIBILITY.md](https://github.com/ehsaniara/joblet-proto/blob/main/COMPATIBILITY.md).
