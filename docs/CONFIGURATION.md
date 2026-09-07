# Configuration

The engine is configured entirely through environment variables.

| Variable           | Default                   | Purpose                                                                                     |
|--------------------|---------------------------|---------------------------------------------------------------------------------------------|
| `FLOW_LISTEN_ADDR` | `:50055`                  | Address the `FlowService` gRPC server listens on.                                           |
| `JOBLET_INSECURE`  | *(off)*                   | `1` dials joblet in plaintext (dev/e2e only).                                               |
| `JOBLET_ADDR`      | `localhost:50051`         | joblet target **in insecure mode**.                                                         |
| `JOBLET_CONFIG`    | *(standard search paths)* | Explicit `rnx-config.yml` for mTLS.                                                         |
| `JOBLET_NODE`      | *(isDefault node)*        | Which node entry in `rnx-config.yml` to dial; empty uses the node marked `isDefault: true`. |

## Connecting to joblet

The engine reaches joblet in one of two modes.

### mTLS (default)

The engine loads a node from an `rnx-config.yml` - the same file and format rnx
uses - and dials joblet with the node's embedded client certificate, verifying
the server as `joblet` over TLS 1.3.

```yaml
# rnx-config.yml
version: "3.0"
nodes:
  admin:
    isDefault: true  # used when no node name is specified
    address: "10.0.0.10:50051"
    cert: |
      -----BEGIN CERTIFICATE-----
      ...
    key: |
      -----BEGIN PRIVATE KEY-----
      ...
    ca: |
      -----BEGIN CERTIFICATE-----
      ...
```

Config file search order (first match wins), unless `JOBLET_CONFIG` is set:

1. `./rnx-config.yml`
2. `./config/rnx-config.yml`
3. `~/.rnx/rnx-config.yml`
4. `/etc/joblet/rnx-config.yml`
5. `/opt/joblet/config/rnx-config.yml`

```bash
FLOW_LISTEN_ADDR=:50055 JOBLET_NODE=admin ./bin/joblet-flow
```

The engine fails to start if the config/node can't be loaded (unless
`JOBLET_INSECURE=1`). The connection is lazy - an unreachable joblet is not
detected until the first activity is dispatched.

### Insecure (dev/e2e)

```bash
JOBLET_INSECURE=1 JOBLET_ADDR=localhost:50051 ./bin/joblet-flow
```

No certificates required. Against a real joblet (which mandates mTLS) activity
dispatch will fail - this mode is for engine-only development and the
lifecycle e2e suite.

## SDK ↔ engine link

Workers and clients connect to the engine over **plaintext gRPC** (the mTLS is
between the engine and joblet). SDKs read the engine address from `FLOW_ADDR`
(default `127.0.0.1:50055`).
