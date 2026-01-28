package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/spf13/cobra"
)

// TestSetVersionInfo tests the version info setter.
func TestSetVersionInfo(t *testing.T) {
	// Save original values
	origVersion := version
	origCommit := commit
	origBuildTime := buildTime
	defer func() {
		version = origVersion
		commit = origCommit
		buildTime = origBuildTime
	}()

	SetVersionInfo("1.2.3", "abc123def", "2026-01-24")

	if version != "1.2.3" {
		t.Errorf("version = %q, want %q", version, "1.2.3")
	}
	if commit != "abc123def" {
		t.Errorf("commit = %q, want %q", commit, "abc123def")
	}
	if buildTime != "2026-01-24" {
		t.Errorf("buildTime = %q, want %q", buildTime, "2026-01-24")
	}
}

// TestExecute tests the root Execute function doesn't panic.
func TestExecute(t *testing.T) {
	// Reset root command args to avoid test interference
	rootCmd.SetArgs([]string{"--help"})

	// Should not panic and should return nil for --help
	err := Execute()
	if err != nil {
		t.Errorf("Execute() with --help error = %v", err)
	}
}

// TestVersionCommand tests the version command output.
func TestVersionCommand(t *testing.T) {
	// Save original values
	origVersion := version
	origCommit := commit
	origBuildTime := buildTime
	defer func() {
		version = origVersion
		commit = origCommit
		buildTime = origBuildTime
	}()

	SetVersionInfo("2.0.0", "test-commit", "2026-01-24T12:00:00Z")

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	versionCmd.Run(nil, nil)

	w.Close()
	var stdout bytes.Buffer
	stdout.ReadFrom(r)
	os.Stdout = oldStdout

	output := stdout.String()
	if !strings.Contains(output, "2.0.0") {
		t.Errorf("version output should contain version, got: %s", output)
	}
	if !strings.Contains(output, "test-commit") {
		t.Errorf("version output should contain commit, got: %s", output)
	}
	if !strings.Contains(output, "2026-01-24T12:00:00Z") {
		t.Errorf("version output should contain build time, got: %s", output)
	}
}

// TestRootCmdStructure tests the root command structure.
func TestRootCmdStructure(t *testing.T) {
	t.Parallel()

	if rootCmd == nil {
		t.Fatal("rootCmd is nil")
	}

	if rootCmd.Use != "vcdeploy-agent" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "vcdeploy-agent")
	}

	// Check subcommands exist
	subcommands := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	expectedCommands := []string{"start", "status", "register", "version"}
	for _, name := range expectedCommands {
		if !subcommands[name] {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

// TestGlobalFlags tests that global flags are registered.
func TestGlobalFlags(t *testing.T) {
	t.Parallel()

	configFlag := rootCmd.PersistentFlags().Lookup("config")
	if configFlag == nil {
		t.Error("--config flag not registered")
	}
}

// TestStartCmdFlags tests the start command has expected flags.
func TestStartCmdFlags(t *testing.T) {
	t.Parallel()

	expectedFlags := []string{"master", "token"}
	for _, name := range expectedFlags {
		flag := startCmd.Flags().Lookup(name)
		if flag == nil {
			t.Errorf("expected flag --%s not found on start command", name)
		}
	}
}

// TestRegisterCmdFlags tests the register command has expected flags.
func TestRegisterCmdFlags(t *testing.T) {
	t.Parallel()

	expectedFlags := []string{"master", "token"}
	for _, name := range expectedFlags {
		flag := registerCmd.Flags().Lookup(name)
		if flag == nil {
			t.Errorf("expected flag --%s not found on register command", name)
		}
	}
}

// TestCheckAgentProcess tests the checkAgentProcess helper.
func TestCheckAgentProcess(t *testing.T) {
	t.Parallel()

	t.Run("non-existent PID file", func(t *testing.T) {
		running, pid := checkAgentProcess("/nonexistent/path/pid")
		if running {
			t.Error("expected not running for non-existent file")
		}
		if pid != 0 {
			t.Errorf("expected pid 0, got %d", pid)
		}
	})

	t.Run("invalid PID content", func(t *testing.T) {
		tmpDir := t.TempDir()
		pidFile := filepath.Join(tmpDir, "test.pid")

		if err := os.WriteFile(pidFile, []byte("not a number"), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		running, pid := checkAgentProcess(pidFile)
		if running {
			t.Error("expected not running for invalid PID")
		}
		if pid != 0 {
			t.Errorf("expected pid 0, got %d", pid)
		}
	})

	t.Run("stale PID (non-existent process)", func(t *testing.T) {
		tmpDir := t.TempDir()
		pidFile := filepath.Join(tmpDir, "test.pid")

		// Use a very high PID that's unlikely to exist
		if err := os.WriteFile(pidFile, []byte("999999999"), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		running, _ := checkAgentProcess(pidFile)
		if running {
			t.Error("expected not running for stale PID")
		}
	})

	t.Run("current process PID", func(t *testing.T) {
		tmpDir := t.TempDir()
		pidFile := filepath.Join(tmpDir, "test.pid")

		// Write the current process's PID
		currentPID := os.Getpid()
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(currentPID)), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		running, pid := checkAgentProcess(pidFile)
		if !running {
			t.Error("expected running for current process PID")
		}
		if pid != currentPID {
			t.Errorf("expected pid %d, got %d", currentPID, pid)
		}
	})
}

// TestRunStatusNoConfig tests runStatus with missing config.
func TestRunStatusNoConfig(t *testing.T) {
	// Create a fresh command to avoid flag conflicts
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "/nonexistent/config.yaml", "config file")

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runStatus(cmd, nil)

	w.Close()
	var stdout bytes.Buffer
	stdout.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runStatus() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "NOT CONFIGURED") {
		t.Errorf("output should show NOT CONFIGURED, got: %s", output)
	}
}

// TestRunStatusWithConfig tests runStatus with valid config.
func TestRunStatusWithConfig(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agent.yaml")

	cfg := config.DefaultAgentConfig()
	cfg.Agent.ID = "test-agent"
	cfg.Master.Address = "localhost:9001"

	if err := config.SaveAgentConfig(cfg, configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	// Create a fresh command
	cmd := &cobra.Command{}
	cmd.Flags().String("config", configPath, "config file")

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runStatus(cmd, nil)

	w.Close()
	var stdout bytes.Buffer
	stdout.ReadFrom(r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runStatus() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "test-agent") {
		t.Error("output should contain agent ID")
	}
	if !strings.Contains(output, "localhost:9001") {
		t.Error("output should contain master address")
	}
	if !strings.Contains(output, "STOPPED") {
		t.Error("output should show STOPPED status")
	}
}

// TestRunRegisterMissingFlags tests runRegister with missing required flags.
func TestRunRegisterMissingMaster(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agent.yaml")

	cmd := &cobra.Command{}
	cmd.Flags().String("master", "", "Master address")
	cmd.Flags().String("token", "some-token", "Registration token")
	cmd.Flags().String("config", configPath, "config file")

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runRegister(cmd, nil)

	w.Close()
	var stdout bytes.Buffer
	stdout.ReadFrom(r)
	os.Stdout = oldStdout

	// Empty master should cause connection error
	if err == nil {
		t.Fatal("expected error for empty master address")
	}
}

// TestConfigSaveAndLoad tests config file operations.
func TestConfigSaveAndLoad(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agent.yaml")

	// Create and save config
	cfg := config.DefaultAgentConfig()
	cfg.Agent.ID = "test-agent-123"
	cfg.Master.Address = "master.example.com:9001"
	cfg.Agent.Labels = map[string]string{
		"env":    "test",
		"region": "us-east-1",
	}

	if err := config.SaveAgentConfig(cfg, configPath); err != nil {
		t.Fatalf("SaveAgentConfig() error = %v", err)
	}

	// Load and verify
	loaded, err := config.LoadAgentConfig(configPath)
	if err != nil {
		t.Fatalf("LoadAgentConfig() error = %v", err)
	}

	if loaded.Agent.ID != cfg.Agent.ID {
		t.Errorf("Agent.ID = %q, want %q", loaded.Agent.ID, cfg.Agent.ID)
	}
	if loaded.Master.Address != cfg.Master.Address {
		t.Errorf("Master.Address = %q, want %q", loaded.Master.Address, cfg.Master.Address)
	}
	if loaded.Agent.Labels["env"] != "test" {
		t.Error("Labels were not preserved")
	}
}
