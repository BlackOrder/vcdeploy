//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// knownHostsStore provides thread-safe storage for host keys discovered during tests.
// This implements Trust-On-First-Use (TOFU) with verification on subsequent connections.
type knownHostsStore struct {
	mu    sync.RWMutex
	hosts map[string]ssh.PublicKey
}

var testKnownHosts = &knownHostsStore{
	hosts: make(map[string]ssh.PublicKey),
}

// trustOnFirstUseCallback returns a HostKeyCallback that trusts the first key seen
// for a host and verifies all subsequent connections use the same key.
// This is more secure than InsecureIgnoreHostKey while still allowing dynamic test environments.
func (k *knownHostsStore) trustOnFirstUseCallback() ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		k.mu.Lock()
		defer k.mu.Unlock()

		existingKey, exists := k.hosts[hostname]
		if !exists {
			// First connection - trust and store the key
			k.hosts[hostname] = key
			return nil
		}

		// Subsequent connection - verify key matches
		if string(existingKey.Marshal()) != string(key.Marshal()) {
			return fmt.Errorf("host key mismatch for %s: possible MITM attack", hostname)
		}
		return nil
	}
}

// getSSHClientConfig returns a secure SSH client config using TOFU verification.
func getSSHClientConfig() *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User: "deploy",
		Auth: []ssh.AuthMethod{
			ssh.Password("deploypass"),
		},
		HostKeyCallback: testKnownHosts.trustOnFirstUseCallback(),
		Timeout:         10 * time.Second,
	}
}

// TestSSHTargetConnectivity verifies the SSH target is reachable.
func TestSSHTargetConnectivity(t *testing.T) {
	cfg := getTestConfig()

	// Wait for target SSH to be ready
	addr := fmt.Sprintf("%s:%s", cfg.TargetSSHHost, cfg.TargetSSHPort)
	err := waitForTCPPort(addr, 60*time.Second)
	if err != nil {
		t.Skipf("SSH target not available: %v", err)
	}

	sshConfig := getSSHClientConfig()

	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		t.Fatalf("failed to connect to SSH target: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput("echo hello")
	if err != nil {
		t.Fatalf("failed to run command: %v", err)
	}

	if string(output) != "hello\n" {
		t.Errorf("unexpected output: %q", string(output))
	}
}

// TestSSHTargetDeployDirectory verifies the deploy directory exists.
func TestSSHTargetDeployDirectory(t *testing.T) {
	cfg := getTestConfig()

	addr := fmt.Sprintf("%s:%s", cfg.TargetSSHHost, cfg.TargetSSHPort)
	err := waitForTCPPort(addr, 60*time.Second)
	if err != nil {
		t.Skipf("SSH target not available: %v", err)
	}

	sshConfig := getSSHClientConfig()

	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		t.Fatalf("failed to connect to SSH target: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	// Check if /deploy directory exists
	err = session.Run("test -d /deploy")
	if err != nil {
		t.Errorf("/deploy directory does not exist on target")
	}
}

// TestDeploymentSimulation simulates a basic deployment workflow.
func TestDeploymentSimulation(t *testing.T) {
	cfg := getTestConfig()

	// Check SSH target
	addr := fmt.Sprintf("%s:%s", cfg.TargetSSHHost, cfg.TargetSSHPort)
	err := waitForTCPPort(addr, 60*time.Second)
	if err != nil {
		t.Skipf("SSH target not available: %v", err)
	}

	sshConfig := getSSHClientConfig()

	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		t.Fatalf("failed to connect to SSH target: %v", err)
	}
	defer client.Close()

	// Simulate deployment steps
	steps := []struct {
		name string
		cmd  string
	}{
		{"create release directory", "mkdir -p /deploy/test-app/releases/v1.0.0"},
		{"create index file", "echo 'version=1.0.0' > /deploy/test-app/releases/v1.0.0/index.html"},
		{"create current symlink", "ln -sfn /deploy/test-app/releases/v1.0.0 /deploy/test-app/current"},
		{"verify symlink", "readlink /deploy/test-app/current"},
		{"verify content", "cat /deploy/test-app/current/index.html"},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			session, err := client.NewSession()
			if err != nil {
				t.Fatalf("failed to create session: %v", err)
			}
			defer session.Close()

			output, err := session.CombinedOutput(step.cmd)
			if err != nil {
				t.Errorf("command %q failed: %v, output: %s", step.cmd, err, output)
			}
		})
	}

	// Cleanup
	t.Cleanup(func() {
		session, err := client.NewSession()
		if err != nil {
			t.Logf("Warning: failed to create cleanup session: %v", err)
			return
		}
		defer session.Close()
		if err := session.Run("rm -rf /deploy/test-app"); err != nil {
			t.Logf("Warning: cleanup command failed: %v", err)
		}
	})
}

// TestRollbackSimulation simulates a rollback operation.
func TestRollbackSimulation(t *testing.T) {
	cfg := getTestConfig()

	addr := fmt.Sprintf("%s:%s", cfg.TargetSSHHost, cfg.TargetSSHPort)
	err := waitForTCPPort(addr, 60*time.Second)
	if err != nil {
		t.Skipf("SSH target not available: %v", err)
	}

	sshConfig := getSSHClientConfig()

	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		t.Fatalf("failed to connect to SSH target: %v", err)
	}
	defer client.Close()

	// Setup - create two releases
	setupCmds := []string{
		"mkdir -p /deploy/rollback-app/releases/v1.0.0",
		"echo 'v1' > /deploy/rollback-app/releases/v1.0.0/version.txt",
		"mkdir -p /deploy/rollback-app/releases/v2.0.0",
		"echo 'v2' > /deploy/rollback-app/releases/v2.0.0/version.txt",
		"ln -sfn /deploy/rollback-app/releases/v2.0.0 /deploy/rollback-app/current",
	}

	for _, cmd := range setupCmds {
		session, err := client.NewSession()
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
		if err := session.Run(cmd); err != nil {
			session.Close()
			t.Fatalf("setup command failed: %s: %v", cmd, err)
		}
		session.Close()
	}

	// Verify current is v2
	t.Run("verify current is v2", func(t *testing.T) {
		session, err := client.NewSession()
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
		defer session.Close()
		output, err := session.Output("cat /deploy/rollback-app/current/version.txt")
		if err != nil {
			t.Fatalf("failed to read version: %v", err)
		}
		if string(output) != "v2\n" {
			t.Errorf("expected v2, got %q", string(output))
		}
	})

	// Rollback to v1
	t.Run("rollback to v1", func(t *testing.T) {
		session, err := client.NewSession()
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
		defer session.Close()
		if err := session.Run("ln -sfn /deploy/rollback-app/releases/v1.0.0 /deploy/rollback-app/current"); err != nil {
			t.Fatalf("rollback failed: %v", err)
		}
	})

	// Verify current is now v1
	t.Run("verify current is v1 after rollback", func(t *testing.T) {
		session, err := client.NewSession()
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
		defer session.Close()
		output, err := session.Output("cat /deploy/rollback-app/current/version.txt")
		if err != nil {
			t.Fatalf("failed to read version: %v", err)
		}
		if string(output) != "v1\n" {
			t.Errorf("expected v1 after rollback, got %q", string(output))
		}
	})

	// Cleanup
	t.Cleanup(func() {
		session, err := client.NewSession()
		if err != nil {
			t.Logf("Warning: failed to create cleanup session: %v", err)
			return
		}
		defer session.Close()
		if err := session.Run("rm -rf /deploy/rollback-app"); err != nil {
			t.Logf("Warning: cleanup command failed: %v", err)
		}
	})
}

// waitForTCPPort waits for a TCP port to become available.
func waitForTCPPort(addr string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s", addr)
		default:
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err == nil {
				conn.Close()
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}
