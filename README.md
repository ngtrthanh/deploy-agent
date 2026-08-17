# deploy-agent

Small native deployment reconciler for Docker Compose hosts.

**Tiếng Việt:** [README.vi.md](README.vi.md)

DA is designed as **one native binary per host**, managing many deployment units. It does not run in Docker, does not need an inbound port, and normally runs as a short-lived one-shot job from the OS scheduler.

```text
host
└── deploy-agent
    ├── wsm-edge
    ├── matflow
    ├── cems-etl
    └── hpr-traffic
```

The v0.2 target is simple:

```text
Git says what SHOULD run
Docker says what IS running
DA makes them equal
```

## Install

Linux / macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/ngtrthanh/deploy-agent/main/scripts/get.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/ngtrthanh/deploy-agent/main/scripts/get.ps1 | iex
```

Prebuilt targets: Linux `amd64/arm64/armv7/armv6/386`, Windows `amd64/arm64/386`, macOS `amd64/arm64`. Downloads are SHA-256 checked.

`edge` follows `main`. Production should pin a stable `vX.Y.Z` release.

## Promotion: what triggers an upgrade?

Application source and deployment authority are separate.

| Repository | Question |
|---|---|
| application repo | Which releases exist? |
| `deploy-state` | Which release is allowed to run where? |

A feature or bug fix follows this path:

```text
feature / bug request
    ↓
source PR → CI → merge
    ↓
build image once
    ↓
candidate image@sha256:BBBB
    ↓
promotion PR changes desired digest AAAA → BBBB
    ↓
promotion PR merged                 ← upgrade trigger
    ↓
DA fleet polls and converges
```

No webhook is required. Rollback is the same mechanism: promote an earlier digest.

Example desired state:

```json
{
  "apiVersion": "deploy/v1",
  "kind": "Release",
  "metadata": {
    "service": "wsm-edge",
    "environment": "edge-prod"
  },
  "spec": {
    "image": "ghcr.io/example/wsm-edge-server",
    "digest": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "rollout": { "strategy": "recreate" },
    "migration": { "required": false }
  },
  "provenance": {
    "git_sha": "1111111111111111111111111111111111111111"
  }
}
```

`spec.digest` is the canonical deployment identity. Tags are convenience only.

## Verification without modifying the app

DA must not require every application to implement DA-specific endpoints.

Artifact identity is proven by the host runtime:

```text
desired image@sha256:BBBB
        ↓
docker pull by digest
        ↓
docker compose recreate
        ↓
docker inspect running container
        ↓
running artifact == desired artifact
```

Application readiness is a separate probe. v0.2 should support:

```text
http          existing /health, /healthz, or another URL
docker-health Docker HEALTHCHECK
command       command executed in/for the service
tcp           port accepts a connection
process       container is running and stable
```

A DA-aware application may additionally expose `/healthz`, `/readyz`, and `/api/ops/identity`. Those endpoints provide stronger application-level evidence and better observability, but they are **optional**, not a prerequisite for management by DA.

### WSM Edge example: zero app changes

If WSM Edge already exposes `GET /health`, DA can upgrade it without changing source code:

```text
desired digest BBBB
    ↓
current digest AAAA
    ↓
backup hook
    ↓
pull image@BBBB
    ↓
compose override pins image@BBBB
    ↓
force-recreate only the service
    ↓
Docker artifact proof
    +
existing /health == 200
    ↓
ACCEPT BBBB
```

If verification fails and no migration ran, DA restores the previous accepted digest.

## Reconciliation

Recommended production mode:

```text
OS scheduler every 30–60 s
        ↓
deploy-agent reconcile
        ↓
scan deployment units
        ↓
read desired → observe actual → converge
        ↓
exit
```

This survives reboot and DA crashes because the OS calls it again. Desired-state failure must not disturb a currently accepted service.

A reconciliation must eventually include: per-unit lock, bounded command timeouts, exponential backoff with jitter, attempt ceiling, durable last-run status, and last-known-good desired-state cache.

## One DA, many deployment units

Target host layout:

```text
/etc/deploy-agent/
├── agent.json
└── apps/
    ├── wsm-edge.json
    ├── matflow.json
    └── hpr-traffic.json
```

Windows uses the same model under `C:\ProgramData\deploy-agent\`.

Locks are per deployment unit so one slow deployment does not block unrelated applications on the same host.

## Commands

Current CLI:

```bash
deploy-agent -config deploy-agent.json once
deploy-agent -config deploy-agent.json check
deploy-agent -config deploy-agent.json run
deploy-agent -version
```

`once` is the preferred production execution model. Fleet/multi-unit orchestration is part of the v0.2 work and is not yet wired in the current CLI.

## Current v0.2 branch status

`feature/v0.2-digest-reconciler` is an active migration branch, not a release. Lower layers are being moved from Git-SHA/tag identity to digest identity and split verification. Upper-layer wiring, fleet mode, generic probes, retry policy, locking, desired-state caching, and demo/docs migration are still in progress.

At the current branch head, GitHub CI stops at the `gofmt` check before vet/test/build. Do not treat the branch as production-ready until CI and the Docker E2E demo are green again.

## Demo

`demo/` currently proves the v0.1 end-to-end flow. It will be moved to `deploy/v1`, digest-pinned deployment, generic verification, upgrade, rollback, and clean teardown before v0.2 is merged to `main`.

## Design rules

- One DA binary per host, not one DA per container.
- Deployment unit may contain one or several Compose services.
- Digest is deployment identity; Git SHA is provenance.
- Promotion is a Git desired-state change, not a host command.
- Runtime artifact proof does not depend on the application claiming its own digest.
- Existing apps can be managed without source modification when a suitable generic readiness probe exists.
- DA-aware endpoints are optional stronger evidence.
- Normal production execution is one-shot and scheduler-driven.
- A failed desired-state fetch must leave the currently accepted service alone.
