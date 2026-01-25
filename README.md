# vcdeploy

A deployment platform with master-agent architecture for automated, webhook-driven deployments.

## Features

- **Webhook-driven deployments** from GitHub, GitLab, and Bitbucket
- **Hybrid deployment targets**: Agent-based (real-time logs) or SSH-based (no agent required)
- **Zero-downtime deployments** using symlink-based releases
- **Centralized configuration**: Project configs and secrets stored on master
- **Full web UI** for management with dark mode default
- **RBAC** with optional 2FA for UI access
- **Comprehensive audit logging** for CLI and UI actions
- **Automatic backups** for database and config files
- **Auto key rotation** for secrets encryption

## Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                        vcdeploy Master                         │
│  • HTTP API + Web UI (port 9000)                               │
│  • gRPC server for agents (port 9001)                          │
│  • SQLite database                                             │
│  • Webhook receiver                                            │
└────────────────────────────────────────────────────────────────┘
        │                                    │
        │ gRPC (persistent)                  │ SSH (on-demand)
        ▼                                    ▼
┌─────────────────┐                ┌─────────────────┐
│    Agent        │                │   SSH Target    │
│  • Real-time    │                │  • Jump server  │
│    log stream   │                │    support      │
│  • Health       │                │  • No agent     │
│    metrics      │                │    required     │
└─────────────────┘                └─────────────────┘
```

## Quick Start

### Install

```bash
# Build from source
make build

# Install binaries
sudo make install

# Install systemd services
sudo make install-systemd
```

### Initialize Master

```bash
# Create config directory
sudo mkdir -p /etc/vcdeploy/{tls,keys,projects,types}
sudo mkdir -p /var/lib/vcdeploy/{backups,data}

# Copy example config
sudo cp configs/master.yaml.example /etc/vcdeploy/master.yaml

# Start master (first run generates admin password)
vcdeploy master start
```

### Register an Agent

```bash
# On the agent server
sudo mkdir -p /etc/vcdeploy/agent
sudo cp configs/agent.yaml.example /etc/vcdeploy/agent.yaml

# Edit config to set agent ID and master address
sudo vim /etc/vcdeploy/agent.yaml

# Register with master
vcdeploy-agent register --master=master.example.com:9001 --token=<token>

# Start agent
vcdeploy-agent start
```

### Create a Project

```bash
# Add project type (or use built-in: laravel, symfony, nextjs, nodejs, static)
vcdeploy type create my-custom-type

# Add project
vcdeploy project add my-app

# Set secrets
vcdeploy secret set my-app/_default DB_HOST
vcdeploy secret set my-app/production DB_PASSWORD
vcdeploy secret set my-app/staging DB_PASSWORD

# Deploy
vcdeploy project deploy my-app --target=staging
```

## Configuration

### Master Config (`/etc/vcdeploy/master.yaml`)

```yaml
server:
  listen: ":9000"
  tls:
    enabled: true
    cert: /etc/vcdeploy/tls/cert.pem
    key: /etc/vcdeploy/tls/key.pem

grpc:
  listen: ":9001"

security:
  key_rotation:
    enabled: true
    interval: 720h  # 30 days
  require_2fa_admin: true
```

### Project Config (`/etc/vcdeploy/projects/<name>.yaml`)

```yaml
name: my-app
type: laravel
repository: git@github.com:org/my-app.git

watch:
  branches: [main, develop]
  guards:
    reject_force_push: true

targets:
  production:
    agents: [prod-web-01, prod-web-02]
    branch: main
    path: /var/www/my-app
```

See `configs/` directory for complete examples.

## CLI Reference

```bash
# Master management
vcdeploy master start|stop|status
vcdeploy master rotate-key
vcdeploy master backup create|list|restore

# Project management
vcdeploy project list|add|edit|delete|validate
vcdeploy project deploy <name> [--target=x] [--dry-run] [--force]
vcdeploy project rollback <name> [--target=x] [--release=n]

# Type management
vcdeploy type list|create|edit|delete

# Secrets management
vcdeploy secret set <project/scope> <key>
vcdeploy secret list <project>
vcdeploy secret delete <project/scope> <key>
vcdeploy secret import <project/scope>
vcdeploy secret backup --output=file.vcbackup
vcdeploy secret restore file.vcbackup
```

## Deployment Strategy

vcdeploy uses a symlink-based deployment strategy (like Capistrano/Deployer):

```
/var/www/my-app/
├── current -> releases/5       # Symlink to active release
├── releases/
│   ├── 1/
│   ├── 2/
│   ├── 3/
│   ├── 4/
│   └── 5/                      # Latest release
├── shared/                     # Persistent files
│   ├── .env
│   └── storage/
└── repo/                       # Git cache
```

- **Zero-downtime**: Atomic symlink swap
- **Instant rollback**: Just update symlink to previous release
- **Shared files**: `.env`, uploads, etc. persist across releases

## Security

- **Secrets**: AES-256-GCM encrypted in SQLite, master key from file or env
- **Key rotation**: Automatic monthly rotation (configurable)
- **Authentication**: Password + optional TOTP 2FA
- **Agent auth**: mTLS certificates after initial token registration
- **Webhook auth**: Signature verification (GitHub HMAC, GitLab token)
- **Audit logging**: All actions logged with user, source, timestamp

## Performance Tuning

### Experimental JSON v2 (Go 1.25+)

vcdeploy can benefit from the experimental JSON v2 implementation in Go 1.25, which provides improved performance for JSON encoding/decoding operations (used heavily in webhook processing and API responses).

To enable:

```bash
# Build with experimental JSON v2
GOEXPERIMENT=jsonv2 go build ./...

# Or run with experimental JSON v2
GOEXPERIMENT=jsonv2 ./vcdeploy master start
```

**Note:** This is an experimental feature. Test thoroughly before using in production.

### Experimental Green Tea GC (Go 1.25+)

For long-running server processes, the experimental Green Tea garbage collector can reduce GC overhead by 10-40%:

```bash
GOEXPERIMENT=greenteagc ./vcdeploy master start
```

## License

MIT License - see LICENSE file.
