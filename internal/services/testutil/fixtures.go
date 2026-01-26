// Package testutil provides test helpers for service tests.
package testutil

import (
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// UserFixture creates a test user with sensible defaults.
func UserFixture(opts ...func(*storage.User)) *storage.User {
	now := time.Now()
	user := &storage.User{
		Username:     fmt.Sprintf("user_%d", now.UnixNano()),
		PasswordHash: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", // "password"
		Email:        fmt.Sprintf("user_%d@example.com", now.UnixNano()),
		Role:         "user",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	for _, opt := range opts {
		opt(user)
	}
	return user
}

// WithUsername sets the username.
func WithUsername(username string) func(*storage.User) {
	return func(u *storage.User) {
		u.Username = username
	}
}

// WithEmail sets the email.
func WithEmail(email string) func(*storage.User) {
	return func(u *storage.User) {
		u.Email = email
	}
}

// WithRole sets the role.
func WithRole(role string) func(*storage.User) {
	return func(u *storage.User) {
		u.Role = role
	}
}

// WithTOTP sets TOTP configuration.
func WithTOTP(secret string, enabled bool) func(*storage.User) {
	return func(u *storage.User) {
		u.TOTPSecret = secret
		u.TOTPEnabled = enabled
	}
}

// ProjectFixture creates a test project with sensible defaults.
func ProjectFixture(opts ...func(*storage.Project)) *storage.Project {
	now := time.Now()
	project := &storage.Project{
		Name:       fmt.Sprintf("project_%d", now.UnixNano()),
		Repository: "https://github.com/example/repo.git",
		Branch:     "main",
		DeployPath: "/var/www/project",
		Type:       "docker-compose",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	for _, opt := range opts {
		opt(project)
	}
	return project
}

// WithProjectName sets the project name.
func WithProjectName(name string) func(*storage.Project) {
	return func(p *storage.Project) {
		p.Name = name
	}
}

// WithRepository sets the repository URL.
func WithRepository(repo string) func(*storage.Project) {
	return func(p *storage.Project) {
		p.Repository = repo
	}
}

// WithBranch sets the branch.
func WithBranch(branch string) func(*storage.Project) {
	return func(p *storage.Project) {
		p.Branch = branch
	}
}

// WithDeployPath sets the deploy path.
func WithDeployPath(path string) func(*storage.Project) {
	return func(p *storage.Project) {
		p.DeployPath = path
	}
}

// WithProjectType sets the project type.
func WithProjectType(projectType string) func(*storage.Project) {
	return func(p *storage.Project) {
		p.Type = projectType
	}
}

// DeploymentFixture creates a test deployment with sensible defaults.
func DeploymentFixture(projectName string, opts ...func(*storage.DeploymentRecord)) *storage.DeploymentRecord {
	now := time.Now()
	deployment := &storage.DeploymentRecord{
		ID:            fmt.Sprintf("deploy_%d", now.UnixNano()),
		Project:       projectName,
		Target:        "production",
		Branch:        "main",
		CommitHash:    "abc123def456",
		Status:        "pending",
		ReleaseNumber: 1,
		StartedAt:     now,
		TriggeredBy:   "test-user",
		TriggerSource: "manual",
	}
	for _, opt := range opts {
		opt(deployment)
	}
	return deployment
}

// WithDeploymentID sets the deployment ID.
func WithDeploymentID(id string) func(*storage.DeploymentRecord) {
	return func(d *storage.DeploymentRecord) {
		d.ID = id
	}
}

// WithStatus sets the deployment status.
func WithStatus(status string) func(*storage.DeploymentRecord) {
	return func(d *storage.DeploymentRecord) {
		d.Status = status
	}
}

// WithTarget sets the deployment target.
func WithTarget(target string) func(*storage.DeploymentRecord) {
	return func(d *storage.DeploymentRecord) {
		d.Target = target
	}
}

// WithTriggeredBy sets who triggered the deployment.
func WithTriggeredBy(triggeredBy string) func(*storage.DeploymentRecord) {
	return func(d *storage.DeploymentRecord) {
		d.TriggeredBy = triggeredBy
	}
}

// WithCommitHash sets the commit hash.
func WithCommitHash(hash string) func(*storage.DeploymentRecord) {
	return func(d *storage.DeploymentRecord) {
		d.CommitHash = hash
	}
}

// WithCompletedAt sets the completion time.
func WithCompletedAt(t time.Time) func(*storage.DeploymentRecord) {
	return func(d *storage.DeploymentRecord) {
		d.CompletedAt = &t
	}
}

// AgentFixture creates a test agent with sensible defaults.
func AgentFixture(opts ...func(*storage.Agent)) *storage.Agent {
	now := time.Now()
	agent := &storage.Agent{
		ID:           fmt.Sprintf("agent_%d", now.UnixNano()),
		Hostname:     "test-host",
		Labels:       map[string]string{"env": "test"},
		Capabilities: `["docker", "compose"]`,
		Status:       "online",
		LastSeenAt:   now,
		RegisteredAt: now,
	}
	for _, opt := range opts {
		opt(agent)
	}
	return agent
}

// WithAgentID sets the agent ID.
func WithAgentID(id string) func(*storage.Agent) {
	return func(a *storage.Agent) {
		a.ID = id
	}
}

// WithHostname sets the hostname.
func WithHostname(hostname string) func(*storage.Agent) {
	return func(a *storage.Agent) {
		a.Hostname = hostname
	}
}

// WithAgentStatus sets the agent status.
func WithAgentStatus(status string) func(*storage.Agent) {
	return func(a *storage.Agent) {
		a.Status = status
	}
}

// WithLabels sets the agent labels.
func WithLabels(labels map[string]string) func(*storage.Agent) {
	return func(a *storage.Agent) {
		a.Labels = labels
	}
}

// SessionFixture creates a test session with sensible defaults.
func SessionFixture(userID int64, opts ...func(*storage.Session)) *storage.Session {
	now := time.Now()
	session := &storage.Session{
		ID:        fmt.Sprintf("session_%d", now.UnixNano()),
		UserID:    userID,
		Token:     fmt.Sprintf("token_%d", now.UnixNano()),
		IPAddress: "127.0.0.1",
		UserAgent: "Test Agent",
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
		LastUsed:  now,
	}
	for _, opt := range opts {
		opt(session)
	}
	return session
}

// WithSessionID sets the session ID.
func WithSessionID(id string) func(*storage.Session) {
	return func(s *storage.Session) {
		s.ID = id
	}
}

// WithToken sets the session token.
func WithToken(token string) func(*storage.Session) {
	return func(s *storage.Session) {
		s.Token = token
	}
}

// WithIPAddress sets the IP address.
func WithIPAddress(ip string) func(*storage.Session) {
	return func(s *storage.Session) {
		s.IPAddress = ip
	}
}

// WithExpiresAt sets the expiration time.
func WithExpiresAt(t time.Time) func(*storage.Session) {
	return func(s *storage.Session) {
		s.ExpiresAt = t
	}
}

// APIKeyFixture creates a test API key with sensible defaults.
func APIKeyFixture(userID int64, opts ...func(*storage.APIKey)) *storage.APIKey {
	now := time.Now()
	apiKey := &storage.APIKey{
		UserID:    userID,
		Name:      fmt.Sprintf("key_%d", now.UnixNano()),
		KeyHash:   "sha256_hash_placeholder",
		KeyPrefix: "vc_test_",
		Scopes:    `["read", "write"]`,
		CreatedAt: now,
	}
	for _, opt := range opts {
		opt(apiKey)
	}
	return apiKey
}

// WithKeyName sets the API key name.
func WithKeyName(name string) func(*storage.APIKey) {
	return func(k *storage.APIKey) {
		k.Name = name
	}
}

// WithScopes sets the API key scopes.
func WithScopes(scopes string) func(*storage.APIKey) {
	return func(k *storage.APIKey) {
		k.Scopes = scopes
	}
}

// WithKeyHash sets the API key hash.
func WithKeyHash(hash string) func(*storage.APIKey) {
	return func(k *storage.APIKey) {
		k.KeyHash = hash
	}
}

// WithKeyExpiresAt sets the API key expiration time.
func WithKeyExpiresAt(t time.Time) func(*storage.APIKey) {
	return func(k *storage.APIKey) {
		k.ExpiresAt = &t
	}
}

// AuditEntryFixture creates a test audit entry with sensible defaults.
func AuditEntryFixture(opts ...func(*storage.AuditEntry)) *storage.AuditEntry {
	now := time.Now()
	entry := &storage.AuditEntry{
		Timestamp: now,
		Source:    "test",
		User:      "test-user",
		Action:    "create",
		Resource:  "test-resource",
		Details:   "Test audit entry",
		IPAddress: "127.0.0.1",
		Result:    "success",
	}
	for _, opt := range opts {
		opt(entry)
	}
	return entry
}

// WithAuditAction sets the audit action.
func WithAuditAction(action string) func(*storage.AuditEntry) {
	return func(e *storage.AuditEntry) {
		e.Action = action
	}
}

// WithAuditResource sets the audit resource.
func WithAuditResource(resource string) func(*storage.AuditEntry) {
	return func(e *storage.AuditEntry) {
		e.Resource = resource
	}
}

// WithAuditUser sets the audit user.
func WithAuditUser(user string) func(*storage.AuditEntry) {
	return func(e *storage.AuditEntry) {
		e.User = user
	}
}

// WithAuditResult sets the audit result.
func WithAuditResult(result string) func(*storage.AuditEntry) {
	return func(e *storage.AuditEntry) {
		e.Result = result
	}
}

// SecretFixture creates a test secret with sensible defaults.
func SecretFixture(project, scope, key string, opts ...func(*storage.Secret)) *storage.Secret {
	now := time.Now()
	secret := &storage.Secret{
		Project:        project,
		Scope:          scope,
		Key:            key,
		ValueEncrypted: []byte("encrypted_value_placeholder"),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	for _, opt := range opts {
		opt(secret)
	}
	return secret
}

// WithEncryptedValue sets the encrypted value.
func WithEncryptedValue(value []byte) func(*storage.Secret) {
	return func(s *storage.Secret) {
		s.ValueEncrypted = value
	}
}

// DeploymentLogFixture creates a test deployment log with sensible defaults.
func DeploymentLogFixture(deploymentID string, opts ...func(*storage.DeploymentLog)) *storage.DeploymentLog {
	now := time.Now()
	log := &storage.DeploymentLog{
		DeploymentID: deploymentID,
		Level:        "info",
		Message:      "Test log message",
		Source:       "test",
		CreatedAt:    now,
	}
	for _, opt := range opts {
		opt(log)
	}
	return log
}

// WithLogLevel sets the log level.
func WithLogLevel(level string) func(*storage.DeploymentLog) {
	return func(l *storage.DeploymentLog) {
		l.Level = level
	}
}

// WithLogMessage sets the log message.
func WithLogMessage(message string) func(*storage.DeploymentLog) {
	return func(l *storage.DeploymentLog) {
		l.Message = message
	}
}

// ProjectTypeFixture creates a test project type with sensible defaults.
func ProjectTypeFixture(opts ...func(*storage.ProjectType)) *storage.ProjectType {
	now := time.Now()
	pt := &storage.ProjectType{
		Name:        fmt.Sprintf("type_%d", now.UnixNano()),
		Description: "Test project type",
		BuildCmd:    "make build",
		CreatedAt:   now,
	}
	for _, opt := range opts {
		opt(pt)
	}
	return pt
}

// WithTypeName sets the project type name.
func WithTypeName(name string) func(*storage.ProjectType) {
	return func(pt *storage.ProjectType) {
		pt.Name = name
	}
}

// WithTypeDescription sets the project type description.
func WithTypeDescription(desc string) func(*storage.ProjectType) {
	return func(pt *storage.ProjectType) {
		pt.Description = desc
	}
}

// WithBuildCmd sets the build command.
func WithBuildCmd(cmd string) func(*storage.ProjectType) {
	return func(pt *storage.ProjectType) {
		pt.BuildCmd = cmd
	}
}
