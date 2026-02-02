// Package provision handles agent provisioning and lifecycle management.
package provision

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Worker handles async provisioning jobs.
type Worker struct {
	provisioner *SSHProvisioner
	store       storage.Store
	logger      *zap.Logger
	queue       chan string // job IDs
	workers     int
	wg          sync.WaitGroup
	shutdown    chan struct{}
	shutdownMu  sync.Mutex
	isShutdown  bool
}

// NewWorker creates a new provisioning worker.
func NewWorker(provisioner *SSHProvisioner, store storage.Store, logger *zap.Logger, workers int) *Worker {
	if workers < 1 {
		workers = 2
	}
	return &Worker{
		provisioner: provisioner,
		store:       store,
		logger:      logger.Named("provision-worker"),
		queue:       make(chan string, 100),
		workers:     workers,
		shutdown:    make(chan struct{}),
	}
}

// Start starts the worker goroutines.
func (w *Worker) Start() {
	for i := 0; i < w.workers; i++ {
		w.wg.Add(1)
		go w.worker(i)
	}
	w.logger.Info("Provisioning worker started", zap.Int("workers", w.workers))
}

// Submit submits a new provisioning job for async execution.
func (w *Worker) Submit(ctx context.Context, req *SSHProvisionRequest) (string, error) {
	w.shutdownMu.Lock()
	if w.isShutdown {
		w.shutdownMu.Unlock()
		return "", fmt.Errorf("worker is shutdown")
	}
	w.shutdownMu.Unlock()

	// Create job ID
	jobID := uuid.New().String()

	// Create job record
	job := &storage.ProvisionJob{
		ID:         jobID,
		TargetHost: req.TargetHost,
		TargetPort: req.TargetPort,
		TargetUser: req.SSHUser,
		Status:     "pending",
		Stage:      "queued",
		Progress:   0,
		StartedAt:  time.Now(),
	}

	if err := w.store.CreateProvisionJob(ctx, job); err != nil {
		return "", fmt.Errorf("save job: %w", err)
	}

	w.log(ctx, jobID, "info", "Job queued for provisioning %s@%s", req.SSHUser, req.TargetHost)

	// Queue the job ID
	select {
	case w.queue <- jobID:
		return jobID, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// GetJob retrieves a job by ID.
func (w *Worker) GetJob(ctx context.Context, jobID string) (*storage.ProvisionJob, error) {
	return w.store.GetProvisionJob(ctx, jobID)
}

// GetLogs retrieves logs for a job.
func (w *Worker) GetLogs(ctx context.Context, jobID string) ([]*storage.ProvisionLog, error) {
	return w.store.GetProvisionLogs(ctx, jobID)
}

// Shutdown gracefully shuts down the worker, waiting for in-progress jobs.
func (w *Worker) Shutdown(timeout time.Duration) {
	w.shutdownMu.Lock()
	if w.isShutdown {
		w.shutdownMu.Unlock()
		return
	}
	w.isShutdown = true
	w.shutdownMu.Unlock()

	close(w.shutdown)
	close(w.queue)

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		w.logger.Info("Provisioning worker shutdown complete")
	case <-time.After(timeout):
		w.logger.Warn("Provisioning worker shutdown timed out")
	}
}

// worker is the main worker goroutine.
func (w *Worker) worker(id int) {
	defer w.wg.Done()
	w.logger.Debug("Worker started", zap.Int("worker_id", id))

	for {
		select {
		case jobID, ok := <-w.queue:
			if !ok {
				w.logger.Debug("Worker queue closed", zap.Int("worker_id", id))
				return
			}
			w.runJob(context.Background(), jobID)
		case <-w.shutdown:
			w.logger.Debug("Worker received shutdown signal", zap.Int("worker_id", id))
			return
		}
	}
}

// runJob executes a single provisioning job.
func (w *Worker) runJob(ctx context.Context, jobID string) {
	job, err := w.store.GetProvisionJob(ctx, jobID)
	if err != nil {
		w.logger.Error("Failed to get provision job", zap.String("job_id", jobID), zap.Error(err))
		return
	}

	// Update status to running
	if err := w.store.UpdateProvisionJobStatus(ctx, jobID, "running", "starting", "", 5); err != nil {
		w.logger.Error("Failed to update job status", zap.String("job_id", jobID), zap.Error(err))
	}

	w.log(ctx, jobID, "info", "Starting provisioning for %s@%s:%d",
		job.TargetUser, job.TargetHost, job.TargetPort)

	// Build the SSH provision request from the stored job
	req := &SSHProvisionRequest{
		AgentID:    fmt.Sprintf("agent-%s", jobID[:8]), // Generate agent ID from job ID
		TargetHost: job.TargetHost,
		TargetPort: job.TargetPort,
		SSHUser:    job.TargetUser,
	}

	// Create a logging wrapper that captures output
	loggingCallback := func(stage string, progress int, message string) {
		w.log(ctx, jobID, "info", "[%s] %s", stage, message)
		_ = w.store.UpdateProvisionJobStatus(ctx, jobID, "running", stage, "", progress)
	}

	// Execute provisioning
	_, err = w.provisionWithLogging(ctx, req, loggingCallback)

	if err != nil {
		w.log(ctx, jobID, "error", "Provisioning failed: %v", err)
		if updateErr := w.store.UpdateProvisionJobStatus(ctx, jobID, "failed", "error", err.Error(), 100); updateErr != nil {
			w.logger.Error("Failed to update failed job status", zap.Error(updateErr))
		}
		return
	}

	w.log(ctx, jobID, "info", "Provisioning completed successfully")
	if err := w.store.UpdateProvisionJobStatus(ctx, jobID, "completed", "done", "", 100); err != nil {
		w.logger.Error("Failed to update completed job status", zap.Error(err))
	}
}

// provisionWithLogging runs provisioning with progress callbacks.
func (w *Worker) provisionWithLogging(ctx context.Context, req *SSHProvisionRequest, callback func(stage string, progress int, message string)) (*SSHProvisionResult, error) {
	callback("connecting", 10, fmt.Sprintf("Connecting to %s:%d", req.TargetHost, req.TargetPort))

	// Use the SSHProvisioner to do the actual work
	if w.provisioner == nil {
		return nil, fmt.Errorf("SSH provisioner not configured")
	}

	callback("provisioning", 30, "Starting SSH provisioning")

	result, err := w.provisioner.SSHProvision(ctx, req)
	if err != nil {
		return nil, err
	}

	callback("verifying", 80, "Verifying agent installation")
	callback("complete", 100, "Provisioning complete")

	return result, nil
}

// log saves a provision log entry.
func (w *Worker) log(ctx context.Context, jobID, level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if err := w.store.SaveProvisionLog(ctx, jobID, level, msg); err != nil {
		w.logger.Error("Failed to save provision log",
			zap.String("job_id", jobID),
			zap.Error(err))
	}
	w.logger.Info(msg, zap.String("job_id", jobID), zap.String("level", level))
}
