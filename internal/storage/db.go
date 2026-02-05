// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"go.uber.org/zap"
	_ "modernc.org/sqlite" // sqlite driver for database/sql
)

// DB wraps the SQLite database connection.
type DB struct {
	conn   *sql.DB
	path   string
	logger *zap.Logger
}

// New creates a new database connection.
func New(path string, logger *zap.Logger) (*DB, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Configure connection pool for SQLite
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := conn.Ping(); err != nil {
		conn.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	db := &DB{conn: conn, path: path, logger: logger}

	// Handle migration from legacy inline schema
	if err := db.migrateFromLegacy(); err != nil {
		conn.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("legacy migration check: %w", err)
	}

	// Run versioned migrations
	if err := db.MigrateUp(context.Background()); err != nil {
		conn.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return db, nil
}

// Open is an alias for New
func Open(path string) (*DB, error) {
	return New(path, nil)
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying sql.DB connection.
// Use this when you need direct database access (e.g., for KMS initialization).
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// RunInTransaction executes the given function within a database transaction.
// If the function returns an error, the transaction is rolled back.
// Otherwise, the transaction is committed.
func (db *DB) RunInTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Join(fmt.Errorf("rollback failed: %w", rbErr), err)
		}
		return fmt.Errorf("transaction function: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// --- Helper functions ---

func mapToJSON(m map[string]string) string {
	if len(m) == 0 { // nil map has len 0
		return "{}"
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func jsonToMap(s string) map[string]string {
	if s == "" || s == "{}" {
		return make(map[string]string)
	}
	result := make(map[string]string)
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return make(map[string]string)
	}
	return result
}

// Backup creates a backup of the database
func (db *DB) Backup(destPath string) error {
	// Close current operations and copy file
	src, err := os.Open(db.path)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(destPath) // #nosec G304 - destPath is admin-controlled backup destination
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy database: %w", err)
	}
	return nil
}
