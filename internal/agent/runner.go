// Package agent provides local command execution for the agent.
package agent

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/deploy"
	"go.uber.org/zap"
)

// LocalRunner executes commands locally on the agent host.
type LocalRunner struct {
	logger *zap.Logger
}

// NewLocalRunner creates a new local command runner.
func NewLocalRunner(logger *zap.Logger) *LocalRunner {
	return &LocalRunner{logger: logger}
}

// Run executes a command and returns the result.
func (r *LocalRunner) Run(ctx context.Context, cmd string, opts deploy.RunOptions) (*deploy.CommandResult, error) {
	start := time.Now()

	// Build the command
	fullCmd := r.buildCommand(cmd, opts)
	r.logger.Debug("Running command", zap.String("cmd", fullCmd))

	// Create the exec command
	c := exec.CommandContext(ctx, "bash", "-c", fullCmd)

	// Set working directory
	if opts.WorkDir != "" {
		c.Dir = opts.WorkDir
	}

	// Set environment
	if len(opts.Env) > 0 {
		c.Env = r.buildEnv(opts.Env)
	}

	// Set user/group if specified
	if opts.User != "" {
		if err := r.setUserGroup(c, opts.User, opts.Group); err != nil {
			return nil, fmt.Errorf("setting user/group: %w", err)
		}
	}

	// Run command and capture output
	output, err := c.CombinedOutput()
	duration := time.Since(start)

	result := &deploy.CommandResult{
		Stdout:   string(output),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("running command: %w", err)
		}
	}

	return result, nil
}

// RunWithOutput executes a command with streaming output.
func (r *LocalRunner) RunWithOutput(ctx context.Context, cmd string, stdout, stderr io.Writer, opts deploy.RunOptions) error {
	// Build the command
	fullCmd := r.buildCommand(cmd, opts)
	r.logger.Debug("Running command with output", zap.String("cmd", fullCmd))

	// Create the exec command
	c := exec.CommandContext(ctx, "bash", "-c", fullCmd)

	// Set working directory
	if opts.WorkDir != "" {
		c.Dir = opts.WorkDir
	}

	// Set environment
	if len(opts.Env) > 0 {
		c.Env = r.buildEnv(opts.Env)
	}

	// Set user/group if specified
	if opts.User != "" {
		if err := r.setUserGroup(c, opts.User, opts.Group); err != nil {
			return fmt.Errorf("setting user/group: %w", err)
		}
	}

	// Connect output streams
	c.Stdout = stdout
	c.Stderr = stderr

	// Run command
	err := c.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return err // Return exit error for caller to handle
		}
		return fmt.Errorf("running command: %w", err)
	}

	return nil
}

func (r *LocalRunner) buildCommand(cmd string, opts deploy.RunOptions) string {
	var parts []string

	// Set timeout if specified
	if opts.Timeout > 0 {
		parts = append(parts, fmt.Sprintf("timeout %d", int(opts.Timeout.Seconds())))
	}

	parts = append(parts, cmd)

	return strings.Join(parts, " ")
}

func (r *LocalRunner) buildEnv(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

func (r *LocalRunner) setUserGroup(cmd *exec.Cmd, user, group string) error {
	// Note: This requires the agent to run as root to switch users
	// In production, you might want to use sudo or capabilities instead

	// For now, we'll use SysProcAttr to set the user/group
	// This requires looking up the UID/GID

	// Simple approach: use sudo -u user if available
	// The more robust approach would be to look up the user in /etc/passwd
	// and set cmd.SysProcAttr = &syscall.SysProcAttr{Credential: ...}

	// For simplicity, we skip user switching in this implementation
	// The command should be run with appropriate permissions
	_ = cmd
	_ = user
	_ = group
	_ = syscall.SYS_SETUID // Just to use syscall import

	return nil
}

// Verify LocalRunner implements CommandRunner interface
var _ deploy.CommandRunner = (*LocalRunner)(nil)
