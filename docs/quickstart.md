# Quick Start

Get vcdeploy up and running in 5 minutes.

## Prerequisites

- Linux, macOS, or FreeBSD
- Network connectivity between master and agents
- SQLite3 (included in most systems)

## Installation

<!-- tabs:start -->

### **Homebrew**

```bash
# Add the tap
brew tap blackorder/tap

# Install vcdeploy
brew install vcdeploy
```

### **Debian/Ubuntu**

```bash
# Download the latest .deb package
curl -LO https://github.com/blackorder/vcdeploy/releases/latest/download/vcdeploy_amd64.deb

# Install
sudo dpkg -i vcdeploy_amd64.deb
```

### **RHEL/CentOS/Fedora**

```bash
# Download the latest .rpm package
curl -LO https://github.com/blackorder/vcdeploy/releases/latest/download/vcdeploy_amd64.rpm

# Install
sudo rpm -i vcdeploy_amd64.rpm
```

### **Binary**

```bash
# Download and extract
curl -sSL https://github.com/blackorder/vcdeploy/releases/latest/download/vcdeploy_linux_amd64.tar.gz | tar xz

# Move to PATH
sudo mv vcdeploy vcdeploy-agent /usr/local/bin/
```

<!-- tabs:end -->

## Master Server Setup

### 1. Create Configuration

```yaml
# /etc/vcdeploy/master.yaml
server:
  listen: ":9000"
  tls:
    enabled: false

grpc:
  listen: ":9001"

security:
  session_timeout: 24h
  require_2fa_admin: false
  key_rotation:
    enabled: true
    interval: 720h  # 30 days

logs:
  application:
    level: info
    retention: 720h
  deployment:
    retention: 2160h  # 90 days
    max_size_mb: 100
  audit:
    retention: 8760h  # 365 days

backup:
  database:
    enabled: true
    interval: 24h
    retention: 168h  # 7 days
    path: /var/lib/vcdeploy/backups
```

### 2. Start the Server

```bash
# Start in foreground
vcdeploy master start

# Or via systemd (recommended for production)
sudo systemctl enable --now vcdeploy-master
```

### 3. Access the Web UI

Open http://localhost:9000 in your browser.

**First-time setup:** You'll be redirected to the setup wizard to create your admin account.

**Or create admin via CLI:**
```bash
vcdeploy admin --username admin --email admin@example.com
```

## Agent Setup

Agents run on target servers and execute deployments.

### 1. Create Configuration

```yaml
# /etc/vcdeploy/agent.yaml
master:
  address: "master.example.com:9001"
  token: ""  # Set after registration
  cert: /etc/vcdeploy/agent/cert.pem
  allow_insecure: false
  reconnect:
    initial_delay: 1s
    max_delay: 5m
    heartbeat_interval: 10s

agent:
  id: "agent-001"
  labels:
    environment: production
    role: web
  update_policy: immediate

paths:
  releases: /var/www/

archive:
  cache_dir: /var/lib/vcdeploy/cache/archives
  keep_count: 5

execution:
  user: www-data
  group: www-data
  timeout: 600s
  use_namespaces: true
  allowed_env_vars:
    - PATH
    - HOME
    - USER
    - LANG

health:
  disk_warning_threshold: 90
  report_interval: 30s

graceful_shutdown:
  drain_timeout: 600s
```

### 2. Register and Start the Agent

```bash
# Register with master (get token from master UI or CLI)
vcdeploy-agent register --master master.example.com:9001 --token <registration-token>

# Start the agent
vcdeploy-agent start

# Or via systemd (recommended)
sudo systemctl enable --now vcdeploy-agent
```

## Create Your First Project

### 1. Add a Project via CLI

```bash
vcdeploy project create myapp
```

You'll be prompted for:
- Repository URL
- Default branch
- Deploy path
- Project type

### 2. Configure the Project

Project configurations are stored in the database. You can also create a YAML file:

```yaml
# Project configuration (via Web UI or API)
name: myapp
type: laravel  # or: nodejs, python, static, custom
repository: git@github.com:myorg/myapp.git
archived: false

watch:
  branches:
    - main
    - develop
  actions:
    - push
    - pull_request.merged
  guards:
    reject_force_push: true
    require_ci_pass: false

deployment:
  on_busy: cancel  # cancel | queue | skip
  strategy: symlink
  keep_releases: 5

targets:
  production:
    agents:
      - agent-001
      - agent-002
    branch: main
    path: /var/www/myapp

hooks:
  pre_deploy:
    - command: systemctl stop myapp
  post_deploy:
    - command: composer install --no-dev
    - command: php artisan migrate --force
    - command: php artisan config:cache
  reload:
    - service: php-fpm
      action: reload
  rollback:
    - command: php artisan migrate:rollback

health:
  url: https://myapp.example.com/health
  timeout: 30s
  retries: 3
  rollback_on_fail: true
```

### 3. Set Secrets

```bash
# Set database password
vcdeploy secret set myapp DB_PASSWORD

# Set from stdin
echo "your-api-key" | vcdeploy secret set myapp API_KEY --stdin

# Import from .env file
cat .env.production | vcdeploy secret import myapp/production
```

### 4. Deploy

```bash
# Deploy via CLI
vcdeploy project deploy myapp

# Deploy specific target
vcdeploy project deploy myapp --target production

# Dry run (validate without deploying)
vcdeploy project deploy myapp --dry-run
```

### 5. Monitor Deployment

```bash
# List recent deployments
vcdeploy deploy list

# Check status
vcdeploy deploy show <deployment-id>

# View logs
vcdeploy deploy logs <deployment-id> --follow
```

## Webhook Integration

### GitHub

1. In your GitHub repository, go to **Settings → Webhooks**
2. Add webhook:
   - **Payload URL:** `https://your-master:9000/webhook/github/{project-name}`
   - **Content type:** `application/json`
   - **Secret:** Configure in vcdeploy settings
   - **Events:** Push, Pull Requests

### GitLab

1. In your GitLab project, go to **Settings → Webhooks**
2. Add webhook:
   - **URL:** `https://your-master:9000/webhooks/gitlab`
   - **Secret Token:** Configure in vcdeploy settings
   - **Trigger:** Push events, Merge request events

### Bitbucket

1. In your Bitbucket repository, go to **Settings → Webhooks**
2. Add webhook:
   - **URL:** `https://your-master:9000/webhooks/bitbucket`
   - **Triggers:** Repository push

## Next Steps

- [Installation Guide](installation.md) - Detailed installation options
- [Master Configuration](config/master.md) - All configuration options
- [Agent Configuration](config/agent.md) - Agent setup and options
- [Projects Configuration](config/projects.md) - Advanced project settings
- [CLI Reference](cli/README.md) - Complete command reference
- [API Reference](api.md) - REST API documentation
