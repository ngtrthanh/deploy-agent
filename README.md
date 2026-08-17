# deploy-agent

A small, generic deployment reconciler for immutable container releases.

`deploy-agent` does not trust a mutable tag, a running container, or HTTP 200 alone. A deployment is accepted only when the managed application reports the exact desired Git SHA from its own `/healthz` endpoint.

## v0.1 scope

- desired release from local file or HTTP
- full 40-character Git SHA as canonical identity
- immutable image tag `sha-<first12>` by default
- Docker Compose runtime
- application `/healthz` verification
- exact service + Git SHA matching
- atomic accepted-state persistence
- automatic rollback to the previous accepted release
- `once`, `run`, and `check` commands
- optional deploy-agent `/healthz` in long-running `run` mode
- Linux systemd timer packaging
- Windows Scheduled Task installer
- dependency-free Go binary (standard library only)

## Quick start

The managed Compose file must use a deployment image variable:

```yaml
services:
  app:
    image: ${DEPLOY_IMAGE}
```

Create a desired pointer containing the full Git SHA:

```text
7bfe1179656431d785142ffb1afba3a6001a30f1
```

Copy `deploy-agent.example.json`, edit it, then:

```bash
go build -o deploy-agent ./cmd/deploy-agent
./deploy-agent -config deploy-agent.json check
./deploy-agent -config deploy-agent.json once
```

For a daemon-style process:

```bash
./deploy-agent -config deploy-agent.json run
```

## State machine

```text
CHECK DESIRED
    |
    +-- running SHA == desired SHA --> ACCEPT / NO-OP
    |
    v
PULL IMMUTABLE IMAGE
    |
    v
RECREATE SERVICE
    |
    v
VERIFY APP /healthz
    |
    +-- exact SHA match --> ACCEPT + persist state
    |
    +-- failure --> ROLLBACK previous accepted SHA
```

## Contracts

- [`docs/health-contract.md`](docs/health-contract.md) — minimum app-side `/healthz` contract.
- [`docs/app-healthz-integration.md`](docs/app-healthz-integration.md) — how to add `/healthz` to Rust, Go, Python, Node, and .NET apps.
- [`docs/deployment-contract.md`](docs/deployment-contract.md) — desired release, Compose, verification, and rollback contract.

## Examples

- [`examples/matflow.json`](examples/matflow.json)
- [`examples/wsm-edge.json`](examples/wsm-edge.json)

## Security rule

The desired Git SHA must never be injected into the managed app at deployment time. The app must report a SHA baked into the artifact by CI; otherwise a wrong image could falsely claim the expected identity.
