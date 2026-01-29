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
sudo mv vcdeploy /usr/local/bin/
```

<!-- tabs:end -->

## Master Server Setup

### 1. Create Configuration

```yaml
# /etc/vcdeploy/master.yaml
server:
  http_addr: ":8080"
  grpc_addr: ":9090"

database:
  path: /var/lib/vcdeploy/vcdeploy.db

security:
  session_secret: "generate-a-random-32-byte-string"
  kms_key: "generate-a-random-32-byte-key"

logging:
  level: info
  format: json
```

### 2. Start the Server

```bash
# Direct
vcdeploy master --config /etc/vcdeploy/master.yaml

# Or via systemd
sudo systemctl enable --now vcdeploy-master
```

### 3. Access the Web UI

Open http://localhost:8080 in your browser. Default credentials:
- Username: `admin`
- Password: `admin` (change immediately!)

## Agent Setup

### 1. Create Configuration

```yaml
# /etc/vcdeploy/agent.yaml
agent:
  id: "agent-001"
  master_addr: "master.example.com:9090"

security:
  # TLS certificates for mTLS (optional but recommended)
  cert_file: /etc/vcdeploy/certs/agent.crt
  key_file: /etc/vcdeploy/certs/agent.key
  ca_file: /etc/vcdeploy/certs/ca.crt

logging:
  level: info
  format: json
```

### 2. Start the Agent

```bash
# Direct
vcdeploy agent --config /etc/vcdeploy/agent.yaml

# Or via systemd
sudo systemctl enable --now vcdeploy-agent
```

## Create Your First Project

### 1. Define a Project

```yaml
# /etc/vcdeploy/projects/myapp.yaml
name: myapp
repo: git@github.com:myorg/myapp.git
branch: main
deploy_path: /var/www/myapp

agents:
  - agent-001
  - agent-002

hooks:
  pre_deploy:
    - "systemctl stop myapp"
  post_deploy:
    - "composer install --no-dev"
    - "systemctl start myapp"

health_check:
  url: "http://localhost:8080/health"
  timeout: 30s
```

### 2. Deploy

```bash
# Via CLI
vcdeploy deploy myapp

# Or via API
curl -X POST http://localhost:8080/api/v1/deployments \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"project": "myapp"}'
```

## Next Steps

- [Installation Guide](installation.md) - Detailed installation options
- [Master Configuration](config/master.md) - All configuration options
- [Projects Configuration](config/projects.md) - Advanced project settings
- [Metrics & Monitoring](operations/metrics.md) - Set up observability
