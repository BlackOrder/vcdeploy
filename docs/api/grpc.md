# gRPC API Reference

vcdeploy uses gRPC for communication between the master server and agents.

## Overview

| Property | Value |
|----------|-------|
| Protocol | gRPC over HTTP/2 |
| Port | 9090 (default) |
| TLS | Required in production |
| Serialization | Protocol Buffers |

## Service Definitions

### Agent Service

```protobuf
service AgentService {
  // Agent registration and heartbeat
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  
  // Command execution
  rpc Execute(stream ExecuteRequest) returns (stream ExecuteResponse);
  
  // File operations
  rpc UploadFile(stream FileChunk) returns (UploadResponse);
  rpc DownloadFile(DownloadRequest) returns (stream FileChunk);
}
```

### Deploy Service

```protobuf
service DeployService {
  // Deployment operations
  rpc Deploy(DeployRequest) returns (stream DeployStatus);
  rpc Rollback(RollbackRequest) returns (stream DeployStatus);
  rpc Cancel(CancelRequest) returns (CancelResponse);
  
  // Status queries
  rpc GetStatus(StatusRequest) returns (StatusResponse);
}
```

## Messages

### RegisterRequest

```protobuf
message RegisterRequest {
  string agent_id = 1;
  string hostname = 2;
  string version = 3;
  map<string, string> labels = 4;
  repeated string capabilities = 5;
}
```

### ExecuteRequest

```protobuf
message ExecuteRequest {
  string task_id = 1;
  string command = 2;
  string working_dir = 3;
  map<string, string> environment = 4;
  int32 timeout_seconds = 5;
}
```

### ExecuteResponse

```protobuf
message ExecuteResponse {
  string task_id = 1;
  enum Status {
    RUNNING = 0;
    COMPLETED = 1;
    FAILED = 2;
    TIMEOUT = 3;
  }
  Status status = 2;
  int32 exit_code = 3;
  bytes stdout = 4;
  bytes stderr = 5;
}
```

### DeployStatus

```protobuf
message DeployStatus {
  string deployment_id = 1;
  string phase = 2;  // "preparing", "transferring", "executing", "verifying", "complete"
  float progress = 3;  // 0.0 - 1.0
  string message = 4;
  google.protobuf.Timestamp timestamp = 5;
}
```

## Connection Management

### Client Configuration

```go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
)

// Load TLS credentials
creds, err := credentials.NewClientTLSFromFile(certFile, "")

// Create connection
conn, err := grpc.Dial(
    "master:9090",
    grpc.WithTransportCredentials(creds),
    grpc.WithKeepaliveParams(keepalive.ClientParameters{
        Time:                10 * time.Second,
        Timeout:             5 * time.Second,
        PermitWithoutStream: true,
    }),
)
```

### Server Configuration

```yaml
# master.yaml
grpc:
  port: 9090
  
  # TLS Configuration
  tls:
    enabled: true
    cert_file: /etc/vcdeploy/certs/server.crt
    key_file: /etc/vcdeploy/certs/server.key
    ca_file: /etc/vcdeploy/certs/ca.crt
    
  # Connection limits
  max_connections: 1000
  max_concurrent_streams: 100
  
  # Keepalive
  keepalive:
    time: 30s
    timeout: 10s
    min_time: 10s
```

## Authentication

### mTLS (Mutual TLS)

Both client and server verify certificates:

```yaml
# Agent configuration
master:
  address: master.example.com:9090
  tls:
    cert_file: /etc/vcdeploy/certs/agent.crt
    key_file: /etc/vcdeploy/certs/agent.key
    ca_file: /etc/vcdeploy/certs/ca.crt
```

### Token Authentication

Alternative to mTLS:

```go
// Agent sends token in metadata
md := metadata.Pairs("authorization", "Bearer "+token)
ctx := metadata.NewOutgoingContext(ctx, md)
```

## Streaming Patterns

### Server Streaming (Deployment Status)

```go
// Server
func (s *Server) Deploy(req *DeployRequest, stream DeployService_DeployServer) error {
    for status := range deployment.StatusChannel() {
        if err := stream.Send(status); err != nil {
            return err
        }
    }
    return nil
}

// Client
stream, err := client.Deploy(ctx, request)
for {
    status, err := stream.Recv()
    if err == io.EOF {
        break
    }
    // Process status update
}
```

### Bidirectional Streaming (Command Execution)

```go
// Server
func (s *Server) Execute(stream AgentService_ExecuteServer) error {
    for {
        req, err := stream.Recv()
        if err == io.EOF {
            return nil
        }
        // Execute command and send response
        stream.Send(&ExecuteResponse{...})
    }
}
```

## Error Handling

### gRPC Status Codes

| Code | Usage |
|------|-------|
| `OK` | Success |
| `INVALID_ARGUMENT` | Bad request |
| `NOT_FOUND` | Resource not found |
| `PERMISSION_DENIED` | Auth failure |
| `UNAVAILABLE` | Service down |
| `DEADLINE_EXCEEDED` | Timeout |

### Error Response

```go
import "google.golang.org/grpc/status"

// Return error with status
return status.Errorf(codes.NotFound, "agent %s not found", agentID)

// Client handling
if st, ok := status.FromError(err); ok {
    switch st.Code() {
    case codes.NotFound:
        // Handle not found
    case codes.Unavailable:
        // Retry
    }
}
```

## Interceptors

### Server Interceptors

```go
server := grpc.NewServer(
    grpc.ChainUnaryInterceptor(
        logging.UnaryServerInterceptor(),
        auth.UnaryServerInterceptor(),
        metrics.UnaryServerInterceptor(),
        recovery.UnaryServerInterceptor(),
    ),
    grpc.ChainStreamInterceptor(
        logging.StreamServerInterceptor(),
        auth.StreamServerInterceptor(),
    ),
)
```

## Debugging

### grpcurl

```bash
# List services
grpcurl -plaintext localhost:9090 list

# Describe service
grpcurl -plaintext localhost:9090 describe AgentService

# Call method
grpcurl -plaintext -d '{"agent_id": "test"}' \
  localhost:9090 vcdeploy.AgentService/Register
```

### Reflection

Enable for development:
```go
import "google.golang.org/grpc/reflection"

reflection.Register(server)
```

### Logging

```yaml
# Enable gRPC debug logging
grpc:
  debug: true
  verbosity: 2
```

## Performance Tuning

### Connection Pooling

```go
// Client-side connection pool
pool := grpcpool.New(func() (*grpc.ClientConn, error) {
    return grpc.Dial(address, opts...)
}, 10, 100, time.Hour)
```

### Compression

```go
// Enable compression
conn, _ := grpc.Dial(address,
    grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)),
)
```
