//go:build e2e

// Package e2e provides end-to-end API tests for vcdeploy.
package e2e

import (
	"os"
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestMain runs setup/teardown for all E2E tests.
func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}

// setupTest creates a new test context and waits for the master.
func setupTest(t *testing.T) *testutil.APITestContext {
	t.Helper()
	ctx := testutil.NewAPITestContext(t)
	ctx.MustWaitForMaster()
	return ctx
}
