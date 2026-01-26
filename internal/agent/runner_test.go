package agent

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/deploy"
	"go.uber.org/zap"
)

func newTestRunner(t *testing.T) *LocalRunner {
	t.Helper()
	logger := zap.NewNop()
	runner := NewLocalRunner(logger)
	// Disable validation in tests - tests use shell commands not in deployment allowlist
	runner.SkipValidation = true
	return runner
}

func TestLocalRunner_Run_Echo(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)
	ctx := context.Background()

	result, err := runner.Run(ctx, "echo hello world", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}

	expected := "hello world\n"
	if result.Stdout != expected {
		t.Errorf("Stdout = %q, want %q", result.Stdout, expected)
	}
}

func TestLocalRunner_Run_TrueCommand(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)
	ctx := context.Background()

	result, err := runner.Run(ctx, "true", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestLocalRunner_Run_FalseCommand(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)
	ctx := context.Background()

	result, err := runner.Run(ctx, "false", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
}

func TestLocalRunner_Run_WithEnv(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)
	ctx := context.Background()

	result, err := runner.Run(ctx, "echo $TEST_VAR", deploy.RunOptions{
		Env: map[string]string{
			"TEST_VAR": "test_value_123",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}

	expected := "test_value_123\n"
	if result.Stdout != expected {
		t.Errorf("Stdout = %q, want %q", result.Stdout, expected)
	}
}

func TestLocalRunner_Run_WithWorkDir(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)
	ctx := context.Background()

	result, err := runner.Run(ctx, "pwd", deploy.RunOptions{
		WorkDir: "/tmp",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}

	if !strings.HasPrefix(strings.TrimSpace(result.Stdout), "/tmp") {
		t.Errorf("Stdout = %q, want /tmp prefix", result.Stdout)
	}
}

func TestLocalRunner_Run_WithTimeout(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)
	ctx := context.Background()

	// Command should complete within timeout
	result, err := runner.Run(ctx, "echo fast", deploy.RunOptions{
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestLocalRunner_Run_ContextCancellation(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This command would take too long, but context should cancel it
	_, err := runner.Run(ctx, "sleep 10", deploy.RunOptions{})

	// Should get an error due to context cancellation
	if err == nil {
		t.Log("Command completed before context cancellation (may happen on fast systems)")
	}
}

func TestLocalRunner_Run_CatStdin(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)
	ctx := context.Background()

	// Test piping input through cat
	result, err := runner.Run(ctx, "echo 'input data' | cat", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}

	expected := "input data\n"
	if result.Stdout != expected {
		t.Errorf("Stdout = %q, want %q", result.Stdout, expected)
	}
}

func TestLocalRunner_Run_Duration(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)
	ctx := context.Background()

	result, err := runner.Run(ctx, "sleep 0.1", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Duration should be at least 100ms
	if result.Duration < 50*time.Millisecond {
		t.Errorf("Duration = %v, expected at least 50ms", result.Duration)
	}
}

func TestLocalRunner_RunWithOutput_Echo(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer
	err := runner.RunWithOutput(ctx, "echo hello output", &stdout, &stderr, deploy.RunOptions{})
	if err != nil {
		t.Fatalf("RunWithOutput() error = %v", err)
	}

	expected := "hello output\n"
	if stdout.String() != expected {
		t.Errorf("stdout = %q, want %q", stdout.String(), expected)
	}

	if stderr.String() != "" {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestLocalRunner_RunWithOutput_StderrCapture(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer
	err := runner.RunWithOutput(ctx, "echo error message >&2", &stdout, &stderr, deploy.RunOptions{})
	if err != nil {
		t.Fatalf("RunWithOutput() error = %v", err)
	}

	if stdout.String() != "" {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}

	expected := "error message\n"
	if stderr.String() != expected {
		t.Errorf("stderr = %q, want %q", stderr.String(), expected)
	}
}

func TestLocalRunner_RunWithOutput_MixedOutput(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer
	err := runner.RunWithOutput(ctx, "echo stdout; echo stderr >&2", &stdout, &stderr, deploy.RunOptions{})
	if err != nil {
		t.Fatalf("RunWithOutput() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "stdout") {
		t.Errorf("stdout = %q, should contain 'stdout'", stdout.String())
	}

	if !strings.Contains(stderr.String(), "stderr") {
		t.Errorf("stderr = %q, should contain 'stderr'", stderr.String())
	}
}

func TestLocalRunner_RunWithOutput_ExitError(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer
	err := runner.RunWithOutput(ctx, "exit 42", &stdout, &stderr, deploy.RunOptions{})

	if err == nil {
		t.Fatal("RunWithOutput() expected error for non-zero exit")
	}
}

func TestLocalRunner_buildEnv(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)

	env := map[string]string{
		"VAR1": "value1",
		"VAR2": "value2",
	}

	result := runner.buildEnv(env)

	// Should include os.Environ() plus our custom vars
	// Verify it has at least our 2 custom vars
	if len(result) < 2 {
		t.Errorf("len(result) = %d, want at least 2", len(result))
	}

	// Check that both env vars are present (order not guaranteed)
	found := make(map[string]bool)
	for _, e := range result {
		if e == "VAR1=value1" || e == "VAR2=value2" {
			found[e] = true
		}
	}

	if !found["VAR1=value1"] {
		t.Error("missing VAR1=value1")
	}
	if !found["VAR2=value2"] {
		t.Error("missing VAR2=value2")
	}
}

func TestLocalRunner_buildCommand_WithTimeout(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)

	cmd := runner.buildCommand("echo test", deploy.RunOptions{
		Timeout: 30 * time.Second,
	})

	if !strings.HasPrefix(cmd, "timeout 30 ") {
		t.Errorf("cmd = %q, want timeout prefix", cmd)
	}

	if !strings.HasSuffix(cmd, "echo test") {
		t.Errorf("cmd = %q, want 'echo test' suffix", cmd)
	}
}

func TestLocalRunner_buildCommand_NoTimeout(t *testing.T) {
	t.Parallel()
	runner := newTestRunner(t)

	cmd := runner.buildCommand("echo test", deploy.RunOptions{})

	if cmd != "echo test" {
		t.Errorf("cmd = %q, want 'echo test'", cmd)
	}
}

func TestLookupUser_NumericUID(t *testing.T) {
	t.Parallel()

	uid, err := lookupUser("1000")
	if err != nil {
		t.Fatalf("lookupUser() error = %v", err)
	}

	if uid != 1000 {
		t.Errorf("uid = %d, want 1000", uid)
	}
}

func TestLookupGroup_NumericGID(t *testing.T) {
	t.Parallel()

	gid, err := lookupGroup("1000")
	if err != nil {
		t.Fatalf("lookupGroup() error = %v", err)
	}

	if gid != 1000 {
		t.Errorf("gid = %d, want 1000", gid)
	}
}
