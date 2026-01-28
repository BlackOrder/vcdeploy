// Package testutil provides shared testing utilities for E2E, CLI, and integration tests.
package testutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CLIRunner provides utilities for running CLI commands in tests.
type CLIRunner struct {
	binaryPath string
	workDir    string
	env        []string
	timeout    time.Duration
}

// NewCLIRunner creates a new CLI runner for testing.
func NewCLIRunner(binaryPath string) *CLIRunner {
	return &CLIRunner{
		binaryPath: binaryPath,
		timeout:    2 * time.Minute,
		env:        os.Environ(),
	}
}

// WithWorkDir sets the working directory for commands.
func (r *CLIRunner) WithWorkDir(dir string) *CLIRunner {
	r.workDir = dir
	return r
}

// WithTimeout sets the command timeout.
func (r *CLIRunner) WithTimeout(d time.Duration) *CLIRunner {
	r.timeout = d
	return r
}

// WithEnv adds environment variables.
func (r *CLIRunner) WithEnv(key, value string) *CLIRunner {
	r.env = append(r.env, key+"="+value)
	return r
}

// Run executes a CLI command and returns the result.
func (r *CLIRunner) Run(args ...string) *CLIResult {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.binaryPath, args...) //nolint:gosec // Test utility requires running CLI commands
	if r.workDir != "" {
		cmd.Dir = r.workDir
	}
	cmd.Env = r.env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
	}

	return &CLIResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Error:    err,
		Command:  fmt.Sprintf("%s %s", r.binaryPath, strings.Join(args, " ")),
	}
}

// RunWithInput executes a CLI command with stdin input.
func (r *CLIRunner) RunWithInput(input string, args ...string) *CLIResult {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.binaryPath, args...) //nolint:gosec // Test utility requires running CLI commands
	if r.workDir != "" {
		cmd.Dir = r.workDir
	}
	cmd.Env = r.env
	cmd.Stdin = strings.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
	}

	return &CLIResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Error:    err,
		Command:  fmt.Sprintf("%s %s", r.binaryPath, strings.Join(args, " ")),
	}
}

// CLIResult holds the result of a CLI command execution.
type CLIResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Error    error
	Command  string
}

// Success returns true if the command exited with code 0.
func (r *CLIResult) Success() bool {
	return r.ExitCode == 0
}

// Failed returns true if the command exited with a non-zero code.
func (r *CLIResult) Failed() bool {
	return r.ExitCode != 0
}

// Output returns combined stdout and stderr.
func (r *CLIResult) Output() string {
	return r.Stdout + r.Stderr
}

// ContainsStdout returns true if stdout contains the substring.
func (r *CLIResult) ContainsStdout(s string) bool {
	return strings.Contains(r.Stdout, s)
}

// ContainsStderr returns true if stderr contains the substring.
func (r *CLIResult) ContainsStderr(s string) bool {
	return strings.Contains(r.Stderr, s)
}

// ContainsOutput returns true if combined output contains the substring.
func (r *CLIResult) ContainsOutput(s string) bool {
	return strings.Contains(r.Output(), s)
}

// CLIAssertions provides assertion helpers for CLI results.
type CLIAssertions struct {
	t TestingT
}

// NewCLIAssertions creates a new CLI assertions instance.
func NewCLIAssertions(t TestingT) *CLIAssertions {
	return &CLIAssertions{t: t}
}

// Success asserts the command succeeded (exit code 0).
func (a *CLIAssertions) Success(r *CLIResult) {
	a.t.Helper()
	if r.ExitCode != 0 {
		a.t.Errorf("expected success, got exit code %d\nCommand: %s\nStdout: %s\nStderr: %s",
			r.ExitCode, r.Command, r.Stdout, r.Stderr)
	}
}

// Failed asserts the command failed (non-zero exit code).
func (a *CLIAssertions) Failed(r *CLIResult) {
	a.t.Helper()
	if r.ExitCode == 0 {
		a.t.Errorf("expected failure, got success\nCommand: %s\nStdout: %s",
			r.Command, r.Stdout)
	}
}

// ExitCode asserts the command exited with a specific code.
func (a *CLIAssertions) ExitCode(r *CLIResult, code int) {
	a.t.Helper()
	if r.ExitCode != code {
		a.t.Errorf("expected exit code %d, got %d\nCommand: %s\nStdout: %s\nStderr: %s",
			code, r.ExitCode, r.Command, r.Stdout, r.Stderr)
	}
}

// StdoutContains asserts stdout contains a substring.
func (a *CLIAssertions) StdoutContains(r *CLIResult, s string) {
	a.t.Helper()
	if !r.ContainsStdout(s) {
		a.t.Errorf("expected stdout to contain %q\nCommand: %s\nStdout: %s",
			s, r.Command, r.Stdout)
	}
}

// StderrContains asserts stderr contains a substring.
func (a *CLIAssertions) StderrContains(r *CLIResult, s string) {
	a.t.Helper()
	if !r.ContainsStderr(s) {
		a.t.Errorf("expected stderr to contain %q\nCommand: %s\nStderr: %s",
			s, r.Command, r.Stderr)
	}
}

// OutputContains asserts combined output contains a substring.
func (a *CLIAssertions) OutputContains(r *CLIResult, s string) {
	a.t.Helper()
	if !r.ContainsOutput(s) {
		a.t.Errorf("expected output to contain %q\nCommand: %s\nOutput: %s",
			s, r.Command, r.Output())
	}
}

// StdoutNotContains asserts stdout does not contain a substring.
func (a *CLIAssertions) StdoutNotContains(r *CLIResult, s string) {
	a.t.Helper()
	if r.ContainsStdout(s) {
		a.t.Errorf("expected stdout to not contain %q\nCommand: %s\nStdout: %s",
			s, r.Command, r.Stdout)
	}
}

// StderrNotContains asserts stderr does not contain a substring.
func (a *CLIAssertions) StderrNotContains(r *CLIResult, s string) {
	a.t.Helper()
	if r.ContainsStderr(s) {
		a.t.Errorf("expected stderr to not contain %q\nCommand: %s\nStderr: %s",
			s, r.Command, r.Stderr)
	}
}

// StdoutEmpty asserts stdout is empty.
func (a *CLIAssertions) StdoutEmpty(r *CLIResult) {
	a.t.Helper()
	if r.Stdout != "" {
		a.t.Errorf("expected empty stdout\nCommand: %s\nStdout: %s",
			r.Command, r.Stdout)
	}
}

// StderrEmpty asserts stderr is empty.
func (a *CLIAssertions) StderrEmpty(r *CLIResult) {
	a.t.Helper()
	if r.Stderr != "" {
		a.t.Errorf("expected empty stderr\nCommand: %s\nStderr: %s",
			r.Command, r.Stderr)
	}
}
