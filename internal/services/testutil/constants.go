// Package testutil provides test helpers for service tests.
package testutil

import "time"

// Test constants for consistent test data across all service tests.
const (
	// TestTimeout is the default timeout for test operations.
	TestTimeout = 30 * time.Second

	// TestPassword is a valid password that meets strength requirements.
	TestPassword = "StrongP@ss123!"

	// TestWeakPassword is a password that fails strength requirements.
	TestWeakPassword = "weak"

	// TestEmail is a sample email for test users.
	TestEmail = "test@example.com"

	// TestUsername is a sample username for test users.
	TestUsername = "testuser"

	// TestRole is the default role for test users.
	TestRole = "user"

	// TestAdminRole is the admin role.
	TestAdminRole = "admin"

	// TestProjectName is a sample project name.
	TestProjectName = "test-project"

	// TestRepository is a sample git repository URL.
	TestRepository = "https://github.com/example/repo.git"

	// TestBranch is the default branch for test projects.
	TestBranch = "main"

	// TestDeployPath is a sample deployment path.
	TestDeployPath = "/var/www/test"

	// TestAgentID is a sample agent ID.
	TestAgentID = "agent-test-001"

	// TestHostname is a sample hostname.
	TestHostname = "test-host"

	// TestIPAddress is a sample IP address.
	TestIPAddress = "127.0.0.1"

	// TestUserAgent is a sample user agent string.
	TestUserAgent = "Test Agent/1.0"

	// TestTOTPSecret is a sample TOTP secret (test-only, not production credentials).
	TestTOTPSecret = "JBSWY3DPEHPK3PXP" // #nosec G101 - intentional test-only credential

	// TestAPIKeyPrefix is a sample API key prefix.
	TestAPIKeyPrefix = "vc_test_"

	// TestWebhookSecret is a sample webhook secret.
	TestWebhookSecret = "webhook-secret-123"
)

// TestSessionDuration is the default session duration for tests.
var TestSessionDuration = 24 * time.Hour

// TestAPIKeyDuration is the default API key duration for tests.
var TestAPIKeyDuration = 720 * time.Hour // 30 days
