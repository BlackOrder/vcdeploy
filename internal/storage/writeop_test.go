package storage

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupWriteOpTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create a simple test table
	_, err = db.Exec(`CREATE TABLE test_items (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		value INTEGER
	)`)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	return db
}

// testExecutor is a simple executor for testing that handles test_items table
func testExecutor(tx *sql.Tx, op WriteOp) error {
	switch data := op.Data.(type) {
	case map[string]any:
		switch op.Type {
		case WriteOpInsert:
			_, err := tx.Exec("INSERT INTO test_items (name, value) VALUES (?, ?)",
				data["name"], data["value"])
			return err
		case WriteOpUpdate:
			_, err := tx.Exec("UPDATE test_items SET value = ? WHERE name = ?",
				data["value"], data["name"])
			return err
		case WriteOpDelete:
			_, err := tx.Exec("DELETE FROM test_items WHERE name = ?", data["name"])
			return err
		}
	}
	return nil
}

func TestWriteOpType_String(t *testing.T) {
	tests := []struct {
		opType WriteOpType
		want   string
	}{
		{WriteOpInsert, "INSERT"},
		{WriteOpUpdate, "UPDATE"},
		{WriteOpDelete, "DELETE"},
		{WriteOpType(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.opType.String(); got != tt.want {
				t.Errorf("WriteOpType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewWriteOp(t *testing.T) {
	before := time.Now()
	op := NewWriteOp(WriteOpInsert, "users", map[string]any{"name": "test"})
	after := time.Now()

	if op.Type != WriteOpInsert {
		t.Errorf("Type = %v, want %v", op.Type, WriteOpInsert)
	}
	if op.Table != "users" {
		t.Errorf("Table = %v, want users", op.Table)
	}
	if op.Timestamp.Before(before) || op.Timestamp.After(after) {
		t.Errorf("Timestamp not in expected range")
	}
}

func TestWriteOp_Insert(t *testing.T) {
	db := setupWriteOpTestDB(t)

	op := NewWriteOp(WriteOpInsert, "test_items", map[string]any{
		"name":  "item1",
		"value": 100,
	})

	count, err := FlushBatchFunc(db, []WriteOp{op}, testExecutor)
	if err != nil {
		t.Fatalf("FlushBatchFunc failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Verify the insert
	var name string
	var value int
	err = db.QueryRow("SELECT name, value FROM test_items WHERE name = ?", "item1").Scan(&name, &value)
	if err != nil {
		t.Fatalf("failed to query inserted item: %v", err)
	}
	if name != "item1" || value != 100 {
		t.Errorf("got name=%s, value=%d; want name=item1, value=100", name, value)
	}
}

func TestWriteOp_Update(t *testing.T) {
	db := setupWriteOpTestDB(t)

	// First insert an item
	_, err := db.Exec("INSERT INTO test_items (name, value) VALUES (?, ?)", "item1", 100)
	if err != nil {
		t.Fatalf("failed to insert test item: %v", err)
	}

	// Now update it
	op := NewWriteOp(WriteOpUpdate, "test_items", map[string]any{
		"name":  "item1",
		"value": 200,
	})

	count, err := FlushBatchFunc(db, []WriteOp{op}, testExecutor)
	if err != nil {
		t.Fatalf("FlushBatchFunc failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Verify the update
	var value int
	err = db.QueryRow("SELECT value FROM test_items WHERE name = ?", "item1").Scan(&value)
	if err != nil {
		t.Fatalf("failed to query updated item: %v", err)
	}
	if value != 200 {
		t.Errorf("value = %d, want 200", value)
	}
}

func TestWriteOp_Delete(t *testing.T) {
	db := setupWriteOpTestDB(t)

	// First insert an item
	_, err := db.Exec("INSERT INTO test_items (name, value) VALUES (?, ?)", "item1", 100)
	if err != nil {
		t.Fatalf("failed to insert test item: %v", err)
	}

	// Now delete it
	op := NewWriteOp(WriteOpDelete, "test_items", map[string]any{
		"name": "item1",
	})

	count, err := FlushBatchFunc(db, []WriteOp{op}, testExecutor)
	if err != nil {
		t.Fatalf("FlushBatchFunc failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Verify the delete
	var cnt int
	err = db.QueryRow("SELECT COUNT(*) FROM test_items WHERE name = ?", "item1").Scan(&cnt)
	if err != nil {
		t.Fatalf("failed to query count: %v", err)
	}
	if cnt != 0 {
		t.Errorf("count = %d, want 0 (item should be deleted)", cnt)
	}
}

func TestFlushBatch_Empty(t *testing.T) {
	db := setupWriteOpTestDB(t)

	count, err := FlushBatchFunc(db, []WriteOp{}, testExecutor)
	if err != nil {
		t.Fatalf("FlushBatchFunc failed: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestFlushBatch_SingleOp(t *testing.T) {
	db := setupWriteOpTestDB(t)

	op := NewWriteOp(WriteOpInsert, "test_items", map[string]any{
		"name":  "single",
		"value": 42,
	})

	count, err := FlushBatchFunc(db, []WriteOp{op}, testExecutor)
	if err != nil {
		t.Fatalf("FlushBatchFunc failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestFlushBatch_MultipleOps(t *testing.T) {
	db := setupWriteOpTestDB(t)

	ops := []WriteOp{
		NewWriteOp(WriteOpInsert, "test_items", map[string]any{"name": "item1", "value": 1}),
		NewWriteOp(WriteOpInsert, "test_items", map[string]any{"name": "item2", "value": 2}),
		NewWriteOp(WriteOpInsert, "test_items", map[string]any{"name": "item3", "value": 3}),
	}

	count, err := FlushBatchFunc(db, ops, testExecutor)
	if err != nil {
		t.Fatalf("FlushBatchFunc failed: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}

	// Verify all items inserted
	var total int
	err = db.QueryRow("SELECT COUNT(*) FROM test_items").Scan(&total)
	if err != nil {
		t.Fatalf("failed to query count: %v", err)
	}
	if total != 3 {
		t.Errorf("total items = %d, want 3", total)
	}
}

func TestFlushBatch_TransactionRollback(t *testing.T) {
	db := setupWriteOpTestDB(t)

	// Insert one item successfully first
	_, err := db.Exec("INSERT INTO test_items (name, value) VALUES (?, ?)", "existing", 100)
	if err != nil {
		t.Fatalf("failed to insert test item: %v", err)
	}

	// Create an executor that fails on the second operation
	failingExecutor := func(tx *sql.Tx, op WriteOp) error {
		data := op.Data.(map[string]any)
		if data["name"] == "fail" {
			return errors.New("intentional failure")
		}
		_, err := tx.Exec("INSERT INTO test_items (name, value) VALUES (?, ?)",
			data["name"], data["value"])
		return err
	}

	ops := []WriteOp{
		NewWriteOp(WriteOpInsert, "test_items", map[string]any{"name": "item1", "value": 1}),
		NewWriteOp(WriteOpInsert, "test_items", map[string]any{"name": "fail", "value": 2}),
		NewWriteOp(WriteOpInsert, "test_items", map[string]any{"name": "item3", "value": 3}),
	}

	count, err := FlushBatchFunc(db, ops, failingExecutor)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (ops executed before failure)", count)
	}

	// Verify transaction was rolled back - only "existing" should remain
	var total int
	err = db.QueryRow("SELECT COUNT(*) FROM test_items").Scan(&total)
	if err != nil {
		t.Fatalf("failed to query count: %v", err)
	}
	if total != 1 {
		t.Errorf("total items = %d, want 1 (transaction should have rolled back)", total)
	}
}

func TestWriteBatcher_FlushBatch(t *testing.T) {
	db := setupWriteOpTestDB(t)

	batcher := NewWriteBatcher(db, testExecutor, 100*time.Millisecond, 10)

	ops := []WriteOp{
		NewWriteOp(WriteOpInsert, "test_items", map[string]any{"name": "item1", "value": 1}),
		NewWriteOp(WriteOpInsert, "test_items", map[string]any{"name": "item2", "value": 2}),
	}

	count, err := batcher.FlushBatch(ops)
	if err != nil {
		t.Fatalf("FlushBatch failed: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestWriteBatcher_Config(t *testing.T) {
	db := setupWriteOpTestDB(t)

	batcher := NewWriteBatcher(db, testExecutor, 500*time.Millisecond, 100)

	if batcher.FlushInterval() != 500*time.Millisecond {
		t.Errorf("FlushInterval = %v, want 500ms", batcher.FlushInterval())
	}
	if batcher.BatchSize() != 100 {
		t.Errorf("BatchSize = %d, want 100", batcher.BatchSize())
	}
}
