# Metrics & Monitoring

vcdeploy exposes Prometheus metrics for comprehensive observability.

## Metrics Endpoint

Metrics are available at `/metrics` on the master server:

```bash
curl http://localhost:9000/metrics
```

## Available Metrics

### Deployment Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `vcdeploy_deployments_total` | Counter | `project`, `status` | Total deployments by project and status |
| `vcdeploy_deployment_duration_seconds` | Histogram | `project` | Deployment duration in seconds |

### HTTP Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `vcdeploy_http_requests_total` | Counter | `method`, `path`, `status` | Total HTTP requests |
| `vcdeploy_http_request_duration_seconds` | Histogram | `method`, `path` | HTTP request latency |
| `vcdeploy_http_requests_in_flight` | Gauge | - | Current in-flight requests |

### Agent Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `vcdeploy_agent_connected` | Gauge | `agent_id` | Agent connection status (1=connected, 0=disconnected) |
| `vcdeploy_agent_cpu_percent` | Gauge | `agent_id` | Agent CPU usage percentage |
| `vcdeploy_agent_memory_percent` | Gauge | `agent_id` | Agent memory usage percentage |
| `vcdeploy_agent_disk_percent` | Gauge | `agent_id` | Agent disk usage percentage |

### gRPC Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `vcdeploy_grpc_stream_duration_seconds` | Histogram | `agent_id` | gRPC stream session duration |

### Database Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `vcdeploy_db_query_duration_seconds` | Histogram | `operation` | Database query latency |

### Webhook Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `vcdeploy_webhooks_received_total` | Counter | `source`, `event` | Webhooks received by source and event type |

### Build Info

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `vcdeploy_build_info` | Gauge | `version`, `commit`, `date` | Build information |

## Prometheus Configuration

Add vcdeploy to your Prometheus scrape configuration:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'vcdeploy'
    static_configs:
      - targets: ['vcdeploy-master:9000']
    metrics_path: /metrics
    scrape_interval: 15s
```

## Grafana Dashboard

### Import Dashboard

1. Log into Grafana
2. Go to Dashboards → Import
3. Upload the JSON or paste the dashboard ID

### Example Dashboard JSON

```json
{
  "title": "vcdeploy Overview",
  "panels": [
    {
      "title": "Deployments per Hour",
      "type": "graph",
      "targets": [
        {
          "expr": "sum(rate(vcdeploy_deployments_total[1h])) by (project)",
          "legendFormat": "{{project}}"
        }
      ]
    },
    {
      "title": "Deployment Success Rate",
      "type": "stat",
      "targets": [
        {
          "expr": "sum(rate(vcdeploy_deployments_total{status=\"success\"}[24h])) / sum(rate(vcdeploy_deployments_total[24h])) * 100"
        }
      ]
    },
    {
      "title": "Agent Status",
      "type": "table",
      "targets": [
        {
          "expr": "vcdeploy_agent_connected",
          "legendFormat": "{{agent_id}}"
        }
      ]
    },
    {
      "title": "HTTP Request Latency (p99)",
      "type": "graph",
      "targets": [
        {
          "expr": "histogram_quantile(0.99, rate(vcdeploy_http_request_duration_seconds_bucket[5m]))"
        }
      ]
    }
  ]
}
```

## Alerting Rules

Example Prometheus alerting rules:

```yaml
# alerts.yml
groups:
  - name: vcdeploy
    rules:
      - alert: DeploymentFailureRateHigh
        expr: |
          sum(rate(vcdeploy_deployments_total{status="failed"}[1h])) 
          / sum(rate(vcdeploy_deployments_total[1h])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: High deployment failure rate
          description: "Deployment failure rate is {{ $value | humanizePercentage }}"

      - alert: AgentDisconnected
        expr: vcdeploy_agent_connected == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: Agent disconnected
          description: "Agent {{ $labels.agent_id }} has been disconnected for 5 minutes"

      - alert: HighAPILatency
        expr: |
          histogram_quantile(0.99, rate(vcdeploy_http_request_duration_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: High API latency
          description: "99th percentile API latency is {{ $value | humanizeDuration }}"

      - alert: AgentHighCPU
        expr: vcdeploy_agent_cpu_percent > 90
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: Agent high CPU usage
          description: "Agent {{ $labels.agent_id }} CPU usage is {{ $value }}%"

      - alert: AgentHighDisk
        expr: vcdeploy_agent_disk_percent > 90
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: Agent disk space critical
          description: "Agent {{ $labels.agent_id }} disk usage is {{ $value }}%"
```

## Health Endpoints

vcdeploy provides Kubernetes-compatible health endpoints:

| Endpoint | Purpose | Auth |
|----------|---------|------|
| `/healthz` | Liveness probe | No |
| `/livez` | Liveness probe (alias) | No |
| `/readyz` | Readiness probe | No |
| `/api/v1/health` | Detailed health status | Yes |

### Kubernetes Integration

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: vcdeploy
          livenessProbe:
            httpGet:
              path: /livez
              port: 9000
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /readyz
              port: 9000
            initialDelaySeconds: 5
            periodSeconds: 5
```

### Health Response Format

`/api/v1/health` returns detailed health information:

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
      "message": "3/3 agents connected"
    }
  }
}
```

## Next Steps

- [Logging](logging.md) - Configure structured logging
- [Troubleshooting](troubleshooting.md) - Common issues and solutions
