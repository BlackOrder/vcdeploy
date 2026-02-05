package provision

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// mockWorkerStore implements the minimal Store interface needed for worker tests.
type mockWorkerStore struct {
	*storage.MemoryStore
	pendingJobs []*storage.ProvisionJob
}

func newMockWorkerStore(t *testing.T) *mockWorkerStore {
	ms := storage.NewMemoryStore(&storage.MemoryStoreConfig{})
	return &mockWorkerStore{
		MemoryStore: ms,
		pendingJobs: make([]*storage.ProvisionJob, 0),
	}
}

func (s *mockWorkerStore) ListPendingProvisionJobs(ctx context.Context) ([]*storage.ProvisionJob, error) {
	return s.pendingJobs, nil
}

func (s *mockWorkerStore) addPendingJob(job *storage.ProvisionJob) {
	s.pendingJobs = append(s.pendingJobs, job)
}

func TestNewWorker(t *testing.T) {
	store := newMockWorkerStore(t)
	logger := zap.NewNop()

	w := NewWorker(nil, store, logger, 2)
	if w == nil {
		t.Fatal("NewWorker() returned nil")
	}

	if w.workers != 2 {
		t.Errorf("workers = %d, want 2", w.workers)
	}

	if w.pollRunning == nil {
		t.Error("pollRunning map is nil")
	}

	if w.pollCtx == nil {
		t.Error("pollCtx is nil")
	}

	if w.pollCancel == nil {
		t.Error("pollCancel is nil")
	}
}

func TestWorker_StartStop(t *testing.T) {
	store := newMockWorkerStore(t)
	logger := zap.NewNop()

	w := NewWorker(nil, store, logger, 1)
	w.Start()

	// Give some time for goroutines to start
	time.Sleep(50 * time.Millisecond)

	// Should be able to shutdown gracefully
	w.Shutdown(time.Second)
}

func TestWorker_SubmitJob(t *testing.T) {
	store := newMockWorkerStore(t)
	logger := zap.NewNop()

	w := NewWorker(nil, store, logger, 1)
	w.Start()
	defer w.Shutdown(time.Second)

	ctx := context.Background()

	// Create a provision request
	req := &SSHProvisionRequest{
		AgentID:    "test-agent",
		TargetHost: "localhost",
		TargetPort: 22,
		SSHUser:    "root",
	}

	// Submit the job
	jobID, err := w.Submit(ctx, req)
	if err != nil {
		t.Errorf("Submit() error = %v", err)
	}
	if jobID == "" {
		t.Error("Submit() returned empty job ID")
	}
}

func TestWorker_SubmitJobQueueFull(t *testing.T) {
	store := newMockWorkerStore(t)
	logger := zap.NewNop()

	// Create worker with very small queue but don't start (so no consumers)
	w := NewWorker(nil, store, logger, 0)

	ctx := context.Background()

	// Fill the queue manually (since no workers are consuming)
	// The queue has capacity 100 by default, so we need to fill it
fillLoop:
	for i := 0; i < 100; i++ {
		select {
		case w.queue <- "job":
		default:
			break fillLoop
		}
	}

	// Use a context with timeout so we don't block forever
	ctxTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	// Now submit should fail (timeout or queue full)
	req := &SSHProvisionRequest{
		AgentID:    "test-agent",
		TargetHost: "localhost",
		TargetPort: 22,
		SSHUser:    "root",
	}
	_, err := w.Submit(ctxTimeout, req)
	if err == nil {
		t.Error("Submit() should return error when queue is full and context times out")
	}
}

func TestWorker_PollOnce(t *testing.T) {
	store := newMockWorkerStore(t)
	logger := zap.NewNop()

	// Use 0 workers so jobs stay in queue and aren't consumed
	w := NewWorker(nil, store, logger, 0)
	// Don't start the full worker, just test poll behavior

	ctx := context.Background()

	// Create a pending job in the store
	job := &storage.ProvisionJob{
		ID:         "pending-job-1",
		TargetHost: "localhost",
		TargetPort: 22,
		TargetUser: "root",
		Status:     "pending",
	}
	if err := store.CreateProvisionJob(ctx, job); err != nil {
		t.Fatalf("CreateProvisionJob() error = %v", err)
	}
	store.addPendingJob(job)

	// Trigger a poll
	w.pollOnce()

	// Check that the job is marked as running in pollRunning
	w.pollMu.Lock()
	_, exists := w.pollRunning[job.ID]
	w.pollMu.Unlock()

	if !exists {
		t.Error("Job should be marked as running in pollRunning")
	}

	// Check that the job was added to the queue
	select {
	case queuedID := <-w.queue:
		if queuedID != job.ID {
			t.Errorf("Queued job ID = %s, want %s", queuedID, job.ID)
		}
	default:
		t.Error("Job should have been queued")
	}
}

func TestWorker_PollDeduplication(t *testing.T) {
	store := newMockWorkerStore(t)
	logger := zap.NewNop()

	// Use 0 workers so jobs stay in queue (min is 2, but we'll just let them sit)
	w := NewWorker(nil, store, logger, 2)

	ctx := context.Background()

	// Create a pending job
	job := &storage.ProvisionJob{
		ID:         "dedupe-job-1",
		TargetHost: "localhost",
		TargetPort: 22,
		TargetUser: "root",
		Status:     "pending",
	}
	if err := store.CreateProvisionJob(ctx, job); err != nil {
		t.Fatalf("CreateProvisionJob() error = %v", err)
	}
	store.addPendingJob(job)

	// Poll twice
	w.pollOnce()
	w.pollOnce()

	// Check pollRunning - should only have 1 entry
	w.pollMu.Lock()
	count := len(w.pollRunning)
	w.pollMu.Unlock()

	if count != 1 {
		t.Errorf("pollRunning should have 1 job, got %d", count)
	}
}

func TestWorker_ShutdownCancelsPoll(t *testing.T) {
	store := newMockWorkerStore(t)
	logger := zap.NewNop()

	w := NewWorker(nil, store, logger, 1)
	w.Start()

	// Give some time for poll goroutine to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown should complete without hanging
	done := make(chan struct{})
	go func() {
		w.Shutdown(time.Second)
		close(done)
	}()

	select {
	case <-done:
		// Good, shutdown completed
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown timed out - poll goroutine may not have stopped")
	}
}

func TestWorker_DoubleShutdown(t *testing.T) {
	store := newMockWorkerStore(t)
	logger := zap.NewNop()

	w := NewWorker(nil, store, logger, 1)
	w.Start()

	// Double shutdown should not panic
	w.Shutdown(time.Second)
	w.Shutdown(time.Second) // Should be a no-op
}
