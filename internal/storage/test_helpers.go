package storage

import (
	"context"
	"testing"
	"time"
)

// TestMemoryStore provides a test-friendly MemoryStore with helper methods.
type TestMemoryStore struct {
	*MemoryStore
	t *testing.T
}

// NewTestMemoryStore creates a new MemoryStore for testing with no persistence.
// The store will be automatically closed when the test finishes.
func NewTestMemoryStore(t *testing.T) *TestMemoryStore {
	t.Helper()

	store := NewMemoryStore(nil)

	t.Cleanup(func() {
		store.Close()
	})

	return &TestMemoryStore{
		MemoryStore: store,
		t:           t,
	}
}

// MustCreateUser creates a user and fails the test if it errors.
func (s *TestMemoryStore) MustCreateUser(username, email, passwordHash string) *User {
	s.t.Helper()

	user := &User{
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
	}

	if err := s.CreateUser(context.Background(), user); err != nil {
		s.t.Fatalf("MustCreateUser() error = %v", err)
	}

	return user
}

// MustCreateProject creates a project and fails the test if it errors.
func (s *TestMemoryStore) MustCreateProject(name string) *Project {
	s.t.Helper()

	ctx := context.Background()
	project := &Project{Name: name}

	if err := s.CreateProject(ctx, project); err != nil {
		s.t.Fatalf("MustCreateProject() error = %v", err)
	}

	return project
}

// MustCreateSession creates a session and fails the test if it errors.
func (s *TestMemoryStore) MustCreateSession(userID int64, token string, expiresAt time.Time) *Session {
	s.t.Helper()

	session := &Session{
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
	}

	if err := s.CreateSession(context.Background(), session); err != nil {
		s.t.Fatalf("MustCreateSession() error = %v", err)
	}

	return session
}

// MustCreateAPIKey creates an API key and fails the test if it errors.
func (s *TestMemoryStore) MustCreateAPIKey(userID int64, name, keyHash string) *APIKey {
	s.t.Helper()

	key := &APIKey{
		UserID:  userID,
		Name:    name,
		KeyHash: keyHash,
	}

	if err := s.CreateAPIKey(context.Background(), key); err != nil {
		s.t.Fatalf("MustCreateAPIKey() error = %v", err)
	}

	return key
}

// MustUpsertAgent creates or updates an agent and fails the test if it errors.
func (s *TestMemoryStore) MustUpsertAgent(id, hostname, status string) *Agent {
	s.t.Helper()

	agent := &Agent{
		ID:       id,
		Hostname: hostname,
		Status:   AgentStatus(status),
	}

	if err := s.UpsertAgent(context.Background(), agent); err != nil {
		s.t.Fatalf("MustUpsertAgent() error = %v", err)
	}

	return agent
}

// MustCreateDeployment creates a deployment and fails the test if it errors.
func (s *TestMemoryStore) MustCreateDeployment(id, project, status string) *DeploymentRecord {
	s.t.Helper()

	deployment := &DeploymentRecord{
		ID:        id,
		Project:   project,
		Status:    DeploymentStatus(status),
		StartedAt: time.Now(),
	}

	if err := s.CreateDeployment(context.Background(), deployment); err != nil {
		s.t.Fatalf("MustCreateDeployment() error = %v", err)
	}

	return deployment
}

// MustSetSetting creates or updates a setting and fails the test if it errors.
func (s *TestMemoryStore) MustSetSetting(category, key, value string) {
	s.t.Helper()

	if err := s.SetSetting(context.Background(), category, key, value, "string", false); err != nil {
		s.t.Fatalf("MustSetSetting() error = %v", err)
	}
}

// MustBlockIP blocks an IP and fails the test if it errors.
func (s *TestMemoryStore) MustBlockIP(ip, reason string, expiresAt time.Time) *BlockedIP {
	s.t.Helper()

	blocked := &BlockedIP{
		IPAddress: ip,
		Reason:    reason,
		ExpiresAt: expiresAt,
	}

	if err := s.BlockIP(context.Background(), blocked); err != nil {
		s.t.Fatalf("MustBlockIP() error = %v", err)
	}

	return blocked
}

// MustLogAudit logs an audit entry and fails the test if it errors.
func (s *TestMemoryStore) MustLogAudit(action, user, resource string) *AuditEntry {
	s.t.Helper()

	entry := &AuditEntry{
		Action:    action,
		User:      user,
		Resource:  resource,
		Timestamp: time.Now(),
	}

	if err := s.LogAudit(context.Background(), entry); err != nil {
		s.t.Fatalf("MustLogAudit() error = %v", err)
	}

	return entry
}

// SeedTestData populates the store with common test data.
func (s *TestMemoryStore) SeedTestData() *TestData {
	s.t.Helper()

	data := &TestData{}

	// Create users
	data.AdminUser = s.MustCreateUser("admin", "admin@example.com", "adminpasshash")
	data.RegularUser = s.MustCreateUser("user1", "user1@example.com", "user1passhash")

	// Create projects
	data.Project1 = s.MustCreateProject("project-alpha")
	data.Project2 = s.MustCreateProject("project-beta")

	// Create sessions
	data.AdminSession = s.MustCreateSession(data.AdminUser.ID, "admin-session-token", time.Now().Add(24*time.Hour))
	data.UserSession = s.MustCreateSession(data.RegularUser.ID, "user-session-token", time.Now().Add(24*time.Hour))

	// Create API keys
	data.AdminAPIKey = s.MustCreateAPIKey(data.AdminUser.ID, "admin-key", "admin-key-hash")

	// Create agents
	data.Agent1 = s.MustUpsertAgent("agent-1", "server1.example.com", "online")
	data.Agent2 = s.MustUpsertAgent("agent-2", "server2.example.com", "online")

	// Create deployments
	data.Deployment1 = s.MustCreateDeployment("deploy-1", data.Project1.Name, "success")
	data.Deployment2 = s.MustCreateDeployment("deploy-2", data.Project2.Name, "pending")

	// Create settings
	s.MustSetSetting("app", "name", "vcdeploy")
	s.MustSetSetting("app", "version", "1.0.0")

	return data
}

// TestData holds references to seeded test data.
type TestData struct {
	AdminUser    *User
	RegularUser  *User
	Project1     *Project
	Project2     *Project
	AdminSession *Session
	UserSession  *Session
	AdminAPIKey  *APIKey
	Agent1       *Agent
	Agent2       *Agent
	Deployment1  *DeploymentRecord
	Deployment2  *DeploymentRecord
}
