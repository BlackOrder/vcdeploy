// Package security provides security utilities for vcdeploy.
package security

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// CommandValidator validates commands against a strict allowlist.
// It implements zero-trust command validation to prevent injection attacks.
type CommandValidator struct {
	// AllowedBinaries are the only executables that can be run.
	// Paths must be absolute (e.g., "/usr/bin/git") or base names (e.g., "git").
	AllowedBinaries map[string]bool

	// AllowedPatterns are regex patterns for matching complex commands.
	// Use sparingly and only for well-understood command patterns.
	AllowedPatterns []*regexp.Regexp

	// BlockedSubstrings are patterns that are always rejected.
	// These include known command injection vectors.
	BlockedSubstrings []string
}

// Common blocked substrings for command injection prevention.
var defaultBlockedSubstrings = []string{
	// Shell metacharacters that enable chaining
	"$(", "`", "${", "&&", "||", ";", "|",
	// Redirection operators
	">", "<", ">>", "<<",
	// Backgrounding
	"&",
	// Subshell and eval
	"eval ", "exec ",
	// Newlines (can bypass single-line parsing)
	"\n", "\r",
	// Null bytes
	"\x00",
}

// Commonly safe deployment binaries.
var defaultAllowedBinaries = map[string]bool{
	// Git operations
	"git":          true,
	"/usr/bin/git": true,

	// PHP/Composer
	"php":          true,
	"composer":     true,
	"/usr/bin/php": true,

	// Node.js/NPM
	"node": true,
	"npm":  true,
	"npx":  true,
	"yarn": true,
	"pnpm": true,

	// Python
	"python":  true,
	"python3": true,
	"pip":     true,
	"pip3":    true,

	// Common deployment tools
	"rsync": true,
	"cp":    true,
	"mv":    true,
	"rm":    true,
	"mkdir": true,
	"chmod": true,
	"chown": true,
	"ln":    true,
	"tar":   true,
	"unzip": true,

	// Shell utilities (for hooks and logging)
	"echo":  true,
	"cat":   true,
	"grep":  true,
	"head":  true,
	"tail":  true,
	"wc":    true,
	"date":  true,
	"sleep": true,
	"test":  true,
	"true":  true,
	"false": true,
	"exit":  true, // For deployment scripts
	"pwd":   true, // Print working directory

	// Shell interpreters (needed for complex hooks)
	"sh":   true,
	"bash": true,

	// System services (for reload hooks)
	"systemctl": true,
	"service":   true,

	// Database migrations
	"artisan": true, // Laravel
	"rake":    true, // Ruby/Rails
	"bundle":  true, // Bundler

	// Container tools
	"docker":         true,
	"docker-compose": true,

	// Timeout wrapper
	"timeout": true,
}

// ErrCommandBlocked is returned when a command is blocked by the validator.
var ErrCommandBlocked = errors.New("command blocked by security policy")

// NewCommandValidator creates a CommandValidator with default settings.
func NewCommandValidator() *CommandValidator {
	return &CommandValidator{
		AllowedBinaries:   copyMap(defaultAllowedBinaries),
		BlockedSubstrings: append([]string{}, defaultBlockedSubstrings...),
	}
}

// NewStrictCommandValidator creates a CommandValidator with no defaults.
// Use this when you want full control over the allowlist.
func NewStrictCommandValidator() *CommandValidator {
	return &CommandValidator{
		AllowedBinaries:   make(map[string]bool),
		BlockedSubstrings: append([]string{}, defaultBlockedSubstrings...),
	}
}

// AllowBinary adds a binary to the allowlist.
func (v *CommandValidator) AllowBinary(name string) *CommandValidator {
	v.AllowedBinaries[name] = true
	return v
}

// AllowPattern adds a regex pattern for command matching.
// The pattern should match the entire command string.
func (v *CommandValidator) AllowPattern(pattern *regexp.Regexp) *CommandValidator {
	v.AllowedPatterns = append(v.AllowedPatterns, pattern)
	return v
}

// BlockSubstring adds a substring that will cause commands to be rejected.
func (v *CommandValidator) BlockSubstring(s string) *CommandValidator {
	v.BlockedSubstrings = append(v.BlockedSubstrings, s)
	return v
}

// Validate checks if a command is allowed to execute.
// Returns nil if the command is safe, or an error describing why it was blocked.
func (v *CommandValidator) Validate(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("%w: empty command", ErrCommandBlocked)
	}

	// Check for blocked substrings first (fast path for injection attempts)
	for _, blocked := range v.BlockedSubstrings {
		if strings.Contains(cmd, blocked) {
			return fmt.Errorf("%w: contains blocked pattern %q", ErrCommandBlocked, blocked)
		}
	}

	// Extract the binary name from the command
	binary := extractBinary(cmd)
	if binary == "" {
		return fmt.Errorf("%w: could not extract binary name", ErrCommandBlocked)
	}

	// Check if binary is in allowlist
	if v.AllowedBinaries[binary] {
		return nil
	}

	// Check if binary base name is allowed (for absolute paths)
	baseName := extractBaseName(binary)
	if v.AllowedBinaries[baseName] {
		return nil
	}

	// Check against allowed patterns
	for _, pattern := range v.AllowedPatterns {
		if pattern.MatchString(cmd) {
			return nil
		}
	}

	return fmt.Errorf("%w: binary %q not in allowlist", ErrCommandBlocked, binary)
}

// ValidateHooks validates a list of hook commands.
// Returns the first error encountered, or nil if all hooks are valid.
func (v *CommandValidator) ValidateHooks(hooks []string) error {
	for i, hook := range hooks {
		if err := v.Validate(hook); err != nil {
			return fmt.Errorf("hook[%d] (%q): %w", i, truncateForError(hook), err)
		}
	}
	return nil
}

// MustValidate panics if the command is not valid.
// Deprecated: Use Validate() instead and handle errors properly.
// This function should ONLY be used in tests where panics are expected.
// Using this in production code WILL cause application crashes.
func (v *CommandValidator) MustValidate(cmd string) {
	if err := v.Validate(cmd); err != nil {
		panic(fmt.Sprintf("command validation failed: %v", err))
	}
}

// extractBinary extracts the binary name from a command string.
// Handles prefixes like "cd /path &&" and environment variables.
func extractBinary(cmd string) string {
	cmd = strings.TrimSpace(cmd)

	// Skip common prefixes like "cd /path &&"
	// Note: We've already blocked && in the default blocklist,
	// but this handles cases where it's explicitly allowed
	if idx := strings.Index(cmd, " "); idx > 0 {
		firstWord := cmd[:idx]
		if firstWord == "cd" {
			// Find the && and skip past it
			if andIdx := strings.Index(cmd, "&&"); andIdx > 0 {
				cmd = strings.TrimSpace(cmd[andIdx+2:])
			}
		}
	}

	// Handle inline environment variables (VAR=value cmd)
	for strings.Contains(cmd, "=") {
		parts := strings.SplitN(cmd, " ", 2)
		if len(parts) < 2 {
			break
		}
		if !strings.Contains(parts[0], "=") {
			break
		}
		cmd = strings.TrimSpace(parts[1])
	}

	// Extract first word (the binary)
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// extractBaseName returns the base name of a path.
func extractBaseName(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// truncateForError truncates a string for use in error messages.
func truncateForError(s string) string {
	const maxLen = 50
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// copyMap creates a shallow copy of a map.
func copyMap(m map[string]bool) map[string]bool {
	result := make(map[string]bool, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
