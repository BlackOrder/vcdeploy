# Agent Configuration

Agents are configured via a YAML file, typically located at `/etc/vcdeploy/agent.yaml`.

## Configuration File

```yaml
# /etc/vcdeploy/agent.yaml

# Master connection
master_address: "master.example.com:9001"
token: "agent-secret-token"

# Agent identity
agent_id: ""                    # Auto-generated if empty
hostname: ""                    # Auto-detected if empty
labels:
  environment: "production"
  region: "us-east-1"

# TLS settings (optional)
tls:
  enabled: true
  insecure: false              # Skip certificate verification
  cert_file: ""                # For mTLS authentication
  key_file: ""
  ca_file: ""                  # Custom CA certificate

# Connection settings
connection:
  heartbeat_interval: "30s"
  reconnect_delay: "5s"
  max_reconnect_delay: "5m"

# Deployment settings
deploy:
  base_path: "/var/www"        # Base directory for deployments
  keep_releases: 5             # Number of releases to retain
  shared_dirs:                 # Directories shared between releases
    - "logs"
    - "uploads"
    - "storage"
  shared_files:                # Files shared between releases
    - ".env"
  
# Execution settings
executor:
  timeout: "30m"               # Max deployment time
  shell: "/bin/bash"           # Shell for commands
  working_dir: ""              # Override working directory
  environment:                 # Additional environment variables
    PATH: "/usr/local/bin:/usr/bin:/bin"

# Logging
logging:
  level: "info"
  format: "json"
  output: "stdout"
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `VCDEPLOY_AGENT_MASTER_ADDRESS` | Master server address |
| `VCDEPLOY_AGENT_TOKEN` | Authentication token |
| `VCDEPLOY_AGENT_ID` | Agent identifier |
| `VCDEPLOY_AGENT_HOSTNAME` | Override hostname |
| `VCDEPLOY_AGENT_LOG_LEVEL` | Log verbosity |

## Agent Labels

Labels are used for targeting specific agents:

```yaml
labels:
  environment: "production"
  region: "us-east-1"
  tier: "web"
```

In project configuration:
```yaml
deploy:
  targets:
    - agent_labels:
        environment: "production"
        tier: "web"
```

## Directory Structure

After deployment:
```
/var/www/myapp/
├── current -> releases/20240115-120000/
├── releases/
│   ├── 20240115-120000/
│   ├── 20240114-093000/
│   └── 20240113-150000/
└── shared/
    ├── logs/
    ├── uploads/
    └── .env
```

## Shared Files/Directories

Shared items persist across deployments:

```yaml
deploy:
  shared_dirs:
    - "logs"           # Logs persist
    - "storage"        # User uploads
    - "node_modules"   # Speed up builds
  shared_files:
    - ".env"           # Environment config
    - "config/database.yml"
```

## Git Configuration

For private repositories:

```yaml
git:
  ssh_key_path: "/etc/vcdeploy/deploy_key"
  strict_host_key_checking: false
```

Or use deploy tokens:
```yaml
git:
  username: "deploy-token"
  password: "glpat-xxxxxxxxxxxx"
```

## Resource Limits

```yaml
executor:
  resource_limits:
    max_memory: "2G"
    max_cpu: 2
    max_file_size: "100M"
```

## Security

The agent runs with minimal permissions:
- Creates directories under `base_path`
- Executes configured commands
- No inbound network connections

For additional security:
```yaml
security:
  allowed_commands:
    - "npm"
    - "yarn"
    - "composer"
    - "make"
  disallowed_paths:
    - "/etc"
    - "/root"
```

## Default Values

| Setting | Default |
|---------|---------|
| Heartbeat interval | `30s` |
| Keep releases | `5` |
| Deployment timeout | `30m` |
| Shell | `/bin/bash` |
| Log level | `info` |
