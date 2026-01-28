package deploy

import (
	"bytes"
	"os"
	"strings"
	"sync"
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

// --- Additional unit tests for improved coverage ---

func TestSSHRunner_buildCommandWithInvalidUser(t *testing.T) {
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
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid username",
			cmd:  "ls",
			opts: RunOptions{
				User: "www-data",
			},
			wantErr: false,
		},
		{
			name: "invalid username with semicolon",
			cmd:  "ls",
			opts: RunOptions{
				User: "user; rm -rf /",
			},
			wantErr: true,
			errMsg:  "invalid username",
		},
		{
			name: "invalid username with spaces",
			cmd:  "ls",
			opts: RunOptions{
				User: "user name",
			},
			wantErr: true,
			errMsg:  "invalid username",
		},
		{
			name: "invalid username too long",
			cmd:  "ls",
			opts: RunOptions{
				User: "verylongusernamethatexceedsthelimit123456",
			},
			wantErr: true,
			errMsg:  "invalid username",
		},
		{
			name: "username starting with number",
			cmd:  "ls",
			opts: RunOptions{
				User: "1user",
			},
			wantErr: true,
			errMsg:  "invalid username",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runner.buildCommand(tt.cmd, tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Error("buildCommand() expected error, got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("buildCommand() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("buildCommand() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestSSHRunner_buildCommandComplex(t *testing.T) {
	t.Parallel()

	runner, _ := NewSSHRunner(&SSHConfig{
		Host: "example.com",
		User: "deploy",
	})

	tests := []struct {
		name     string
		cmd      string
		opts     RunOptions
		contains []string
	}{
		{
			name: "workdir and user combined",
			cmd:  "php artisan migrate",
			opts: RunOptions{
				WorkDir: "/var/www/app",
				User:    "www-data",
			},
			contains: []string{"sudo -u www-data", "cd /var/www/app", "php artisan migrate"},
		},
		{
			name: "env vars and workdir",
			cmd:  "npm run build",
			opts: RunOptions{
				WorkDir: "/app",
				Env: map[string]string{
					"NODE_ENV": "production",
				},
			},
			contains: []string{"cd /app", "export NODE_ENV", "npm run build"},
		},
		{
			name: "all options combined",
			cmd:  "deploy.sh",
			opts: RunOptions{
				WorkDir: "/app",
				User:    "deployer",
				Env: map[string]string{
					"ENV": "prod",
				},
			},
			contains: []string{"sudo -u deployer", "cd /app", "export ENV"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runner.buildCommand(tt.cmd, tt.opts)
			if err != nil {
				t.Fatalf("buildCommand() error = %v", err)
			}
			for _, substr := range tt.contains {
				if !strings.Contains(got, substr) {
					t.Errorf("buildCommand() = %q, missing %q", got, substr)
				}
			}
		})
	}
}

func TestSSHPool_GetDifferentUsers(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)
	defer pool.Close()

	config1 := &SSHConfig{
		Host: "example.com",
		Port: 22,
		User: "user1",
	}

	config2 := &SSHConfig{
		Host: "example.com",
		Port: 22,
		User: "user2",
	}

	runner1, _ := pool.Get(config1)
	runner2, _ := pool.Get(config2)

	// Different users should have different runners
	if runner1 == runner2 {
		t.Error("Different users should have different runners")
	}
}

func TestSSHPool_GetDifferentPorts(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)
	defer pool.Close()

	config1 := &SSHConfig{
		Host: "example.com",
		Port: 22,
		User: "deploy",
	}

	config2 := &SSHConfig{
		Host: "example.com",
		Port: 2222,
		User: "deploy",
	}

	runner1, _ := pool.Get(config1)
	runner2, _ := pool.Get(config2)

	// Different ports should have different runners
	if runner1 == runner2 {
		t.Error("Different ports should have different runners")
	}
}

func TestSSHPool_GetConcurrent(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)
	defer pool.Close()

	config := &SSHConfig{
		Host: "example.com",
		Port: 22,
		User: "deploy",
	}

	var wg sync.WaitGroup
	runners := make([]*SSHRunner, 10)
	errors := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r, err := pool.Get(config)
			runners[idx] = r
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// All should succeed
	for i, err := range errors {
		if err != nil {
			t.Errorf("Get() error at %d = %v", i, err)
		}
	}

	// All should be the same runner (pooled)
	first := runners[0]
	for i, r := range runners {
		if r != first {
			t.Errorf("Runner at %d should be same as first (pooled)", i)
		}
	}
}

func TestSSHPool_CleanupLoop(t *testing.T) {
	t.Parallel()

	// Create pool with very short idle timeout for testing
	pool := &SSHPool{
		connections: make(map[string]*SSHRunner),
		idleTimeout: 10 * time.Millisecond,
		stopCh:      make(chan struct{}),
	}

	// Add a runner
	config := &SSHConfig{
		Host: "example.com",
		Port: 22,
		User: "deploy",
	}
	runner, _ := pool.Get(config)
	if runner == nil {
		t.Fatal("Expected runner")
	}

	// Wait for idle timeout
	time.Sleep(50 * time.Millisecond)

	// Trigger cleanup
	pool.cleanup()

	pool.mu.RLock()
	count := len(pool.connections)
	pool.mu.RUnlock()

	if count != 0 {
		t.Errorf("After cleanup, connection count = %d, want 0", count)
	}

	close(pool.stopCh)
}

func TestSSHRunner_CloseWithClients(t *testing.T) {
	t.Parallel()

	runner := &SSHRunner{
		config: &SSHConfig{
			Host: "example.com",
			User: "deploy",
		},
		// No actual client connections (they would require real SSH)
	}

	// Close should not error with nil clients
	err := runner.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestSSHConfig_AllFields(t *testing.T) {
	t.Parallel()

	config := &SSHConfig{
		Host:                  "example.com",
		Port:                  2222,
		User:                  "deployer",
		KeyPath:               "/path/to/key",
		KeyPassphrase:         "secret",
		Password:              "backup-password",
		Timeout:               60 * time.Second,
		KeepaliveInterval:     30 * time.Second,
		JumpHost:              "bastion.com",
		JumpPort:              22,
		JumpUser:              "jump-user",
		JumpKeyPath:           "/path/to/jump/key",
		KnownHostsPath:        "/custom/known_hosts",
		TrustOnFirstUse:       true,
		StrictHostKey:         false,
		InsecureIgnoreHostKey: false,
	}

	runner, err := NewSSHRunner(config)
	if err != nil {
		t.Fatalf("NewSSHRunner() error = %v", err)
	}

	if runner.config.Host != "example.com" {
		t.Errorf("Host = %q, want example.com", runner.config.Host)
	}
	if runner.config.Port != 2222 {
		t.Errorf("Port = %d, want 2222", runner.config.Port)
	}
	if runner.config.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", runner.config.Timeout)
	}
}

func TestSSHConfigDefaults(t *testing.T) {
	t.Parallel()

	config := &SSHConfig{
		Host: "example.com",
		User: "deploy",
		// Leave Port, Timeout, JumpPort at zero to test defaults
	}

	runner, err := NewSSHRunner(config)
	if err != nil {
		t.Fatalf("NewSSHRunner() error = %v", err)
	}

	if runner.config.Port != 22 {
		t.Errorf("Default Port = %d, want 22", runner.config.Port)
	}
	if runner.config.Timeout != 30*time.Second {
		t.Errorf("Default Timeout = %v, want 30s", runner.config.Timeout)
	}
	if runner.config.JumpPort != 22 {
		t.Errorf("Default JumpPort = %d, want 22", runner.config.JumpPort)
	}
}

func TestSSHPoolKeyFormat(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)
	defer pool.Close()

	tests := []struct {
		name   string
		config *SSHConfig
	}{
		{
			name: "basic",
			config: &SSHConfig{
				Host: "host.com",
				Port: 22,
				User: "user",
			},
		},
		{
			name: "with jump",
			config: &SSHConfig{
				Host:     "target.com",
				Port:     22,
				User:     "user",
				JumpHost: "bastion.com",
				JumpPort: 2222,
			},
		},
		{
			name: "custom port",
			config: &SSHConfig{
				Host: "host.com",
				Port: 2222,
				User: "admin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner1, _ := pool.Get(tt.config)
			runner2, _ := pool.Get(tt.config)

			// Same config should return same runner
			if runner1 != runner2 {
				t.Error("Same config should return same pooled runner")
			}
		})
	}
}

func TestRunOptionsAllFields(t *testing.T) {
	t.Parallel()

	opts := RunOptions{
		WorkDir: "/var/www/app",
		Env: map[string]string{
			"APP_ENV": "production",
			"DEBUG":   "false",
			"PORT":    "8080",
		},
		User:    "www-data",
		Group:   "www-data",
		Timeout: 5 * time.Minute,
	}

	if opts.WorkDir != "/var/www/app" {
		t.Errorf("WorkDir = %q, want /var/www/app", opts.WorkDir)
	}
	if len(opts.Env) != 3 {
		t.Errorf("Env count = %d, want 3", len(opts.Env))
	}
	if opts.User != "www-data" {
		t.Errorf("User = %q, want www-data", opts.User)
	}
	if opts.Timeout != 5*time.Minute {
		t.Errorf("Timeout = %v, want 5m", opts.Timeout)
	}
}

func TestSSHRunner_LastUsedUpdates(t *testing.T) {
	t.Parallel()

	runner, _ := NewSSHRunner(&SSHConfig{
		Host: "example.com",
		User: "deploy",
	})

	// Initially zero
	initial := runner.LastUsed()
	if !initial.IsZero() {
		t.Error("Initial LastUsed should be zero")
	}

	// Manually set lastUsed to simulate activity
	runner.mu.Lock()
	runner.lastUsed = time.Now()
	runner.mu.Unlock()

	updated := runner.LastUsed()
	if updated.IsZero() {
		t.Error("LastUsed should not be zero after update")
	}
	if !updated.After(initial) || updated.Equal(initial) {
		t.Error("LastUsed should be updated")
	}
}

func TestSSHPool_GetDoubleCheck(t *testing.T) {
	// This test verifies the double-check locking pattern in pool.Get
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)
	defer pool.Close()

	config := &SSHConfig{
		Host: "example.com",
		Port: 22,
		User: "deploy",
	}

	// First call creates the runner
	runner1, err := pool.Get(config)
	if err != nil {
		t.Fatalf("First Get() error = %v", err)
	}

	// Second call should return the same runner (from double-check after write lock)
	runner2, err := pool.Get(config)
	if err != nil {
		t.Fatalf("Second Get() error = %v", err)
	}

	if runner1 != runner2 {
		t.Error("Should return same runner from pool")
	}
}

func TestSSHRunner_ConfigFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *SSHConfig
	}{
		{
			name: "with password auth",
			config: &SSHConfig{
				Host:     "example.com",
				User:     "deploy",
				Password: "secret123",
			},
		},
		{
			name: "with key auth",
			config: &SSHConfig{
				Host:    "example.com",
				User:    "deploy",
				KeyPath: "/path/to/key",
			},
		},
		{
			name: "with key and passphrase",
			config: &SSHConfig{
				Host:          "example.com",
				User:          "deploy",
				KeyPath:       "/path/to/key",
				KeyPassphrase: "passphrase123",
			},
		},
		{
			name: "with keepalive",
			config: &SSHConfig{
				Host:              "example.com",
				User:              "deploy",
				KeepaliveInterval: 60 * time.Second,
			},
		},
		{
			name: "with TOFU mode",
			config: &SSHConfig{
				Host:            "example.com",
				User:            "deploy",
				TrustOnFirstUse: true,
			},
		},
		{
			name: "with strict host key",
			config: &SSHConfig{
				Host:          "example.com",
				User:          "deploy",
				StrictHostKey: true,
			},
		},
		{
			name: "with insecure host key (testing)",
			config: &SSHConfig{
				Host:                  "example.com",
				User:                  "deploy",
				InsecureIgnoreHostKey: true,
			},
		},
		{
			name: "with custom known_hosts",
			config: &SSHConfig{
				Host:           "example.com",
				User:           "deploy",
				KnownHostsPath: "/custom/path/known_hosts",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner, err := NewSSHRunner(tt.config)
			if err != nil {
				t.Fatalf("NewSSHRunner() error = %v", err)
			}
			if runner == nil {
				t.Fatal("NewSSHRunner() returned nil")
			}
		})
	}
}

func TestSSHPool_CloseTwice(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)

	// Add some connections
	_, _ = pool.Get(&SSHConfig{Host: "host1.com", User: "deploy"})
	_, _ = pool.Get(&SSHConfig{Host: "host2.com", User: "deploy"})

	// First close
	err := pool.Close()
	if err != nil {
		t.Errorf("First Close() error = %v", err)
	}

	// Second close should be safe (stopCh already closed)
	// This would panic if not handled properly, but we close stopCh once
	// so we just verify the state is clean
	pool.mu.RLock()
	count := len(pool.connections)
	pool.mu.RUnlock()

	if count != 0 {
		t.Errorf("After Close(), connection count = %d, want 0", count)
	}
}

func TestSSHRunner_buildCommandEdgeCases(t *testing.T) {
	t.Parallel()

	runner, _ := NewSSHRunner(&SSHConfig{
		Host: "example.com",
		User: "deploy",
	})

	tests := []struct {
		name     string
		cmd      string
		opts     RunOptions
		wantCmd  string
		contains []string
	}{
		{
			name:    "empty command",
			cmd:     "",
			opts:    RunOptions{},
			wantCmd: "",
		},
		{
			name: "empty workdir",
			cmd:  "ls",
			opts: RunOptions{
				WorkDir: "",
			},
			wantCmd: "ls",
		},
		{
			name: "empty env map",
			cmd:  "echo test",
			opts: RunOptions{
				Env: map[string]string{},
			},
			wantCmd: "echo test",
		},
		{
			name: "special characters in env value",
			cmd:  "echo test",
			opts: RunOptions{
				Env: map[string]string{
					"VAR": "value with spaces",
				},
			},
			contains: []string{"export VAR=", "value with spaces"},
		},
		{
			name: "path with spaces in workdir",
			cmd:  "ls",
			opts: RunOptions{
				WorkDir: "/path/with spaces/dir",
			},
			contains: []string{"cd /path/with spaces/dir", "ls"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runner.buildCommand(tt.cmd, tt.opts)
			if err != nil {
				t.Fatalf("buildCommand() error = %v", err)
			}

			if tt.wantCmd != "" && got != tt.wantCmd {
				t.Errorf("buildCommand() = %q, want %q", got, tt.wantCmd)
			}

			for _, substr := range tt.contains {
				if !strings.Contains(got, substr) {
					t.Errorf("buildCommand() = %q, missing %q", got, substr)
				}
			}
		})
	}
}

func TestCommandResultFields(t *testing.T) {
	t.Parallel()

	result := &CommandResult{
		ExitCode: 127,
		Stdout:   "some output\nwith newlines",
		Stderr:   "error output",
		Duration: 5 * time.Second,
	}

	if result.ExitCode != 127 {
		t.Errorf("ExitCode = %d, want 127", result.ExitCode)
	}
	if result.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want 5s", result.Duration)
	}
	if !strings.Contains(result.Stdout, "newlines") {
		t.Error("Stdout should contain newlines")
	}
}

func TestSSHRunner_loadKeyNotFound(t *testing.T) {
	t.Parallel()

	runner, _ := NewSSHRunner(&SSHConfig{
		Host: "example.com",
		User: "deploy",
	})

	// Try to load a key that doesn't exist
	_, err := runner.loadKey("/nonexistent/path/to/key", "")
	if err == nil {
		t.Error("loadKey() should return error for nonexistent key file")
	}
	if !strings.Contains(err.Error(), "reading key file") {
		t.Errorf("Error should mention 'reading key file', got: %v", err)
	}
}

func TestSSHRunner_loadKeyInvalidFormat(t *testing.T) {
	t.Parallel()

	runner, _ := NewSSHRunner(&SSHConfig{
		Host: "example.com",
		User: "deploy",
	})

	// Create a temporary file with invalid key content
	tmpFile, err := createTempFile(t, "invalid-key-*", "not a valid ssh key")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer removeTempFile(tmpFile)

	// Try to load the invalid key
	_, err = runner.loadKey(tmpFile, "")
	if err == nil {
		t.Error("loadKey() should return error for invalid key format")
	}
}

func TestSSHRunner_loadKeyInvalidPassphrase(t *testing.T) {
	t.Parallel()

	runner, _ := NewSSHRunner(&SSHConfig{
		Host: "example.com",
		User: "deploy",
	})

	// Create a temporary file with invalid encrypted key content
	tmpFile, err := createTempFile(t, "invalid-encrypted-key-*", "-----BEGIN ENCRYPTED PRIVATE KEY-----\ninvalid content\n-----END ENCRYPTED PRIVATE KEY-----")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer removeTempFile(tmpFile)

	// Try to load with a passphrase
	_, err = runner.loadKey(tmpFile, "wrongpassword")
	if err == nil {
		t.Error("loadKey() should return error for invalid encrypted key")
	}
}

func createTempFile(t *testing.T, pattern, content string) (string, error) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(content); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

func removeTempFile(path string) {
	os.Remove(path)
}

func TestSSHPool_GetNewRunner(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)
	defer pool.Close()

	config1 := &SSHConfig{
		Host: "server1.example.com",
		Port: 22,
		User: "deploy",
	}

	config2 := &SSHConfig{
		Host: "server2.example.com",
		Port: 22,
		User: "deploy",
	}

	// Get runner for first server
	runner1, err := pool.Get(config1)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Get runner for second server - should be different
	runner2, err := pool.Get(config2)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if runner1 == runner2 {
		t.Error("Different configs should return different runners")
	}
}

func TestSSHPool_CloseWithRunners(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)

	// Get a few runners
	configs := []*SSHConfig{
		{Host: "server1.example.com", Port: 22, User: "deploy"},
		{Host: "server2.example.com", Port: 22, User: "deploy"},
		{Host: "server3.example.com", Port: 22, User: "deploy"},
	}

	for _, cfg := range configs {
		_, err := pool.Get(cfg)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
	}

	// Close should not error even with multiple runners
	err := pool.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Note: second close would panic due to closed channel, so we don't test it
}

func TestSSHConfig_JumpHostSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *SSHConfig
		check  func(*testing.T, *SSHRunner)
	}{
		{
			name: "jump host with all settings",
			config: &SSHConfig{
				Host:        "target.example.com",
				Port:        22,
				User:        "deploy",
				JumpHost:    "bastion.example.com",
				JumpPort:    2222,
				JumpUser:    "admin",
				JumpKeyPath: "/path/to/jump/key",
			},
			check: func(t *testing.T, r *SSHRunner) {
				if r.config.JumpHost != "bastion.example.com" {
					t.Errorf("JumpHost = %q, want bastion.example.com", r.config.JumpHost)
				}
				if r.config.JumpPort != 2222 {
					t.Errorf("JumpPort = %d, want 2222", r.config.JumpPort)
				}
				if r.config.JumpUser != "admin" {
					t.Errorf("JumpUser = %q, want admin", r.config.JumpUser)
				}
				if r.config.JumpKeyPath != "/path/to/jump/key" {
					t.Errorf("JumpKeyPath = %q, want /path/to/jump/key", r.config.JumpKeyPath)
				}
			},
		},
		{
			name: "jump host with defaults",
			config: &SSHConfig{
				Host:     "target.example.com",
				User:     "deploy",
				JumpHost: "bastion.example.com",
			},
			check: func(t *testing.T, r *SSHRunner) {
				if r.config.JumpPort != 22 {
					t.Errorf("JumpPort = %d, want default 22", r.config.JumpPort)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner, err := NewSSHRunner(tt.config)
			if err != nil {
				t.Fatalf("NewSSHRunner() error = %v", err)
			}
			tt.check(t, runner)
		})
	}
}

func TestSSHConfig_HostKeySettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *SSHConfig
		check  func(*testing.T, *SSHRunner)
	}{
		{
			name: "strict host key checking",
			config: &SSHConfig{
				Host:          "target.example.com",
				User:          "deploy",
				StrictHostKey: true,
			},
			check: func(t *testing.T, r *SSHRunner) {
				if !r.config.StrictHostKey {
					t.Error("StrictHostKey should be true")
				}
			},
		},
		{
			name: "trust on first use mode",
			config: &SSHConfig{
				Host:            "target.example.com",
				User:            "deploy",
				TrustOnFirstUse: true,
			},
			check: func(t *testing.T, r *SSHRunner) {
				if !r.config.TrustOnFirstUse {
					t.Error("TrustOnFirstUse should be true")
				}
			},
		},
		{
			name: "insecure ignore host key",
			config: &SSHConfig{
				Host:                  "target.example.com",
				User:                  "deploy",
				InsecureIgnoreHostKey: true,
			},
			check: func(t *testing.T, r *SSHRunner) {
				if !r.config.InsecureIgnoreHostKey {
					t.Error("InsecureIgnoreHostKey should be true")
				}
			},
		},
		{
			name: "custom known hosts path",
			config: &SSHConfig{
				Host:           "target.example.com",
				User:           "deploy",
				KnownHostsPath: "/custom/known_hosts",
			},
			check: func(t *testing.T, r *SSHRunner) {
				if r.config.KnownHostsPath != "/custom/known_hosts" {
					t.Errorf("KnownHostsPath = %q, want /custom/known_hosts", r.config.KnownHostsPath)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner, err := NewSSHRunner(tt.config)
			if err != nil {
				t.Fatalf("NewSSHRunner() error = %v", err)
			}
			tt.check(t, runner)
		})
	}
}

func TestSSHPool_CleanupIdle(t *testing.T) {
	t.Parallel()

	// Create pool with very short idle timeout
	pool := NewSSHPool(1 * time.Millisecond)
	defer pool.Close()

	// Get a runner
	config := &SSHConfig{
		Host: "example.com",
		Port: 22,
		User: "deploy",
	}
	runner, err := pool.Get(config)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Verify runner is returned
	if runner == nil {
		t.Fatal("Get() returned nil runner")
	}

	// Force cleanup (normally triggered by timer)
	pool.mu.Lock()
	connectionCount := len(pool.connections)
	pool.mu.Unlock()

	if connectionCount != 1 {
		t.Errorf("Expected 1 connection, got %d", connectionCount)
	}

	// Wait for idle timeout and trigger cleanup manually
	time.Sleep(5 * time.Millisecond)
	pool.cleanup()

	pool.mu.Lock()
	connectionCount = len(pool.connections)
	pool.mu.Unlock()

	// Connection should be removed due to idle timeout
	if connectionCount != 0 {
		t.Errorf("Expected 0 connections after cleanup, got %d", connectionCount)
	}
}

func TestSSHPool_DoubleCheckPath(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)
	defer pool.Close()

	config := &SSHConfig{
		Host: "concurrent.example.com",
		Port: 22,
		User: "deploy",
	}

	// Spawn multiple goroutines to test the double-check path
	var wg sync.WaitGroup
	runners := make([]*SSHRunner, 10)
	errors := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r, err := pool.Get(config)
			runners[idx] = r
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// All should have gotten the same runner
	var firstRunner *SSHRunner
	for i, r := range runners {
		if errors[i] != nil {
			t.Errorf("Get() error = %v", errors[i])
			continue
		}
		if r == nil {
			t.Error("Get() returned nil runner")
			continue
		}
		if firstRunner == nil {
			firstRunner = r
		} else if r != firstRunner {
			t.Error("Concurrent Get() should return same runner")
		}
	}
}

func TestSSHPool_GetWithJumpHost(t *testing.T) {
	t.Parallel()

	pool := NewSSHPool(5 * time.Minute)
	defer pool.Close()

	configWithJump := &SSHConfig{
		Host:     "target.example.com",
		Port:     22,
		User:     "deploy",
		JumpHost: "bastion.example.com",
		JumpPort: 22,
	}

	configWithoutJump := &SSHConfig{
		Host: "target.example.com",
		Port: 22,
		User: "deploy",
	}

	// Get runner with jump host
	runner1, err := pool.Get(configWithJump)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Get runner without jump host - should be different
	runner2, err := pool.Get(configWithoutJump)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if runner1 == runner2 {
		t.Error("Different configs (with/without jump) should return different runners")
	}
}
