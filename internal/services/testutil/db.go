// Package testutil provides test helpers for service tests.
package testutil

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/security"
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
		_ = db.Close() // #nosec G104 - best effort cleanup in test
	}

	return db, cleanup
}

// NewTestStore creates a temporary SQLite database for testing and returns
// the storage.Store interface. This is the preferred method for new tests.
func NewTestStore(t *testing.T) (storage.Store, func()) {
	t.Helper()
	db, cleanup := NewTestDB(t)
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
		_ = db.Close() // #nosec G104 - best effort cleanup in benchmark
	}

	return db, cleanup
}

// SetupBenchStore creates a temporary SQLite database for benchmarks and returns
// the storage.Store interface. This is the preferred method for new benchmarks.
func SetupBenchStore(b *testing.B) (storage.Store, func()) {
	b.Helper()
	db, cleanup := SetupBenchDB(b)
	return db, cleanup
}

// NewTestMasterKey generates a random MasterKey for test use.
func NewTestMasterKey(t testing.TB) *security.MasterKey {
	t.Helper()
	mk, err := security.GenerateMasterKey()
	if err != nil {
		t.Fatalf("testutil.NewTestMasterKey: %v", err)
	}
	return mk
}

// NewTestKMS creates a KMS instance backed by a test store with a random MasterKey.
// Returns the KMS instance already initialized with an encryption key.
func NewTestKMS(t testing.TB, store storage.Store) *security.KMS {
	t.Helper()
	mk := NewTestMasterKey(t)
	kms, err := security.NewKMS(context.Background(), store, nil, mk)
	if err != nil {
		t.Fatalf("testutil.NewTestKMS: NewKMS: %v", err)
	}
	if err := kms.Initialize(context.Background()); err != nil {
		t.Fatalf("testutil.NewTestKMS: Initialize: %v", err)
	}
	return kms
}
