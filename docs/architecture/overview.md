# Architecture Overview

vcdeploy follows a master-agent architecture for distributed deployment orchestration.

## System Components

```
┌───────────────────────────────────────────────────────────────────────────────┐
│                              Master Server                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │
│  │ REST API │  │ Web UI   │  │ gRPC     │  │ Webhook  │  │ Notification │   │
│  │ Handler  │  │ Server   │  │ Server   │  │ Handler  │  │   Manager    │   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────┬───────┘   │
│       │             │             │             │                │           │
│       └─────────────┴──────┬──────┴─────────────┴────────────────┘           │
│                            │                                                  │
│  ┌─────────────────────────▼──────────────────────────────────────────────┐  │
│  │                        Security Middleware                              │  │
│  │   • Auth (JWT/Session)   • RBAC Enforcement   • CSP   • Rate Limiting │  │
│  └─────────────────────────────────────────────────────────────────────────┘  │
│                            │                                                  │
│  ┌─────────────────────────▼──────────────────────────────────────────────┐  │
│  │                         Service Layer                                   │  │
│  │  ┌──────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐            │  │
│  │  │ Projects │ │Deployments │ │  Secrets   │ │   Users    │            │  │
│  │  └──────────┘ └────────────┘ └────────────┘ └────────────┘            │  │
│  │  ┌──────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐            │  │
│  │  │  Agents  │ │  API Keys  │ │  Sessions  │ │  Webhooks  │            │  │
│  │  └──────────┘ └────────────┘ └────────────┘ └────────────┘            │  │
│  │  ┌──────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐            │  │
│  │  │  Audit   │ │ Host Keys  │ │ Provision  │ │ Proj Types │            │  │
│  │  └──────────┘ └────────────┘ └────────────┘ └────────────┘            │  │
│  └─────────────────────────────────────────────────────────────────────────┘  │
│                            │                                                  │
│       ┌────────────┬───────┴───────┬───────────────┬────────────────┐        │
│       │            │               │               │                │         │
│  ┌────▼────┐  ┌────▼────┐   ┌─────▼─────┐   ┌─────▼─────┐   ┌──────▼─────┐  │
│  │ SQLite  │  │   KMS   │   │ Scheduler │   │  Alert    │   │  Tracing   │  │
│  │   DB    │  │         │   │           │   │  Manager  │   │ (OTel)     │  │
│  └─────────┘  └─────────┘   └───────────┘   └───────────┘   └────────────┘  │
└───────────────────────────────────────────────────────────────────────────────┘
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

The master server is the central control plane responsible for:

- **Orchestration**: Coordinating deployments across multiple agents
- **Configuration**: Managing projects, secrets, and settings
- **Interface**: Providing REST API, gRPC API, and Web UI
- **Webhooks**: Processing Git provider events (GitHub, GitLab, Bitbucket)
- **Notifications**: Sending alerts via Slack, Discord, Email, and custom webhooks
- **Monitoring**: Tracking agent health and deployment status

### Entry Points

| Interface | Default Port | Purpose |
|-----------|--------------|---------|
| REST API | 9000 | CLI, automation, integrations |
| Web UI | 9000 | Browser-based management |
| gRPC | 9001 | Agent communication |
| Webhooks | 9000 | Git provider callbacks |

### Security Middleware Stack

```
Request → Auth → RBAC → CSP → Rate Limit → Handler
```

- **Authentication**: JWT tokens for API, sessions for Web UI
- **RBAC Enforcement**: Role-based access control (admin, user, viewer)
- **CSP Middleware**: Content Security Policy headers
- **Rate Limiting**: Request throttling per user/IP

### Service Layer

The service layer encapsulates business logic:

| Service | Responsibility |
|---------|---------------|
| `ProjectService` | CRUD operations for projects |
| `DeploymentService` | Deployment orchestration |
| `SecretService` | Encrypted secret management |
| `UserService` | User account management |
| `AgentService` | Agent registration and status |
| `TargetService` | Deployment target management |
| `ArchiveService` | Archive creation and caching |
| `APIKeyService` | API key lifecycle |
| `SessionService` | User session management |
| `WebhookService` | Webhook configuration |
| `AuditService` | Operation logging |
| `HostKeyService` | SSH known hosts management |
| `ProvisionService` | Remote server provisioning |
| `ProjectTypeService` | Project templates |

### Storage Layer

SQLite database with multiple logical stores:

```
┌─────────────────────────────────────────────────┐
│                   SQLite DB                      │
├─────────────────────────────────────────────────┤
│  • Users & Sessions    • Projects & Types       │
│  • Agents & Status     • Deployments & Logs     │
│  • Targets             • Deployment Targets     │
│  • Deployment Env Snapshots • Archive Cache       │
│  • Secrets (encrypted) • Settings               │
│  • API Keys            • Audit Trail            │
│  • SSH Host Keys       • Encryption Keys        │
│  • Webhooks            • Health Configs         │
└─────────────────────────────────────────────────┘
```

### Identifiers (XID)

All database tables use **XID** string primary keys instead of auto-increment integers.
XIDs are generated by the [`rs/xid`](https://github.com/rs/xid) library.

| Property | Value |
|---|---|
| **Format** | 20-character base32 string (`0-9`, `a-v`) |
| **Encoding** | 4-byte timestamp + 3-byte machine + 2-byte PID + 3-byte counter |
| **Sortable** | Lexicographic ordering = creation order |
| **Column type** | `TEXT PRIMARY KEY` (SQLite) |
| **Example** | `csfq6tl0e0qoefbg4110` |

**Benefits over auto-increment:**
- Portable across database instances (backup import/export matches by ID)
- No sequence conflicts during merge or restore
- URL-safe without encoding
- Globally unique without coordination
- Settings use stable hardcoded XIDs so defaults are idempotent across installations

### Key Management System (KMS)

Built-in encryption for secrets with key versioning:

- **Algorithm**: AES-256-GCM
- **Key Versioning**: Supports key rotation
- **Ciphertext Format**: `v1:{key_id}:{nonce}:{ciphertext}`
- **Backward Compatibility**: Old keys retained for decryption

### Scheduler

Background job scheduling for:
- Database backups
- Key rotation reminders
- Log cleanup
- Scheduled deployments
- Health check polling

### Alert Manager

System health monitoring and alerting:

| Alert Type | Trigger |
|------------|---------|
| `agent_disconnected` | Agent loses connection |
| `agent_reconnected` | Agent comes back online |
| `disk_warning` | Disk usage > 80% |
| `disk_critical` | Disk usage > 90% |
| `high_memory` | Memory usage > 85% |
| `high_cpu` | CPU usage > 90% |
| `deployment_stuck` | Deployment exceeds timeout |
| `high_error_rate` | Error rate threshold exceeded |

### Notification Manager

Multi-channel notification delivery:

| Channel | Configuration |
|---------|--------------|
| **Slack** | Webhook URL, channel, custom messages |
| **Discord** | Webhook URL, embeds with status colors |
| **Email** | SMTP configuration, HTML templates |
| **Webhook** | Custom HTTP endpoints with HMAC signing |

## Agents

Lightweight daemons running on deployment targets:

- **Connection**: Persistent gRPC stream to master
- **Execution**: Archive extraction, scripts, service restarts
- **Reporting**: Health metrics, deployment status
- **Self-Update**: Remote binary updates from master

### Agent Lifecycle

```
┌────────────┐    ┌────────────┐    ┌────────────┐    ┌────────────┐
│   Start    │───►│  Connect   │───►│  Register  │───►│   Ready    │
└────────────┘    └────────────┘    └────────────┘    └─────┬──────┘
                                                            │
                        ┌───────────────────────────────────┘
                        │
                        ▼
┌────────────────────────────────────────────────────────────────────┐
│                        Command Loop                                 │
│  ┌─────────┐   ┌─────────────┐   ┌───────────┐   ┌─────────────┐  │
│  │ Receive │──►│   Execute   │──►│  Report   │──►│  Heartbeat  │  │
│  │ Command │   │  (Deploy)   │   │  Status   │   │  (30s)      │  │
│  └─────────┘   └─────────────┘   └───────────┘   └─────────────┘  │
└────────────────────────────────────────────────────────────────────┘
```

## Communication Flows

### Deployment Flow

```
┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐
│ Trigger │───►│Validate │───►│Schedule │───►│ Execute │───►│ Notify  │
└─────────┘    └─────────┘    └─────────┘    └─────────┘    └─────────┘
     │              │              │              │              │
     │              │              │              │              │
  CLI/API/      Project        Queue for      Commands       Slack/
  Webhook       config &       target         via gRPC       Discord/
                secrets        dispatch       stream         Email
```

### Webhook Flow

```
┌────────────┐    ┌────────────┐    ┌────────────┐    ┌────────────┐
│    Git     │    │  Webhook   │    │  Project   │    │   Deploy   │
│  Provider  │───►│  Handler   │───►│   Match    │───►│  Trigger   │
└────────────┘    └────────────┘    └────────────┘    └────────────┘
    │                  │                  │                  │
  Push/             Validate           Find             Queue for
  Tag/PR            signature          matching          agents
                                       project
```

### Secret Resolution Flow

```
┌────────────┐    ┌────────────┐    ┌────────────┐    ┌────────────┐
│  Deploy    │    │   Lookup   │    │   KMS      │    │  Inject    │
│  Start     │───►│   Secret   │───►│  Decrypt   │───►│  Into Env  │
└────────────┘    └────────────┘    └────────────┘    └────────────┘
```

## Security Model

### Transport Security

| Connection | Security |
|------------|----------|
| REST API | HTTPS (TLS 1.3) |
| Web UI | HTTPS (TLS 1.3) |
| gRPC | mTLS (mutual TLS) |
| Webhooks | HMAC signature validation |

### Authentication

| Method | Use Case |
|--------|----------|
| Session Cookies | Web UI |
| JWT Bearer Tokens | API access |
| API Keys | Automation/CI |
| Agent Tokens | Agent registration |
| mTLS Certificates | Agent connections |

### Authorization (RBAC)

| Role | Permissions |
|------|-------------|
| `admin` | Full access, user management |
| `user` | Create/manage own projects |
| `viewer` | Read-only access |

### Secrets at Rest

- All secrets encrypted with AES-256-GCM
- Encryption keys stored separately from data
- Key rotation supported with version tracking

### Audit Trail

All operations logged with:
- User identity
- Action performed
- Resource affected
- Timestamp
- IP address

## Observability

### Tracing (OpenTelemetry)

Distributed tracing across:
- HTTP requests
- gRPC calls
- Database operations
- Deployment steps

### Metrics

Prometheus-compatible metrics:
- Request latency
- Deployment duration
- Agent connection status
- Error rates

### Logging

Structured logging (JSON) via Zap:
- Request IDs for correlation
- Log levels (debug, info, warn, error)
- Configurable output (file, stdout)

## Scalability Considerations

| Component | Scaling Strategy |
|-----------|-----------------|
| Master | Single instance (SQLite) |
| Agents | Horizontal (unlimited) |
| Database | SQLite (local) |
| Notifications | Async/parallel |

For high-availability requirements exceeding single-master capacity, consider deploying multiple vcdeploy instances with separate agent pools.

## Next Steps

- [Master Server](master.md) - Detailed master architecture
- [Agents](agents.md) - Agent architecture and lifecycle
- [gRPC Communication](grpc.md) - Protocol details
