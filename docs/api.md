# API Reference

vcdeploy provides a comprehensive REST API for managing deployments, projects, agents, and system configuration.

## Base URL

All API endpoints are relative to your vcdeploy master server URL:

- **Local development:** `http://localhost:9000`
- **Production:** Your configured server URL

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

## Health Endpoints

### Liveness Probe

```
GET /healthz
```

Returns `200 OK` if the process is alive. No authentication required.

**Response:**
```
ok
```

### Readiness Probe

```
GET /readyz
```

Returns `200 OK` if the server can serve traffic, `503 Service Unavailable` otherwise.

**Response:**
```
ok
```

### Detailed Health Status

```
GET /api/v1/health
```

Returns detailed health information.

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
      "message": "3 agents online"
    }
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Prometheus Metrics

```
GET /metrics
```

Returns Prometheus-formatted metrics for monitoring integration.

---

## Authentication Endpoints

### Login

```
POST /api/v1/auth/login
```

Authenticate with username and password.

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
    "id": "proj_abc123",
    "name": "my-app",
    "description": "My application",
    "repo_url": "https://github.com/org/my-app",
    "branch": "main",
    "deploy_path": "/var/www/my-app",
    "agents": ["agent_1", "agent_2"],
    "created_at": "2024-01-15T10:30:00Z"
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
  "repo_url": "https://github.com/org/my-app",
  "branch": "main",
  "deploy_path": "/var/www/my-app",
  "agents": ["agent_1", "agent_2"]
}
```

### Get Project

```
GET /api/v1/projects/{projectId}
```

### Update Project

```
PUT /api/v1/projects/{projectId}
```

### Delete Project

```
DELETE /api/v1/projects/{projectId}
```

---

## Deployment Endpoints

### List Deployments

```
GET /api/v1/deployments
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| project_id | string | Filter by project |
| status | string | Filter by status (pending, running, success, failed, cancelled) |
| limit | integer | Max results (default: 50) |
| offset | integer | Pagination offset |

**Response:**
```json
[
  {
    "id": "deploy_xyz789",
    "project_id": "proj_abc123",
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
  "project_id": "proj_abc123",
  "version": "v1.2.3",
  "commit": "abc123def",
  "force": false
}
```

### Get Deployment

```
GET /api/v1/deployments/{deploymentId}
```

### Cancel Deployment

```
POST /api/v1/deployments/{deploymentId}/cancel
```

### Get Deployment Logs

```
GET /api/v1/deployments/{deploymentId}/logs
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| after_id | integer | Return logs after this ID (for polling) |

**Response:**
```json
[
  {
    "id": 1,
    "deployment_id": "deploy_xyz789",
    "timestamp": "2024-01-15T10:30:00Z",
    "level": "info",
    "message": "Starting deployment...",
    "source": "master"
  }
]
```

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
    "id": "agent_1",
    "hostname": "server1.example.com",
    "status": "online",
    "version": "1.0.0",
    "os": "linux",
    "arch": "amd64",
    "last_seen_at": "2024-01-15T10:30:00Z",
    "labels": {
      "env": "production",
      "region": "us-east-1"
    },
    "stats": {
      "cpu_percent": 25.5,
      "memory_percent": 60.2,
      "disk_percent": 45.0
    }
  }
]
```

### Get Agent

```
GET /api/v1/agents/{agentId}
```

### Delete Agent

```
DELETE /api/v1/agents/{agentId}
```

---

## Secret Endpoints

### List Secrets

```
GET /api/v1/secrets
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| project | string | Filter by project |
| scope | string | Filter by scope (global, project, environment) |

**Response:**
```json
[
  {
    "key": "DATABASE_URL",
    "project": "",
    "scope": "global",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
]
```

> **Note:** Secret values are never returned in API responses.

### Create/Update Secret

```
POST /api/v1/secrets
```

**Request:**
```json
{
  "key": "DATABASE_URL",
  "value": "postgres://user:pass@host/db",
  "project": "",
  "scope": "global"
}
```

### Delete Secret

```
DELETE /api/v1/secrets/{key}?scope=global
```

---

## Webhook Endpoints

### GitHub Webhook

```
POST /api/v1/webhooks/github
```

Receives webhook events from GitHub to trigger deployments.

**Required Headers:**

| Header | Description |
|--------|-------------|
| X-Hub-Signature-256 | HMAC signature for verification |
| X-GitHub-Event | Event type (push, pull_request, etc.) |

Configure this endpoint in your GitHub repository settings to automatically deploy on push events.

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
