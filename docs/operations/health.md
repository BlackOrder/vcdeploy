# Health Checks

vcdeploy provides comprehensive health check capabilities at multiple levels.

## Server Health Endpoints

### Kubernetes-Style Probes

vcdeploy exposes standard Kubernetes health endpoints:

| Endpoint | Purpose | Returns |
|----------|---------|---------|
| `/healthz` | Liveness probe | 200 if process is alive |
| `/livez` | Liveness probe (alias) | 200 if process is alive |
| `/readyz` | Readiness probe | 200 if ready to serve traffic |

#### Liveness Probes (`/healthz`, `/livez`)

These endpoints return `200 OK` if the process is running. They do not check dependencies.

```bash
curl -i http://localhost:8080/healthz
# HTTP/1.1 200 OK
# ok
```

#### Readiness Probe (`/readyz`)

This endpoint checks if the server can serve traffic by verifying database connectivity.

```bash
curl -i http://localhost:8080/readyz
# HTTP/1.1 200 OK
# ok

# If database is unavailable:
# HTTP/1.1 503 Service Unavailable
# database not ready
```

### Detailed Health (`/api/v1/health`)

The detailed health endpoint provides comprehensive status information:

```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/health
```

Response:
```json
{
  "status": "healthy",
  "timestamp": "2025-01-20T12:00:00Z",
  "checks": {
    "database": {
      "status": "healthy",
      "latency": "1.234ms"
    },
    "grpc": {
      "status": "healthy"
    },
    "agents": {
      "status": "healthy",
      "message": "5/5 agents connected"
    }
  }
}
```

Status values:
- `healthy` - All checks passing
- `degraded` - Some non-critical checks failing
- `unhealthy` - Critical checks failing

## Project Health Checks

Configure health checks for deployed applications:

```yaml
# Project configuration
health_check:
  enabled: true
  url: "http://localhost:8080/health"
  method: GET
  timeout: 30s
  interval: 10s
  retries: 3
  expected_status: 200
  expected_body: "ok"
```

### Health Check Types

#### HTTP Health Check

```yaml
health_check:
  type: http
  url: "http://localhost:8080/health"
  method: GET
  timeout: 10s
  headers:
    Authorization: "Bearer ${HEALTH_TOKEN}"
  expected_status: 200
```

#### TCP Health Check

```yaml
health_check:
  type: tcp
  host: localhost
  port: 3306
  timeout: 5s
```

#### Command Health Check

```yaml
health_check:
  type: command
  command: "/usr/local/bin/health-check.sh"
  timeout: 30s
  expected_exit_code: 0
```

## Agent Health Monitoring

The master server continuously monitors agent health:

### Connection Status

```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/agents
```

Response includes health metrics:
```json
{
  "agents": [
    {
      "id": "agent-001",
      "status": "connected",
      "last_seen": "2025-01-20T12:00:00Z",
      "metrics": {
        "cpu_percent": 45.2,
        "memory_percent": 62.1,
        "disk_percent": 34.5
      }
    }
  ]
}
```

### Health Alerts

Configure alerts for agent health:

```yaml
# Master configuration
alerts:
  agent_disconnected:
    threshold: 5m
    severity: critical
  agent_high_cpu:
    threshold: 90
    duration: 10m
    severity: warning
  agent_high_memory:
    threshold: 90
    duration: 10m
    severity: warning
  agent_high_disk:
    threshold: 90
    duration: 5m
    severity: critical
```

## Deployment Health Verification

After deployment, vcdeploy can verify application health:

```yaml
# Project configuration
post_deploy:
  health_check:
    enabled: true
    timeout: 60s
    retries: 5
    retry_interval: 5s
```

### Rollback on Health Failure

```yaml
deployment:
  rollback_on_health_failure: true
  health_check:
    url: "http://localhost:8080/health"
    timeout: 30s
```

If the health check fails after deployment, vcdeploy automatically rolls back to the previous version.

## Monitoring Integration

### Prometheus Metrics

Health check results are exposed as metrics:

```prometheus
# Health check status (1=healthy, 0=unhealthy)
vcdeploy_health_check_status{project="myapp", check="http"} 1

# Health check latency
vcdeploy_health_check_duration_seconds{project="myapp"} 0.045
```

### Alertmanager Integration

Example alert rules:

```yaml
groups:
  - name: health-checks
    rules:
      - alert: ProjectHealthCheckFailing
        expr: vcdeploy_health_check_status == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Health check failing for {{ $labels.project }}"
```

## Troubleshooting

### Health Check Fails After Deployment

1. Check application logs
2. Verify the health endpoint is accessible
3. Check network connectivity
4. Review health check timeout settings

### Database Health Check Fails

1. Verify database connectivity
2. Check database server status
3. Review connection pool settings
4. Check for long-running queries

### Agent Disconnections

1. Check network connectivity
2. Verify gRPC port accessibility
3. Review agent logs
4. Check for firewall rules

## Next Steps

- [Metrics & Monitoring](metrics.md) - Prometheus metrics reference
- [Logging](logging.md) - Structured logging configuration
- [Troubleshooting](troubleshooting.md) - Common issues
