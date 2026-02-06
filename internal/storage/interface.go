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
	DeploymentAgentStore
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
	ProvisionLogStore
	ACMEStore
	HealthCheckStore
	CleanupStore
	BackupStore

	// Security interfaces
	CertificateAuthorityStore
	AgentCertificateStore
	ServerCertificateStore
	RegistrationTokenStore
	SourceCredentialStore
	RevokedCertificateStore
	EncryptionKeyStore
	SSHKeyStore
	CertAuditStore
	RecoveryCodeStore

	// Recipe system interfaces
	RecipeComponentStore
	PlaybookStore
	PlaybookActivationStore
	PlaybookVariableBindingStore
	RawCommandApprovalStore
}

// UserStore defines user-related operations.
type UserStore interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByID(ctx context.Context, id int64) (*User, error)
	ListUsers(ctx context.Context) ([]*User, error)
	// H6: ListUsersPaginated returns users with pagination support.
	ListUsersPaginated(ctx context.Context, limit, offset int) ([]*User, error)
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
	GetAPIKeyByID(ctx context.Context, keyID int64) (*APIKey, error)
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
	ListAgentsPaginated(ctx context.Context, limit, offset int) ([]*Agent, error)
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
	ListDeploymentsPaginated(ctx context.Context, limit, offset int) ([]*DeploymentRecord, error)
	CountDeployments(ctx context.Context) (int64, error)
	CountDeploymentsByStatus(ctx context.Context) (map[string]int64, error)
}

// DeploymentLogStore defines deployment log operations.
type DeploymentLogStore interface {
	CreateDeploymentLog(ctx context.Context, log *DeploymentLog) error
	ListDeploymentLogs(ctx context.Context, deploymentID string) ([]*DeploymentLog, error)
	ListDeploymentLogsAfter(ctx context.Context, deploymentID string, afterID int64) ([]*DeploymentLog, error)
	ListDeploymentLogsPaginated(ctx context.Context, deploymentID string, limit, offset int) ([]*DeploymentLog, error)
	CountDeploymentLogs(ctx context.Context, deploymentID string) (int64, error)
}

// DeploymentAgentStore defines multi-agent deployment tracking operations.
type DeploymentAgentStore interface {
	AssignAgentToDeployment(ctx context.Context, deploymentID, agentID string) error
	AssignAgentsToDeployment(ctx context.Context, deploymentID string, agentIDs []string) error
	GetDeploymentAgents(ctx context.Context, deploymentID string) ([]DeploymentAgent, error)
	GetAgentDeployments(ctx context.Context, agentID string) ([]DeploymentAgent, error)
	UpdateDeploymentAgentStatus(ctx context.Context, deploymentID, agentID string, status DeploymentStatus, errorMsg string) error
	StartDeploymentAgent(ctx context.Context, deploymentID, agentID string) error
	IsAgentAssignedToDeployment(ctx context.Context, deploymentID, agentID string) (bool, error)
	GetDeploymentAgentStatus(ctx context.Context, deploymentID, agentID string) (*DeploymentAgent, error)
	CountDeploymentAgentsByStatus(ctx context.Context, deploymentID string) (map[DeploymentStatus]int, error)
	RemoveAgentFromDeployment(ctx context.Context, deploymentID, agentID string) error
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
	ListSecrets(ctx context.Context, scope string) ([]*SecretInfo, error)
	ListSecretsCtx(ctx context.Context, project string) ([]*Secret, error)
	ListSecretsWithScope(ctx context.Context, project, scope string) ([]*Secret, error)
	ListAllSecretsCtx(ctx context.Context) ([]*Secret, error)
	DeleteSecret(ctx context.Context, scope, key string) error
	DeleteSecretCtx(ctx context.Context, project, scope, key string) error
	ExportAllSecrets(ctx context.Context) (map[string]map[string]string, error)
}

// ProjectStore defines project-related operations.
type ProjectStore interface {
	CreateProject(ctx context.Context, project *Project) error
	GetProjectByID(ctx context.Context, id int64) (*Project, error)
	GetProjectByName(ctx context.Context, name string) (*Project, error)
	ListProjects(ctx context.Context) ([]*Project, error)
	ListProjectsPaginated(ctx context.Context, limit, offset int) ([]*Project, error)
	CountProjects(ctx context.Context) (int64, error)
	UpdateProjectByID(ctx context.Context, p *Project) error
	UpdateProjectByName(ctx context.Context, p *Project) error
	UpdateProjectHealthCheck(ctx context.Context, projectID int64, healthCheckID *int64, autoRollback, rollbackOnHealthFail bool) error
	DeleteProjectByID(ctx context.Context, id int64) error
	DeleteProject(ctx context.Context, name string) error
}

// ProjectTypeStore defines project type operations.
type ProjectTypeStore interface {
	CreateProjectType(ctx context.Context, pt *ProjectType) error
	ListProjectTypes(ctx context.Context) ([]*ProjectType, error)
	GetProjectTypeByName(ctx context.Context, name string) (*ProjectType, error)
	UpdateProjectTypeByName(ctx context.Context, pt *ProjectType) error
	DeleteProjectType(ctx context.Context, name string) error
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

// ProvisionLogStore defines provision log operations.
type ProvisionLogStore interface {
	SaveProvisionLog(ctx context.Context, jobID, level, message string) error
	ListProvisionLogs(ctx context.Context, jobID string) ([]*ProvisionLog, error)
}

// ACMEStore defines ACME certificate and account storage operations.
type ACMEStore interface {
	// Certificate operations
	GetACMECertificate(ctx context.Context, domain string) (*ACMECertificate, error)
	SaveACMECertificate(ctx context.Context, cert *ACMECertificate) error
	DeleteACMECertificate(ctx context.Context, domain string) error
	ListACMECertificates(ctx context.Context) ([]*ACMECertificate, error)

	// Account operations
	GetACMEAccount(ctx context.Context, email string) (*ACMEAccount, error)
	SaveACMEAccount(ctx context.Context, account *ACMEAccount) error
	DeleteACMEAccount(ctx context.Context, email string) error
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

// --- Security Store Interfaces ---

// CertificateAuthorityStore defines CA-related operations.
type CertificateAuthorityStore interface {
	// GetCA returns a CA by ID.
	GetCA(ctx context.Context, id string) (*CertificateAuthority, error)
	// GetCurrentCA returns the currently active CA.
	GetCurrentCA(ctx context.Context) (*CertificateAuthority, error)
	// ListCAs returns all certificate authorities.
	ListCAs(ctx context.Context) ([]*CertificateAuthority, error)
	// SaveCA creates or updates a certificate authority.
	SaveCA(ctx context.Context, ca *CertificateAuthority) error
	// SetCurrentCA marks a CA as current and deactivates others.
	SetCurrentCA(ctx context.Context, id string) error
}

// AgentCertificateStore defines agent certificate operations.
type AgentCertificateStore interface {
	// GetAgentCert returns the active certificate for an agent.
	GetAgentCert(ctx context.Context, agentID string) (*AgentCertificate, error)
	// GetAgentCertBySerial returns a certificate by serial number.
	GetAgentCertBySerial(ctx context.Context, serialNumber string) (*AgentCertificate, error)
	// ListAgentCerts returns all agent certificates.
	ListAgentCerts(ctx context.Context) ([]*AgentCertificate, error)
	// ListAgentCertsByAgent returns all certificates for an agent.
	ListAgentCertsByAgent(ctx context.Context, agentID string) ([]*AgentCertificate, error)
	// SaveAgentCert creates or updates an agent certificate.
	SaveAgentCert(ctx context.Context, cert *AgentCertificate) error
	// RevokeAgentCert revokes an agent's certificate.
	RevokeAgentCert(ctx context.Context, agentID, reason, revokedBy string) error
	// RevokeAgentCertBySerial revokes a specific certificate by serial.
	RevokeAgentCertBySerial(ctx context.Context, serialNumber, reason, revokedBy string) error
	// UpdateAgentCertStatus updates certificate status (e.g., mark expired).
	UpdateAgentCertStatus(ctx context.Context, serialNumber, status string) error
}

// ServerCertificateStore defines server certificate operations.
type ServerCertificateStore interface {
	// GetServerCert returns the certificate for a hostname.
	GetServerCert(ctx context.Context, hostname string) (*ServerCertificate, error)
	// ListServerCerts returns all server certificates.
	ListServerCerts(ctx context.Context) ([]*ServerCertificate, error)
	// SaveServerCert creates or updates a server certificate.
	SaveServerCert(ctx context.Context, cert *ServerCertificate) error
	// DeleteServerCert removes a server certificate.
	DeleteServerCert(ctx context.Context, hostname string) error
}

// RegistrationTokenStore defines registration token operations.
type RegistrationTokenStore interface {
	// GetRegistrationToken returns a token by its value.
	GetRegistrationToken(ctx context.Context, token string) (*RegistrationToken, error)
	// ListRegistrationTokens returns all registration tokens.
	ListRegistrationTokens(ctx context.Context) ([]*RegistrationToken, error)
	// SaveRegistrationToken creates or updates a registration token.
	SaveRegistrationToken(ctx context.Context, rt *RegistrationToken) error
	// DeleteRegistrationToken removes a registration token.
	DeleteRegistrationToken(ctx context.Context, token string) error
	// MarkTokenUsed marks a token as used.
	MarkTokenUsed(ctx context.Context, token string) error
	// CleanupExpiredTokens removes expired tokens.
	CleanupExpiredTokens(ctx context.Context) (int64, error)
}

// SourceCredentialStore defines source credential operations.
type SourceCredentialStore interface {
	// GetSourceCredential returns a credential by ID.
	GetSourceCredential(ctx context.Context, id int64) (*SourceCredential, error)
	// GetSourceCredentialByName returns a credential by name.
	GetSourceCredentialByName(ctx context.Context, name string) (*SourceCredential, error)
	// ListSourceCredentials returns all source credentials.
	ListSourceCredentials(ctx context.Context) ([]*SourceCredential, error)
	// SaveSourceCredential creates or updates a source credential.
	SaveSourceCredential(ctx context.Context, cred *SourceCredential) error
	// DeleteSourceCredential removes a source credential.
	DeleteSourceCredential(ctx context.Context, id int64) error
}

// RevokedCertificateStore defines CRL operations.
type RevokedCertificateStore interface {
	// IsRevoked checks if a certificate serial is revoked.
	IsRevoked(ctx context.Context, serialNumber string) (bool, error)
	// ListRevokedCerts returns all revoked certificates.
	ListRevokedCerts(ctx context.Context) ([]*RevokedCertificate, error)
	// SaveRevokedCert adds a certificate to the revocation list.
	SaveRevokedCert(ctx context.Context, revoked *RevokedCertificate) error
}

// EncryptionKeyStore defines KMS key operations.
type EncryptionKeyStore interface {
	// GetEncryptionKey returns a key by ID.
	GetEncryptionKey(ctx context.Context, id string) (*EncryptionKey, error)
	// GetCurrentEncryptionKey returns the currently active encryption key.
	GetCurrentEncryptionKey(ctx context.Context) (*EncryptionKey, error)
	// ListEncryptionKeys returns all encryption keys.
	ListEncryptionKeys(ctx context.Context) ([]*EncryptionKey, error)
	// SaveEncryptionKey creates or updates an encryption key.
	SaveEncryptionKey(ctx context.Context, key *EncryptionKey) error
	// UpdateEncryptionKeyStatus updates a key's status.
	UpdateEncryptionKeyStatus(ctx context.Context, id string, status string, scheduledDeletion *time.Time) error
}

// SSHKeyStore defines SSH key operations.
type SSHKeyStore interface {
	// GetSSHKey returns an SSH key by ID.
	GetSSHKey(ctx context.Context, id int64) (*SSHKey, error)
	// GetSSHKeyByName returns an SSH key by name.
	GetSSHKeyByName(ctx context.Context, name string) (*SSHKey, error)
	// ListSSHKeys returns all SSH keys.
	ListSSHKeys(ctx context.Context) ([]*SSHKey, error)
	// SaveSSHKey creates or updates an SSH key.
	SaveSSHKey(ctx context.Context, key *SSHKey) error
	// DeleteSSHKey removes an SSH key.
	DeleteSSHKey(ctx context.Context, id int64) error
}

// CertAuditStore defines certificate audit log operations.
type CertAuditStore interface {
	// SaveCertAuditEvent logs a certificate audit event.
	SaveCertAuditEvent(ctx context.Context, event *CertAuditEvent) error
	// ListCertAuditEvents returns certificate audit events with optional filtering.
	ListCertAuditEvents(ctx context.Context, filter CertAuditFilter) ([]*CertAuditEvent, error)
}

// RecoveryCodeStore defines TOTP recovery code operations.
type RecoveryCodeStore interface {
	// SaveRecoveryCodes saves a set of recovery codes for a user (replaces any existing).
	SaveRecoveryCodes(ctx context.Context, userID int64, codes []*RecoveryCode) error
	// ListRecoveryCodes returns all recovery codes for a user.
	ListRecoveryCodes(ctx context.Context, userID int64) ([]*RecoveryCode, error)
	// UseRecoveryCode marks a recovery code as used.
	UseRecoveryCode(ctx context.Context, codeID int64) error
	// DeleteRecoveryCodes removes all recovery codes for a user.
	DeleteRecoveryCodes(ctx context.Context, userID int64) error
	// CountUnusedRecoveryCodes returns the count of unused codes for a user.
	CountUnusedRecoveryCodes(ctx context.Context, userID int64) (int, error)
}

// --- Recipe System Store Interfaces ---

// RecipeComponentStore defines recipe component operations.
type RecipeComponentStore interface {
	// CreateRecipeComponent creates a new recipe component.
	CreateRecipeComponent(ctx context.Context, component *RecipeComponent) error
	// GetRecipeComponent returns a component by namespace, slug, and version.
	GetRecipeComponent(ctx context.Context, namespace, slug, version string) (*RecipeComponent, error)
	// GetRecipeComponentByID returns a component by ID.
	GetRecipeComponentByID(ctx context.Context, id int64) (*RecipeComponent, error)
	// ListRecipeComponents returns all components in a namespace.
	ListRecipeComponents(ctx context.Context, namespace string, includeDeprecated bool) ([]*RecipeComponent, error)
	// ListRecipeComponentVersions returns all versions of a component.
	ListRecipeComponentVersions(ctx context.Context, namespace, slug string) ([]*RecipeComponent, error)
	// UpdateRecipeComponent updates an existing component.
	UpdateRecipeComponent(ctx context.Context, component *RecipeComponent) error
	// DeleteRecipeComponent deletes a component by ID.
	DeleteRecipeComponent(ctx context.Context, id int64) error
}

// PlaybookStore defines playbook operations.
type PlaybookStore interface {
	// CreatePlaybook creates a new playbook.
	CreatePlaybook(ctx context.Context, playbook *Playbook) error
	// GetPlaybook returns a playbook by namespace, slug, and version.
	GetPlaybook(ctx context.Context, namespace, slug, version string) (*Playbook, error)
	// GetPlaybookByID returns a playbook by ID.
	GetPlaybookByID(ctx context.Context, id int64) (*Playbook, error)
	// ListPlaybooks returns playbooks filtered by namespace and/or framework type.
	ListPlaybooks(ctx context.Context, namespace, frameworkType string, includeDeprecated bool) ([]*Playbook, error)
	// ListPlaybookVersions returns all versions of a playbook.
	ListPlaybookVersions(ctx context.Context, namespace, slug string) ([]*Playbook, error)
	// UpdatePlaybook updates an existing playbook.
	UpdatePlaybook(ctx context.Context, playbook *Playbook) error
	// DeletePlaybook deletes a playbook by ID.
	DeletePlaybook(ctx context.Context, id int64) error
}

// PlaybookActivationStore defines playbook activation operations.
type PlaybookActivationStore interface {
	// CreatePlaybookActivation creates a new activation linking a project to a playbook.
	CreatePlaybookActivation(ctx context.Context, activation *PlaybookActivation) error
	// GetPlaybookActivation returns the activation for a project.
	GetPlaybookActivation(ctx context.Context, projectID int64) (*PlaybookActivation, error)
	// GetPlaybookActivationByID returns an activation by ID.
	GetPlaybookActivationByID(ctx context.Context, id int64) (*PlaybookActivation, error)
	// ListActivationsByPlaybook returns all activations using a specific playbook.
	ListActivationsByPlaybook(ctx context.Context, playbookID int64) ([]*PlaybookActivation, error)
	// DeletePlaybookActivation deletes an activation by ID.
	DeletePlaybookActivation(ctx context.Context, id int64) error
}

// PlaybookVariableBindingStore defines variable binding operations.
type PlaybookVariableBindingStore interface {
	// CreateVariableBinding creates a new variable binding.
	CreateVariableBinding(ctx context.Context, binding *PlaybookVariableBinding) error
	// GetVariableBindings returns all bindings for an activation.
	GetVariableBindings(ctx context.Context, activationID int64) ([]*PlaybookVariableBinding, error)
	// UpdateVariableBinding updates an existing binding.
	UpdateVariableBinding(ctx context.Context, binding *PlaybookVariableBinding) error
	// DeleteVariableBinding deletes a binding by ID.
	DeleteVariableBinding(ctx context.Context, id int64) error
	// FindBindingsBySourceRef finds bindings that reference a specific source (env key or secret).
	FindBindingsBySourceRef(ctx context.Context, sourceType, sourceRef string) ([]*PlaybookVariableBinding, error)
}

// RawCommandApprovalStore defines RAW command approval operations.
type RawCommandApprovalStore interface {
	// CreateRawApproval creates an approval record for a RAW component.
	CreateRawApproval(ctx context.Context, approval *RawCommandApproval) error
	// GetRawApproval returns the approval for a component.
	GetRawApproval(ctx context.Context, componentID int64) (*RawCommandApproval, error)
	// DeleteRawApproval deletes an approval by component ID.
	DeleteRawApproval(ctx context.Context, componentID int64) error
	// ListRawApprovals returns all RAW command approvals.
	ListRawApprovals(ctx context.Context) ([]*RawCommandApproval, error)
}

// Ensure DB implements Store at compile time.
var _ Store = (*DB)(nil)
