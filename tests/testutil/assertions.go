// Package testutil provides shared testing utilities for E2E, CLI, and integration tests.
package testutil

import (
	"io"
	"net/http"
	"testing"
)

// Assertions provides test assertion helpers.
type Assertions struct {
	t *testing.T
}

// NewAssertions creates a new Assertions instance.
func NewAssertions(t *testing.T) *Assertions {
	return &Assertions{t: t}
}

// StatusOK asserts that the response status is 200 OK.
func (a *Assertions) StatusOK(resp *http.Response) {
	a.t.Helper()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		a.t.Errorf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}
}

// StatusCreated asserts that the response status is 201 Created.
func (a *Assertions) StatusCreated(resp *http.Response) {
	a.t.Helper()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		a.t.Errorf("expected status 201, got %d: %s", resp.StatusCode, string(body))
	}
}

// StatusNoContent asserts that the response status is 204 No Content.
func (a *Assertions) StatusNoContent(resp *http.Response) {
	a.t.Helper()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		a.t.Errorf("expected status 204, got %d: %s", resp.StatusCode, string(body))
	}
}

// StatusBadRequest asserts that the response status is 400 Bad Request.
func (a *Assertions) StatusBadRequest(resp *http.Response) {
	a.t.Helper()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		a.t.Errorf("expected status 400, got %d: %s", resp.StatusCode, string(body))
	}
}

// StatusUnauthorized asserts that the response status is 401 Unauthorized.
func (a *Assertions) StatusUnauthorized(resp *http.Response) {
	a.t.Helper()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		a.t.Errorf("expected status 401, got %d: %s", resp.StatusCode, string(body))
	}
}

// StatusForbidden asserts that the response status is 403 Forbidden.
func (a *Assertions) StatusForbidden(resp *http.Response) {
	a.t.Helper()
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		a.t.Errorf("expected status 403, got %d: %s", resp.StatusCode, string(body))
	}
}

// StatusNotFound asserts that the response status is 404 Not Found.
func (a *Assertions) StatusNotFound(resp *http.Response) {
	a.t.Helper()
	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		a.t.Errorf("expected status 404, got %d: %s", resp.StatusCode, string(body))
	}
}

// StatusMethodNotAllowed asserts that the response status is 405 Method Not Allowed.
func (a *Assertions) StatusMethodNotAllowed(resp *http.Response) {
	a.t.Helper()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		body, _ := io.ReadAll(resp.Body)
		a.t.Errorf("expected status 405, got %d: %s", resp.StatusCode, string(body))
	}
}

// StatusConflict asserts that the response status is 409 Conflict.
func (a *Assertions) StatusConflict(resp *http.Response) {
	a.t.Helper()
	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		a.t.Errorf("expected status 409, got %d: %s", resp.StatusCode, string(body))
	}
}

// StatusInternalServerError asserts that the response status is 500 Internal Server Error.
func (a *Assertions) StatusInternalServerError(resp *http.Response) {
	a.t.Helper()
	if resp.StatusCode != http.StatusInternalServerError {
		body, _ := io.ReadAll(resp.Body)
		a.t.Errorf("expected status 500, got %d: %s", resp.StatusCode, string(body))
	}
}

// Status asserts that the response status matches the expected status.
func (a *Assertions) Status(resp *http.Response, expected int) {
	a.t.Helper()
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		a.t.Errorf("expected status %d, got %d: %s", expected, resp.StatusCode, string(body))
	}
}

// NoServerError asserts that the response status is not 5xx.
func (a *Assertions) NoServerError(resp *http.Response) {
	a.t.Helper()
	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(resp.Body)
		a.t.Errorf("unexpected server error %d: %s", resp.StatusCode, string(body))
	}
}

// HasField asserts that a map has the given field.
func (a *Assertions) HasField(m map[string]interface{}, field string) {
	a.t.Helper()
	if _, ok := m[field]; !ok {
		a.t.Errorf("expected field %q to exist", field)
	}
}

// HasFields asserts that a map has all the given fields.
func (a *Assertions) HasFields(m map[string]interface{}, fields ...string) {
	a.t.Helper()
	for _, field := range fields {
		if _, ok := m[field]; !ok {
			a.t.Errorf("expected field %q to exist", field)
		}
	}
}

// Equal asserts that two values are equal.
func (a *Assertions) Equal(got, want interface{}) {
	a.t.Helper()
	if got != want {
		a.t.Errorf("got %v, want %v", got, want)
	}
}

// NotEqual asserts that two values are not equal.
func (a *Assertions) NotEqual(got, want interface{}) {
	a.t.Helper()
	if got == want {
		a.t.Errorf("got %v, want it to be different from %v", got, want)
	}
}

// True asserts that a condition is true.
func (a *Assertions) True(condition bool, msg string) {
	a.t.Helper()
	if !condition {
		a.t.Errorf("assertion failed: %s", msg)
	}
}

// False asserts that a condition is false.
func (a *Assertions) False(condition bool, msg string) {
	a.t.Helper()
	if condition {
		a.t.Errorf("assertion failed: %s", msg)
	}
}

// Nil asserts that a value is nil.
func (a *Assertions) Nil(v interface{}) {
	a.t.Helper()
	if v != nil {
		a.t.Errorf("expected nil, got %v", v)
	}
}

// NotNil asserts that a value is not nil.
func (a *Assertions) NotNil(v interface{}) {
	a.t.Helper()
	if v == nil {
		a.t.Error("expected non-nil value")
	}
}

// NoError asserts that an error is nil.
func (a *Assertions) NoError(err error) {
	a.t.Helper()
	if err != nil {
		a.t.Errorf("unexpected error: %v", err)
	}
}

// Error asserts that an error is not nil.
func (a *Assertions) Error(err error) {
	a.t.Helper()
	if err == nil {
		a.t.Error("expected error, got nil")
	}
}

// Contains asserts that a string contains a substring.
func (a *Assertions) Contains(s, substr string) {
	a.t.Helper()
	if !containsString(s, substr) {
		a.t.Errorf("expected %q to contain %q", s, substr)
	}
}

// NotContains asserts that a string does not contain a substring.
func (a *Assertions) NotContains(s, substr string) {
	a.t.Helper()
	if containsString(s, substr) {
		a.t.Errorf("expected %q to not contain %q", s, substr)
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
