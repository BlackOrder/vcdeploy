// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"time"
)

// Store defines the interface for all storage operations.
// Implementations include SQLite (DB) and in-memory (MemoryStore).
type Store interface {
	// Lifecycle methods
	Close() error
	Conn() *sql.DB // Returns underlying connection (may be nil for memory-only stores)

	// Transaction support
	RunInTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error

	// Embed all entity-specific interfaces
	UserStore
	SessionStore
	APIKeyStore
	AgentStore
	AgentBinaryStore
	AgentUpdateStore
	DeploymentStore
	DeploymentLogStore
	DeploymentRollbackStore
	ScheduledDeploymentStore
	AuditStore
	SecretStore
	ProjectStore
	ProjectTypeStore
	ProjectWebhookStore
	SettingStore
	SSHHostKeyStore
	JumpServerStore
	BlockedIPStore
	RateLimitStore
	ProvisionStore
	HealthCheckStore
	CleanupStore
	BackupStore
}

// UserStore defines user-related operations.
type UserStore interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByID(ctx context.Context, id int64) (*User, error)
	ListUsers(ctx context.Context) ([]*User, error)
	CountUsers(ctx context.Context) (int64, error)
	UpdateUserByID(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, id int64) error
}

// SessionStore defines session-related operations.
type SessionStore interface {
	CreateSession(ctx context.Context, session *Session) error
	GetSessionByToken(ctx context.Context, token string) (*Session, error)
	DeleteSession(ctx context.Context, token string) error
	DeleteExpiredSessions(ctx context.Context) (int64, error)
	DeleteUserSessions(ctx context.Context, userID int64) error
	ListUserSessions(ctx context.Context, userID int64) ([]*Session, error)
}

// APIKeyStore defines API key-related operations.
type APIKeyStore interface {
	CreateAPIKey(ctx context.Context, key *APIKey) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error)
	UpdateAPIKeyUsage(ctx context.Context, keyID int64) error
	DeleteAPIKey(ctx context.Context, keyID int64) error
	ListAPIKeys(ctx context.Context, userID int64) ([]*APIKey, error)
}

// AgentStore defines agent-related operations.
type AgentStore interface {
	UpsertAgent(ctx context.Context, agent *Agent) error
	GetAgent(ctx context.Context, id string) (*Agent, error)
	ListAgents(ctx context.Context) ([]*Agent, error)
	CountAgents(ctx context.Context) (int64, error)
	CountAgentsByStatus(ctx context.Context) (map[string]int64, error)
	DeleteAgent(ctx context.Context, id string) error
	UpdateAgentVersion(ctx context.Context, agentID, version string) error
	UpdateAgentUpdateError(ctx context.Context, agentID, errorMsg string) error
	UpdateAgentUpdatePolicy(ctx context.Context, agentID, policy, windowStart, windowEnd string) error
	ListAgentsNeedingUpdate(ctx context.Context) ([]*Agent, error)
}

// AgentBinaryStore defines agent binary-related operations.
type AgentBinaryStore interface {
	CreateAgentBinary(ctx context.Context, binary *AgentBinary) error
	GetAgentBinary(ctx context.Context, id int64) (*AgentBinary, error)
	GetAgentBinaryByVersion(ctx context.Context, version, os, arch string) (*AgentBinary, error)
	GetCurrentAgentBinary(ctx context.Context, os, arch string) (*AgentBinary, error)
	ListAgentBinaries(ctx context.Context) ([]*AgentBinary, error)
	SetCurrentAgentBinary(ctx context.Context, id int64) error
	DeleteAgentBinary(ctx context.Context, id int64) error
}

// AgentUpdateStore defines agent update history operations.
type AgentUpdateStore interface {
	CreateAgentUpdateHistory(ctx context.Context, history *AgentUpdateHistory) error
	GetAgentUpdateHistory(ctx context.Context, id int64) (*AgentUpdateHistory, error)
	UpdateAgentUpdateHistory(ctx context.Context, history *AgentUpdateHistory) error
	ListAgentUpdateHistory(ctx context.Context, agentID string, limit, offset int) ([]*AgentUpdateHistory, int64, error)
	ListAllAgentUpdateHistory(ctx context.Context, limit, offset int) ([]*AgentUpdateHistory, int64, error)
	GetLatestAgentUpdateHistory(ctx context.Context, agentID string) (*AgentUpdateHistory, error)
}

// DeploymentStore defines deployment-related operations.
type DeploymentStore interface {
	CreateDeployment(ctx context.Context, d *DeploymentRecord) error
	GetDeployment(ctx context.Context, id string) (*DeploymentRecord, error)
	UpdateDeployment(ctx context.Context, d *DeploymentRecord) error
	ListDeploymentsRecent(ctx context.Context, limit int) ([]*DeploymentRecord, error)
	CountDeploymentsByStatus(ctx context.Context) (map[string]int64, error)

	// Legacy CLI methods
	InsertDeployment(d *DeploymentCLI) error
	SaveDeployment(d *DeploymentCLI) error
}

// DeploymentLogStore defines deployment log operations.
type DeploymentLogStore interface {
	CreateDeploymentLog(ctx context.Context, log *DeploymentLog) error
	ListDeploymentLogs(ctx context.Context, deploymentID string) ([]*DeploymentLog, error)
	ListDeploymentLogsAfter(ctx context.Context, deploymentID string, afterID int64) ([]*DeploymentLog, error)
}

// DeploymentRollbackStore defines rollback operations.
type DeploymentRollbackStore interface {
	CreateDeploymentRollback(ctx context.Context, rollback *DeploymentRollback) error
	GetDeploymentRollback(ctx context.Context, id int64) (*DeploymentRollback, error)
	UpdateDeploymentRollback(ctx context.Context, rollback *DeploymentRollback) error
	ListDeploymentRollbacks(ctx context.Context, projectName string, limit, offset int) ([]*DeploymentRollback, int64, error)
	GetLatestRollbackForDeployment(ctx context.Context, deploymentID string) (*DeploymentRollback, error)
}

// ScheduledDeploymentStore defines scheduled deployment operations.
type ScheduledDeploymentStore interface {
	CreateScheduledDeployment(ctx context.Context, id, project, target, branch string, scheduledAt time.Time, scheduledBy string) error
	ListPendingScheduledDeployments(ctx context.Context) ([]*ScheduledDeployment, error)
	CancelScheduledDeployment(ctx context.Context, id string) error
}

// AuditStore defines audit log operations.
type AuditStore interface {
	LogAudit(ctx context.Context, entry *AuditEntry) error
	LogAuditWithSnapshot(ctx context.Context, entry *AuditEntry, resourceSnapshot any) error
	ListAuditLogs(ctx context.Context, limit int, offset int) ([]*AuditEntry, error)
	ListAuditLogsSince(ctx context.Context, since time.Time) ([]*AuditEntry, error)
}

// SecretStore defines secret-related operations.
type SecretStore interface {
	SetSecretEncrypted(ctx context.Context, project, scope, key string, valueEncrypted []byte) error
	GetSecret(ctx context.Context, project, scope, key string) (*Secret, error)
	ListSecrets(scope string) ([]*SecretInfo, error)
	ListSecretsCtx(ctx context.Context, project string) ([]*Secret, error)
	ListSecretsWithScope(ctx context.Context, project, scope string) ([]*Secret, error)
	ListAllSecretsCtx(ctx context.Context) ([]*Secret, error)
	DeleteSecret(scope, key string) error
	DeleteSecretCtx(ctx context.Context, project, scope, key string) error
	ExportAllSecrets() (map[string]map[string]string, error)
}

// ProjectStore defines project-related operations.
type ProjectStore interface {
	CreateProject(project *Project) error
	GetProjectByName(ctx context.Context, name string) (*Project, error)
	ListProjects() ([]*Project, error)
	UpdateProjectByName(ctx context.Context, p *Project) error
	UpdateProjectHealthCheck(ctx context.Context, projectID int64, healthCheckID *int64, autoRollback, rollbackOnHealthFail bool) error
	DeleteProject(name string) error
}

// ProjectTypeStore defines project type operations.
type ProjectTypeStore interface {
	CreateProjectType(pt *ProjectType) error
	ListProjectTypes() ([]*ProjectType, error)
	GetProjectTypeByName(name string) (*ProjectType, error)
	UpdateProjectTypeByName(pt *ProjectType) error
	DeleteProjectType(name string) error
}

// ProjectWebhookStore defines project webhook operations.
type ProjectWebhookStore interface {
	GetProjectWebhook(ctx context.Context, projectID int64, provider string) (*ProjectWebhook, error)
	SetProjectWebhook(ctx context.Context, projectID int64, provider string, secretEncrypted []byte, enabled, requireSecret bool) error
	ListProjectWebhooks(ctx context.Context, projectID int64) ([]*ProjectWebhook, error)
	DeleteProjectWebhook(ctx context.Context, projectID int64, provider string) error
}

// SettingStore defines settings operations.
type SettingStore interface {
	GetSetting(ctx context.Context, category, key string) (*Setting, error)
	SetSetting(ctx context.Context, category, key, value, valueType string, encrypted bool) error
	ListSettingsByCategory(ctx context.Context, category string) ([]*Setting, error)
	ListAllSettings(ctx context.Context) ([]*Setting, error)
	DeleteSetting(ctx context.Context, category, key string) error
	HasSettings(ctx context.Context) (bool, error)
}

// SSHHostKeyStore defines SSH host key operations.
type SSHHostKeyStore interface {
	CreateSSHHostKey(ctx context.Context, key *SSHHostKey) error
	GetSSHHostKey(ctx context.Context, hostname string, port int, keyType string) (*SSHHostKey, error)
	GetSSHHostKeysByHost(ctx context.Context, hostname string, port int) ([]*SSHHostKey, error)
	ListSSHHostKeys(ctx context.Context) ([]*SSHHostKey, error)
	UpdateSSHHostKeyTrust(ctx context.Context, id int64, trusted bool, verifiedBy string) error
	DeleteSSHHostKey(ctx context.Context, id int64) error
	DeleteSSHHostKeysByHost(ctx context.Context, hostname string, port int) (int64, error)
}

// JumpServerStore defines SSH jump server operations.
type JumpServerStore interface {
	CreateJumpServer(ctx context.Context, js *SSHJumpServer) error
	GetJumpServer(ctx context.Context, id int64) (*SSHJumpServer, error)
	GetJumpServerByName(ctx context.Context, name string) (*SSHJumpServer, error)
	ListJumpServers(ctx context.Context) ([]*SSHJumpServer, error)
	UpdateJumpServer(ctx context.Context, js *SSHJumpServer) error
	DeleteJumpServer(ctx context.Context, id int64) error
}

// BlockedIPStore defines IP blocking operations.
type BlockedIPStore interface {
	BlockIP(ctx context.Context, block *BlockedIP) error
	UnblockIP(ctx context.Context, ip string) error
	IsIPBlocked(ctx context.Context, ip string) (bool, error)
	GetBlockedIP(ctx context.Context, ip string) (*BlockedIP, error)
	ListBlockedIPs(ctx context.Context, limit, offset int) ([]*BlockedIP, int64, error)
}

// RateLimitStore defines rate limiting operations.
type RateLimitStore interface {
	RecordRateLimitRequest(ctx context.Context, key, bucket string, windowStart, windowEnd time.Time) error
	GetRateLimitCount(ctx context.Context, key, bucket string, since time.Time) (int64, error)
}

// ProvisionStore defines provisioning job operations.
type ProvisionStore interface {
	CreateProvisionJob(ctx context.Context, job *ProvisionJob) error
	GetProvisionJob(ctx context.Context, id string) (*ProvisionJob, error)
	UpdateProvisionJobStatus(ctx context.Context, id, status, stage, errorMessage string, progress int) error
	ListPendingProvisionJobs(ctx context.Context) ([]*ProvisionJob, error)
	ListProvisionJobsByHost(ctx context.Context, host string, limit, offset int) ([]*ProvisionJob, int64, error)
}

// HealthCheckStore defines health check configuration operations.
type HealthCheckStore interface {
	CreateHealthCheckConfig(ctx context.Context, config *HealthCheckConfig) error
	GetHealthCheckConfig(ctx context.Context, id int64) (*HealthCheckConfig, error)
	GetGlobalHealthCheckConfig(ctx context.Context) (*HealthCheckConfig, error)
	GetHealthCheckConfigForProject(ctx context.Context, projectID int64) (*HealthCheckConfig, error)
	UpdateHealthCheckConfig(ctx context.Context, config *HealthCheckConfig) error
	ListHealthCheckConfigs(ctx context.Context) ([]*HealthCheckConfig, error)
	DeleteHealthCheckConfig(ctx context.Context, id int64) error
}

// CleanupStore defines cleanup/maintenance operations.
type CleanupStore interface {
	CleanupExpiredSessions(ctx context.Context, cutoff time.Time) (int64, error)
	CleanupOldDeployments(ctx context.Context, cutoff time.Time) (int64, error)
	CleanupOldDeploymentLogs(ctx context.Context, cutoff time.Time) (int64, error)
	CleanupOldAuditLogs(ctx context.Context, cutoff time.Time) (int64, error)
	CleanupExpiredBlockedIPs(ctx context.Context) (int64, error)
	CleanupRateLimitRecords(ctx context.Context, before time.Time) (int64, error)
	CleanupOldProvisionJobs(ctx context.Context, before time.Time) (int64, error)
	CleanupExpiredAPIKeys(ctx context.Context, now time.Time) (int64, error)
	CleanupOrphanedWebhooks(ctx context.Context) (int64, error)
	MarkStaleAgents(ctx context.Context, cutoff time.Time) (int64, error)
}

// BackupStore defines backup operations.
type BackupStore interface {
	Backup(destPath string) error
}

// Ensure DB implements Store at compile time.
var _ Store = (*DB)(nil)
