package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// WriteOpType represents the type of write operation.
type WriteOpType int

const (
	// WriteOpInsert represents an INSERT operation.
	WriteOpInsert WriteOpType = iota
	// WriteOpUpdate represents an UPDATE operation.
	WriteOpUpdate
	// WriteOpDelete represents a DELETE operation.
	WriteOpDelete
)

// String returns the string representation of WriteOpType.
func (t WriteOpType) String() string {
	switch t {
	case WriteOpInsert:
		return "INSERT"
	case WriteOpUpdate:
		return "UPDATE"
	case WriteOpDelete:
		return "DELETE"
	default:
		return "UNKNOWN"
	}
}

// WriteOp represents a pending write operation to be batched.
type WriteOp struct {
	Type      WriteOpType
	Table     string
	Data      any
	Timestamp time.Time
}

// NewWriteOp creates a new WriteOp with the current timestamp.
func NewWriteOp(opType WriteOpType, table string, data any) WriteOp {
	return WriteOp{
		Type:      opType,
		Table:     table,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// WriteBatcher handles batching write operations and flushing to SQLite.
type WriteBatcher struct {
	db            *sql.DB
	executor      WriteOpExecutor
	flushInterval time.Duration
	batchSize     int
}

// WriteOpExecutor is a function type that executes a single write operation within a transaction.
type WriteOpExecutor func(tx *sql.Tx, op WriteOp) error

// NewWriteBatcher creates a new WriteBatcher.
func NewWriteBatcher(db *sql.DB, executor WriteOpExecutor, flushInterval time.Duration, batchSize int) *WriteBatcher {
	return &WriteBatcher{
		db:            db,
		executor:      executor,
		flushInterval: flushInterval,
		batchSize:     batchSize,
	}
}

// FlushBatch executes a batch of write operations in a single transaction.
// Returns the number of operations successfully executed and any error.
func (b *WriteBatcher) FlushBatch(batch []WriteOp) (int, error) {
	if len(batch) == 0 {
		return 0, nil
	}

	tx, err := b.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			tx.Rollback() //nolint:errcheck // best effort rollback
		}
	}()

	executed := 0
	for _, op := range batch {
		if err := b.executor(tx, op); err != nil {
			return executed, fmt.Errorf("execute %s on %s: %w", op.Type, op.Table, err)
		}
		executed++
	}

	if err := tx.Commit(); err != nil {
		return executed, fmt.Errorf("commit transaction: %w", err)
	}
	tx = nil // Prevent rollback in defer
	return executed, nil
}

// FlushInterval returns the configured flush interval.
func (b *WriteBatcher) FlushInterval() time.Duration {
	return b.flushInterval
}

// BatchSize returns the configured batch size.
func (b *WriteBatcher) BatchSize() int {
	return b.batchSize
}

// FlushBatchFunc is a standalone function to flush a batch of write operations.
// This is useful for testing or when you don't need the full WriteBatcher.
func FlushBatchFunc(db *sql.DB, batch []WriteOp, executor WriteOpExecutor) (int, error) {
	if len(batch) == 0 {
		return 0, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			tx.Rollback() //nolint:errcheck // best effort rollback
		}
	}()

	executed := 0
	for _, op := range batch {
		if err := executor(tx, op); err != nil {
			return executed, fmt.Errorf("execute %s on %s: %w", op.Type, op.Table, err)
		}
		executed++
	}

	if err := tx.Commit(); err != nil {
		return executed, fmt.Errorf("commit transaction: %w", err)
	}
	tx = nil
	return executed, nil
}
