#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DA_BIN="${DA_BIN:-$ROOT/bin/deploy-agent}"
DEMO="$ROOT/demo"
WORK="$DEMO/.work"
REGISTRY_NAME="deploy-agent-demo-registry"
IMAGE="localhost:5000/deploy-agent-demo"
SHA1="1111111111111111111111111111111111111111"
SHA2="2222222222222222222222222222222222222222"

cleanup() {
  set +e
  if command -v docker >/dev/null 2>&1; then
    DEPLOY_IMAGE="$IMAGE:sha-${SHA2:0:12}" docker compose -f "$DEMO/compose.yml" down -v --remove-orphans >/dev/null 2>&1 || true
    docker rm -f "$REGISTRY_NAME" >/dev/null 2>&1 || true
    docker image rm "$IMAGE:sha-${SHA1:0:12}" "$IMAGE:sha-${SHA2:0:12}" >/dev/null 2>&1 || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

command -v docker >/dev/null || { echo "docker is required" >&2; exit 1; }
[ -x "$DA_BIN" ] || { echo "deploy-agent binary not found: $DA_BIN" >&2; exit 1; }

rm -rf "$WORK"
mkdir -p "$WORK"

docker run -d --name "$REGISTRY_NAME" -p 127.0.0.1:5000:5000 registry:2 >/dev/null

build_push() {
  local sha="$1"
  local tag="sha-${sha:0:12}"
  docker build \
    --build-arg GIT_SHA="$sha" \
    --build-arg IMAGE_TAG="$tag" \
    -t "$IMAGE:$tag" "$DEMO/app" >/dev/null
  docker push "$IMAGE:$tag" >/dev/null
}

build_push "$SHA1"
build_push "$SHA2"

cat > "$WORK/config.json" <<JSON
{
  "app": "deploy-agent-demo",
  "environment": "demo",
  "instance": "github-runner",
  "image": "$IMAGE",
  "desired": { "source": "file", "path": "$WORK/desired.txt" },
  "runtime": {
    "type": "docker-compose",
    "compose_dir": "$DEMO",
    "compose_file": "compose.yml",
    "service": "demo",
    "image_env": "DEPLOY_IMAGE",
    "image_env_value": "image"
  },
  "verify": {
    "url": "http://127.0.0.1:18080/healthz",
    "expected_service": "deploy-agent-demo",
    "request_timeout_seconds": 2,
    "startup_timeout_seconds": 30,
    "retry_interval_seconds": 1
  },
  "state_file": "$WORK/state.json",
  "poll_seconds": 60,
  "rollback": true,
  "agent_health": { "listen": "" }
}
JSON

printf '%s\n' "$SHA1" > "$WORK/desired.txt"
"$DA_BIN" -config "$WORK/config.json" once
curl -fsS http://127.0.0.1:18080/healthz | grep -q "$SHA1"

printf '%s\n' "$SHA2" > "$WORK/desired.txt"
"$DA_BIN" -config "$WORK/config.json" once
curl -fsS http://127.0.0.1:18080/healthz | grep -q "$SHA2"

"$DA_BIN" -config "$WORK/config.json" check >/dev/null

echo "demo PASS: deploy -> update -> verify; cleanup will now remove containers, registry and temp state"
