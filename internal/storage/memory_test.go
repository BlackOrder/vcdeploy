package storage

import (
	"database/sql"
	"sync"
	"testing"
	"time"
)

func TestMemoryStore_New(t *testing.T) {
	// Test with nil config (test-only store)
	s := NewMemoryStore(nil)
	if s == nil {
		t.Fatal("NewMemoryStore returned nil")
	}
	defer s.Close()

	// Verify maps are initialized
	if s.users == nil {
		t.Error("users map not initialized")
	}
	if s.projects == nil {
		t.Error("projects map not initialized")
	}
	if s.agents == nil {
		t.Error("agents map not initialized")
	}
	if s.deployments == nil {
		t.Error("deployments map not initialized")
	}
}

func TestMemoryStore_NewWithConfig(t *testing.T) {
	cfg := DefaultMemoryStoreConfig()
	cfg.ChannelBufferSize = 100

	s := NewMemoryStore(&cfg)
	if s == nil {
		t.Fatal("NewMemoryStore returned nil")
	}
	defer s.Close()

	// Verify configuration was applied
	if cap(s.coreWrites) != 100 {
		t.Errorf("channel buffer size = %d, want 100", cap(s.coreWrites))
	}
}

func TestMemoryStore_Close(t *testing.T) {
	s := NewMemoryStore(nil)

	// Close should not error for a test-only store
	err := s.Close()
	if err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}

	// Verify done channel is closed
	select {
	case <-s.done:
		// Expected - channel is closed
	default:
		t.Error("done channel not closed after Close()")
	}
}

func TestMemoryStore_CloseFlushesWrites(t *testing.T) {
	// This test verifies that Close() waits for write workers to finish.
	// We can't easily test actual flushing without a real database,
	// but we can verify the synchronization works.

	s := NewMemoryStore(nil)

	// Simulate some activity
	s.mu.Lock()
	s.users[1] = &User{ID: 1, Username: "test"}
	s.usersByName["test"] = s.users[1]
	s.mu.Unlock()

	// Close should complete without hanging
	done := make(chan struct{})
	go func() {
		s.Close()
		close(done)
	}()

	select {
	case <-done:
		// Expected - Close completed
	case <-time.After(5 * time.Second):
		t.Fatal("Close() timed out")
	}
}

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	// Test concurrent reads and writes
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			s.mu.Lock()
			s.users[id] = &User{ID: id, Username: "user"}
			s.mu.Unlock()
		}(int64(i))
	}

	// Concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.mu.RLock()
			_ = len(s.users)
			s.mu.RUnlock()
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent access error: %v", err)
	}

	// Verify all writes succeeded
	s.mu.RLock()
	if len(s.users) != 10 {
		t.Errorf("users count = %d, want 10", len(s.users))
	}
	s.mu.RUnlock()
}

func TestMemoryStore_Conn(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	// Conn should return nil for memory-only store
	if s.Conn() != nil {
		t.Error("Conn() should return nil for memory-only store")
	}
}

func TestMemoryStore_RunInTransaction(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	// RunInTransaction should return an error for memory store
	err := s.RunInTransaction(nil, func(tx *sql.Tx) error {
		return nil
	})
	if err == nil {
		t.Error("RunInTransaction should return error for MemoryStore")
	}
}

func TestDefaultMemoryStoreConfig(t *testing.T) {
	cfg := DefaultMemoryStoreConfig()

	// Verify default values
	if cfg.CoreFlushInterval != 100*time.Millisecond {
		t.Errorf("CoreFlushInterval = %v, want 100ms", cfg.CoreFlushInterval)
	}
	if cfg.DeploymentsFlushInterval != 50*time.Millisecond {
		t.Errorf("DeploymentsFlushInterval = %v, want 50ms", cfg.DeploymentsFlushInterval)
	}
	if cfg.AuditBatchSize != 500 {
		t.Errorf("AuditBatchSize = %d, want 500", cfg.AuditBatchSize)
	}
	if cfg.ChannelBufferSize != 10000 {
		t.Errorf("ChannelBufferSize = %d, want 10000", cfg.ChannelBufferSize)
	}
}

func TestNextID(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	// Test ID generation
	id1 := nextID(&s.nextUserID)
	id2 := nextID(&s.nextUserID)
	id3 := nextID(&s.nextUserID)

	if id1 != 1 || id2 != 2 || id3 != 3 {
		t.Errorf("nextID sequence = %d, %d, %d; want 1, 2, 3", id1, id2, id3)
	}
}

func TestSettingKey(t *testing.T) {
	key := settingKey("auth", "session_timeout")
	if key != "auth:session_timeout" {
		t.Errorf("settingKey = %s, want auth:session_timeout", key)
	}
}

func TestSecretKey(t *testing.T) {
	key := secretKey("myproject", "env", "API_KEY")
	if key != "myproject:env:API_KEY" {
		t.Errorf("secretKey = %s, want myproject:env:API_KEY", key)
	}
}

func TestRateLimitKey(t *testing.T) {
	key := rateLimitKey("192.168.1.1", "api")
	if key != "192.168.1.1:api" {
		t.Errorf("rateLimitKey = %s, want 192.168.1.1:api", key)
	}
}

func TestQueueWrite_NonBlocking(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	// Queue should not block when channel has capacity
	op := NewWriteOp(WriteOpInsert, "test", nil)

	done := make(chan struct{})
	go func() {
		s.queueWrite(s.coreWrites, op)
		close(done)
	}()

	select {
	case <-done:
		// Expected - queue completed quickly
	case <-time.After(100 * time.Millisecond):
		t.Error("queueWrite blocked unexpectedly")
	}
}
