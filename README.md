# deploy-agent

Small native deployment reconciler for Docker/Compose hosts.

**Vietnamese:** [README.vi.md](README.vi.md)

## Download a prebuilt binary

Linux / macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/ngtrthanh/deploy-agent/main/scripts/get.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/ngtrthanh/deploy-agent/main/scripts/get.ps1 | iex
```

Default channel is the rolling `edge` release built from `main`. Set `DA_VERSION=vX.Y.Z` to pin a stable release.

Prebuilt targets: Linux `amd64/arm64/armv7/armv6/386`, Windows `amd64/arm64/386`, macOS `amd64/arm64`. Downloads are SHA-256 verified.

## What it does

```text
desired release
      ↓
observe app
      ↓
drift? → pull → docker compose up
      ↓
verify app /healthz
      ↓
accept or rollback
```

DA is a native binary, not a container. In production, run it as a short-lived one-shot job from the OS scheduler so reboot/crash recovery belongs to the OS, not to DA itself.

## Docker demo

```sh
mkdir -p bin
go build -o bin/deploy-agent ./cmd/deploy-agent
bash demo/run.sh
```

The demo starts a local registry, deploys v1, updates to v2, verifies the running app, then removes the demo containers, registry, images, and temporary state.

GitHub Actions runs the same end-to-end demo on every push and pull request.

## Current status

v0.x currently uses Git SHA as deployment identity. The next core revision will adopt digest-pinned identity and the T1 reconciler rules from STD-CICD v2.
