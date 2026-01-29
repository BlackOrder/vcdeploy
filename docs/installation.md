# Installation

Detailed installation instructions for all supported platforms.

## System Requirements

### Minimum Requirements

| Component | Requirement |
|-----------|-------------|
| OS | Linux, macOS, or FreeBSD |
| Architecture | amd64 or arm64 |
| Memory | 256MB (master), 64MB (agent) |
| Disk | 100MB for binaries + database |
| Network | TCP ports 8080 (HTTP), 9090 (gRPC) |

### Recommended for Production

| Component | Recommendation |
|-----------|----------------|
| Memory | 1GB (master), 256MB (agent) |
| Disk | SSD with 10GB+ free space |
| Database | SQLite (default) or PostgreSQL |

## Installation Methods

### Package Managers

#### Homebrew (macOS/Linux)

```bash
# Add the tap
brew tap blackorder/tap

# Install
brew install vcdeploy

# Verify
vcdeploy version
```

#### APT (Debian/Ubuntu)

```bash
# Download latest .deb
VERSION=$(curl -s https://api.github.com/repos/blackorder/vcdeploy/releases/latest | grep tag_name | cut -d'"' -f4)
curl -LO "https://github.com/blackorder/vcdeploy/releases/download/${VERSION}/vcdeploy_${VERSION#v}_linux_amd64.deb"

# Install
sudo apt install ./vcdeploy_*.deb

# Verify
vcdeploy version
```

#### YUM/DNF (RHEL/CentOS/Fedora)

```bash
# Download latest .rpm
VERSION=$(curl -s https://api.github.com/repos/blackorder/vcdeploy/releases/latest | grep tag_name | cut -d'"' -f4)
curl -LO "https://github.com/blackorder/vcdeploy/releases/download/${VERSION}/vcdeploy_${VERSION#v}_linux_amd64.rpm"

# Install
sudo rpm -i vcdeploy_*.rpm

# Verify
vcdeploy version
```

### Binary Downloads

Download pre-compiled binaries from the [releases page](https://github.com/blackorder/vcdeploy/releases).

```bash
# Linux amd64
curl -sSL https://github.com/blackorder/vcdeploy/releases/latest/download/vcdeploy_linux_amd64.tar.gz | tar xz
sudo mv vcdeploy /usr/local/bin/

# Linux arm64
curl -sSL https://github.com/blackorder/vcdeploy/releases/latest/download/vcdeploy_linux_arm64.tar.gz | tar xz
sudo mv vcdeploy /usr/local/bin/

# macOS amd64
curl -sSL https://github.com/blackorder/vcdeploy/releases/latest/download/vcdeploy_darwin_amd64.tar.gz | tar xz
sudo mv vcdeploy /usr/local/bin/

# macOS arm64 (Apple Silicon)
curl -sSL https://github.com/blackorder/vcdeploy/releases/latest/download/vcdeploy_darwin_arm64.tar.gz | tar xz
sudo mv vcdeploy /usr/local/bin/

# FreeBSD amd64
curl -sSL https://github.com/blackorder/vcdeploy/releases/latest/download/vcdeploy_freebsd_amd64.tar.gz | tar xz
sudo mv vcdeploy /usr/local/bin/
```

### Docker

```bash
# Pull the image
docker pull ghcr.io/blackorder/vcdeploy:latest

# Run master server
docker run -d \
  --name vcdeploy-master \
  -p 8080:8080 \
  -p 9090:9090 \
  -v /var/lib/vcdeploy:/data \
  -v /etc/vcdeploy:/etc/vcdeploy:ro \
  ghcr.io/blackorder/vcdeploy:latest \
  master --config /etc/vcdeploy/master.yaml
```

### Building from Source

See [Building from Source](development/building.md) for instructions.

## Service Installation

### systemd (Linux)

The packages automatically install systemd unit files. For manual installation:

```bash
# Master service
sudo cp /path/to/vcdeploy-master.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable vcdeploy-master
sudo systemctl start vcdeploy-master

# Agent service
sudo cp /path/to/vcdeploy-agent.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable vcdeploy-agent
sudo systemctl start vcdeploy-agent
```

### launchd (macOS)

```bash
# Copy plist files
sudo cp com.blackorder.vcdeploy.master.plist /Library/LaunchDaemons/
sudo cp com.blackorder.vcdeploy.agent.plist /Library/LaunchDaemons/

# Load services
sudo launchctl load /Library/LaunchDaemons/com.blackorder.vcdeploy.master.plist
sudo launchctl load /Library/LaunchDaemons/com.blackorder.vcdeploy.agent.plist
```

### OpenRC (Alpine/Gentoo)

```bash
# Copy init scripts
sudo cp vcdeploy-master /etc/init.d/
sudo cp vcdeploy-agent /etc/init.d/

# Enable services
sudo rc-update add vcdeploy-master default
sudo rc-update add vcdeploy-agent default

# Start services
sudo rc-service vcdeploy-master start
sudo rc-service vcdeploy-agent start
```

### FreeBSD rc.d

```bash
# Copy rc scripts
sudo cp vcdeploy_master /usr/local/etc/rc.d/
sudo cp vcdeploy_agent /usr/local/etc/rc.d/

# Enable in /etc/rc.conf
echo 'vcdeploy_master_enable="YES"' | sudo tee -a /etc/rc.conf
echo 'vcdeploy_agent_enable="YES"' | sudo tee -a /etc/rc.conf

# Start services
sudo service vcdeploy_master start
sudo service vcdeploy_agent start
```

## Directory Structure

After installation, vcdeploy uses the following directories:

| Path | Purpose |
|------|---------|
| `/etc/vcdeploy/` | Configuration files |
| `/var/lib/vcdeploy/` | Database and state |
| `/var/log/vcdeploy/` | Log files (if file logging enabled) |
| `/usr/local/bin/vcdeploy` | Binary (manual install) |
| `/usr/bin/vcdeploy` | Binary (package install) |

## Upgrading

### Package Managers

```bash
# Homebrew
brew upgrade vcdeploy

# APT
sudo apt update && sudo apt upgrade vcdeploy

# YUM/DNF
sudo yum update vcdeploy
```

### Binary

1. Stop services
2. Download new binary
3. Replace old binary
4. Start services

```bash
sudo systemctl stop vcdeploy-master vcdeploy-agent
curl -sSL https://github.com/blackorder/vcdeploy/releases/latest/download/vcdeploy_linux_amd64.tar.gz | tar xz
sudo mv vcdeploy /usr/local/bin/
sudo systemctl start vcdeploy-master vcdeploy-agent
```

## Next Steps

- [Quick Start](quickstart.md) - Basic setup tutorial
- [Master Configuration](config/master.md) - Configure the master server
- [Agent Configuration](config/agent.md) - Configure agents
