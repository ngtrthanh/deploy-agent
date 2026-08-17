# deploy-agent

Generic deployment agent for immutable, Git-SHA-addressed application deployments across Linux and Windows environments.

## Core rule

`deploy-agent` does not decide that a deployment is healthy from a container tag or HTTP 200 alone. The managed application must expose a deployment-proof endpoint:

```text
GET /healthz
```

The agent deploys the desired immutable artifact, calls the application's `/healthz`, and accepts the deployment only when the application reports the expected `git_sha`.

```text
desired SHA
   ↓
pull immutable artifact
   ↓
recreate / restart
   ↓
GET app:/healthz
   ↓
status == ok && git_sha == desired SHA
   ↓
ACCEPT

otherwise
   ↓
ROLLBACK
```

See:

- [Application health contract](docs/health-contract.md)
- [How to add `/healthz` to an application](docs/app-healthz-integration.md)

## Responsibility boundary

| Component | Responsibility |
|---|---|
| CI | test, lint, build immutable artifact |
| Registry | store `sha-<git_sha>` artifacts |
| Deployment control | define desired SHA per app/environment/instance |
| deploy-agent | detect drift, deploy, verify, accept or rollback |
| managed application | expose `/healthz` with its own runtime identity |

The application's `/healthz` is the deployment proof. The agent may later expose its own health endpoint, but that only proves the agent itself is alive.
