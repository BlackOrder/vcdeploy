# Webhooks

vcdeploy supports webhooks for event notifications and CI/CD integrations.

## Overview

Webhooks allow external systems to receive notifications when events occur in vcdeploy.

## Supported Events

| Event | Trigger |
|-------|---------|
| `deployment.started` | Deployment begins |
| `deployment.completed` | Deployment succeeds |
| `deployment.failed` | Deployment fails |
| `deployment.rolled_back` | Rollback executed |
| `agent.connected` | Agent comes online |
| `agent.disconnected` | Agent goes offline |
| `project.created` | New project created |
| `project.updated` | Project modified |
| `alert.triggered` | Alert condition met |

## Configuration

### Master Configuration

```yaml
# master.yaml
webhooks:
  enabled: true
  
  # Global settings
  timeout: 30s
  retry_count: 3
  retry_delay: 5s
  
  # Webhook endpoints
  endpoints:
    - name: slack-notifications
      url: https://hooks.slack.com/services/xxx/yyy/zzz
      events:
        - deployment.completed
        - deployment.failed
      secret: ${SLACK_WEBHOOK_SECRET}
      
    - name: ci-callback
      url: https://ci.example.com/webhooks/vcdeploy
      events: ["*"]  # All events
      headers:
        Authorization: "Bearer ${CI_TOKEN}"
```

### Per-Project Webhooks

```yaml
# projects/myapp.yaml
name: myapp
webhooks:
  - url: https://app.pagerduty.com/webhooks/xxx
    events:
      - deployment.failed
```

## Payload Format

### Standard Payload

```json
{
  "event": "deployment.completed",
  "timestamp": "2024-01-15T10:30:00Z",
  "id": "evt_abc123",
  "data": {
    "deployment_id": "deploy_xyz789",
    "project": "myapp",
    "environment": "production",
    "version": "v1.2.3",
    "duration_seconds": 45,
    "status": "success"
  },
  "source": {
    "master_id": "master-01",
    "version": "0.10.0"
  }
}
```

### Event-Specific Data

**deployment.started:**
```json
{
  "event": "deployment.started",
  "data": {
    "deployment_id": "deploy_xyz789",
    "project": "myapp",
    "environment": "production",
    "version": "v1.2.3",
    "triggered_by": "user@example.com",
    "agents": ["web-01", "web-02"]
  }
}
```

**deployment.failed:**
```json
{
  "event": "deployment.failed",
  "data": {
    "deployment_id": "deploy_xyz789",
    "project": "myapp",
    "environment": "production",
    "version": "v1.2.3",
    "error": "Command failed with exit code 1",
    "failed_agent": "web-02",
    "failed_step": "post_deploy"
  }
}
```

**agent.disconnected:**
```json
{
  "event": "agent.disconnected",
  "data": {
    "agent_id": "agent_abc",
    "hostname": "web-01.example.com",
    "last_seen": "2024-01-15T10:29:00Z",
    "reason": "heartbeat_timeout"
  }
}
```

## Security

### Signature Verification

vcdeploy signs webhooks using HMAC-SHA256:

```
X-VCDeploy-Signature: sha256=abc123...
X-VCDeploy-Timestamp: 1705315800
```

### Verification Example (Go)

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
)

func verifySignature(payload []byte, signature, timestamp, secret string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(timestamp))
    mac.Write(payload)
    expected := hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(signature), []byte("sha256="+expected))
}
```

### Verification Example (Python)

```python
import hmac
import hashlib

def verify_signature(payload: bytes, signature: str, timestamp: str, secret: str) -> bool:
    expected = hmac.new(
        secret.encode(),
        timestamp.encode() + payload,
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(f"sha256={expected}", signature)
```

## Incoming Webhooks

vcdeploy can also receive webhooks to trigger deployments:

### Trigger Endpoint

```
POST /api/v1/webhooks/trigger
```

### GitHub Webhook

```yaml
# projects/myapp.yaml
triggers:
  - type: github
    events:
      - push
    branches:
      - main
    secret: ${GITHUB_WEBHOOK_SECRET}
```

### GitLab Webhook

```yaml
triggers:
  - type: gitlab
    events:
      - push
      - tag_push
    branches:
      - main
    secret: ${GITLAB_WEBHOOK_SECRET}
```

### Generic Webhook

```bash
curl -X POST https://vcdeploy.example.com/api/v1/webhooks/trigger \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Secret: mysecret" \
  -d '{
    "project": "myapp",
    "environment": "staging",
    "ref": "main"
  }'
```

## Retry Policy

Failed webhook deliveries are retried with exponential backoff:

| Attempt | Delay |
|---------|-------|
| 1 | Immediate |
| 2 | 5 seconds |
| 3 | 30 seconds |
| 4 | 2 minutes |
| 5 | 10 minutes |

### Viewing Webhook Status

```bash
# List recent webhook deliveries
vcdeploy webhook list

# View delivery details
vcdeploy webhook show evt_abc123

# Retry failed delivery
vcdeploy webhook retry evt_abc123
```

## Testing Webhooks

### Test Endpoint

```bash
# Send test event to all configured webhooks
vcdeploy webhook test --event deployment.completed

# Test specific webhook
vcdeploy webhook test --endpoint slack-notifications
```

### Local Testing with ngrok

```bash
# Expose local endpoint
ngrok http 3000

# Configure webhook URL
webhooks:
  endpoints:
    - url: https://abc123.ngrok.io/webhook
```

## Integrations

### Slack

```yaml
endpoints:
  - name: slack
    url: https://hooks.slack.com/services/T00/B00/xxx
    events:
      - deployment.completed
      - deployment.failed
    # Optional: Transform to Slack format
    format: slack
```

### PagerDuty

```yaml
endpoints:
  - name: pagerduty
    url: https://events.pagerduty.com/v2/enqueue
    events:
      - deployment.failed
      - alert.triggered
    headers:
      Content-Type: application/json
    format: pagerduty
```

### Microsoft Teams

```yaml
endpoints:
  - name: teams
    url: https://outlook.office.com/webhook/xxx
    events:
      - deployment.completed
      - deployment.failed
    format: teams
```

## Troubleshooting

### Webhook Not Firing

1. Check webhook is enabled:
   ```bash
   vcdeploy config show webhooks
   ```

2. Verify event subscription:
   ```bash
   vcdeploy webhook list --verbose
   ```

3. Check delivery logs:
   ```bash
   vcdeploy logs --filter webhook
   ```

### Signature Mismatch

- Verify secret matches on both sides
- Check for encoding issues (UTF-8)
- Ensure timestamp is being included in signature

### Timeout Issues

Increase timeout for slow endpoints:
```yaml
webhooks:
  endpoints:
    - name: slow-service
      url: https://slow.example.com/webhook
      timeout: 60s
```
