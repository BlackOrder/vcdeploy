# Master Server Architecture

The master server is the central control plane for vcdeploy, responsible for:
- Coordinating deployments across agents
- Managing configuration and secrets
- Serving the web UI and REST API
- Processing webhooks from Git providers

## Components

```
┌─────────────────────────────────────────────────────────────────┐
│                        Master Server                             │
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │   REST API   │  │   gRPC API   │  │   Web UI     │          │
│  │   (HTTP)     │  │   (TLS)      │  │   (HTTP)     │          │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │
│         │                 │                 │                   │
│  ┌──────▼─────────────────▼─────────────────▼───────┐          │
│  │                  Router / Middleware              │          │
│  │  • Authentication    • Rate Limiting              │          │
│  │  • Request ID        • Metrics                    │          │
│  └──────────────────────┬───────────────────────────┘          │
│                         │                                       │
│  ┌──────────────────────▼───────────────────────────┐          │
│  │                Service Layer                      │          │
│  │  • Projects      • Deployments    • Agents        │          │
│  │  • Secrets       • Users          • Settings      │          │
│  └──────────────────────┬───────────────────────────┘          │
│                         │                                       │
│  ┌──────────────────────▼───────────────────────────┐          │
│  │              Storage Layer (SQLite)               │          │
│  └──────────────────────────────────────────────────┘          │
└─────────────────────────────────────────────────────────────────┘
```

## Deployment Workflow

1. **Trigger**: Webhook received or manual trigger via API/UI
2. **Validation**: Project configuration validated
3. **Scheduling**: Deployment queued for target agents
4. **Execution**: Commands sent to agents via gRPC
5. **Monitoring**: Progress tracked via heartbeats
6. **Completion**: Final status recorded, notifications sent

## High Availability

The master server supports:
- Graceful shutdown with in-flight request completion
- Automatic database migrations on startup
- Configuration hot-reload for certain settings

## Security

- HTTPS/TLS for all external communication
- mTLS for agent connections
- JWT-based authentication for API access
- RBAC for user permissions
- Encrypted secrets storage

## Configuration

See [Master Configuration](config/master.md) for detailed configuration options.

## Resource Requirements

| Deployment Size | CPU | Memory | Disk |
|----------------|-----|--------|------|
| Small (<10 agents) | 1 core | 512MB | 1GB |
| Medium (10-50 agents) | 2 cores | 1GB | 5GB |
| Large (50+ agents) | 4 cores | 2GB | 10GB |
