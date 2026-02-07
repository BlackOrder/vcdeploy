# Master Configuration

The master server is configured via a YAML file, typically located at `/etc/vcdeploy/master.yaml`.

## Complete Configuration Reference

```yaml
# /etc/vcdeploy/master.yaml

# HTTP Server settings
server:
  listen: ":9000"              # HTTP API and Web UI address
  https_address: ":9443"       # Optional separate HTTPS port
  socket_path: "/var/run/vcdeploy/vcdeploy.sock"  # Unix socket for local CLI
  tls:
    mode: "static"             # disabled, static, or acme
    cert_file: "/etc/vcdeploy/tls/cert.pem"
    key_file: "/etc/vcdeploy/tls/key.pem"
    force_https: true          # Redirect HTTP to HTTPS
    min_version: "1.2"         # Minimum TLS version
    
    # ACME (Let's Encrypt) configuration
    acme:
      email: "admin@example.com"
      domains:
        - "vcdeploy.example.com"
      staging: false
      cache_dir: "/var/lib/vcdeploy/acme"

# gRPC Server settings (agent connections)
grpc:
  listen: ":9001"              # gRPC address for agents

# Archive settings (deployment archive management)
archive:
  storage_dir: /var/lib/vcdeploy/archives    # Archive storage directory
  keep_count: 10                              # Max cached archives per project
  max_total_size: 5GB                         # Max total archive cache size
  signed_url_expiry: 10m                      # HMAC signed URL expiry

# SSH settings (for provisioning and git repo access — not used for deployment)
ssh:
  default_user: "deploy"                      # Default SSH username
  default_key: "/etc/vcdeploy/keys/default.pem"  # Default SSH private key
  known_hosts: "/etc/vcdeploy/known_hosts"    # Known hosts file
  connection_timeout: 30s
  keepalive_interval: 15s
  idle_timeout: 60s
  jump_servers:                               # Bastion/jump servers
    - name: bastion
      host: bastion.example.com:22
      user: deploy
      key: /etc/vcdeploy/keys/bastion.pem

# Security settings
security:
  session_timeout: 24h                        # Web session timeout
  require_2fa_admin: true                     # Require TOTP for admin users
  reauth_address: ":9002"                     # Dedicated port for re-authentication
  key_rotation:
    enabled: true                             # Enable automatic key rotation
    interval: 720h                            # Rotation interval (30 days)

# Backup settings
backup:
  database:
    enabled: true
    interval: 720h                            # Backup interval (30 days)
    retention: 8760h                          # Keep backups for 365 days
    path: /var/lib/vcdeploy/backups/
  config:
    versions: 5                               # Keep 5 config backup versions

# Logging settings
logs:
  application:
    level: info                               # debug, info, warn, error
    retention: 720h                           # Keep logs for 30 days
  deployment:
    retention: 2160h                          # Keep deployment logs for 90 days
    max_size_mb: 100                          # Max size per deployment log
  audit:
    retention: 8760h                          # Keep audit logs for 365 days
    export:
      enabled: false
      destination: /var/log/vcdeploy/audit/
      schedule: "0 0 1 * *"                   # Monthly export (cron format)
  rotation:
    schedule: "0 3 * * *"                     # Daily at 3 AM (cron format)

# OpenTelemetry tracing
tracing:
  enabled: false
  endpoint: "localhost:4317"                  # OTLP endpoint
  service_name: "vcdeploy"
  sample_rate: 0.1                            # 10% sampling
  insecure: false                             # Use TLS for OTLP

# System alerting
alerting:
  enabled: true
  disk_warning_percent: 80
  disk_critical_percent: 90
  memory_warning_percent: 85
  cpu_warning_percent: 90
  deployment_timeout: 30m                     # Alert if deployment exceeds this
  alert_cooldown: 15m                         # Minimum time between alerts

# Incoming webhook settings (from Git providers)
webhooks:
  github:
    enabled: true
    path: /webhook/github
  gitlab:
    enabled: true
    path: /webhook/gitlab
  bitbucket:
    enabled: true
    path: /webhook/bitbucket

# Outgoing notification settings
notifications:
  providers:
    slack:
      enabled: false
      webhook_url: "https://hooks.slack.com/services/..."
      channel: "#deployments"
      username: "VCDeploy"
      icon_emoji: ":rocket:"
    email:
      enabled: false
      smtp:
        host: smtp.example.com
        port: 587
        user: ""
        password: ""
        tls: true
        from_address: "vcdeploy@example.com"
        from_name: "VCDeploy"
        to_addresses:
          - ops@example.com
    discord:
      enabled: false
      webhook_url: "https://discord.com/api/webhooks/..."
      username: "VCDeploy"
      avatar_url: ""
    webhook:
      enabled: false
      url: "https://example.com/webhook"
      method: POST
      headers:
        Authorization: "Bearer token"
      secret: ""

# API settings
api:
  enabled: true
  rate_limit:
    enabled: false                            # Enable rate limiting
    requests_per_second: 100                  # Requests per second limit
    burst_size: 50                            # Burst allowance

# Storage settings
storage:
  use_memory_cache: true                      # Enable in-memory cache layer

# UI appearance
appearance:
  theme: dark                                 # dark or light
```

## Configuration Sections

### Server

HTTP server configuration for the REST API and Web UI.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `listen` | string | `:9000` | Address to listen on |
| `https_address` | string | - | Optional separate HTTPS port |
| `socket_path` | string | `/var/run/vcdeploy/vcdeploy.sock` | Unix socket for local CLI access |
| `tls.mode` | string | `disabled` | TLS mode: disabled, static, or acme |
| `tls.cert_file` | string | - | TLS certificate path (static mode) |
| `tls.key_file` | string | - | TLS private key path (static mode) |
| `tls.force_https` | bool | `false` | Redirect HTTP to HTTPS |
| `tls.min_version` | string | `1.2` | Minimum TLS version (1.2 or 1.3) |
| `tls.acme.email` | string | - | ACME contact email (acme mode) |
| `tls.acme.domains` | list | `[]` | Domains for ACME certs |
| `tls.acme.staging` | bool | `false` | Use Let's Encrypt staging |
| `tls.acme.cache_dir` | string | - | Certificate cache directory |

### gRPC

gRPC server for agent connections.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `listen` | string | `:9001` | gRPC listen address |

### SSH

SSH connection settings for provisioning and git repository access. Not used for deployment.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default_user` | string | `deploy` | Default SSH username |
| `default_key` | string | - | Default SSH private key path |
| `known_hosts` | string | - | SSH known hosts file |
| `connection_timeout` | duration | `30s` | Connection timeout |
| `keepalive_interval` | duration | `15s` | SSH keepalive interval |
| `idle_timeout` | duration | `60s` | Idle connection timeout |
| `jump_servers` | list | `[]` | Bastion/jump server configs |

### Security

Security and authentication settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `session_timeout` | duration | `24h` | Web session timeout |
| `require_2fa_admin` | bool | `true` | Require TOTP for admin users |
| `reauth_address` | string | - | Dedicated port for re-authentication (optional) |
| `key_rotation.enabled` | bool | `true` | Enable automatic key rotation |
| `key_rotation.interval` | duration | `720h` | Key rotation interval |

### Backup

Automatic backup configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `database.enabled` | bool | `true` | Enable database backups |
| `database.interval` | duration | `720h` | Backup frequency |
| `database.retention` | duration | `8760h` | Backup retention period |
| `database.path` | string | `/var/lib/vcdeploy/backups/` | Backup storage path |
| `config.versions` | int | `5` | Config backup versions to keep |

### Logs

Logging configuration for different log types.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `application.level` | string | `info` | Log level (debug, info, warn, error) |
| `application.retention` | duration | `720h` | Application log retention |
| `deployment.retention` | duration | `2160h` | Deployment log retention |
| `deployment.max_size_mb` | int | `100` | Max size per deployment log |
| `audit.retention` | duration | `8760h` | Audit log retention |
| `audit.export.enabled` | bool | `false` | Enable audit log export |
| `rotation.schedule` | string | `0 3 * * *` | Log rotation schedule (cron) |

### Tracing

OpenTelemetry distributed tracing configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable tracing |
| `endpoint` | string | `localhost:4317` | OTLP collector endpoint |
| `service_name` | string | `vcdeploy` | Service name in traces |
| `sample_rate` | float | `0.1` | Trace sampling rate (0.0-1.0) |
| `insecure` | bool | `false` | Use insecure OTLP connection |

### Alerting

System monitoring and alerting thresholds.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable alerting |
| `disk_warning_percent` | float | `80` | Disk usage warning threshold |
| `disk_critical_percent` | float | `90` | Disk usage critical threshold |
| `memory_warning_percent` | float | `85` | Memory usage warning threshold |
| `cpu_warning_percent` | float | `90` | CPU usage warning threshold |
| `deployment_timeout` | duration | `30m` | Alert if deployment exceeds this |
| `alert_cooldown` | duration | `15m` | Minimum time between alerts |

### Webhooks

Incoming webhook configuration for Git providers.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `github.enabled` | bool | `true` | Enable GitHub webhooks |
| `github.path` | string | `/webhook/github` | GitHub webhook path |
| `gitlab.enabled` | bool | `true` | Enable GitLab webhooks |
| `gitlab.path` | string | `/webhook/gitlab` | GitLab webhook path |
| `bitbucket.enabled` | bool | `true` | Enable Bitbucket webhooks |
| `bitbucket.path` | string | `/webhook/bitbucket` | Bitbucket webhook path |

### Notifications

Outgoing notification configuration.

See [Notifications Guide](../operations/logging.md#notifications) for detailed setup.

### API

API server configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable REST API |
| `rate_limit.enabled` | bool | `false` | Enable rate limiting |
| `rate_limit.requests_per_second` | float | `100` | Max requests per second |
| `rate_limit.burst_size` | int | `50` | Burst allowance above limit |

### Storage

Storage and caching configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `use_memory_cache` | bool | `true` | Enable in-memory cache layer with batched SQLite persistence. Eliminates SQLITE_BUSY errors from concurrent access. |

## TLS Configuration

### Generate Self-Signed Certificates (Development)

```bash
# Generate CA
openssl genrsa -out ca.key 4096
openssl req -x509 -new -nodes -key ca.key -sha256 -days 365 -out ca.crt \
  -subj "/CN=VCDeploy CA"

# Generate server certificate
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr \
  -subj "/CN=vcdeploy.local"
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt -days 365 -sha256
```

### Using Let's Encrypt (Production)

Use certbot or similar to obtain certificates:

```bash
sudo certbot certonly --standalone -d vcdeploy.example.com

# Update config
server:
  tls:
    enabled: true
    cert: /etc/letsencrypt/live/vcdeploy.example.com/fullchain.pem
    key: /etc/letsencrypt/live/vcdeploy.example.com/privkey.pem
```

## Database

The database path is determined by the system configuration, typically:

- Linux: `/var/lib/vcdeploy/vcdeploy.db`
- macOS: `~/Library/Application Support/vcdeploy/vcdeploy.db`

The database location is not configurable in the YAML file—it's determined by the operating system's standard data directories.

## Default Values Summary

| Setting | Default |
|---------|---------|
| HTTP address | `:9000` |
| gRPC address | `:9001` |
| Session timeout | `24h` |
| Application log level | `info` |
| Deployment log retention | `90 days` |
| Audit log retention | `365 days` |
| Key rotation interval | `30 days` |
| Backup interval | `30 days` |
| Backup retention | `365 days` |

## Environment Variables

The following environment variables can be used to configure vcdeploy. Environment variables take precedence over config file values for the fields they override.

### Server Configuration

| Variable | Config Path | Default | Description |
|----------|-------------|---------|-------------|
| `VCDEPLOY_SERVER_LISTEN` | `server.listen` | `:9000` | HTTP listen address |
| `VCDEPLOY_GRPC_LISTEN` | `grpc.listen` | `:9001` | gRPC listen address |
| `VCDEPLOY_LOG_LEVEL` | `logs.application.level` | `info` | Log level |

### Directory Configuration

| Variable | Config Path | Default | Description |
|----------|-------------|---------|-------------|
| `VCDEPLOY_CONFIG_DIR` | - | `/etc/vcdeploy` | Configuration directory |
| `VCDEPLOY_DATA_DIR` | - | `/var/lib/vcdeploy` | Data directory |
| `VCDEPLOY_LOG_DIR` | - | `/var/log/vcdeploy` | Log directory |
| `VCDEPLOY_RUN_DIR` | - | `/var/run/vcdeploy` | Runtime directory |
| `VCDEPLOY_SYSTEM_CONFIG` | - | - | Override system config path |

### Security

| Variable | Config Path | Default | Description |
|----------|-------------|---------|-------------|
| `VCDEPLOY_MASTER_KEY` | - | Auto-generated | Master encryption key (base64) |
| `VCDEPLOY_ADMIN_USERNAME` | - | `admin` | Initial admin username |
| `VCDEPLOY_ADMIN_PASSWORD` | - | - | Initial admin password |
| `VCDEPLOY_ADMIN_EMAIL` | - | `admin@localhost` | Initial admin email |

### CLI Configuration

| Variable | Config Path | Default | Description |
|----------|-------------|---------|-------------|
| `VCDEPLOY_MASTER` | - | - | Master server address for remote CLI |
| `VCDEPLOY_TOKEN` | - | - | API token for remote CLI authentication |

### Testing (Development Only)

| Variable | Config Path | Default | Description |
|----------|-------------|---------|-------------|
| `VCDEPLOY_TEST_MODE` | - | `false` | Enable test mode (disables some security checks) |

> **Warning:** `VCDEPLOY_TEST_MODE` bypasses security guards. Never use in production.

## See Also

- [Quick Start](../quickstart.md)
- [Agent Configuration](agent.md)
- [Projects Configuration](projects.md)
- [Secrets Management](secrets.md)
