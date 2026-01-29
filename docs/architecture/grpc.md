# gRPC Communication

vcdeploy uses gRPC for all master-agent communication, providing:
- Efficient binary protocol
- Bidirectional streaming
- Strong typing via Protocol Buffers
- Built-in TLS support

## Protocol Overview

```
┌────────────┐                          ┌────────────┐
│   Master   │                          │   Agent    │
│            │                          │            │
│  ┌──────┐  │    AgentService.Connect  │  ┌──────┐  │
│  │ gRPC │◄─┼──────────────────────────┼──│ gRPC │  │
│  │Server│  │                          │  │Client│  │
│  │      │──┼──────────────────────────┼─►│      │  │
│  └──────┘  │    Heartbeat/Commands    │  └──────┘  │
│            │                          │            │
└────────────┘                          └────────────┘
```

## Services

### AgentService

The main service for agent communication:

```protobuf
service AgentService {
  // Bidirectional stream for agent connection
  rpc Connect(stream AgentMessage) returns (stream MasterMessage);
  
  // Heartbeat for health monitoring
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
}
```

### Message Types

**Agent → Master:**
- `Register`: Initial connection with agent info
- `Heartbeat`: Periodic health update
- `DeploymentStatus`: Progress updates
- `LogEntry`: Deployment logs

**Master → Agent:**
- `Acknowledged`: Registration confirmation
- `DeployCommand`: Start deployment
- `CancelCommand`: Abort deployment
- `ConfigUpdate`: Configuration changes

## Authentication

Agents authenticate using one of:

1. **Token-based**: Pre-shared token in configuration
2. **mTLS**: Mutual TLS with client certificates

### Token Authentication

```yaml
# Agent config
master_address: "master.example.com:50051"
token: "agent-secret-token"
```

### mTLS Authentication

```yaml
# Agent config
master_address: "master.example.com:50051"
tls:
  cert_file: "/etc/vcdeploy/agent.crt"
  key_file: "/etc/vcdeploy/agent.key"
  ca_file: "/etc/vcdeploy/ca.crt"
```

## Connection Lifecycle

```
Agent                                Master
  │                                    │
  │─────── Connect() ─────────────────►│
  │                                    │
  │◄────── Acknowledged ───────────────│
  │                                    │
  │─────── Heartbeat (every 30s) ─────►│
  │◄────── HeartbeatResponse ──────────│
  │                                    │
  │◄────── DeployCommand ──────────────│
  │─────── DeploymentStatus ──────────►│
  │─────── DeploymentStatus ──────────►│
  │─────── DeploymentComplete ────────►│
  │                                    │
  │         ... continues ...          │
```

## Reconnection

On disconnection, agents automatically reconnect:

1. Initial retry: 1 second
2. Exponential backoff: 2x each attempt
3. Maximum delay: 5 minutes
4. Jitter: ±10% to prevent thundering herd

## Timeouts

| Operation | Timeout |
|-----------|---------|
| Connect | 30 seconds |
| Heartbeat interval | 30 seconds |
| Heartbeat timeout | 90 seconds |
| Deploy command | configurable |

## Error Handling

gRPC status codes used:

| Code | Meaning |
|------|---------|
| OK | Success |
| UNAUTHENTICATED | Invalid credentials |
| PERMISSION_DENIED | Not authorized |
| UNAVAILABLE | Service temporarily unavailable |
| DEADLINE_EXCEEDED | Operation timeout |

## Observability

All gRPC operations emit:
- OpenTelemetry traces with context propagation
- Prometheus metrics for latency and errors
- Structured logs with request IDs
