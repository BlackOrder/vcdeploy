# gRPC API Reference

vcdeploy uses gRPC for communication between the master server and agents.

## Overview

| Property | Value |
|----------|-------|
| Protocol | gRPC over HTTP/2 |
| Port | 9001 (default) |
| TLS | mTLS in production |
| Serialization | Protocol Buffers v3 |
| Package | `vcdeploy.v1` |

## Service Definition

### AgentService

The single service for all agent-master communication:

```protobuf
service AgentService {
  // Register registers an agent with the master
  rpc Register(RegisterRequest) returns (RegisterResponse);
  
  // Connect establishes a bidirectional stream for commands and status
  rpc Connect(stream AgentMessage) returns (stream MasterMessage);
  
  // Heartbeat sends periodic health updates
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
}
```

## Registration

### RegisterRequest

Sent by agent to register with master:

```protobuf
message RegisterRequest {
  string agent_id = 1;           // Unique agent identifier
  string token = 2;              // Authentication token
  string hostname = 3;           // Agent hostname
  map<string, string> labels = 4; // Agent labels/tags
  AgentCapabilities capabilities = 5;
}

message AgentCapabilities {
  bool can_use_namespaces = 1;   // Container namespace support
  repeated string allowed_users = 2; // Users agent can switch to
  int64 disk_space_bytes = 3;    // Available disk space
  int64 memory_bytes = 4;        // Available memory
}
```

### RegisterResponse

Returned after successful registration:

```protobuf
message RegisterResponse {
  bool success = 1;
  string error = 2;              // Error message if success=false
  bytes certificate = 3;         // mTLS certificate for future connections
  bytes ca_certificate = 4;      // CA certificate for verification
}
```

## Heartbeat

### HeartbeatRequest

Sent periodically by agent:

```protobuf
message HeartbeatRequest {
  string agent_id = 1;
  int64 timestamp = 2;
  AgentStats stats = 3;
  repeated DeploymentStatus active_deployments = 4;
  string version = 5;            // Current agent version
  string os = 6;                 // Agent OS (linux, darwin, windows)
  string arch = 7;               // Agent architecture (amd64, arm64)
}

message AgentStats {
  double cpu_percent = 1;
  double memory_percent = 2;
  double disk_percent = 3;
  int64 disk_free_bytes = 4;
  int32 active_connections = 5;
}
```

### HeartbeatResponse

Acknowledges heartbeat, may include update notification:

```protobuf
message HeartbeatResponse {
  bool ok = 1;
  int64 server_timestamp = 2;
  UpdateNotification update_available = 3; // Present if agent should update
}

message UpdateNotification {
  string version = 1;            // New version to update to
  string download_url = 2;       // URL to download the binary
  string checksum_sha256 = 3;    // SHA256 checksum for verification
  int64 size_bytes = 4;          // Expected file size
  bool force = 5;                // Ignore update policy, update immediately
}
```

## Bidirectional Stream

The `Connect` RPC establishes a persistent bidirectional stream.

### AgentMessage (Agent → Master)

```protobuf
message AgentMessage {
  oneof message {
    DeploymentStatus deployment_status = 1;
    DeploymentLog deployment_log = 2;
    CommandResult command_result = 3;
    AgentReady agent_ready = 4;
    UpdateResult update_result = 5;
    HealthCheckResult health_check_result = 6;
  }
}
```

### MasterMessage (Master → Agent)

```protobuf
message MasterMessage {
  oneof message {
    DeployCommand deploy_command = 1;
    RollbackCommand rollback_command = 2;
    CancelCommand cancel_command = 3;
    HealthCheckCommand health_check_command = 4;
    UpdateCommand update_command = 5;
  }
}
```

## Deployment Messages

### DeployCommand

Instructs agent to deploy a project:

```protobuf
message DeployCommand {
  string deployment_id = 1;
  string project = 2;
  string target = 3;
  string repository = 4;
  string branch = 5;
  string commit = 6;
  string path = 7;
  DeploymentSettings settings = 8;
  map<string, string> env_vars = 9;
  bytes env_file_content = 10;
  repeated string pre_deploy_hooks = 11;
  repeated string post_deploy_hooks = 12;
  repeated ServiceReload reload_services = 13;
}

message DeploymentSettings {
  string strategy = 1;           // symlink | inplace
  int32 keep_releases = 2;
  repeated string shared_dirs = 3;
  repeated string shared_files = 4;
  repeated string writable_dirs = 5;
  string execution_user = 6;
  string execution_group = 7;
  int32 timeout_seconds = 8;
}

message ServiceReload {
  string service = 1;
  string action = 2;             // reload | restart | start | stop
}
```

### DeploymentStatus

Reports deployment progress:

```protobuf
message DeploymentStatus {
  string deployment_id = 1;
  DeploymentState state = 2;
  string message = 3;
  int64 timestamp = 4;
  int32 progress_percent = 5;
  string current_step = 6;
  int32 release_number = 7;
}

enum DeploymentState {
  DEPLOYMENT_STATE_UNSPECIFIED = 0;
  DEPLOYMENT_STATE_PENDING = 1;
  DEPLOYMENT_STATE_PREPARING = 2;
  DEPLOYMENT_STATE_CLONING = 3;
  DEPLOYMENT_STATE_BUILDING = 4;
  DEPLOYMENT_STATE_DEPLOYING = 5;
  DEPLOYMENT_STATE_VERIFYING = 6;
  DEPLOYMENT_STATE_COMPLETED = 7;
  DEPLOYMENT_STATE_FAILED = 8;
  DEPLOYMENT_STATE_CANCELLED = 9;
  DEPLOYMENT_STATE_ROLLING_BACK = 10;
}
```

### DeploymentLog

Streams log output from deployment:

```protobuf
message DeploymentLog {
  string deployment_id = 1;
  int64 timestamp = 2;
  LogLevel level = 3;
  string message = 4;
  string source = 5;             // hook name, git, etc.
}

enum LogLevel {
  LOG_LEVEL_UNSPECIFIED = 0;
  LOG_LEVEL_DEBUG = 1;
  LOG_LEVEL_INFO = 2;
  LOG_LEVEL_WARN = 3;
  LOG_LEVEL_ERROR = 4;
}
```

### CommandResult

Reports result of a command execution:

```protobuf
message CommandResult {
  string deployment_id = 1;
  string command = 2;
  int32 exit_code = 3;
  string stdout = 4;
  string stderr = 5;
  int64 duration_ms = 6;
}
```

## Rollback Messages

### RollbackCommand

Instructs agent to rollback to a previous release:

```protobuf
message RollbackCommand {
  string deployment_id = 1;
  string project = 2;
  string target = 3;
  string path = 4;
  int32 release_number = 5;      // 0 = previous release
  repeated string rollback_hooks = 6;
  repeated ServiceReload reload_services = 7;
}
```

## Health Check Messages

### HealthCheckCommand

Instructs agent to check health:

```protobuf
message HealthCheckCommand {
  string deployment_id = 1;
  string url = 2;
  string method = 3;             // HTTP method (GET, POST, etc.)
  int32 timeout_seconds = 4;
  int32 retries = 5;
  int32 retry_delay_seconds = 6;
  int32 expected_status = 7;     // Expected HTTP status code (default 200)
  map<string, string> headers = 8;
  string body = 9;               // Request body for POST
  string body_contains = 10;     // Response should contain this string
  bool trigger_rollback = 11;    // Whether to trigger rollback on failure
  int32 release_number = 12;     // Current release number (for rollback context)
}
```

### HealthCheckResult

Reports health check result:

```protobuf
message HealthCheckResult {
  string deployment_id = 1;
  bool success = 2;
  int32 status_code = 3;
  int64 response_time_ms = 4;
  string error = 5;
  int32 retry_count = 6;         // How many retries were attempted
  bool trigger_rollback = 7;     // Whether to trigger automatic rollback
}
```

## Update Messages

### UpdateCommand

Instructs agent to update itself:

```protobuf
message UpdateCommand {
  string version = 1;            // Version to update to
  string download_url = 2;       // URL to download the binary
  string checksum_sha256 = 3;    // SHA256 checksum for verification
  int64 size_bytes = 4;          // Expected file size
  bool force = 5;                // Restart even if health check fails
}
```

### UpdateResult

Reports result of an agent update:

```protobuf
message UpdateResult {
  string from_version = 1;
  string to_version = 2;
  bool success = 3;
  string error = 4;
  bool rolled_back = 5;          // True if update failed and agent rolled back
}
```

## Other Messages

### CancelCommand

Instructs agent to cancel an in-progress deployment:

```protobuf
message CancelCommand {
  string deployment_id = 1;
  string reason = 2;
}
```

### AgentReady

Signals agent is ready for commands:

```protobuf
message AgentReady {
  string agent_id = 1;
  int64 timestamp = 2;
}
```

## Connection Flow

```
Agent                                        Master
  │                                            │
  │──────────── Register() ────────────────────►│
  │◄─────────── RegisterResponse ──────────────│
  │                                            │
  │══════════════ Connect() ═══════════════════│ (bidirectional stream)
  │──────────── AgentReady ────────────────────►│
  │                                            │
  │──────────── Heartbeat() ───────────────────►│ (every 30s)
  │◄─────────── HeartbeatResponse ─────────────│
  │                                            │
  │◄─────────── DeployCommand ─────────────────│
  │──────────── DeploymentStatus ──────────────►│
  │──────────── DeploymentLog ─────────────────►│
  │──────────── DeploymentStatus (complete) ───►│
  │                                            │
  │◄─────────── HealthCheckCommand ────────────│
  │──────────── HealthCheckResult ─────────────►│
  │                                            │
```

## Configuration

### Master Configuration

```yaml
# master.yaml
grpc:
  address: ":9001"
  max_recv_msg_size: 16777216   # 16MB
  max_send_msg_size: 16777216   # 16MB
  keepalive_time: 30s
  keepalive_timeout: 10s
```

### Agent Configuration

```yaml
# agent.yaml
master:
  address: "master.example.com:9001"
  timeout: 30s
  insecure: false              # Allow insecure connection (development only)
```

## Authentication

### Token Authentication

```yaml
# agent.yaml
master:
  address: "master.example.com:9001"
  token: "agent-secret-token"
```

The agent sends the token in the `RegisterRequest.token` field.

### mTLS Authentication

After successful registration, the master returns certificates:

```go
// RegisterResponse fields:
// - certificate: Agent's client certificate
// - ca_certificate: CA certificate for verification
```

Subsequent connections use mutual TLS:

```yaml
# agent.yaml
master:
  address: "master.example.com:9001"
  tls:
    cert_file: /etc/vcdeploy/certs/agent.crt
    key_file: /etc/vcdeploy/certs/agent.key
    ca_file: /etc/vcdeploy/certs/ca.crt
```

## Error Handling

### gRPC Status Codes

| Code | Usage |
|------|-------|
| `OK` | Success |
| `INVALID_ARGUMENT` | Bad request parameters |
| `NOT_FOUND` | Agent/project not found |
| `UNAUTHENTICATED` | Invalid credentials |
| `PERMISSION_DENIED` | Authorization failure |
| `UNAVAILABLE` | Service temporarily unavailable |
| `DEADLINE_EXCEEDED` | Operation timeout |

### Error Response Pattern

```go
import "google.golang.org/grpc/status"

// Server returns error
return status.Errorf(codes.NotFound, "agent %s not found", agentID)

// Client handles error
if st, ok := status.FromError(err); ok {
    switch st.Code() {
    case codes.NotFound:
        // Handle not found
    case codes.Unavailable:
        // Retry with backoff
    }
}
```

## Reconnection

On disconnection, agents automatically reconnect:

1. Initial retry: 1 second
2. Exponential backoff: 2x each attempt
3. Maximum delay: 5 minutes
4. Jitter: ±10% to prevent thundering herd

## Debugging

### grpcurl

```bash
# List services (requires reflection enabled)
grpcurl -plaintext localhost:9001 list

# Describe service
grpcurl -plaintext localhost:9001 describe vcdeploy.v1.AgentService

# Call Register
grpcurl -plaintext -d '{
  "agent_id": "test-001",
  "token": "secret",
  "hostname": "test-host"
}' localhost:9001 vcdeploy.v1.AgentService/Register
```

### Enable Reflection (Development)

```go
import "google.golang.org/grpc/reflection"

// In server setup
reflection.Register(grpcServer)
```

## Performance

### Keepalive Settings

```go
// Client
grpc.WithKeepaliveParams(keepalive.ClientParameters{
    Time:                30 * time.Second,
    Timeout:             10 * time.Second,
    PermitWithoutStream: true,
})

// Server
grpc.KeepaliveParams(keepalive.ServerParameters{
    Time:    30 * time.Second,
    Timeout: 10 * time.Second,
})
```

### Message Size Limits

Default: 4MB. Increase for large deployments:

```yaml
grpc:
  max_recv_msg_size: 16777216  # 16MB
  max_send_msg_size: 16777216  # 16MB
```

## Related Documentation

- [Architecture Overview](architecture/overview.md) - System architecture
- [Agent Architecture](architecture/agents.md) - Agent details
- [Agent Configuration](config/agent.md) - Agent setup
