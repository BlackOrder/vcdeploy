//go:build integration

package deploy

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestSSHRunner_Integration tests SSH runner with a real SSH server container.
func TestSSHRunner_Integration(t *testing.T) {
	ctx := context.Background()

	// Start SSH container
	container, err := startSSHContainer(ctx, t)
	if err != nil {
		t.Fatalf("failed to start SSH container: %v", err)
	}

	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "2222")

	config := &SSHConfig{
		Host:                  host,
		Port:                  port.Int(),
		User:                  "testuser",
		Password:              "testpass",
		Timeout:               30 * time.Second,
		InsecureIgnoreHostKey: true, // Skip host key verification for testing
	}

	runner, err := NewSSHRunner(config)
	if err != nil {
		t.Fatalf("NewSSHRunner() error = %v", err)
	}
	defer runner.Close()

	t.Run("connect", func(t *testing.T) {
		err := runner.Connect(ctx)
		if err != nil {
			t.Fatalf("Connect() error = %v", err)
		}
	})

	t.Run("run simple command", func(t *testing.T) {
		result, err := runner.Run(ctx, "echo hello", RunOptions{})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("unexpected exit code: %d", result.ExitCode)
		}
		if !bytes.Contains([]byte(result.Stdout), []byte("hello")) {
			t.Errorf("unexpected output: %s", result.Stdout)
		}
	})

	t.Run("run with workdir", func(t *testing.T) {
		result, err := runner.Run(ctx, "pwd", RunOptions{
			WorkDir: "/tmp",
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !bytes.Contains([]byte(result.Stdout), []byte("/tmp")) {
			t.Errorf("unexpected output: %s", result.Stdout)
		}
	})

	t.Run("run with env", func(t *testing.T) {
		result, err := runner.Run(ctx, "echo $MY_VAR", RunOptions{
			Env: map[string]string{
				"MY_VAR": "test_value",
			},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !bytes.Contains([]byte(result.Stdout), []byte("test_value")) {
			t.Errorf("unexpected output: %s", result.Stdout)
		}
	})

	t.Run("run failing command", func(t *testing.T) {
		result, err := runner.Run(ctx, "exit 42", RunOptions{})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.ExitCode != 42 {
			t.Errorf("expected exit code 42, got %d", result.ExitCode)
		}
	})

	t.Run("run with streaming output", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := runner.RunWithOutput(ctx, "echo stdout && echo stderr >&2", &stdout, &stderr, RunOptions{})
		if err != nil {
			t.Fatalf("RunWithOutput() error = %v", err)
		}
		if !bytes.Contains(stdout.Bytes(), []byte("stdout")) {
			t.Errorf("unexpected stdout: %s", stdout.String())
		}
		if !bytes.Contains(stderr.Bytes(), []byte("stderr")) {
			t.Errorf("unexpected stderr: %s", stderr.String())
		}
	})

	t.Run("last used updated", func(t *testing.T) {
		before := runner.LastUsed()
		time.Sleep(10 * time.Millisecond)
		runner.Run(ctx, "echo test", RunOptions{})
		after := runner.LastUsed()
		if !after.After(before) {
			t.Error("LastUsed() should be updated after Run()")
		}
	})
}

// TestSSHPool_Integration tests SSH pool with real connections.
func TestSSHPool_Integration(t *testing.T) {
	ctx := context.Background()

	container, err := startSSHContainer(ctx, t)
	if err != nil {
		t.Fatalf("failed to start SSH container: %v", err)
	}

	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "2222")

	pool := NewSSHPool(5 * time.Minute)
	defer pool.Close()

	config := &SSHConfig{
		Host:                  host,
		Port:                  port.Int(),
		User:                  "testuser",
		Password:              "testpass",
		Timeout:               30 * time.Second,
		InsecureIgnoreHostKey: true, // Skip host key verification for testing
	}

	t.Run("get and run", func(t *testing.T) {
		runner, err := pool.Get(config)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		result, err := runner.Run(ctx, "hostname", RunOptions{})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("unexpected exit code: %d", result.ExitCode)
		}
	})

	t.Run("connection reuse", func(t *testing.T) {
		runner1, _ := pool.Get(config)
		runner2, _ := pool.Get(config)

		if runner1 != runner2 {
			t.Error("same config should return same runner")
		}

		// Both should work
		runner1.Run(ctx, "echo 1", RunOptions{})
		runner2.Run(ctx, "echo 2", RunOptions{})
	})
}

func startSSHContainer(ctx context.Context, t *testing.T) (testcontainers.Container, error) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "lscr.io/linuxserver/openssh-server:latest",
		ExposedPorts: []string{"2222/tcp"},
		Env: map[string]string{
			"PUID":            "1000",
			"PGID":            "1000",
			"TZ":              "UTC",
			"USER_NAME":       "testuser",
			"USER_PASSWORD":   "testpass",
			"PASSWORD_ACCESS": "true",
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("sshd is listening").WithStartupTimeout(90*time.Second),
			wait.ForListeningPort("2222/tcp").WithStartupTimeout(90*time.Second),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	// Additional wait for SSH to be truly ready
	time.Sleep(2 * time.Second)

	t.Cleanup(func() {
		container.Terminate(ctx)
	})

	return container, nil
}
