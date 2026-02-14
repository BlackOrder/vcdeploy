// Package storage provides database operations for vcdeploy.
package storage

import "errors"

// Sentinel errors for storage operations.
// All packages should import these from storage to ensure errors.Is() works correctly.
var (
	// ErrNotFound is returned when a requested record does not exist.
	ErrNotFound = errors.New("not found")

	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")
)
