package commands

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"go.uber.org/zap"
)

func TestNewAppContext(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()

	if ctx == nil {
		t.Fatal("NewAppContext() returned nil")
	}

	if ctx.Stdout == nil {
		t.Error("Stdout should not be nil")
	}
	if ctx.Stderr == nil {
		t.Error("Stderr should not be nil")
	}
	if ctx.Stdin == nil {
		t.Error("Stdin should not be nil")
	}
	if ctx.Context == nil {
		t.Error("Context should not be nil")
	}
}

func TestAppContext_WithLogger(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()
	logger, _ := zap.NewDevelopment()

	result := ctx.WithLogger(logger)

	if result != ctx {
		t.Error("WithLogger should return same context for chaining")
	}
	if ctx.Logger != logger {
		t.Error("Logger should be set")
	}
}

func TestAppContext_WithConfig(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()
	cfg := &config.MasterConfig{}

	result := ctx.WithConfig(cfg)

	if result != ctx {
		t.Error("WithConfig should return same context for chaining")
	}
	if ctx.Config != cfg {
		t.Error("Config should be set")
	}
}

func TestAppContext_WithStdout(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()
	buf := &bytes.Buffer{}

	result := ctx.WithStdout(buf)

	if result != ctx {
		t.Error("WithStdout should return same context for chaining")
	}
	if ctx.Stdout != buf {
		t.Error("Stdout should be set")
	}
}

func TestAppContext_WithStderr(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()
	buf := &bytes.Buffer{}

	result := ctx.WithStderr(buf)

	if result != ctx {
		t.Error("WithStderr should return same context for chaining")
	}
	if ctx.Stderr != buf {
		t.Error("Stderr should be set")
	}
}

func TestAppContext_WithStdin(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()
	reader := strings.NewReader("test input")

	result := ctx.WithStdin(reader)

	if result != ctx {
		t.Error("WithStdin should return same context for chaining")
	}
	if ctx.Stdin != reader {
		t.Error("Stdin should be set")
	}
}

func TestAppContext_WithContext(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()
	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := ctx.WithContext(baseCtx)

	if result != ctx {
		t.Error("WithContext should return same context for chaining")
	}
	if ctx.Context != baseCtx {
		t.Error("Context should be set")
	}
}

func TestAppContext_WithConfigPath(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()
	path := "/etc/vcdeploy/master.yaml"

	result := ctx.WithConfigPath(path)

	if result != ctx {
		t.Error("WithConfigPath should return same context for chaining")
	}
	if ctx.ConfigPath != path {
		t.Error("ConfigPath should be set")
	}
}

func TestAppContext_WithDryRun(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()

	result := ctx.WithDryRun(true)

	if result != ctx {
		t.Error("WithDryRun should return same context for chaining")
	}
	if !ctx.DryRun {
		t.Error("DryRun should be true")
	}
}

func TestAppContext_Printf(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	ctx := NewAppContext().WithStdout(buf)

	ctx.Printf("Hello, %s!", "World")

	if buf.String() != "Hello, World!" {
		t.Errorf("Printf output = %q, want %q", buf.String(), "Hello, World!")
	}
}

func TestAppContext_Println(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	ctx := NewAppContext().WithStdout(buf)

	ctx.Println("Hello", "World")

	if buf.String() != "Hello World\n" {
		t.Errorf("Println output = %q, want %q", buf.String(), "Hello World\n")
	}
}

func TestAppContext_Errorf(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	ctx := NewAppContext().WithStderr(buf)

	ctx.Errorf("Error: %s", "test")

	if buf.String() != "Error: test" {
		t.Errorf("Errorf output = %q, want %q", buf.String(), "Error: test")
	}
}

func TestAppContext_Errorln(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	ctx := NewAppContext().WithStderr(buf)

	ctx.Errorln("Error occurred")

	if buf.String() != "Error occurred\n" {
		t.Errorf("Errorln output = %q, want %q", buf.String(), "Error occurred\n")
	}
}

func TestAppContext_Close(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()
	// Should not panic when called with nil values
	ctx.Close()
}

func TestAppContext_Chaining(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("input")
	logger, _ := zap.NewDevelopment()

	ctx := NewAppContext().
		WithLogger(logger).
		WithStdout(stdout).
		WithStderr(stderr).
		WithStdin(stdin).
		WithConfigPath("/test/path").
		WithDryRun(true)

	if ctx.Logger != logger {
		t.Error("Logger not set after chaining")
	}
	if ctx.Stdout != stdout {
		t.Error("Stdout not set after chaining")
	}
	if ctx.Stderr != stderr {
		t.Error("Stderr not set after chaining")
	}
	if ctx.Stdin != stdin {
		t.Error("Stdin not set after chaining")
	}
	if ctx.ConfigPath != "/test/path" {
		t.Error("ConfigPath not set after chaining")
	}
	if !ctx.DryRun {
		t.Error("DryRun not set after chaining")
	}
}

func TestNewCommandFactory(t *testing.T) {
	t.Parallel()

	appCtx := NewAppContext()
	factory := NewCommandFactory(appCtx)

	if factory == nil {
		t.Fatal("NewCommandFactory() returned nil")
	}
	if factory.Context() != appCtx {
		t.Error("Factory should return the same context")
	}
}

func TestVersionRunner_Run(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	ctx := NewAppContext().WithStdout(buf)

	runner := NewVersionRunner(ctx, "1.0.0", "abc123", "2024-01-01")
	err := runner.Run()

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "1.0.0") {
		t.Error("Output should contain version")
	}
	if !strings.Contains(output, "abc123") {
		t.Error("Output should contain commit")
	}
	if !strings.Contains(output, "2024-01-01") {
		t.Error("Output should contain build time")
	}
}

func TestMasterStatusRunner_Run(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	ctx := NewAppContext().WithStdout(buf)

	runner := NewMasterStatusRunner(ctx)
	err := runner.Run()

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "status") {
		t.Error("Output should mention status")
	}
}

func TestNewProjectListRunner(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()
	runner := NewProjectListRunner(ctx)

	if runner == nil {
		t.Fatal("NewProjectListRunner() returned nil")
	}
	if runner.ctx != ctx {
		t.Error("Runner should store context")
	}
}

// mockReadWriter implements io.Reader and io.Writer for testing
type mockReadWriter struct {
	io.Reader
	io.Writer
}

func TestAppContext_InputOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		format string
		args   []interface{}
		want   string
	}{
		{
			name:   "simple string",
			format: "hello",
			args:   nil,
			want:   "hello",
		},
		{
			name:   "with format args",
			format: "count: %d",
			args:   []interface{}{42},
			want:   "count: 42",
		},
		{
			name:   "multiple args",
			format: "%s has %d items",
			args:   []interface{}{"list", 5},
			want:   "list has 5 items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			buf := &bytes.Buffer{}
			ctx := NewAppContext().WithStdout(buf)

			ctx.Printf(tt.format, tt.args...)

			if buf.String() != tt.want {
				t.Errorf("output = %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

// TestAppContext_WithStorage tests the WithStorage method.
func TestAppContext_WithStorage(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()

	// Note: We can't easily create a real storage.DB in tests,
	// but we can test the method exists and sets nil
	result := ctx.WithStorage(nil)

	if result != ctx {
		t.Error("WithStorage should return same context for chaining")
	}
	if ctx.Storage != nil {
		t.Error("Storage should be nil when set to nil")
	}
}

// TestMasterStatusRunner_SetMasterAddr tests the SetMasterAddr method.
func TestMasterStatusRunner_SetMasterAddr(t *testing.T) {
	t.Parallel()

	ctx := NewAppContext()
	runner := NewMasterStatusRunner(ctx)

	runner.SetMasterAddr("localhost:9999")

	if runner.masterAddr != "localhost:9999" {
		t.Errorf("masterAddr = %q, want %q", runner.masterAddr, "localhost:9999")
	}
}

// TestMasterStatusRunner_RunWithStats tests the master status runner with stats.
func TestMasterStatusRunner_RunWithStats(t *testing.T) {
	t.Parallel()

	// Create a mock server that returns health and stats
	server := httptest.NewServer(http.NewServeMux())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})
	mux.HandleFunc("/api/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"projects":5,"connected_agents":3}`))
	})
	server.Config.Handler = mux
	defer server.Close()

	buf := &bytes.Buffer{}
	ctx := NewAppContext().WithStdout(buf)
	runner := NewMasterStatusRunner(ctx)
	runner.SetMasterAddr(strings.TrimPrefix(server.URL, "http://"))

	err := runner.Run()
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ONLINE") {
		t.Errorf("output should contain ONLINE, got: %s", output)
	}
}

// TestMasterStatusRunner_RunUnhealthy tests master status when unhealthy.
func TestMasterStatusRunner_RunUnhealthy(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	buf := &bytes.Buffer{}
	ctx := NewAppContext().WithStdout(buf)
	runner := NewMasterStatusRunner(ctx)
	runner.SetMasterAddr(strings.TrimPrefix(server.URL, "http://"))

	err := runner.Run()
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "UNHEALTHY") {
		t.Errorf("output should contain UNHEALTHY, got: %s", output)
	}
}
