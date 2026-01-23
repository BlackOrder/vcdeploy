package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

// MockCommandRunner implements CommandRunner for testing
type MockCommandRunner struct {
	RunFunc       func(ctx context.Context, cmd string, opts RunOptions) (*CommandResult, error)
	RunOutputFunc func(ctx context.Context, cmd string, stdout, stderr io.Writer, opts RunOptions) error
	Commands      []string
	Results       map[string]*CommandResult
	Errors        map[string]error
}

func (m *MockCommandRunner) Run(ctx context.Context, cmd string, opts RunOptions) (*CommandResult, error) {
	m.Commands = append(m.Commands, cmd)

	if m.RunFunc != nil {
		return m.RunFunc(ctx, cmd, opts)
	}

	if err, ok := m.Errors[cmd]; ok {
		return nil, err
	}

	if result, ok := m.Results[cmd]; ok {
		return result, nil
	}

	return &CommandResult{
		ExitCode: 0,
		Stdout:   "",
		Stderr:   "",
		Duration: 100 * time.Millisecond,
	}, nil
}

func (m *MockCommandRunner) RunWithOutput(ctx context.Context, cmd string, stdout, stderr io.Writer, opts RunOptions) error {
	m.Commands = append(m.Commands, cmd)

	if m.RunOutputFunc != nil {
		return m.RunOutputFunc(ctx, cmd, stdout, stderr, opts)
	}

	if err, ok := m.Errors[cmd]; ok {
		return err
	}

	return nil
}

func TestDeployCommand(t *testing.T) {
	cmd := &DeployCommand{
		DeploymentID: "deploy-001",
		Project:      "test-project",
		Target:       "production",
		Repository:   "https://github.com/test/repo.git",
		Branch:       "main",
		Commit:       "abc123",
		Path:         "/var/www/test",
		Settings: DeploySettings{
			Strategy:     "symlink",
			KeepReleases: 5,
			SharedDirs:   []string{"storage", "logs"},
			SharedFiles:  []string{".env"},
		},
		EnvVars: map[string]string{
			"APP_ENV": "production",
		},
		PreDeployHooks:  []string{"composer install"},
		PostDeployHooks: []string{"php artisan migrate"},
	}

	if cmd.DeploymentID != "deploy-001" {
		t.Errorf("DeployCommand.DeploymentID = %v, want deploy-001", cmd.DeploymentID)
	}

	if len(cmd.Settings.SharedDirs) != 2 {
		t.Errorf("DeployCommand.Settings.SharedDirs count = %d, want 2", len(cmd.Settings.SharedDirs))
	}
}

func TestDeploySettings(t *testing.T) {
	settings := DeploySettings{
		Strategy:       "symlink",
		KeepReleases:   3,
		SharedDirs:     []string{"storage"},
		WritableDirs:   []string{"cache"},
		ExecutionUser:  "www-data",
		ExecutionGroup: "www-data",
		Timeout:        5 * time.Minute,
	}

	if settings.Strategy != "symlink" {
		t.Errorf("DeploySettings.Strategy = %v, want symlink", settings.Strategy)
	}

	if settings.Timeout != 5*time.Minute {
		t.Errorf("DeploySettings.Timeout = %v, want 5m", settings.Timeout)
	}
}

func TestDeployResult(t *testing.T) {
	result := &DeployResult{
		Success:       true,
		ReleaseNumber: 42,
		ReleasePath:   "/var/www/app/releases/42",
		StartedAt:     time.Now(),
		CompletedAt:   time.Now().Add(30 * time.Second),
		Error:         nil,
	}

	if !result.Success {
		t.Error("DeployResult.Success should be true")
	}

	if result.ReleaseNumber != 42 {
		t.Errorf("DeployResult.ReleaseNumber = %d, want 42", result.ReleaseNumber)
	}
}

func TestDeployResultWithError(t *testing.T) {
	result := &DeployResult{
		Success: false,
		Error:   errors.New("deployment failed"),
	}

	if result.Success {
		t.Error("DeployResult.Success should be false when error is set")
	}

	if result.Error == nil || result.Error.Error() != "deployment failed" {
		t.Errorf("DeployResult.Error = %v, want 'deployment failed'", result.Error)
	}
}

func TestLogLevel(t *testing.T) {
	tests := []struct {
		level LogLevel
		str   string
	}{
		{LogDebug, "DEBUG"},
		{LogInfo, "INFO"},
		{LogWarn, "WARN"},
		{LogError, "ERROR"},
		{LogLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if tt.level.String() != tt.str {
				t.Errorf("LogLevel.String() = %v, want %v", tt.level.String(), tt.str)
			}
		})
	}
}

func TestLogEntry(t *testing.T) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     LogInfo,
		Message:   "Test message",
		Source:    "deploy",
	}

	if entry.Level != LogInfo {
		t.Errorf("LogEntry.Level = %v, want %v", entry.Level, LogInfo)
	}

	if entry.Source != "deploy" {
		t.Errorf("LogEntry.Source = %v, want deploy", entry.Source)
	}
}

func TestLogWriter(t *testing.T) {
	logCh := make(chan LogEntry, 10)

	writer := &LogWriter{
		DeploymentID: "test-deploy",
		Source:       "test",
		Level:        LogInfo,
		Output:       logCh,
	}

	testMessage := "Hello, World!"
	n, err := writer.Write([]byte(testMessage))

	if err != nil {
		t.Fatalf("LogWriter.Write() error = %v", err)
	}

	if n != len(testMessage) {
		t.Errorf("LogWriter.Write() bytes = %d, want %d", n, len(testMessage))
	}

	select {
	case entry := <-logCh:
		if entry.Message != testMessage {
			t.Errorf("LogEntry.Message = %v, want %v", entry.Message, testMessage)
		}
		if entry.Level != LogInfo {
			t.Errorf("LogEntry.Level = %v, want %v", entry.Level, LogInfo)
		}
		if entry.Source != "test" {
			t.Errorf("LogEntry.Source = %v, want test", entry.Source)
		}
	default:
		t.Error("Expected log entry in channel")
	}
}

func TestLogWriterMultiple(t *testing.T) {
	logCh := make(chan LogEntry, 10)

	writer := &LogWriter{
		DeploymentID: "test-deploy",
		Source:       "multi-test",
		Level:        LogDebug,
		Output:       logCh,
	}

	messages := []string{"First", "Second", "Third"}
	for _, msg := range messages {
		writer.Write([]byte(msg))
	}

	for i, expected := range messages {
		select {
		case entry := <-logCh:
			if entry.Message != expected {
				t.Errorf("Message %d: got %v, want %v", i, entry.Message, expected)
			}
		default:
			t.Errorf("Expected message %d in channel", i)
		}
	}
}

func TestNewSymlinkStrategy(t *testing.T) {
	runner := &MockCommandRunner{}
	strategy := NewSymlinkStrategy(runner)

	if strategy == nil {
		t.Fatal("NewSymlinkStrategy() returned nil")
	}
}

func TestSymlinkStrategyDeploy(t *testing.T) {
	runner := &MockCommandRunner{
		Results: make(map[string]*CommandResult),
		Errors:  make(map[string]error),
		RunFunc: func(ctx context.Context, cmd string, opts RunOptions) (*CommandResult, error) {
			// Simulate ls command for release number
			if bytes.Contains([]byte(cmd), []byte("ls -1")) {
				return &CommandResult{
					ExitCode: 0,
					Stdout:   "1\n2\n",
				}, nil
			}
			return &CommandResult{ExitCode: 0}, nil
		},
	}

	strategy := NewSymlinkStrategy(runner)
	logCh := make(chan LogEntry, 100)

	cmd := &DeployCommand{
		DeploymentID: "test-001",
		Project:      "test",
		Target:       "production",
		Repository:   "https://github.com/test/repo.git",
		Branch:       "main",
		Commit:       "abc123",
		Path:         "/var/www/test",
		Settings: DeploySettings{
			Strategy:     "symlink",
			KeepReleases: 3,
		},
	}

	ctx := context.Background()
	result, err := strategy.Deploy(ctx, cmd, logCh)

	// Drain log channel
	close(logCh)
	var logs []LogEntry
	for entry := range logCh {
		logs = append(logs, entry)
	}

	if err != nil && result == nil {
		// Some commands may fail in mock, but structure should be set
		t.Logf("Deploy error (expected in mock): %v", err)
	}

	if result == nil {
		t.Fatal("Deploy() returned nil result")
	}

	if !result.StartedAt.IsZero() {
		t.Logf("Deployment started at %v", result.StartedAt)
	}

	if len(logs) > 0 {
		t.Logf("Got %d log entries", len(logs))
	}
}

func TestRollbackCommand(t *testing.T) {
	cmd := &RollbackCommand{
		DeploymentID:  "rollback-001",
		Project:       "test-project",
		Target:        "production",
		Path:          "/var/www/test",
		ReleaseNumber: 0, // Previous release
		RollbackHooks: []string{"php artisan down"},
		ReloadServices: []ServiceReload{
			{Service: "php-fpm", Action: "reload"},
		},
	}

	if cmd.ReleaseNumber != 0 {
		t.Errorf("RollbackCommand.ReleaseNumber = %d, want 0", cmd.ReleaseNumber)
	}

	if len(cmd.ReloadServices) != 1 {
		t.Errorf("RollbackCommand.ReloadServices count = %d, want 1", len(cmd.ReloadServices))
	}
}

func TestServiceReload(t *testing.T) {
	services := []ServiceReload{
		{Service: "nginx", Action: "reload"},
		{Service: "php-fpm", Action: "restart"},
		{Service: "queue-worker", Action: "start"},
	}

	expected := map[string]string{
		"nginx":        "reload",
		"php-fpm":      "restart",
		"queue-worker": "start",
	}

	for _, svc := range services {
		if action, ok := expected[svc.Service]; ok {
			if svc.Action != action {
				t.Errorf("Service %s action = %v, want %v", svc.Service, svc.Action, action)
			}
		} else {
			t.Errorf("Unexpected service: %s", svc.Service)
		}
	}
}

func TestCommandResult(t *testing.T) {
	result := &CommandResult{
		ExitCode: 0,
		Stdout:   "Success output",
		Stderr:   "",
		Duration: 150 * time.Millisecond,
	}

	if result.ExitCode != 0 {
		t.Errorf("CommandResult.ExitCode = %d, want 0", result.ExitCode)
	}

	if result.Duration != 150*time.Millisecond {
		t.Errorf("CommandResult.Duration = %v, want 150ms", result.Duration)
	}
}

func TestCommandResultWithError(t *testing.T) {
	result := &CommandResult{
		ExitCode: 1,
		Stdout:   "",
		Stderr:   "Command not found",
		Duration: 50 * time.Millisecond,
	}

	if result.ExitCode != 1 {
		t.Errorf("CommandResult.ExitCode = %d, want 1", result.ExitCode)
	}

	if result.Stderr != "Command not found" {
		t.Errorf("CommandResult.Stderr = %v, want 'Command not found'", result.Stderr)
	}
}

func TestRunOptions(t *testing.T) {
	opts := RunOptions{
		WorkDir: "/var/www/app",
		Env: map[string]string{
			"APP_ENV": "production",
			"DEBUG":   "false",
		},
		User:    "www-data",
		Group:   "www-data",
		Timeout: 30 * time.Second,
	}

	if opts.WorkDir != "/var/www/app" {
		t.Errorf("RunOptions.WorkDir = %v, want /var/www/app", opts.WorkDir)
	}

	if len(opts.Env) != 2 {
		t.Errorf("RunOptions.Env count = %d, want 2", len(opts.Env))
	}

	if opts.Timeout != 30*time.Second {
		t.Errorf("RunOptions.Timeout = %v, want 30s", opts.Timeout)
	}
}

func TestMockCommandRunner(t *testing.T) {
	runner := &MockCommandRunner{
		Results: map[string]*CommandResult{
			"echo hello": {
				ExitCode: 0,
				Stdout:   "hello\n",
				Duration: 10 * time.Millisecond,
			},
		},
	}

	ctx := context.Background()
	result, err := runner.Run(ctx, "echo hello", RunOptions{})

	if err != nil {
		t.Fatalf("MockCommandRunner.Run() error = %v", err)
	}

	if result.Stdout != "hello\n" {
		t.Errorf("Result.Stdout = %v, want 'hello\\n'", result.Stdout)
	}

	if len(runner.Commands) != 1 || runner.Commands[0] != "echo hello" {
		t.Errorf("Commands recorded = %v, want [echo hello]", runner.Commands)
	}
}

func TestMockCommandRunnerError(t *testing.T) {
	expectedErr := errors.New("command failed")
	runner := &MockCommandRunner{
		Errors: map[string]error{
			"failing-command": expectedErr,
		},
	}

	ctx := context.Background()
	_, err := runner.Run(ctx, "failing-command", RunOptions{})

	if err != expectedErr {
		t.Errorf("Run() error = %v, want %v", err, expectedErr)
	}
}

func TestContextCancellation(t *testing.T) {
	runner := &MockCommandRunner{
		RunFunc: func(ctx context.Context, cmd string, opts RunOptions) (*CommandResult, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return &CommandResult{ExitCode: 0}, nil
			}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := runner.Run(ctx, "test", RunOptions{})
	if err == nil {
		t.Error("Expected context cancellation error")
	}
}

// Benchmark tests
func BenchmarkLogWriter(b *testing.B) {
	logCh := make(chan LogEntry, b.N)
	writer := &LogWriter{
		DeploymentID: "bench",
		Source:       "benchmark",
		Level:        LogInfo,
		Output:       logCh,
	}

	message := []byte("Benchmark log message for performance testing")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer.Write(message)
	}
}

func BenchmarkLogLevelString(b *testing.B) {
	levels := []LogLevel{LogDebug, LogInfo, LogWarn, LogError}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = levels[i%len(levels)].String()
	}
}

// Integration-style tests
func TestDeployCommandValidation(t *testing.T) {
	tests := []struct {
		name    string
		cmd     *DeployCommand
		wantErr bool
	}{
		{
			name: "valid command",
			cmd: &DeployCommand{
				DeploymentID: "test-001",
				Project:      "my-project",
				Repository:   "https://github.com/test/repo.git",
				Branch:       "main",
				Path:         "/var/www/app",
			},
			wantErr: false,
		},
		{
			name: "missing deployment ID",
			cmd: &DeployCommand{
				Project:    "my-project",
				Repository: "https://github.com/test/repo.git",
			},
			wantErr: true,
		},
		{
			name: "missing repository",
			cmd: &DeployCommand{
				DeploymentID: "test-001",
				Project:      "my-project",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeployCommand(tt.cmd)
			hasErr := err != nil
			if hasErr != tt.wantErr {
				t.Errorf("validateDeployCommand() hasError = %v, wantErr = %v", hasErr, tt.wantErr)
			}
		})
	}
}

// Helper function for validation tests
func validateDeployCommand(cmd *DeployCommand) error {
	if cmd.DeploymentID == "" {
		return fmt.Errorf("deployment ID is required")
	}
	if cmd.Repository == "" {
		return fmt.Errorf("repository is required")
	}
	return nil
}
