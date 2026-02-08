// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// CachedStore wraps a MemoryStore and provides access to the underlying DB for migrations.
type CachedStore struct {
	*MemoryStore
	db *DB
}

// UnderlyingDB returns the underlying DB for direct access (e.g., migrations).
func (cs *CachedStore) UnderlyingDB() *DB {
	return cs.db
}

// Conn returns the underlying database connection for health checks.
// This overrides MemoryStore.Conn() which returns nil.
func (cs *CachedStore) Conn() *sql.DB {
	return cs.db.Conn()
}

// Close closes the CachedStore, flushing pending writes and closing the database.
func (cs *CachedStore) Close() error {
	// Signal MemoryStore workers to stop and drain pending writes.
	// We pass nil DB pointers to MemoryStore config, so it won't try to close them.
	// The done channel is closed to signal workers to stop.
	close(cs.MemoryStore.done)
	cs.MemoryStore.wg.Wait()

	// Close the underlying database (we own this connection)
	if err := cs.db.Close(); err != nil {
		return fmt.Errorf("closing database: %w", err)
	}
	return nil
}

// Reload refreshes all in-memory data from the underlying SQLite database.
// This is used after direct DB modifications (e.g., import) to sync the cache.
func (cs *CachedStore) Reload(ctx context.Context) error {
	return cs.MemoryStore.LoadFromDB(ctx, cs.db)
}

// NewCachedStore creates a MemoryStore backed by SQLite.
// It opens the database, runs migrations, loads existing data into memory,
// and starts background persistence workers.
//
// The returned CachedStore serves all reads from memory and batches writes
// to SQLite, eliminating SQLITE_BUSY errors from concurrent access.
func NewCachedStore(dbPath string, logger *zap.Logger) (*CachedStore, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Open the SQLite database (handles migrations)
	db, err := New(dbPath, logger)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Get the underlying connection for MemoryStore
	conn := db.Conn()

	// Configure MemoryStore with the database connection.
	// We use only CoreDB and let all writes go through a single channel
	// to avoid SQLite locking issues with concurrent writes.
	cfg := DefaultMemoryStoreConfig()
	cfg.CoreDB = conn
	// Don't set other DB pointers - MemoryStore will only start one worker
	// and we manage DB lifecycle in CachedStore.Close()
	cfg.Logger = logger

	// Create the MemoryStore
	memStore := NewMemoryStore(&cfg)

	// Load existing data from SQLite into memory
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := memStore.LoadFromDB(ctx, db); err != nil {
		// Clean up on failure
		close(memStore.done) // Signal workers to stop
		memStore.wg.Wait()   // Wait for workers to finish
		_ = db.Close()       // #nosec G104 - cleanup on error path
		return nil, fmt.Errorf("loading data into memory: %w", err)
	}

	logger.Info("storage initialized with memory cache",
		zap.String("db_path", dbPath),
	)

	return &CachedStore{
		MemoryStore: memStore,
		db:          db,
	}, nil
}
