# Logging

vcdeploy provides structured logging for debugging and observability.

## Log Levels

| Level | Description |
|-------|-------------|
| `debug` | Verbose debugging information |
| `info` | Normal operational messages |
| `warn` | Warning conditions |
| `error` | Error conditions |

## Configuration

### Master Server

```yaml
# master.yaml
logging:
  level: "info"
  format: "json"      # json or console
  output: "stdout"    # stdout, stderr, or file path
```

### Agent

```yaml
# agent.yaml
logging:
  level: "info"
  format: "json"
  output: "stdout"
```

## Log Formats

### JSON Format (Recommended for Production)

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

### Console Format (Development)

```
2024-01-15T10:30:00.000Z INFO server/handler.go:123 deployment started {"request_id": "abc-123", "project": "myapp"}
```

## Request Correlation

All requests include a unique `X-Request-ID` header:

```json
{
  "level": "info",
  "msg": "handling request",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "method": "POST",
  "path": "/api/v1/deployments"
}
```

## Deployment Logs

Deployment execution logs are captured and stored:

```bash
# View deployment logs
vcdeploy deploy logs <deployment-id>

# Stream logs in real-time
vcdeploy deploy logs <deployment-id> --follow
```

### Log Storage

Deployment logs are stored in the database with:
- Timestamp
- Log level
- Source (master/agent)
- Message

## Log Rotation

For file-based logging, use logrotate:

```
# /etc/logrotate.d/vcdeploy
/var/log/vcdeploy/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0644 vcdeploy vcdeploy
}
```

## Systemd Journal

When running as a systemd service, logs go to the journal:

```bash
# View master logs
journalctl -u vcdeploy -f

# View agent logs
journalctl -u vcdeploy-agent -f

# View logs since boot
journalctl -u vcdeploy -b

# View last hour
journalctl -u vcdeploy --since "1 hour ago"
```

## Log Aggregation

### Shipping to External Systems

For production, ship logs to a centralized system:

**Loki (Grafana)**
```yaml
logging:
  output: "stdout"
```
Use Promtail to collect from stdout.

**Elasticsearch**
```yaml
logging:
  format: "json"
  output: "/var/log/vcdeploy/master.log"
```
Use Filebeat to ship to Elasticsearch.

## Debugging

### Enable Debug Logging

```bash
# Via environment variable
VCDEPLOY_LOG_LEVEL=debug vcdeploy master start

# Via config
logging:
  level: "debug"
```

### Common Debug Scenarios

**Agent Connection Issues:**
```bash
# On agent
VCDEPLOY_LOG_LEVEL=debug vcdeploy-agent 2>&1 | grep -i "connect\|tls\|auth"
```

**Deployment Failures:**
```bash
# Check deployment logs
vcdeploy deploy logs <id>

# Check master logs during deployment
journalctl -u vcdeploy --since "5 minutes ago" | grep <deployment-id>
```

## Audit Logging

Security-relevant events are logged separately:

```bash
# View audit log
vcdeploy audit list

# Export for compliance
vcdeploy audit export --start 2024-01-01 --end 2024-01-31 > audit.json
```

Events logged:
- User authentication
- Secret access
- Configuration changes
- Deployment triggers
- Agent connections
