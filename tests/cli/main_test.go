//go:build cli

// Package cli provides CLI command tests for vcdeploy.
package cli

import (
	"os"
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// cli is the shared CLI runner for tests.
var cli *testutil.CLIRunner

// TestMain runs setup/teardown for all CLI tests.
func TestMain(m *testing.M) {
	cfg := testutil.GetConfig()
	cli = testutil.NewCLIRunner(cfg.VCDeployBinary)

	code := m.Run()
	os.Exit(code)
}

// setupTest creates a new CLI test context.
func setupTest(t *testing.T) *testutil.CLITestContext {
	t.Helper()
	return testutil.NewCLITestContext(t)
}

// setupTestParallel creates a new CLI test context for parallel tests.
func setupTestParallel(t *testing.T) *testutil.CLITestContext {
	t.Helper()
	testutil.SetupParallel(t)
	return setupTest(t)
}
