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
│  │  • Git ops  │  • Symlink mgmt      │  • CPU/Memory      │  │
│  │  • Scripts  │  • Directory ops     │  • Disk usage      │  │
│  │  • Commands │  • Permissions       │  • Process info    │  │
│  └─────────────┴───────────────────────┴────────────────────┘  │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    Local Storage                          │  │
│  │           • Releases    • Shared files   • Logs          │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Deployment Process

When an agent receives a deployment command:

1. **Prepare**: Create release directory
2. **Fetch**: Clone repository or pull updates
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
