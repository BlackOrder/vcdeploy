# Master Server Architecture

The master server is the central control plane for vcdeploy, orchestrating deployments and managing all system configuration.

## Core Responsibilities

- **Deployment Orchestration**: Coordinating multi-agent deployments
- **Configuration Management**: Projects, secrets, settings
- **User Management**: Authentication, authorization, sessions
- **Agent Management**: Registration, health monitoring, commands
- **Webhook Processing**: Git provider event handling
- **Notification Dispatch**: Multi-channel alerting

## Component Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Master Server                                     │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                        Entry Points                                     │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌────────────┐ │ │
│  │  │   REST API   │  │   Web UI     │  │   gRPC API   │  │  Webhooks  │ │ │
│  │  │   :9000      │  │   :9000      │  │   :9001      │  │  :9000     │ │ │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └─────┬──────┘ │ │
│  └─────────┼─────────────────┼─────────────────┼────────────────┼────────┘ │
│            │                 │                 │                │          │
│  ┌─────────▼─────────────────▼─────────────────┴────────────────▼────────┐ │
│  │                     Security Middleware Stack                          │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │ │
│  │  │    Auth     │  │    RBAC     │  │     CSP     │  │ Rate Limit  │  │ │
│  │  │ JWT/Session │  │ Enforcement │  │  Middleware │  │             │  │ │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘  │ │
│  └────────────────────────────────┬──────────────────────────────────────┘ │
│                                   │                                        │
│  ┌────────────────────────────────▼──────────────────────────────────────┐ │
│  │                         Service Layer                                  │ │
│  │  ┌───────────┐  ┌─────────────┐  ┌────────────┐  ┌─────────────────┐ │ │
│  │  │  Project  │  │ Deployment  │  │   Secret   │  │      User       │ │ │
│  │  │  Service  │  │   Service   │  │  Service   │  │    Service      │ │ │
│  │  └───────────┘  └─────────────┘  └────────────┘  └─────────────────┘ │ │
│  │  ┌───────────┐  ┌─────────────┐  ┌────────────┐  ┌─────────────────┐ │ │
│  │  │   Agent   │  │   API Key   │  │  Session   │  │    Webhook      │ │ │
│  │  │  Service  │  │   Service   │  │  Service   │  │    Service      │ │ │
│  │  └───────────┘  └─────────────┘  └────────────┘  └─────────────────┘ │ │
│  │  ┌───────────┐  ┌─────────────┐  ┌────────────┐  ┌─────────────────┐ │ │
│  │  │   Audit   │  │  Host Key   │  │ Provision  │  │  Project Type   │ │ │
│  │  │  Service  │  │   Service   │  │  Service   │  │    Service      │ │ │
│  │  └───────────┘  └─────────────┘  └────────────┘  └─────────────────┘ │ │
│  │  ┌───────────┐  ┌─────────────┐                                     │ │
│  │  │  Target  │  │  Archive    │                                     │ │
│  │  │  Service │  │   Service   │                                     │ │
│  │  └───────────┘  └─────────────┘                                     │ │
│  └────────────────────────────────┬──────────────────────────────────────┘ │
│                                   │                                        │
│  ┌────────┬───────────────────────┼─────────────────┬──────────┬────────┐ │
│  │        │                       │                 │          │        │ │
│  │  ┌─────▼─────┐          ┌─────▼─────┐     ┌─────▼────┐ ┌───▼────┐  │ │
│  │  │  SQLite   │          │    KMS    │     │Scheduler │ │ Alert  │  │ │
│  │  │    DB     │          │           │     │          │ │Manager │  │ │
│  │  └───────────┘          └───────────┘     └──────────┘ └────────┘  │ │
│  │                                                                     │ │
│  │  ┌───────────┐          ┌───────────┐     ┌──────────┐             │ │
│  │  │ Notifier  │          │  Tracing  │     │  CA Mgr  │             │ │
│  │  │  Manager  │          │  (OTel)   │     │  (mTLS)  │             │ │
│  │  └───────────┘          └───────────┘     └──────────┘             │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────┘
```

## Entry Points

### REST API (Port 9000)

HTTP/JSON API for external integrations:

```go
// Route registration in internal/server/master.go
mux.HandleFunc("/api/v1/projects", s.withAuth(s.handleProjectsAPI))
mux.HandleFunc("/api/v1/projects/", s.withAuth(s.handleProjectAPI))
mux.HandleFunc("/api/v1/agents", s.withAuth(s.handleAgentsAPI))
mux.HandleFunc("/api/v1/users", s.withAuth(s.handleUsersAPI))
mux.HandleFunc("/api/v1/secrets", s.withAuth(s.handleSecretsAPI))
// ... more routes
```

### Web UI (Port 9000)

HTML templates with HTMX for dynamic updates:
- Dashboard with deployment overview
- Project management interface
- Agent status monitoring
- Settings and configuration

### gRPC API (Port 9001)

Bidirectional streaming for agent communication:

```protobuf
service AgentService {
  rpc Connect(stream AgentMessage) returns (stream MasterMessage);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
}
```

### Webhooks (Port 9000)

Git provider callbacks:
- GitHub: `/webhook/github/{project}`
- GitLab: `/webhook/gitlab/{project}`
- Bitbucket: `/webhook/bitbucket/{project}`

## Security Middleware

### Authentication Middleware

```go
type SecurityMiddleware struct {
    store   storage.Store
    logger  *zap.Logger
}

// Validates JWT tokens or session cookies
func (m *SecurityMiddleware) withAuth(next http.Handler) http.Handler
```

Supports:
- JWT Bearer tokens (API access)
- Session cookies (Web UI)
- API keys (automation)

### RBAC Enforcement Middleware

```go
type EnforcementMiddleware struct {
    // Checks user roles and API key scopes
}

func (m *EnforcementMiddleware) CheckWriteAccess(ctx context.Context) (string, int, bool)
func (m *EnforcementMiddleware) CheckAdminAccess(ctx context.Context) (string, int, bool)
```

Role hierarchy:
1. `admin` - Full system access
2. `user` - Project management
3. `viewer` - Read-only access

### CSP Middleware

Content Security Policy headers for Web UI:

```go
type CSPMiddleware struct {
    // Sets security headers
}

// Adds Content-Security-Policy, X-Frame-Options, etc.
func (m *CSPMiddleware) wrap(next http.Handler) http.Handler
```

### Rate Limiter

Request throttling:

```go
type RateLimiter struct {
    limits    RateLimitConfig
    // Per-user/IP request tracking
}

func (r *RateLimiter) wrap(next http.Handler) http.Handler
```

## Service Layer

Each service encapsulates business logic for a domain:

### ProjectService

```go
type ProjectService interface {
    Create(ctx context.Context, name, repo, branch, path, typ string) (*Project, error)
    List(ctx context.Context) ([]*Project, error)
    Get(ctx context.Context, name string) (*Project, error)
    Update(ctx context.Context, project *Project) error
    Delete(ctx context.Context, name string) error
}
```

### DeploymentService

```go
type DeploymentService interface {
    Trigger(ctx context.Context, project, target, branch string) (*Deployment, error)
    GetStatus(ctx context.Context, deployID string) (*DeploymentStatus, error)
    List(ctx context.Context, projectName string) ([]*Deployment, error)
    Cancel(ctx context.Context, deployID string) error
}
```

### SecretService

```go
type SecretService interface {
    Set(ctx context.Context, project, scope, key, value string) error
    Get(ctx context.Context, project, scope, key string) (string, error)
    List(ctx context.Context, project string) ([]*SecretInfo, error)
    Delete(ctx context.Context, project, scope, key string) error
}
```

### AgentService

```go
type AgentService interface {
    Register(ctx context.Context, agent *Agent) error
    List(ctx context.Context) ([]*Agent, error)
    Get(ctx context.Context, id string) (*Agent, error)
    UpdateStatus(ctx context.Context, id, status string) error
    Delete(ctx context.Context, id string) error
}
```

## Infrastructure Components

### SQLite Database

Single-file database with all system data:

```sql
-- Core tables
users, sessions, api_keys
projects, project_types, project_webhooks
agents, targets, deployment_targets
deployments, deployment_logs, deployment_env_snapshots
secrets, settings, audit_log
encryption_keys, encryption_key_usage
ssh_host_keys, ssh_jump_servers
health_check_configs, archive_cache
```

### Key Management System (KMS)

Encryption key lifecycle management:

```go
type KMS struct {
    db         *sql.DB
    currentKey *EncryptionKey
    cache      map[string]*EncryptionKey
}

func (k *KMS) Encrypt(ctx context.Context, plaintext []byte) (string, error)
func (k *KMS) Decrypt(ctx context.Context, ciphertext string) ([]byte, error)
func (k *KMS) RotateKey(ctx context.Context) (*EncryptionKey, error)
```

Key states: `pending` → `active` → `inactive` → `scheduled` → `deleted`

### Scheduler

Background job execution:

```go
type Scheduler struct {
    jobs []ScheduledJob
}

// Jobs include:
// - Database backups
// - Log cleanup
// - Key rotation
// - Health checks
```

### Alert Manager

System health monitoring:

```go
type Manager struct {
    notifier   *notify.Manager
    thresholds Thresholds
}

// Alert types:
// - agent_disconnected, agent_reconnected
// - disk_warning, disk_critical
// - high_memory, high_cpu
// - deployment_stuck
// - high_error_rate
```

### Notification Manager

Multi-channel notification dispatch:

```go
type Manager struct {
    notifiers []Notifier
}

// Registered notifiers:
// - SlackNotifier
// - DiscordNotifier
// - EmailNotifier
// - WebhookNotifier
```

### CA Manager

Certificate authority for agent mTLS:

```go
type CAManager struct {
    // Issues and validates agent certificates
}

func (ca *CAManager) IssueCertificate(agentID string) (*tls.Certificate, error)
func (ca *CAManager) ValidateCertificate(cert *x509.Certificate) error
```

## Deployment Workflow

### 1. Trigger

Deployment initiated via:
- CLI: `vcdeploy deploy create --project myapp --target production`
- API: `POST /api/v1/deployments`
- Webhook: Push event from Git provider
- Web UI: Deploy button

### 2. Validation

```go
func (s *MasterServer) validateDeployment(project *Project, target string) error {
    // Check project configuration
    // Resolve targets and verify agents are connected
    // Validate secrets are available
    // Check deployment locks
    // Create env snapshot (capture secrets at deployment time)
}
```

### 3. Scheduling

```go
func (s *MasterServer) scheduleDeployment(deploy *Deployment) error {
    // Create deployment record
    // Resolve deployment targets
    // Create archive from repository
    // Queue commands for each target's agent
}
```

### 4. Execution

Commands sent via gRPC stream:

```protobuf
message DeployCommand {
    string deployment_id = 1;
    string project = 2;
    string repository = 3;
    string branch = 4;
    string path = 5;
    repeated string pre_deploy = 6;
    repeated string post_deploy = 7;
}
```

### 5. Monitoring

Agent status updates streamed back:

```protobuf
message DeploymentStatus {
    string deployment_id = 1;
    string status = 2;  // pending, running, success, failed
    string message = 3;
    int32 progress = 4;
}
```

### 6. Completion

```go
func (s *MasterServer) completeDeployment(deploy *Deployment, status string) {
    // Update deployment record
    // Run health checks (if configured)
    // Send notifications
    // Log audit entry
}
```

## High Availability Considerations

### Graceful Shutdown

```go
func (s *MasterServer) Shutdown(ctx context.Context) error {
    // Stop accepting new requests
    // Wait for in-flight requests
    // Close agent connections
    // Flush notifications
    // Close database
}
```

### Database Migrations

Automatic schema updates on startup:

```go
func (s *MasterServer) runMigrations() error {
    // Check current schema version
    // Apply pending migrations
    // Update version
}
```

### Configuration Hot-Reload

Certain settings can be updated without restart:
- Rate limits
- Notification channels
- Log levels

## Resource Requirements

| Deployment Size | Agents | CPU | Memory | Disk |
|----------------|--------|-----|--------|------|
| Small | <10 | 1 core | 512MB | 1GB |
| Medium | 10-50 | 2 cores | 1GB | 5GB |
| Large | 50-100 | 4 cores | 2GB | 10GB |

## Configuration

Master server is configured via `/etc/vcdeploy/master.yaml`:

```yaml
server:
  address: ":9000"

grpc:
  address: ":9001"
  
security:
  session_secret: "..."
  
# See config/master.md for full reference
```

## Related Documentation

- [Architecture Overview](architecture/overview.md) - System-wide architecture
- [Agent Architecture](architecture/agents.md) - Agent details
- [Master Configuration](config/master.md) - Configuration reference
- [API Reference](api/README.md) - REST API documentation
