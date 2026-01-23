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
	logger, _ := zap.NewDevelopment()
	runner := NewLocalRunner(logger)

	if runner == nil {
		t.Fatal("NewLocalRunner() returned nil")
	}

	if runner.logger == nil {
		t.Error("NewLocalRunner() did not set logger")
	}
}

func TestLocalRunnerRunSimple(t *testing.T) {
	logger, _ := zap.NewDevelopment()
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
	logger, _ := zap.NewDevelopment()
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
	logger, _ := zap.NewDevelopment()
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
	logger, _ := zap.NewDevelopment()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	opts := deploy.RunOptions{
		WorkDir: "/tmp",
	}

	result, err := runner.Run(ctx, "pwd", opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Stdout != "/tmp\n" {
		t.Errorf("stdout = %q, want %q", result.Stdout, "/tmp\n")
	}
}

func TestLocalRunnerRunWithTimeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()
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
	logger, _ := zap.NewDevelopment()
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
	logger, _ := zap.NewDevelopment()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer
	err := runner.RunWithOutput(ctx, "exit 1", &stdout, &stderr, deploy.RunOptions{})

	if err == nil {
		t.Error("RunWithOutput() expected error for exit 1")
	}
}

func TestLocalRunnerBuildCommand(t *testing.T) {
	logger, _ := zap.NewDevelopment()
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
	logger, _ := zap.NewDevelopment()
	runner := NewLocalRunner(logger)

	env := map[string]string{
		"VAR1": "value1",
		"VAR2": "value2",
	}

	result := runner.buildEnv(env)

	if len(result) != 2 {
		t.Errorf("buildEnv() returned %d items, want 2", len(result))
	}
}

func TestLocalRunnerImplementsInterface(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	runner := NewLocalRunner(logger)

	var _ deploy.CommandRunner = runner
}

func TestNewAgent(t *testing.T) {
	logger, _ := zap.NewDevelopment()

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
	logger, _ := zap.NewDevelopment()

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

func BenchmarkLocalRunnerSimpleCommand(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	runner := NewLocalRunner(logger)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runner.Run(ctx, "true", deploy.RunOptions{})
	}
}
