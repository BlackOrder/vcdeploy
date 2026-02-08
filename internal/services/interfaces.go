// Package services provides service layer interfaces and implementations for vcdeploy.
//
// # Overview
//
// The services package defines the core business logic interfaces for vcdeploy.
// All interfaces follow these conventions:
//   - Accept context.Context as first parameter for cancellation and deadlines
//   - Return domain-specific errors from the errors.go file
//   - Are stateless - all state is managed by the underlying storage layer
//
// # Service Interfaces
//
// Core services:
//   - UserServicer: User account management and authentication
//   - SessionServicer: User session lifecycle management
//   - APIKeyServicer: API key creation and validation
//   - ProjectServicer: Project configuration management
//   - DeploymentServicer: Deployment execution and logging
//   - AgentServicer: Deployment agent registration and status
//
// Supporting services:
//   - SecretServicer: Encrypted secret storage (AES-256-GCM)
//   - SettingsServicer: Application configuration
//   - AuditServicer: Audit logging for compliance
//   - HostKeyServicer: SSH host key management
//   - ProjectTypeServicer: Project type templates
//   - RateLimitServicer: Rate limiting and IP blocking
//   - ProvisionServicer: Agent provisioning jobs
//   - WebhookServicer: Project webhook management
//
// # Pagination
//
// List methods that support pagination use the Pagination type from types.go
// and return ListResult[T] which includes total count and pagination metadata.
//
// # Error Handling
//
// All services return errors defined in errors.go. Use errors.Is() or errors.As()
// to check for specific error types:
//
//	if errors.Is(err, services.ErrNotFound) { ... }
//	var inputErr *services.InputError
//	if errors.As(err, &inputErr) { ... }
package services

import (
	"context"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// SecretServicer defines the interface for secret management.
type SecretServicer interface {
	Set(ctx context.Context, project, scope, key, value string) error
	Get(ctx context.Context, project, scope, key string) (string, error)
	Delete(ctx context.Context, project, scope, key string) error
	List(ctx context.Context, project, scope string) ([]SecretMetadata, error)
	ListByProject(ctx context.Context, project string) ([]SecretMetadata, error)
	ListAll(ctx context.Context) ([]SecretMetadata, error)
	Export(ctx context.Context, project, scope string) (map[string]string, error)
	ExportEnvFile(ctx context.Context, project, scope string) (string, error)
	Import(ctx context.Context, project, scope string, secrets map[string]string) error
	ReEncryptAll(ctx context.Context) error
}

// SecretMetadata represents secret info without the value.
type SecretMetadata struct {
	ID        string
	Project   string
	Scope     string
	Key       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SettingsServicer defines the interface for settings management.
type SettingsServicer interface {
	IsInitialized(ctx context.Context) (bool, error)
	Get(ctx context.Context, category, key string) (string, error)
	GetRequired(ctx context.Context, category, key string) (string, error)
	GetRequiredInt(ctx context.Context, category, key string) (int, error)
	GetRequiredBool(ctx context.Context, category, key string) (bool, error)
	GetRequiredDuration(ctx context.Context, category, key string) (time.Duration, error)
	GetString(ctx context.Context, category, key, defaultVal string) (string, error)
	GetInt(ctx context.Context, category, key string, defaultVal int) (int, error)
	GetBool(ctx context.Context, category, key string, defaultVal bool) (bool, error)
	GetDuration(ctx context.Context, category, key string, defaultVal time.Duration) (time.Duration, error)
	Set(ctx context.Context, category, key, value string, encrypted bool) error
	SetInt(ctx context.Context, category, key string, value int) error
	SetBool(ctx context.Context, category, key string, value bool) error
	SetDuration(ctx context.Context, category, key string, value time.Duration) error
	SetRaw(ctx context.Context, category, key, value, valueType string, encrypted bool) error
	Delete(ctx context.Context, category, key string) error
	ListByCategory(ctx context.Context, category string) ([]SettingMetadata, error)
	ListAll(ctx context.Context) ([]SettingMetadata, error)
}

// SettingMetadata represents setting info.
type SettingMetadata struct {
	ID          string
	Category    string
	Key         string
	Value       string
	ValueType   string
	Encrypted   bool
	Description string
}

// CreateUserOptions holds optional parameters for user creation.
type CreateUserOptions struct {
	TOTPEnabled bool
	TOTPSecret  string
}

// CreateUserOption is a functional option for user creation.
type CreateUserOption func(*CreateUserOptions)

// WithTOTP enables TOTP for the new user with the provided secret.
func WithTOTP(secret string) CreateUserOption {
	return func(opts *CreateUserOptions) {
		opts.TOTPEnabled = true
		opts.TOTPSecret = secret
	}
}

// UserServicer defines the interface for user management.
type UserServicer interface {
	Create(ctx context.Context, username, password, email, role string, opts ...CreateUserOption) (*storage.User, error)
	GetByID(ctx context.Context, id string) (*storage.User, error)
	GetByUsername(ctx context.Context, username string) (*storage.User, error)
	List(ctx context.Context) ([]*storage.User, error)
	// H6: ListPaginated returns users with pagination support.
	ListPaginated(ctx context.Context, p Pagination) (*ListResult[*storage.User], error)
	Count(ctx context.Context) (int64, error)
	Update(ctx context.Context, user *storage.User) error
	Delete(ctx context.Context, id string) error
	DeleteWithCleanup(ctx context.Context, id string) error
	VerifyPassword(ctx context.Context, username, password string) (*storage.User, error)
	UpdatePassword(ctx context.Context, userID string, newPassword string) error
	SetTOTP(ctx context.Context, userID string, secret string, enabled bool) error
}

// SessionServicer defines the interface for session management.
type SessionServicer interface {
	Create(ctx context.Context, userID string, ipAddress, userAgent string, duration time.Duration) (*storage.Session, error)
	GetByToken(ctx context.Context, token string) (*storage.Session, error)
	Delete(ctx context.Context, token string) error
	DeleteAllForUser(ctx context.Context, userID string) error
	DeleteExpired(ctx context.Context) (int64, error)
	ListForUser(ctx context.Context, userID string) ([]*storage.Session, error)
}

// APIKeyServicer defines the interface for API key management.
type APIKeyServicer interface {
	Create(ctx context.Context, userID string, name string, scopes []string, expiresAt *time.Time) (rawKey string, key *storage.APIKey, err error)
	GetByID(ctx context.Context, keyID string) (*storage.APIKey, error)
	GetByRawKey(ctx context.Context, rawKey string) (*storage.APIKey, error)
	Delete(ctx context.Context, keyID string) error
	List(ctx context.Context, userID string) ([]*storage.APIKey, error)
	UpdateUsage(ctx context.Context, keyID string) error
	CleanupExpired(ctx context.Context) (int64, error)
}

// ProjectServicer defines the interface for project management.
type ProjectServicer interface {
	Create(ctx context.Context, name, repository, branch, deployPath, projectType string) (*storage.Project, error)
	GetByID(ctx context.Context, id string) (*storage.Project, error)
	GetByName(ctx context.Context, name string) (*storage.Project, error)
	List(ctx context.Context) ([]*storage.Project, error)
	ListPaginated(ctx context.Context, p Pagination) (*ListResult[*storage.Project], error)
	Count(ctx context.Context) (int64, error)
	UpdateByID(ctx context.Context, project *storage.Project) error
	Update(ctx context.Context, project *storage.Project) error
	DeleteByID(ctx context.Context, id string) error
	Delete(ctx context.Context, name string) error
	DeleteWithCleanup(ctx context.Context, name string) error
}

// WebhookServicer defines the interface for project webhook management.
type WebhookServicer interface {
	Get(ctx context.Context, projectID string, provider string) (*storage.ProjectWebhook, error)
	Set(ctx context.Context, projectID string, provider string, secret []byte, enabled, requireSecret bool) error
	List(ctx context.Context, projectID string) ([]*storage.ProjectWebhook, error)
	Delete(ctx context.Context, projectID string, provider string) error
	// CleanupOrphanedWebhooks removes webhooks referencing deleted projects.
	CleanupOrphanedWebhooks(ctx context.Context) (int64, error)
}

// DeploymentServicer defines the interface for deployment management.
type DeploymentServicer interface {
	Create(ctx context.Context, deployment *storage.DeploymentRecord) error
	GetByID(ctx context.Context, id string) (*storage.DeploymentRecord, error)
	Update(ctx context.Context, deployment *storage.DeploymentRecord) error
	ListRecent(ctx context.Context, limit int) ([]*storage.DeploymentRecord, error)
	ListPaginated(ctx context.Context, p Pagination) (*ListResult[*storage.DeploymentRecord], error)
	CountByStatus(ctx context.Context) (map[string]int64, error)
	Cancel(ctx context.Context, id string) error
	// Logs
	CreateLog(ctx context.Context, log *storage.DeploymentLog) error
	CreateLogsBatch(ctx context.Context, deploymentID string, logs []*storage.DeploymentLog) error
	ListLogs(ctx context.Context, deploymentID string) ([]*storage.DeploymentLog, error)
	ListLogsAfter(ctx context.Context, deploymentID string, afterID string) ([]*storage.DeploymentLog, error)
	ListLogsPaginated(ctx context.Context, deploymentID string, p Pagination) (*ListResult[*storage.DeploymentLog], error)
	// Scheduled
	CreateScheduled(ctx context.Context, id, project, target, branch string, scheduledAt time.Time, scheduledBy string) error
	ListPendingScheduled(ctx context.Context) ([]*storage.ScheduledDeployment, error)
	CancelScheduled(ctx context.Context, id string) error
	// Cleanup
	CleanupOld(ctx context.Context, cutoff time.Time) (int64, error)
	CleanupOldLogs(ctx context.Context, cutoff time.Time) (int64, error)
}

// AgentServicer defines the interface for agent management.
type AgentServicer interface {
	Upsert(ctx context.Context, agent *storage.Agent) error
	GetByID(ctx context.Context, id string) (*storage.Agent, error)
	List(ctx context.Context) ([]*storage.Agent, error)
	ListPaginated(ctx context.Context, p Pagination) (*ListResult[*storage.Agent], error)
	Count(ctx context.Context) (int64, error)
	CountByStatus(ctx context.Context) (map[string]int64, error)
	Delete(ctx context.Context, id string) error
	MarkStale(ctx context.Context, cutoff time.Time) (int64, error)
}

// AuditServicer defines the interface for audit logging.
type AuditServicer interface {
	Log(ctx context.Context, entry *storage.AuditEntry) error
	LogWithSnapshot(ctx context.Context, entry *storage.AuditEntry, resourceSnapshot any) error
	List(ctx context.Context, limit, offset int) ([]*storage.AuditEntry, error)
	Cleanup(ctx context.Context, cutoff time.Time) (int64, error)
}

// HostKeyServicer defines the interface for SSH host key management.
type HostKeyServicer interface {
	Create(ctx context.Context, key *storage.SSHHostKey) error
	Get(ctx context.Context, hostname string, port int, keyType string) (*storage.SSHHostKey, error)
	GetByHost(ctx context.Context, hostname string, port int) ([]*storage.SSHHostKey, error)
	List(ctx context.Context) ([]*storage.SSHHostKey, error)
	UpdateTrust(ctx context.Context, id string, trusted bool, verifiedBy string) error
	Delete(ctx context.Context, id string) error
	DeleteByHost(ctx context.Context, hostname string, port int) (int64, error)
}

// ProjectTypeServicer defines the interface for project type management.
type ProjectTypeServicer interface {
	Create(ctx context.Context, name, description, buildCmd string) (*storage.ProjectType, error)
	GetByName(ctx context.Context, name string) (*storage.ProjectType, error)
	List(ctx context.Context) ([]*storage.ProjectType, error)
	Update(ctx context.Context, pt *storage.ProjectType) error
	Delete(ctx context.Context, name string) error
}

// RateLimitServicer defines rate limiting operations.
type RateLimitServicer interface {
	// BlockIP blocks an IP address for the specified duration.
	BlockIP(ctx context.Context, ip, reason string, duration time.Duration, blockedBy string) error

	// UnblockIP removes a block on an IP address.
	UnblockIP(ctx context.Context, ip string) error

	// IsBlocked checks if an IP is currently blocked.
	IsBlocked(ctx context.Context, ip string) (bool, error)

	// GetBlock retrieves block details for an IP.
	GetBlock(ctx context.Context, ip string) (*storage.BlockedIP, error)

	// ListBlocked returns all blocked IPs with pagination.
	ListBlocked(ctx context.Context, pagination Pagination) (*ListResult[*storage.BlockedIP], error)

	// CleanupExpiredBlocks removes expired IP blocks.
	CleanupExpiredBlocks(ctx context.Context) (int64, error)

	// RecordRequest records a request for rate limiting.
	RecordRequest(ctx context.Context, key, bucket string, windowDuration time.Duration) error

	// GetRequestCount returns request count for a key within a window.
	GetRequestCount(ctx context.Context, key, bucket string, window time.Duration) (int64, error)

	// CleanupOldRequests removes old rate limit records.
	CleanupOldRequests(ctx context.Context, before time.Time) (int64, error)
}

// ProvisionServicer defines agent provisioning operations.
type ProvisionServicer interface {
	// CreateJob creates a new provisioning job.
	CreateJob(ctx context.Context, job *storage.ProvisionJob) error

	// GetJob retrieves a provisioning job by ID.
	GetJob(ctx context.Context, id string) (*storage.ProvisionJob, error)

	// UpdateStatus updates the status of a provisioning job.
	UpdateStatus(ctx context.Context, id, status, stage, errorMessage string, progress int) error

	// ListPending returns all pending provisioning jobs.
	ListPending(ctx context.Context) ([]*storage.ProvisionJob, error)

	// ListByHost returns provisioning jobs for a specific host.
	ListByHost(ctx context.Context, host string, pagination Pagination) (*ListResult[*storage.ProvisionJob], error)

	// Cancel cancels a pending provisioning job.
	Cancel(ctx context.Context, id string) error

	// Cleanup removes old completed/failed jobs.
	Cleanup(ctx context.Context, before time.Time) (int64, error)
}
