# Logging

vcdeploy provides structured logging for debugging, operations, and compliance.

## Log Categories

vcdeploy manages several types of logs:

| Category | Description | Storage |
|----------|-------------|---------|
| **Application** | Master/agent operational logs | stdout/journal |
| **Deployment** | Per-deployment execution logs | Database |
| **Audit** | User actions and system events | Database |

## Configuration

### Master Configuration

```yaml
# master.yaml
logs:
  # Application logs
  application:
    level: "info"              # debug, info, warn, error
    retention: "168h"          # 7 days

  # Deployment logs
  deployment:
    retention: "720h"          # 30 days
    max_size_mb: 100           # Per-deployment size limit

  # Audit logs
  audit:
    retention: "8760h"         # 1 year
    export:
      enabled: false
      destination: "/var/log/vcdeploy/audit"
      schedule: "0 0 * * *"    # Daily at midnight

  # Log rotation
  rotation:
    schedule: "0 2 * * *"      # Daily at 2 AM
```

### Agent Configuration

```yaml
# agent.yaml
# Agent logs go to stdout/stderr
# Use systemd journal or redirect to file
```

## Log Levels

| Level | Description | When to Use |
|-------|-------------|-------------|
| `debug` | Verbose debugging information | Development, troubleshooting |
| `info` | Normal operational messages | Production default |
| `warn` | Warning conditions | Production default |
| `error` | Error conditions | Always enabled |

## Application Logs

Application logs are structured JSON for easy parsing:

### JSON Format

```json
{
  "level": "info",
  "ts": "2024-01-15T10:30:00.000Z",
  "caller": "server/handler.go:123",
  "msg": "deployment started",
  "request_id": "abc-123",
  "project": "myapp",
  "deployment_id": "deploy-456"
}
```

### Log Fields

| Field | Description |
|-------|-------------|
| `level` | Log severity |
| `ts` | ISO 8601 timestamp |
| `caller` | Source file and line |
| `msg` | Log message |
| `request_id` | Request correlation ID |
| `error` | Error details (if applicable) |

### Request Correlation

All HTTP requests include a unique `X-Request-ID` header. This ID appears in all related log entries:

```json
{"level":"info","msg":"handling request","request_id":"550e8400-e29b-41d4-a716-446655440000","method":"POST","path":"/api/v1/deployments"}
{"level":"info","msg":"deployment started","request_id":"550e8400-e29b-41d4-a716-446655440000","project":"myapp"}
{"level":"info","msg":"deployment completed","request_id":"550e8400-e29b-41d4-a716-446655440000","status":"success"}
```

## Deployment Logs

Deployment execution logs are captured and stored in the database:

### Viewing Logs

```bash
# View deployment logs
vcdeploy deploy logs <deployment-id>

# Stream logs in real-time
vcdeploy deploy logs <deployment-id> --follow

# Filter by level
vcdeploy deploy logs <deployment-id> --level error
```

### Log Entry Structure

Each deployment log entry contains:
- **Timestamp**: When the log was created
- **Level**: Log severity (debug, info, warn, error)
- **Source**: Where the log came from (git, hook name, etc.)
- **Message**: Log content

### Log Retention

Deployment logs are automatically cleaned up based on retention settings:

```yaml
logs:
  deployment:
    retention: "720h"    # Keep logs for 30 days
    max_size_mb: 100     # Truncate logs exceeding 100MB
```

## Audit Logs

Audit logs track all user actions and system events for compliance:

### Recorded Events

| Event Type | Description |
|------------|-------------|
| `auth.login` | User login |
| `auth.logout` | User logout |
| `auth.failed` | Failed login attempt |
| `user.create` | User created |
| `user.update` | User modified |
| `user.delete` | User deleted |
| `project.create` | Project created |
| `project.update` | Project modified |
| `project.delete` | Project deleted |
| `deploy.trigger` | Deployment triggered |
| `deploy.cancel` | Deployment cancelled |
| `secret.set` | Secret created/updated |
| `secret.delete` | Secret deleted |
| `agent.register` | Agent registered |
| `agent.delete` | Agent removed |

### Viewing Audit Logs

```bash
# List recent audit events
vcdeploy audit list

# Filter by user
vcdeploy audit list --user admin

# Filter by action
vcdeploy audit list --action deploy.trigger

# Filter by time range
vcdeploy audit list --since "24h"
```

### Audit Export

Export audit logs for compliance:

```yaml
logs:
  audit:
    export:
      enabled: true
      destination: "/var/log/vcdeploy/audit"
      schedule: "0 0 * * *"   # Daily at midnight
```

Exported files are JSON-L format:
```
{"timestamp":"2024-01-15T10:30:00Z","user":"admin","action":"deploy.trigger","resource":"myapp","success":true}
{"timestamp":"2024-01-15T10:31:00Z","user":"admin","action":"secret.set","resource":"myapp/DATABASE_URL","success":true}
```

## Systemd Integration

When running as systemd services, logs go to the journal:

### Viewing Logs

```bash
# Master logs
journalctl -u vcdeploy -f

# Agent logs
journalctl -u vcdeploy-agent -f

# Since last boot
journalctl -u vcdeploy -b

# Last hour
journalctl -u vcdeploy --since "1 hour ago"

# With JSON output
journalctl -u vcdeploy -o json
```

### Filtering

```bash
# Only errors
journalctl -u vcdeploy -p err

# Specific time range
journalctl -u vcdeploy --since "2024-01-15" --until "2024-01-16"

# Grep pattern
journalctl -u vcdeploy | grep "deployment"
```

## Log Rotation

### Using logrotate

For file-based logging (if redirected):

```bash
# /etc/logrotate.d/vcdeploy
/var/log/vcdeploy/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0644 vcdeploy vcdeploy
    postrotate
        systemctl reload vcdeploy 2>/dev/null || true
    endscript
}
```

### Database Log Cleanup

vcdeploy automatically cleans up old logs:

```yaml
logs:
  rotation:
    schedule: "0 2 * * *"   # Run cleanup daily at 2 AM
```

## Log Aggregation

### Shipping to External Systems

**Loki (Grafana)**
```bash
# Use Promtail to collect from journald
# /etc/promtail/config.yml
scrape_configs:
  - job_name: vcdeploy
    journal:
      labels:
        job: vcdeploy
      matches:
        - _SYSTEMD_UNIT=vcdeploy.service
```

**Elasticsearch**
```bash
# Use Filebeat to ship logs
# /etc/filebeat/filebeat.yml
filebeat.inputs:
  - type: journald
    id: vcdeploy
    include_matches:
      - _SYSTEMD_UNIT=vcdeploy.service
```

**Datadog**
```bash
# Configure Datadog agent
# /etc/datadog-agent/conf.d/journald.d/conf.yaml
logs:
  - type: journald
    container_mode: true
    include_units:
      - vcdeploy.service
```

## Debugging

### Enable Debug Logging

Temporarily enable debug logging:

```bash
# Set environment variable
export VCDEPLOY_LOG_LEVEL=debug
vcdeploy master start

# Or in config
logs:
  application:
    level: "debug"
```

### Common Debug Scenarios

**Agent Connection Issues**
```bash
journalctl -u vcdeploy-agent -f | grep -E "connect|error|retry"
```

**Webhook Processing**
```bash
journalctl -u vcdeploy | grep webhook
```

**Deployment Execution**
```bash
vcdeploy deploy logs <deployment-id> --level debug
```

## Related Documentation

- [Master Configuration](config/master.md) - Full log configuration
- [Health Checks](operations/health.md) - Monitoring setup
- [Troubleshooting](operations/troubleshooting.md) - Common issues
