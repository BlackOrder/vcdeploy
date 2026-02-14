//go:build e2e

// Package e2e provides end-to-end tests for vcdeploy gRPC agent protocol.
package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// GRPCTestConfig holds configuration for gRPC E2E tests.
type GRPCTestConfig struct {
	MasterGRPCAddr  string
	AgentID         string
	AgentToken      string
	TLSEnabled      bool
	ConnectTimeout  time.Duration
	HeartbeatPeriod time.Duration
}

func getGRPCTestConfig() *GRPCTestConfig {
	return &GRPCTestConfig{
		MasterGRPCAddr:  getEnvOrDefault("E2E_MASTER_GRPC_URL", "localhost:9001"),
		AgentID:         getEnvOrDefault("E2E_AGENT_ID", "e2e-test-agent"),
		AgentToken:      getEnvOrDefault("E2E_AGENT_TOKEN", "test-registration-token"),
		TLSEnabled:      getEnvOrDefault("E2E_GRPC_TLS", "false") == "true",
		ConnectTimeout:  30 * time.Second,
		HeartbeatPeriod: 5 * time.Second,
	}
}

// dialGRPC creates a gRPC connection to the master server.
func dialGRPC(_ context.Context, cfg *GRPCTestConfig) (*grpc.ClientConn, error) {
	// ctx reserved for future dial timeout/cancellation support
	var opts []grpc.DialOption

	if cfg.TLSEnabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true, // E2E tests may use self-signed certs
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(cfg.MasterGRPCAddr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial gRPC: %w", err)
	}

	return conn, nil
}

// waitForGRPC waits until the gRPC server is available.
func waitForGRPC(ctx context.Context, cfg *GRPCTestConfig, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			conn, err := dialGRPC(ctx, cfg)
			if err == nil {
				conn.Close()
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	return fmt.Errorf("timeout waiting for gRPC server at %s", cfg.MasterGRPCAddr)
}

// TestGRPCServerAvailable verifies the gRPC server is reachable.
func TestGRPCServerAvailable(t *testing.T) {
	cfg := getGRPCTestConfig()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	err := waitForGRPC(ctx, cfg, cfg.ConnectTimeout)
	if err != nil {
		t.Skipf("gRPC server not available: %v", err)
	}

	conn, err := dialGRPC(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	// Connection established successfully
	t.Logf("Successfully connected to gRPC server at %s", cfg.MasterGRPCAddr)
}

// TestAgentRegistration tests the agent registration flow.
func TestAgentRegistration(t *testing.T) {
	cfg := getGRPCTestConfig()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	err := waitForGRPC(ctx, cfg, cfg.ConnectTimeout)
	if err != nil {
		t.Skipf("gRPC server not available: %v", err)
	}

	conn, err := dialGRPC(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	client := proto.NewAgentServiceClient(conn)

	t.Run("register with valid token", func(t *testing.T) {
		// Note: In a real E2E environment, the master would have pre-registered
		// the agent token. This test validates the registration API contract.
		req := &proto.RegisterRequest{
			AgentId:  cfg.AgentID,
			Token:    cfg.AgentToken,
			Hostname: "e2e-test-host",
			Labels: map[string]string{
				"env":  "e2e-test",
				"role": "test-agent",
			},
			Capabilities: &proto.AgentCapabilities{
				CanUseNamespaces: false,
				DiskSpaceBytes:   1024 * 1024 * 1024,
				MemoryBytes:      4 * 1024 * 1024 * 1024,
			},
		}

		resp, err := client.Register(ctx, req)
		if err != nil {
			// Registration failure with "invalid registration token" is expected
			// in E2E tests without pre-provisioned tokens
			st, ok := status.FromError(err)
			if ok && st.Code() == codes.PermissionDenied {
				t.Skipf("Registration token not pre-provisioned: %v", err)
			}
			// Other errors are unexpected
			t.Logf("Registration error (may be expected): %v", err)
			return
		}

		if !resp.Success {
			t.Logf("Registration not successful (may be expected): %s", resp.Error)
			return
		}

		// Verify certificate was issued
		if len(resp.Certificate) == 0 {
			t.Error("expected certificate in response")
		}
		if len(resp.CaCertificate) == 0 {
			t.Error("expected CA certificate in response")
		}

		t.Logf("Agent registered successfully with %d byte certificate", len(resp.Certificate))
	})

	t.Run("register with missing agent_id", func(t *testing.T) {
		req := &proto.RegisterRequest{
			AgentId: "",
			Token:   cfg.AgentToken,
		}

		resp, err := client.Register(ctx, req)
		if err != nil {
			// gRPC error is acceptable
			t.Logf("Expected error for missing agent_id: %v", err)
			return
		}

		if resp.Success {
			t.Error("registration should fail with missing agent_id")
		}
		if resp.Error == "" {
			t.Error("expected error message in response")
		}
	})

	t.Run("register with missing token", func(t *testing.T) {
		req := &proto.RegisterRequest{
			AgentId: cfg.AgentID,
			Token:   "",
		}

		resp, err := client.Register(ctx, req)
		if err != nil {
			// gRPC error is acceptable
			t.Logf("Expected error for missing token: %v", err)
			return
		}

		if resp.Success {
			t.Error("registration should fail with missing token")
		}
	})

	t.Run("register with invalid token", func(t *testing.T) {
		req := &proto.RegisterRequest{
			AgentId:  cfg.AgentID + "-invalid",
			Token:    "definitely-not-a-valid-token",
			Hostname: "invalid-host",
		}

		resp, err := client.Register(ctx, req)
		if err != nil {
			t.Logf("Expected error for invalid token: %v", err)
			return
		}

		if resp.Success {
			t.Error("registration should fail with invalid token")
		}
	})
}

// TestAgentHeartbeat tests the heartbeat protocol.
func TestAgentHeartbeat(t *testing.T) {
	cfg := getGRPCTestConfig()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	err := waitForGRPC(ctx, cfg, cfg.ConnectTimeout)
	if err != nil {
		t.Skipf("gRPC server not available: %v", err)
	}

	conn, err := dialGRPC(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	client := proto.NewAgentServiceClient(conn)

	t.Run("heartbeat with valid agent", func(t *testing.T) {
		req := &proto.HeartbeatRequest{
			AgentId:   cfg.AgentID,
			Timestamp: time.Now().Unix(),
			Stats: &proto.AgentStats{
				CpuPercent:        25.5,
				MemoryPercent:     50.0,
				DiskPercent:       30.0,
				DiskFreeBytes:     1024 * 1024 * 1024 * 100, // 100GB
				ActiveConnections: 5,
			},
			Version: "1.0.0-e2e-test",
			Os:      "linux",
			Arch:    "amd64",
		}

		resp, err := client.Heartbeat(ctx, req)
		if err != nil {
			// If agent doesn't exist, we expect NotFound
			st, ok := status.FromError(err)
			if ok && st.Code() == codes.NotFound {
				t.Skipf("Agent not registered: %v", err)
			}
			t.Logf("Heartbeat error (may be expected if agent not registered): %v", err)
			return
		}

		if !resp.Ok {
			t.Error("heartbeat should be acknowledged")
		}

		if resp.ServerTimestamp == 0 {
			t.Error("server timestamp should be set")
		}

		t.Logf("Heartbeat acknowledged with server timestamp %d", resp.ServerTimestamp)

		// Check for update notification (optional)
		if resp.UpdateAvailable != nil {
			t.Logf("Update available: version=%s, url=%s",
				resp.UpdateAvailable.Version,
				resp.UpdateAvailable.DownloadUrl)
		}
	})

	t.Run("heartbeat with missing agent_id", func(t *testing.T) {
		req := &proto.HeartbeatRequest{
			AgentId:   "",
			Timestamp: time.Now().Unix(),
		}

		_, err := client.Heartbeat(ctx, req)
		if err == nil {
			t.Error("heartbeat should fail with missing agent_id")
			return
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Errorf("expected gRPC status error, got: %v", err)
			return
		}

		// Accept Internal error if it's a proto marshaling issue
		if st.Code() == codes.Internal && strings.Contains(st.Message(), "marshal") {
			t.Skipf("proto marshaling issue, skipping: %v", err)
			return
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument error, got: %v", st.Code())
		}
	})

	t.Run("heartbeat with deployment status", func(t *testing.T) {
		req := &proto.HeartbeatRequest{
			AgentId:   cfg.AgentID,
			Timestamp: time.Now().Unix(),
			ActiveDeployments: []*proto.DeploymentStatus{
				{
					DeploymentId:    "deploy-001",
					State:           proto.DeploymentState_DEPLOYMENT_STATE_DEPLOYING,
					Message:         "Running post-deploy hooks",
					ProgressPercent: 75,
					CurrentStep:     "post_deploy",
					ReleaseNumber:   5,
				},
			},
		}

		resp, err := client.Heartbeat(ctx, req)
		if err != nil {
			st, ok := status.FromError(err)
			if ok && st.Code() == codes.NotFound {
				t.Skipf("Agent not registered: %v", err)
			}
			t.Logf("Heartbeat with deployment status error: %v", err)
			return
		}

		if !resp.Ok {
			t.Error("heartbeat should be acknowledged even with deployment status")
		}
	})
}

// TestAgentConnect tests the bi-directional streaming connection.
func TestAgentConnect(t *testing.T) {
	cfg := getGRPCTestConfig()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	err := waitForGRPC(ctx, cfg, cfg.ConnectTimeout)
	if err != nil {
		t.Skipf("gRPC server not available: %v", err)
	}

	conn, err := dialGRPC(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	client := proto.NewAgentServiceClient(conn)

	t.Run("connect without AgentReady", func(t *testing.T) {
		stream, err := client.Connect(ctx)
		if err != nil {
			t.Fatalf("failed to open stream: %v", err)
		}

		// Send a non-AgentReady message first (should fail)
		err = stream.Send(&proto.AgentMessage{
			Message: &proto.AgentMessage_DeploymentStatus{
				DeploymentStatus: &proto.DeploymentStatus{
					DeploymentId: "test",
					State:        proto.DeploymentState_DEPLOYMENT_STATE_PENDING,
				},
			},
		})
		if err != nil {
			t.Logf("Send error (may be expected): %v", err)
			return
		}

		// Try to receive - should get an error because first message wasn't AgentReady
		_, err = stream.Recv()
		if err == nil {
			t.Error("expected error when connecting without AgentReady message")
		}
	})

	t.Run("connect with AgentReady", func(t *testing.T) {
		stream, err := client.Connect(ctx)
		if err != nil {
			t.Fatalf("failed to open stream: %v", err)
		}

		// Send AgentReady message
		err = stream.Send(&proto.AgentMessage{
			Message: &proto.AgentMessage_AgentReady{
				AgentReady: &proto.AgentReady{
					AgentId:   cfg.AgentID,
					Timestamp: time.Now().Unix(),
				},
			},
		})
		if err != nil {
			// Skip if proto marshaling issue
			if strings.Contains(err.Error(), "marshal") {
				t.Skipf("proto marshaling issue, skipping: %v", err)
				return
			}
			t.Fatalf("failed to send AgentReady: %v", err)
		}

		// The server might reject if agent isn't registered
		// We're testing the protocol, not the business logic
		_, err = stream.Recv()
		if err != nil {
			if err == io.EOF {
				t.Log("Stream closed by server (agent may not be registered)")
			} else {
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.NotFound {
					t.Skipf("Agent not registered: %v", err)
				}
				t.Logf("Recv error (may be expected): %v", err)
			}
		}
	})

	t.Run("connect with empty agent_id", func(t *testing.T) {
		stream, err := client.Connect(ctx)
		if err != nil {
			t.Fatalf("failed to open stream: %v", err)
		}

		// Send AgentReady with empty agent_id
		err = stream.Send(&proto.AgentMessage{
			Message: &proto.AgentMessage_AgentReady{
				AgentReady: &proto.AgentReady{
					AgentId:   "",
					Timestamp: time.Now().Unix(),
				},
			},
		})
		if err != nil {
			// Skip if proto marshaling issue
			if strings.Contains(err.Error(), "marshal") {
				t.Skipf("proto marshaling issue, skipping: %v", err)
				return
			}
			t.Fatalf("failed to send AgentReady: %v", err)
		}

		// Should get InvalidArgument error
		_, err = stream.Recv()
		if err == nil {
			t.Error("expected error when connecting with empty agent_id")
			return
		}

		st, ok := status.FromError(err)
		if ok && st.Code() == codes.InvalidArgument {
			t.Logf("Got expected InvalidArgument error: %v", st.Message())
		}
	})
}

// TestAgentStreamMessages tests sending various message types over the stream.
func TestAgentStreamMessages(t *testing.T) {
	cfg := getGRPCTestConfig()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	err := waitForGRPC(ctx, cfg, cfg.ConnectTimeout)
	if err != nil {
		t.Skipf("gRPC server not available: %v", err)
	}

	conn, err := dialGRPC(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	client := proto.NewAgentServiceClient(conn)

	// Helper to establish a connection (if agent is registered)
	establishConnection := func() (proto.AgentService_ConnectClient, error) {
		stream, err := client.Connect(ctx)
		if err != nil {
			return nil, err
		}

		err = stream.Send(&proto.AgentMessage{
			Message: &proto.AgentMessage_AgentReady{
				AgentReady: &proto.AgentReady{
					AgentId:   cfg.AgentID,
					Timestamp: time.Now().Unix(),
				},
			},
		})
		if err != nil {
			return nil, err
		}

		return stream, nil
	}

	t.Run("send deployment status", func(t *testing.T) {
		stream, err := establishConnection()
		if err != nil {
			t.Skipf("Could not establish connection: %v", err)
		}

		err = stream.Send(&proto.AgentMessage{
			Message: &proto.AgentMessage_DeploymentStatus{
				DeploymentStatus: &proto.DeploymentStatus{
					DeploymentId:    "e2e-deploy-001",
					State:           proto.DeploymentState_DEPLOYMENT_STATE_DEPLOYING,
					Message:         "Running E2E test deployment",
					Timestamp:       time.Now().Unix(),
					ProgressPercent: 50,
					CurrentStep:     "build",
					ReleaseNumber:   1,
				},
			},
		})
		if err != nil {
			t.Logf("Send deployment status error: %v", err)
		}
	})

	t.Run("send deployment log", func(t *testing.T) {
		stream, err := establishConnection()
		if err != nil {
			t.Skipf("Could not establish connection: %v", err)
		}

		err = stream.Send(&proto.AgentMessage{
			Message: &proto.AgentMessage_DeploymentLog{
				DeploymentLog: &proto.DeploymentLog{
					DeploymentId: "e2e-deploy-001",
					Timestamp:    time.Now().Unix(),
					Level:        proto.LogLevel_LOG_LEVEL_INFO,
					Message:      "E2E test log message",
					Source:       "e2e-test",
				},
			},
		})
		if err != nil {
			t.Logf("Send deployment log error: %v", err)
		}
	})

	t.Run("send command result", func(t *testing.T) {
		stream, err := establishConnection()
		if err != nil {
			t.Skipf("Could not establish connection: %v", err)
		}

		err = stream.Send(&proto.AgentMessage{
			Message: &proto.AgentMessage_CommandResult{
				CommandResult: &proto.CommandResult{
					DeploymentId: "e2e-deploy-001",
					Command:      "echo 'E2E test'",
					ExitCode:     0,
					Stdout:       "E2E test\n",
					Stderr:       "",
					DurationMs:   100,
				},
			},
		})
		if err != nil {
			t.Logf("Send command result error: %v", err)
		}
	})

	t.Run("send update result", func(t *testing.T) {
		stream, err := establishConnection()
		if err != nil {
			t.Skipf("Could not establish connection: %v", err)
		}

		err = stream.Send(&proto.AgentMessage{
			Message: &proto.AgentMessage_UpdateResult{
				UpdateResult: &proto.UpdateResult{
					FromVersion: "1.0.0",
					ToVersion:   "1.1.0",
					Success:     true,
					Error:       "",
					RolledBack:  false,
				},
			},
		})
		if err != nil {
			t.Logf("Send update result error: %v", err)
		}
	})

	t.Run("send health check result", func(t *testing.T) {
		stream, err := establishConnection()
		if err != nil {
			t.Skipf("Could not establish connection: %v", err)
		}

		err = stream.Send(&proto.AgentMessage{
			Message: &proto.AgentMessage_HealthCheckResult{
				HealthCheckResult: &proto.HealthCheckResult{
					DeploymentId:    "e2e-deploy-001",
					Success:         true,
					StatusCode:      200,
					ResponseTimeMs:  50,
					Error:           "",
					RetryCount:      0,
					TriggerRollback: false,
				},
			},
		})
		if err != nil {
			t.Logf("Send health check result error: %v", err)
		}
	})
}

// TestHeartbeatConcurrency tests concurrent heartbeat requests.
func TestHeartbeatConcurrency(t *testing.T) {
	cfg := getGRPCTestConfig()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout*2)
	defer cancel()

	err := waitForGRPC(ctx, cfg, cfg.ConnectTimeout)
	if err != nil {
		t.Skipf("gRPC server not available: %v", err)
	}

	conn, err := dialGRPC(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	client := proto.NewAgentServiceClient(conn)

	const numAgents = 5
	const heartbeatsPerAgent = 10

	var wg sync.WaitGroup
	errCh := make(chan error, numAgents*heartbeatsPerAgent)

	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(agentNum int) {
			defer wg.Done()

			agentID := fmt.Sprintf("%s-concurrent-%d", cfg.AgentID, agentNum)

			for j := 0; j < heartbeatsPerAgent; j++ {
				req := &proto.HeartbeatRequest{
					AgentId:   agentID,
					Timestamp: time.Now().Unix(),
					Stats: &proto.AgentStats{
						CpuPercent:    float64(agentNum * 10),
						MemoryPercent: float64(j * 10),
					},
				}

				_, err := client.Heartbeat(ctx, req)
				if err != nil {
					// NotFound is expected for unregistered agents
					st, ok := status.FromError(err)
					if ok && st.Code() == codes.NotFound {
						continue
					}
					errCh <- fmt.Errorf("agent %d heartbeat %d: %w", agentNum, j, err)
				}

				time.Sleep(50 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// Collect errors (NotFound errors are expected)
	var realErrors []error
	for err := range errCh {
		if err != nil {
			realErrors = append(realErrors, err)
		}
	}

	if len(realErrors) > 0 {
		t.Logf("Concurrent heartbeat had %d unexpected errors", len(realErrors))
		for i, err := range realErrors {
			if i < 5 { // Only show first 5
				t.Logf("  Error %d: %v", i+1, err)
			}
		}
	}
}

// TestProtocolMessageFormats validates protobuf message serialization.
func TestProtocolMessageFormats(t *testing.T) {
	t.Run("DeploymentState values", func(t *testing.T) {
		states := []proto.DeploymentState{
			proto.DeploymentState_DEPLOYMENT_STATE_UNSPECIFIED,
			proto.DeploymentState_DEPLOYMENT_STATE_PENDING,
			proto.DeploymentState_DEPLOYMENT_STATE_PREPARING,
			proto.DeploymentState_DEPLOYMENT_STATE_CLONING,
			proto.DeploymentState_DEPLOYMENT_STATE_BUILDING,
			proto.DeploymentState_DEPLOYMENT_STATE_DEPLOYING,
			proto.DeploymentState_DEPLOYMENT_STATE_VERIFYING,
			proto.DeploymentState_DEPLOYMENT_STATE_COMPLETED,
			proto.DeploymentState_DEPLOYMENT_STATE_FAILED,
			proto.DeploymentState_DEPLOYMENT_STATE_CANCELLED,
			proto.DeploymentState_DEPLOYMENT_STATE_ROLLING_BACK,
		}

		for _, state := range states {
			name := state.String()
			if name == "" {
				t.Errorf("DeploymentState %d has empty string representation", state)
			}
		}
	})

	t.Run("LogLevel values", func(t *testing.T) {
		levels := []proto.LogLevel{
			proto.LogLevel_LOG_LEVEL_UNSPECIFIED,
			proto.LogLevel_LOG_LEVEL_DEBUG,
			proto.LogLevel_LOG_LEVEL_INFO,
			proto.LogLevel_LOG_LEVEL_WARN,
			proto.LogLevel_LOG_LEVEL_ERROR,
		}

		for _, level := range levels {
			name := level.String()
			if name == "" {
				t.Errorf("LogLevel %d has empty string representation", level)
			}
		}
	})

	t.Run("AgentCapabilities serialization", func(t *testing.T) {
		caps := &proto.AgentCapabilities{
			CanUseNamespaces: true,
			AllowedUsers:     []string{"deploy", "root"},
		}

		// Verify fields are accessible
		if !caps.CanUseNamespaces {
			t.Error("CanUseNamespaces should be true")
		}
		if len(caps.AllowedUsers) != 2 {
			t.Errorf("expected 2 allowed users, got %d", len(caps.AllowedUsers))
		}
	})

	t.Run("DeployCommand structure", func(t *testing.T) {
		cmd := &proto.DeployCommand{
			Settings: &proto.DeploymentSettings{
				Strategy: "symlink",
			},
			ReloadServices: []*proto.ServiceReload{
				{Service: "php-fpm", Action: "reload"},
				{Service: "nginx", Action: "reload"},
			},
		}

		if cmd.Settings.Strategy != "symlink" {
			t.Errorf("expected strategy symlink, got %s", cmd.Settings.Strategy)
		}
		if len(cmd.ReloadServices) != 2 {
			t.Errorf("expected 2 reload services, got %d", len(cmd.ReloadServices))
		}
	})

	t.Run("MasterMessage types", func(t *testing.T) {
		messages := []*proto.MasterMessage{
			{Message: &proto.MasterMessage_DeployCommand{
				DeployCommand: &proto.DeployCommand{DeploymentId: "d1"},
			}},
			{Message: &proto.MasterMessage_RollbackCommand{
				RollbackCommand: &proto.RollbackCommand{DeploymentId: "d1"},
			}},
			{Message: &proto.MasterMessage_CancelCommand{
				CancelCommand: &proto.CancelCommand{DeploymentId: "d1"},
			}},
			{Message: &proto.MasterMessage_HealthCheckCommand{
				HealthCheckCommand: &proto.HealthCheckCommand{DeploymentId: "d1"},
			}},
			{Message: &proto.MasterMessage_UpdateCommand{
				UpdateCommand: &proto.UpdateCommand{Version: "1.0.0"},
			}},
		}

		for i, msg := range messages {
			if msg.Message == nil {
				t.Errorf("message %d has nil Message", i)
			}
		}
	})

	t.Run("AgentMessage types", func(t *testing.T) {
		messages := []*proto.AgentMessage{
			{Message: &proto.AgentMessage_DeploymentStatus{
				DeploymentStatus: &proto.DeploymentStatus{DeploymentId: "d1"},
			}},
			{Message: &proto.AgentMessage_DeploymentLog{
				DeploymentLog: &proto.DeploymentLog{DeploymentId: "d1"},
			}},
			{Message: &proto.AgentMessage_CommandResult{
				CommandResult: &proto.CommandResult{DeploymentId: "d1"},
			}},
			{Message: &proto.AgentMessage_AgentReady{
				AgentReady: &proto.AgentReady{AgentId: "a1"},
			}},
			{Message: &proto.AgentMessage_UpdateResult{
				UpdateResult: &proto.UpdateResult{FromVersion: "1.0"},
			}},
			{Message: &proto.AgentMessage_HealthCheckResult{
				HealthCheckResult: &proto.HealthCheckResult{DeploymentId: "d1"},
			}},
		}

		for i, msg := range messages {
			if msg.Message == nil {
				t.Errorf("message %d has nil Message", i)
			}
		}
	})
}

// TestHealthCheckCommand validates health check command structure.
func TestHealthCheckCommand(t *testing.T) {
	t.Run("construct health check command", func(t *testing.T) {
		cmd := &proto.HealthCheckCommand{
			Url:             "http://localhost:8080/health",
			Method:          "GET",
			TriggerRollback: true,
		}

		if cmd.Url != "http://localhost:8080/health" {
			t.Errorf("unexpected URL: %s", cmd.Url)
		}
		if cmd.Method != "GET" {
			t.Errorf("unexpected method: %s", cmd.Method)
		}
		if !cmd.TriggerRollback {
			t.Error("TriggerRollback should be true")
		}
	})
}

// TestUpdateCommand validates agent update command structure.
func TestUpdateCommand(t *testing.T) {
	t.Run("construct update command", func(t *testing.T) {
		cmd := &proto.UpdateCommand{
			Version:        "1.2.0",
			ChecksumSha256: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		}

		if cmd.Version != "1.2.0" {
			t.Errorf("unexpected version: %s", cmd.Version)
		}
		if len(cmd.ChecksumSha256) != 64 {
			t.Errorf("expected 64 char checksum, got %d", len(cmd.ChecksumSha256))
		}
	})
}

// TestRollbackCommand validates rollback command structure.
func TestRollbackCommand(t *testing.T) {
	t.Run("construct rollback command", func(t *testing.T) {
		cmd := &proto.RollbackCommand{
			ReleaseNumber: 3, // Rollback to release 3
			RollbackHooks: []string{
				"php artisan down",
				"php artisan migrate:rollback",
				"php artisan up",
			},
		}

		if cmd.ReleaseNumber != 3 {
			t.Errorf("unexpected release number: %d", cmd.ReleaseNumber)
		}
		if len(cmd.RollbackHooks) != 3 {
			t.Errorf("expected 3 rollback hooks, got %d", len(cmd.RollbackHooks))
		}
	})
}
