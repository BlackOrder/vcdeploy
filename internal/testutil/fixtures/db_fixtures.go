// Package testutil provides database test fixtures for vcdeploy.
package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// DBFixtures provides pre-built test data for database operations.
type DBFixtures struct {
	t *testing.T
}

// NewDBFixtures creates a new DBFixtures instance.
func NewDBFixtures(t *testing.T) *DBFixtures {
	return &DBFixtures{t: t}
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string { return &s }

// DefaultAgent returns a standard test agent.
func (f *DBFixtures) DefaultAgent() *storage.Agent {
	f.t.Helper()
	now := time.Now()
	return &storage.Agent{
		ID:           "agent-001",
		Hostname:     "test-server.example.com",
		Labels:       map[string]string{"env": "test", "role": "web"},
		Capabilities: `{"docker":true,"systemd":true}`,
		Status:       "online",
		LastSeenAt:   now,
		RegisteredAt: now.Add(-24 * time.Hour),
		Certificate:  "test-certificate-pem",
	}
}

// AgentWithID returns an agent with a specific ID.
func (f *DBFixtures) AgentWithID(id string) *storage.Agent {
	f.t.Helper()
	agent := f.DefaultAgent()
	agent.ID = id
	return agent
}

// OfflineAgent returns an agent that hasn't been seen recently.
func (f *DBFixtures) OfflineAgent() *storage.Agent {
	f.t.Helper()
	agent := f.DefaultAgent()
	agent.ID = "agent-offline"
	agent.Status = "offline"
	agent.LastSeenAt = time.Now().Add(-1 * time.Hour)
	return agent
}

// DefaultProject returns a standard test project.
func (f *DBFixtures) DefaultProject() *storage.Project {
	f.t.Helper()
	now := time.Now()
	return &storage.Project{
		ID:         "project-001",
		Name:       "test-project",
		Repository: "https://github.com/test/project.git",
		Branch:     "main",
		DeployPath: "/var/www/test-project",
		TypeID:     strPtr("nodejs"),
		CreatedAt:  now.Add(-7 * 24 * time.Hour),
		UpdatedAt:  now,
	}
}

// ProjectWithName returns a project with a specific name.
func (f *DBFixtures) ProjectWithName(name string) *storage.Project {
	f.t.Helper()
	project := f.DefaultProject()
	project.Name = name
	return project
}

// ProjectWithID returns a project with a specific ID.
func (f *DBFixtures) ProjectWithID(id string) *storage.Project {
	f.t.Helper()
	project := f.DefaultProject()
	project.ID = id
	return project
}

// PHPProject returns a PHP project fixture.
func (f *DBFixtures) PHPProject() *storage.Project {
	f.t.Helper()
	project := f.DefaultProject()
	project.Name = "php-app"
	project.TypeID = strPtr("php")
	project.DeployPath = "/var/www/php-app"
	return project
}

// PythonProject returns a Python project fixture.
func (f *DBFixtures) PythonProject() *storage.Project {
	f.t.Helper()
	project := f.DefaultProject()
	project.Name = "python-api"
	project.TypeID = strPtr("python")
	project.DeployPath = "/opt/apps/python-api"
	return project
}

// DefaultDeployment returns a standard test deployment.
func (f *DBFixtures) DefaultDeployment() *storage.DeploymentRecord {
	f.t.Helper()
	now := time.Now()
	return &storage.DeploymentRecord{
		ID:            "deploy-001",
		Project:       "test-project",
		Target:        "agent-001",
		Branch:        "main",
		CommitHash:    "abc123def456789",
		Status:        "completed",
		ReleaseNumber: 1,
		StartedAt:     now.Add(-5 * time.Minute),
		CompletedAt:   &now,
		TriggeredBy:   "testuser",
		TriggerSource: "api",
		ErrorMessage:  "",
	}
}

// DeploymentWithID returns a deployment with a specific ID.
func (f *DBFixtures) DeploymentWithID(id string) *storage.DeploymentRecord {
	f.t.Helper()
	deployment := f.DefaultDeployment()
	deployment.ID = id
	return deployment
}

// PendingDeployment returns a deployment in pending state.
func (f *DBFixtures) PendingDeployment() *storage.DeploymentRecord {
	f.t.Helper()
	deployment := f.DefaultDeployment()
	deployment.ID = "deploy-pending"
	deployment.Status = "pending"
	deployment.CompletedAt = nil
	return deployment
}

// RunningDeployment returns a deployment in running state.
func (f *DBFixtures) RunningDeployment() *storage.DeploymentRecord {
	f.t.Helper()
	deployment := f.DefaultDeployment()
	deployment.ID = "deploy-running"
	deployment.Status = "running"
	deployment.CompletedAt = nil
	return deployment
}

// FailedDeployment returns a deployment in failed state.
func (f *DBFixtures) FailedDeployment() *storage.DeploymentRecord {
	f.t.Helper()
	now := time.Now()
	deployment := f.DefaultDeployment()
	deployment.ID = "deploy-failed"
	deployment.Status = "failed"
	deployment.CompletedAt = &now
	deployment.ErrorMessage = "Build failed: npm install returned exit code 1"
	return deployment
}

// DefaultDeploymentLog returns a standard deployment log entry.
func (f *DBFixtures) DefaultDeploymentLog() *storage.DeploymentLog {
	f.t.Helper()
	return &storage.DeploymentLog{
		ID:           "log-001",
		DeploymentID: "deploy-001",
		Level:        "INFO",
		Message:      "Starting deployment",
		Source:       "agent",
		CreatedAt:    time.Now(),
	}
}

// LogWithLevel returns a log entry with a specific level.
func (f *DBFixtures) LogWithLevel(level, message string) *storage.DeploymentLog {
	f.t.Helper()
	log := f.DefaultDeploymentLog()
	log.Level = level
	log.Message = message
	return log
}

// ErrorLog returns an error-level log entry.
func (f *DBFixtures) ErrorLog(message string) *storage.DeploymentLog {
	f.t.Helper()
	return f.LogWithLevel("ERROR", message)
}

// DebugLog returns a debug-level log entry.
func (f *DBFixtures) DebugLog(message string) *storage.DeploymentLog {
	f.t.Helper()
	return f.LogWithLevel("DEBUG", message)
}

// DefaultUser returns a standard test user.
func (f *DBFixtures) DefaultUser() *storage.User {
	f.t.Helper()
	now := time.Now()
	return &storage.User{
		ID:                 "user-001",
		Username:           "testuser",
		PasswordHash:       "$2a$10$abcdefghijklmnopqrstuvwxyz", // bcrypt hash placeholder
		Email:              "test@example.com",
		Role:               "admin",
		TOTPSecret:         "",
		TOTPEnabled:        false,
		MustChangePassword: false,
		CreatedAt:          now.Add(-30 * 24 * time.Hour),
		UpdatedAt:          now,
	}
}

// UserWithRole returns a user with a specific role.
func (f *DBFixtures) UserWithRole(role string) *storage.User {
	f.t.Helper()
	user := f.DefaultUser()
	user.Role = role
	return user
}

// ViewerUser returns a user with viewer role.
func (f *DBFixtures) ViewerUser() *storage.User {
	f.t.Helper()
	user := f.DefaultUser()
	user.ID = "user-002"
	user.Username = "viewer"
	user.Email = "viewer@example.com"
	user.Role = "viewer"
	return user
}

// RegularUser returns a user with regular 'user' role.
func (f *DBFixtures) RegularUser() *storage.User {
	f.t.Helper()
	user := f.DefaultUser()
	user.ID = "user-003"
	user.Username = "regularuser"
	user.Email = "regularuser@example.com"
	user.Role = "user"
	return user
}

// DefaultAPIKey returns a standard test API key.
func (f *DBFixtures) DefaultAPIKey() *storage.APIKey {
	f.t.Helper()
	now := time.Now()
	expiresAt := now.Add(365 * 24 * time.Hour)
	return &storage.APIKey{
		ID:         "apikey-001",
		UserID:     "user-001",
		Name:       "test-api-key",
		KeyHash:    "sha256:abcdef123456",
		KeyPrefix:  "vcd_test",
		Scopes:     `["deploy:read","deploy:write","project:read"]`,
		ExpiresAt:  &expiresAt,
		LastUsedAt: nil,
		CreatedAt:  now,
	}
}

// ExpiredAPIKey returns an expired API key.
func (f *DBFixtures) ExpiredAPIKey() *storage.APIKey {
	f.t.Helper()
	key := f.DefaultAPIKey()
	key.ID = "apikey-002"
	key.Name = "expired-key"
	expiredAt := time.Now().Add(-24 * time.Hour)
	key.ExpiresAt = &expiredAt
	return key
}

// DefaultSecret returns a standard test secret.
func (f *DBFixtures) DefaultSecret() *storage.Secret {
	f.t.Helper()
	now := time.Now()
	return &storage.Secret{
		ID:             "secret-001",
		Project:        "test-project",
		Scope:          "global",
		Key:            "DATABASE_URL",
		ValueEncrypted: []byte("encrypted-value-placeholder"),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// SecretForProject returns a secret for a specific project.
func (f *DBFixtures) SecretForProject(project, key string) *storage.Secret {
	f.t.Helper()
	secret := f.DefaultSecret()
	secret.Project = project
	secret.Key = key
	return secret
}

// DefaultAuditEntry returns a standard audit entry.
func (f *DBFixtures) DefaultAuditEntry() *storage.AuditEntry {
	f.t.Helper()
	return &storage.AuditEntry{
		ID:        "audit-001",
		Timestamp: time.Now(),
		Source:    "api",
		User:      "testuser",
		Action:    "deployment.create",
		Resource:  "deployment",
		Details:   `{"project":"test-project","branch":"main"}`,
		IPAddress: "192.168.1.1",
		Result:    "success",
	}
}

// AuditEntryForAction returns an audit entry for a specific action.
func (f *DBFixtures) AuditEntryForAction(action, resource, result string) *storage.AuditEntry {
	f.t.Helper()
	entry := f.DefaultAuditEntry()
	entry.Action = action
	entry.Resource = resource
	entry.Result = result
	return entry
}

// DefaultSetting returns a standard setting entry.
func (f *DBFixtures) DefaultSetting() *storage.Setting {
	f.t.Helper()
	now := time.Now()
	return &storage.Setting{
		ID:          "setting-001",
		Category:    "app",
		Key:         "name",
		Value:       "VCDeploy Test",
		ValueType:   "string",
		Encrypted:   false,
		Description: "Application name",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// SettingWithKey returns a setting with a specific category/key/value.
func (f *DBFixtures) SettingWithKey(category, key, value string) *storage.Setting {
	f.t.Helper()
	setting := f.DefaultSetting()
	setting.Category = category
	setting.Key = key
	setting.Value = value
	return setting
}

// DefaultProjectType returns a standard project type.
func (f *DBFixtures) DefaultProjectType() *storage.ProjectType {
	f.t.Helper()
	now := time.Now()
	return &storage.ProjectType{
		ID:           "ptype-001",
		Name:         "nodejs",
		Description:  "Node.js application",
		BuildCmd:     "npm ci && npm run build",
		ProjectCount: 0,
		CreatedAt:    now,
	}
}

// PHPProjectType returns a PHP project type.
func (f *DBFixtures) PHPProjectType() *storage.ProjectType {
	f.t.Helper()
	pt := f.DefaultProjectType()
	pt.ID = "ptype-002"
	pt.Name = "php"
	pt.Description = "PHP application with Composer"
	pt.BuildCmd = "composer install --no-dev && php artisan migrate"
	return pt
}

// DefaultSession returns a standard session.
func (f *DBFixtures) DefaultSession() *storage.Session {
	f.t.Helper()
	now := time.Now()
	return &storage.Session{
		ID:        "session-001",
		UserID:    "user-001",
		Token:     "session-001",
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0 (Test Agent)",
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
		LastUsed:  now,
	}
}

// ExpiredSession returns an expired session.
func (f *DBFixtures) ExpiredSession() *storage.Session {
	f.t.Helper()
	session := f.DefaultSession()
	session.ID = "session-expired"
	session.ExpiresAt = time.Now().Add(-1 * time.Hour)
	return session
}

// SeedTestDB seeds a database with common test data.
func (f *DBFixtures) SeedTestDB(store storage.Store) error {
	f.t.Helper()
	ctx := context.Background()

	// Create agents (using UpsertAgent)
	for _, agent := range []*storage.Agent{
		f.DefaultAgent(),
		f.OfflineAgent(),
	} {
		if err := store.UpsertAgent(ctx, agent); err != nil {
			return err
		}
	}

	// Create users
	for _, user := range []*storage.User{
		f.DefaultUser(),
		f.ViewerUser(),
		f.RegularUser(),
	} {
		if err := store.CreateUser(ctx, user); err != nil {
			return err
		}
	}

	// Create project types
	for _, pt := range []*storage.ProjectType{
		f.DefaultProjectType(),
		f.PHPProjectType(),
	} {
		if err := store.CreateProjectType(ctx, pt); err != nil {
			return err
		}
	}

	return nil
}
