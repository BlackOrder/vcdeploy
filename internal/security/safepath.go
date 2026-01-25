// Package security provides safe path operations to prevent path traversal attacks.
package security

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrPathTraversal is returned when a path operation would escape the base directory.
var ErrPathTraversal = errors.New("path traversal detected: path escapes base directory")

// ErrAbsolutePath is returned when an absolute path is provided where a relative path is expected.
var ErrAbsolutePath = errors.New("absolute path not allowed: expected relative path")

// ErrEmptyPath is returned when an empty path is provided.
var ErrEmptyPath = errors.New("empty path not allowed")

// SafeJoin safely joins a base directory with an untrusted relative path.
// It ensures the resulting path stays within the base directory.
// Returns ErrPathTraversal if the path would escape the base directory.
// Returns ErrAbsolutePath if the relative path is absolute.
// Returns ErrEmptyPath if either path is empty.
func SafeJoin(base, relativePath string) (string, error) {
	if base == "" || relativePath == "" {
		return "", ErrEmptyPath
	}

	// Clean the base path first
	cleanBase := filepath.Clean(base)

	// Check if relativePath is absolute
	if filepath.IsAbs(relativePath) {
		return "", ErrAbsolutePath
	}

	// Join and clean the paths
	joined := filepath.Join(cleanBase, relativePath)
	cleanJoined := filepath.Clean(joined)

	// Verify the result is within the base directory
	// We add a separator to cleanBase to ensure we match the directory, not a prefix
	// e.g., /base should not match /base-other
	if !strings.HasPrefix(cleanJoined+string(filepath.Separator), cleanBase+string(filepath.Separator)) &&
		cleanJoined != cleanBase {
		return "", ErrPathTraversal
	}

	return cleanJoined, nil
}

// IsWithinBase checks if a path is safely within a base directory.
// Returns true if the path is within the base directory, false otherwise.
// Both paths are cleaned before comparison.
func IsWithinBase(base, path string) bool {
	if base == "" || path == "" {
		return false
	}

	cleanBase := filepath.Clean(base)
	cleanPath := filepath.Clean(path)

	// Convert to absolute paths for comparison
	// This handles cases where one path is absolute and one is relative
	absBase, err := filepath.Abs(cleanBase)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return false
	}

	// Check if path starts with base directory
	// We add separator to prevent prefix matching (e.g., /base vs /base-other)
	return strings.HasPrefix(absPath+string(filepath.Separator), absBase+string(filepath.Separator)) ||
		absPath == absBase
}

// ValidateRelativePath checks if a path is relative and does not contain
// dangerous traversal sequences.
// Returns nil if the path is safe, or an error describing the problem.
func ValidateRelativePath(path string) error {
	if path == "" {
		return ErrEmptyPath
	}

	if filepath.IsAbs(path) {
		return ErrAbsolutePath
	}

	// Clean the path and check if it would escape current directory
	cleaned := filepath.Clean(path)

	// Check for directory traversal
	// A cleaned path that starts with ".." would escape the current directory
	if strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return ErrPathTraversal
	}

	return nil
}

// SanitizePath removes dangerous sequences from a path.
// This is a defensive measure but should NOT be relied upon alone.
// Always use SafeJoin or IsWithinBase for actual path validation.
func SanitizePath(path string) string {
	// Clean the path to resolve . and ..
	cleaned := filepath.Clean(path)

	// Remove any remaining .. sequences (shouldn't happen after Clean, but defensive)
	for strings.Contains(cleaned, "..") {
		cleaned = strings.ReplaceAll(cleaned, "..", "")
	}

	// Remove null bytes which could be used for truncation attacks
	cleaned = strings.ReplaceAll(cleaned, "\x00", "")

	return filepath.Clean(cleaned)
}
