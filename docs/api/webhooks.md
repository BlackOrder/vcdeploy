# Webhooks

vcdeploy supports both incoming webhooks (from Git providers) and outgoing notifications.

## Incoming Webhooks (Git Providers)

Receive push events from Git providers to automatically trigger deployments.

### Supported Providers

| Provider | Endpoint | Events |
|----------|----------|--------|
| GitHub | `/webhook/github/{project}` | push, pull_request, create, delete, ping |
| GitLab | `/webhook/gitlab/{project}` | Push Hook, Merge Request Hook, Tag Push Hook |
| Bitbucket | `/webhook/bitbucket/{project}` | repo:push, pullrequest:* |

### GitHub Webhooks

#### Setup

1. Go to your GitHub repository → Settings → Webhooks → Add webhook
2. Configure:
   - **Payload URL**: `https://vcdeploy.example.com/webhook/github/myapp`
   - **Content type**: `application/json`
   - **Secret**: Your webhook secret
   - **Events**: Select "Just the push event" or customize

#### Supported Events

| Event | Description |
|-------|-------------|
| `push` | Triggers deployment on branch push |
| `pull_request` | PR opened/closed/merged events |
| `create` | Tag creation |
| `delete` | Tag/branch deletion |
| `ping` | Webhook connectivity test |

#### Signature Verification

GitHub signs payloads with HMAC-SHA256:

```
X-Hub-Signature-256: sha256=abc123...
```

vcdeploy validates signatures using the configured secret.

#### Payload Processing

Push event payload:
```json
{
  "ref": "refs/heads/main",
  "before": "abc123...",
  "after": "def456...",
  "forced": false,
  "deleted": false,
  "repository": {
    "full_name": "org/myapp",
    "clone_url": "https://github.com/org/myapp.git"
  },
  "head_commit": {
    "id": "def456...",
    "message": "Fix bug",
    "author": {
      "name": "Developer",
      "email": "dev@example.com"
    }
  }
}
```

### GitLab Webhooks

#### Setup

1. Go to your GitLab project → Settings → Webhooks
2. Configure:
   - **URL**: `https://vcdeploy.example.com/webhook/gitlab/myapp`
   - **Secret token**: Your webhook secret
   - **Triggers**: Push events, Merge request events, Tag push events

#### Supported Events

| Event | Description |
|-------|-------------|
| `Push Hook` | Triggers deployment on push |
| `Merge Request Hook` | MR opened/merged/closed |
| `Tag Push Hook` | Tag creation/deletion |

#### Signature Verification

GitLab sends the secret in a header:

```
X-Gitlab-Token: your-secret-token
```

### Bitbucket Webhooks

#### Setup

1. Go to your Bitbucket repository → Settings → Webhooks → Add webhook
2. Configure:
   - **Title**: vcdeploy
   - **URL**: `https://vcdeploy.example.com/webhook/bitbucket/myapp`
   - **Secret**: Your webhook secret (optional)
   - **Triggers**: Repository push, Pull request created/updated/merged

#### Supported Events

| Event | Description |
|-------|-------------|
| `repo:push` | Triggers deployment on push |
| `pullrequest:created` | PR created |
| `pullrequest:fulfilled` | PR merged |
| `pullrequest:rejected` | PR declined |
| `pullrequest:updated` | PR updated |

### Configuring Webhook Secrets

In the Web UI or via API:

```bash
# Set webhook secret for GitHub (use project ID)
curl -X POST https://vcdeploy.example.com/api/v1/projects/{id}/webhooks \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"provider": "github", "secret": "my-webhook-secret", "enabled": true}'
```

Or configure per-project:

```yaml
# projects/myapp.yaml
webhooks:
  github:
    secret: "${GITHUB_WEBHOOK_SECRET}"
    events:
      - push
    branches:
      - main
      - develop
```

### Webhook Security

#### Secret Requirement

You can require webhook secrets:

```yaml
webhooks:
  github:
    secret: "my-secret"
    require_secret: true  # Reject unsigned requests
```

#### Recommended Settings

1. **Always use secrets** - Prevents unauthorized deployments
2. **Use HTTPS** - Encrypts webhook payload in transit
3. **Restrict events** - Only enable events you need
4. **Limit branches** - Only deploy specific branches

---

## Outgoing Notifications

Send deployment events to external services.

### Supported Channels

| Channel | Configuration |
|---------|--------------|
| **Slack** | Webhook URL |
| **Discord** | Webhook URL |
| **Email** | SMTP settings |
| **Custom Webhook** | HTTP endpoint |

### Slack Notifications

#### Configuration

```yaml
# master.yaml
notifications:
  slack:
    webhook_url: "https://hooks.slack.com/services/T00/B00/xxx"
    channel: "#deployments"
    username: "VCDeploy"
    icon_emoji: ":rocket:"
```

#### Message Format

Slack notifications include:
- Project name and environment
- Deployment status (success/failed)
- Version/commit
- Triggered by
- Timestamp
- Link to deployment details

### Discord Notifications

#### Configuration

```yaml
# master.yaml
notifications:
  discord:
    webhook_url: "https://discord.com/api/webhooks/xxx/yyy"
    username: "VCDeploy"
    avatar_url: "https://example.com/vcdeploy-icon.png"
```

#### Message Format

Discord notifications use rich embeds with:
- Color-coded status (green/yellow/red/orange)
- Emoji indicators
- Status, version, triggered by fields
- Timestamp
- Optional deployment URL

### Email Notifications

#### Configuration

```yaml
# master.yaml
notifications:
  email:
    smtp_host: "smtp.example.com"
    smtp_port: 587
    username: "notifications@example.com"
    password: "${SMTP_PASSWORD}"
    from_address: "notifications@example.com"
    from_name: "VCDeploy"
    to_addresses:
      - "team@example.com"
      - "oncall@example.com"
    template_dir: "/etc/vcdeploy/templates/email"  # Optional
```

#### Message Format

HTML-formatted emails with:
- Deployment summary
- Status badge
- Project details
- Version information
- Triggered by
- Timestamp
- Link to deployment details

### Custom Webhooks

Send JSON payloads to any HTTP endpoint.

#### Configuration

```yaml
# master.yaml
notifications:
  webhooks:
    - name: "ci-callback"
      url: "https://ci.example.com/webhooks/vcdeploy"
      method: "POST"
      secret: "${WEBHOOK_SECRET}"
      headers:
        Authorization: "Bearer ${CI_TOKEN}"
```

#### Payload Format

```json
{
  "type": "deployment",
  "project_id": "myapp",
  "project_name": "My Application",
  "environment": "production",
  "deploy_id": "deploy-123",
  "version": "v1.2.3",
  "status": "success",
  "user": "developer@example.com",
  "message": "Deployed successfully",
  "url": "https://vcdeploy.example.com/deployments/deploy-123",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

#### HMAC Signature

Custom webhooks are signed with HMAC-SHA256:

```
X-VCDeploy-Signature: sha256=abc123...
```

Verify signatures:

```python
import hmac
import hashlib

def verify(payload: bytes, signature: str, secret: str) -> bool:
    expected = hmac.new(
        secret.encode(),
        payload,
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(f"sha256={expected}", signature)
```

### Per-Project Notifications

Override global notifications for specific projects:

```yaml
# projects/myapp.yaml
notifications:
  on_success:
    - slack: "#deployments"
    - email: "team@example.com"
  on_failure:
    - slack: "#alerts"
    - email: "oncall@example.com"
    - webhook: "https://pagerduty.com/hook/xxx"
```

### Notification Events

| Event | Description |
|-------|-------------|
| `deployment.started` | Deployment begins |
| `deployment.success` | Deployment succeeds |
| `deployment.failed` | Deployment fails |
| `deployment.rolled_back` | Rollback executed |

---

## API Endpoints

### Webhook Configuration

```bash
# List project webhooks
GET /api/v1/projects/{id}/webhooks

# Configure webhook for provider
POST /api/v1/projects/{id}/webhooks
{
  "provider": "github",
  "secret": "webhook-secret",
  "enabled": true,
  "require_secret": true
}

# Update webhook
PUT /api/v1/projects/{id}/webhooks/{provider}

# Delete webhook
DELETE /api/v1/projects/{id}/webhooks/{provider}
```

### Manual Trigger

Trigger deployment via API (not via Git webhook):

```bash
POST /api/v1/deployments
{
  "project": "myapp",
  "target": "production",
  "ref": "main"
}
```

---

## Troubleshooting

### Webhook Not Triggering Deployment

1. **Check webhook configuration**:
   ```bash
   vcdeploy project show myapp --webhooks
   ```

2. **Verify secret matches**:
   - GitHub: Must match X-Hub-Signature-256
   - GitLab: Must match X-Gitlab-Token
   - Bitbucket: Optional but recommended

3. **Check branch filters**:
   ```yaml
   watch:
     branches:
       - main  # Only deploys from main
   ```

4. **View webhook logs**:
   ```bash
   vcdeploy logs --filter webhook
   ```

### Signature Verification Failing

- Ensure secret is identical on both sides
- Check for trailing whitespace in secret
- Verify UTF-8 encoding

### Notification Not Sending

1. **Check notification configuration**:
   ```bash
   vcdeploy config show notifications
   ```

2. **Verify connectivity**:
   - Slack: Test webhook URL
   - Email: Check SMTP settings
   - Discord: Verify webhook URL

3. **View notification logs**:
   ```bash
   vcdeploy logs --filter notify
   ```

---

## Related Documentation

- [Project Configuration](config/projects.md) - Webhook settings per project
- [Master Configuration](config/master.md) - Global notification settings
- [API Reference](api/README.md) - REST API endpoints
