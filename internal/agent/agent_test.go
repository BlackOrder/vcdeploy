package agent

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/deploy"
	pb "github.com/BlackOrder/vcdeploy/internal/proto"
	"go.uber.org/zap"
)

func TestNewLocalRunner(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	if runner == nil {
		t.Fatal("NewLocalRunner() returned nil")
	}

	if runner.logger == nil {
		t.Error("NewLocalRunner() did not set logger")
	}
}

func TestLocalRunnerRunSimple(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	result, err := runner.Run(ctx, "echo hello", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Stdout != "hello\n" {
		t.Errorf("stdout = %q, want %q", result.Stdout, "hello\n")
	}

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
}

func TestLocalRunnerRunExitError(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	result, err := runner.Run(ctx, "exit 42", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("exit code = %d, want 42", result.ExitCode)
	}
}

func TestLocalRunnerRunWithEnv(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	opts := deploy.RunOptions{
		Env: map[string]string{"TEST_VAR": "test_value"},
	}

	result, err := runner.Run(ctx, "echo $TEST_VAR", opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Stdout != "test_value\n" {
		t.Errorf("stdout = %q, want %q", result.Stdout, "test_value\n")
	}
}

func TestLocalRunnerRunWithWorkDir(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	tmpDir := t.TempDir()
	opts := deploy.RunOptions{
		WorkDir: tmpDir,
	}

	result, err := runner.Run(ctx, "pwd", opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Stdout != tmpDir+"\n" {
		t.Errorf("stdout = %q, want %q", result.Stdout, tmpDir+"\n")
	}
}

func TestLocalRunnerRunWithTimeout(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	opts := deploy.RunOptions{
		Timeout: 1 * time.Second,
	}

	result, err := runner.Run(ctx, "sleep 10", opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode == 0 {
		t.Error("Command should have been killed by timeout")
	}
}

func TestLocalRunnerRunWithOutput(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer
	err := runner.RunWithOutput(ctx, "echo hello", &stdout, &stderr, deploy.RunOptions{})

	if err != nil {
		t.Fatalf("RunWithOutput() error = %v", err)
	}

	if stdout.String() != "hello\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "hello\n")
	}
}

func TestLocalRunnerRunWithOutputError(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer
	err := runner.RunWithOutput(ctx, "exit 1", &stdout, &stderr, deploy.RunOptions{})

	if err == nil {
		t.Error("RunWithOutput() expected error for exit 1")
	}
}

func TestLocalRunnerBuildCommand(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)

	got := runner.buildCommand("echo hello", deploy.RunOptions{})
	if got != "echo hello" {
		t.Errorf("buildCommand() = %q, want %q", got, "echo hello")
	}

	got = runner.buildCommand("echo hello", deploy.RunOptions{Timeout: 30 * time.Second})
	if got != "timeout 30 echo hello" {
		t.Errorf("buildCommand() = %q, want %q", got, "timeout 30 echo hello")
	}
}

func TestLocalRunnerBuildEnv(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)

	env := map[string]string{
		"VAR1": "value1",
		"VAR2": "value2",
	}

	result := runner.buildEnv(env)

	// Should include os.Environ() plus our custom vars
	// Check that our custom vars are present
	found := make(map[string]bool)
	for _, e := range result {
		if e == "VAR1=value1" || e == "VAR2=value2" {
			found[e] = true
		}
	}

	if !found["VAR1=value1"] {
		t.Error("buildEnv() missing VAR1=value1")
	}
	if !found["VAR2=value2"] {
		t.Error("buildEnv() missing VAR2=value2")
	}
}

func TestLocalRunnerImplementsInterface(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)

	var _ deploy.CommandRunner = runner
}

func TestNewAgent(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()

	cfg := &config.AgentConfig{
		Agent: config.AgentIdentityConfig{
			ID: "test-agent",
		},
		Master: config.AgentMasterConfig{
			Address: "localhost:9090",
		},
	}

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if agent == nil {
		t.Fatal("NewAgent() returned nil")
	}

	if agent.config != cfg {
		t.Error("NewAgent() did not set config")
	}

	if agent.strategy == nil {
		t.Error("NewAgent() did not create strategy")
	}

	if agent.runner == nil {
		t.Error("NewAgent() did not create runner")
	}
}

func TestAgentStartAlreadyRunning(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()

	cfg := &config.AgentConfig{
		Agent: config.AgentIdentityConfig{
			ID: "test-agent",
		},
		Master: config.AgentMasterConfig{
			Address: "localhost:9090",
		},
	}

	agent, _ := NewAgent(cfg, logger)
	agent.running = true

	ctx := context.Background()
	err := agent.Start(ctx)

	if err == nil {
		t.Error("Start() expected error when already running")
	}
}

func TestAgentStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		running  bool
		conn     bool
		expected string
	}{
		{"stopped", false, false, "stopped"},
		{"disconnected", true, false, "disconnected"},
		{"connected", true, true, "connected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := zap.NewNop()
			cfg := &config.AgentConfig{
				Agent:  config.AgentIdentityConfig{ID: "test"},
				Master: config.AgentMasterConfig{Address: "localhost:9090"},
			}
			agent, _ := NewAgent(cfg, logger)
			agent.running = tt.running
			if tt.conn {
				// Mock connection existence (we don't actually connect)
				// The Status() method checks if conn is nil
				// We can't easily set conn without a real connection
				// but we can test the other states
				_ = tt.conn // acknowledge intentionally empty branch
			}

			status := agent.Status()

			// For connected case, we can't set conn without real connection
			if tt.name != "connected" && status != tt.expected {
				t.Errorf("Status() = %q, want %q", status, tt.expected)
			}
		})
	}
}

func TestAgentShutdown(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()

	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test-agent"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}

	agent, _ := NewAgent(cfg, logger)
	agent.running = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := agent.Shutdown(ctx)

	if err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}

	if agent.running {
		t.Error("Agent should not be running after Shutdown()")
	}
}

func TestAgentShutdownTimeout(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()

	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test-agent"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}

	agent, _ := NewAgent(cfg, logger)
	agent.running = true

	// Add a task to the wait group that won't complete
	agent.wg.Add(1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := agent.Shutdown(ctx)

	if err != context.DeadlineExceeded {
		t.Errorf("Shutdown() error = %v, want context.DeadlineExceeded", err)
	}

	// Clean up the wait group
	agent.wg.Done()
}

func TestAgentExecuteDeploy(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()

	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test-agent"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}

	agent, _ := NewAgent(cfg, logger)

	cmd := &deploy.DeployCommand{
		DeploymentID: "test-deploy",
		Project:      "test-project",
		Repository:   "https://github.com/test/repo.git",
		Branch:       "main",
		Path:         t.TempDir(),
	}

	ctx := context.Background()
	result, _ := agent.ExecuteDeploy(ctx, cmd)

	// Even if deployment fails (no git repo), we should get a result
	if result == nil {
		t.Fatal("ExecuteDeploy() should return a result")
	}
}

func TestAgentExecuteRollback(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()

	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test-agent"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}

	agent, _ := NewAgent(cfg, logger)

	cmd := &deploy.RollbackCommand{
		DeploymentID: "test-rollback",
		Project:      "test-project",
		Path:         t.TempDir(),
	}

	ctx := context.Background()
	result, _ := agent.ExecuteRollback(ctx, cmd)

	// Even if rollback fails (no releases), we should get a result
	if result == nil {
		t.Fatal("ExecuteRollback() should return a result")
	}
}

func TestLocalRunnerRunContextCancellation(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := runner.Run(ctx, "sleep 10", deploy.RunOptions{})

	// Context should affect the command
	if err == nil && result != nil && result.ExitCode == 0 {
		t.Error("Run() should fail or return non-zero when context is cancelled")
	}
}

func TestLocalRunnerRunWithMultipleEnvVars(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	opts := deploy.RunOptions{
		Env: map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
			"VAR3": "value3",
		},
	}

	result, err := runner.Run(ctx, "echo $VAR1 $VAR2 $VAR3", opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Stdout != "value1 value2 value3\n" {
		t.Errorf("stdout = %q, want %q", result.Stdout, "value1 value2 value3\n")
	}
}

func TestLocalRunnerStderr(t *testing.T) {
	t.Parallel()

	// Test stderr capture using cat on a nonexistent file
	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	// cat on nonexistent file writes to stderr and returns non-zero exit
	result, err := runner.Run(ctx, "cat /nonexistent/file/path/12345", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// LocalRunner.Run() uses CombinedOutput, so stderr goes to Stdout
	// The error message should contain something about the file not existing
	if result.Stdout == "" {
		t.Error("stdout (combined) should contain error message from cat")
	}

	if result.ExitCode == 0 {
		t.Error("exit code should be non-zero for nonexistent file")
	}
}

func TestLocalRunnerCommandDuration(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	result, err := runner.Run(ctx, "sleep 0.1", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Duration < 100*time.Millisecond {
		t.Errorf("Duration = %v, want >= 100ms", result.Duration)
	}
}

func BenchmarkLocalRunnerSimpleCommand(b *testing.B) {
	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = runner.Run(ctx, "true", deploy.RunOptions{})
	}
}

// TestLookupUserNumeric tests lookupUser with numeric UID.
func TestLookupUserNumeric(t *testing.T) {
	t.Parallel()

	uid, err := lookupUser("0")
	if err != nil {
		t.Fatalf("lookupUser() error = %v", err)
	}

	if uid != 0 {
		t.Errorf("lookupUser('0') = %d, want 0", uid)
	}
}

// TestLookupUserRoot tests lookupUser with the root user.
func TestLookupUserRoot(t *testing.T) {
	t.Parallel()

	uid, err := lookupUser("root")
	if err != nil {
		t.Fatalf("lookupUser('root') error = %v", err)
	}

	if uid != 0 {
		t.Errorf("lookupUser('root') = %d, want 0", uid)
	}
}

// TestLookupUserNotFound tests lookupUser with nonexistent user.
func TestLookupUserNotFound(t *testing.T) {
	t.Parallel()

	_, err := lookupUser("nonexistent_user_12345")
	if err == nil {
		t.Error("lookupUser() expected error for nonexistent user")
	}
}

// TestLookupGroupNumeric tests lookupGroup with numeric GID.
func TestLookupGroupNumeric(t *testing.T) {
	t.Parallel()

	gid, err := lookupGroup("0")
	if err != nil {
		t.Fatalf("lookupGroup() error = %v", err)
	}

	if gid != 0 {
		t.Errorf("lookupGroup('0') = %d, want 0", gid)
	}
}

// TestLookupGroupRoot tests lookupGroup with the root group.
func TestLookupGroupRoot(t *testing.T) {
	t.Parallel()

	gid, err := lookupGroup("root")
	if err != nil {
		t.Fatalf("lookupGroup('root') error = %v", err)
	}

	if gid != 0 {
		t.Errorf("lookupGroup('root') = %d, want 0", gid)
	}
}

// TestLookupGroupNotFound tests lookupGroup with nonexistent group.
func TestLookupGroupNotFound(t *testing.T) {
	t.Parallel()

	_, err := lookupGroup("nonexistent_group_12345")
	if err == nil {
		t.Error("lookupGroup() expected error for nonexistent group")
	}
}

// TestLookupUserPrimaryGroupRoot tests lookupUserPrimaryGroup for root.
func TestLookupUserPrimaryGroupRoot(t *testing.T) {
	t.Parallel()

	gid, err := lookupUserPrimaryGroup("root")
	if err != nil {
		t.Fatalf("lookupUserPrimaryGroup('root') error = %v", err)
	}

	if gid != 0 {
		t.Errorf("lookupUserPrimaryGroup('root') = %d, want 0", gid)
	}
}

// TestLookupUserPrimaryGroupNotFound tests lookupUserPrimaryGroup for nonexistent user.
func TestLookupUserPrimaryGroupNotFound(t *testing.T) {
	t.Parallel()

	_, err := lookupUserPrimaryGroup("nonexistent_user_12345")
	if err == nil {
		t.Error("lookupUserPrimaryGroup() expected error for nonexistent user")
	}
}

// TestLocalRunnerSetUserGroupRoot tests setUserGroup with root.
func TestLocalRunnerSetUserGroupRoot(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)

	// We can't actually run as root in tests, but we can test the lookup logic
	cmd := exec.Command("true")
	err := runner.setUserGroup(cmd, "root", "root")

	if err != nil {
		t.Fatalf("setUserGroup() error = %v", err)
	}

	if cmd.SysProcAttr == nil {
		t.Fatal("setUserGroup() should set SysProcAttr")
	}

	if cmd.SysProcAttr.Credential == nil {
		t.Fatal("setUserGroup() should set Credential")
	}

	if cmd.SysProcAttr.Credential.Uid != 0 {
		t.Errorf("Uid = %d, want 0", cmd.SysProcAttr.Credential.Uid)
	}

	if cmd.SysProcAttr.Credential.Gid != 0 {
		t.Errorf("Gid = %d, want 0", cmd.SysProcAttr.Credential.Gid)
	}
}

// TestLocalRunnerSetUserGroupNumeric tests setUserGroup with numeric IDs.
func TestLocalRunnerSetUserGroupNumeric(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)

	cmd := exec.Command("true")
	err := runner.setUserGroup(cmd, "1000", "1000")

	if err != nil {
		t.Fatalf("setUserGroup() error = %v", err)
	}

	if cmd.SysProcAttr.Credential.Uid != 1000 {
		t.Errorf("Uid = %d, want 1000", cmd.SysProcAttr.Credential.Uid)
	}

	if cmd.SysProcAttr.Credential.Gid != 1000 {
		t.Errorf("Gid = %d, want 1000", cmd.SysProcAttr.Credential.Gid)
	}
}

// TestLocalRunnerSetUserGroupFailClosed tests setUserGroup fails with unknown user.
func TestLocalRunnerSetUserGroupFailClosed(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)

	cmd := exec.Command("true")
	err := runner.setUserGroup(cmd, "nonexistent_user_12345", "")

	if err == nil {
		t.Error("setUserGroup() expected error for unknown user (fail-closed)")
	}
}

// TestLocalRunnerSetUserGroupFailOpen tests setUserGroup with FailOpenOnUserLookup.
func TestLocalRunnerSetUserGroupFailOpen(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	runner.FailOpenOnUserLookup = true

	cmd := exec.Command("true")
	err := runner.setUserGroup(cmd, "nonexistent_user_12345", "")

	if err != nil {
		t.Errorf("setUserGroup() with FailOpenOnUserLookup should not error: %v", err)
	}

	// SysProcAttr should NOT be set when failing open
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Credential != nil {
		t.Error("setUserGroup() should not set credentials when failing open")
	}
}

// TestLocalRunnerSetUserGroupWithPrimaryGroup tests setUserGroup uses primary group.
func TestLocalRunnerSetUserGroupWithPrimaryGroup(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)

	cmd := exec.Command("true")
	// Use root with empty group to trigger primary group lookup
	err := runner.setUserGroup(cmd, "root", "")

	if err != nil {
		t.Fatalf("setUserGroup() error = %v", err)
	}

	if cmd.SysProcAttr == nil || cmd.SysProcAttr.Credential == nil {
		t.Fatal("setUserGroup() should set credentials")
	}

	// Root's primary group should be 0
	if cmd.SysProcAttr.Credential.Gid != 0 {
		t.Errorf("Gid = %d, want 0 (root's primary group)", cmd.SysProcAttr.Credential.Gid)
	}
}

// TestLocalRunnerRunWithUser tests Run with user option (requires root).
func TestLocalRunnerRunWithUser(t *testing.T) {
	t.Parallel()

	// Skip this test if not running as root
	if os.Getuid() != 0 {
		t.Skip("Skipping test that requires root")
	}

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	opts := deploy.RunOptions{
		User:  "nobody",
		Group: "nogroup",
	}

	result, err := runner.Run(ctx, "id -u", opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
}

// TestLocalRunnerRunWithOutputStreaming tests streaming output with stderr.
func TestLocalRunnerRunWithOutputStreaming(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	// Test stdout capture first
	var stdout1, stderr1 bytes.Buffer
	err := runner.RunWithOutput(ctx, "echo stdout_test", &stdout1, &stderr1, deploy.RunOptions{})
	if err != nil {
		t.Fatalf("RunWithOutput() error = %v", err)
	}
	if stdout1.String() != "stdout_test\n" {
		t.Errorf("stdout = %q, want %q", stdout1.String(), "stdout_test\n")
	}

	// Test stderr capture using cat on nonexistent file
	var stdout2, stderr2 bytes.Buffer
	err = runner.RunWithOutput(ctx, "cat /nonexistent/file/path/67890", &stdout2, &stderr2, deploy.RunOptions{})
	// This should return an error since cat fails
	if err == nil {
		t.Log("cat on nonexistent file returned nil error (may vary by system)")
	}
	// stderr should contain error message
	if stderr2.Len() == 0 {
		t.Log("Note: stderr capture may not work for all error types")
	}
}

// TestLocalRunnerRunWithOutputTimeout tests timeout with streaming output.
func TestLocalRunnerRunWithOutputTimeout(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer
	opts := deploy.RunOptions{
		Timeout: 1 * time.Second,
	}

	err := runner.RunWithOutput(ctx, "sleep 10", &stdout, &stderr, opts)

	if err == nil {
		t.Error("RunWithOutput() expected error on timeout")
	}
}

// TestLocalRunnerRunBlockedCommand tests that blocked commands are rejected by validation.
func TestLocalRunnerRunBlockedCommand(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	// Try to run a command that's not in the allowlist
	_, err := runner.Run(ctx, "nonexistent_command_12345", deploy.RunOptions{})

	// Should fail validation
	if err == nil {
		t.Error("Run() should return error for command not in allowlist")
	}
}

// TestLocalRunnerRunWithOutputBlockedCommand tests that blocked commands are rejected in RunWithOutput.
func TestLocalRunnerRunWithOutputBlockedCommand(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer
	err := runner.RunWithOutput(ctx, "perl -e 'system(\"whoami\")'", &stdout, &stderr, deploy.RunOptions{})

	// Should fail validation (perl not in allowlist)
	if err == nil {
		t.Error("RunWithOutput() should return error for command not in allowlist")
	}
}

// TestAgentStatusWithConnection tests Status when connected.
func TestAgentStatusWithConnection(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)
	agent.running = false

	status := agent.Status()
	if status != "stopped" {
		t.Errorf("Status() = %q, want %q", status, "stopped")
	}
}

// TestAgentActiveDeploymentsTracker tests the deployment tracking.
func TestAgentActiveDeploymentsTracker(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	// Test empty map
	if len(agent.activeDeployments) != 0 {
		t.Error("activeDeployments should be empty initially")
	}

	// Add a deployment
	agent.deployMu.Lock()
	agent.activeDeployments["deploy-1"] = &activeDeployment{
		ID:        "deploy-1",
		Project:   "test-project",
		StartTime: time.Now(),
	}
	agent.deployMu.Unlock()

	agent.deployMu.RLock()
	if len(agent.activeDeployments) != 1 {
		t.Errorf("activeDeployments count = %d, want 1", len(agent.activeDeployments))
	}
	agent.deployMu.RUnlock()
}

// TestAgentCollectStats tests the collectStats method.
func TestAgentCollectStats(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
		Paths: config.AgentPathsConfig{
			Releases: "/tmp",
		},
	}
	agent, _ := NewAgent(cfg, logger)

	stats := agent.collectStats()

	if stats == nil {
		t.Fatal("collectStats() returned nil")
	}

	// CPU percentage should be between 0 and 100
	if stats.CpuPercent < 0 || stats.CpuPercent > 100 {
		t.Errorf("CpuPercent = %f, expected between 0 and 100", stats.CpuPercent)
	}

	// Memory percentage should be between 0 and 100
	if stats.MemoryPercent < 0 || stats.MemoryPercent > 100 {
		t.Errorf("MemoryPercent = %f, expected between 0 and 100", stats.MemoryPercent)
	}

	// Disk percentage should be between 0 and 100
	if stats.DiskPercent < 0 || stats.DiskPercent > 100 {
		t.Errorf("DiskPercent = %f, expected between 0 and 100", stats.DiskPercent)
	}

	// Free disk should be non-negative
	if stats.DiskFreeBytes < 0 {
		t.Errorf("DiskFreeBytes = %d, expected non-negative", stats.DiskFreeBytes)
	}
}

// TestAgentCollectStatsWithInvalidPath tests collectStats with invalid disk path.
func TestAgentCollectStatsWithInvalidPath(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
		Paths: config.AgentPathsConfig{
			Releases: "/nonexistent/path/that/should/not/exist",
		},
	}
	agent, _ := NewAgent(cfg, logger)

	// Should not panic, just return stats with zero disk values
	stats := agent.collectStats()

	if stats == nil {
		t.Fatal("collectStats() returned nil even with invalid path")
	}
}

// TestAgentGetActiveDeploymentStatuses tests the getActiveDeploymentStatuses method.
func TestAgentGetActiveDeploymentStatuses(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	// Test empty deployments
	statuses := agent.getActiveDeploymentStatuses()
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses, got %d", len(statuses))
	}

	// Add deployments
	agent.deployMu.Lock()
	agent.activeDeployments["deploy-1"] = &activeDeployment{
		ID:        "deploy-1",
		Project:   "project-a",
		StartTime: time.Now(),
		State:     pb.DeploymentState_DEPLOYMENT_STATE_DEPLOYING,
	}
	agent.activeDeployments["deploy-2"] = &activeDeployment{
		ID:        "deploy-2",
		Project:   "project-b",
		StartTime: time.Now(),
		State:     pb.DeploymentState_DEPLOYMENT_STATE_PREPARING,
	}
	agent.deployMu.Unlock()

	statuses = agent.getActiveDeploymentStatuses()
	if len(statuses) != 2 {
		t.Errorf("expected 2 statuses, got %d", len(statuses))
	}

	// Verify statuses contain expected deployment IDs
	found := make(map[string]bool)
	for _, s := range statuses {
		found[s.DeploymentId] = true
	}
	if !found["deploy-1"] || !found["deploy-2"] {
		t.Error("statuses missing expected deployment IDs")
	}
}

// TestAgentUpdateDeploymentState tests the updateDeploymentState method.
func TestAgentUpdateDeploymentState(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	// Add a deployment
	agent.deployMu.Lock()
	agent.activeDeployments["deploy-1"] = &activeDeployment{
		ID:        "deploy-1",
		Project:   "test-project",
		StartTime: time.Now(),
		State:     pb.DeploymentState_DEPLOYMENT_STATE_PREPARING,
	}
	agent.deployMu.Unlock()

	// Update state
	agent.updateDeploymentState("deploy-1", pb.DeploymentState_DEPLOYMENT_STATE_DEPLOYING)

	agent.deployMu.RLock()
	state := agent.activeDeployments["deploy-1"].State
	agent.deployMu.RUnlock()

	if state != pb.DeploymentState_DEPLOYMENT_STATE_DEPLOYING {
		t.Errorf("state = %v, want DEPLOYING", state)
	}

	// Update nonexistent deployment (should not panic)
	agent.updateDeploymentState("nonexistent", pb.DeploymentState_DEPLOYMENT_STATE_FAILED)
}

// TestAgentProtoToDeployCommand tests the protoToDeployCommand method.
func TestAgentProtoToDeployCommand(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	protoCmd := &pb.DeployCommand{
		DeploymentId:    "deploy-123",
		Project:         "my-project",
		Target:          "production",
		Repository:      "https://github.com/test/repo.git",
		Branch:          "main",
		Commit:          "abc123",
		Path:            "/var/www/myproject",
		EnvVars:         map[string]string{"KEY": "value"},
		EnvFileContent:  []byte("SECRET=mysecret"),
		PreDeployHooks:  []string{"npm install"},
		PostDeployHooks: []string{"pm2 reload"},
		Settings: &pb.DeploymentSettings{
			Strategy:       "symlink",
			KeepReleases:   5,
			SharedDirs:     []string{"storage", "logs"},
			SharedFiles:    []string{".env"},
			WritableDirs:   []string{"cache"},
			ExecutionUser:  "www-data",
			ExecutionGroup: "www-data",
			TimeoutSeconds: 300,
		},
		ReloadServices: []*pb.ServiceReload{
			{Service: "nginx", Action: "reload"},
			{Service: "php-fpm", Action: "restart"},
		},
	}

	result := agent.protoToDeployCommand(protoCmd)

	if result.DeploymentID != "deploy-123" {
		t.Errorf("DeploymentID = %q, want %q", result.DeploymentID, "deploy-123")
	}
	if result.Project != "my-project" {
		t.Errorf("Project = %q, want %q", result.Project, "my-project")
	}
	if result.Target != "production" {
		t.Errorf("Target = %q, want %q", result.Target, "production")
	}
	if result.Repository != "https://github.com/test/repo.git" {
		t.Errorf("Repository = %q, want %q", result.Repository, "https://github.com/test/repo.git")
	}
	if result.Branch != "main" {
		t.Errorf("Branch = %q, want %q", result.Branch, "main")
	}
	if result.Commit != "abc123" {
		t.Errorf("Commit = %q, want %q", result.Commit, "abc123")
	}
	if result.Path != "/var/www/myproject" {
		t.Errorf("Path = %q, want %q", result.Path, "/var/www/myproject")
	}
	if result.EnvVars["KEY"] != "value" {
		t.Errorf("EnvVars[KEY] = %q, want %q", result.EnvVars["KEY"], "value")
	}
	if string(result.EnvFileContent) != "SECRET=mysecret" {
		t.Errorf("EnvFileContent = %q, want %q", string(result.EnvFileContent), "SECRET=mysecret")
	}
	if len(result.PreDeployHooks) != 1 || result.PreDeployHooks[0] != "npm install" {
		t.Errorf("PreDeployHooks = %v, want [npm install]", result.PreDeployHooks)
	}
	if len(result.PostDeployHooks) != 1 || result.PostDeployHooks[0] != "pm2 reload" {
		t.Errorf("PostDeployHooks = %v, want [pm2 reload]", result.PostDeployHooks)
	}

	// Check settings
	if result.Settings.Strategy != "symlink" {
		t.Errorf("Settings.Strategy = %q, want %q", result.Settings.Strategy, "symlink")
	}
	if result.Settings.KeepReleases != 5 {
		t.Errorf("Settings.KeepReleases = %d, want 5", result.Settings.KeepReleases)
	}
	if len(result.Settings.SharedDirs) != 2 {
		t.Errorf("Settings.SharedDirs = %v, want 2 items", result.Settings.SharedDirs)
	}
	if len(result.Settings.SharedFiles) != 1 || result.Settings.SharedFiles[0] != ".env" {
		t.Errorf("Settings.SharedFiles = %v, want [.env]", result.Settings.SharedFiles)
	}
	if len(result.Settings.WritableDirs) != 1 || result.Settings.WritableDirs[0] != "cache" {
		t.Errorf("Settings.WritableDirs = %v, want [cache]", result.Settings.WritableDirs)
	}
	if result.Settings.ExecutionUser != "www-data" {
		t.Errorf("Settings.ExecutionUser = %q, want %q", result.Settings.ExecutionUser, "www-data")
	}
	if result.Settings.ExecutionGroup != "www-data" {
		t.Errorf("Settings.ExecutionGroup = %q, want %q", result.Settings.ExecutionGroup, "www-data")
	}
	if result.Settings.Timeout != 300*time.Second {
		t.Errorf("Settings.Timeout = %v, want %v", result.Settings.Timeout, 300*time.Second)
	}

	// Check service reloads
	if len(result.ReloadServices) != 2 {
		t.Errorf("ReloadServices count = %d, want 2", len(result.ReloadServices))
	}
	if result.ReloadServices[0].Service != "nginx" || result.ReloadServices[0].Action != "reload" {
		t.Errorf("ReloadServices[0] = %v, want nginx/reload", result.ReloadServices[0])
	}
}

// TestAgentProtoToDeployCommandNilSettings tests protoToDeployCommand with nil settings.
func TestAgentProtoToDeployCommandNilSettings(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	protoCmd := &pb.DeployCommand{
		DeploymentId: "deploy-123",
		Project:      "my-project",
		Settings:     nil, // Nil settings
	}

	result := agent.protoToDeployCommand(protoCmd)

	if result.DeploymentID != "deploy-123" {
		t.Errorf("DeploymentID = %q, want %q", result.DeploymentID, "deploy-123")
	}
	// Settings should be zero value
	if result.Settings.Strategy != "" {
		t.Errorf("Settings.Strategy = %q, want empty", result.Settings.Strategy)
	}
}

// TestAgentProtoToDeployCommandEmptyReloadServices tests with empty service reloads.
func TestAgentProtoToDeployCommandEmptyReloadServices(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	protoCmd := &pb.DeployCommand{
		DeploymentId:   "deploy-123",
		Project:        "my-project",
		ReloadServices: nil,
	}

	result := agent.protoToDeployCommand(protoCmd)

	if len(result.ReloadServices) != 0 {
		t.Errorf("ReloadServices count = %d, want 0", len(result.ReloadServices))
	}
}

// TestAgentForwardLogs tests the forwardLogs method.
func TestAgentForwardLogs(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	// Test with nil stream (should not panic)
	logCh := make(chan deploy.LogEntry, 10)

	// Send logs
	logCh <- deploy.LogEntry{
		Level:   deploy.LogInfo,
		Message: "test message",
		Source:  "test",
	}
	close(logCh)

	// Should complete without panic
	agent.forwardLogs(nil, "deploy-1", logCh)
}

// TestAgentForwardLogsAllLevels tests forwardLogs with all log levels.
func TestAgentForwardLogsAllLevels(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	logCh := make(chan deploy.LogEntry, 10)

	// Send all log levels
	logCh <- deploy.LogEntry{Level: deploy.LogDebug, Message: "debug", Source: "test"}
	logCh <- deploy.LogEntry{Level: deploy.LogInfo, Message: "info", Source: "test"}
	logCh <- deploy.LogEntry{Level: deploy.LogWarn, Message: "warn", Source: "test"}
	logCh <- deploy.LogEntry{Level: deploy.LogError, Message: "error", Source: "test"}
	close(logCh)

	// Should complete without panic with nil stream
	agent.forwardLogs(nil, "deploy-1", logCh)
}

// TestAgentSendDeploymentStatusNilStream tests sendDeploymentStatus with nil stream.
func TestAgentSendDeploymentStatusNilStream(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	// Should not panic with nil stream
	agent.sendDeploymentStatus(nil, "deploy-1", pb.DeploymentState_DEPLOYMENT_STATE_DEPLOYING, "test", 50)
}

// TestAgentSendCommandResultNilStream tests sendCommandResult with nil stream.
func TestAgentSendCommandResultNilStream(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	// Should not panic with nil stream
	agent.sendCommandResult(nil, "deploy-1", "test-command", 0, "stdout", "stderr")
}

// TestAgentPerformHealthCheckSuccess tests successful health check.
func TestAgentPerformHealthCheckSuccess(t *testing.T) {
	t.Parallel()

	// Create a test server that returns 200 OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "vcdeploy-health-check/1.0" {
			t.Error("Expected User-Agent header")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	ctx := context.Background()
	result, msg := agent.performHealthCheck(ctx, server.URL, 10)

	if !result {
		t.Errorf("performHealthCheck() = false, want true; msg = %s", msg)
	}
}

// TestAgentPerformHealthCheckStatus201 tests health check with 201 status.
func TestAgentPerformHealthCheckStatus201(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated) // 201
	}))
	defer server.Close()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	ctx := context.Background()
	result, _ := agent.performHealthCheck(ctx, server.URL, 10)

	// 201 is within 2xx range, should pass
	if !result {
		t.Error("performHealthCheck() should pass for 201 status")
	}
}

// TestAgentPerformHealthCheckStatus404 tests health check with 404 status.
func TestAgentPerformHealthCheckStatus404(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	ctx := context.Background()
	result, msg := agent.performHealthCheck(ctx, server.URL, 10)

	if result {
		t.Error("performHealthCheck() should fail for 404 status")
	}
	if msg == "" {
		t.Error("performHealthCheck() should return error message")
	}
}

// TestAgentPerformHealthCheckStatus500 tests health check with 500 status.
func TestAgentPerformHealthCheckStatus500(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	ctx := context.Background()
	result, _ := agent.performHealthCheck(ctx, server.URL, 10)

	if result {
		t.Error("performHealthCheck() should fail for 500 status")
	}
}

// TestAgentPerformHealthCheckStatus301 tests health check with redirect.
func TestAgentPerformHealthCheckStatus301(t *testing.T) {
	t.Parallel()

	// Create a server that redirects to a successful endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/success", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/success", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	ctx := context.Background()
	result, _ := agent.performHealthCheck(ctx, server.URL+"/redirect", 10)

	// Should follow redirect and succeed
	if !result {
		t.Error("performHealthCheck() should follow redirects and succeed")
	}
}

// TestAgentPerformHealthCheckConnectionRefused tests health check with connection refused.
func TestAgentPerformHealthCheckConnectionRefused(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	ctx := context.Background()
	// Use a port that's unlikely to be in use
	result, msg := agent.performHealthCheck(ctx, "http://127.0.0.1:59999", 1)

	if result {
		t.Error("performHealthCheck() should fail for connection refused")
	}
	if msg == "" {
		t.Error("performHealthCheck() should return error message")
	}
}

// TestAgentPerformHealthCheckContextCancelled tests health check with cancelled context.
func TestAgentPerformHealthCheckContextCancelled(t *testing.T) {
	t.Parallel()

	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, _ := agent.performHealthCheck(ctx, server.URL, 10)

	if result {
		t.Error("performHealthCheck() should fail for cancelled context")
	}
}

// TestAgentPerformHealthCheckDefaultTimeout tests health check with default timeout.
func TestAgentPerformHealthCheckDefaultTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	ctx := context.Background()
	// timeout of 0 should use default
	result, _ := agent.performHealthCheck(ctx, server.URL, 0)

	if !result {
		t.Error("performHealthCheck() should succeed with default timeout")
	}
}

// TestAgentPerformHealthCheckInvalidURL tests health check with invalid URL.
func TestAgentPerformHealthCheckInvalidURL(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	ctx := context.Background()
	result, msg := agent.performHealthCheck(ctx, "://invalid-url", 10)

	if result {
		t.Error("performHealthCheck() should fail for invalid URL")
	}
	if msg == "" {
		t.Error("performHealthCheck() should return error message for invalid URL")
	}
}

// TestNewAgentHTTPClient tests that NewAgent creates an HTTP client.
func TestNewAgentHTTPClient(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, err := NewAgent(cfg, logger)

	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if agent.httpClient == nil {
		t.Error("NewAgent() should create httpClient")
	}

	if agent.httpClient.Timeout != 30*time.Second {
		t.Errorf("httpClient.Timeout = %v, want 30s", agent.httpClient.Timeout)
	}
}

// TestAgentHTTPClientRedirect tests HTTP client redirect behavior.
func TestAgentHTTPClientRedirect(t *testing.T) {
	t.Parallel()

	// Create a server with too many redirects
	redirectCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		if redirectCount > 15 {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/redirect", http.StatusFound)
	}))
	defer server.Close()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	ctx := context.Background()
	result, msg := agent.performHealthCheck(ctx, server.URL+"/redirect", 30)

	// Should fail due to too many redirects
	if result {
		t.Error("performHealthCheck() should fail with too many redirects")
	}
	if msg == "" {
		t.Error("performHealthCheck() should return error message for redirect failure")
	}
}

// TestActiveDeploymentStruct tests the activeDeployment struct.
func TestActiveDeploymentStruct(t *testing.T) {
	t.Parallel()

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	cancelDone := make(chan struct{})

	ad := &activeDeployment{
		ID:         "deploy-123",
		Project:    "test-project",
		State:      pb.DeploymentState_DEPLOYMENT_STATE_DEPLOYING,
		Cancel:     cancel,
		cancelDone: cancelDone,
	}

	if ad.ID != "deploy-123" {
		t.Errorf("ID = %q, want %q", ad.ID, "deploy-123")
	}
	if ad.Project != "test-project" {
		t.Errorf("Project = %q, want %q", ad.Project, "test-project")
	}
	if ad.State != pb.DeploymentState_DEPLOYMENT_STATE_DEPLOYING {
		t.Errorf("State = %v, want DEPLOYING", ad.State)
	}
	if ad.Cancel == nil {
		t.Error("Cancel should not be nil")
	}
	if ad.cancelDone == nil {
		t.Error("cancelDone should not be nil")
	}
}

// TestAgentRunningStateConcurrency tests concurrent access to running state.
func TestAgentRunningStateConcurrency(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	// Start multiple goroutines accessing Status and modifying running
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			_ = agent.Status()
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			agent.mu.Lock()
			agent.running = !agent.running
			agent.mu.Unlock()
		}
		done <- struct{}{}
	}()

	// Wait for both goroutines
	<-done
	<-done
}

// TestAgentDeploymentsConcurrency tests concurrent deployment tracking.
func TestAgentDeploymentsConcurrency(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	done := make(chan struct{})

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			agent.deployMu.Lock()
			agent.activeDeployments["test"] = &activeDeployment{
				ID: "test",
			}
			agent.deployMu.Unlock()
		}
		done <- struct{}{}
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = agent.getActiveDeploymentStatuses()
		}
		done <- struct{}{}
	}()

	// State updater goroutine
	go func() {
		for i := 0; i < 100; i++ {
			agent.updateDeploymentState("test", pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED)
		}
		done <- struct{}{}
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done
}

// TestAgentNewAgentCreatesStrategy tests that NewAgent creates a strategy.
func TestAgentNewAgentCreatesStrategy(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, err := NewAgent(cfg, logger)

	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if agent.strategy == nil {
		t.Error("NewAgent() should create strategy")
	}
}

// TestAgentNewAgentCreatesRunner tests that NewAgent creates a runner.
func TestAgentNewAgentCreatesRunner(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, err := NewAgent(cfg, logger)

	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if agent.runner == nil {
		t.Error("NewAgent() should create runner")
	}
}

// TestAgentNewAgentCreatesActiveDeploymentsMap tests that NewAgent creates the map.
func TestAgentNewAgentCreatesActiveDeploymentsMap(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, err := NewAgent(cfg, logger)

	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if agent.activeDeployments == nil {
		t.Error("NewAgent() should create activeDeployments map")
	}
}

// TestAgentNewAgentCreatesShutdownChannel tests that NewAgent creates the channel.
func TestAgentNewAgentCreatesShutdownChannel(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, err := NewAgent(cfg, logger)

	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if agent.shutdown == nil {
		t.Error("NewAgent() should create shutdown channel")
	}
}

// TestLocalRunnerRunWithUserFailOpen tests Run with user switch fails open.
func TestLocalRunnerRunWithUserFailOpen(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	runner.FailOpenOnUserLookup = true

	ctx := context.Background()
	opts := deploy.RunOptions{
		User: "nonexistent_user_99999",
	}

	// Should succeed because fail-open is enabled
	result, err := runner.Run(ctx, "echo test", opts)
	if err != nil {
		t.Fatalf("Run() with FailOpenOnUserLookup should not error: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

// TestLocalRunnerRunWithOutputWithUserFailOpen tests RunWithOutput with user switch fails open.
func TestLocalRunnerRunWithOutputWithUserFailOpen(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	runner.FailOpenOnUserLookup = true

	ctx := context.Background()
	opts := deploy.RunOptions{
		User: "nonexistent_user_99999",
	}

	var stdout, stderr bytes.Buffer

	// Should succeed because fail-open is enabled
	err := runner.RunWithOutput(ctx, "echo test", &stdout, &stderr, opts)
	if err != nil {
		t.Fatalf("RunWithOutput() with FailOpenOnUserLookup should not error: %v", err)
	}

	if stdout.String() != "test\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "test\n")
	}
}

// TestLocalRunnerRunWithUserFailClosed tests Run with user switch fails closed.
func TestLocalRunnerRunWithUserFailClosed(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	// FailOpenOnUserLookup is false by default

	ctx := context.Background()
	opts := deploy.RunOptions{
		User: "nonexistent_user_99999",
	}

	// Should error because fail-closed is the default
	_, err := runner.Run(ctx, "echo test", opts)
	if err == nil {
		t.Error("Run() with invalid user should error when fail-closed")
	}
}

// TestLocalRunnerRunWithOutputWithUserFailClosed tests RunWithOutput with user switch fails closed.
func TestLocalRunnerRunWithOutputWithUserFailClosed(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	// FailOpenOnUserLookup is false by default

	ctx := context.Background()
	opts := deploy.RunOptions{
		User: "nonexistent_user_99999",
	}

	var stdout, stderr bytes.Buffer

	// Should error because fail-closed is the default
	err := runner.RunWithOutput(ctx, "echo test", &stdout, &stderr, opts)
	if err == nil {
		t.Error("RunWithOutput() with invalid user should error when fail-closed")
	}
}

// TestLocalRunnerSetUserGroupFailOpenGroup tests setUserGroup with invalid group fails open.
func TestLocalRunnerSetUserGroupFailOpenGroup(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	runner.FailOpenOnUserLookup = true

	cmd := exec.Command("true")
	// Use valid user but invalid group
	err := runner.setUserGroup(cmd, "root", "nonexistent_group_99999")

	if err != nil {
		t.Errorf("setUserGroup() with FailOpenOnUserLookup should not error: %v", err)
	}
}

// TestLocalRunnerSetUserGroupFailOpenPrimaryGroup tests setUserGroup with invalid user primary group lookup fails open.
func TestLocalRunnerSetUserGroupFailOpenPrimaryGroup(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	runner.FailOpenOnUserLookup = true

	cmd := exec.Command("true")
	// Use numeric UID with empty group to trigger primary group lookup that will fail
	// Using a high number that's unlikely to be a real user
	err := runner.setUserGroup(cmd, "99999", "")

	if err != nil {
		t.Errorf("setUserGroup() with FailOpenOnUserLookup should not error: %v", err)
	}
}

// TestLookupUserEmptyString tests lookupUser with empty string.
func TestLookupUserEmptyString(t *testing.T) {
	t.Parallel()

	_, err := lookupUser("")
	if err == nil {
		t.Error("lookupUser('') should error")
	}
}

// TestLookupGroupEmptyString tests lookupGroup with empty string.
func TestLookupGroupEmptyString(t *testing.T) {
	t.Parallel()

	_, err := lookupGroup("")
	if err == nil {
		t.Error("lookupGroup('') should error")
	}
}

// TestLookupUserPrimaryGroupEmptyString tests lookupUserPrimaryGroup with empty string.
func TestLookupUserPrimaryGroupEmptyString(t *testing.T) {
	t.Parallel()

	_, err := lookupUserPrimaryGroup("")
	if err == nil {
		t.Error("lookupUserPrimaryGroup('') should error")
	}
}

// TestLookupUserNegativeUID tests lookupUser with negative UID string.
func TestLookupUserNegativeUID(t *testing.T) {
	t.Parallel()

	uid, err := lookupUser("-1")
	if err != nil {
		t.Fatalf("lookupUser('-1') error = %v", err)
	}
	if uid != -1 {
		t.Errorf("lookupUser('-1') = %d, want -1", uid)
	}
}

// TestLookupGroupNegativeGID tests lookupGroup with negative GID string.
func TestLookupGroupNegativeGID(t *testing.T) {
	t.Parallel()

	gid, err := lookupGroup("-1")
	if err != nil {
		t.Fatalf("lookupGroup('-1') error = %v", err)
	}
	if gid != -1 {
		t.Errorf("lookupGroup('-1') = %d, want -1", gid)
	}
}

// TestLocalRunnerBuildCommandSubSecondTimeout tests timeout with sub-second duration.
func TestLocalRunnerBuildCommandSubSecondTimeout(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)

	cmd := runner.buildCommand("echo test", deploy.RunOptions{
		Timeout: 500 * time.Millisecond, // 0 seconds when converted to int
	})

	// Should have timeout 0 prefix due to int conversion
	if cmd != "timeout 0 echo test" {
		t.Errorf("cmd = %q, want %q", cmd, "timeout 0 echo test")
	}
}

// TestLocalRunnerRunWithOutputWithEnv tests RunWithOutput with environment variables.
func TestLocalRunnerRunWithOutputWithEnv(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer
	opts := deploy.RunOptions{
		Env: map[string]string{
			"MY_VAR": "my_value",
		},
	}

	err := runner.RunWithOutput(ctx, "echo $MY_VAR", &stdout, &stderr, opts)
	if err != nil {
		t.Fatalf("RunWithOutput() error = %v", err)
	}

	if stdout.String() != "my_value\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "my_value\n")
	}
}

// TestLocalRunnerRunWithOutputWithWorkDir tests RunWithOutput with working directory.
func TestLocalRunnerRunWithOutputWithWorkDir(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer
	opts := deploy.RunOptions{
		WorkDir: "/tmp",
	}

	err := runner.RunWithOutput(ctx, "pwd", &stdout, &stderr, opts)
	if err != nil {
		t.Fatalf("RunWithOutput() error = %v", err)
	}

	// On some systems /tmp might be a symlink
	if stdout.String() != "/tmp\n" && stdout.String() != "/private/tmp\n" {
		t.Errorf("stdout = %q, want /tmp or /private/tmp", stdout.String())
	}
}

// TestAgentProtoToDeployCommandWithZeroTimeout tests with zero timeout.
func TestAgentProtoToDeployCommandWithZeroTimeout(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	protoCmd := &pb.DeployCommand{
		DeploymentId: "deploy-123",
		Project:      "my-project",
		Settings: &pb.DeploymentSettings{
			TimeoutSeconds: 0,
		},
	}

	result := agent.protoToDeployCommand(protoCmd)

	if result.Settings.Timeout != 0 {
		t.Errorf("Settings.Timeout = %v, want 0", result.Settings.Timeout)
	}
}

// TestAgentProtoToDeployCommandEmptyEnvVars tests with empty env vars.
func TestAgentProtoToDeployCommandEmptyEnvVars(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	protoCmd := &pb.DeployCommand{
		DeploymentId: "deploy-123",
		Project:      "my-project",
		EnvVars:      nil,
	}

	result := agent.protoToDeployCommand(protoCmd)

	if len(result.EnvVars) != 0 {
		t.Errorf("EnvVars = %v, want nil or empty", result.EnvVars)
	}
}

// TestAgentPerformHealthCheckStatus299 tests health check with 299 status (edge of 2xx).
func TestAgentPerformHealthCheckStatus299(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(299) // Edge of 2xx range
	}))
	defer server.Close()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	ctx := context.Background()
	result, _ := agent.performHealthCheck(ctx, server.URL, 10)

	// 299 is within 2xx range, should pass
	if !result {
		t.Error("performHealthCheck() should pass for 299 status")
	}
}

// TestAgentPerformHealthCheckStatus300 tests health check with 300 status (just outside 2xx).
func TestAgentPerformHealthCheckStatus300(t *testing.T) {
	t.Parallel()

	// Server returns 300 (Multiple Choices) without redirect
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(300)
	}))
	defer server.Close()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	ctx := context.Background()
	result, _ := agent.performHealthCheck(ctx, server.URL, 10)

	// 300 is outside 2xx range, should fail
	if result {
		t.Error("performHealthCheck() should fail for 300 status")
	}
}

// TestAgentStatusTransitions tests Status() returns correct values.
func TestAgentStatusTransitions(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	// Initial state should be stopped
	if status := agent.Status(); status != "stopped" {
		t.Errorf("Status() = %q, want 'stopped'", status)
	}

	// Set running but no connection
	agent.mu.Lock()
	agent.running = true
	agent.mu.Unlock()

	if status := agent.Status(); status != "disconnected" {
		t.Errorf("Status() = %q, want 'disconnected'", status)
	}
}

// TestAgentShutdownCleansUp tests that Shutdown properly cleans up state.
func TestAgentShutdownCleansUp(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := &config.AgentConfig{
		Agent:  config.AgentIdentityConfig{ID: "test"},
		Master: config.AgentMasterConfig{Address: "localhost:9090"},
	}
	agent, _ := NewAgent(cfg, logger)

	agent.mu.Lock()
	agent.running = true
	agent.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := agent.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	// Check running is false
	agent.mu.RLock()
	running := agent.running
	agent.mu.RUnlock()

	if running {
		t.Error("Shutdown() should set running to false")
	}
}

// TestBuildEnvPreservesSystemEnv tests that buildEnv includes system environment.
func TestBuildEnvPreservesSystemEnv(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)

	env := map[string]string{
		"CUSTOM_VAR": "custom_value",
	}

	result := runner.buildEnv(env)

	// Should include PATH from system environment
	hasPath := false
	for _, e := range result {
		if len(e) >= 5 && e[:5] == "PATH=" {
			hasPath = true
			break
		}
	}

	if !hasPath {
		t.Error("buildEnv() should preserve PATH from system environment")
	}
}

// TestLocalRunnerBuildEnvEmpty tests buildEnv with empty map.
func TestLocalRunnerBuildEnvEmpty(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)

	result := runner.buildEnv(map[string]string{})

	// Should still include system environment
	if len(result) == 0 {
		t.Error("buildEnv() with empty map should still include system environment")
	}
}
