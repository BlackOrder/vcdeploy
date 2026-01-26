// Package services provides service layer interfaces and implementations for vcdeploy.
package services

import (
	"errors"
	"fmt"
)

// Sentinel errors for common cases.
var (
	// ErrNotFound indicates the requested resource was not found.
	ErrNotFound = errors.New("resource not found")

	// ErrDuplicate indicates the resource already exists.
	ErrDuplicate = errors.New("resource already exists")

	// ErrInvalidInput indicates the input validation failed.
	ErrInvalidInput = errors.New("invalid input")

	// ErrUnauthorized indicates the operation is not authorized.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates the operation is forbidden.
	ErrForbidden = errors.New("forbidden")

	// ErrConflict indicates a conflict with current state.
	ErrConflict = errors.New("conflict with current state")

	// ErrInternal indicates an internal error.
	ErrInternal = errors.New("internal error")

	// ErrAlreadyExists indicates the resource already exists (alias for ErrDuplicate).
	ErrAlreadyExists = ErrDuplicate

	// ErrOperationFailed indicates a general operation failure.
	ErrOperationFailed = errors.New("operation failed")

	// ErrContextCancelled indicates the context was cancelled.
	ErrContextCancelled = errors.New("context cancelled")

	// ErrTimeout indicates the operation timed out.
	ErrTimeout = errors.New("operation timed out")
)

// ServiceError wraps errors with additional context.
type ServiceError struct {
	Op       string // Operation that failed (e.g., "users.Create")
	Err      error  // Underlying error
	Resource string // Resource type (e.g., "user", "project")
	ID       string // Resource identifier if applicable
}

func (e *ServiceError) Error() string {
	if e.ID != "" {
		return fmt.Sprintf("%s: %s %s: %v", e.Op, e.Resource, e.ID, e.Err)
	}
	if e.Resource != "" {
		return fmt.Sprintf("%s: %s: %v", e.Op, e.Resource, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *ServiceError) Unwrap() error {
	return e.Err
}

// Error constructors for convenience.

// NotFound creates a not found error with context.
func NotFound(op, resource, id string) error {
	return &ServiceError{Op: op, Err: ErrNotFound, Resource: resource, ID: id}
}

// Duplicate creates a duplicate error with context.
func Duplicate(op, resource, id string) error {
	return &ServiceError{Op: op, Err: ErrDuplicate, Resource: resource, ID: id}
}

// InvalidInput creates an invalid input error with context.
func InvalidInput(op, message string) error {
	return &ServiceError{Op: op, Err: fmt.Errorf("%s: %w", message, ErrInvalidInput)}
}

// Internal creates an internal error with context.
func Internal(op string, err error) error {
	return &ServiceError{Op: op, Err: fmt.Errorf("%w: %w", err, ErrInternal)} //nolint:errorlint // intentional double wrap
}

// Unauthorized creates an unauthorized error with context.
func Unauthorized(op, message string) error {
	return &ServiceError{Op: op, Err: fmt.Errorf("%s: %w", message, ErrUnauthorized)}
}

// Forbidden creates a forbidden error with context.
func Forbidden(op, message string) error {
	return &ServiceError{Op: op, Err: fmt.Errorf("%s: %w", message, ErrForbidden)}
}

// Conflict creates a conflict error with context.
func Conflict(op, resource, id string) error {
	return &ServiceError{Op: op, Err: ErrConflict, Resource: resource, ID: id}
}

// Error checking helpers.

// IsNotFound returns true if the error is or wraps ErrNotFound.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsDuplicate returns true if the error is or wraps ErrDuplicate.
func IsDuplicate(err error) bool {
	return errors.Is(err, ErrDuplicate)
}

// IsInvalidInput returns true if the error is or wraps ErrInvalidInput.
func IsInvalidInput(err error) bool {
	return errors.Is(err, ErrInvalidInput)
}

// IsUnauthorized returns true if the error is or wraps ErrUnauthorized.
func IsUnauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}

// IsForbidden returns true if the error is or wraps ErrForbidden.
func IsForbidden(err error) bool {
	return errors.Is(err, ErrForbidden)
}

// IsConflict returns true if the error is or wraps ErrConflict.
func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}

// IsInternal returns true if the error is or wraps ErrInternal.
func IsInternal(err error) bool {
	return errors.Is(err, ErrInternal)
}
