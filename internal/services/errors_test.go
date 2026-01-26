package services

import (
	"errors"
	"testing"
)

func TestNotFound(t *testing.T) {
	err := NotFound("users.GetByID", "user", "123")

	if !IsNotFound(err) {
		t.Error("Expected IsNotFound to return true")
	}

	if !errors.Is(err, ErrNotFound) {
		t.Error("Expected errors.Is to match ErrNotFound")
	}

	expected := "users.GetByID: user 123: resource not found"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

func TestDuplicate(t *testing.T) {
	err := Duplicate("users.Create", "user", "admin")

	if !IsDuplicate(err) {
		t.Error("Expected IsDuplicate to return true")
	}

	if !errors.Is(err, ErrDuplicate) {
		t.Error("Expected errors.Is to match ErrDuplicate")
	}

	expected := "users.Create: user admin: resource already exists"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

func TestInvalidInput(t *testing.T) {
	err := InvalidInput("users.Create", "username cannot be empty")

	if !IsInvalidInput(err) {
		t.Error("Expected IsInvalidInput to return true")
	}

	if !errors.Is(err, ErrInvalidInput) {
		t.Error("Expected errors.Is to match ErrInvalidInput")
	}
}

func TestInternal(t *testing.T) {
	originalErr := errors.New("database connection failed")
	err := Internal("users.Create", originalErr)

	if !IsInternal(err) {
		t.Error("Expected IsInternal to return true")
	}

	if !errors.Is(err, ErrInternal) {
		t.Error("Expected errors.Is to match ErrInternal")
	}
}

func TestUnauthorized(t *testing.T) {
	err := Unauthorized("users.Delete", "invalid token")

	if !IsUnauthorized(err) {
		t.Error("Expected IsUnauthorized to return true")
	}

	if !errors.Is(err, ErrUnauthorized) {
		t.Error("Expected errors.Is to match ErrUnauthorized")
	}
}

func TestForbidden(t *testing.T) {
	err := Forbidden("users.Delete", "admin required")

	if !IsForbidden(err) {
		t.Error("Expected IsForbidden to return true")
	}

	if !errors.Is(err, ErrForbidden) {
		t.Error("Expected errors.Is to match ErrForbidden")
	}
}

func TestConflict(t *testing.T) {
	err := Conflict("deployments.Create", "deployment", "dep-123")

	if !IsConflict(err) {
		t.Error("Expected IsConflict to return true")
	}

	if !errors.Is(err, ErrConflict) {
		t.Error("Expected errors.Is to match ErrConflict")
	}
}

func TestServiceErrorUnwrap(t *testing.T) {
	err := NotFound("test.Op", "resource", "id")

	var svcErr *ServiceError
	if !errors.As(err, &svcErr) {
		t.Error("Expected errors.As to match ServiceError")
	}

	if svcErr.Op != "test.Op" {
		t.Errorf("Expected Op 'test.Op', got %q", svcErr.Op)
	}

	if svcErr.Resource != "resource" {
		t.Errorf("Expected Resource 'resource', got %q", svcErr.Resource)
	}

	if svcErr.ID != "id" {
		t.Errorf("Expected ID 'id', got %q", svcErr.ID)
	}
}

func TestServiceErrorWithoutID(t *testing.T) {
	err := &ServiceError{
		Op:       "test.Op",
		Err:      ErrNotFound,
		Resource: "user",
	}

	expected := "test.Op: user: resource not found"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

func TestServiceErrorWithoutResource(t *testing.T) {
	err := &ServiceError{
		Op:  "test.Op",
		Err: ErrInternal,
	}

	expected := "test.Op: internal error"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

func TestErrAlreadyExistsAlias(t *testing.T) {
	// ErrAlreadyExists should be an alias for ErrDuplicate
	if !errors.Is(ErrAlreadyExists, ErrDuplicate) {
		t.Error("Expected ErrAlreadyExists to be ErrDuplicate")
	}
}

func TestErrorCheckersWithNil(t *testing.T) {
	tests := []struct {
		name    string
		checker func(error) bool
	}{
		{"IsNotFound", IsNotFound},
		{"IsDuplicate", IsDuplicate},
		{"IsInvalidInput", IsInvalidInput},
		{"IsUnauthorized", IsUnauthorized},
		{"IsForbidden", IsForbidden},
		{"IsConflict", IsConflict},
		{"IsInternal", IsInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.checker(nil) {
				t.Errorf("%s(nil) should return false", tt.name)
			}
		})
	}
}

func TestErrorCheckersWithUnrelatedError(t *testing.T) {
	unrelated := errors.New("some other error")

	tests := []struct {
		name    string
		checker func(error) bool
	}{
		{"IsNotFound", IsNotFound},
		{"IsDuplicate", IsDuplicate},
		{"IsInvalidInput", IsInvalidInput},
		{"IsUnauthorized", IsUnauthorized},
		{"IsForbidden", IsForbidden},
		{"IsConflict", IsConflict},
		{"IsInternal", IsInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.checker(unrelated) {
				t.Errorf("%s(unrelated) should return false", tt.name)
			}
		})
	}
}
