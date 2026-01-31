package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// MemoryStore is an in-memory implementation of Store with batched SQLite persistence.
// All reads are served from memory; writes update memory immediately and queue
// persistence operations to background workers.
type MemoryStore struct {
	mu     sync.RWMutex
	logger *zap.Logger

	// Persistence backends (multiple SQLite files)
	// These may be nil for test-only stores
	coreDB        *sql.DB
	projectsDB    *sql.DB
	agentsDB      *sql.DB
	deploymentsDB *sql.DB
	auditDB       *sql.DB
	ratelimitDB   *sql.DB
	provisionDB   *sql.DB

	// Write channels (one per database for batching)
	coreWrites        chan WriteOp
	projectsWrites    chan WriteOp
	agentsWrites      chan WriteOp
	deploymentsWrites chan WriteOp
	auditWrites       chan WriteOp
	ratelimitWrites   chan WriteOp
	provisionWrites   chan WriteOp

	// In-memory data (source of truth for reads)
	users         map[int64]*User
	usersByName   map[string]*User
	sessions      map[string]*Session
	apiKeys       map[string]*APIKey // keyed by hash
	apiKeysByUser map[int64][]*APIKey

	settings map[string]*Setting // keyed by "category:key"

	projects       map[int64]*Project
	projectsByName map[string]*Project
	projectTypes   map[int64]*ProjectType
	webhooks       map[int64]*ProjectWebhook
	secrets        map[string]*Secret // keyed by "project:scope:key"

	agents        map[string]*Agent
	agentBinaries map[int64]*AgentBinary
	agentUpdates  map[int64]*AgentUpdateHistory

	deployments        map[string]*DeploymentRecord
	deploymentLogs     map[string][]*DeploymentLog
	deploymentRollbacks map[int64]*DeploymentRollback
	scheduledDeploys   map[string]*ScheduledDeployment

	auditLogs []*AuditEntry // append-only slice

	blockedIPs     map[string]*BlockedIP
	rateLimits     map[string]*RateLimitRecord // keyed by "key:bucket"

	provisionJobs map[string]*ProvisionJob
	sshHostKeys   map[int64]*SSHHostKey
	jumpServers   map[int64]*SSHJumpServer

	healthCheckConfigs map[int64]*HealthCheckConfig

	// ID generators (atomic)
	nextUserID           atomic.Int64
	nextAPIKeyID         atomic.Int64
	nextProjectID        atomic.Int64
	nextProjectTypeID    atomic.Int64
	nextWebhookID        atomic.Int64
	nextSecretID         atomic.Int64
	nextAgentBinaryID    atomic.Int64
	nextAgentUpdateID    atomic.Int64
	nextDeploymentLogID  atomic.Int64
	nextRollbackID       atomic.Int64
	nextAuditID          atomic.Int64
	nextBlockedIPID      atomic.Int64
	nextRateLimitID      atomic.Int64
	nextProvisionJobID   atomic.Int64
	nextSSHHostKeyID     atomic.Int64
	nextJumpServerID     atomic.Int64
	nextHealthCheckID    atomic.Int64
	nextSettingID        atomic.Int64

	// Shutdown coordination
	done chan struct{}
	wg   sync.WaitGroup
}

// MemoryStoreConfig holds configuration for creating a MemoryStore.
type MemoryStoreConfig struct {
	// Database connections (all optional - nil for test-only store)
	CoreDB        *sql.DB
	ProjectsDB    *sql.DB
	AgentsDB      *sql.DB
	DeploymentsDB *sql.DB
	AuditDB       *sql.DB
	RateLimitDB   *sql.DB
	ProvisionDB   *sql.DB

	// Flush configuration per database
	CoreFlushInterval        time.Duration
	ProjectsFlushInterval    time.Duration
	AgentsFlushInterval      time.Duration
	DeploymentsFlushInterval time.Duration
	AuditFlushInterval       time.Duration
	RateLimitFlushInterval   time.Duration
	ProvisionFlushInterval   time.Duration

	CoreBatchSize        int
	ProjectsBatchSize    int
	AgentsBatchSize      int
	DeploymentsBatchSize int
	AuditBatchSize       int
	RateLimitBatchSize   int
	ProvisionBatchSize   int

	// Channel buffer sizes
	ChannelBufferSize int

	Logger *zap.Logger
}

// DefaultMemoryStoreConfig returns default configuration values.
func DefaultMemoryStoreConfig() MemoryStoreConfig {
	return MemoryStoreConfig{
		CoreFlushInterval:        100 * time.Millisecond,
		ProjectsFlushInterval:    200 * time.Millisecond,
		AgentsFlushInterval:      200 * time.Millisecond,
		DeploymentsFlushInterval: 50 * time.Millisecond,
		AuditFlushInterval:       50 * time.Millisecond,
		RateLimitFlushInterval:   1 * time.Second,
		ProvisionFlushInterval:   200 * time.Millisecond,

		CoreBatchSize:        50,
		ProjectsBatchSize:    100,
		AgentsBatchSize:      100,
		DeploymentsBatchSize: 200,
		AuditBatchSize:       500,
		RateLimitBatchSize:   1000,
		ProvisionBatchSize:   50,

		ChannelBufferSize: 10000,
	}
}

// NewMemoryStore creates a new MemoryStore.
// Pass nil config for a test-only store with no persistence.
func NewMemoryStore(cfg *MemoryStoreConfig) *MemoryStore {
	if cfg == nil {
		defaults := DefaultMemoryStoreConfig()
		cfg = &defaults
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	bufSize := cfg.ChannelBufferSize
	if bufSize == 0 {
		bufSize = 10000
	}

	s := &MemoryStore{
		logger: logger,

		// Database connections
		coreDB:        cfg.CoreDB,
		projectsDB:    cfg.ProjectsDB,
		agentsDB:      cfg.AgentsDB,
		deploymentsDB: cfg.DeploymentsDB,
		auditDB:       cfg.AuditDB,
		ratelimitDB:   cfg.RateLimitDB,
		provisionDB:   cfg.ProvisionDB,

		// Write channels
		coreWrites:        make(chan WriteOp, bufSize),
		projectsWrites:    make(chan WriteOp, bufSize),
		agentsWrites:      make(chan WriteOp, bufSize),
		deploymentsWrites: make(chan WriteOp, bufSize),
		auditWrites:       make(chan WriteOp, bufSize),
		ratelimitWrites:   make(chan WriteOp, bufSize),
		provisionWrites:   make(chan WriteOp, bufSize),

		// Initialize all maps
		users:         make(map[int64]*User),
		usersByName:   make(map[string]*User),
		sessions:      make(map[string]*Session),
		apiKeys:       make(map[string]*APIKey),
		apiKeysByUser: make(map[int64][]*APIKey),

		settings: make(map[string]*Setting),

		projects:       make(map[int64]*Project),
		projectsByName: make(map[string]*Project),
		projectTypes:   make(map[int64]*ProjectType),
		webhooks:       make(map[int64]*ProjectWebhook),
		secrets:        make(map[string]*Secret),

		agents:        make(map[string]*Agent),
		agentBinaries: make(map[int64]*AgentBinary),
		agentUpdates:  make(map[int64]*AgentUpdateHistory),

		deployments:         make(map[string]*DeploymentRecord),
		deploymentLogs:      make(map[string][]*DeploymentLog),
		deploymentRollbacks: make(map[int64]*DeploymentRollback),
		scheduledDeploys:    make(map[string]*ScheduledDeployment),

		auditLogs: make([]*AuditEntry, 0),

		blockedIPs: make(map[string]*BlockedIP),
		rateLimits: make(map[string]*RateLimitRecord),

		provisionJobs: make(map[string]*ProvisionJob),
		sshHostKeys:   make(map[int64]*SSHHostKey),
		jumpServers:   make(map[int64]*SSHJumpServer),

		healthCheckConfigs: make(map[int64]*HealthCheckConfig),

		done: make(chan struct{}),
	}

	// Start write workers for each database that has a connection
	if cfg.CoreDB != nil {
		s.startWriteWorker("core", cfg.CoreDB, s.coreWrites, cfg.CoreFlushInterval, cfg.CoreBatchSize)
	}
	if cfg.ProjectsDB != nil {
		s.startWriteWorker("projects", cfg.ProjectsDB, s.projectsWrites, cfg.ProjectsFlushInterval, cfg.ProjectsBatchSize)
	}
	if cfg.AgentsDB != nil {
		s.startWriteWorker("agents", cfg.AgentsDB, s.agentsWrites, cfg.AgentsFlushInterval, cfg.AgentsBatchSize)
	}
	if cfg.DeploymentsDB != nil {
		s.startWriteWorker("deployments", cfg.DeploymentsDB, s.deploymentsWrites, cfg.DeploymentsFlushInterval, cfg.DeploymentsBatchSize)
	}
	if cfg.AuditDB != nil {
		s.startWriteWorker("audit", cfg.AuditDB, s.auditWrites, cfg.AuditFlushInterval, cfg.AuditBatchSize)
	}
	if cfg.RateLimitDB != nil {
		s.startWriteWorker("ratelimit", cfg.RateLimitDB, s.ratelimitWrites, cfg.RateLimitFlushInterval, cfg.RateLimitBatchSize)
	}
	if cfg.ProvisionDB != nil {
		s.startWriteWorker("provision", cfg.ProvisionDB, s.provisionWrites, cfg.ProvisionFlushInterval, cfg.ProvisionBatchSize)
	}

	return s
}

// startWriteWorker starts a background goroutine that batches and flushes writes.
func (s *MemoryStore) startWriteWorker(name string, db *sql.DB, writes <-chan WriteOp, flushInterval time.Duration, batchSize int) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()

		batch := make([]WriteOp, 0, batchSize)

		flush := func() {
			if len(batch) == 0 {
				return
			}
			count, err := FlushBatchFunc(db, batch, s.executeWriteOp)
			if err != nil {
				s.logger.Error("failed to flush batch",
					zap.String("db", name),
					zap.Int("count", count),
					zap.Int("total", len(batch)),
					zap.Error(err))
			} else {
				s.logger.Debug("flushed batch",
					zap.String("db", name),
					zap.Int("count", count))
			}
			batch = batch[:0]
		}

		for {
			select {
			case op := <-writes:
				batch = append(batch, op)
				if len(batch) >= batchSize {
					flush()
				}
			case <-ticker.C:
				flush()
			case <-s.done:
				// Drain remaining writes from channel
				draining := true
				for draining {
					select {
					case op := <-writes:
						batch = append(batch, op)
					default:
						draining = false
					}
				}
				flush()
				return
			}
		}
	}()
}

// executeWriteOp executes a single write operation within a transaction.
// This is the callback passed to FlushBatchFunc.
func (s *MemoryStore) executeWriteOp(tx *sql.Tx, op WriteOp) error {
	// This will be implemented per-table in the entity-specific files.
	// For now, return nil as a placeholder.
	// Each entity file will register its executor.
	return nil
}

// Close signals all write workers to stop, waits for them to drain,
// and closes all database connections.
func (s *MemoryStore) Close() error {
	// Signal all workers to stop
	close(s.done)

	// Wait for workers to flush remaining writes
	s.wg.Wait()

	// Close all database connections
	var errs []error
	if s.coreDB != nil {
		if err := s.coreDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close core db: %w", err))
		}
	}
	if s.projectsDB != nil {
		if err := s.projectsDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close projects db: %w", err))
		}
	}
	if s.agentsDB != nil {
		if err := s.agentsDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close agents db: %w", err))
		}
	}
	if s.deploymentsDB != nil {
		if err := s.deploymentsDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close deployments db: %w", err))
		}
	}
	if s.auditDB != nil {
		if err := s.auditDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close audit db: %w", err))
		}
	}
	if s.ratelimitDB != nil {
		if err := s.ratelimitDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close ratelimit db: %w", err))
		}
	}
	if s.provisionDB != nil {
		if err := s.provisionDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close provision db: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// Conn returns nil for MemoryStore as it doesn't have a single connection.
// Use specific database connections if needed.
func (s *MemoryStore) Conn() *sql.DB {
	return nil
}

// RunInTransaction is not supported for MemoryStore in the same way as SQLite.
// Memory operations are atomic per-method; this is a no-op wrapper.
func (s *MemoryStore) RunInTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error {
	// For memory store, we don't have real transactions.
	// The caller should use the memory store methods directly.
	return fmt.Errorf("RunInTransaction not supported on MemoryStore; use methods directly")
}

// queueWrite sends a write operation to the appropriate channel.
// This is non-blocking if channel has capacity.
func (s *MemoryStore) queueWrite(ch chan<- WriteOp, op WriteOp) {
	select {
	case ch <- op:
		// Queued successfully
	default:
		// Channel full - log warning but don't block
		s.logger.Warn("write channel full, operation may be delayed",
			zap.String("table", op.Table),
			zap.String("type", op.Type.String()))
		// Force send (will block if necessary)
		ch <- op
	}
}

// nextID atomically increments and returns the next ID for a given counter.
func nextID(counter *atomic.Int64) int64 {
	return counter.Add(1)
}

// settingKey returns the map key for a setting.
func settingKey(category, key string) string {
	return category + ":" + key
}

// secretKey returns the map key for a secret.
func secretKey(project, scope, key string) string {
	return project + ":" + scope + ":" + key
}

// rateLimitKey returns the map key for a rate limit record.
func rateLimitKey(key, bucket string) string {
	return key + ":" + bucket
}

// Ensure MemoryStore implements Store at compile time.
var _ Store = (*MemoryStore)(nil)
