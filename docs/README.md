<p align="center">
  <img src="assets/logo.svg" alt="vcdeploy" width="240">
</p>

<h1 align="center">vcdeploy</h1>

<p align="center">
  <strong>A modern deployment orchestration platform for Unix systems</strong>
</p>

<p align="center">
  <a href="https://github.com/blackorder/vcdeploy/actions/workflows/ci.yml"><img src="https://github.com/blackorder/vcdeploy/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
[![Go Report Card](https://goreportcard.com/badge/github.com/blackorder/vcdeploy)](https://goreportcard.com/report/github.com/blackorder/vcdeploy)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## What is vcdeploy?

vcdeploy is a lightweight, secure deployment orchestration platform designed for Unix systems. It provides:

- **Centralized Control**: Master server manages deployments across multiple agents
- **Real-time Streaming**: gRPC-based communication with bidirectional streaming
- **Secure by Default**: mTLS encryption, secret management, and audit logging
- **Modern Observability**: Prometheus metrics, structured logging, and health checks

## Quick Start

```bash
# Install via Homebrew (macOS/Linux)
brew install blackorder/tap/vcdeploy

# Or download from releases
curl -sSL https://github.com/blackorder/vcdeploy/releases/latest/download/vcdeploy_linux_amd64.tar.gz | tar xz

# Start the master server
vcdeploy master --config /etc/vcdeploy/master.yaml

# Start an agent
vcdeploy agent --config /etc/vcdeploy/agent.yaml
```

## Architecture

```
┌─────────────────┐         ┌─────────────────┐
│   Master Server │◄────────┤   Web UI / CLI  │
│                 │  HTTP   │                 │
│  - REST API     │         └─────────────────┘
│  - gRPC Server  │
│  - SQLite DB    │         ┌─────────────────┐
│  - Scheduler    │◄────────┤   Webhooks      │
└────────┬────────┘  HTTP   │  (GitHub, etc)  │
         │                  └─────────────────┘
         │ gRPC (mTLS)
         │
    ┌────┴────┬────────────┐
    │         │            │
    ▼         ▼            ▼
┌───────┐ ┌───────┐ ┌───────────┐
│ Agent │ │ Agent │ │   Agent   │
│  #1   │ │  #2   │ │    #N     │
└───────┘ └───────┘ └───────────┘
```

## Features

### Deployment Orchestration
- Project-based deployment configurations
- Target-based deployment (agent or master-local)
- Pre/post deployment hooks
- Rollback support
- Health checks integration

### Security
- mTLS for agent communication
- Built-in secret management (AES-256-GCM)
- Audit logging for compliance
- Role-based access control

### Observability
- Prometheus metrics endpoint (`/metrics`)
- Kubernetes-style health probes (`/healthz`, `/readyz`, `/livez`)
- Structured JSON logging with request correlation
- Real-time deployment log streaming

### Platform Support
- **Linux**: amd64, arm64 (deb, rpm, tar.gz)
- **macOS**: amd64, arm64 (Homebrew, tar.gz)
- **FreeBSD**: amd64, arm64 (tar.gz)
- **Docker**: Multi-arch images available

## Documentation

- [Quick Start Guide](quickstart.md) - Get up and running in 5 minutes
- [Installation Guide](installation.md) - Detailed installation instructions
- [Architecture Overview](architecture/overview.md) - How vcdeploy works
- [Targets Configuration](config/targets.md) - Deployment target setup
- [API Reference](api.md) - REST API documentation
- [gRPC API](api/grpc.md) - Agent communication protocol

## License

vcdeploy is licensed under the MIT License. See [LICENSE](https://github.com/blackorder/vcdeploy/blob/main/LICENSE) for details.
