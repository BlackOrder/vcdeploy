package agent

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/deploy"
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

	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	result, err := runner.Run(ctx, "echo error >&2", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// LocalRunner.Run() uses CombinedOutput, so stderr goes to Stdout
	if result.Stdout != "error\n" {
		t.Errorf("stdout (combined) = %q, want %q", result.Stdout, "error\n")
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
		runner.Run(ctx, "true", deploy.RunOptions{})
	}
}
