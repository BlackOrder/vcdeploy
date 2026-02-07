# vcdeploy REST API Map

> **Version:** 1.0  
> **Base URL:** `/api/v1`  
> **Auth:** Bearer token or `?api_key=` query param

This document defines the canonical REST API surface. All endpoints follow REST conventions with consistent request/response patterns.

> **Note:** All resource identifiers use XID format (20-character, time-sortable, globally unique). Deployment IDs, provision job IDs, and archive IDs are XIDs. Resources also have a `uid` field (XID) for cross-server identity used in import/export.

---

## Table of Contents

1. [Response Format](#response-format)
2. [Authentication](#authentication)
3. [Health & System](#health--system)
4. [Core Resources](#core-resources)
5. [Deployment Resources](#deployment-resources)
6. [Security Resources](#security-resources)
7. [Recipe Resources](#recipe-resources)
8. [Real-time Endpoints](#real-time-endpoints)
9. [Bulk Operations](#bulk-operations)

---

## Response Format

### Success Response

```json
{
  "data": { ... },
  "meta": {
    "total": 100,
    "page": 1,
    "per_page": 20
  }
}
```

### Error Response

```json
{
  "error": {
    "code": "validation_error",
    "message": "Invalid project name",
    "details": {
      "field": "name",
      "reason": "must be lowercase alphanumeric with hyphens"
    }
  }
}
```

### HTTP Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 201 | Created |
| 204 | No Content (successful DELETE) |
| 400 | Bad Request (validation error) |
| 401 | Unauthorized (missing/invalid auth) |
| 403 | Forbidden (insufficient permissions) |
| 404 | Not Found |
| 409 | Conflict (duplicate resource) |
| 422 | Unprocessable Entity (semantic error) |
| 500 | Internal Server Error |

---

## Authentication

### Login

```
POST /auth/login
```

Request:
```json
{
  "username": "admin",
  "password": "secret",
  "totp_code": "123456"  // Optional, required if 2FA enabled
}
```

Response:
```json
{
  "token": "eyJ...",
  "expires_at": "2026-02-07T12:00:00Z",
  "user": {
    "id": 1,
    "username": "admin",
    "role": "admin"
  }
}
```

### Current User

```
GET /auth/me
```

Response: User object with current session info.

---

## Health & System

### Health Probes (No Auth)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/livez` | GET | Kubernetes liveness probe — returns 200 if process is alive |
| `/readyz` | GET | Kubernetes readiness probe — returns 200 if ready to serve |
| `/metrics` | GET | Prometheus metrics endpoint |
| `/api/v1/health` | GET | Detailed health check with component status |

### Dashboard Statistics

```
GET /stats
```

Response:
```json
{
  "total_projects": 42,
  "total_agents": 8,
  "connected_agents": 6,
  "deployments_today": 15,
  "success_rate": 94.5,
  "recent_deployments": [...]
}
```

### Deployment Analytics

```
GET /stats/deployments?range=7d&project=myapp
```

Query params:
- `range`: Time range (`1d`, `7d`, `30d`, `90d`)
- `project`: Filter by project name (optional)
- `agent`: Filter by agent ID (optional)

Response:
```json
{
  "total": 156,
  "successful": 148,
  "failed": 8,
  "success_rate": 94.87,
  "avg_duration_seconds": 45.2,
  "by_day": [
    { "date": "2026-02-01", "total": 22, "successful": 21 },
    ...
  ]
}
```

### Agent Analytics

```
GET /stats/agents?range=7d
```

Response:
```json
{
  "total": 8,
  "online": 6,
  "offline": 2,
  "utilization": [
    { "agent_id": "agent-1", "name": "web-1", "deployments": 45, "avg_cpu": 23.5 },
    ...
  ]
}
```

---

## Core Resources

### Users

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/users` | List all users |
| POST | `/users` | Create user |
| GET | `/users/{id}` | Get user by ID |
| PUT | `/users/{id}` | Update user |
| DELETE | `/users/{id}` | Delete user |
| PUT | `/users/{id}/password` | Change password |
| GET | `/users/{id}/totp` | Get TOTP status |
| POST | `/users/{id}/totp/setup` | Begin TOTP setup |
| PUT | `/users/{id}/totp` | Enable TOTP |
| DELETE | `/users/{id}/totp` | Disable TOTP |
| POST | `/users/{id}/totp/recovery` | Regenerate recovery codes |

Self-service (current user):
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/users/me/totp` | Get own TOTP status |
| POST | `/users/me/totp/setup` | Begin own TOTP setup |
| PUT | `/users/me/totp` | Enable own TOTP |
| DELETE | `/users/me/totp` | Disable own TOTP |
| POST | `/users/me/totp/recovery` | Regenerate own recovery codes |

### Projects

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/projects` | List projects |
| POST | `/projects` | Create project |
| GET | `/projects/{name}` | Get project |
| PUT | `/projects/{name}` | Update project |
| DELETE | `/projects/{name}` | Delete project |
| POST | `/projects/{name}/clone` | Clone project to new name |
| POST | `/projects/{name}/validate` | Validate project config |
| GET | `/projects/{name}/webhooks` | List project webhooks |
| POST | `/projects/{name}/webhooks` | Configure webhook |
| DELETE | `/projects/{name}/webhooks/{provider}` | Remove webhook |
| POST | `/projects/{name}/webhooks/{provider}/test` | Test webhook |
| POST | `/projects/{name}/webhooks/{provider}/rotate` | Rotate webhook secret |
| POST | `/projects/{name}/archive` | Archive project (hide from listings, prevent deployments) |
| POST | `/projects/{name}/unarchive` | Unarchive project (restore to active state) |

Query params for list:
- `search`: Search by name
- `type`: Filter by project type
- `agent`: Filter by assigned agent
- `archived`: Filter by archived status (`true`, `false`)

Project objects include an `archived` boolean field.

### Project Types

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/project-types` | List project types |
| POST | `/project-types` | Create project type |
| GET | `/project-types/{name}` | Get project type |
| PUT | `/project-types/{name}` | Update project type |
| DELETE | `/project-types/{name}` | Delete project type |

### Agents

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/agents` | List agents |
| POST | `/agents` | Register agent manually |
| GET | `/agents/{id}` | Get agent details |
| PUT | `/agents/{id}` | Update agent (name, groups, tags) |
| DELETE | `/agents/{id}` | Remove agent |
| POST | `/agents/tokens` | Generate registration token |

Query params for list:
- `status`: `online`, `offline`, `all`
- `group`: Filter by group name
- `search`: Search by name

### Targets

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/targets` | List all targets (global, with `?project=` filter) |
| GET | `/projects/{name}/targets` | List targets for a project |
| POST | `/projects/{name}/targets` | Create target for a project |
| GET | `/targets/{id}` | Get target details |
| PUT | `/targets/{id}` | Update target |
| DELETE | `/targets/{id}` | Delete target |

Query params for list:
- `project`: Filter by project name
- `agent`: Filter by agent name
- `search`: Search by target name

Target object:
```json
{
  "id": 1,
  "name": "production",
  "project_id": 1,
  "project_name": "myapp",
  "agent_id": 5,
  "agent_name": "web-1",
  "path": "/var/www/myapp",
  "created_at": "2025-01-15T10:00:00Z",
  "updated_at": "2025-01-15T10:00:00Z"
}
```

Create target request:
```json
{
  "name": "production",
  "path": "/var/www/myapp",
  "agent_id": 5
}
```

Note: Omitting `agent_id` creates a master-local target (deployed directly by the master server).

### Settings

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/settings` | List all settings (grouped by category) |
| GET | `/settings/{category}` | Get settings for category |
| PUT | `/settings/{category}` | Update category settings |
| GET | `/settings/export` | Export all settings as JSON |
| POST | `/settings/import` | Import settings from JSON |

Categories: `general`, `appearance`, `security`, `notifications`, `deployments`

### Secrets

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/secrets` | List secrets (metadata only) |
| POST | `/secrets` | Create/update secret |
| GET | `/secrets/{scope}/{key}` | Get secret value |
| DELETE | `/secrets/{scope}/{key}` | Delete secret |
| POST | `/secrets/bulk` | Bulk import secrets |
| GET | `/secrets/export` | Export secrets (encrypted) |
| POST | `/secrets/import` | Import secrets (encrypted) |

Query params:
- `project`: Filter by project scope
- `scope`: `global`, `project`

### API Keys

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api-keys` | List API keys |
| POST | `/api-keys` | Create API key |
| GET | `/api-keys/{id}` | Get API key details |
| DELETE | `/api-keys/{id}` | Revoke API key |

### Audit

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/audit` | List audit logs |
| GET | `/audit/export` | Export audit logs |

Query params:
- `user`: Filter by username
- `action`: Filter by action type
- `resource`: Filter by resource type
- `from`, `to`: Date range
- `format`: `json`, `csv`

---

## Deployment Resources

### Deployments

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/deployments` | List deployments |
| POST | `/deployments` | Create deployment |
| GET | `/deployments/{id}` | Get deployment |
| DELETE | `/deployments/{id}` | Cancel deployment |
| POST | `/deployments/{id}/retry` | Retry failed deployment |
| GET | `/deployments/{id}/archive` | Download deployment archive (HMAC-signed URL) |
| GET | `/deployments/{id}/logs` | Get deployment logs |
| GET | `/deployments/{id}/logs/stream` | SSE stream of logs |

Retry request body:
```json
{
  "wait": 30    // Optional, wait for completion in seconds
}
```
Note: Retries all targets from the original deployment. No target, branch, or commit overrides are supported.

Archive download:
```
GET /deployments/{id}/archive?token={hmac}&expires={timestamp}
```
- Response: `application/gzip` binary stream
- Auth: HMAC signature (not standard JWT/API key)

Query params for list:
- `project`: Filter by project name
- `type`: `deploy`, `rollback`
- `status`: `pending`, `running`, `success`, `failed`, `cancelled`
- `target`: Filter by target name
- `from`, `to`: Date range

Create deployment request:
```json
{
  "project": "myapp",
  "type": "deploy",           // "deploy" or "rollback"
  "branch": "main",           // Optional, defaults to project default
  "commit": "abc123",         // Optional, specific commit
  "rollback_to": "deploy-42", // Required if type=rollback
  "target": "production",     // Single target (shorthand for targets array)
  "targets": ["production", "staging"],  // Multiple targets
  "all_targets": true,        // Deploy to all project targets (mutex with target/targets)
  "dry_run": false,           // Optional, simulate only
  "wait": 30,                 // Optional, wait for completion in seconds (default: 30)
  "scheduled_at": "..."       // Optional, RFC3339 timestamp
}
```

Note: `target` (string) is shorthand for `targets` (array) with one element. `all_targets` is mutually exclusive with `target`/`targets`. If the project has exactly one target, it is used implicitly.

### Provision Jobs

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/provision-jobs` | List provision jobs |
| POST | `/provision-jobs` | Create provision job |
| GET | `/provision-jobs/{id}` | Get provision job |
| DELETE | `/provision-jobs/{id}` | Cancel provision job |
| GET | `/provision-jobs/{id}/logs` | Get provision logs |
| GET | `/provision-jobs/{id}/logs/stream` | SSE stream of logs |

Query params:
- `agent`: Filter by agent ID
- `status`: `pending`, `running`, `success`, `failed`

### Health Checks

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health-checks` | List health check configs |
| POST | `/health-checks` | Create health check |
| GET | `/health-checks/{name}` | Get health check |
| PUT | `/health-checks/{name}` | Update health check |
| DELETE | `/health-checks/{name}` | Delete health check |
| POST | `/health-checks/{name}/run` | Run health check now |
| GET | `/health-checks/status` | Get global health status |

---

## Security Resources

### Certificates

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/certificates` | List agent certificates |
| GET | `/certificates/{agent-id}` | Get certificate details |
| DELETE | `/certificates/{agent-id}` | Revoke certificate |
| GET | `/certificates/ca` | Get CA certificate info |
| POST | `/certificates/ca/rotate` | Rotate CA certificate |
| GET | `/certificates/server` | Get server certificate |
| GET | `/certificates/audit` | Certificate audit log |

### TLS

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/tls` | Get TLS configuration |
| PUT | `/tls` | Update TLS settings |
| POST | `/tls/renew` | Force ACME renewal |

### SSH Keys

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/ssh-keys` | List SSH keys |
| POST | `/ssh-keys` | Create/import SSH key |
| GET | `/ssh-keys/{id}` | Get SSH key (public part) |
| DELETE | `/ssh-keys/{id}` | Delete SSH key |
| POST | `/ssh-keys/generate` | Generate new SSH key pair |

### Host Keys

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/host-keys` | List known host keys |
| POST | `/host-keys` | Add host key |
| GET | `/host-keys/{id}` | Get host key |
| DELETE | `/host-keys/{id}` | Remove host key |
| POST | `/host-keys/scan` | Scan host for keys |

### Jump Servers

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/jump-servers` | List jump servers |
| POST | `/jump-servers` | Create jump server |
| GET | `/jump-servers/{id}` | Get jump server |
| PUT | `/jump-servers/{id}` | Update jump server |
| DELETE | `/jump-servers/{id}` | Delete jump server |
| POST | `/jump-servers/{id}/test` | Test connectivity |

### Credentials

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/credentials` | List credentials |
| POST | `/credentials` | Create credential |
| GET | `/credentials/{id}` | Get credential |
| PUT | `/credentials/{id}` | Update credential |
| DELETE | `/credentials/{id}` | Delete credential |
| POST | `/credentials/{id}/test` | Test credential |

### Blocked IPs

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/blocked-ips` | List blocked IPs |
| POST | `/blocked-ips` | Block IP |
| GET | `/blocked-ips/{ip}` | Get block details |
| DELETE | `/blocked-ips/{ip}` | Unblock IP |

---

## Recipe Resources

### Components

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/recipes/components` | List components |
| POST | `/recipes/components` | Create component |
| GET | `/recipes/components/{slug}` | Get component |
| PUT | `/recipes/components/{slug}` | Update component |
| DELETE | `/recipes/components/{slug}` | Delete component |

### Playbooks

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/recipes/playbooks` | List playbooks |
| POST | `/recipes/playbooks` | Create playbook |
| GET | `/recipes/playbooks/{slug}` | Get playbook |
| PUT | `/recipes/playbooks/{slug}` | Update playbook |
| DELETE | `/recipes/playbooks/{slug}` | Delete playbook |
| GET | `/recipes/playbooks/{slug}/variables` | Get playbook variables |
| POST | `/recipes/playbooks/customize` | Create customized playbook |

### Activations

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/recipes/activations` | List activations |
| POST | `/recipes/activations` | Create activation |
| GET | `/recipes/activations/{id}` | Get activation |
| PUT | `/recipes/activations/{id}` | Update activation |
| DELETE | `/recipes/activations/{id}` | Delete activation |

### Raw Approvals

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/recipes/raw-approvals` | List approvals |
| POST | `/recipes/raw-approvals` | Create approval |
| GET | `/recipes/raw-approvals/{id}` | Get approval |
| PUT | `/recipes/raw-approvals/{id}` | Update approval |
| DELETE | `/recipes/raw-approvals/{id}` | Delete approval |

### Import/Export

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/recipes/export` | Export all recipes |
| POST | `/recipes/import` | Import recipes |

### Migration

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/recipes/migration/{project}/preview` | Preview migration |
| POST | `/recipes/migration/{project}` | Execute migration |

---

## Real-time Endpoints

All SSE endpoints use `text/event-stream` content type.

### Event Stream

```
GET /events/stream?types=deployment,agent,audit
```

Query params:
- `types`: Comma-separated event types to subscribe
- `project`: Filter by project
- `agent`: Filter by agent

Event format:
```
event: deployment
data: {"id": "deploy-123", "status": "running", "project": "myapp", "target": "production"}

event: target_result
data: {"deployment_id": "deploy-123", "target": "production", "status": "success"}

event: agent
data: {"id": "agent-1", "status": "online"}
```

### Deployment Logs Stream

```
GET /deployments/{id}/logs/stream
```

Streams log lines as they're produced:
```
data: {"timestamp": "...", "level": "info", "message": "Starting deployment..."}
data: {"timestamp": "...", "level": "info", "message": "Pulling latest code..."}
```

### Provision Logs Stream

```
GET /provision-jobs/{id}/logs/stream
```

Same format as deployment logs.

---

## Bulk Operations

### Bulk Deploy

```
POST /deployments/bulk
```

Request:
```json
{
  "projects": [
    {
      "name": "app-1",
      "branch": "main",
      "targets": ["production"]
    },
    {
      "name": "app-2",
      "targets": ["staging", "production"]
    },
    {
      "name": "app-3",
      "all_targets": true
    }
  ],
  "wait": 30,
  "dry_run": false
}
```

Response:
```json
{
  "deployments": [
    { "project": "app-1", "id": "deploy-101", "status": "pending", "targets": ["production"] },
    { "project": "app-2", "id": "deploy-102", "status": "pending", "targets": ["staging", "production"] },
    { "project": "app-3", "id": "deploy-103", "status": "pending", "targets": ["web-1", "web-2"] }
  ]
}
```

### Bulk Cancel

```
DELETE /deployments/bulk
```

Request:
```json
{
  "ids": ["deploy-101", "deploy-102", "deploy-103"]
}
```

### Bulk Secret Import

```
POST /secrets/bulk
```

Request:
```json
{
  "scope": "project",
  "project": "myapp",
  "secrets": [
    { "key": "DB_HOST", "value": "localhost" },
    { "key": "DB_PORT", "value": "5432" }
  ]
}
```

### Bulk Agent Action

```
POST /agents/bulk
```

Request:
```json
{
  "ids": ["agent-1", "agent-2"],
  "action": "update",
  "data": {
    "groups": ["web-servers"]
  }
}
```

Actions: `update`, `delete`, `provision`
---

## Admin Resources

### Backup

All backup endpoints require admin role.

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/admin/backup/export` | Start backup export |
| GET | `/admin/backup/download/{token}` | Download export file |
| POST | `/admin/backup/import/upload` | Upload backup + compute diff |
| POST | `/admin/backup/import/execute` | Execute import with strategies |

Export request:
```json
{
  "passphrase": "my-secret-passphrase"
}
```

Export response:
```json
{
  "download_url": "/api/v1/admin/backup/download/abc123",
  "expires": "2025-01-15T11:00:00Z"
}
```

Import upload: `multipart/form-data` with `file` and `passphrase` fields.

Upload response:
```json
{
  "session_id": "import-abc123",
  "diff": {
    "tables": [
      { "name": "projects", "new": 3, "changed": 1, "only_in_current": 0 },
      { "name": "targets", "new": 5, "changed": 0, "only_in_current": 2 }
    ]
  }
}
```

Execute import request:
```json
{
  "session_id": "import-abc123",
  "strategies": {
    "projects": "merge",
    "targets": "replace",
    "secrets": "skip"
  }
}
```

Strategies: `merge` (add new, update changed), `replace` (drop + reimport), `skip` (ignore table).

### Maintenance

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/admin/maintenance` | Get maintenance mode status |
| POST | `/admin/maintenance` | Toggle maintenance mode |

Toggle request:
```json
{
  "enabled": true
}
```

Status response:
```json
{
  "enabled": true,
  "since": "2025-01-15T10:00:00Z",
  "message": "System backup in progress"
}
```

During maintenance: all mutation endpoints return `503 Service Unavailable` with `Retry-After: 1800` header. GET endpoints and maintenance/backup endpoints remain accessible.

---

## Appendix: Removed Endpoints

These endpoints are **removed** in favor of the canonical patterns documented above.

### Deployment Shortcuts (Use `POST /deployments`)

| Removed Endpoint | Replacement |
|------------------|-------------|
| `POST /projects/{name}/deploy` | `POST /deployments` with `project` field |
| `POST /projects/{name}/rollback` | `POST /deployments` with `type: rollback` |
| `GET /rollbacks` | `GET /deployments?type=rollback` |
| `GET /rollbacks/{id}` | `GET /deployments/{id}` |
| `POST /rollbacks/{id}/cancel` | `DELETE /deployments/{id}` |
| `POST /deployments/{id}/cancel` | `DELETE /deployments/{id}` |

### Provision Shortcuts (Use `/provision-jobs`)

| Removed Endpoint | Replacement |
|------------------|-------------|
| `POST /agents/{id}/provision` | `POST /provision-jobs` with `agent_id` |
| `GET /agents/{id}/provision` | `GET /provision-jobs?agent={id}` |

### TOTP Consolidation (Use `/users/*/totp`)

| Removed Endpoint | Replacement |
|------------------|-------------|
| `POST /totp/setup` | `POST /users/me/totp/setup` |
| `POST /totp/enable` | `PUT /users/me/totp` |
| `POST /totp/disable` | `DELETE /users/me/totp` |
| `POST /totp/recovery/regenerate` | `POST /users/me/totp/recovery` |
| `GET /admin/totp/users` | `GET /users?totp=enabled` |
| `GET /admin/totp/status/{id}` | `GET /users/{id}/totp` |
| `POST /admin/totp/disable` | `DELETE /users/{id}/totp` |

### TLS Consolidation (Use `/tls`)

| Removed Endpoint | Replacement |
|------------------|-------------|
| `GET /tls/status` | `GET /tls` |
| `PUT /tls/settings` | `PUT /tls` |

### Health Check Consolidation

| Removed Endpoint | Replacement |
|------------------|-------------|
| `GET /health-checks/global` | `GET /health-checks/status` |

### Deprecated Probes

| Deprecated | Replacement | Notes |
|------------|-------------|-------|
| `GET /healthz` | `GET /livez` | Returns deprecation header |

---

## Appendix: Route Renames

| Old Route | New Route | Reason |
|-----------|-----------|--------|
| `/blocked` | `/blocked-ips` | Clarity: describes what's blocked |