# Architecture Overview

vcdeploy follows a master-agent architecture for distributed deployment orchestration.

## Components

```
┌─────────────────────────────────────────────────────────────┐
│                      Master Server                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │ REST API │  │ Web UI   │  │ gRPC     │  │ Webhook  │   │
│  │ Handler  │  │ Server   │  │ Server   │  │ Handler  │   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘   │
│       │             │             │             │          │
│       └─────────────┴──────┬──────┴─────────────┘          │
│                            │                               │
│                    ┌───────▼───────┐                       │
│                    │   Business    │                       │
│                    │    Logic      │                       │
│                    └───────┬───────┘                       │
│                            │                               │
│       ┌────────────┬───────┴───────┬────────────┐         │
│       │            │               │            │          │
│  ┌────▼────┐  ┌────▼────┐   ┌─────▼─────┐ ┌────▼────┐    │
│  │ SQLite  │  │ Secret  │   │ Scheduler │ │ Agent   │    │
│  │   DB    │  │   KMS   │   │           │ │ Manager │    │
│  └─────────┘  └─────────┘   └───────────┘ └─────────┘    │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ gRPC (mTLS)
                              │
         ┌────────────────────┼────────────────────┐
         │                    │                    │
         ▼                    ▼                    ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│     Agent 1     │  │     Agent 2     │  │     Agent N     │
│  ┌───────────┐  │  │  ┌───────────┐  │  │  ┌───────────┐  │
│  │   gRPC    │  │  │  │   gRPC    │  │  │  │   gRPC    │  │
│  │  Client   │  │  │  │  Client   │  │  │  │  Client   │  │
│  └─────┬─────┘  │  │  └─────┬─────┘  │  │  └─────┬─────┘  │
│        │        │  │        │        │  │        │        │
│  ┌─────▼─────┐  │  │  ┌─────▼─────┐  │  │  ┌─────▼─────┐  │
│  │  Deploy   │  │  │  │  Deploy   │  │  │  │  Deploy   │  │
│  │  Engine   │  │  │  │  Engine   │  │  │  │  Engine   │  │
│  └───────────┘  │  │  └───────────┘  │  │  └───────────┘  │
└─────────────────┘  └─────────────────┘  └─────────────────┘
```

## Master Server

The master server is the central control plane:

- **REST API**: Handles HTTP requests from CLI and external systems
- **Web UI**: Browser-based management interface
- **gRPC Server**: Bidirectional streaming with agents
- **Webhook Handler**: Receives webhooks from Git providers

### Database

SQLite is used by default for simplicity. The schema includes:
- Users and authentication
- Agents and connection state
- Projects and configurations
- Deployments and logs
- Audit trail

### Secret Management

Secrets are encrypted using AES-256-GCM with a KMS key stored separately from the database.

## Agents

Agents run on target servers and execute deployments:

- **gRPC Client**: Maintains persistent connection to master
- **Deploy Engine**: Executes deployment steps
- **Health Reporting**: Sends metrics to master

### Agent Lifecycle

1. Agent starts and reads configuration
2. Connects to master via gRPC
3. Authenticates and registers
4. Enters command loop
5. Executes received commands
6. Reports results via stream

## Communication Flow

### Deployment Flow

```
User → REST API → Master → gRPC Stream → Agent
                    ↑                      ↓
                    └──── Results ─────────┘
```

### Webhook Flow

```
Git Provider → Webhook → Master → Project Match → Deploy
```

## Security Model

- **Transport**: mTLS between master and agents
- **Authentication**: JWT tokens for API access
- **Authorization**: Role-based access control
- **Secrets**: Encrypted at rest with AES-256-GCM
- **Audit**: All operations logged with user context

## Next Steps

- [Master Server](master.md) - Detailed master architecture
- [Agents](agents.md) - Agent architecture and lifecycle
- [Communication](grpc.md) - gRPC protocol details
