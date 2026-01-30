# Master Configuration

The master server is configured via a YAML file, typically located at `/etc/vcdeploy/master.yaml`.

## Configuration File

```yaml
# /etc/vcdeploy/master.yaml

# Server settings
server:
  listen: ":9000"              # HTTP API and UI
  tls:
    enabled: true
    cert: "/etc/vcdeploy/tls/cert.pem"
    key: "/etc/vcdeploy/tls/key.pem"

# gRPC settings (agent connections)
grpc:
  listen: ":9001"

# Database settings
database:
  path: "/var/lib/vcdeploy/vcdeploy.db"
  max_connections: 10
  busy_timeout: "5s"

# Authentication
auth:
  jwt_secret: "your-secure-secret"  # Generate with: openssl rand -base64 32
  session_timeout: "24h"
  totp_enabled: true               # Enable 2FA

# Logging
logging:
  level: "info"                    # debug, info, warn, error
  format: "json"                   # json or console
  output: "stdout"                 # stdout, stderr, or file path

# Tracing (OpenTelemetry)
tracing:
  enabled: false
  endpoint: "localhost:4317"
  service_name: "vcdeploy-master"
  sample_rate: 0.1                 # 10% sampling

# Alerting
alerting:
  enabled: true
  disk_warning_percent: 80
  disk_critical_percent: 90
  memory_warning_percent: 85
  cpu_warning_percent: 90
  deployment_timeout: "30m"
  alert_cooldown: "15m"

# Notifications
notifications:
  slack:
    enabled: true
    webhook_url: "https://hooks.slack.com/services/..."
  email:
    enabled: false
    smtp_host: "smtp.example.com"
    smtp_port: 587
    from: "vcdeploy@example.com"
```

## Environment Variables

All configuration can also be set via environment variables:

| Variable | Description |
|----------|-------------|
| `VCDEPLOY_SERVER_HTTP_ADDRESS` | HTTP listen address |
| `VCDEPLOY_SERVER_GRPC_ADDRESS` | gRPC listen address |
| `VCDEPLOY_DATABASE_PATH` | Database file path |
| `VCDEPLOY_AUTH_JWT_SECRET` | JWT signing secret |
| `VCDEPLOY_LOG_LEVEL` | Log verbosity level |

## TLS Configuration

### Self-Signed Certificates (Development)

```bash
# Generate self-signed certificate
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes
```

### Let's Encrypt (Production)

vcdeploy supports automatic ACME certificate management:

```yaml
server:
  https_enabled: true
  acme:
    enabled: true
    email: "admin@example.com"
    domains:
      - "vcdeploy.example.com"
    staging: false              # Set true for testing
```

## Default Values

| Setting | Default |
|---------|---------|
| HTTP address | `:9000` |
| gRPC address | `:9001` |
| Database path | `/var/lib/vcdeploy/vcdeploy.db` |
| Session timeout | `24h` |
| Log level | `info` |
| Tracing enabled | `false` |
| Alerting enabled | `true` |

## Secrets Encryption

The master encrypts all secrets at rest using AES-256-GCM:

```yaml
secrets:
  encryption_key: "32-byte-key-here"  # Must be exactly 32 bytes
```

Generate a key:
```bash
openssl rand -base64 32
```

## RBAC Configuration

```yaml
auth:
  rbac:
    enabled: true
    default_role: "viewer"    # Default role for new users
```

Available roles:
- `admin`: Full access
- `deployer`: Can trigger deployments
- `viewer`: Read-only access

## Rate Limiting

```yaml
rate_limit:
  enabled: true
  requests_per_second: 100
  burst: 200
```
