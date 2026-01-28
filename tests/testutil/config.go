// Package testutil provides shared testing utilities for E2E, CLI, and integration tests.
package testutil

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

// Config holds test configuration loaded from environment variables.
type Config struct {
	// Master URLs
	MasterHTTPURL string
	MasterGRPCURL string

	// SSH Target
	TargetSSHHost string
	TargetSSHPort int
	TargetSSHUser string
	TargetSSHPass string

	// Git Server
	GitServerURL  string
	GitServerUser string
	GitServerPass string

	// Authentication
	APIToken      string
	AdminUsername string
	AdminPassword string

	// Test settings
	Timeout       time.Duration
	RetryInterval time.Duration
	Parallel      bool

	// Binary paths (for CLI tests)
	VCDeployBinary      string
	VCDeployAgentBinary string
}

var (
	globalConfig *Config
	configOnce   sync.Once
)

// GetConfig returns the test configuration, loading it once from environment.
func GetConfig() *Config {
	configOnce.Do(func() {
		globalConfig = loadConfig()
	})
	return globalConfig
}

func loadConfig() *Config {
	sshPort, _ := strconv.Atoi(getEnvOrDefault("E2E_TARGET_SSH_PORT", "12222"))
	timeout, _ := time.ParseDuration(getEnvOrDefault("E2E_TIMEOUT", "2m"))
	retryInterval, _ := time.ParseDuration(getEnvOrDefault("E2E_RETRY_INTERVAL", "500ms"))

	return &Config{
		// Master URLs
		MasterHTTPURL: getEnvOrDefault("E2E_MASTER_HTTP_URL", "http://localhost:18080"),
		MasterGRPCURL: getEnvOrDefault("E2E_MASTER_GRPC_URL", "localhost:19090"),

		// SSH Target
		TargetSSHHost: getEnvOrDefault("E2E_TARGET_SSH_HOST", "localhost"),
		TargetSSHPort: sshPort,
		TargetSSHUser: getEnvOrDefault("E2E_TARGET_SSH_USER", "deploy"),
		TargetSSHPass: getEnvOrDefault("E2E_TARGET_SSH_PASS", "deploypass"),

		// Git Server
		GitServerURL:  getEnvOrDefault("E2E_GIT_SERVER_URL", "http://localhost:13000"),
		GitServerUser: getEnvOrDefault("E2E_GIT_USER", "testuser"),
		GitServerPass: getEnvOrDefault("E2E_GIT_PASS", "testpass"),

		// Authentication
		APIToken:      getEnvOrDefault("E2E_API_TOKEN", "test-api-token"),
		AdminUsername: getEnvOrDefault("E2E_ADMIN_USER", "admin"),
		AdminPassword: getEnvOrDefault("E2E_ADMIN_PASS", "Changeme12345!"),

		// Test settings
		Timeout:       timeout,
		RetryInterval: retryInterval,
		Parallel:      os.Getenv("TEST_NO_PARALLEL") == "",

		// Binary paths
		VCDeployBinary:      getEnvOrDefault("VCDEPLOY_BINARY", "./bin/vcdeploy"),
		VCDeployAgentBinary: getEnvOrDefault("VCDEPLOY_AGENT_BINARY", "./bin/vcdeploy-agent"),
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// SkipIfNoParallel skips the test if parallel execution is disabled.
// This is useful for tests that must run in parallel.
func SkipIfNoParallel(t TestingT) {
	t.Helper()
	if os.Getenv("TEST_NO_PARALLEL") != "" {
		t.Skip("Skipping parallel-only test: TEST_NO_PARALLEL is set")
	}
}

// SetupParallel marks the test as parallel if TEST_NO_PARALLEL is not set.
func SetupParallel(t TestingT) {
	t.Helper()
	if os.Getenv("TEST_NO_PARALLEL") == "" {
		if pt, ok := t.(ParallelTest); ok {
			pt.Parallel()
		}
	}
}

// TestingT is the minimum interface for testing.T that we need.
type TestingT interface {
	Helper()
	Skip(args ...interface{})
	Skipf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Logf(format string, args ...interface{})
}

// ParallelTest is an interface for tests that support Parallel().
type ParallelTest interface {
	Parallel()
}

// MustEnv gets an environment variable or fails the test.
func MustEnv(t TestingT, key string) string {
	t.Helper()
	val := os.Getenv(key)
	if val == "" {
		t.Fatalf("required environment variable %s not set", key)
	}
	return val
}

// APIURL returns the full API URL for a given path.
func (c *Config) APIURL(path string) string {
	return fmt.Sprintf("%s/api/v1%s", c.MasterHTTPURL, path)
}

// WebURL returns the full web URL for a given path.
func (c *Config) WebURL(path string) string {
	return fmt.Sprintf("%s%s", c.MasterHTTPURL, path)
}
