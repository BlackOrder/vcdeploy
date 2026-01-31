// Package testutil provides shared testing utilities for vcdeploy.
package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// TestDB provides an isolated SQLite database for testing.
type TestDB struct {
	DB      *storage.DB
	Path    string
	t       *testing.T
	cleanup func()
}

// Store returns the storage.Store interface for use with services.
// This allows tests to use the Store interface consistently.
func (tdb *TestDB) Store() storage.Store {
	return tdb.DB
}

// NewTestDB creates a new isolated SQLite database for testing.
// The database is automatically cleaned up when the test completes.
func NewTestDB(t *testing.T) *TestDB {
	t.Helper()

	// Create a temp directory for this test
	tmpDir, err := os.MkdirTemp("", "vcdeploy-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create the database with migrations
	db, err := storage.New(dbPath, zap.NewNop())
	if err != nil {
		os.RemoveAll(tmpDir) //nolint:errcheck // Best effort cleanup in test
		t.Fatalf("failed to create test database: %v", err)
	}

	testDB := &TestDB{
		DB:   db,
		Path: dbPath,
		t:    t,
		cleanup: func() {
			if err := db.Close(); err != nil {
				t.Logf("warning: failed to close test db: %v", err)
			}
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Logf("warning: failed to remove temp dir: %v", err)
			}
		},
	}

	t.Cleanup(testDB.cleanup)
	return testDB
}

// Close closes the database connection. This is called automatically
// via t.Cleanup, but can be called manually if needed.
func (tdb *TestDB) Close() error {
	if tdb.DB != nil {
		return tdb.DB.Close()
	}
	return nil
}

// TestDBPool provides a pool of test databases for parallel testing.
// This is useful when running many tests in parallel that each need
// their own database instance.
type TestDBPool struct {
	mu      sync.Mutex
	dbs     []*TestDB
	maxSize int
	t       *testing.T
}

// NewTestDBPool creates a new pool of test databases.
func NewTestDBPool(t *testing.T, maxSize int) *TestDBPool {
	t.Helper()
	pool := &TestDBPool{
		dbs:     make([]*TestDB, 0, maxSize),
		maxSize: maxSize,
		t:       t,
	}
	t.Cleanup(pool.Close)
	return pool
}

// Get returns a test database from the pool or creates a new one.
func (p *TestDBPool) Get(t *testing.T) *TestDB {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()

	// Create a new TestDB for this test
	testDB := NewTestDB(t)
	p.dbs = append(p.dbs, testDB)
	return testDB
}

// Close closes all databases in the pool.
func (p *TestDBPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, db := range p.dbs {
		if err := db.Close(); err != nil {
			p.t.Logf("warning: failed to close pooled db: %v", err)
		}
	}
	p.dbs = nil
}

// InMemoryDB creates an in-memory SQLite database for fast testing.
// Note: Each call creates an independent in-memory database that is lost when closed.
func InMemoryDB(t *testing.T) *TestDB {
	t.Helper()

	// Use a temporary file-based database with fast WAL mode instead of true in-memory
	// because modernc/sqlite has issues with shared memory mode
	tmpDir, err := os.MkdirTemp("", "vcdeploy-inmem-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "inmem.db")

	db, err := storage.New(dbPath, zap.NewNop())
	if err != nil {
		os.RemoveAll(tmpDir) //nolint:errcheck // Best effort cleanup in test
		t.Fatalf("failed to create in-memory database: %v", err)
	}

	testDB := &TestDB{
		DB:   db,
		Path: dbPath,
		t:    t,
		cleanup: func() {
			if err := db.Close(); err != nil {
				t.Logf("warning: failed to close in-memory db: %v", err)
			}
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Logf("warning: failed to remove temp dir: %v", err)
			}
		},
	}

	t.Cleanup(testDB.cleanup)
	return testDB
}

// TestContext returns a context that is canceled when the test completes.
func TestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}

// RequireNoError is a test helper that fails the test if err is not nil.
func RequireNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err != nil {
		if len(msgAndArgs) > 0 {
			t.Fatalf("%s: %v", fmt.Sprint(msgAndArgs...), err)
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

// RequireError is a test helper that fails the test if err is nil.
func RequireError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err == nil {
		if len(msgAndArgs) > 0 {
			t.Fatalf("%s: expected error but got nil", fmt.Sprint(msgAndArgs...))
		} else {
			t.Fatalf("expected error but got nil")
		}
	}
}
