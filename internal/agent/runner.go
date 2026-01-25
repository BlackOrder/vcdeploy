// Package agent provides local command execution for the agent.
package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
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
	// Start with the current environment to preserve PATH, HOME, etc.
	result := os.Environ()
	// Overlay custom environment variables
	for k, v := range env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

func (r *LocalRunner) setUserGroup(cmd *exec.Cmd, user, group string) error {
	// Note: This requires the agent to run as root to switch users
	// In production, you might want to use sudo or capabilities instead

	// Look up the user's UID
	uid, err := lookupUser(user)
	if err != nil {
		r.logger.Warn("Could not look up user, skipping user switch",
			zap.String("user", user),
			zap.Error(err),
		)
		return nil
	}

	// Look up the group's GID (use user's primary group if not specified)
	gid := uid // Default to UID (same as primary group in many cases)
	if group != "" {
		gid, err = lookupGroup(group)
		if err != nil {
			r.logger.Warn("Could not look up group, using user's primary group",
				zap.String("group", group),
				zap.Error(err),
			)
		}
	} else {
		// Try to get user's primary group
		primaryGid, err := lookupUserPrimaryGroup(user)
		if err != nil {
			r.logger.Warn("Could not look up user's primary group, using uid as gid",
				zap.String("user", user),
				zap.Error(err),
			)
			// gid already defaults to uid above
		} else {
			gid = primaryGid
		}
	}

	// Set the credentials on the command
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
	}

	r.logger.Debug("Set user/group for command",
		zap.String("user", user),
		zap.Int("uid", uid),
		zap.String("group", group),
		zap.Int("gid", gid),
	)

	return nil
}

// lookupUser looks up a user by name and returns the UID.
func lookupUser(username string) (int, error) {
	// First check if it's already a numeric UID
	if uid, err := strconv.Atoi(username); err == nil {
		return uid, nil
	}

	// Look up in /etc/passwd
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return 0, fmt.Errorf("open passwd: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 3 {
			continue
		}
		if fields[0] == username {
			uid, err := strconv.Atoi(fields[2])
			if err != nil {
				return 0, fmt.Errorf("parse uid: %w", err)
			}
			return uid, nil
		}
	}

	return 0, fmt.Errorf("user not found: %s", username)
}

// lookupGroup looks up a group by name and returns the GID.
func lookupGroup(groupname string) (int, error) {
	// First check if it's already a numeric GID
	if gid, err := strconv.Atoi(groupname); err == nil {
		return gid, nil
	}

	// Look up in /etc/group
	f, err := os.Open("/etc/group")
	if err != nil {
		return 0, fmt.Errorf("open group: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 3 {
			continue
		}
		if fields[0] == groupname {
			gid, err := strconv.Atoi(fields[2])
			if err != nil {
				return 0, fmt.Errorf("parse gid: %w", err)
			}
			return gid, nil
		}
	}

	return 0, fmt.Errorf("group not found: %s", groupname)
}

// lookupUserPrimaryGroup looks up a user's primary group from /etc/passwd.
func lookupUserPrimaryGroup(username string) (int, error) {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return 0, fmt.Errorf("open passwd: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 4 {
			continue
		}
		if fields[0] == username {
			gid, err := strconv.Atoi(fields[3])
			if err != nil {
				return 0, fmt.Errorf("parse gid: %w", err)
			}
			return gid, nil
		}
	}

	return 0, fmt.Errorf("user not found: %s", username)
}

// Verify LocalRunner implements CommandRunner interface
var _ deploy.CommandRunner = (*LocalRunner)(nil)
