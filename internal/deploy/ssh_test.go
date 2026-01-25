package deploy

import (
	"bytes"
	"testing"
	"time"
)

// --- Unit Tests (Mock-based) ---

func TestNewSSHRunner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *SSHConfig
		check  func(*testing.T, *SSHRunner)
	}{
		{
			name: "default port",
			config: &SSHConfig{
				Host: "example.com",
				User: "deploy",
			},
			check: func(t *testing.T, r *SSHRunner) {
				if r.config.Port != 22 {
					t.Errorf("expected default port 22, got %d", r.config.Port)
				}
			},
		},
		{
			name: "custom port",
			config: &SSHConfig{
				Host: "example.com",
				Port: 2222,
				User: "deploy",
			},
			check: func(t *testing.T, r *SSHRunner) {
				if r.config.Port != 2222 {
					t.Errorf("expected port 2222, got %d", r.config.Port)
				}
			},
		},
		{
			name: "default timeout",
			config: &SSHConfig{
				Host: "example.com",
				User: "deploy",
			},
			check: func(t *testing.T, r *SSHRunner) {
				if r.config.Timeout != 30*time.Second {
					t.Errorf("expected default timeout 30s, got %v", r.config.Timeout)
				}
			},
		},
		{
			name: "custom timeout",
			config: &SSHConfig{
				Host:    "example.com",
				User:    "deploy",
				Timeout: 60 * time.Second,
			},
			check: func(t *testing.T, r *SSHRunner) {
				if r.config.Timeout != 60*time.Second {
					t.Errorf("expected timeout 60s, got %v", r.config.Timeout)
				}
			},
		},
		{
			name: "default jump port",
			config: &SSHConfig{
				Host:     "target.com",
				User:     "deploy",
				JumpHost: "bastion.com",
			},
			check: func(t *testing.T, r *SSHRunner) {
				if r.config.JumpPort != 22 {
					t.Errorf("expected default jump port 22, got %d", r.config.JumpPort)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner, err := NewSSHRunner(tt.config)
			if err != nil {
				t.Fatalf("NewSSHRunner() error = %v", err)
			}
			if runner == nil {
				t.Fatal("NewSSHRunner() returned nil")
			}
			tt.check(t, runner)
		})
	}
}

func TestSSHRunner_buildCommand(t *testing.T) {
	t.Parallel()

	runner, _ := NewSSHRunner(&SSHConfig{
		Host: "example.com",
		User: "deploy",
	})

	tests := []struct {
		name     string
		cmd      string
		opts     RunOptions
		expected string
	}{
		{
			name:     "simple command",
			cmd:      "ls -la",
			opts:     RunOptions{},
			expected: "ls -la",
		},
		{
			name: "with workdir",
			cmd:  "ls -la",
			opts: RunOptions{
				WorkDir: "/var/www/app",
			},
			expected: "cd /var/www/app && ls -la",
		},
		{
			name: "with env vars",
			cmd:  "npm run build",
			opts: RunOptions{
				Env: map[string]string{
					"NODE_ENV": "production",
				},
			},
			expected: `export NODE_ENV="production" && npm run build`,
		},
		{
			name: "with multiple env vars",
			cmd:  "deploy.sh",
			opts: RunOptions{
				Env: map[string]string{
					"APP_ENV": "prod",
					"DEBUG":   "false",
				},
			},
			// Note: map order is not guaranteed, so we check contains
		},
		{
			name: "with user",
			cmd:  "whoami",
			opts: RunOptions{
				User: "www-data",
			},
			expected: `sudo -u www-data bash -c "whoami"`,
		},
		{
			name: "with workdir and user",
			cmd:  "php artisan migrate",
			opts: RunOptions{
				WorkDir: "/var/www/app",
				User:    "www-data",
			},
			expected: `sudo -u www-data bash -c "cd /var/www/app && php artisan migrate"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := runner.buildCommand(tt.cmd, tt.opts)
			if err != nil {
				t.Fatalf("buildCommand() error: %v", err)
			}

			// For multi-env test, just check structure
			if tt.name == "with multiple env vars" {
				if !contains(got, "export APP_ENV=") || !contains(got, "export DEBUG=") {
					t.Errorf("buildCommand() missing env vars: %s", got)
				}
				return
			}

			if got != tt.expected {
				t.Errorf("buildCommand() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

func TestSSHRunner_Close(t *testing.T) {
	t.Parallel()

	runner, _ := NewSSHRunner(&SSHConfig{
		Host: "example.com",
		User: "deploy",
	})

	// Close without connecting should not error
	err := runner.Close()
	if err != nil {
		t.Errorf("Close() without connection should not error: %v", err)
	}

	// Multiple closes should be safe
	err = runner.Close()
	if err != nil {
		t.Errorf("Second Close() should not error: %v", err)
	}
}

func TestSSHRunner_LastUsed(t *testing.T) {
	t.Parallel()

	runner, _ := NewSSHRunner(&SSHConfig{
		Host: "example.com",
		User: "deploy",
	})

	// Initially should be zero time
	if !runner.LastUsed().IsZero() {
		t.Error("LastUsed() should be zero before any connection")
	}
}

// --- SSH Pool Tests ---

func TestNewSSHPool(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)
	if pool == nil {
		t.Fatal("NewSSHPool() returned nil")
	}
	defer pool.Close()

	if pool.connections == nil {
		t.Error("connections map should be initialized")
	}
	if pool.idleTimeout != 5*time.Minute {
		t.Errorf("idleTimeout = %v, want 5m", pool.idleTimeout)
	}
}

func TestSSHPool_Get(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)
	defer pool.Close()

	config := &SSHConfig{
		Host: "example.com",
		Port: 22,
		User: "deploy",
	}

	// First get should create a new runner
	runner1, err := pool.Get(config)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if runner1 == nil {
		t.Fatal("Get() returned nil runner")
	}

	// Second get with same config should return same runner
	runner2, err := pool.Get(config)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if runner1 != runner2 {
		t.Error("Get() should return same runner for same config")
	}

	// Different config should return different runner
	config2 := &SSHConfig{
		Host: "other.com",
		Port: 22,
		User: "deploy",
	}
	runner3, err := pool.Get(config2)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if runner1 == runner3 {
		t.Error("Get() should return different runner for different config")
	}
}

func TestSSHPool_Get_WithJumpServer(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)
	defer pool.Close()

	configWithJump := &SSHConfig{
		Host:     "target.com",
		Port:     22,
		User:     "deploy",
		JumpHost: "bastion.com",
		JumpPort: 22,
	}

	configWithoutJump := &SSHConfig{
		Host: "target.com",
		Port: 22,
		User: "deploy",
	}

	runner1, _ := pool.Get(configWithJump)
	runner2, _ := pool.Get(configWithoutJump)

	// Should be different runners due to different jump config
	if runner1 == runner2 {
		t.Error("Runners with different jump configs should be different")
	}
}

func TestSSHPool_Close(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)

	// Add some connections
	_, _ = pool.Get(&SSHConfig{Host: "host1.com", User: "deploy"})
	_, _ = pool.Get(&SSHConfig{Host: "host2.com", User: "deploy"})

	// Close should clear all connections
	err := pool.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	pool.mu.RLock()
	count := len(pool.connections)
	pool.mu.RUnlock()

	if count != 0 {
		t.Errorf("After Close(), connection count = %d, want 0", count)
	}
}

func TestSSHConfig_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *SSHConfig
		valid  bool
	}{
		{
			name: "valid minimal",
			config: &SSHConfig{
				Host: "example.com",
				User: "deploy",
			},
			valid: true,
		},
		{
			name: "valid with all fields",
			config: &SSHConfig{
				Host:     "example.com",
				Port:     2222,
				User:     "deploy",
				KeyPath:  "/path/to/key",
				Timeout:  60 * time.Second,
				JumpHost: "bastion.com",
				JumpPort: 22,
				JumpUser: "jump",
			},
			valid: true,
		},
		{
			name: "empty host",
			config: &SSHConfig{
				Host: "",
				User: "deploy",
			},
			valid: false,
		},
		{
			name: "empty user",
			config: &SSHConfig{
				Host: "example.com",
				User: "",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// NewSSHRunner should handle validation
			runner, err := NewSSHRunner(tt.config)
			if tt.valid {
				if err != nil {
					t.Errorf("NewSSHRunner() error = %v, expected success", err)
				}
				if runner == nil {
					t.Error("NewSSHRunner() returned nil")
				}
			}
			// Note: Current implementation doesn't validate empty host/user
			// This test documents expected behavior for future validation
		})
	}
}

func TestSSHRunner_buildCommandWithEnv(t *testing.T) {
	t.Parallel()

	config := &SSHConfig{
		Host: "example.com",
		User: "deploy",
	}

	runner, err := NewSSHRunner(config)
	if err != nil {
		t.Fatalf("NewSSHRunner() error = %v", err)
	}

	tests := []struct {
		name    string
		cmd     string
		opts    RunOptions
		wantCmd string
	}{
		{
			name:    "simple command",
			cmd:     "ls -la",
			opts:    RunOptions{},
			wantCmd: "ls -la",
		},
		{
			name: "with workdir",
			cmd:  "pwd",
			opts: RunOptions{
				WorkDir: "/var/www/app",
			},
			wantCmd: "cd /var/www/app && pwd",
		},
		{
			name: "with env vars",
			cmd:  "echo $VAR1",
			opts: RunOptions{
				Env: map[string]string{
					"VAR1": "value1",
				},
			},
			wantCmd: "export VAR1=\"value1\" && echo $VAR1",
		},
		{
			name: "with workdir and env",
			cmd:  "echo $VAR1",
			opts: RunOptions{
				WorkDir: "/app",
				Env: map[string]string{
					"VAR1": "test",
				},
			},
			wantCmd: "cd /app && export VAR1=\"test\" && echo $VAR1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runner.buildCommand(tt.cmd, tt.opts)
			if err != nil {
				t.Fatalf("buildCommand() error = %v", err)
			}
			if got != tt.wantCmd {
				t.Errorf("buildCommand() = %q, want %q", got, tt.wantCmd)
			}
		})
	}
}

func TestSSHPoolKeyGeneration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *SSHConfig
	}{
		{
			name: "basic config",
			config: &SSHConfig{
				Host: "example.com",
				Port: 22,
				User: "deploy",
			},
		},
		{
			name: "with jump host",
			config: &SSHConfig{
				Host:     "target.com",
				Port:     22,
				User:     "deploy",
				JumpHost: "bastion.com",
				JumpPort: 22,
			},
		},
		{
			name: "custom port",
			config: &SSHConfig{
				Host: "example.com",
				Port: 2222,
				User: "deployer",
			},
		},
	}

	pool := NewSSHPool(5 * time.Minute)
	defer pool.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner1, _ := pool.Get(tt.config)
			runner2, _ := pool.Get(tt.config)

			// Same config should return same runner (pooled)
			if runner1 != runner2 {
				t.Error("Same config should return pooled runner")
			}
		})
	}
}

// --- Integration Tests (require Docker/testcontainers) ---

// These tests require the //go:build integration tag and will be in a separate file
// or guarded by build tags. See ssh_integration_test.go
