// Package testutil provides test setup helpers for E2E, CLI, and integration tests.
package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestSuite provides a base test suite with common setup/teardown logic.
type TestSuite struct {
	Config     *Config
	Client     *HTTPClient
	Seeder     *Seeder
	Cleanup    *Cleanup
	Assertions *Assertions
	T          *testing.T
}

// NewTestSuite creates a new test suite with default configuration.
func NewTestSuite(t *testing.T) *TestSuite {
	cfg := GetConfig()
	client := NewHTTPClient(cfg.MasterHTTPURL, cfg.APIToken)

	return &TestSuite{
		Config:     cfg,
		Client:     client,
		Seeder:     NewSeeder(client),
		Cleanup:    NewCleanup(client),
		Assertions: NewAssertions(t),
		T:          t,
	}
}

// SetupParallel marks the test for parallel execution if allowed.
func (s *TestSuite) SetupParallel() {
	SetupParallel(s.T)
}

// WaitForMaster waits for the master server to be available.
func (s *TestSuite) WaitForMaster() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.Config.Timeout)
	defer cancel()
	return WaitForEndpoint(ctx, s.Config.APIURL("/health"), s.Config.Timeout)
}

// MustWaitForMaster waits for the master or fails the test.
func (s *TestSuite) MustWaitForMaster() {
	s.T.Helper()
	if err := s.WaitForMaster(); err != nil {
		s.T.Skipf("Master not available: %v", err)
	}
}

// Login authenticates with the API and sets the token.
func (s *TestSuite) Login(username, password string) error {
	resp, err := s.Client.Post("/api/v1/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := ReadBody(resp)
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := DecodeJSON(resp, &result); err != nil {
		return fmt.Errorf("failed to decode login response: %w", err)
	}

	if token, ok := result["token"].(string); ok {
		s.Client.SetToken(token)
		return nil
	}

	return fmt.Errorf("no token in login response")
}

// MustLogin authenticates or fails the test.
func (s *TestSuite) MustLogin(username, password string) {
	s.T.Helper()
	if err := s.Login(username, password); err != nil {
		s.T.Fatalf("Login failed: %v", err)
	}
}

// APITestContext provides context for individual API tests.
type APITestContext struct {
	*TestSuite
	createdResources []resourceRef
}

type resourceRef struct {
	kind string
	id   interface{}
}

// NewAPITestContext creates a new API test context.
func NewAPITestContext(t *testing.T) *APITestContext {
	return &APITestContext{
		TestSuite:        NewTestSuite(t),
		createdResources: make([]resourceRef, 0),
	}
}

// TrackResource tracks a resource for cleanup.
func (ctx *APITestContext) TrackResource(kind string, id interface{}) {
	ctx.createdResources = append(ctx.createdResources, resourceRef{kind: kind, id: id})
}

// CleanupResources cleans up all tracked resources.
func (ctx *APITestContext) CleanupResources() {
	// Clean up in reverse order
	for i := len(ctx.createdResources) - 1; i >= 0; i-- {
		res := ctx.createdResources[i]
		var err error
		switch res.kind {
		case "user":
			err = ctx.Cleanup.DeleteUser(res.id)
		case "project":
			err = ctx.Cleanup.DeleteProject(res.id)
		case "secret":
			err = ctx.Cleanup.DeleteSecret(res.id)
		case "api-key":
			err = ctx.Cleanup.DeleteAPIKey(res.id)
		}
		if err != nil {
			ctx.T.Logf("Warning: failed to cleanup %s %v: %v", res.kind, res.id, err)
		}
	}
}

// CLITestContext provides context for CLI tests.
type CLITestContext struct {
	Config     *Config
	CLI        *CLIRunner
	Assertions *CLIAssertions
	T          *testing.T
}

// NewCLITestContext creates a new CLI test context.
func NewCLITestContext(t *testing.T) *CLITestContext {
	cfg := GetConfig()
	return &CLITestContext{
		Config:     cfg,
		CLI:        NewCLIRunner(cfg.VCDeployBinary),
		Assertions: NewCLIAssertions(t),
		T:          t,
	}
}

// SetupParallel marks the test for parallel execution if allowed.
func (ctx *CLITestContext) SetupParallel() {
	SetupParallel(ctx.T)
}

// Retry retries a function until it succeeds or times out.
func Retry(t *testing.T, timeout time.Duration, interval time.Duration, fn func() error) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("retry timed out after %v: %w", timeout, lastErr)
}

// Eventually asserts that a condition becomes true within the timeout.
func Eventually(t *testing.T, timeout time.Duration, condition func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("condition not met within %v: %s", timeout, msg)
}

// Never asserts that a condition remains false for the duration.
func Never(t *testing.T, duration time.Duration, condition func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if condition() {
			t.Errorf("condition became true: %s", msg)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
