# Deployment Contract

`deploy-agent` reconciles a desired immutable release with the application actually serving on a host.

## Desired release format

Preferred pointer content is a full Git SHA:

```text
7bfe1179656431d785142ffb1afba3a6001a30f1
```

The agent derives `sha-7bfe11796564` as the image tag.

A JSON pointer is also supported when the registry tag differs:

```json
{
  "git_sha": "7bfe1179656431d785142ffb1afba3a6001a30f1",
  "image_tag": "release-42"
}
```

Short SHAs are rejected as desired identity.

## Compose contract

The managed Compose file should reference the image via an environment variable owned by the deployment contract:

```yaml
services:
  app:
    image: ${DEPLOY_IMAGE}
```

The agent exports `DEPLOY_IMAGE=<registry>/<app>:<immutable-tag>` only for the `docker compose pull/up` process. It does not inject the desired Git SHA into the application.

## Reconciliation

```text
desired release
  -> read application /healthz
  -> no-op if git_sha matches
  -> pull desired image
  -> recreate service
  -> wait for /healthz
  -> require exact full git_sha match
  -> persist accepted release
  -> on failure, rollback to previous accepted release when available
```

The accepted state file is written atomically on Unix-like systems and best-effort replaced on Windows. It contains the last proven `git_sha`, `image_tag`, and acceptance time.
