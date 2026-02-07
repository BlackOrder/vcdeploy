# Agent Architecture

Agents are lightweight daemons that run on deployment targets, executing commands on behalf of the master server.

## Responsibilities

- Maintain persistent connection to master
- Execute deployment commands
- Report system metrics and health
- Manage local file operations

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                          Agent                                   │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │               gRPC Client (Bidirectional Stream)          │  │
│  │           • Heartbeats    • Command Reception             │  │
│  │           • Log Shipping  • Status Updates                │  │
│  └─────────────────────────┬────────────────────────────────┘  │
│                            │                                    │
│  ┌─────────────┬───────────┴───────────┬────────────────────┐  │
│  │  Executor   │   File Manager        │  Metrics Collector │  │
│  │             │                       │                    │  │
│  │  • Archive  │  • Symlink mgmt      │  • CPU/Memory      │  │
│  │    extract │  • Directory ops     │  • Disk usage      │  │
│  │  • Scripts  │  • Permissions       │  • Process info    │  │
│  │  • Commands │                       │                    │  │
│  └─────────────┴───────────────────────┴────────────────────┘  │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    Local Storage                          │  │
│  │           • Releases    • Shared files   • Logs          │  │
│  │           • Archive Cache                              │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Archive Caching

Agents cache deployment archives locally for fast rollback:

- Archives are stored by commit hash in the cache directory
- Configurable `keep_count` limits per-project cache size
- On rollback, if the archive is cached, no download is needed
- Cache is checked before requesting archive from master

```yaml
archive:
  cache_dir: /var/lib/vcdeploy/cache/archives
  keep_count: 5    # Max cached archives per project
```

## Archive Delivery

Deployment archives are delivered from master to agent via two secure channels:

1. **gRPC Streaming (primary)** — Archive streamed over the mTLS-authenticated gRPC connection via `StreamRepoArchive` RPC
2. **HMAC-Signed HTTP (fallback)** — If gRPC streaming fails, agent downloads via time-limited signed URL provided in `DeployCommand.archive_download_url`

Signed URLs use HMAC-SHA256 with the agent's authentication secret. Default expiry: 10 minutes.

## Deployment Process

When an agent receives a deployment command:

1. **Prepare**: Create release directory
2. **Extract**: Download and extract deployment archive
3. **Build**: Run build commands (if configured)
4. **Link**: Atomically update symlink to new release
5. **Cleanup**: Remove old releases (keep configurable number)
6. **Report**: Send completion status to master

## Symlink Strategy

```
/var/www/myapp/
├── current -> releases/20240115-120000/    # Atomic symlink
├── releases/
│   ├── 20240115-120000/                    # Latest release
│   ├── 20240114-093000/                    # Previous release
│   └── 20240113-150000/                    # Older release
└── shared/
    ├── logs/                               # Persistent logs
    ├── uploads/                            # User uploads
    └── .env                                # Environment file
```

## Connection Management

Agents maintain a persistent gRPC connection with automatic:
- Reconnection with exponential backoff
- Heartbeat monitoring
- TLS certificate validation

## Security

- Agent authentication via token or mTLS
- Sandboxed command execution
- File system restrictions
- No inbound network ports required
- Agent does NOT need git credentials — archives are received from master

## Init System Integration

Agents integrate with system init systems:

| Platform | Init System | Service File |
|----------|-------------|--------------|
| Linux | systemd | `/lib/systemd/system/vcdeploy-agent.service` |
| macOS | launchd | `/Library/LaunchDaemons/com.vcdeploy.agent.plist` |
| Linux (Alpine) | OpenRC | `/etc/init.d/vcdeploy-agent` |
| FreeBSD | rc.d | `/usr/local/etc/rc.d/vcdeploy_agent` |

## Configuration

See [Agent Configuration](config/agent.md) for detailed configuration options.

## Resource Requirements

Agents are designed to be lightweight:

| Metric | Value |
|--------|-------|
| Binary size | ~15MB |
| Memory (idle) | ~20MB |
| Memory (deploying) | ~50MB |
| CPU (idle) | <1% |

## Agent Self-Update

Agents can be updated remotely from the master server without manual intervention.

### Update Policies

Configure how agents receive updates:

| Policy | Behavior |
|--------|----------|
| `immediate` | Update as soon as new version is available |
| `window` | Update only during maintenance window |
| `manual` | Require manual approval for each update |

### Configuration

```yaml
# Agent configuration
update:
  policy: window
  window:
    start: "02:00"
    end: "04:00"
  auto_rollback: true
```

### Update Process

1. **Check**: Master detects agent version mismatch
2. **Download**: Agent downloads new binary from master
3. **Verify**: Binary signature is validated
4. **Replace**: New binary replaces old (with backup)
5. **Restart**: Agent service restarts automatically
6. **Verify**: Master confirms successful update

### Rollback

If an update fails or the agent becomes unhealthy after update:

```bash
# View update history
vcdeploy agent updates --agent-id agent-001

# Rollback to previous version
vcdeploy agent rollback --agent-id agent-001
```

### API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/agents/{id}/updates` | Get update history |
| POST | `/api/v1/agents/{id}/update` | Trigger update |
| POST | `/api/v1/agents/{id}/rollback` | Rollback update |
