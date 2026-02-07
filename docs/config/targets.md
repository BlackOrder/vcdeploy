# Target Configuration

Targets define WHERE a project gets deployed. Each target maps to a deployment destination —
either an agent-managed server or the master itself (local deployment).

## Overview

A target belongs to a project and specifies:
- **Name** — Unique identifier within the project (e.g., "production", "staging")
- **Agent** — Which agent handles the deployment (optional; omit for master-local)
- **Path** — Filesystem path on the target server

The **deployment strategy** (symlink vs inplace) is configured at the **project level** in `deployment.strategy`, not per target. The **orchestration mode** (parallel vs rolling) is also configured at the project level in `orchestration.mode`, and can be overridden per deploy via `--mode`.

## Management

Targets can be managed via:
- **CLI**: `vcdeploy target create/list/show/update/delete`
- **API**: `GET/POST /api/v1/targets`, `GET/PUT/DELETE /api/v1/targets/{id}`
- **UI**: Targets page in web interface
- **YAML**: Defined in project config (synced to database)

## YAML Configuration

In `project.yaml`:

```yaml
targets:
  production:
    agent: web-1
    path: /var/www/myapp
  staging:
    agent: staging-1
    path: /var/www/myapp-staging
  local:
    path: /opt/deploy/myapp    # No agent = master-local deployment
```

## Target Resolution

When deploying:
- `--target production` → deploys to the named target
- `--target prod --target staging` → deploys to multiple targets
- `--all` → deploys to all targets in the project
- No flag + single target → uses the only target implicitly
- No flag + multiple targets → error: must specify `--target` or `--all`

## Master-Local Targets

Targets without an `agent` are deployed locally by the master server.
The master runs the project's deployment strategy (symlink or inplace) using a local command runner.
No gRPC streaming is needed — the archive is extracted directly.

## Strategies

Strategies are configured at the **project level** (`deployment.strategy`), not per target:

| Strategy | Description |
|----------|-------------|
| `symlink` | Numbered releases with atomic symlink swap (default). Supports rollback. |
| `inplace` | Extract directly to target path, overwriting existing files. No rollback support. |

## Orchestration

Orchestration mode controls how multi-target deployments are dispatched:

| Mode | Description |
|------|-------------|
| `parallel` | Deploy to all targets concurrently (default) |
| `rolling` | Deploy to targets sequentially; stop on first failure unless `--continue-on-error` |

Configured at project level (`orchestration.mode`) and overridable via `--mode` flag on `deploy create`.

## Examples

### Single target project

```yaml
# Only one target — used implicitly on deploy
targets:
  production:
    agent: web-1
    path: /var/www/myapp
```

### Multi-target project

```yaml
targets:
  staging:
    agent: staging-1
    path: /var/www/myapp
  production:
    agent: web-1
    path: /var/www/myapp
  local-test:
    path: /opt/test/myapp     # Master-local, no agent
```

### CLI Management

```bash
# List targets for a project
vcdeploy target list --project myapp

# Create a new target
vcdeploy target create production --project myapp --agent web-1 --path /var/www/myapp

# Update a target
vcdeploy target update production --project myapp --agent web-2

# Delete a target
vcdeploy target delete staging --project myapp --force
```

### API Management

```bash
# List targets for a project
curl http://localhost:9000/api/v1/projects/myapp/targets \
  -H "Authorization: Bearer $API_KEY"

# Create a target
curl -X POST http://localhost:9000/api/v1/projects/myapp/targets \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name": "production", "path": "/var/www/myapp", "agent_id": 5}'
```

## Related Documentation

- [Project Configuration](projects.md) — Full project configuration reference
- [Agent Configuration](agent.md) — Agent setup
- [CLI Reference](../ux/cli-map.md) — CLI command reference
- [API Reference](../ux/api-map.md) — REST API reference
