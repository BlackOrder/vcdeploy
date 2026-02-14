# REST API Reference

vcdeploy provides a comprehensive REST API for managing deployments, projects, agents, and system configuration.

## Base URL

All API endpoints are relative to your vcdeploy master server URL:

- **Local development:** `http://localhost:9000`
- **Production:** Your configured server URL (via `server.listen` in master config)

## Authentication

Most endpoints require authentication using one of the following methods:

### Bearer Token (Recommended for API)

```bash
curl -H "Authorization: Bearer <token>" https://vcdeploy.example.com/api/v1/projects
```

### API Key

```bash
curl -H "X-API-Key: <api-key>" https://vcdeploy.example.com/api/v1/projects
```

### Session Cookie (Web UI)

When using the web interface, authentication is handled via session cookies.

## Request/Response Format

- All request and response bodies use JSON format
- Include `Content-Type: application/json` header for POST/PUT requests
- All responses include an `X-Request-ID` header for correlation

## Rate Limiting

API requests are rate limited. When exceeded, endpoints return `429 Too Many Requests` with a `Retry-After` header.

## Role-Based Access

| Role | Permissions |
|------|-------------|
| admin | Full access to all endpoints |
| user | Read access + create deployments |
| viewer | Read-only access |

## Common Response Codes

| Code | Description |
|------|-------------|
| 200 | Success |
| 201 | Created |
| 204 | No Content (successful deletion) |
| 400 | Bad Request - invalid input |
| 401 | Unauthorized - authentication required |
| 403 | Forbidden - insufficient permissions |
| 404 | Not Found |
| 409 | Conflict - resource already exists |
| 429 | Too Many Requests - rate limited |
| 500 | Internal Server Error |

## Error Responses

All error responses follow this format:

```json
{
  "error": "error_code",
  "message": "Human-readable error description",
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

---

## Endpoint Overview

| Category | Endpoints | Auth Required |
|----------|-----------|---------------|
| Health | `/healthz`, `/livez`, `/readyz`, `/api/v1/health` | No (except detailed) |
| Metrics | `/metrics` | No |
| Auth | `/api/v1/auth/login` | No |
| Stats | `/api/v1/stats` | Yes |
| Users | `/api/v1/users`, `/api/v1/users/{id}` | Admin |
| Settings | `/api/v1/settings/{category}`, export/import | Admin |
| Projects | `/api/v1/projects`, `/api/v1/projects/{id}` | Yes |
| Deployments | `/api/v1/deployments`, `/api/v1/deployments/{id}` | Yes |
| Agents | `/api/v1/agents`, `/api/v1/agents/{id}` | Yes |
| Secrets | `/api/v1/secrets` | Yes |
| Project Types | `/api/v1/project-types`, `/api/v1/project-types/{id}` | Yes |
| API Keys | `/api/v1/api-keys`, `/api/v1/api-keys/{id}` | Admin |
| Audit | `/api/v1/audit` | Admin |
| Host Keys | `/api/v1/host-keys`, `/api/v1/host-keys/{id}` | Yes/Admin |
| Jump Servers | `/api/v1/jump-servers`, `/api/v1/jump-servers/{id}` | Yes/Admin |
| Blocked IPs | `/api/v1/blocked-ips`, `/api/v1/blocked-ips/{ip}` | Admin |
| Provision | `/api/v1/provision-jobs`, `/api/v1/provision-jobs/{id}` | Admin |
| Agent Binaries | `/api/v1/binaries`, `/api/v1/binaries/latest` | Yes |
| Certificates | `/api/v1/certificates`, `/api/v1/certificates/{id}` | Admin |
| TLS | `/api/v1/tls`, `/api/v1/tls/renew` | Admin |
| Credentials | `/api/v1/credentials`, `/api/v1/credentials/{id}` | Admin |
| SSH Keys | `/api/v1/ssh-keys`, `/api/v1/ssh-keys/{id}` | Admin |
| Health Checks | `/api/v1/health-checks`, `/api/v1/health-checks/global` | Yes |
| Rollbacks | `/api/v1/rollbacks`, `/api/v1/rollbacks/{id}` | Yes |
| Recipes | `/api/v1/recipes/components`, `/api/v1/recipes/playbooks` | Yes |
| Activations | `/api/v1/recipes/activations` | Yes |
| RAW Approvals | `/api/v1/recipes/raw-approvals` | Admin |
| Webhooks | `/webhook/github/{project}`, `/webhook/gitlab/{project}`, `/webhook/bitbucket/{project}` | Signature |

---

## Health Endpoints

### Liveness Probe

```
GET /healthz
GET /livez
```

Returns `200 OK` if the process is alive. No authentication required. Used by Kubernetes liveness probes.

**Response:**
```
ok
```

### Readiness Probe

```
GET /readyz
```

Returns `200 OK` if the server can serve traffic, `503 Service Unavailable` otherwise. Used by Kubernetes readiness probes.

**Response:**
```
ok
```

### Detailed Health Status

```
GET /api/v1/health
```

Returns detailed health information about all subsystems.

**Response:**
```json
{
  "status": "healthy",
  "checks": {
    "database": {
      "status": "healthy",
      "message": "connected",
      "latency": "1.234ms"
    },
    "grpc": {
      "status": "healthy",
      "message": "listening on :9001"
    },
    "agents": {
      "status": "healthy",
      "message": "3 agents connected"
    }
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Prometheus Metrics

```
GET /metrics
```

Returns Prometheus-formatted metrics for monitoring integration. Includes deployment counts, agent stats, request latencies, and more.

---

## Authentication Endpoints

### Login

```
POST /api/v1/auth/login
```

Authenticate with username and password to receive a JWT token.

**Request:**
```json
{
  "username": "admin",
  "password": "your-password"
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": {
    "id": 1,
    "username": "admin",
    "email": "admin@example.com",
    "role": "admin"
  }
}
```

---

## TOTP (Two-Factor Authentication) Endpoints

### Setup TOTP

```
POST /api/v1/totp/setup
```

Initialize TOTP setup for the current user. Returns a secret and URI for authenticator app configuration.

**Response:**
```json
{
  "secret": "JBSWY3DPEHPK3PXP",
  "uri": "otpauth://totp/vcdeploy:username?secret=JBSWY3DPEHPK3PXP&issuer=vcdeploy"
}
```

> **Note:** Recovery codes are returned from `/api/v1/totp/enable`, not `/setup`.

### Enable TOTP

```
POST /api/v1/totp/enable
```

Enable TOTP after verifying setup with a valid code. Returns recovery codes.

**Request:**
```json
{
  "secret": "JBSWY3DPEHPK3PXP",
  "totp_code": "123456"
}
```

**Response:**
```json
{
  "message": "TOTP enabled successfully",
  "recovery_codes": ["XXXXXXXX", "YYYYYYYY", "ZZZZZZZZ", "..."]
}
```

### Disable TOTP

```
POST /api/v1/totp/disable
```

Disable TOTP for the current user.

**Request:**
```json
{
  "code": "123456"
}
```

**Response:**
```json
{
  "enabled": false,
  "message": "TOTP disabled successfully"
}
```

### Regenerate Recovery Codes

```
POST /api/v1/totp/recovery/regenerate
```

Generate new recovery codes. Previous codes are invalidated.

**Request:**
```json
{
  "code": "123456"
}
```

**Response:**
```json
{
  "recovery_codes": ["XXXX-XXXX-XXXX", "YYYY-YYYY-YYYY", ...]
}
```

---

## Admin TOTP Endpoints

### List Users with TOTP

```
GET /api/v1/admin/totp/users
```

Returns all users with their TOTP status. **Admin only.**

**Response:**
```json
{
  "users": [
    {
      "id": 1,
      "username": "admin",
      "totp_enabled": true,
      "totp_configured_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

### Get User TOTP Status

```
GET /api/v1/admin/totp/status/{username}
```

Returns detailed TOTP status for a specific user. **Admin only.**

**Response:**
```json
{
  "username": "user1",
  "totp_enabled": true,
  "totp_configured_at": "2024-01-15T10:30:00Z",
  "recovery_codes_remaining": 5
}
```

### Admin Disable TOTP

```
POST /api/v1/admin/totp/disable
```

Forcibly disable TOTP for a user (e.g., if they lost their device). **Admin only.**

**Request:**
```json
{
  "username": "user1"
}
```

**Response:**
```json
{
  "message": "TOTP disabled for user user1"
}
```

---

## Stats Endpoint

### Dashboard Statistics

```
GET /api/v1/stats
```

Returns aggregated statistics for the dashboard.

**Response:**
```json
{
  "projects": {
    "total": 12
  },
  "agents": {
    "total": 5,
    "connected": 4
  },
  "deployments": {
    "success": 87,
    "failed": 3,
    "running": 1,
    "total": 91
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

## User Management Endpoints

### List Users

```
GET /api/v1/users
```

Returns all users. **Admin only.**

**Response:**
```json
[
  {
    "id": 1,
    "username": "admin",
    "email": "admin@example.com",
    "role": "admin",
    "createdAt": "2024-01-01T00:00:00Z"
  }
]
```

### Create User

```
POST /api/v1/users
```

**Admin only.**

**Request:**
```json
{
  "username": "newuser",
  "email": "user@example.com",
  "password": "secure-password",
  "role": "user",
  "totp_enabled": false,
  "totp_secret": ""
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `username` | string | Yes | Unique username |
| `email` | string | No | User's email address |
| `password` | string | Yes | Password (must meet complexity requirements) |
| `role` | string | No | One of: `admin`, `user`, `viewer`. Default: `user` |
| `totp_enabled` | boolean | No | Enable TOTP 2FA for this user. Default: `false` |
| `totp_secret` | string | No | TOTP secret (required if `totp_enabled` is `true`) |
```

### Get User

```
GET /api/v1/users/{id}
```

**Admin only.**

### Update User

```
PUT /api/v1/users/{id}
```

**Admin only.**

**Request:**
```json
{
  "email": "newemail@example.com",
  "role": "admin",
  "password": "new-password"
}
```

### Delete User

```
DELETE /api/v1/users/{id}
```

**Admin only.**

---

## Settings Endpoints

### Get Settings Category

```
GET /api/v1/settings/{category}
```

Returns all settings for a category.

**Response:**
```json
{
  "key1": "value1",
  "key2": "value2"
}
```

### Update Settings Category

```
PUT /api/v1/settings/{category}
```

**Admin only.**

**Request:**
```json
{
  "key1": "new-value",
  "key2": "another-value"
}
```

### Export Settings

```
GET /api/v1/settings/export
```

Exports all settings as JSON. **Admin only.**

### Import Settings

```
POST /api/v1/settings/import
```

Imports settings from JSON. **Admin only.**

---

## Project Endpoints

### List Projects

```
GET /api/v1/projects
```

Returns all projects the user has access to.

**Response:**
```json
[
  {
    "id": 1,
    "name": "my-app",
    "description": "My application",
    "type_id": 1,
    "repo_url": "https://github.com/org/my-app",
    "branch": "main",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
]
```

### Create Project

```
POST /api/v1/projects
```

**Request:**
```json
{
  "name": "my-app",
  "description": "My application",
  "type_id": 1,
  "repo_url": "https://github.com/org/my-app",
  "branch": "main"
}
```

### Get Project

```
GET /api/v1/projects/{id}
```

### Update Project

```
PUT /api/v1/projects/{id}
```

### Delete Project

```
DELETE /api/v1/projects/{id}
```

**Admin only.**

---

## Deployment Endpoints

### List Deployments

```
GET /api/v1/deployments
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| project_id | integer | Filter by project ID |
| status | string | Filter by status (pending, running, success, failed, cancelled) |
| limit | integer | Max results (default: 50) |
| offset | integer | Pagination offset |

**Response:**
```json
[
  {
    "id": 1,
    "project_id": 1,
    "project_name": "my-app",
    "status": "success",
    "version": "v1.2.3",
    "commit": "abc123def",
    "triggered_by": "admin",
    "started_at": "2024-01-15T10:30:00Z",
    "finished_at": "2024-01-15T10:32:00Z"
  }
]
```

### Create Deployment

```
POST /api/v1/deployments
```

**Request:**
```json
{
  "project_id": 1,
  "version": "v1.2.3",
  "commit": "abc123def",
  "force": false
}
```

### Get Deployment

```
GET /api/v1/deployments/{id}
```

### Cancel Deployment

```
POST /api/v1/deployments/{id}/cancel
```

### Get Deployment Logs

```
GET /api/v1/deployments/{id}/logs
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| after_id | integer | Return logs after this ID (for polling) |

---

## Agent Endpoints

### List Agents

```
GET /api/v1/agents
```

**Response:**
```json
[
  {
    "id": 1,
    "name": "prod-server-1",
    "hostname": "server1.example.com",
    "status": "connected",
    "version": "1.0.0",
    "os": "linux",
    "arch": "amd64",
    "tags": ["production", "web"],
    "last_seen_at": "2024-01-15T10:30:00Z"
  }
]
```

### Get Agent

```
GET /api/v1/agents/{id}
```

### Delete Agent

```
DELETE /api/v1/agents/{id}
```

**Admin only.** Removes agent registration.

---

## Secret Endpoints

### List Secrets

```
GET /api/v1/secrets
```

Returns metadata only - values are never exposed.

**Response:**
```json
[
  {
    "key": "DATABASE_URL",
    "project_id": null,
    "scope": "global",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
]
```

### Create/Update Secret

```
POST /api/v1/secrets
```

**Request:**
```json
{
  "key": "DATABASE_URL",
  "value": "postgres://user:pass@host/db",
  "project_id": null,
  "scope": "global"
}
```

### Delete Secret

```
DELETE /api/v1/secrets/{key}
```

---

## Project Type Endpoints

### List Project Types

```
GET /api/v1/project-types
```

**Response:**
```json
[
  {
    "id": 1,
    "name": "laravel",
    "description": "Laravel PHP Framework",
    "default_hooks": {...}
  }
]
```

### Create Project Type

```
POST /api/v1/project-types
```

**Admin only.**

### Get/Update/Delete Project Type

```
GET|PUT|DELETE /api/v1/project-types/{id}
```

---

## API Key Endpoints

### List API Keys

```
GET /api/v1/api-keys
```

**Admin only.** Returns all API keys (tokens are masked).

### Create API Key

```
POST /api/v1/api-keys
```

**Admin only.**

**Request:**
```json
{
  "name": "CI/CD Pipeline",
  "scopes": ["read", "deploy"],
  "expires_at": "2025-01-01T00:00:00Z"
}
```

**Response includes the full token (only shown once):**
```json
{
  "id": 1,
  "name": "CI/CD Pipeline",
  "token": "vcd_abc123...",
  "scopes": ["read", "deploy"],
  "created_at": "2024-01-15T10:30:00Z"
}
```

### Delete API Key

```
DELETE /api/v1/api-keys/{id}
```

**Admin only.**

---

## Audit Endpoints

### List Audit Logs

```
GET /api/v1/audit
```

**Admin only.** Returns audit trail of all actions.

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| action | string | Filter by action type |
| resource | string | Filter by resource type |
| user | string | Filter by username |
| limit | integer | Max results |
| offset | integer | Pagination offset |

---

## SSH Host Key Endpoints

### List Host Keys

```
GET /api/v1/host-keys
```

Returns known SSH host keys.

### Create Host Key

```
POST /api/v1/host-keys
```

**Admin only.**

**Request:**
```json
{
  "hostname": "server.example.com",
  "port": 22,
  "key_type": "ssh-ed25519",
  "public_key": "AAAAC3NzaC1lZDI1NTE5...",
  "fingerprint": "SHA256:...",
  "trusted": true
}
```

### Update Host Key Trust

```
PUT /api/v1/host-keys/{id}
```

**Admin only.**

**Request:**
```json
{
  "trusted": true
}
```

### Delete Host Key

```
DELETE /api/v1/host-keys/{id}
```

**Admin only.**

---

## Jump Server Endpoints

### List Jump Servers

```
GET /api/v1/jump-servers
```

Returns configured SSH jump/bastion servers.

### Create Jump Server

```
POST /api/v1/jump-servers
```

**Admin only.**

**Request:**
```json
{
  "name": "bastion-1",
  "host": "bastion.example.com",
  "port": 22,
  "username": "deploy",
  "ssh_key_id": 1
}
```

### Get/Update/Delete Jump Server

```
GET|PUT|DELETE /api/v1/jump-servers/{id}
```

---

## Blocked IP Endpoints

### List Blocked IPs

```
GET /api/v1/blocked-ips
```

**Admin only.**

### Block IP

```
POST /api/v1/blocked-ips
```

**Admin only.**

**Request:**
```json
{
  "ip_address": "192.168.1.100",
  "reason": "Brute force attempts",
  "duration": "24h"
}
```

### Unblock IP

```
DELETE /api/v1/blocked-ips/{ip}
```

**Admin only.**

---

## Provision Endpoints

### List Provision Jobs

```
GET /api/v1/provision
```

Returns agent provisioning jobs.

### Create Provision Job

```
POST /api/v1/provision
```

**Admin only.** Provisions a new agent on a target server.

### Get Provision Job

```
GET /api/v1/provision/{id}
```

---

## Agent Binary Endpoints

### List Agent Binaries

```
GET /api/v1/binaries
```

Returns available agent binary versions.

### Get Latest Binary

```
GET /api/v1/binaries/latest
```

Returns the latest agent binary for download.

### Get Specific Binary

```
GET /api/v1/binaries/{version}
```

---

## Health Check Configuration Endpoints

### List Health Check Configs

```
GET /api/v1/health-checks
```

### Get Global Health Check Config

```
GET /api/v1/health-checks/global
```

### Get/Update/Delete Health Check Config

```
GET|PUT|DELETE /api/v1/health-checks/{id}
```

---

## Rollback Endpoints

### List Rollback Records

```
GET /api/v1/rollbacks
```

Returns deployment rollback history.

### Get Rollback Record

```
GET /api/v1/rollbacks/{id}
```

---

## Incoming Webhook Endpoints

These endpoints receive webhooks from Git providers to trigger deployments.

### GitHub Webhook

```
POST /webhook/github/{project}
```

**Required Headers:**

| Header | Description |
|--------|-------------|
| X-Hub-Signature-256 | HMAC-SHA256 signature |
| X-GitHub-Event | Event type (push, etc.) |

### GitLab Webhook

```
POST /webhook/gitlab/{project}
```

**Required Headers:**

| Header | Description |
|--------|-------------|
| X-Gitlab-Token | Secret token for verification |
| X-Gitlab-Event | Event type (Push Hook, etc.) |

### Bitbucket Webhook

```
POST /webhook/bitbucket/{project}
```

**Required Headers:**

| Header | Description |
|--------|-------------|
| X-Hub-Signature | HMAC signature |
| X-Event-Key | Event type (repo:push, etc.) |

---

## Recipe Endpoints

The Recipe API manages deployment components, playbooks, and activations.

### List Components

```
GET /api/v1/recipes/components
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| namespace | string | Filter by namespace (seed/user) |
| type | string | Filter by component type |
| search | string | Search by name or description |

**Response:**
```json
{
  "components": [
    {
      "id": 1,
      "namespace": "seed",
      "slug": "composer-install",
      "version": "v1.0.0",
      "name": "Composer Install",
      "type": "shell",
      "content": { "command": "composer install --no-dev" }
    }
  ],
  "total": 1
}
```

### Create Component

```
POST /api/v1/recipes/components
```

**Request Body:**
```json
{
  "name": "NPM Install",
  "slug": "npm-install",
  "type": "shell",
  "content": {
    "command": "npm ci --production"
  },
  "variables": [
    {
      "name": "node_env",
      "type": "string",
      "default": "production"
    }
  ]
}
```

### Get Component

```
GET /api/v1/recipes/components/{id}
```

### Update Component

```
PUT /api/v1/recipes/components/{id}
```

### Delete Component

```
DELETE /api/v1/recipes/components/{id}
```

### List Playbooks

```
GET /api/v1/recipes/playbooks
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| namespace | string | Filter by namespace |
| framework_type | string | Filter by framework |

### Create Playbook

```
POST /api/v1/recipes/playbooks
```

**Request Body:**
```json
{
  "name": "Laravel Deployment",
  "slug": "laravel-deploy",
  "version": "v1.0.0",
  "framework_type": "laravel",
  "steps": [
    {
      "order": 1,
      "phase": "pre_deploy",
      "component_ref": "seed:composer-install:v1.0.0"
    },
    {
      "order": 2,
      "phase": "post_deploy",
      "component_ref": "seed:artisan-migrate:v1.0.0"
    }
  ]
}
```

### Get Playbook

```
GET /api/v1/recipes/playbooks/{id}
```

### Update Playbook

```
PUT /api/v1/recipes/playbooks/{id}
```

### Delete Playbook

```
DELETE /api/v1/recipes/playbooks/{id}
```

### Activate Playbook

```
POST /api/v1/recipes/activations
```

**Request Body:**
```json
{
  "project_id": 123,
  "playbook_id": 1,
  "bindings": [
    {
      "variable_name": "deploy_user",
      "source_type": "literal",
      "literal_value": "www-data"
    },
    {
      "variable_name": "db_password",
      "source_type": "secret",
      "source_ref": "db-password"
    }
  ]
}
```

### Get Activation

```
GET /api/v1/recipes/activations/{project_id}
```

### Deactivate Playbook

```
DELETE /api/v1/recipes/activations/{project_id}
```

### RAW Approvals (Admin Only)

#### List Pending Approvals

```
GET /api/v1/recipes/raw-approvals
```

#### Approve RAW Component

```
POST /api/v1/recipes/raw-approvals
```

**Request Body:**
```json
{
  "component_id": 42,
  "approval_note": "Reviewed and safe for production use"
}
```

#### Revoke Approval

```
DELETE /api/v1/recipes/raw-approvals/{component_id}
```

### Export Recipes

```
GET /api/v1/recipes/export
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| include_versions | boolean | Include version history |
| namespace | string | Filter by namespace |

### Import Recipes

```
POST /api/v1/recipes/import
```

**Request Body:** multipart/form-data with recipe JSON file

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| dry_run | boolean | Preview import without changes |

### Migration Preview

```
GET /api/v1/recipes/migration/preview/{project_id}
```

Returns estimated migration details for converting a project's YAML config to a playbook.

### Migrate Project

```
POST /api/v1/recipes/migration/{project_id}
```

**Request Body:**
```json
{
  "name": "My Playbook",
  "version": "v1.0.0",
  "createComponents": true,
  "activate": true
}
```

---

## Certificate Management

Manage TLS certificates for agent authentication.

### List Certificates

```
GET /api/v1/certificates
```

Returns list of all agent certificates.

**Response:**
```json
{
  "items": [
    {
      "id": 1,
      "agentId": "agent-001",
      "serialNumber": "ABC123DEF456",
      "issuedAt": "2026-01-01T00:00:00Z",
      "expiresAt": "2027-01-01T00:00:00Z",
      "status": "active"
    }
  ],
  "totalCount": 1
}
```

### Get Certificate

```
GET /api/v1/certificates/{id}
```

Returns details for a specific certificate.

### Revoke Certificate

```
POST /api/v1/certificates/agents/{agent_id}/revoke
```

Revoke an agent's certificate, preventing it from authenticating.

**Request Body:**
```json
{
  "reason": "Agent decommissioned"
}
```

### Certificate Audit Log

```
GET /api/v1/certificates/audit
```

Returns certificate issuance and revocation history.

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| limit | integer | Maximum entries to return (default: 50) |
| offset | integer | Pagination offset |

---

## TLS Management

Manage TLS configuration for the master server.

### Get TLS Status and Settings

```
GET /api/v1/tls
```

Returns current TLS configuration, certificate status, and ACME settings.

**Response:**
```json
{
  "enabled": true,
  "mode": "acme",
  "acme_domain": "vcdeploy.example.com",
  "certificate_expires": "2026-06-01T00:00:00Z",
  "certificate_issuer": "Let's Encrypt"
}
```

### Update TLS Settings

```
PUT /api/v1/tls
```

Update TLS configuration.

**Request Body:**
```json
{
  "enabled": true,
  "mode": "acme",
  "acme_domain": "vcdeploy.example.com",
  "acme_email": "admin@example.com"
}
```

### Force Certificate Renewal

```
POST /api/v1/tls/renew
```

Forces immediate ACME certificate renewal.

---

## Source Credentials

Manage credentials for Git repository authentication.

### List Credentials

```
GET /api/v1/credentials
```

Returns list of configured source credentials.

**Response:**
```json
{
  "items": [
    {
      "id": 1,
      "name": "github-token",
      "type": "token",
      "urlPattern": "github.com/*",
      "createdAt": "2026-01-01T00:00:00Z"
    }
  ],
  "totalCount": 1
}
```

### Create Credential

```
POST /api/v1/credentials
```

Create a new source credential.

**Request Body (Token):**
```json
{
  "name": "github-pat",
  "type": "token",
  "urlPattern": "github.com/myorg/*",
  "token": "ghp_xxxxxxxxxxxxxxxxxxxx"
}
```

**Request Body (Basic Auth):**
```json
{
  "name": "gitlab-creds",
  "type": "basic",
  "urlPattern": "gitlab.com/myorg/*",
  "username": "deploy-user",
  "password": "secret"
}
```

**Request Body (SSH Key Reference):**
```json
{
  "name": "ssh-deploy",
  "type": "ssh",
  "urlPattern": "git@github.com:myorg/*",
  "sshKeyId": 123
}
```

### Get Credential

```
GET /api/v1/credentials/{id}
```

Returns credential details (secrets are redacted).

### Delete Credential

```
DELETE /api/v1/credentials/{id}
```

Delete a source credential.

### Test Credential

```
POST /api/v1/credentials/{id}/test
```

Test credential connectivity against a repository.

**Request Body:**
```json
{
  "repositoryUrl": "https://github.com/myorg/myrepo.git"
}
```

---

## SSH Keys

Manage SSH keys for provisioning and Git authentication.

### List SSH Keys

```
GET /api/v1/ssh-keys
```

Returns list of managed SSH keys.

**Response:**
```json
{
  "items": [
    {
      "id": 1,
      "name": "deploy-key",
      "type": "ed25519",
      "fingerprint": "SHA256:xxxxx",
      "createdAt": "2026-01-01T00:00:00Z"
    }
  ],
  "totalCount": 1
}
```

### Generate SSH Key

```
POST /api/v1/ssh-keys
```

Generate a new Ed25519 SSH key pair.

**Request Body:**
```json
{
  "name": "new-deploy-key",
  "comment": "Deployment key for production"
}
```

**Response:**
```json
{
  "id": 2,
  "name": "new-deploy-key",
  "type": "ed25519",
  "fingerprint": "SHA256:xxxxx",
  "publicKey": "ssh-ed25519 AAAA... comment"
}
```

### Import SSH Key

```
POST /api/v1/ssh-keys/import
```

Import an existing SSH private key.

**Request Body:**
```json
{
  "name": "existing-key",
  "privateKey": "-----BEGIN OPENSSH PRIVATE KEY-----\n...",
  "passphrase": "optional-passphrase"
}
```

### Get Public Key

```
GET /api/v1/ssh-keys/{id}/public
```

Returns the public key in OpenSSH format.

**Response:**
```
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG... comment
```

### Delete SSH Key

```
DELETE /api/v1/ssh-keys/{id}
```

Delete an SSH key.

---

## OpenAPI Specification

The full OpenAPI 3.1 specification is available at:

- [openapi.yaml](api/openapi.yaml) - Download the spec file
- Use tools like [Swagger UI](https://swagger.io/tools/swagger-ui/) or [ReDoc](https://github.com/Redocly/redoc) to explore interactively

### Using with Swagger UI

```bash
docker run -p 8081:8080 -e SWAGGER_JSON=/spec/openapi.yaml \
  -v $(pwd)/docs/api:/spec swaggerapi/swagger-ui
```

Then open http://localhost:8081 to explore the API interactively.
