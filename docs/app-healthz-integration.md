# How to Add `/healthz` to a Managed Application

This guide shows the standard way to graft the `deploy-agent` health contract into an existing application.

The application owns `/healthz`. `deploy-agent` only calls and verifies it.

## 1. Integration checklist

For every managed application:

1. Add a `GET /healthz` route.
2. Bypass interactive authentication for that exact route.
3. Return JSON with `status`, `service`, `git_sha`, `image_tag`, and `uptime_seconds`.
4. Bake the real Git SHA into the artifact during CI build.
5. Do not inject the desired SHA from deploy-agent at runtime.
6. Return HTTP `200` only after the process is ready.
7. Test locally with `curl`.
8. Configure deploy-agent to verify the returned full SHA.

Minimum verification:

```bash
curl -fsS http://127.0.0.1:<PORT>/healthz
```

Expected shape:

```json
{
  "status": "ok",
  "service": "example-app",
  "git_sha": "<full-40-char-sha>",
  "image_tag": "sha-<short-sha>",
  "uptime_seconds": 10
}
```

---

## 2. Rust + Axum

### Handler

```rust
use axum::{routing::get, Json, Router};
use serde::Serialize;
use std::{sync::OnceLock, time::Instant};

static STARTED: OnceLock<Instant> = OnceLock::new();

#[derive(Serialize)]
struct HealthResponse {
    status: &'static str,
    service: &'static str,
    git_sha: &'static str,
    image_tag: &'static str,
    uptime_seconds: u64,
}

async fn healthz() -> Json<HealthResponse> {
    let started = STARTED.get_or_init(Instant::now);

    Json(HealthResponse {
        status: "ok",
        service: "example-app",
        git_sha: option_env!("GIT_SHA").unwrap_or("unknown"),
        image_tag: option_env!("IMAGE_TAG").unwrap_or("unknown"),
        uptime_seconds: started.elapsed().as_secs(),
    })
}

pub fn health_router() -> Router {
    Router::new().route("/healthz", get(healthz))
}
```

### Important

If the application has auth middleware, bypass `/healthz` before session enforcement.

Conceptually:

```rust
if req.uri().path() == "/healthz" {
    return next.run(req).await;
}
```

### Build metadata

Ensure `GIT_SHA` exists in the environment when `cargo build` runs so `option_env!` embeds it into the binary.

Example Docker build stage:

```dockerfile
ARG GIT_SHA=unknown
ARG IMAGE_TAG=unknown
ENV GIT_SHA=${GIT_SHA}
ENV IMAGE_TAG=${IMAGE_TAG}

RUN cargo build --release
```

This is the preferred model because the running binary carries its own Git identity.

---

## 3. Go

### Application code

```go
package main

import (
    "encoding/json"
    "net/http"
    "time"
)

var (
    gitSHA   = "unknown"
    imageTag = "unknown"
    started  = time.Now()
)

func healthz(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]any{
        "status":         "ok",
        "service":        "example-app",
        "git_sha":        gitSHA,
        "image_tag":      imageTag,
        "uptime_seconds": int64(time.Since(started).Seconds()),
    })
}

func main() {
    http.HandleFunc("/healthz", healthz)
    // register normal routes...
    http.ListenAndServe(":8080", nil)
}
```

### Build metadata

Prefer Go linker variables:

```bash
go build \
  -ldflags "-X main.gitSHA=${GIT_SHA} -X main.imageTag=${IMAGE_TAG}" \
  -o app .
```

This embeds identity in the executable.

---

## 4. Python + FastAPI

```python
import os
import time
from fastapi import FastAPI

app = FastAPI()
STARTED = time.monotonic()

GIT_SHA = os.environ.get("GIT_SHA", "unknown")
IMAGE_TAG = os.environ.get("IMAGE_TAG", "unknown")

@app.get("/healthz", include_in_schema=False)
def healthz():
    return {
        "status": "ok",
        "service": "example-app",
        "git_sha": GIT_SHA,
        "image_tag": IMAGE_TAG,
        "uptime_seconds": int(time.monotonic() - STARTED),
    }
```

For interpreted runtimes, bake the metadata into the immutable image during CI:

```dockerfile
ARG GIT_SHA=unknown
ARG IMAGE_TAG=unknown
ENV GIT_SHA=${GIT_SHA}
ENV IMAGE_TAG=${IMAGE_TAG}
```

Do not override these values from deploy-agent's desired state.

---

## 5. Node.js + Express

```javascript
const express = require('express');
const app = express();
const started = process.uptime;

const gitSha = process.env.GIT_SHA || 'unknown';
const imageTag = process.env.IMAGE_TAG || 'unknown';

app.get('/healthz', (_req, res) => {
  res.json({
    status: 'ok',
    service: 'example-app',
    git_sha: gitSha,
    image_tag: imageTag,
    uptime_seconds: Math.floor(process.uptime()),
  });
});
```

Docker image metadata:

```dockerfile
ARG GIT_SHA=unknown
ARG IMAGE_TAG=unknown
ENV GIT_SHA=${GIT_SHA}
ENV IMAGE_TAG=${IMAGE_TAG}
```

Register `/healthz` before middleware that redirects unauthenticated requests to login, or explicitly exclude it from that middleware.

---

## 6. ASP.NET Core / .NET Minimal API

```csharp
var builder = WebApplication.CreateBuilder(args);
var app = builder.Build();
var startedAt = DateTimeOffset.UtcNow;

var gitSha = Environment.GetEnvironmentVariable("GIT_SHA") ?? "unknown";
var imageTag = Environment.GetEnvironmentVariable("IMAGE_TAG") ?? "unknown";

app.MapGet("/healthz", () => Results.Ok(new
{
    status = "ok",
    service = "example-app",
    git_sha = gitSha,
    image_tag = imageTag,
    uptime_seconds = (long)(DateTimeOffset.UtcNow - startedAt).TotalSeconds
})).AllowAnonymous();

// normal authenticated routes...

app.Run();
```

When authorization uses a fallback policy, `.AllowAnonymous()` is required for `/healthz`.

For stronger identity, Git SHA can also be embedded into assembly informational version during build. The minimum deploy-agent contract only requires that the value reported by the running artifact is immutable and trustworthy.

---

## 7. Dockerfile pattern

Generic pattern:

```dockerfile
ARG GIT_SHA=unknown
ARG IMAGE_TAG=unknown
ARG APP_VERSION=unknown

ENV GIT_SHA=${GIT_SHA}
ENV IMAGE_TAG=${IMAGE_TAG}
ENV APP_VERSION=${APP_VERSION}
```

GitHub Actions:

```yaml
- name: Build image
  uses: docker/build-push-action@v6
  with:
    push: true
    tags: |
      ghcr.io/<org>/<app>:sha-${{ github.sha }}
    build-args: |
      GIT_SHA=${{ github.sha }}
      IMAGE_TAG=sha-${{ github.sha }}
      APP_VERSION=${{ github.ref_name }}
```

A project may use a 12-character SHA in the Docker tag for readability, but `/healthz.git_sha` should report the full SHA.

---

## 8. Existing authentication middleware

A common failure is:

```text
GET /healthz
→ 302 /login
→ login HTML
→ 200
```

A naive updater sees `200` and incorrectly treats this as healthy.

The fix belongs in the application auth layer.

Required behavior:

```text
GET /healthz
→ directly reaches health handler
→ JSON
→ no login redirect
```

Test with redirects disabled or inspect headers:

```bash
curl -i http://127.0.0.1:<PORT>/healthz
```

Expected:

```text
HTTP/1.1 200 OK
Content-Type: application/json
```

Not:

```text
HTTP/1.1 302 Found
Location: /login
```

---

## 9. Readiness

Do not publish `status: ok` before the application is able to serve normal requests.

Recommended startup order:

```text
process starts
   ↓
load config
   ↓
initialize required local state
   ↓
bind application dependencies
   ↓
mark ready
   ↓
/healthz → 200
```

Before readiness:

```text
/healthz → 503
```

Do not make `/healthz` depend on every optional remote service. For edge systems in particular, WAN loss should not cause the deployment agent to roll back a locally healthy application.

---

## 10. deploy-agent configuration

Example managed application config:

```yaml
app: wsm-edge
environment: prod
instance: WHD-NC

artifact:
  image: ghcr.io/ngtrthanh/wsm-edge-server

verify:
  url: http://127.0.0.1:8080/healthz
  expected_service: wsm-edge
  sha_field: git_sha
  timeout_seconds: 60

rollback:
  enabled: true
```

For MatFlow:

```yaml
app: plantops-matflow
environment: prod
instance: inst4

artifact:
  image: ghcr.io/ngtrthanh/plantops-matflow

verify:
  url: http://127.0.0.1:8470/healthz
  expected_service: plantops-matflow
  sha_field: git_sha
  timeout_seconds: 60
```

---

## 11. Acceptance test for every application

Before an application is onboarded to deploy-agent, verify all of these:

```text
[ ] GET /healthz exists
[ ] no login/session required
[ ] no redirect
[ ] JSON response
[ ] service name correct
[ ] full git_sha present
[ ] git_sha came from artifact build
[ ] image_tag present
[ ] uptime_seconds increases
[ ] returns 503 before ready, if startup has a readiness phase
[ ] deploy-agent can reach endpoint from host
[ ] wrong expected SHA causes deployment rejection
```

Only after this test should the application be considered deploy-agent compatible.
