// Package cli provides CLI integration tests for vcdeploy.
//
// These tests verify the vcdeploy CLI commands work correctly against
// a running master server. They are gated behind the "cli" build tag
// and require a properly configured test environment.
//
// To run these tests:
//
//	go test -tags=cli ./tests/cli/...
//
// Required environment variables:
//   - VCDEPLOY_TEST_MASTER_URL: URL of the test master server
//   - VCDEPLOY_TEST_API_TOKEN: API token for authentication
//   - VCDEPLOY_TEST_BINARY: Path to the vcdeploy binary
package cli
