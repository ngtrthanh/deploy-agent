# Managed Application `/healthz` Contract

Status: **v0.1 contract**

This contract defines the minimum interface that an application managed by `deploy-agent` must expose so that a deployment can be proven, not merely assumed.

## 1. Endpoint

```http
GET /healthz
```

Requirements:

- Must not require interactive login, cookie session, or browser redirect.
- Must return JSON.
- Must be lightweight and safe to call repeatedly.
- Must not return secrets, credentials, connection strings, tokens, or internal stack traces.
- Should normally be reachable from the host running `deploy-agent`; it does not need to be publicly exposed.
- Returns HTTP `200` only when this application process is ready to serve.
- Returns non-2xx, preferably `503`, while the process is not ready.

## 2. Minimum response

```json
{
  "status": "ok",
  "service": "wsm-edge",
  "git_sha": "7bfe1179656431d785142ffb1afba3a6001a30f1",
  "image_tag": "sha-7bfe11796564",
  "uptime_seconds": 42
}
```

Required fields:

| Field | Meaning |
|---|---|
| `status` | `ok` when the process is ready |
| `service` | Stable application/service name |
| `git_sha` | Full Git commit SHA baked into the artifact by CI |
| `image_tag` | Human-readable artifact tag if available |
| `uptime_seconds` | Process uptime |

Optional fields:

```json
{
  "environment": "prod",
  "instance": "WHD-NC",
  "version": "1.4.2"
}
```

`environment` and `instance` are useful operational metadata but are not the primary proof of artifact identity.

## 3. Critical identity rule

`git_sha` MUST identify the code actually inside the running artifact.

Therefore:

> **Do not make deploy-agent inject `GIT_SHA=<desired_sha>` into the managed container at deployment time.**

That would allow a wrong image to claim the expected SHA.

The SHA must be embedded by CI during artifact build.

Correct:

```text
CI checkout commit A
   ↓
build artifact with GIT_SHA=A
   ↓
push image sha-A
   ↓
deploy-agent requests sha-A
   ↓
application reports A from inside artifact
```

Incorrect:

```text
pull unknown image
   ↓
deploy-agent sets GIT_SHA=A at runtime
   ↓
unknown image reports A
```

The second model destroys deployment proof.

## 4. CI build metadata

For Docker builds, use build arguments and bake them into the image:

```dockerfile
ARG GIT_SHA=unknown
ARG IMAGE_TAG=unknown
ARG APP_VERSION=unknown

ENV GIT_SHA=${GIT_SHA}
ENV IMAGE_TAG=${IMAGE_TAG}
ENV APP_VERSION=${APP_VERSION}
```

Example GitHub Actions build input:

```yaml
build-args: |
  GIT_SHA=${{ github.sha }}
  IMAGE_TAG=sha-${{ github.sha }}
  APP_VERSION=${{ github.ref_name }}
```

For compiled applications, prefer embedding the SHA into the binary at compile/link time when practical. Runtime environment variables baked into the immutable image are acceptable, but they must originate from the artifact build, not from desired-state deployment configuration.

## 5. Acceptance rule used by deploy-agent

A deployment is accepted only when all required checks pass:

```text
HTTP status == 200
AND response.status == "ok"
AND response.service == expected service
AND response.git_sha == desired full Git SHA
```

Pseudo-code:

```text
health = GET(config.verify.url)

if health.http_status != 200:
    fail

if health.status != "ok":
    fail

if health.service != config.app:
    fail

if normalize(health.git_sha) != desired_sha:
    fail

accept deployment
```

HTTP `200` by itself is never sufficient deployment evidence.

## 6. SHA matching

The application should return the **full 40-character Git SHA**.

The deployment pointer may use a short display tag such as:

```text
sha-7bfe11796564
```

but deploy-agent should resolve and retain the full SHA used for verification.

Preferred:

```text
desired_git_sha = 7bfe1179656431d785142ffb1afba3a6001a30f1
```

Do not accept arbitrary prefix matches unless explicitly configured for legacy compatibility.

## 7. Auth and network exposure

`/healthz` is unauthenticated for machine probing, but should expose only non-sensitive deployment metadata.

Recommended access order:

1. loopback (`127.0.0.1` / `localhost`),
2. private host network,
3. internal service network,
4. public exposure only when necessary and filtered by reverse proxy/firewall.

Application auth middleware must explicitly bypass `/healthz` before login/session enforcement.

A redirect to `/login` is a failed health check even if the final browser page returns HTTP 200.

## 8. Readiness scope

`/healthz` is primarily **process + deployment readiness**, not a complete monitoring system.

It should answer:

```text
Is this the intended application process?
Is it ready?
Which immutable code artifact is it actually running?
```

Avoid making it fail because of every non-critical remote dependency, otherwise temporary WAN/API problems could cause unnecessary rollback loops.

For deeper checks, applications may additionally expose:

```text
/livez
/readyz
/api/ops/deployment
```

but these are outside the minimum v0.1 contract.

## 9. Example deployment verification

```text
desired SHA: 7bfe1179656431d785142ffb1afba3a6001a30f1

GET http://127.0.0.1:8080/healthz

200 OK
{
  "status": "ok",
  "service": "wsm-edge",
  "git_sha": "7bfe1179656431d785142ffb1afba3a6001a30f1",
  "image_tag": "sha-7bfe11796564",
  "uptime_seconds": 11
}

→ ACCEPT
```

Mismatch example:

```text
desired: 7bfe1179...
running: a97465d4...

→ REJECT
→ rollback according to deployment policy
```
