# Agent Configuration

The vcdeploy agent is configured via a YAML file, typically located at `/etc/vcdeploy/agent.yaml`.

## Complete Configuration Reference

```yaml
# /etc/vcdeploy/agent.yaml

# Master connection settings
master:
  address: "master.example.com:9001"   # Master gRPC address
  token: ""                            # Authentication token (set after registration)
  ca_cert: /etc/vcdeploy/agent/ca.pem  # CA certificate for TLS verification
  reconnect:
    initial_delay: 1s                  # Initial reconnect delay
    max_delay: 5m                      # Maximum reconnect delay
    heartbeat_interval: 10s            # Heartbeat frequency

# Agent identity
agent:
  id: ""                               # Unique agent ID (required)
  labels:                              # Labels for agent selection
    environment: production
    role: web
    datacenter: us-east-1
  update_policy: immediate             # immediate | scheduled | manual
  update_window_start: ""              # HH:MM format (for scheduled updates)
  update_window_end: ""                # HH:MM format (for scheduled updates)

# Local paths
paths:
  data: /var/lib/vcdeploy/agent/       # Agent database directory
  releases: /var/www/                  # Release deployment root

# Archive cache settings
archive:
  cache_dir: /var/lib/vcdeploy/cache/archives  # Archive cache directory
  keep_count: 5                                 # Max cached archives per project

# Command execution settings
execution:
  user: www-data                       # User to run commands as
  group: www-data                      # Group to run commands as
  timeout: 600s                        # Command timeout (10 minutes)
  use_namespaces: true                 # Use Linux namespaces for isolation
  allowed_env_vars:                    # Environment variables to preserve
    - PATH
    - HOME
    - USER
    - LANG

# Health reporting
health:
  disk_warning_threshold: 90           # Disk usage warning threshold (%)
  report_interval: 30s                 # Health report frequency

# Graceful shutdown
graceful_shutdown:
  drain_timeout: 600s                  # Time to wait for deployments to complete
```

## Configuration Sections

### Master

Connection settings for the master server.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `address` | string | - | Master gRPC address (host:port) |
| `token` | string | - | Authentication token |
| `ca_cert` | string | `/etc/vcdeploy/agent/ca.pem` | CA certificate for TLS verification |
| `reconnect.initial_delay` | duration | `1s` | Initial reconnect delay |
| `reconnect.max_delay` | duration | `5m` | Maximum reconnect delay |
| `reconnect.heartbeat_interval` | duration | `10s` | Heartbeat frequency |

### Agent Identity

Agent identification and update settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `id` | string | - | Unique agent identifier (required) |
| `labels` | map | `{}` | Key-value labels for agent selection |
| `update_policy` | string | `immediate` | Agent update policy |
| `update_window_start` | string | - | Update window start (HH:MM) |
| `update_window_end` | string | - | Update window end (HH:MM) |

**Update Policies:**

| Policy | Description |
|--------|-------------|
| `immediate` | Apply updates as soon as available |
| `scheduled` | Apply updates only during update window |
| `manual` | Never auto-update, require manual intervention |

### Paths

Local filesystem paths for deployments.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `data` | string | `/var/lib/vcdeploy/agent/` | Agent database and state directory |
| `releases` | string | `/var/www/` | Root directory for release deployments |

### Archive

Archive caching configuration for deployment archives.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `cache_dir` | string | `/var/lib/vcdeploy/cache/archives` | Local archive cache directory |
| `keep_count` | int | `5` | Maximum cached archives per project |

### Execution

Command execution settings for deployment hooks.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `user` | string | `www-data` | Unix user to run commands as |
| `group` | string | `www-data` | Unix group to run commands as |
| `timeout` | duration | `600s` | Maximum command execution time |
| `use_namespaces` | bool | `true` | Use Linux namespaces for isolation |
| `allowed_env_vars` | list | `[PATH, HOME, USER, LANG]` | Environment variables to preserve |

### Health

Health monitoring and reporting settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `disk_warning_threshold` | int | `90` | Disk usage warning threshold (%) |
| `report_interval` | duration | `30s` | Health report frequency |

### Graceful Shutdown

Shutdown behavior when stopping the agent.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `drain_timeout` | duration | `600s` | Time to wait for deployments to complete |

## Agent Registration

Before the agent can connect, it must be registered with the master.

### Method 1: CLI Registration

```bash
# On the master: generate a registration token
vcdeploy agent token <agent-id>

# On the agent server: register
vcdeploy-agent register --master master.example.com:9001 --token <token>
```

### Method 2: Pre-configured Token

1. Generate a token on the master
2. Add the token to the agent config:

```yaml
master:
  address: "master.example.com:9001"
  token: "vcdeploy_agent_abc123..."
```

### Method 3: Environment Variable

```bash
export VCDEPLOY_AGENT_TOKEN="vcdeploy_agent_abc123..."
vcdeploy-agent start
```

## Labels

Labels are key-value pairs used to select agents for deployments.

```yaml
agent:
  id: "web-prod-01"
  labels:
    environment: production
    role: web
    region: us-east-1
    tier: frontend
```

In project configuration, you can select agents by label:

```yaml
targets:
  production:
    agents:
      - label: environment=production,role=web
```

## Update Policies

### Immediate (Default)

Agent updates are applied as soon as they're available:

```yaml
agent:
  update_policy: immediate
```

### Scheduled

Updates are applied only during a maintenance window:

```yaml
agent:
  update_policy: scheduled
  update_window_start: "02:00"  # 2 AM
  update_window_end: "04:00"    # 4 AM
```

### Manual

Updates are never applied automatically:

```yaml
agent:
  update_policy: manual
```

Trigger updates manually via:
```bash
vcdeploy agent update <agent-id>
```

## Execution Isolation

When `use_namespaces: true`, deployment commands run in isolated Linux namespaces:

- **PID namespace**: Commands can't see other processes
- **Network namespace**: Commands can't access the network directly
- **Mount namespace**: Limited filesystem visibility

This provides security isolation between deployments.

To disable (not recommended for production):

```yaml
execution:
  use_namespaces: false
```

## Environment Variables

| Variable | Config Path | Description |
|----------|-------------|-------------|
| `VCDEPLOY_AGENT_ID` | `agent.id` | Agent identifier |
| `VCDEPLOY_MASTER_ADDRESS` | `master.address` | Master address |
| `VCDEPLOY_AGENT_TOKEN` | `master.token` | Authentication token |

## Systemd Service

For production deployments, use systemd:

```ini
# /etc/systemd/system/vcdeploy-agent.service
[Unit]
Description=VCDeploy Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/vcdeploy-agent start --config /etc/vcdeploy/agent.yaml
Restart=always
RestartSec=5
User=root
Environment=HOME=/root

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable vcdeploy-agent
sudo systemctl start vcdeploy-agent
```

## Launchd Service (macOS)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.blackorder.vcdeploy-agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/vcdeploy-agent</string>
        <string>start</string>
        <string>--config</string>
        <string>/etc/vcdeploy/agent.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

Save to `/Library/LaunchDaemons/com.blackorder.vcdeploy-agent.plist` and load:

```bash
sudo launchctl load /Library/LaunchDaemons/com.blackorder.vcdeploy-agent.plist
```

## Default Values Summary

| Setting | Default |
|---------|---------|
| Reconnect initial delay | `1s` |
| Reconnect max delay | `5m` |
| Heartbeat interval | `10s` |
| Update policy | `immediate` |
| Archive cache dir | `/var/lib/vcdeploy/cache/archives` |\n| Archive keep count | `5` |
| Releases path | `/var/www/` |
| Execution user | `www-data` |
| Execution timeout | `10 minutes` |
| Use namespaces | `true` |
| Disk warning threshold | `90%` |
| Health report interval | `30s` |
| Drain timeout | `10 minutes` |

## Troubleshooting

### Agent Won't Connect

1. Check master address is correct and reachable:
   ```bash
   nc -zv master.example.com 9001
   ```

2. Verify token is valid:
   ```bash
   vcdeploy agent list  # on master
   ```

3. Check TLS certificates:
   ```bash
   openssl s_client -connect master.example.com:9001
   ```

### Deployments Fail

1. Check execution user has permissions:
   ```bash
   sudo -u www-data ls /var/www/
   ```

2. Verify paths exist and are writable:
   ```bash
   ls -la /var/lib/vcdeploy/cache/archives/
   ls -la /var/www/
   ```

3. Check agent logs:
   ```bash
   journalctl -u vcdeploy-agent -f
   ```

## See Also

- [Quick Start](../quickstart.md)
- [Master Configuration](master.md)
- [CLI Reference](../cli/README.md)
