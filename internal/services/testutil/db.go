// Package testutil provides test helpers for service tests.
package testutil

import (
	"path/filepath"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// NewTestLogger returns a no-op logger for tests.
// Using zap.NewNop() ensures tests don't produce log output.
func NewTestLogger(t *testing.T) *zap.Logger {
	t.Helper()
	return zap.NewNop()
}

// NewTestDB creates a temporary SQLite database for testing.
// It returns the database instance and a cleanup function.
// The caller should defer the cleanup function to ensure proper cleanup.
func NewTestDB(t *testing.T) (*storage.DB, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := zap.NewNop()
	db, err := storage.New(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

// SetupBenchDB creates a temporary SQLite database for benchmarks.
// It returns the database instance and a cleanup function.
// The caller should defer the cleanup function to ensure proper cleanup.
func SetupBenchDB(b *testing.B) (*storage.DB, func()) {
	b.Helper()

	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	logger := zap.NewNop()
	db, err := storage.New(dbPath, logger)
	if err != nil {
		b.Fatalf("Failed to create benchmark database: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}
