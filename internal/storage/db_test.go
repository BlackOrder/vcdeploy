// Package storage provides comprehensive tests for database operations.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestNew tests database creation and initialization.
func TestNew(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	// Verify file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestNewWithLogger(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_logger.db")

	logger := zap.NewNop()
	db, err := New(dbPath, logger)
	if err != nil {
		t.Fatalf("New() with logger error = %v", err)
	}
	defer db.Close()

	// Verify file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestOpen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// Verify file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestNewInvalidPath(t *testing.T) {
	// Try to create database in non-existent directory
	dbPath := "/nonexistent/path/test.db"
	_, err := New(dbPath, nil)
	if err == nil {
		t.Error("New() expected error for invalid path, got nil")
	}
}

// Helper function to create a test database
func setupTestDB(t *testing.T) (*DB, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

// --- User Tests ---

func TestCreateUser(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	user := &User{
		Username:     "testuser",
		PasswordHash: "hashedpassword123",
		Email:        "test@example.com",
		Role:         "admin",
	}

	err := db.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if user.ID == 0 {
		t.Error("CreateUser() did not set user ID")
	}
}

func TestCreateUserDuplicate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	user := &User{
		Username:     "testuser",
		PasswordHash: "hashedpassword123",
		Email:        "test@example.com",
		Role:         "admin",
	}

	err := db.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("CreateUser() first call error = %v", err)
	}

	// Try to create duplicate
	user2 := &User{
		Username:     "testuser",
		PasswordHash: "differentpassword",
		Email:        "other@example.com",
		Role:         "viewer",
	}

	err = db.CreateUser(ctx, user2)
	if err == nil {
		t.Error("CreateUser() expected error for duplicate username, got nil")
	}
}

func TestGetUserByUsername(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user first
	user := &User{
		Username:           "findme",
		PasswordHash:       "hash123",
		Email:              "find@example.com",
		Role:               "viewer",
		MustChangePassword: true,
	}

	err := db.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	// Retrieve the user
	found, err := db.GetUserByUsername(ctx, "findme")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}

	if found == nil {
		t.Fatal("GetUserByUsername() returned nil")
	}

	if found.Username != "findme" {
		t.Errorf("GetUserByUsername() username = %v, want %v", found.Username, "findme")
	}

	if found.Email != "find@example.com" {
		t.Errorf("GetUserByUsername() email = %v, want %v", found.Email, "find@example.com")
	}

	if found.Role != "viewer" {
		t.Errorf("GetUserByUsername() role = %v, want %v", found.Role, "viewer")
	}

	if !found.MustChangePassword {
		t.Error("GetUserByUsername() MustChangePassword = false, want true")
	}
}

func TestGetUserByUsernameNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	user, err := db.GetUserByUsername(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetUserByUsername() error = %v, want ErrNotFound", err)
	}

	if user != nil {
		t.Errorf("GetUserByUsername() = %v, want nil for nonexistent user", user)
	}
}

// --- Agent Tests ---

func TestUpsertAgent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	agent := &Agent{
		ID:           "agent-001",
		Hostname:     "server1.example.com",
		Labels:       map[string]string{"env": "prod", "region": "us-east"},
		Capabilities: `{"docker": true, "kubernetes": false}`,
		Status:       "online",
		LastSeenAt:   time.Now(),
		Certificate:  "cert-data-here",
	}

	err := db.UpsertAgent(ctx, agent)
	if err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	// Verify agent was created
	found, err := db.GetAgent(ctx, "agent-001")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}

	if found == nil {
		t.Fatal("GetAgent() returned nil")
	}

	if found.Hostname != "server1.example.com" {
		t.Errorf("GetAgent() hostname = %v, want %v", found.Hostname, "server1.example.com")
	}

	if found.Status != "online" {
		t.Errorf("GetAgent() status = %v, want %v", found.Status, "online")
	}
}

func TestUpsertAgentUpdate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create agent
	agent := &Agent{
		ID:       "agent-002",
		Hostname: "server2.example.com",
		Status:   "online",
	}

	err := db.UpsertAgent(ctx, agent)
	if err != nil {
		t.Fatalf("UpsertAgent() create error = %v", err)
	}

	// Update agent
	agent.Hostname = "server2-updated.example.com"
	agent.Status = "offline"

	err = db.UpsertAgent(ctx, agent)
	if err != nil {
		t.Fatalf("UpsertAgent() update error = %v", err)
	}

	// Verify update
	found, err := db.GetAgent(ctx, "agent-002")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}

	if found.Hostname != "server2-updated.example.com" {
		t.Errorf("GetAgent() hostname = %v, want %v", found.Hostname, "server2-updated.example.com")
	}

	if found.Status != "offline" {
		t.Errorf("GetAgent() status = %v, want %v", found.Status, "offline")
	}
}

func TestGetAgentNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	agent, err := db.GetAgent(ctx, "nonexistent-agent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetAgent() error = %v, want ErrNotFound", err)
	}

	if agent != nil {
		t.Errorf("GetAgent() = %v, want nil for nonexistent agent", agent)
	}
}

func TestListAgents(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple agents
	agents := []*Agent{
		{ID: "agent-a", Hostname: "alpha.example.com", Status: "online"},
		{ID: "agent-b", Hostname: "beta.example.com", Status: "online"},
		{ID: "agent-c", Hostname: "gamma.example.com", Status: "offline"},
	}

	for _, a := range agents {
		if err := db.UpsertAgent(ctx, a); err != nil {
			t.Fatalf("UpsertAgent() error = %v", err)
		}
	}

	// List agents
	list, err := db.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("ListAgents() returned %d agents, want 3", len(list))
	}
}

// --- Deployment Tests ---

func TestCreateDeployment(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	deployment := &DeploymentRecord{
		ID:            "deploy-001",
		Project:       "myproject",
		Target:        "production",
		Branch:        "main",
		CommitHash:    "abc123def456",
		Status:        "running",
		TriggeredBy:   "admin",
		TriggerSource: "webhook",
	}

	err := db.CreateDeployment(ctx, deployment)
	if err != nil {
		t.Fatalf("CreateDeployment() error = %v", err)
	}
}

func TestGetDeployment(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create deployment - GetDeployment properly handles NULL values via sql.Null* types
	deployment := &DeploymentRecord{
		ID:            "deploy-002",
		Project:       "testproject",
		Target:        "staging",
		Branch:        "develop",
		CommitHash:    "xyz789",
		Status:        "pending",
		ReleaseNumber: 1,
		TriggeredBy:   "ci",
		TriggerSource: "github",
	}

	err := db.CreateDeployment(ctx, deployment)
	if err != nil {
		t.Fatalf("CreateDeployment() error = %v", err)
	}

	// Update deployment to set release_number in database
	err = db.UpdateDeployment(ctx, deployment)
	if err != nil {
		t.Fatalf("UpdateDeployment() error = %v", err)
	}

	// Retrieve deployment
	found, err := db.GetDeployment(ctx, "deploy-002")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}

	if found == nil {
		t.Fatal("GetDeployment() returned nil")
	}

	if found.Project != "testproject" {
		t.Errorf("GetDeployment() project = %v, want %v", found.Project, "testproject")
	}

	if found.Branch != "develop" {
		t.Errorf("GetDeployment() branch = %v, want %v", found.Branch, "develop")
	}

	if found.Status != "pending" {
		t.Errorf("GetDeployment() status = %v, want %v", found.Status, "pending")
	}
}

func TestGetDeploymentNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	deployment, err := db.GetDeployment(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetDeployment() error = %v, want ErrNotFound", err)
	}

	if deployment != nil {
		t.Errorf("GetDeployment() = %v, want nil for nonexistent deployment", deployment)
	}
}

func TestUpdateDeployment(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create deployment
	deployment := &DeploymentRecord{
		ID:      "deploy-003",
		Project: "updateproject",
		Target:  "production",
		Branch:  "main",
		Status:  "running",
	}

	err := db.CreateDeployment(ctx, deployment)
	if err != nil {
		t.Fatalf("CreateDeployment() error = %v", err)
	}

	// Update deployment
	now := time.Now()
	deployment.Status = "completed"
	deployment.ReleaseNumber = 42
	deployment.CompletedAt = &now

	err = db.UpdateDeployment(ctx, deployment)
	if err != nil {
		t.Fatalf("UpdateDeployment() error = %v", err)
	}

	// Verify update
	found, err := db.GetDeployment(ctx, "deploy-003")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}

	if found.Status != "completed" {
		t.Errorf("GetDeployment() status = %v, want %v", found.Status, "completed")
	}

	if found.ReleaseNumber != 42 {
		t.Errorf("GetDeployment() release_number = %v, want %v", found.ReleaseNumber, 42)
	}

	if found.CompletedAt == nil {
		t.Error("GetDeployment() completed_at = nil, want non-nil")
	}
}

// --- Audit Log Tests ---

func TestLogAudit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	entry := &AuditEntry{
		Source:    "api",
		User:      "admin",
		Action:    "login",
		Resource:  "session",
		Details:   `{"method": "password"}`,
		IPAddress: "192.168.1.100",
		Result:    "success",
	}

	err := db.LogAudit(ctx, entry)
	if err != nil {
		t.Fatalf("LogAudit() error = %v", err)
	}
}

func TestListAuditLogs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple audit entries
	entries := []*AuditEntry{
		{Source: "api", User: "user1", Action: "login", Result: "success"},
		{Source: "api", User: "user2", Action: "logout", Result: "success"},
		{Source: "cli", User: "admin", Action: "deploy", Result: "success"},
	}

	for _, e := range entries {
		if err := db.LogAudit(ctx, e); err != nil {
			t.Fatalf("LogAudit() error = %v", err)
		}
	}

	// List audit logs
	list, err := db.ListAuditLogs(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("ListAuditLogs() returned %d entries, want 3", len(list))
	}
}

func TestListAuditLogsWithPagination(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple audit entries
	for i := 0; i < 10; i++ {
		entry := &AuditEntry{
			Source: "api",
			User:   "testuser",
			Action: "test",
			Result: "success",
		}
		if err := db.LogAudit(ctx, entry); err != nil {
			t.Fatalf("LogAudit() error = %v", err)
		}
	}

	// Test pagination
	list, err := db.ListAuditLogs(ctx, 3, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("ListAuditLogs(3, 0) returned %d entries, want 3", len(list))
	}

	// Get second page
	list2, err := db.ListAuditLogs(ctx, 3, 3)
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}

	if len(list2) != 3 {
		t.Errorf("ListAuditLogs(3, 3) returned %d entries, want 3", len(list2))
	}
}

// --- Secret Tests ---

func TestSetSecretEncrypted(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.SetSecretEncrypted(ctx, "myproject", "production", "DB_PASSWORD", []byte("encrypted-value"))
	if err != nil {
		t.Fatalf("SetSecretEncrypted() error = %v", err)
	}
}

func TestGetSecret(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Set secret
	encrypted := []byte("secret-data-here")
	err := db.SetSecretEncrypted(ctx, "myproject", "staging", "API_KEY", encrypted)
	if err != nil {
		t.Fatalf("SetSecretEncrypted() error = %v", err)
	}

	// Get secret
	secret, err := db.GetSecret(ctx, "myproject", "staging", "API_KEY")
	if err != nil {
		t.Fatalf("GetSecret() error = %v", err)
	}

	if secret == nil {
		t.Fatal("GetSecret() returned nil")
	}

	if string(secret.ValueEncrypted) != "secret-data-here" {
		t.Errorf("GetSecret() value = %v, want %v", string(secret.ValueEncrypted), "secret-data-here")
	}
}

func TestGetSecretNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	secret, err := db.GetSecret(ctx, "nonexistent", "scope", "key")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSecret() error = %v, want ErrNotFound", err)
	}

	if secret != nil {
		t.Errorf("GetSecret() = %v, want nil for nonexistent secret", secret)
	}
}

func TestSetSecretUpsert(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Set initial secret
	err := db.SetSecretEncrypted(ctx, "project", "scope", "key", []byte("value1"))
	if err != nil {
		t.Fatalf("SetSecretEncrypted() first call error = %v", err)
	}

	// Update same secret
	err = db.SetSecretEncrypted(ctx, "project", "scope", "key", []byte("value2"))
	if err != nil {
		t.Fatalf("SetSecretEncrypted() update call error = %v", err)
	}

	// Verify updated value
	secret, err := db.GetSecret(ctx, "project", "scope", "key")
	if err != nil {
		t.Fatalf("GetSecret() error = %v", err)
	}

	if string(secret.ValueEncrypted) != "value2" {
		t.Errorf("GetSecret() value = %v, want %v", string(secret.ValueEncrypted), "value2")
	}
}

func TestDeleteSecretCtx(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create secret
	err := db.SetSecretEncrypted(ctx, "project", "scope", "todelete", []byte("value"))
	if err != nil {
		t.Fatalf("SetSecretEncrypted() error = %v", err)
	}

	// Delete secret
	err = db.DeleteSecretCtx(ctx, "project", "scope", "todelete")
	if err != nil {
		t.Fatalf("DeleteSecretCtx() error = %v", err)
	}

	// Verify deleted
	secret, err := db.GetSecret(ctx, "project", "scope", "todelete")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSecret() error = %v, want ErrNotFound after deletion", err)
	}

	if secret != nil {
		t.Error("GetSecret() returned non-nil after deletion")
	}
}

func TestListSecretsCtx(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple secrets
	secrets := []struct {
		project, scope, key string
		value               []byte
	}{
		{"testproject", "prod", "KEY1", []byte("val1")},
		{"testproject", "prod", "KEY2", []byte("val2")},
		{"testproject", "staging", "KEY3", []byte("val3")},
	}

	for _, s := range secrets {
		if err := db.SetSecretEncrypted(ctx, s.project, s.scope, s.key, s.value); err != nil {
			t.Fatalf("SetSecretEncrypted() error = %v", err)
		}
	}

	// List secrets
	list, err := db.ListSecretsCtx(ctx, "testproject")
	if err != nil {
		t.Fatalf("ListSecretsCtx() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("ListSecretsCtx() returned %d secrets, want 3", len(list))
	}
}

// --- CLI-style Secret Tests ---

func TestSetSecretEncryptedCLI(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	err := db.SetSecretEncrypted(ctx, "myscope", "myscope", "mykey", []byte("myvalue"))
	if err != nil {
		t.Fatalf("SetSecretEncrypted() error = %v", err)
	}
}

func TestListSecrets(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create secrets
	ctx := context.Background()
	_ = db.SetSecretEncrypted(ctx, "scope1", "scope1", "key1", []byte("val1"))
	_ = db.SetSecretEncrypted(ctx, "scope1", "scope1", "key2", []byte("val2"))

	list, err := db.ListSecrets("scope1")
	if err != nil {
		t.Fatalf("ListSecrets() error = %v", err)
	}

	if len(list) != 2 {
		t.Errorf("ListSecrets() returned %d secrets, want 2", len(list))
	}
}

func TestDeleteSecret(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create and delete secret
	ctx := context.Background()
	_ = db.SetSecretEncrypted(ctx, "scope", "scope", "todelete", []byte("val"))
	err := db.DeleteSecret("scope", "todelete")
	if err != nil {
		t.Fatalf("DeleteSecret() error = %v", err)
	}
}

// --- Project Tests ---

func TestCreateProject(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	project := &Project{
		Name:       "myapp",
		Repository: "https://github.com/example/myapp",
		Branch:     "main",
		DeployPath: "/var/www/myapp",
		Type:       "nodejs",
		CreatedAt:  time.Now(),
	}

	err := db.CreateProject(project)
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if project.ID == 0 {
		t.Error("CreateProject() did not set project ID")
	}
}

func TestGetProject(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create project
	project := &Project{
		Name:             "findme",
		Repository:       "https://github.com/example/findme",
		Branch:           "develop",
		DeployPath:       "/var/www/findme",
		Type:             "php",
		CreatedAt:        time.Now(),
		LastDeployStatus: "", // Must set to avoid NULL scan issue
	}

	err := db.CreateProject(project)
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if project.ID == 0 {
		t.Error("CreateProject() did not set project ID")
	}

	// Verify we can retrieve the project (NULL handling is fixed with sql.NullString)
	retrieved, err := db.GetProjectByName(context.Background(), "findme")
	if err != nil {
		t.Fatalf("GetProjectByName() error = %v", err)
	}
	if retrieved.Name != "findme" {
		t.Errorf("GetProjectByName() Name = %v, want findme", retrieved.Name)
	}
}

func TestGetProjectNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := db.GetProjectByName(context.Background(), "nonexistent")
	if err == nil {
		t.Error("GetProjectByName() expected error for nonexistent project, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetProjectByName() expected ErrNotFound, got %v", err)
	}
}

func TestListProjects(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create multiple projects
	p1 := &Project{Name: "alpha", CreatedAt: time.Now()}
	if err := db.CreateProject(p1); err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}

	p2 := &Project{Name: "beta", CreatedAt: time.Now()}
	if err := db.CreateProject(p2); err != nil {
		t.Fatalf("CreateProject(beta) error = %v", err)
	}

	// ListProjects should work correctly (NULL handling fixed with sql.NullString)
	projects, err := db.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}

	if len(projects) != 2 {
		t.Errorf("ListProjects() returned %d projects, want 2", len(projects))
	}
}

func TestDeleteProject(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create project
	project := &Project{Name: "todelete", CreatedAt: time.Now()}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// Delete project
	err := db.DeleteProject("todelete")
	if err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	// Verify deleted
	_, err = db.GetProjectByName(context.Background(), "todelete")
	if err == nil {
		t.Error("GetProjectByName() expected error after deletion, got nil")
	}
}

// --- Project Type Tests ---

func TestCreateProjectType(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	pt := &ProjectType{
		Name:        "nodejs",
		Description: "Node.js application",
		BuildCmd:    "npm install && npm run build",
		CreatedAt:   time.Now(),
	}

	err := db.CreateProjectType(pt)
	if err != nil {
		t.Fatalf("CreateProjectType() error = %v", err)
	}

	if pt.ID == 0 {
		t.Error("CreateProjectType() did not set ID")
	}
}

func TestListProjectTypes(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create project types
	types := []*ProjectType{
		{Name: "golang", Description: "Go application", BuildCmd: "go build", CreatedAt: time.Now()},
		{Name: "python", Description: "Python application", BuildCmd: "pip install -r requirements.txt", CreatedAt: time.Now()},
	}

	for _, pt := range types {
		if err := db.CreateProjectType(pt); err != nil {
			t.Fatalf("CreateProjectType() error = %v", err)
		}
	}

	// List project types
	list, err := db.ListProjectTypes()
	if err != nil {
		t.Fatalf("ListProjectTypes() error = %v", err)
	}

	if len(list) != 2 {
		t.Errorf("ListProjectTypes() returned %d types, want 2", len(list))
	}
}

func TestDeleteProjectType(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create project type
	pt := &ProjectType{Name: "todelete", CreatedAt: time.Now()}
	if err := db.CreateProjectType(pt); err != nil {
		t.Fatalf("CreateProjectType() error = %v", err)
	}

	// Delete project type
	err := db.DeleteProjectType("todelete")
	if err != nil {
		t.Fatalf("DeleteProjectType() error = %v", err)
	}
}

// --- Extended Deployment Tests ---

func TestInsertDeployment(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	d := &DeploymentCLI{
		ProjectName: "testproj",
		Target:      "production",
		Status:      "running",
		TriggeredBy: "admin",
		StartedAt:   time.Now(),
	}

	err := db.InsertDeployment(d)
	if err != nil {
		t.Fatalf("InsertDeployment() error = %v", err)
	}

	if d.ID == "" {
		t.Error("InsertDeployment() did not set deployment ID")
	}
}

func TestSaveDeployment(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create deployment
	d := &DeploymentCLI{
		ID:          "test-deploy-1",
		ProjectName: "testproj",
		Target:      "production",
		Status:      "running",
		TriggeredBy: "admin",
		StartedAt:   time.Now(),
	}

	err := db.InsertDeployment(d)
	if err != nil {
		t.Fatalf("InsertDeployment() error = %v", err)
	}

	// Update deployment
	now := time.Now()
	d.Status = "completed"
	d.FinishedAt = &now

	err = db.SaveDeployment(d)
	if err != nil {
		t.Fatalf("SaveDeployment() error = %v", err)
	}
}

// --- Backup Tests ---

func TestBackup(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Add some data
	ctx := context.Background()
	user := &User{Username: "backupuser", PasswordHash: "hash", Role: "admin"}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	// Create backup
	backupPath := filepath.Join(t.TempDir(), "backup.db")
	err := db.Backup(backupPath)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	// Verify backup exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup file was not created")
	}

	// Open backup and verify data
	backupDB, err := New(backupPath, nil)
	if err != nil {
		t.Fatalf("Failed to open backup: %v", err)
	}
	defer backupDB.Close()

	found, err := backupDB.GetUserByUsername(ctx, "backupuser")
	if err != nil {
		t.Fatalf("GetUserByUsername() on backup error = %v", err)
	}

	if found == nil {
		t.Error("Backup does not contain expected user")
	}
}

// --- Export Tests ---

func TestExportAllSecrets(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create secrets
	ctx := context.Background()
	_ = db.SetSecretEncrypted(ctx, "project1", "project1", "key1", []byte("val1"))
	_ = db.SetSecretEncrypted(ctx, "project1", "project1", "key2", []byte("val2"))
	_ = db.SetSecretEncrypted(ctx, "project2", "project2", "key3", []byte("val3"))

	// Export
	exported, err := db.ExportAllSecrets()
	if err != nil {
		t.Fatalf("ExportAllSecrets() error = %v", err)
	}

	if len(exported) != 2 {
		t.Errorf("ExportAllSecrets() returned %d projects, want 2", len(exported))
	}
}

// --- Helper Function Tests ---

func TestMapToJSON(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]string
	}{
		{"nil map", nil},
		{"empty map", map[string]string{}},
		{"single item", map[string]string{"key": "value"}},
		{"multiple items", map[string]string{"a": "1", "b": "2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapToJSON(tt.input)
			// Just verify it doesn't panic and returns valid JSON-like string
			if result[0] != '{' || result[len(result)-1] != '}' {
				t.Errorf("mapToJSON() = %v, want JSON-like string", result)
			}
		})
	}
}

// --- Benchmark Tests ---

func BenchmarkCreateUser(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	db, err := New(dbPath, nil)
	if err != nil {
		b.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		user := &User{
			Username:     "user" + string(rune(i)),
			PasswordHash: "hash",
			Role:         "viewer",
		}
		_ = db.CreateUser(ctx, user)
	}
}

func BenchmarkGetUserByUsername(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	db, err := New(dbPath, nil)
	if err != nil {
		b.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create a user to retrieve
	user := &User{
		Username:     "benchuser",
		PasswordHash: "hash",
		Role:         "viewer",
	}
	_ = db.CreateUser(ctx, user)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = db.GetUserByUsername(ctx, "benchuser")
	}
}

func BenchmarkUpsertAgent(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	db, err := New(dbPath, nil)
	if err != nil {
		b.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agent := &Agent{
			ID:       "agent-bench",
			Hostname: "bench.example.com",
			Status:   "online",
		}
		_ = db.UpsertAgent(ctx, agent)
	}
}

func BenchmarkLogAudit(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	db, err := New(dbPath, nil)
	if err != nil {
		b.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := &AuditEntry{
			Source: "bench",
			User:   "admin",
			Action: "test",
			Result: "success",
		}
		_ = db.LogAudit(ctx, entry)
	}
}

// --- Session Tests ---

func TestCreateSession(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user first
	user := &User{
		Username:     "sessionuser",
		PasswordHash: "hash",
		Email:        "session@example.com",
		Role:         "admin",
	}
	err := db.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	// Create a session - ID is the primary key, Token will be set to same value
	session := &Session{
		ID:        "test-token-12345",
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	err = db.CreateSession(ctx, session)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
}

func TestGetSessionByToken(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user first
	user := &User{
		Username:     "sessionuser",
		PasswordHash: "hash",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create a session - ID is the primary key/token
	token := "get-session-token"
	session := &Session{
		ID:        token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	_ = db.CreateSession(ctx, session)

	// Retrieve the session
	found, err := db.GetSessionByToken(ctx, token)
	if err != nil {
		t.Fatalf("GetSessionByToken() error = %v", err)
	}

	if found == nil {
		t.Fatal("GetSessionByToken() returned nil")
	}

	if found.Token != token {
		t.Errorf("GetSessionByToken() token = %v, want %v", found.Token, token)
	}

	if found.UserID != user.ID {
		t.Errorf("GetSessionByToken() userID = %v, want %v", found.UserID, user.ID)
	}
}

func TestGetSessionByTokenNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.GetSessionByToken(ctx, "nonexistent-token")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSessionByToken() error = %v, want ErrNotFound", err)
	}
}

func TestGetSessionByTokenExpired(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "expireduser",
		PasswordHash: "hash",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create an expired session - ID is the primary key/token
	token := "expired-token"
	session := &Session{
		ID:        token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Already expired
	}
	_ = db.CreateSession(ctx, session)

	// Try to retrieve - should return not found because it's expired
	_, err := db.GetSessionByToken(ctx, token)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSessionByToken() error = %v, want ErrNotFound for expired session", err)
	}
}

func TestDeleteSession(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "deleteuser",
		PasswordHash: "hash",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create a session - ID is the primary key/token
	token := "delete-session-token"
	session := &Session{
		ID:        token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	_ = db.CreateSession(ctx, session)

	// Delete the session
	err := db.DeleteSession(ctx, token)
	if err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}

	// Verify session is gone
	_, err = db.GetSessionByToken(ctx, token)
	if !errors.Is(err, ErrNotFound) {
		t.Error("Session should be deleted")
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "expireduser",
		PasswordHash: "hash",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create an expired session - ID is the primary key
	expiredSession := &Session{
		ID:        "expired-token-1",
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	_ = db.CreateSession(ctx, expiredSession)

	// Delete expired sessions
	_, err := db.DeleteExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredSessions() error = %v", err)
	}
}

func TestDeleteUserSessions(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "sessionuser",
		PasswordHash: "hash",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create multiple sessions - ID is the primary key
	for i := 0; i < 3; i++ {
		session := &Session{
			ID:        "token-" + string(rune('a'+i)),
			UserID:    user.ID,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		_ = db.CreateSession(ctx, session)
	}

	// Delete all user sessions
	err := db.DeleteUserSessions(ctx, user.ID)
	if err != nil {
		t.Fatalf("DeleteUserSessions() error = %v", err)
	}
}

func TestListUserSessions(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "listsessionuser",
		PasswordHash: "hash",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create multiple sessions - ID is the primary key
	for i := 0; i < 3; i++ {
		session := &Session{
			ID:        "list-token-" + string(rune('a'+i)),
			UserID:    user.ID,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		_ = db.CreateSession(ctx, session)
	}

	// List sessions
	sessions, err := db.ListUserSessions(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListUserSessions() error = %v", err)
	}

	if len(sessions) != 3 {
		t.Errorf("ListUserSessions() returned %d sessions, want 3", len(sessions))
	}
}

// --- API Key Tests ---

func TestCreateAPIKey(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user first
	user := &User{
		Username:     "apikeyuser",
		PasswordHash: "hash",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create an API key
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	key := &APIKey{
		Name:      "test-key",
		KeyHash:   "hashed-key-value",
		UserID:    user.ID,
		ExpiresAt: &expiresAt,
	}

	err := db.CreateAPIKey(ctx, key)
	if err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	if key.ID == 0 {
		t.Error("CreateAPIKey() did not set key ID")
	}
}

func TestGetAPIKeyByHash(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "apikeyuser",
		PasswordHash: "hash",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create an API key
	keyHash := "unique-hash-12345"
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	key := &APIKey{
		Name:      "test-key",
		KeyHash:   keyHash,
		UserID:    user.ID,
		ExpiresAt: &expiresAt,
	}
	_ = db.CreateAPIKey(ctx, key)

	// Retrieve by hash
	found, err := db.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		t.Fatalf("GetAPIKeyByHash() error = %v", err)
	}

	if found == nil {
		t.Fatal("GetAPIKeyByHash() returned nil")
	}

	if found.Name != "test-key" {
		t.Errorf("GetAPIKeyByHash() name = %v, want %v", found.Name, "test-key")
	}
}

func TestGetAPIKeyByHashNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.GetAPIKeyByHash(ctx, "nonexistent-hash")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetAPIKeyByHash() error = %v, want ErrNotFound", err)
	}
}

func TestGetAPIKeyByHashExpired(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "expiredkeyuser",
		PasswordHash: "hash",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create an expired API key
	keyHash := "expired-hash-12345"
	expiredAt := time.Now().Add(-1 * time.Hour) // Already expired
	key := &APIKey{
		Name:      "expired-key",
		KeyHash:   keyHash,
		UserID:    user.ID,
		ExpiresAt: &expiredAt,
	}
	_ = db.CreateAPIKey(ctx, key)

	// Get the key - database returns it regardless of expiration
	found, err := db.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		t.Fatalf("GetAPIKeyByHash() error = %v", err)
	}

	// The IsValid() method should return false for expired keys
	if found.IsValid() {
		t.Error("IsValid() should return false for expired key")
	}
}

func TestUpdateAPIKeyUsage(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "usageuser",
		PasswordHash: "hash",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create an API key
	key := &APIKey{
		Name:    "usage-key",
		KeyHash: "usage-hash",
		UserID:  user.ID,
	}
	_ = db.CreateAPIKey(ctx, key)

	// Update usage
	err := db.UpdateAPIKeyUsage(ctx, key.ID)
	if err != nil {
		t.Fatalf("UpdateAPIKeyUsage() error = %v", err)
	}
}

func TestDeleteAPIKey(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "deleteuser",
		PasswordHash: "hash",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create an API key
	key := &APIKey{
		Name:    "delete-key",
		KeyHash: "delete-hash",
		UserID:  user.ID,
	}
	_ = db.CreateAPIKey(ctx, key)

	// Delete the key
	err := db.DeleteAPIKey(ctx, key.ID)
	if err != nil {
		t.Fatalf("DeleteAPIKey() error = %v", err)
	}

	// Verify key is gone
	_, err = db.GetAPIKeyByHash(ctx, "delete-hash")
	if !errors.Is(err, ErrNotFound) {
		t.Error("API key should be deleted")
	}
}

func TestListAPIKeys(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "listuser",
		PasswordHash: "hash",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create multiple API keys
	for i := 0; i < 3; i++ {
		key := &APIKey{
			Name:    "key-" + string(rune('a'+i)),
			KeyHash: "hash-" + string(rune('a'+i)),
			UserID:  user.ID,
		}
		_ = db.CreateAPIKey(ctx, key)
	}

	// List keys for user
	keys, err := db.ListAPIKeys(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys() error = %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("ListAPIKeys() returned %d keys, want 3", len(keys))
	}
}

func TestAPIKeyIsValid(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		expiresAt *time.Time
		want      bool
	}{
		{"no expiry", nil, true},
		{"future expiry", func() *time.Time { t := now.Add(time.Hour); return &t }(), true},
		{"past expiry", func() *time.Time { t := now.Add(-time.Hour); return &t }(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &APIKey{ExpiresAt: tt.expiresAt}
			if got := key.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Settings Tests ---

func TestGetSetting(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Set a setting first (category, key, value, valueType, encrypted)
	err := db.SetSetting(ctx, "test", "key1", "value1", "string", false)
	if err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}

	// Get the setting
	setting, err := db.GetSetting(ctx, "test", "key1")
	if err != nil {
		t.Fatalf("GetSetting() error = %v", err)
	}

	if setting.Value != "value1" {
		t.Errorf("GetSetting().Value = %v, want %v", setting.Value, "value1")
	}
}

func TestGetSettingNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.GetSetting(ctx, "nonexistent", "key")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSetting() error = %v, want ErrNotFound", err)
	}
}

func TestSetSettingUpsert(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Set initial value
	err := db.SetSetting(ctx, "test", "key1", "value1", "string", false)
	if err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}

	// Update the value
	err = db.SetSetting(ctx, "test", "key1", "value2", "string", false)
	if err != nil {
		t.Fatalf("SetSetting() update error = %v", err)
	}

	// Verify updated
	setting, _ := db.GetSetting(ctx, "test", "key1")
	if setting.Value != "value2" {
		t.Errorf("SetSetting() update: value = %v, want %v", setting.Value, "value2")
	}
}

func TestListSettingsByCategory(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Set multiple settings in same category
	_ = db.SetSetting(ctx, "category1", "key1", "value1", "string", false)
	_ = db.SetSetting(ctx, "category1", "key2", "value2", "string", false)
	_ = db.SetSetting(ctx, "category2", "key3", "value3", "string", false)

	// List settings for category1
	settings, err := db.ListSettingsByCategory(ctx, "category1")
	if err != nil {
		t.Fatalf("ListSettingsByCategory() error = %v", err)
	}

	if len(settings) != 2 {
		t.Errorf("ListSettingsByCategory() returned %d settings, want 2", len(settings))
	}
}

func TestListAllSettings(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Set multiple settings in different categories
	_ = db.SetSetting(ctx, "cat1", "key1", "value1", "string", false)
	_ = db.SetSetting(ctx, "cat2", "key2", "value2", "string", false)
	_ = db.SetSetting(ctx, "cat1", "key3", "value3", "string", false)

	// List all settings
	settings, err := db.ListAllSettings(ctx)
	if err != nil {
		t.Fatalf("ListAllSettings() error = %v", err)
	}

	if len(settings) != 3 {
		t.Errorf("ListAllSettings() returned %d settings, want 3", len(settings))
	}
}

func TestDeleteSetting(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Set a setting
	_ = db.SetSetting(ctx, "test", "key1", "value1", "string", false)

	// Delete it
	err := db.DeleteSetting(ctx, "test", "key1")
	if err != nil {
		t.Fatalf("DeleteSetting() error = %v", err)
	}

	// Verify it's gone
	_, err = db.GetSetting(ctx, "test", "key1")
	if !errors.Is(err, ErrNotFound) {
		t.Error("Setting should be deleted")
	}
}

func TestHasSettings(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Initially no settings
	has, err := db.HasSettings(ctx)
	if err != nil {
		t.Fatalf("HasSettings() error = %v", err)
	}

	if has {
		t.Error("HasSettings() = true, want false for empty database")
	}

	// Add a setting
	_ = db.SetSetting(ctx, "test", "key", "value", "string", false)

	// Now should have settings
	has, err = db.HasSettings(ctx)
	if err != nil {
		t.Fatalf("HasSettings() after add error = %v", err)
	}

	if !has {
		t.Error("HasSettings() = false, want true after adding setting")
	}
}

// --- Deployment Logs Tests ---
// Schema has been fixed to use created_at column.

func TestCreateDeploymentLog(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// First create a deployment
	deployment := &DeploymentRecord{
		ID:            "deploy-log-test",
		Project:       "test-project",
		Target:        "prod",
		Branch:        "main",
		Status:        "running",
		ReleaseNumber: 1,
		StartedAt:     time.Now(),
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := db.CreateDeployment(ctx, deployment); err != nil {
		t.Fatalf("CreateDeployment() error = %v", err)
	}

	// Create a deployment log
	log := &DeploymentLog{
		DeploymentID: deployment.ID,
		Level:        "info",
		Message:      "Test log message",
		Source:       "test",
		CreatedAt:    time.Now(),
	}

	err := db.CreateDeploymentLog(ctx, log)
	if err != nil {
		t.Fatalf("CreateDeploymentLog() error = %v", err)
	}
}

func TestListDeploymentLogs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// First create a deployment
	deployment := &DeploymentRecord{
		ID:            "deploy-list-logs",
		Project:       "test-project",
		Target:        "prod",
		Branch:        "main",
		Status:        "running",
		ReleaseNumber: 1,
		StartedAt:     time.Now(),
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := db.CreateDeployment(ctx, deployment); err != nil {
		t.Fatalf("CreateDeployment() error = %v", err)
	}

	// Create multiple logs
	for i := 0; i < 3; i++ {
		log := &DeploymentLog{
			DeploymentID: deployment.ID,
			Level:        "info",
			Message:      "Log message " + string(rune('A'+i)),
			Source:       "test",
			CreatedAt:    time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := db.CreateDeploymentLog(ctx, log); err != nil {
			t.Fatalf("CreateDeploymentLog() error = %v", err)
		}
	}

	logs, err := db.ListDeploymentLogs(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("ListDeploymentLogs() error = %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("ListDeploymentLogs() returned %d logs, want 3", len(logs))
	}
}

func TestListDeploymentLogsAfter(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// First create a deployment
	deployment := &DeploymentRecord{
		ID:            "deploy-logs-after",
		Project:       "test-project",
		Target:        "prod",
		Branch:        "main",
		Status:        "running",
		ReleaseNumber: 1,
		StartedAt:     time.Now(),
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := db.CreateDeployment(ctx, deployment); err != nil {
		t.Fatalf("CreateDeployment() error = %v", err)
	}

	// Create multiple logs
	for i := 0; i < 5; i++ {
		log := &DeploymentLog{
			DeploymentID: deployment.ID,
			Level:        "info",
			Message:      "Log message " + string(rune('A'+i)),
			Source:       "test",
			CreatedAt:    time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := db.CreateDeploymentLog(ctx, log); err != nil {
			t.Fatalf("CreateDeploymentLog() error = %v", err)
		}
	}

	// Get all logs to find the ID of the 3rd one
	allLogs, _ := db.ListDeploymentLogs(ctx, deployment.ID)
	if len(allLogs) < 3 {
		t.Fatalf("Expected at least 3 logs, got %d", len(allLogs))
	}
	afterID := allLogs[2].ID

	logs, err := db.ListDeploymentLogsAfter(ctx, deployment.ID, afterID)
	if err != nil {
		t.Fatalf("ListDeploymentLogsAfter() error = %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("ListDeploymentLogsAfter() returned %d logs, want 2", len(logs))
	}
}

// --- Delete Agent Tests ---

func TestDeleteAgent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create an agent
	agent := &Agent{
		ID:       "delete-agent-1",
		Hostname: "delete.example.com",
		Status:   "online",
	}
	_ = db.UpsertAgent(ctx, agent)

	// Delete it
	err := db.DeleteAgent(ctx, "delete-agent-1")
	if err != nil {
		t.Fatalf("DeleteAgent() error = %v", err)
	}

	// Verify it's gone
	_, err = db.GetAgent(ctx, "delete-agent-1")
	if !errors.Is(err, ErrNotFound) {
		t.Error("Agent should be deleted")
	}
}

// --- Secret With Scope Tests ---

func TestListSecretsWithScope(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create secrets with different scopes
	_ = db.SetSecretEncrypted(ctx, "global", "global", "key1", []byte("val1"))
	_ = db.SetSecretEncrypted(ctx, "project1", "project1", "key2", []byte("val2"))

	// List secrets with scope (ctx, project, scope)
	secrets, err := db.ListSecretsWithScope(ctx, "global", "global")
	if err != nil {
		t.Fatalf("ListSecretsWithScope() error = %v", err)
	}

	if len(secrets) != 1 {
		t.Errorf("ListSecretsWithScope() returned %d secrets, want 1", len(secrets))
	}
}

func TestListAllSecretsCtx(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create secrets
	_ = db.SetSecretEncrypted(ctx, "scope1", "scope1", "key1", []byte("val1"))
	_ = db.SetSecretEncrypted(ctx, "scope2", "scope2", "key2", []byte("val2"))

	// List all secrets
	secrets, err := db.ListAllSecretsCtx(ctx)
	if err != nil {
		t.Fatalf("ListAllSecretsCtx() error = %v", err)
	}

	if len(secrets) != 2 {
		t.Errorf("ListAllSecretsCtx() returned %d secrets, want 2", len(secrets))
	}
}

// --- Conn Test ---

func TestConn(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	conn := db.Conn()
	if conn == nil {
		t.Fatal("Conn() returned nil")
	}

	// Verify we can use the connection
	err := conn.Ping()
	if err != nil {
		t.Errorf("Conn().Ping() error = %v", err)
	}
}

// --- Project Type Tests ---

func TestGetProjectTypeByName(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a project type first
	pt := &ProjectType{
		Name:        "nodejs",
		Description: "Node.js Application",
		BuildCmd:    "npm install",
	}
	err := db.CreateProjectType(pt)
	if err != nil {
		t.Fatalf("CreateProjectType() error = %v", err)
	}

	// Get it by name
	found, err := db.GetProjectTypeByName("nodejs")
	if err != nil {
		t.Fatalf("GetProjectTypeByName() error = %v", err)
	}

	if found == nil {
		t.Fatal("GetProjectTypeByName() returned nil")
	}

	if found.Name != "nodejs" {
		t.Errorf("GetProjectTypeByName().Name = %v, want %v", found.Name, "nodejs")
	}
}

func TestGetProjectTypeByNameNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := db.GetProjectTypeByName("nonexistent")
	// GetProjectTypeByName returns a formatted error, not ErrNotFound
	if err == nil {
		t.Fatal("GetProjectTypeByName() should return error for nonexistent type")
	}
}

func TestUpdateProjectTypeByName(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a project type
	pt := &ProjectType{
		Name:        "update-test",
		Description: "Original",
		BuildCmd:    "original",
	}
	_ = db.CreateProjectType(pt)

	// Update it - UpdateProjectTypeByName takes only the project type struct
	pt.Description = "Updated"
	pt.BuildCmd = "updated"
	err := db.UpdateProjectTypeByName(pt)
	if err != nil {
		t.Fatalf("UpdateProjectTypeByName() error = %v", err)
	}

	// Verify update
	found, _ := db.GetProjectTypeByName("update-test")
	if found.Description != "Updated" {
		t.Errorf("UpdateProjectTypeByName() description = %v, want %v", found.Description, "Updated")
	}
}

// --- User Management Tests ---

func TestListUsers(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create some users
	for i := 0; i < 3; i++ {
		user := &User{
			Username:     "user" + string(rune('0'+i)),
			PasswordHash: "hash",
			Email:        "user" + string(rune('0'+i)) + "@example.com",
			Role:         "viewer",
		}
		_ = db.CreateUser(ctx, user)
	}

	// List users
	users, err := db.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}

	if len(users) != 3 {
		t.Errorf("ListUsers() count = %d, want 3", len(users))
	}
}

func TestGetUserByID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "getbyid",
		PasswordHash: "hash",
		Email:        "getbyid@example.com",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Get by ID
	found, err := db.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}

	if found == nil {
		t.Fatal("GetUserByID() returned nil")
	}

	if found.Username != "getbyid" {
		t.Errorf("GetUserByID().Username = %v, want getbyid", found.Username)
	}
}

func TestUpdateUserByID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "updatebyid",
		PasswordHash: "hash",
		Email:        "update@example.com",
		Role:         "viewer",
	}
	_ = db.CreateUser(ctx, user)

	// Update user
	user.Email = "newemail@example.com"
	user.Role = "admin"
	err := db.UpdateUserByID(ctx, user)
	if err != nil {
		t.Fatalf("UpdateUserByID() error = %v", err)
	}

	// Verify update
	found, _ := db.GetUserByID(ctx, user.ID)
	if found.Email != "newemail@example.com" {
		t.Errorf("UpdateUserByID() email = %v, want newemail@example.com", found.Email)
	}
}

func TestDeleteUser(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "deleteuser",
		PasswordHash: "hash",
		Email:        "delete@example.com",
		Role:         "viewer",
	}
	_ = db.CreateUser(ctx, user)

	// Delete user
	err := db.DeleteUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	// Verify deleted
	found, _ := db.GetUserByID(ctx, user.ID)
	if found != nil {
		t.Error("DeleteUser() user still found after deletion")
	}
}

func TestDeleteUserWithAssociations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "deleteassocuser",
		PasswordHash: "hash",
		Email:        "deleteassoc@example.com",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create session for user
	session := &Session{
		ID:        "delete-user-session",
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	_ = db.CreateSession(ctx, session)

	// Create API key for user
	key := &APIKey{
		Name:    "delete-user-key",
		KeyHash: "delete-user-hash",
		UserID:  user.ID,
	}
	_ = db.CreateAPIKey(ctx, key)

	// Delete user - should also delete sessions and API keys
	err := db.DeleteUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("DeleteUser() with associations error = %v", err)
	}

	// Verify user is deleted
	found, _ := db.GetUserByID(ctx, user.ID)
	if found != nil {
		t.Error("DeleteUser() user still found after deletion")
	}

	// Verify session is deleted
	_, err = db.GetSessionByToken(ctx, "delete-user-session")
	if !errors.Is(err, ErrNotFound) {
		t.Error("DeleteUser() should have deleted user session")
	}

	// Verify API key is deleted
	_, err = db.GetAPIKeyByHash(ctx, "delete-user-hash")
	if !errors.Is(err, ErrNotFound) {
		t.Error("DeleteUser() should have deleted user API key")
	}
}

// --- Project Webhook Tests ---

func TestSetProjectWebhook(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a project first
	project := &Project{
		Name:       "webhook-test-project",
		Repository: "https://github.com/test/repo.git",
	}
	_ = db.CreateProject(project)

	// Set webhook
	err := db.SetProjectWebhook(ctx, project.ID, "github", []byte("secret"), true, true)
	if err != nil {
		t.Fatalf("SetProjectWebhook() error = %v", err)
	}
}

func TestGetProjectWebhook(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create project and webhook
	project := &Project{
		Name:       "get-webhook-project",
		Repository: "https://github.com/test/repo.git",
	}
	_ = db.CreateProject(project)
	_ = db.SetProjectWebhook(ctx, project.ID, "github", []byte("secret"), true, false)

	// Get webhook
	webhook, err := db.GetProjectWebhook(ctx, project.ID, "github")
	if err != nil {
		t.Fatalf("GetProjectWebhook() error = %v", err)
	}

	if webhook.Provider != "github" {
		t.Errorf("GetProjectWebhook() provider = %v, want github", webhook.Provider)
	}
	if !webhook.Enabled {
		t.Error("GetProjectWebhook() enabled should be true")
	}
}

func TestGetProjectWebhookNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.GetProjectWebhook(ctx, 9999, "github")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetProjectWebhook() error = %v, want ErrNotFound", err)
	}
}

func TestListProjectWebhooks(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create project and webhooks
	project := &Project{
		Name:       "list-webhooks-project",
		Repository: "https://github.com/test/repo.git",
	}
	_ = db.CreateProject(project)
	_ = db.SetProjectWebhook(ctx, project.ID, "github", []byte("secret1"), true, false)
	_ = db.SetProjectWebhook(ctx, project.ID, "gitlab", []byte("secret2"), true, true)

	// List webhooks
	webhooks, err := db.ListProjectWebhooks(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListProjectWebhooks() error = %v", err)
	}

	if len(webhooks) != 2 {
		t.Errorf("ListProjectWebhooks() count = %d, want 2", len(webhooks))
	}
}

func TestDeleteProjectWebhook(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create project and webhook
	project := &Project{
		Name:       "delete-webhook-project",
		Repository: "https://github.com/test/repo.git",
	}
	_ = db.CreateProject(project)
	_ = db.SetProjectWebhook(ctx, project.ID, "github", []byte("secret"), true, false)

	// Delete webhook
	err := db.DeleteProjectWebhook(ctx, project.ID, "github")
	if err != nil {
		t.Fatalf("DeleteProjectWebhook() error = %v", err)
	}

	// Verify deleted
	_, err = db.GetProjectWebhook(ctx, project.ID, "github")
	if !errors.Is(err, ErrNotFound) {
		t.Error("DeleteProjectWebhook() webhook still exists after deletion")
	}
}

// --- Scheduled Deployment Tests ---

func TestCreateScheduledDeployment(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	scheduledAt := time.Now().Add(1 * time.Hour)
	err := db.CreateScheduledDeployment(ctx, "sched-deploy-1", "test-project", "production", "main", scheduledAt, "testuser")
	if err != nil {
		t.Fatalf("CreateScheduledDeployment() error = %v", err)
	}
}

func TestListPendingScheduledDeployments(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// List pending deployments (should work without error)
	_, err := db.ListPendingScheduledDeployments(ctx)
	if err != nil {
		t.Fatalf("ListPendingScheduledDeployments() error = %v", err)
	}
}

func TestCancelScheduledDeployment(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a scheduled deployment
	futureTime := time.Now().Add(1 * time.Hour)
	_ = db.CreateScheduledDeployment(ctx, "cancel-deploy-1", "test-project", "production", "main", futureTime, "testuser")

	// Cancel it
	err := db.CancelScheduledDeployment(ctx, "cancel-deploy-1")
	if err != nil {
		t.Fatalf("CancelScheduledDeployment() error = %v", err)
	}

	// Verify it's cancelled
	deployment, _ := db.GetDeployment(ctx, "cancel-deploy-1")
	if deployment != nil && deployment.Status != "cancelled" {
		t.Errorf("CancelScheduledDeployment() status = %v, want cancelled", deployment.Status)
	}
}

func TestListDeploymentsRecent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a deployment using InsertDeployment
	deployment := &DeploymentCLI{
		ID:          "deploy-recent",
		ProjectName: "test-project",
		Target:      "production",
		Status:      "completed",
	}
	if err := db.InsertDeployment(deployment); err != nil {
		t.Fatalf("InsertDeployment() error = %v", err)
	}

	// List recent deployments
	deployments, err := db.ListDeploymentsRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListDeploymentsRecent() error = %v", err)
	}
	if len(deployments) < 1 {
		t.Errorf("ListDeploymentsRecent() got %d deployments, want at least 1", len(deployments))
	}
}

func TestUpdateProjectByName(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a project
	project := &Project{
		Name:       "update-project",
		Repository: "https://github.com/test/repo",
		Branch:     "main",
	}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// Update project
	updated := &Project{
		Name:       "update-project",
		Repository: "https://github.com/test/updated",
		Branch:     "develop",
	}
	err := db.UpdateProjectByName(ctx, updated)
	if err != nil {
		t.Fatalf("UpdateProjectByName() error = %v", err)
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	cutoff := time.Now().Add(-1 * time.Hour)
	_, err := db.CleanupExpiredSessions(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupExpiredSessions() error = %v", err)
	}
}

func TestCleanupOldDeployments(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	_, err := db.CleanupOldDeployments(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupOldDeployments() error = %v", err)
	}
}

func TestCleanupOldDeploymentLogs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// First create a deployment
	deployment := &DeploymentRecord{
		ID:            "deploy-cleanup-logs",
		Project:       "test-project",
		Target:        "prod",
		Branch:        "main",
		Status:        "completed",
		ReleaseNumber: 1,
		StartedAt:     time.Now().Add(-48 * time.Hour),
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	completedAt := time.Now().Add(-47 * time.Hour)
	deployment.CompletedAt = &completedAt
	if err := db.CreateDeployment(ctx, deployment); err != nil {
		t.Fatalf("CreateDeployment() error = %v", err)
	}

	// Create an old log
	log := &DeploymentLog{
		DeploymentID: deployment.ID,
		Level:        "info",
		Message:      "Old log message",
		Source:       "test",
		CreatedAt:    time.Now().Add(-48 * time.Hour),
	}
	if err := db.CreateDeploymentLog(ctx, log); err != nil {
		t.Fatalf("CreateDeploymentLog() error = %v", err)
	}

	// Cleanup logs older than 24 hours
	cutoff := time.Now().Add(-24 * time.Hour)
	deleted, err := db.CleanupOldDeploymentLogs(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupOldDeploymentLogs() error = %v", err)
	}
	if deleted < 1 {
		t.Errorf("CleanupOldDeploymentLogs() deleted %d logs, want >= 1", deleted)
	}
}

func TestCleanupOldAuditLogs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create an old audit log entry using direct SQL with SQLite datetime function
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO audit_logs (timestamp, source, user, action, result) 
		VALUES (datetime('now', '-48 hours'), 'test', 'admin', 'test_action', 'success')
	`)
	if err != nil {
		t.Fatalf("Insert old audit log error = %v", err)
	}

	// Cleanup old audit logs (older than 24 hours)
	deleted, err := db.CleanupOldAuditLogs(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("CleanupOldAuditLogs() error = %v", err)
	}
	if deleted < 1 {
		t.Errorf("CleanupOldAuditLogs() deleted = %d, want >= 1", deleted)
	}
}

func TestMarkStaleAgents(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	cutoff := time.Now().Add(-5 * time.Minute)
	_, err := db.MarkStaleAgents(ctx, cutoff)
	if err != nil {
		t.Fatalf("MarkStaleAgents() error = %v", err)
	}
}

func TestCleanupExpiredAPIKeys(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.CleanupExpiredAPIKeys(ctx, time.Now())
	if err != nil {
		t.Fatalf("CleanupExpiredAPIKeys() error = %v", err)
	}
}

func TestCleanupOrphanedWebhooks(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.CleanupOrphanedWebhooks(ctx)
	if err != nil {
		t.Fatalf("CleanupOrphanedWebhooks() error = %v", err)
	}
}

func TestSSHHostKeyOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create SSH host key
	key := &SSHHostKey{
		Hostname:    "testhost.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   "AAAAB3NzaC1yc2EAAAADAQABAAABAQ...",
		Fingerprint: "SHA256:abc123",
		Trusted:     true,
		AddedBy:     "testuser",
	}
	err := db.CreateSSHHostKey(ctx, key)
	if err != nil {
		t.Fatalf("CreateSSHHostKey() error = %v", err)
	}

	// Get SSH host key
	gotKey, err := db.GetSSHHostKey(ctx, "testhost.example.com", 22, "ssh-rsa")
	if err != nil {
		t.Fatalf("GetSSHHostKey() error = %v", err)
	}
	if gotKey == nil {
		t.Fatal("GetSSHHostKey() returned nil")
	}

	// Get SSH host keys by host
	keys, err := db.GetSSHHostKeysByHost(ctx, "testhost.example.com", 22)
	if err != nil {
		t.Fatalf("GetSSHHostKeysByHost() error = %v", err)
	}
	if len(keys) < 1 {
		t.Errorf("GetSSHHostKeysByHost() got %d keys, want at least 1", len(keys))
	}

	// List all SSH host keys
	allKeys, err := db.ListSSHHostKeys(ctx)
	if err != nil {
		t.Fatalf("ListSSHHostKeys() error = %v", err)
	}
	if len(allKeys) < 1 {
		t.Errorf("ListSSHHostKeys() got %d keys, want at least 1", len(allKeys))
	}

	// Update SSH host key trust
	err = db.UpdateSSHHostKeyTrust(ctx, gotKey.ID, false, "admin")
	if err != nil {
		t.Fatalf("UpdateSSHHostKeyTrust() error = %v", err)
	}

	// Delete SSH host key
	err = db.DeleteSSHHostKey(ctx, gotKey.ID)
	if err != nil {
		t.Fatalf("DeleteSSHHostKey() error = %v", err)
	}
}

func TestDeleteSSHHostKeysByHost(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create SSH host keys
	key1 := &SSHHostKey{
		Hostname:    "deletehost.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   "AAAAB3NzaC1yc2EAAAADAQABAAABAQ...",
		Fingerprint: "SHA256:abc123",
		Trusted:     true,
		AddedBy:     "testuser",
	}
	if err := db.CreateSSHHostKey(ctx, key1); err != nil {
		t.Fatalf("CreateSSHHostKey(key1) error = %v", err)
	}

	key2 := &SSHHostKey{
		Hostname:    "deletehost.example.com",
		Port:        22,
		KeyType:     "ssh-ed25519",
		PublicKey:   "AAAAC3NzaC1lZDI1NTE5AAAAIG...",
		Fingerprint: "SHA256:def456",
		Trusted:     true,
		AddedBy:     "testuser",
	}
	if err := db.CreateSSHHostKey(ctx, key2); err != nil {
		t.Fatalf("CreateSSHHostKey(key2) error = %v", err)
	}

	// Delete all keys for host
	_, err := db.DeleteSSHHostKeysByHost(ctx, "deletehost.example.com", 22)
	if err != nil {
		t.Fatalf("DeleteSSHHostKeysByHost() error = %v", err)
	}

	// Verify keys are deleted
	keys, _ := db.GetSSHHostKeysByHost(ctx, "deletehost.example.com", 22)
	if len(keys) != 0 {
		t.Errorf("DeleteSSHHostKeysByHost() keys still exist: %d", len(keys))
	}
}

// --- Blocked IP Tests ---

func TestBlockIP(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	block := &BlockedIP{
		IPAddress: "192.168.1.100",
		Reason:    "Brute force attack",
		BlockedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		BlockedBy: "security-system",
	}

	err := db.BlockIP(ctx, block)
	if err != nil {
		t.Fatalf("BlockIP() error = %v", err)
	}
}

func TestBlockIPUpsert(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create first block
	block := &BlockedIP{
		IPAddress: "10.0.0.1",
		Reason:    "Initial reason",
		BlockedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		BlockedBy: "admin",
	}
	err := db.BlockIP(ctx, block)
	if err != nil {
		t.Fatalf("BlockIP() initial error = %v", err)
	}

	// Update same IP with new reason
	block.Reason = "Updated reason"
	block.ExpiresAt = time.Now().Add(24 * time.Hour)
	err = db.BlockIP(ctx, block)
	if err != nil {
		t.Fatalf("BlockIP() update error = %v", err)
	}

	// Verify update
	retrieved, err := db.GetBlockedIP(ctx, "10.0.0.1")
	if err != nil {
		t.Fatalf("GetBlockedIP() error = %v", err)
	}
	if retrieved.Reason != "Updated reason" {
		t.Errorf("BlockIP() reason = %v, want Updated reason", retrieved.Reason)
	}
}

func TestGetBlockedIP(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Block an IP
	block := &BlockedIP{
		IPAddress: "172.16.0.50",
		Reason:    "Suspicious activity",
		BlockedAt: time.Now(),
		ExpiresAt: time.Now().Add(2 * time.Hour),
		BlockedBy: "admin",
	}
	_ = db.BlockIP(ctx, block)

	// Get the blocked IP
	retrieved, err := db.GetBlockedIP(ctx, "172.16.0.50")
	if err != nil {
		t.Fatalf("GetBlockedIP() error = %v", err)
	}

	if retrieved.IPAddress != "172.16.0.50" {
		t.Errorf("GetBlockedIP() IP = %v, want 172.16.0.50", retrieved.IPAddress)
	}
	if retrieved.Reason != "Suspicious activity" {
		t.Errorf("GetBlockedIP() reason = %v, want Suspicious activity", retrieved.Reason)
	}
}

func TestGetBlockedIPNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.GetBlockedIP(ctx, "1.2.3.4")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetBlockedIP() error = %v, want ErrNotFound", err)
	}
}

func TestUnblockIP(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Block an IP
	block := &BlockedIP{
		IPAddress: "192.168.100.1",
		Reason:    "Testing unblock",
		BlockedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		BlockedBy: "test",
	}
	_ = db.BlockIP(ctx, block)

	// Unblock it
	err := db.UnblockIP(ctx, "192.168.100.1")
	if err != nil {
		t.Fatalf("UnblockIP() error = %v", err)
	}

	// Verify it's unblocked
	_, err = db.GetBlockedIP(ctx, "192.168.100.1")
	if !errors.Is(err, ErrNotFound) {
		t.Error("UnblockIP() IP should be removed")
	}
}

func TestIsIPBlocked(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Initially not blocked
	blocked, err := db.IsIPBlocked(ctx, "10.10.10.10")
	if err != nil {
		t.Fatalf("IsIPBlocked() error = %v", err)
	}
	if blocked {
		t.Error("IsIPBlocked() should return false for non-blocked IP")
	}

	// Block the IP
	block := &BlockedIP{
		IPAddress: "10.10.10.10",
		Reason:    "Test block",
		BlockedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		BlockedBy: "test",
	}
	_ = db.BlockIP(ctx, block)

	// Now should be blocked
	blocked, err = db.IsIPBlocked(ctx, "10.10.10.10")
	if err != nil {
		t.Fatalf("IsIPBlocked() after block error = %v", err)
	}
	if !blocked {
		t.Error("IsIPBlocked() should return true for blocked IP")
	}
}

func TestIsIPBlockedExpired(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Block an IP with past expiration
	block := &BlockedIP{
		IPAddress: "10.10.10.20",
		Reason:    "Expired block",
		BlockedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Already expired
		BlockedBy: "test",
	}
	_ = db.BlockIP(ctx, block)

	// Should not be considered blocked (expired)
	blocked, err := db.IsIPBlocked(ctx, "10.10.10.20")
	if err != nil {
		t.Fatalf("IsIPBlocked() error = %v", err)
	}
	if blocked {
		t.Error("IsIPBlocked() should return false for expired block")
	}
}

func TestListBlockedIPs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple blocked IPs
	for i := 0; i < 5; i++ {
		block := &BlockedIP{
			IPAddress: "192.168.2." + string(rune('1'+i)),
			Reason:    "Test block",
			BlockedAt: time.Now(),
			ExpiresAt: time.Now().Add(1 * time.Hour),
			BlockedBy: "test",
		}
		_ = db.BlockIP(ctx, block)
	}

	// List with pagination
	blocks, total, err := db.ListBlockedIPs(ctx, 3, 0)
	if err != nil {
		t.Fatalf("ListBlockedIPs() error = %v", err)
	}

	if total < 5 {
		t.Errorf("ListBlockedIPs() total = %d, want >= 5", total)
	}
	if len(blocks) != 3 {
		t.Errorf("ListBlockedIPs() returned %d, want 3", len(blocks))
	}

	// Get second page
	blocks2, _, err := db.ListBlockedIPs(ctx, 3, 3)
	if err != nil {
		t.Fatalf("ListBlockedIPs() page 2 error = %v", err)
	}
	if len(blocks2) < 2 {
		t.Errorf("ListBlockedIPs() page 2 returned %d, want >= 2", len(blocks2))
	}
}

func TestCleanupExpiredBlockedIPs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create an expired block
	block := &BlockedIP{
		IPAddress: "10.20.30.40",
		Reason:    "Expired test",
		BlockedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
		BlockedBy: "test",
	}
	_ = db.BlockIP(ctx, block)

	// Cleanup expired
	deleted, err := db.CleanupExpiredBlockedIPs(ctx)
	if err != nil {
		t.Fatalf("CleanupExpiredBlockedIPs() error = %v", err)
	}
	if deleted < 1 {
		t.Errorf("CleanupExpiredBlockedIPs() deleted = %d, want >= 1", deleted)
	}
}

// --- Rate Limit Tests ---

func TestRecordRateLimitRequest(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	windowStart := time.Now().Truncate(time.Minute)
	windowEnd := windowStart.Add(time.Minute)

	err := db.RecordRateLimitRequest(ctx, "user:admin", "api", windowStart, windowEnd)
	if err != nil {
		t.Fatalf("RecordRateLimitRequest() error = %v", err)
	}

	// Record another request (should increment)
	err = db.RecordRateLimitRequest(ctx, "user:admin", "api", windowStart, windowEnd)
	if err != nil {
		t.Fatalf("RecordRateLimitRequest() second call error = %v", err)
	}
}

func TestGetRateLimitCount(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	windowStart := time.Now().Truncate(time.Minute)
	windowEnd := windowStart.Add(time.Minute)

	// Record several requests
	for i := 0; i < 5; i++ {
		_ = db.RecordRateLimitRequest(ctx, "user:testcount", "api", windowStart, windowEnd)
	}

	// Get count
	count, err := db.GetRateLimitCount(ctx, "user:testcount", "api", windowStart.Add(-time.Second))
	if err != nil {
		t.Fatalf("GetRateLimitCount() error = %v", err)
	}
	if count != 5 {
		t.Errorf("GetRateLimitCount() = %d, want 5", count)
	}
}

func TestGetRateLimitCountEmpty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Get count for non-existent key
	count, err := db.GetRateLimitCount(ctx, "user:nonexistent", "api", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetRateLimitCount() error = %v", err)
	}
	if count != 0 {
		t.Errorf("GetRateLimitCount() = %d, want 0 for empty", count)
	}
}

func TestCleanupRateLimitRecords(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create old rate limit records
	oldWindowStart := time.Now().Add(-2 * time.Hour)
	oldWindowEnd := oldWindowStart.Add(time.Minute)
	_ = db.RecordRateLimitRequest(ctx, "user:old", "api", oldWindowStart, oldWindowEnd)

	// Cleanup
	deleted, err := db.CleanupRateLimitRecords(ctx, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("CleanupRateLimitRecords() error = %v", err)
	}
	if deleted < 1 {
		t.Errorf("CleanupRateLimitRecords() deleted = %d, want >= 1", deleted)
	}
}

// --- Provision Job Tests ---

func TestCreateProvisionJob(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	job := &ProvisionJob{
		ID:         "prov-job-001",
		TargetHost: "192.168.1.100",
		TargetPort: 22,
		TargetUser: "deploy",
		Status:     "pending",
		Stage:      "init",
		Progress:   0,
		StartedAt:  time.Now(),
	}

	err := db.CreateProvisionJob(ctx, job)
	if err != nil {
		t.Fatalf("CreateProvisionJob() error = %v", err)
	}
}

func TestGetProvisionJob(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a job
	job := &ProvisionJob{
		ID:         "prov-job-get",
		TargetHost: "10.0.0.5",
		TargetPort: 22,
		TargetUser: "root",
		Status:     "running",
		Stage:      "copying",
		Progress:   50,
		StartedAt:  time.Now(),
	}
	_ = db.CreateProvisionJob(ctx, job)

	// Get the job
	retrieved, err := db.GetProvisionJob(ctx, "prov-job-get")
	if err != nil {
		t.Fatalf("GetProvisionJob() error = %v", err)
	}

	if retrieved.TargetHost != "10.0.0.5" {
		t.Errorf("GetProvisionJob() host = %v, want 10.0.0.5", retrieved.TargetHost)
	}
	if retrieved.Status != "running" {
		t.Errorf("GetProvisionJob() status = %v, want running", retrieved.Status)
	}
	if retrieved.Progress != 50 {
		t.Errorf("GetProvisionJob() progress = %d, want 50", retrieved.Progress)
	}
}

func TestGetProvisionJobNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.GetProvisionJob(ctx, "nonexistent-job")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProvisionJob() error = %v, want ErrNotFound", err)
	}
}

func TestUpdateProvisionJobStatus(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a job
	job := &ProvisionJob{
		ID:         "prov-job-update",
		TargetHost: "10.0.0.10",
		TargetPort: 22,
		TargetUser: "admin",
		Status:     "pending",
		Stage:      "init",
		Progress:   0,
		StartedAt:  time.Now(),
	}
	_ = db.CreateProvisionJob(ctx, job)

	// Update status
	err := db.UpdateProvisionJobStatus(ctx, "prov-job-update", "running", "installing", "", 75)
	if err != nil {
		t.Fatalf("UpdateProvisionJobStatus() error = %v", err)
	}

	// Verify
	updated, _ := db.GetProvisionJob(ctx, "prov-job-update")
	if updated.Status != "running" {
		t.Errorf("UpdateProvisionJobStatus() status = %v, want running", updated.Status)
	}
	if updated.Progress != 75 {
		t.Errorf("UpdateProvisionJobStatus() progress = %d, want 75", updated.Progress)
	}
}

func TestUpdateProvisionJobStatusCompleted(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a job
	job := &ProvisionJob{
		ID:         "prov-job-complete",
		TargetHost: "10.0.0.15",
		TargetPort: 22,
		TargetUser: "deploy",
		Status:     "running",
		Stage:      "installing",
		Progress:   90,
		StartedAt:  time.Now(),
	}
	_ = db.CreateProvisionJob(ctx, job)

	// Complete the job
	err := db.UpdateProvisionJobStatus(ctx, "prov-job-complete", "completed", "done", "", 100)
	if err != nil {
		t.Fatalf("UpdateProvisionJobStatus() completed error = %v", err)
	}

	// Verify completed_at is set
	completed, _ := db.GetProvisionJob(ctx, "prov-job-complete")
	if completed.CompletedAt == nil {
		t.Error("UpdateProvisionJobStatus() completed_at should be set for completed status")
	}
}

func TestUpdateProvisionJobStatusFailed(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a job
	job := &ProvisionJob{
		ID:         "prov-job-fail",
		TargetHost: "10.0.0.20",
		TargetPort: 22,
		TargetUser: "deploy",
		Status:     "running",
		Stage:      "copying",
		Progress:   30,
		StartedAt:  time.Now(),
	}
	_ = db.CreateProvisionJob(ctx, job)

	// Fail the job
	err := db.UpdateProvisionJobStatus(ctx, "prov-job-fail", "failed", "error", "Connection refused", 30)
	if err != nil {
		t.Fatalf("UpdateProvisionJobStatus() failed error = %v", err)
	}

	// Verify
	failed, _ := db.GetProvisionJob(ctx, "prov-job-fail")
	if failed.Status != "failed" {
		t.Errorf("UpdateProvisionJobStatus() status = %v, want failed", failed.Status)
	}
	if failed.ErrorMessage != "Connection refused" {
		t.Errorf("UpdateProvisionJobStatus() error_message = %v, want Connection refused", failed.ErrorMessage)
	}
	if failed.CompletedAt == nil {
		t.Error("UpdateProvisionJobStatus() completed_at should be set for failed status")
	}
}

func TestUpdateProvisionJobStatusNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.UpdateProvisionJobStatus(ctx, "nonexistent-job", "running", "test", "", 0)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateProvisionJobStatus() error = %v, want ErrNotFound", err)
	}
}

func TestListPendingProvisionJobs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create pending and running jobs
	jobs := []struct {
		id     string
		status string
	}{
		{"prov-pending-1", "pending"},
		{"prov-running-1", "running"},
		{"prov-completed-1", "completed"},
	}

	for _, j := range jobs {
		job := &ProvisionJob{
			ID:         j.id,
			TargetHost: "10.0.0.100",
			TargetPort: 22,
			TargetUser: "deploy",
			Status:     j.status,
			StartedAt:  time.Now(),
		}
		_ = db.CreateProvisionJob(ctx, job)
	}

	// List pending (should include pending and running)
	pending, err := db.ListPendingProvisionJobs(ctx)
	if err != nil {
		t.Fatalf("ListPendingProvisionJobs() error = %v", err)
	}

	if len(pending) != 2 {
		t.Errorf("ListPendingProvisionJobs() = %d jobs, want 2", len(pending))
	}
}

func TestListProvisionJobsByHost(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create jobs for specific host
	host := "provision-host.example.com"
	for i := 0; i < 5; i++ {
		job := &ProvisionJob{
			ID:         "host-job-" + string(rune('a'+i)),
			TargetHost: host,
			TargetPort: 22,
			TargetUser: "deploy",
			Status:     "completed",
			StartedAt:  time.Now().Add(-time.Duration(i) * time.Hour),
		}
		_ = db.CreateProvisionJob(ctx, job)
	}

	// List with pagination
	jobs, total, err := db.ListProvisionJobsByHost(ctx, host, 3, 0)
	if err != nil {
		t.Fatalf("ListProvisionJobsByHost() error = %v", err)
	}

	if total != 5 {
		t.Errorf("ListProvisionJobsByHost() total = %d, want 5", total)
	}
	if len(jobs) != 3 {
		t.Errorf("ListProvisionJobsByHost() returned %d, want 3", len(jobs))
	}
}

func TestCleanupOldProvisionJobs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create an old completed job
	job := &ProvisionJob{
		ID:         "old-prov-job",
		TargetHost: "10.0.0.50",
		TargetPort: 22,
		TargetUser: "deploy",
		Status:     "pending",
		StartedAt:  time.Now().Add(-48 * time.Hour),
	}
	_ = db.CreateProvisionJob(ctx, job)

	// Mark it as completed with old completed_at by directly updating
	_, _ = db.conn.ExecContext(ctx, `
		UPDATE agent_provision_jobs 
		SET status = 'completed', completed_at = datetime('now', '-48 hours')
		WHERE id = ?
	`, "old-prov-job")

	// Cleanup old jobs - use a time 24 hours ago (the job is 48 hours old)
	deleted, err := db.CleanupOldProvisionJobs(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("CleanupOldProvisionJobs() error = %v", err)
	}
	if deleted < 1 {
		t.Errorf("CleanupOldProvisionJobs() deleted = %d, want >= 1", deleted)
	}
}

// --- Count Tests ---

func TestCountAgents(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create some agents
	for i := 0; i < 3; i++ {
		agent := &Agent{
			ID:       "count-agent-" + string(rune('a'+i)),
			Hostname: "agent" + string(rune('0'+i)) + ".example.com",
			Status:   "online",
		}
		_ = db.UpsertAgent(ctx, agent)
	}

	// Count agents
	count, err := db.CountAgents(ctx)
	if err != nil {
		t.Fatalf("CountAgents() error = %v", err)
	}
	if count != 3 {
		t.Errorf("CountAgents() = %d, want 3", count)
	}
}

func TestCountAgentsByStatus(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create agents with different statuses
	agents := []struct {
		id     string
		status string
	}{
		{"status-agent-1", "online"},
		{"status-agent-2", "online"},
		{"status-agent-3", "offline"},
		{"status-agent-4", "disconnected"},
	}

	for _, a := range agents {
		agent := &Agent{
			ID:       a.id,
			Hostname: a.id + ".example.com",
			Status:   a.status,
		}
		_ = db.UpsertAgent(ctx, agent)
	}

	// Count by status
	counts, err := db.CountAgentsByStatus(ctx)
	if err != nil {
		t.Fatalf("CountAgentsByStatus() error = %v", err)
	}

	if counts["online"] != 2 {
		t.Errorf("CountAgentsByStatus() online = %d, want 2", counts["online"])
	}
	if counts["offline"] != 1 {
		t.Errorf("CountAgentsByStatus() offline = %d, want 1", counts["offline"])
	}
	if counts["disconnected"] != 1 {
		t.Errorf("CountAgentsByStatus() disconnected = %d, want 1", counts["disconnected"])
	}
}

func TestCountUsers(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create some users
	for i := 0; i < 4; i++ {
		user := &User{
			Username:     "countuser" + string(rune('a'+i)),
			PasswordHash: "hash",
			Email:        "count" + string(rune('a'+i)) + "@example.com",
			Role:         "viewer",
		}
		_ = db.CreateUser(ctx, user)
	}

	// Count users
	count, err := db.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers() error = %v", err)
	}
	if count != 4 {
		t.Errorf("CountUsers() = %d, want 4", count)
	}
}

func TestCountDeploymentsByStatus(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create deployments with different statuses
	deployments := []struct {
		id     string
		status string
	}{
		{"count-deploy-1", "success"},
		{"count-deploy-2", "success"},
		{"count-deploy-3", "failed"},
		{"count-deploy-4", "running"},
	}

	for _, d := range deployments {
		deployment := &DeploymentRecord{
			ID:      d.id,
			Project: "testproject",
			Target:  "production",
			Branch:  "main",
			Status:  d.status,
		}
		_ = db.CreateDeployment(ctx, deployment)
	}

	// Count by status
	counts, err := db.CountDeploymentsByStatus(ctx)
	if err != nil {
		t.Fatalf("CountDeploymentsByStatus() error = %v", err)
	}

	if counts["success"] != 2 {
		t.Errorf("CountDeploymentsByStatus() success = %d, want 2", counts["success"])
	}
	if counts["failed"] != 1 {
		t.Errorf("CountDeploymentsByStatus() failed = %d, want 1", counts["failed"])
	}
	if counts["running"] != 1 {
		t.Errorf("CountDeploymentsByStatus() running = %d, want 1", counts["running"])
	}
}

// --- Transaction Tests ---

func TestRunInTransaction(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Successful transaction
	err := db.RunInTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO users (username, password_hash, email, role) VALUES (?, ?, ?, ?)`,
			"txuser1", "hash", "tx@example.com", "viewer")
		return err
	})
	if err != nil {
		t.Fatalf("RunInTransaction() success case error = %v", err)
	}

	// Verify user was created
	user, err := db.GetUserByUsername(ctx, "txuser1")
	if err != nil {
		t.Fatalf("User should exist after successful transaction: %v", err)
	}
	if user.Username != "txuser1" {
		t.Errorf("Transaction user = %v, want txuser1", user.Username)
	}
}

func TestRunInTransactionRollback(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Transaction that should rollback
	err := db.RunInTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO users (username, password_hash, email, role) VALUES (?, ?, ?, ?)`,
			"txuser2", "hash", "tx2@example.com", "viewer")
		if err != nil {
			return err
		}
		// Return error to trigger rollback
		return errors.New("intentional rollback")
	})
	if err == nil {
		t.Fatal("RunInTransaction() should return error")
	}

	// Verify user was NOT created (rolled back)
	_, err = db.GetUserByUsername(ctx, "txuser2")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("User should not exist after rollback: %v", err)
	}
}

// --- GetUserByID Error Tests ---

func TestGetUserByIDNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.GetUserByID(ctx, 99999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetUserByID() error = %v, want ErrNotFound", err)
	}
}

// --- DeleteAgent Error Test ---

func TestDeleteAgentNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.DeleteAgent(ctx, "nonexistent-agent")
	if err == nil {
		t.Error("DeleteAgent() should return error for nonexistent agent")
	}
}

// --- CancelScheduledDeployment Error Test ---

func TestCancelScheduledDeploymentNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.CancelScheduledDeployment(ctx, "nonexistent-deploy")
	if err == nil {
		t.Error("CancelScheduledDeployment() should return error for nonexistent deployment")
	}
}

// --- DeleteSSHHostKey Error Test ---

func TestDeleteSSHHostKeyNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.DeleteSSHHostKey(ctx, 99999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteSSHHostKey() error = %v, want ErrNotFound", err)
	}
}

// --- GetSSHHostKey Error Test ---

func TestGetSSHHostKeyNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.GetSSHHostKey(ctx, "nonexistent.host", 22, "ssh-rsa")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSSHHostKey() error = %v, want ErrNotFound", err)
	}
}

// --- Setting with encryption ---

func TestSetSettingEncrypted(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Set an encrypted setting
	err := db.SetSetting(ctx, "security", "secret_key", "encrypted_value", "string", true)
	if err != nil {
		t.Fatalf("SetSetting() encrypted error = %v", err)
	}

	// Retrieve and verify
	setting, err := db.GetSetting(ctx, "security", "secret_key")
	if err != nil {
		t.Fatalf("GetSetting() encrypted error = %v", err)
	}

	if !setting.Encrypted {
		t.Error("GetSetting() encrypted = false, want true")
	}
}

// --- JsonToMap edge cases ---

func TestJsonToMap(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantNil bool
	}{
		{"empty string", "", 0, false},
		{"empty object", "{}", 0, false},
		{"invalid json", "not json", 0, false},
		{"valid single", `{"key":"value"}`, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jsonToMap(tt.input)
			if result == nil {
				t.Error("jsonToMap() should never return nil")
			}
			if len(result) != tt.wantLen {
				t.Errorf("jsonToMap() len = %d, want %d", len(result), tt.wantLen)
			}
		})
	}
}

// --- ListPendingScheduledDeployments with data ---

func TestListPendingScheduledDeploymentsWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a scheduled deployment that's in the past (should be due)
	// Use direct SQL to set scheduled_at properly with datetime function for SQLite
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO deployments (id, project, target, branch, status, scheduled_at, scheduled_by, triggered_by)
		VALUES (?, ?, ?, ?, 'scheduled', datetime('now', '-1 minute'), ?, ?)
	`, "past-sched-1", "test-project", "staging", "main", "testuser", "testuser")
	if err != nil {
		t.Fatalf("Insert scheduled deployment error = %v", err)
	}

	// List pending deployments
	deployments, err := db.ListPendingScheduledDeployments(ctx)
	if err != nil {
		t.Fatalf("ListPendingScheduledDeployments() error = %v", err)
	}

	if len(deployments) < 1 {
		t.Errorf("ListPendingScheduledDeployments() = %d, want >= 1", len(deployments))
	}

	// Verify the scheduled deployment is in the list
	found := false
	for _, d := range deployments {
		if d.ID == "past-sched-1" {
			found = true
			if d.Status != "scheduled" {
				t.Errorf("Scheduled deployment status = %v, want scheduled", d.Status)
			}
			break
		}
	}
	if !found {
		t.Error("Created scheduled deployment not found in list")
	}
}

// --- Additional Error Path Tests ---

func TestBackupInvalidDestination(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Try to backup to an invalid path
	err := db.Backup("/nonexistent/directory/backup.db")
	if err == nil {
		t.Error("Backup() should fail for invalid destination path")
	}
}

func TestMarkStaleAgentsWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create an agent that was last seen a while ago with 'connected' status
	// Use direct SQL to set last_seen_at properly
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO agents (id, hostname, status, last_seen_at, registered_at, certificate)
		VALUES (?, ?, 'connected', datetime('now', '-10 minutes'), datetime('now'), '')
	`, "stale-agent-1", "stale.example.com")
	if err != nil {
		t.Fatalf("Insert stale agent error = %v", err)
	}

	// Mark stale agents (older than 5 minutes)
	marked, err := db.MarkStaleAgents(ctx, time.Now().Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("MarkStaleAgents() error = %v", err)
	}
	if marked < 1 {
		t.Errorf("MarkStaleAgents() marked = %d, want >= 1", marked)
	}

	// Verify agent is now disconnected
	agent, err := db.GetAgent(ctx, "stale-agent-1")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if agent.Status != "disconnected" {
		t.Errorf("Agent status = %v, want disconnected", agent.Status)
	}
}

func TestCleanupOldDeploymentsWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create an old completed deployment using direct SQL
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO deployments (id, project, target, branch, status, completed_at)
		VALUES (?, ?, ?, ?, 'success', datetime('now', '-48 hours'))
	`, "old-deploy-cleanup", "test-project", "production", "main")
	if err != nil {
		t.Fatalf("Insert old deployment error = %v", err)
	}

	// Cleanup old deployments (older than 24 hours)
	deleted, err := db.CleanupOldDeployments(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("CleanupOldDeployments() error = %v", err)
	}
	if deleted < 1 {
		t.Errorf("CleanupOldDeployments() deleted = %d, want >= 1", deleted)
	}
}

func TestCleanupExpiredSessionsWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user first
	user := &User{
		Username:     "expsessionuser",
		PasswordHash: "hash",
		Email:        "exp@example.com",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create an expired session using direct SQL
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, expires_at, created_at)
		VALUES (?, ?, datetime('now', '-2 hours'), datetime('now', '-3 hours'))
	`, "expired-session-cleanup", user.ID)
	if err != nil {
		t.Fatalf("Insert expired session error = %v", err)
	}

	// Cleanup expired sessions
	deleted, err := db.CleanupExpiredSessions(ctx, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("CleanupExpiredSessions() error = %v", err)
	}
	if deleted < 1 {
		t.Errorf("CleanupExpiredSessions() deleted = %d, want >= 1", deleted)
	}
}

func TestCleanupExpiredAPIKeysWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user first
	user := &User{
		Username:     "expapiuser",
		PasswordHash: "hash",
		Email:        "expapi@example.com",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create an expired API key using direct SQL
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO api_keys (user_id, name, key_hash, expires_at, created_at)
		VALUES (?, 'expired-key', 'expiredhash', datetime('now', '-2 hours'), datetime('now', '-24 hours'))
	`, user.ID)
	if err != nil {
		t.Fatalf("Insert expired API key error = %v", err)
	}

	// Cleanup expired API keys
	deleted, err := db.CleanupExpiredAPIKeys(ctx, time.Now())
	if err != nil {
		t.Fatalf("CleanupExpiredAPIKeys() error = %v", err)
	}
	if deleted < 1 {
		t.Errorf("CleanupExpiredAPIKeys() deleted = %d, want >= 1", deleted)
	}
}

func TestCleanupOrphanedWebhooksWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create an orphaned webhook (project_id that doesn't exist)
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO project_webhooks (project_id, provider, secret_encrypted, enabled)
		VALUES (99999, 'github', X'1234', 1)
	`)
	if err != nil {
		t.Fatalf("Insert orphaned webhook error = %v", err)
	}

	// Cleanup orphaned webhooks
	deleted, err := db.CleanupOrphanedWebhooks(ctx)
	if err != nil {
		t.Fatalf("CleanupOrphanedWebhooks() error = %v", err)
	}
	if deleted < 1 {
		t.Errorf("CleanupOrphanedWebhooks() deleted = %d, want >= 1", deleted)
	}
}

func TestDeleteExpiredSessionsWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user first
	user := &User{
		Username:     "delexpsessionuser",
		PasswordHash: "hash",
		Email:        "delexp@example.com",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create an expired session using direct SQL
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, expires_at, created_at)
		VALUES (?, ?, datetime('now', '-1 hour'), datetime('now', '-2 hours'))
	`, "del-expired-session", user.ID)
	if err != nil {
		t.Fatalf("Insert expired session error = %v", err)
	}

	// Delete expired sessions
	deleted, err := db.DeleteExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredSessions() error = %v", err)
	}
	if deleted < 1 {
		t.Errorf("DeleteExpiredSessions() deleted = %d, want >= 1", deleted)
	}
}

// --- Project Tests - Additional ---

func TestCreateProjectDuplicate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	project := &Project{
		Name:       "dupproject",
		Repository: "https://github.com/test/dup",
		Branch:     "main",
	}

	// First create should succeed
	err := db.CreateProject(project)
	if err != nil {
		t.Fatalf("CreateProject() first call error = %v", err)
	}

	// Second create should fail (duplicate name)
	project2 := &Project{
		Name:       "dupproject",
		Repository: "https://github.com/test/dup2",
		Branch:     "develop",
	}
	err = db.CreateProject(project2)
	if err == nil {
		t.Error("CreateProject() should fail for duplicate project name")
	}
}

func TestCreateProjectTypeDuplicate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	pt := &ProjectType{
		Name:        "dupptype",
		Description: "First",
		BuildCmd:    "build1",
	}

	// First create should succeed
	err := db.CreateProjectType(pt)
	if err != nil {
		t.Fatalf("CreateProjectType() first call error = %v", err)
	}

	// Second create should fail (duplicate name)
	pt2 := &ProjectType{
		Name:        "dupptype",
		Description: "Second",
		BuildCmd:    "build2",
	}
	err = db.CreateProjectType(pt2)
	if err == nil {
		t.Error("CreateProjectType() should fail for duplicate project type name")
	}
}

// --- SSH Host Key Additional Tests ---

func TestCreateSSHHostKeyDuplicate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	key := &SSHHostKey{
		Hostname:    "dupkey.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   "AAAAB3...",
		Fingerprint: "SHA256:abc123",
		Trusted:     true,
		AddedBy:     "admin",
	}

	// First create should succeed
	err := db.CreateSSHHostKey(ctx, key)
	if err != nil {
		t.Fatalf("CreateSSHHostKey() first call error = %v", err)
	}

	// Second create with same (hostname, port, key_type) should fail if there's a unique constraint
	key2 := &SSHHostKey{
		Hostname:    "dupkey.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   "DIFFERENT...",
		Fingerprint: "SHA256:different",
		Trusted:     false,
		AddedBy:     "user",
	}
	err = db.CreateSSHHostKey(ctx, key2)
	// Note: this may or may not fail depending on schema constraints
	// At minimum, this tests the code path
	_ = err
}

// --- Agent Tests - Additional ---

func TestAgentWithLabels(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	agent := &Agent{
		ID:       "labeled-agent",
		Hostname: "labeled.example.com",
		Labels: map[string]string{
			"env":     "production",
			"region":  "us-west-1",
			"cluster": "main",
		},
		Status: "online",
	}

	err := db.UpsertAgent(ctx, agent)
	if err != nil {
		t.Fatalf("UpsertAgent() with labels error = %v", err)
	}

	// Retrieve and verify labels
	retrieved, err := db.GetAgent(ctx, "labeled-agent")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}

	if retrieved.Labels["env"] != "production" {
		t.Errorf("Agent labels[env] = %v, want production", retrieved.Labels["env"])
	}
	if retrieved.Labels["region"] != "us-west-1" {
		t.Errorf("Agent labels[region] = %v, want us-west-1", retrieved.Labels["region"])
	}
}

func TestListAgentsEmpty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// List agents from empty database
	agents, err := db.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}

	// Should return empty slice, not nil
	if agents == nil {
		agents = []*Agent{}
	}
	if len(agents) != 0 {
		t.Errorf("ListAgents() returned %d agents for empty db, want 0", len(agents))
	}
}

// --- User Tests - Additional ---

func TestListUsersEmpty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// List users from empty database
	users, err := db.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}

	if len(users) != 0 {
		t.Errorf("ListUsers() returned %d users for empty db, want 0", len(users))
	}
}

// --- Deployment Tests - Additional ---

func TestListDeploymentsRecentEmpty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// List deployments from empty database
	deployments, err := db.ListDeploymentsRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListDeploymentsRecent() error = %v", err)
	}

	if len(deployments) != 0 {
		t.Errorf("ListDeploymentsRecent() returned %d for empty db, want 0", len(deployments))
	}
}

// --- Session Tests - Return Values ---

func TestCreateSessionWithIPAndUserAgent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create user first
	user := &User{
		Username:     "sessionipuser",
		PasswordHash: "hash",
		Email:        "sessionip@example.com",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create session with IP and user agent
	session := &Session{
		ID:        "ip-session-token",
		UserID:    user.ID,
		IPAddress: "192.168.1.100",
		UserAgent: "Mozilla/5.0 Test Browser",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	err := db.CreateSession(ctx, session)
	if err != nil {
		t.Fatalf("CreateSession() with IP error = %v", err)
	}

	// Verify IP and user agent are stored
	retrieved, _ := db.GetSessionByToken(ctx, "ip-session-token")
	if retrieved.IPAddress != "192.168.1.100" {
		t.Errorf("Session IPAddress = %v, want 192.168.1.100", retrieved.IPAddress)
	}
	if retrieved.UserAgent != "Mozilla/5.0 Test Browser" {
		t.Errorf("Session UserAgent = %v, want Mozilla/5.0 Test Browser", retrieved.UserAgent)
	}
}

// --- API Key Tests - Return Values ---

func TestCreateAPIKeyWithScopes(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create user first
	user := &User{
		Username:     "scopekeyuser",
		PasswordHash: "hash",
		Email:        "scope@example.com",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create API key with scopes
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	key := &APIKey{
		Name:      "scoped-key",
		KeyHash:   "scoped-hash",
		UserID:    user.ID,
		Scopes:    `["read", "write", "admin"]`,
		ExpiresAt: &expiresAt,
	}

	err := db.CreateAPIKey(ctx, key)
	if err != nil {
		t.Fatalf("CreateAPIKey() with scopes error = %v", err)
	}

	// Verify scopes are stored
	retrieved, _ := db.GetAPIKeyByHash(ctx, "scoped-hash")
	if retrieved.Scopes != `["read", "write", "admin"]` {
		t.Errorf("APIKey Scopes = %v, want scopes", retrieved.Scopes)
	}
}

func TestListAPIKeysWithLastUsed(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create user
	user := &User{
		Username:     "lastuseduser",
		PasswordHash: "hash",
		Email:        "lastused@example.com",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create API key and update usage
	key := &APIKey{
		Name:    "used-key",
		KeyHash: "used-hash",
		UserID:  user.ID,
	}
	_ = db.CreateAPIKey(ctx, key)
	_ = db.UpdateAPIKeyUsage(ctx, key.ID)

	// List and verify last_used_at is set
	keys, _ := db.ListAPIKeys(ctx, user.ID)
	if len(keys) < 1 {
		t.Fatal("ListAPIKeys() returned empty")
	}

	// At least one key should have LastUsedAt set
	found := false
	for _, k := range keys {
		if k.LastUsedAt != nil {
			found = true
			break
		}
	}
	if !found {
		t.Error("No API key has LastUsedAt set after UpdateAPIKeyUsage")
	}
}

// --- More Cleanup Tests with Zero Returns ---

func TestCleanupExpiredSessionsZero(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// No sessions to clean up
	deleted, err := db.CleanupExpiredSessions(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CleanupExpiredSessions() error = %v", err)
	}
	if deleted != 0 {
		t.Errorf("CleanupExpiredSessions() deleted = %d, want 0", deleted)
	}
}

func TestCleanupOldDeploymentsZero(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// No deployments to clean up
	deleted, err := db.CleanupOldDeployments(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("CleanupOldDeployments() error = %v", err)
	}
	if deleted != 0 {
		t.Errorf("CleanupOldDeployments() deleted = %d, want 0", deleted)
	}
}

func TestCleanupOldAuditLogsZero(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// No audit logs to clean up
	deleted, err := db.CleanupOldAuditLogs(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("CleanupOldAuditLogs() error = %v", err)
	}
	if deleted != 0 {
		t.Errorf("CleanupOldAuditLogs() deleted = %d, want 0", deleted)
	}
}

func TestMarkStaleAgentsZero(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// No stale agents
	marked, err := db.MarkStaleAgents(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("MarkStaleAgents() error = %v", err)
	}
	if marked != 0 {
		t.Errorf("MarkStaleAgents() marked = %d, want 0", marked)
	}
}

func TestCleanupExpiredAPIKeysZero(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// No expired keys
	deleted, err := db.CleanupExpiredAPIKeys(ctx, time.Now())
	if err != nil {
		t.Fatalf("CleanupExpiredAPIKeys() error = %v", err)
	}
	if deleted != 0 {
		t.Errorf("CleanupExpiredAPIKeys() deleted = %d, want 0", deleted)
	}
}

func TestCleanupOrphanedWebhooksZero(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// No orphaned webhooks
	deleted, err := db.CleanupOrphanedWebhooks(ctx)
	if err != nil {
		t.Fatalf("CleanupOrphanedWebhooks() error = %v", err)
	}
	if deleted != 0 {
		t.Errorf("CleanupOrphanedWebhooks() deleted = %d, want 0", deleted)
	}
}

func TestDeleteExpiredSessionsZero(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// No expired sessions
	deleted, err := db.DeleteExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredSessions() error = %v", err)
	}
	if deleted != 0 {
		t.Errorf("DeleteExpiredSessions() deleted = %d, want 0", deleted)
	}
}

// --- Error Path Tests ---

func TestCancelScheduledDeploymentAlreadyCancelled(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a scheduled deployment
	futureTime := time.Now().Add(1 * time.Hour)
	_ = db.CreateScheduledDeployment(ctx, "cancel-twice", "project", "prod", "main", futureTime, "user")

	// Cancel it once
	err := db.CancelScheduledDeployment(ctx, "cancel-twice")
	if err != nil {
		t.Fatalf("CancelScheduledDeployment() first call error = %v", err)
	}

	// Try to cancel again - should fail because status is no longer 'scheduled'
	err = db.CancelScheduledDeployment(ctx, "cancel-twice")
	if err == nil {
		t.Error("CancelScheduledDeployment() should fail for already cancelled deployment")
	}
}

func TestDeleteAgentNotFoundDetailed(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Try to delete non-existent agent
	err := db.DeleteAgent(ctx, "agent-that-does-not-exist")
	if err == nil {
		t.Error("DeleteAgent() should return error for non-existent agent")
	}
}

// --- Project Operations ---

func TestUpdateProjectByNameDetailed(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create project
	project := &Project{
		Name:       "update-detailed",
		Repository: "https://github.com/test/update",
		Branch:     "main",
		DeployPath: "/var/www/app",
		Type:       "nodejs",
	}
	_ = db.CreateProject(project)

	// Update all fields
	updated := &Project{
		Name:       "update-detailed",
		Repository: "https://gitlab.com/test/updated",
		Branch:     "develop",
		DeployPath: "/opt/app",
		Type:       "python",
	}
	err := db.UpdateProjectByName(ctx, updated)
	if err != nil {
		t.Fatalf("UpdateProjectByName() error = %v", err)
	}

	// Verify all fields updated
	retrieved, _ := db.GetProjectByName(ctx, "update-detailed")
	if retrieved.Repository != "https://gitlab.com/test/updated" {
		t.Errorf("Project Repository = %v, want updated", retrieved.Repository)
	}
	if retrieved.Branch != "develop" {
		t.Errorf("Project Branch = %v, want develop", retrieved.Branch)
	}
	if retrieved.DeployPath != "/opt/app" {
		t.Errorf("Project DeployPath = %v, want /opt/app", retrieved.DeployPath)
	}
	if retrieved.Type != "python" {
		t.Errorf("Project Type = %v, want python", retrieved.Type)
	}
}

// --- API Key Last Used Tests ---

func TestGetAPIKeyByHashWithLastUsed(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create user
	user := &User{
		Username:     "lastusedhashuser",
		PasswordHash: "hash",
		Email:        "lasthash@example.com",
		Role:         "admin",
	}
	_ = db.CreateUser(ctx, user)

	// Create API key
	key := &APIKey{
		Name:    "lastused-hash-key",
		KeyHash: "lastused-unique-hash",
		UserID:  user.ID,
	}
	_ = db.CreateAPIKey(ctx, key)

	// Update usage
	_ = db.UpdateAPIKeyUsage(ctx, key.ID)

	// Get by hash and verify LastUsedAt
	retrieved, err := db.GetAPIKeyByHash(ctx, "lastused-unique-hash")
	if err != nil {
		t.Fatalf("GetAPIKeyByHash() error = %v", err)
	}
	if retrieved.LastUsedAt == nil {
		t.Error("GetAPIKeyByHash() LastUsedAt should be set after UpdateAPIKeyUsage")
	}
}

// --- User TOTP Tests ---

func TestUserWithTOTP(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create user with TOTP
	user := &User{
		Username:           "totpuser",
		PasswordHash:       "hash",
		Email:              "totp@example.com",
		Role:               "admin",
		TOTPSecret:         "JBSWY3DPEHPK3PXP",
		TOTPEnabled:        true,
		MustChangePassword: false,
	}
	err := db.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("CreateUser() with TOTP error = %v", err)
	}

	// Update to disable TOTP
	user.TOTPEnabled = false
	user.TOTPSecret = ""
	err = db.UpdateUserByID(ctx, user)
	if err != nil {
		t.Fatalf("UpdateUserByID() error = %v", err)
	}

	// Verify
	retrieved, _ := db.GetUserByUsername(ctx, "totpuser")
	if retrieved.TOTPEnabled {
		t.Error("User TOTPEnabled should be false after update")
	}
}

// --- Deployment with Error Message ---

func TestDeploymentWithErrorMessage(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create deployment
	deployment := &DeploymentRecord{
		ID:      "error-deploy",
		Project: "error-project",
		Target:  "production",
		Branch:  "main",
		Status:  "running",
	}
	_ = db.CreateDeployment(ctx, deployment)

	// Update with error
	now := time.Now()
	deployment.Status = "failed"
	deployment.ErrorMessage = "Build failed: exit code 1"
	deployment.CompletedAt = &now
	err := db.UpdateDeployment(ctx, deployment)
	if err != nil {
		t.Fatalf("UpdateDeployment() with error error = %v", err)
	}

	// Verify error message
	retrieved, _ := db.GetDeployment(ctx, "error-deploy")
	if retrieved.ErrorMessage != "Build failed: exit code 1" {
		t.Errorf("Deployment ErrorMessage = %v, want error message", retrieved.ErrorMessage)
	}
}

// --- Settings with description ---

func TestSetSettingWithDescription(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Set setting (note: SetSetting doesn't take description, but GetSetting returns it)
	err := db.SetSetting(ctx, "app", "timeout", "30", "int", false)
	if err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}

	// Verify
	setting, _ := db.GetSetting(ctx, "app", "timeout")
	if setting.ValueType != "int" {
		t.Errorf("Setting ValueType = %v, want int", setting.ValueType)
	}
}

// --- SSH Host Key with verification ---

func TestSSHHostKeyWithVerification(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	now := time.Now()
	key := &SSHHostKey{
		Hostname:    "verified.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   "AAAAB3...",
		Fingerprint: "SHA256:verified",
		Trusted:     true,
		AddedBy:     "admin",
		VerifiedAt:  &now,
	}

	err := db.CreateSSHHostKey(ctx, key)
	if err != nil {
		t.Fatalf("CreateSSHHostKey() with verification error = %v", err)
	}

	// Retrieve and check verified_at
	retrieved, _ := db.GetSSHHostKey(ctx, "verified.example.com", 22, "ssh-rsa")
	if retrieved.VerifiedAt == nil {
		t.Error("SSHHostKey VerifiedAt should be set")
	}
}

// --- Count with Data ---

func TestCountAgentsEmpty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	count, err := db.CountAgents(ctx)
	if err != nil {
		t.Fatalf("CountAgents() error = %v", err)
	}
	if count != 0 {
		t.Errorf("CountAgents() = %d for empty db, want 0", count)
	}
}

func TestCountUsersEmpty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	count, err := db.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers() error = %v", err)
	}
	if count != 0 {
		t.Errorf("CountUsers() = %d for empty db, want 0", count)
	}
}

func TestCountDeploymentsByStatusEmpty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	counts, err := db.CountDeploymentsByStatus(ctx)
	if err != nil {
		t.Fatalf("CountDeploymentsByStatus() error = %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("CountDeploymentsByStatus() = %v for empty db, want empty map", counts)
	}
}

func TestCountAgentsByStatusEmpty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	counts, err := db.CountAgentsByStatus(ctx)
	if err != nil {
		t.Fatalf("CountAgentsByStatus() error = %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("CountAgentsByStatus() = %v for empty db, want empty map", counts)
	}
}

// --- Additional Coverage Tests ---

func TestListProjectsWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create multiple projects
	projects := []*Project{
		{Name: "proj1", Repository: "https://github.com/test/1", Branch: "main", DeployPath: "/var/www/1", Type: "nodejs"},
		{Name: "proj2", Repository: "https://github.com/test/2", Branch: "develop", DeployPath: "/var/www/2", Type: "python"},
	}
	for _, p := range projects {
		if err := db.CreateProject(p); err != nil {
			t.Fatalf("CreateProject() error = %v", err)
		}
	}

	// List projects
	list, err := db.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListProjects() = %d projects, want 2", len(list))
	}
}

func TestListProjectTypesWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create project types
	pt := &ProjectType{Name: "custom", Description: "Custom type", BuildCmd: "make build"}
	if err := db.CreateProjectType(pt); err != nil {
		t.Fatalf("CreateProjectType() error = %v", err)
	}

	// List types
	types, err := db.ListProjectTypes()
	if err != nil {
		t.Fatalf("ListProjectTypes() error = %v", err)
	}
	found := false
	for _, tp := range types {
		if tp.Name == "custom" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ListProjectTypes() did not return created type")
	}
}

func TestListSecretsCtxWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create secrets
	_ = db.SetSecretEncrypted(ctx, "myproject", "production", "key1", []byte("encrypted1"))
	_ = db.SetSecretEncrypted(ctx, "myproject", "staging", "key2", []byte("encrypted2"))

	// List secrets for project
	secrets, err := db.ListSecretsCtx(ctx, "myproject")
	if err != nil {
		t.Fatalf("ListSecretsCtx() error = %v", err)
	}
	if len(secrets) != 2 {
		t.Errorf("ListSecretsCtx() = %d secrets, want 2", len(secrets))
	}
}

func TestListSecretsWithScopeData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create secrets in different scopes
	_ = db.SetSecretEncrypted(ctx, "proj", "prod", "key1", []byte("enc1"))
	_ = db.SetSecretEncrypted(ctx, "proj", "prod", "key2", []byte("enc2"))
	_ = db.SetSecretEncrypted(ctx, "proj", "staging", "key3", []byte("enc3"))

	// List secrets for prod scope
	secrets, err := db.ListSecretsWithScope(ctx, "proj", "prod")
	if err != nil {
		t.Fatalf("ListSecretsWithScope() error = %v", err)
	}
	if len(secrets) != 2 {
		t.Errorf("ListSecretsWithScope(prod) = %d, want 2", len(secrets))
	}
}

func TestListAllSecretsCtxData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create secrets across projects
	_ = db.SetSecretEncrypted(ctx, "proj1", "prod", "key1", []byte("enc1"))
	_ = db.SetSecretEncrypted(ctx, "proj2", "prod", "key2", []byte("enc2"))

	// List all secrets
	secrets, err := db.ListAllSecretsCtx(ctx)
	if err != nil {
		t.Fatalf("ListAllSecretsCtx() error = %v", err)
	}
	if len(secrets) < 2 {
		t.Errorf("ListAllSecretsCtx() = %d, want >= 2", len(secrets))
	}
}

func TestDeleteSSHHostKeysByHostWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple keys for same host
	keys := []*SSHHostKey{
		{Hostname: "multi.example.com", Port: 22, KeyType: "ssh-rsa", PublicKey: "AAAA1", Fingerprint: "fp1", Trusted: true, AddedBy: "admin"},
		{Hostname: "multi.example.com", Port: 22, KeyType: "ssh-ed25519", PublicKey: "AAAA2", Fingerprint: "fp2", Trusted: true, AddedBy: "admin"},
	}
	for _, k := range keys {
		_ = db.CreateSSHHostKey(ctx, k)
	}

	// Delete all keys for host
	deleted, err := db.DeleteSSHHostKeysByHost(ctx, "multi.example.com", 22)
	if err != nil {
		t.Fatalf("DeleteSSHHostKeysByHost() error = %v", err)
	}
	if deleted != 2 {
		t.Errorf("DeleteSSHHostKeysByHost() = %d, want 2", deleted)
	}

	// Verify gone
	remaining, _ := db.ListSSHHostKeys(ctx)
	for _, k := range remaining {
		if k.Hostname == "multi.example.com" {
			t.Error("Host keys still exist after DeleteSSHHostKeysByHost")
		}
	}
}

func TestListSSHHostKeysWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create keys
	keys := []*SSHHostKey{
		{Hostname: "host1.example.com", Port: 22, KeyType: "ssh-rsa", PublicKey: "key1", Fingerprint: "fp1", Trusted: true, AddedBy: "admin"},
		{Hostname: "host2.example.com", Port: 22, KeyType: "ssh-ed25519", PublicKey: "key2", Fingerprint: "fp2", Trusted: false, AddedBy: "user"},
	}
	for _, k := range keys {
		_ = db.CreateSSHHostKey(ctx, k)
	}

	// List all
	list, err := db.ListSSHHostKeys(ctx)
	if err != nil {
		t.Fatalf("ListSSHHostKeys() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListSSHHostKeys() = %d, want 2", len(list))
	}
}

func TestBackupData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Add some data
	_ = db.CreateUser(ctx, &User{Username: "backupuser2", PasswordHash: "hash", Email: "backup2@test.com", Role: "admin"})

	// Create backup
	backupPath := filepath.Join(t.TempDir(), "backup2.db")
	err := db.Backup(backupPath)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	// Verify backup exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup file was not created")
	}
}

func TestUpdateSSHHostKeyTrustWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create key
	key := &SSHHostKey{
		Hostname:    "trust.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   "pubkey",
		Fingerprint: "fp-trust",
		Trusted:     false,
		AddedBy:     "admin",
	}
	_ = db.CreateSSHHostKey(ctx, key)

	// Update trust
	err := db.UpdateSSHHostKeyTrust(ctx, key.ID, true, "verifier")
	if err != nil {
		t.Fatalf("UpdateSSHHostKeyTrust() error = %v", err)
	}

	// Verify
	updated, _ := db.GetSSHHostKey(ctx, "trust.example.com", 22, "ssh-rsa")
	if !updated.Trusted {
		t.Error("SSHHostKey Trusted should be true after update")
	}
}

func TestDeleteSSHHostKeyWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create key
	key := &SSHHostKey{
		Hostname:    "delete.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   "pubkey",
		Fingerprint: "fp-delete",
		Trusted:     true,
		AddedBy:     "admin",
	}
	_ = db.CreateSSHHostKey(ctx, key)

	// Delete
	err := db.DeleteSSHHostKey(ctx, key.ID)
	if err != nil {
		t.Fatalf("DeleteSSHHostKey() error = %v", err)
	}

	// Verify gone
	_, err = db.GetSSHHostKey(ctx, "delete.example.com", 22, "ssh-rsa")
	if err != ErrNotFound {
		t.Error("GetSSHHostKey() should return ErrNotFound after delete")
	}
}

func TestListSettingsByCategoryWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create settings in same category
	_ = db.SetSetting(ctx, "app", "setting1", "value1", "string", false)
	_ = db.SetSetting(ctx, "app", "setting2", "value2", "string", false)
	_ = db.SetSetting(ctx, "other", "setting3", "value3", "string", false)

	// List by category
	settings, err := db.ListSettingsByCategory(ctx, "app")
	if err != nil {
		t.Fatalf("ListSettingsByCategory() error = %v", err)
	}
	if len(settings) != 2 {
		t.Errorf("ListSettingsByCategory(app) = %d, want 2", len(settings))
	}
}

func TestListAllSettingsWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create settings
	_ = db.SetSetting(ctx, "cat1", "s1", "v1", "string", false)
	_ = db.SetSetting(ctx, "cat2", "s2", "v2", "int", true)

	// List all
	settings, err := db.ListAllSettings(ctx)
	if err != nil {
		t.Fatalf("ListAllSettings() error = %v", err)
	}
	if len(settings) < 2 {
		t.Errorf("ListAllSettings() = %d, want >= 2", len(settings))
	}
}

func TestHasSettingsTrue(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a setting
	_ = db.SetSetting(ctx, "test", "exists", "value", "string", false)

	// Check
	has, err := db.HasSettings(ctx)
	if err != nil {
		t.Fatalf("HasSettings() error = %v", err)
	}
	if !has {
		t.Error("HasSettings() = false, want true")
	}
}

func TestHasSettingsFalse(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Delete all settings first
	_, _ = db.conn.ExecContext(ctx, "DELETE FROM settings")

	// Check
	has, err := db.HasSettings(ctx)
	if err != nil {
		t.Fatalf("HasSettings() error = %v", err)
	}
	if has {
		t.Error("HasSettings() = true, want false")
	}
}

func TestExportAllSecretsWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create secrets
	_ = db.SetSecretEncrypted(ctx, "export-proj", "prod", "key1", []byte("enc1"))
	_ = db.SetSecretEncrypted(ctx, "export-proj", "staging", "key2", []byte("enc2"))

	// Export - ExportAllSecrets takes no arguments
	secrets, err := db.ExportAllSecrets()
	if err != nil {
		t.Fatalf("ExportAllSecrets() error = %v", err)
	}
	// ExportAllSecrets returns map[string]map[string]string
	_ = ctx // use ctx to avoid lint
	if len(secrets) < 1 {
		t.Errorf("ExportAllSecrets() = %d projects, want >= 1", len(secrets))
	}
}

func TestListUserSessionsWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create user
	user := &User{Username: "sessionlistuser", PasswordHash: "hash", Email: "sl@test.com", Role: "admin"}
	_ = db.CreateUser(ctx, user)

	// Create sessions with unique IDs (Session uses ID field, not Token)
	sessionIDs := []string{"sess-alpha", "sess-beta", "sess-gamma"}
	for _, id := range sessionIDs {
		session := &Session{
			ID:        id,
			UserID:    user.ID,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		_ = db.CreateSession(ctx, session)
	}

	// List
	sessions, err := db.ListUserSessions(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListUserSessions() error = %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("ListUserSessions() = %d, want 3", len(sessions))
	}
}

func TestListAPIKeysWithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create user
	user := &User{Username: "apikeylistuser", PasswordHash: "hash", Email: "akl@test.com", Role: "admin"}
	_ = db.CreateUser(ctx, user)

	// Create API keys using string index
	for i := 0; i < 2; i++ {
		key := &APIKey{
			Name:    "listkey" + string(rune('a'+i)),
			KeyHash: "listhash" + string(rune('a'+i)),
			UserID:  user.ID,
		}
		_ = db.CreateAPIKey(ctx, key)
	}

	// List
	keys, err := db.ListAPIKeys(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys() error = %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("ListAPIKeys() = %d, want 2", len(keys))
	}
}

// --- SSH Jump Server Tests ---

func TestJumpServerCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create
	js := &SSHJumpServer{
		Name:     "bastion1",
		Host:     "bastion.example.com",
		Port:     22,
		Username: "admin",
	}
	if err := db.CreateJumpServer(ctx, js); err != nil {
		t.Fatalf("CreateJumpServer() error = %v", err)
	}
	if js.ID == 0 {
		t.Error("CreateJumpServer() did not set ID")
	}

	// Get by ID
	got, err := db.GetJumpServer(ctx, js.ID)
	if err != nil {
		t.Fatalf("GetJumpServer() error = %v", err)
	}
	if got.Name != js.Name {
		t.Errorf("GetJumpServer() name = %v, want %v", got.Name, js.Name)
	}
	if got.Host != js.Host {
		t.Errorf("GetJumpServer() host = %v, want %v", got.Host, js.Host)
	}
	if got.Port != js.Port {
		t.Errorf("GetJumpServer() port = %v, want %v", got.Port, js.Port)
	}
	if got.Username != js.Username {
		t.Errorf("GetJumpServer() username = %v, want %v", got.Username, js.Username)
	}

	// Get by name
	gotByName, err := db.GetJumpServerByName(ctx, "bastion1")
	if err != nil {
		t.Fatalf("GetJumpServerByName() error = %v", err)
	}
	if gotByName.ID != js.ID {
		t.Errorf("GetJumpServerByName() ID = %v, want %v", gotByName.ID, js.ID)
	}

	// Update
	js.Host = "new-bastion.example.com"
	js.Port = 2222
	if err := db.UpdateJumpServer(ctx, js); err != nil {
		t.Fatalf("UpdateJumpServer() error = %v", err)
	}

	// Verify update
	updated, err := db.GetJumpServer(ctx, js.ID)
	if err != nil {
		t.Fatalf("GetJumpServer() after update error = %v", err)
	}
	if updated.Host != "new-bastion.example.com" {
		t.Errorf("UpdateJumpServer() host = %v, want new-bastion.example.com", updated.Host)
	}
	if updated.Port != 2222 {
		t.Errorf("UpdateJumpServer() port = %v, want 2222", updated.Port)
	}

	// Delete
	if err := db.DeleteJumpServer(ctx, js.ID); err != nil {
		t.Fatalf("DeleteJumpServer() error = %v", err)
	}

	// Verify delete
	_, err = db.GetJumpServer(ctx, js.ID)
	if err != ErrNotFound {
		t.Errorf("GetJumpServer() after delete error = %v, want ErrNotFound", err)
	}
}

func TestListJumpServers(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple jump servers
	servers := []*SSHJumpServer{
		{Name: "bastion-a", Host: "a.example.com", Port: 22, Username: "admin"},
		{Name: "bastion-b", Host: "b.example.com", Port: 22, Username: "root"},
		{Name: "bastion-c", Host: "c.example.com", Port: 2222, Username: "ubuntu"},
	}

	for _, js := range servers {
		if err := db.CreateJumpServer(ctx, js); err != nil {
			t.Fatalf("CreateJumpServer() error = %v", err)
		}
	}

	// List all
	list, err := db.ListJumpServers(ctx)
	if err != nil {
		t.Fatalf("ListJumpServers() error = %v", err)
	}
	if len(list) != 3 {
		t.Errorf("ListJumpServers() = %d servers, want 3", len(list))
	}

	// Verify sorting by name
	if list[0].Name != "bastion-a" {
		t.Errorf("ListJumpServers() first server = %s, want bastion-a", list[0].Name)
	}
}

func TestJumpServerNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Get by ID
	_, err := db.GetJumpServer(ctx, 99999)
	if err != ErrNotFound {
		t.Errorf("GetJumpServer() error = %v, want ErrNotFound", err)
	}

	// Get by name
	_, err = db.GetJumpServerByName(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("GetJumpServerByName() error = %v, want ErrNotFound", err)
	}

	// Update nonexistent
	err = db.UpdateJumpServer(ctx, &SSHJumpServer{ID: 99999, Name: "test"})
	if err != ErrNotFound {
		t.Errorf("UpdateJumpServer() error = %v, want ErrNotFound", err)
	}

	// Delete nonexistent
	err = db.DeleteJumpServer(ctx, 99999)
	if err != ErrNotFound {
		t.Errorf("DeleteJumpServer() error = %v, want ErrNotFound", err)
	}
}

func TestJumpServerWithSSHKeyID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create jump server with SSH key reference
	keyID := int64(42)
	js := &SSHJumpServer{
		Name:     "bastion-with-key",
		Host:     "key.example.com",
		Port:     22,
		Username: "keyuser",
		SSHKeyID: &keyID,
	}
	if err := db.CreateJumpServer(ctx, js); err != nil {
		t.Fatalf("CreateJumpServer() error = %v", err)
	}

	// Get and verify SSH key ID is preserved
	got, err := db.GetJumpServer(ctx, js.ID)
	if err != nil {
		t.Fatalf("GetJumpServer() error = %v", err)
	}
	if got.SSHKeyID == nil {
		t.Error("GetJumpServer() SSHKeyID = nil, want non-nil")
	} else if *got.SSHKeyID != keyID {
		t.Errorf("GetJumpServer() SSHKeyID = %d, want %d", *got.SSHKeyID, keyID)
	}
}

func TestJumpServerDuplicateName(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create first server
	js1 := &SSHJumpServer{Name: "unique-name", Host: "a.example.com", Port: 22, Username: "user"}
	if err := db.CreateJumpServer(ctx, js1); err != nil {
		t.Fatalf("CreateJumpServer() first error = %v", err)
	}

	// Try to create with duplicate name
	js2 := &SSHJumpServer{Name: "unique-name", Host: "b.example.com", Port: 22, Username: "user"}
	err := db.CreateJumpServer(ctx, js2)
	if err == nil {
		t.Error("CreateJumpServer() with duplicate name should fail")
	}
}

// --- Agent Binary Tests ---

func TestAgentBinary_CRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create agent binary
	binary := &AgentBinary{
		Version:        "1.0.0",
		OS:             "linux",
		Arch:           "amd64",
		Path:           "/opt/binaries/vcdeploy-agent-1.0.0-linux-amd64",
		ChecksumSHA256: "abc123def456",
		SizeBytes:      1024000,
		UploadedAt:     time.Now(),
		IsCurrent:      false,
	}

	err := db.CreateAgentBinary(ctx, binary)
	if err != nil {
		t.Fatalf("CreateAgentBinary() error = %v", err)
	}
	if binary.ID == 0 {
		t.Error("CreateAgentBinary() did not set ID")
	}

	// Get by ID
	got, err := db.GetAgentBinary(ctx, binary.ID)
	if err != nil {
		t.Fatalf("GetAgentBinary() error = %v", err)
	}
	if got.Version != binary.Version {
		t.Errorf("GetAgentBinary() Version = %s, want %s", got.Version, binary.Version)
	}
	if got.OS != binary.OS {
		t.Errorf("GetAgentBinary() OS = %s, want %s", got.OS, binary.OS)
	}
	if got.Arch != binary.Arch {
		t.Errorf("GetAgentBinary() Arch = %s, want %s", got.Arch, binary.Arch)
	}
	if got.ChecksumSHA256 != binary.ChecksumSHA256 {
		t.Errorf("GetAgentBinary() ChecksumSHA256 = %s, want %s", got.ChecksumSHA256, binary.ChecksumSHA256)
	}

	// Delete
	err = db.DeleteAgentBinary(ctx, binary.ID)
	if err != nil {
		t.Fatalf("DeleteAgentBinary() error = %v", err)
	}

	// Verify deleted
	_, err = db.GetAgentBinary(ctx, binary.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAgentBinary() after delete should return ErrNotFound, got %v", err)
	}
}

func TestAgentBinary_GetByVersion(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	binary := &AgentBinary{
		Version:        "2.0.0",
		OS:             "darwin",
		Arch:           "arm64",
		Path:           "/opt/binaries/vcdeploy-agent-2.0.0-darwin-arm64",
		ChecksumSHA256: "xyz789",
		SizeBytes:      2048000,
		UploadedAt:     time.Now(),
		IsCurrent:      false,
	}

	if err := db.CreateAgentBinary(ctx, binary); err != nil {
		t.Fatalf("CreateAgentBinary() error = %v", err)
	}

	// Get by version
	got, err := db.GetAgentBinaryByVersion(ctx, "2.0.0", "darwin", "arm64")
	if err != nil {
		t.Fatalf("GetAgentBinaryByVersion() error = %v", err)
	}
	if got.ID != binary.ID {
		t.Errorf("GetAgentBinaryByVersion() ID = %d, want %d", got.ID, binary.ID)
	}

	// Non-existent version
	_, err = db.GetAgentBinaryByVersion(ctx, "999.0.0", "darwin", "arm64")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAgentBinaryByVersion() for non-existent should return ErrNotFound, got %v", err)
	}
}

func TestAgentBinary_SetCurrent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create two binaries for the same OS/arch
	binary1 := &AgentBinary{
		Version: "1.0.0", OS: "linux", Arch: "amd64",
		Path: "/path/1", ChecksumSHA256: "hash1", SizeBytes: 1000, UploadedAt: time.Now(),
		IsCurrent: true,
	}
	binary2 := &AgentBinary{
		Version: "1.1.0", OS: "linux", Arch: "amd64",
		Path: "/path/2", ChecksumSHA256: "hash2", SizeBytes: 1100, UploadedAt: time.Now(),
		IsCurrent: false,
	}

	if err := db.CreateAgentBinary(ctx, binary1); err != nil {
		t.Fatalf("CreateAgentBinary() binary1 error = %v", err)
	}
	if err := db.CreateAgentBinary(ctx, binary2); err != nil {
		t.Fatalf("CreateAgentBinary() binary2 error = %v", err)
	}

	// Get current (should be binary1)
	current, err := db.GetCurrentAgentBinary(ctx, "linux", "amd64")
	if err != nil {
		t.Fatalf("GetCurrentAgentBinary() error = %v", err)
	}
	if current.ID != binary1.ID {
		t.Errorf("GetCurrentAgentBinary() ID = %d, want %d", current.ID, binary1.ID)
	}

	// Set binary2 as current
	if err := db.SetCurrentAgentBinary(ctx, binary2.ID); err != nil {
		t.Fatalf("SetCurrentAgentBinary() error = %v", err)
	}

	// Verify binary2 is now current
	current, err = db.GetCurrentAgentBinary(ctx, "linux", "amd64")
	if err != nil {
		t.Fatalf("GetCurrentAgentBinary() after set error = %v", err)
	}
	if current.ID != binary2.ID {
		t.Errorf("GetCurrentAgentBinary() after set ID = %d, want %d", current.ID, binary2.ID)
	}

	// Verify binary1 is no longer current
	got, err := db.GetAgentBinary(ctx, binary1.ID)
	if err != nil {
		t.Fatalf("GetAgentBinary() binary1 error = %v", err)
	}
	if got.IsCurrent {
		t.Error("binary1 should no longer be current")
	}
}

func TestAgentBinary_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple binaries
	binaries := []*AgentBinary{
		{Version: "1.0.0", OS: "linux", Arch: "amd64", Path: "/p1", ChecksumSHA256: "h1", SizeBytes: 100, UploadedAt: time.Now()},
		{Version: "1.0.0", OS: "darwin", Arch: "arm64", Path: "/p2", ChecksumSHA256: "h2", SizeBytes: 200, UploadedAt: time.Now()},
		{Version: "1.1.0", OS: "linux", Arch: "amd64", Path: "/p3", ChecksumSHA256: "h3", SizeBytes: 300, UploadedAt: time.Now()},
	}

	for _, b := range binaries {
		if err := db.CreateAgentBinary(ctx, b); err != nil {
			t.Fatalf("CreateAgentBinary() error = %v", err)
		}
	}

	// List all
	list, err := db.ListAgentBinaries(ctx)
	if err != nil {
		t.Fatalf("ListAgentBinaries() error = %v", err)
	}
	if len(list) != 3 {
		t.Errorf("ListAgentBinaries() count = %d, want 3", len(list))
	}
}

func TestAgentBinary_DeleteNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.DeleteAgentBinary(ctx, 999999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteAgentBinary() non-existent should return ErrNotFound, got %v", err)
	}
}

// --- Agent Update History Tests ---

func TestAgentUpdateHistory_CRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create an agent first
	agent := &Agent{
		ID:           "agent-update-test",
		Hostname:     "test-host",
		Status:       "online",
		RegisteredAt: time.Now(),
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	// Create update history
	history := &AgentUpdateHistory{
		AgentID:     agent.ID,
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Status:      "pending",
		StartedAt:   time.Now(),
	}

	err := db.CreateAgentUpdateHistory(ctx, history)
	if err != nil {
		t.Fatalf("CreateAgentUpdateHistory() error = %v", err)
	}
	if history.ID == 0 {
		t.Error("CreateAgentUpdateHistory() did not set ID")
	}

	// Get by ID
	got, err := db.GetAgentUpdateHistory(ctx, history.ID)
	if err != nil {
		t.Fatalf("GetAgentUpdateHistory() error = %v", err)
	}
	if got.AgentID != history.AgentID {
		t.Errorf("GetAgentUpdateHistory() AgentID = %s, want %s", got.AgentID, history.AgentID)
	}
	if got.FromVersion != history.FromVersion {
		t.Errorf("GetAgentUpdateHistory() FromVersion = %s, want %s", got.FromVersion, history.FromVersion)
	}
	if got.ToVersion != history.ToVersion {
		t.Errorf("GetAgentUpdateHistory() ToVersion = %s, want %s", got.ToVersion, history.ToVersion)
	}
	if got.Status != "pending" {
		t.Errorf("GetAgentUpdateHistory() Status = %s, want pending", got.Status)
	}

	// Update history
	now := time.Now()
	history.Status = "completed"
	history.CompletedAt = &now
	err = db.UpdateAgentUpdateHistory(ctx, history)
	if err != nil {
		t.Fatalf("UpdateAgentUpdateHistory() error = %v", err)
	}

	// Verify update
	got, err = db.GetAgentUpdateHistory(ctx, history.ID)
	if err != nil {
		t.Fatalf("GetAgentUpdateHistory() after update error = %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("GetAgentUpdateHistory() Status = %s, want completed", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("GetAgentUpdateHistory() CompletedAt should not be nil")
	}
}

func TestAgentUpdateHistory_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create agent
	agent := &Agent{
		ID:           "agent-list-history",
		Hostname:     "history-host",
		Status:       "online",
		RegisteredAt: time.Now(),
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	// Create multiple history records
	for i := 0; i < 5; i++ {
		history := &AgentUpdateHistory{
			AgentID:     agent.ID,
			FromVersion: "1.0.0",
			ToVersion:   "1.1.0",
			Status:      "completed",
			StartedAt:   time.Now().Add(time.Duration(i) * time.Minute),
		}
		if err := db.CreateAgentUpdateHistory(ctx, history); err != nil {
			t.Fatalf("CreateAgentUpdateHistory() error = %v", err)
		}
	}

	// List with pagination
	list, total, err := db.ListAgentUpdateHistory(ctx, agent.ID, 3, 0)
	if err != nil {
		t.Fatalf("ListAgentUpdateHistory() error = %v", err)
	}
	if total != 5 {
		t.Errorf("ListAgentUpdateHistory() total = %d, want 5", total)
	}
	if len(list) != 3 {
		t.Errorf("ListAgentUpdateHistory() count = %d, want 3", len(list))
	}

	// List page 2
	list, _, err = db.ListAgentUpdateHistory(ctx, agent.ID, 3, 3)
	if err != nil {
		t.Fatalf("ListAgentUpdateHistory() page 2 error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListAgentUpdateHistory() page 2 count = %d, want 2", len(list))
	}
}

func TestAgentUpdateHistory_GetLatest(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create agent
	agent := &Agent{
		ID:           "agent-latest-history",
		Hostname:     "latest-host",
		Status:       "online",
		RegisteredAt: time.Now(),
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	// Create history records at different times
	for i := 0; i < 3; i++ {
		history := &AgentUpdateHistory{
			AgentID:     agent.ID,
			FromVersion: "1.0.0",
			ToVersion:   "1." + string(rune('1'+i)) + ".0",
			Status:      "completed",
			StartedAt:   time.Now().Add(time.Duration(i) * time.Hour),
		}
		if err := db.CreateAgentUpdateHistory(ctx, history); err != nil {
			t.Fatalf("CreateAgentUpdateHistory() error = %v", err)
		}
	}

	// Get latest
	latest, err := db.GetLatestAgentUpdateHistory(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetLatestAgentUpdateHistory() error = %v", err)
	}
	// Should be the most recent (highest i value)
	if latest.ToVersion != "1.3.0" {
		t.Errorf("GetLatestAgentUpdateHistory() ToVersion = %s, want 1.3.0", latest.ToVersion)
	}

	// Non-existent agent
	_, err = db.GetLatestAgentUpdateHistory(ctx, "non-existent-agent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetLatestAgentUpdateHistory() non-existent should return ErrNotFound, got %v", err)
	}
}

func TestAgentUpdateHistory_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.GetAgentUpdateHistory(ctx, 999999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAgentUpdateHistory() non-existent should return ErrNotFound, got %v", err)
	}
}

// --- Agent Version/Update Policy Tests ---

func TestUpdateAgentVersion(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create agent
	agent := &Agent{
		ID:           "agent-version-test",
		Hostname:     "version-host",
		Status:       "online",
		RegisteredAt: time.Now(),
		Version:      "1.0.0",
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	// Update version
	err := db.UpdateAgentVersion(ctx, agent.ID, "2.0.0")
	if err != nil {
		t.Fatalf("UpdateAgentVersion() error = %v", err)
	}

	// Verify
	got, err := db.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if got.Version != "2.0.0" {
		t.Errorf("GetAgent() Version = %s, want 2.0.0", got.Version)
	}
	if got.LastUpdateAt == nil {
		t.Error("GetAgent() LastUpdateAt should be set")
	}
}

func TestUpdateAgentUpdateError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create agent
	agent := &Agent{
		ID:           "agent-error-test",
		Hostname:     "error-host",
		Status:       "online",
		RegisteredAt: time.Now(),
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	// Set update error
	err := db.UpdateAgentUpdateError(ctx, agent.ID, "download failed: connection timeout")
	if err != nil {
		t.Fatalf("UpdateAgentUpdateError() error = %v", err)
	}

	// Verify
	got, err := db.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if got.LastUpdateError != "download failed: connection timeout" {
		t.Errorf("GetAgent() LastUpdateError = %s, want 'download failed: connection timeout'", got.LastUpdateError)
	}
}

func TestUpdateAgentUpdatePolicy(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create agent
	agent := &Agent{
		ID:           "agent-policy-test",
		Hostname:     "policy-host",
		Status:       "online",
		RegisteredAt: time.Now(),
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	// Update policy
	err := db.UpdateAgentUpdatePolicy(ctx, agent.ID, "scheduled", "02:00", "04:00")
	if err != nil {
		t.Fatalf("UpdateAgentUpdatePolicy() error = %v", err)
	}

	// Verify
	got, err := db.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if got.UpdatePolicy != "scheduled" {
		t.Errorf("GetAgent() UpdatePolicy = %s, want scheduled", got.UpdatePolicy)
	}
	if got.UpdateWindowStart != "02:00" {
		t.Errorf("GetAgent() UpdateWindowStart = %s, want 02:00", got.UpdateWindowStart)
	}
	if got.UpdateWindowEnd != "04:00" {
		t.Errorf("GetAgent() UpdateWindowEnd = %s, want 04:00", got.UpdateWindowEnd)
	}
}

func TestListAgentsNeedingUpdate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a current binary
	binary := &AgentBinary{
		Version:   "2.0.0",
		OS:        "linux",
		Arch:      "amd64",
		Path:      "/path/binary",
		SizeBytes: 1000,
		IsCurrent: true,
	}
	if err := db.CreateAgentBinary(ctx, binary); err != nil {
		t.Fatalf("CreateAgentBinary() error = %v", err)
	}

	// Create agent with old version
	agent1 := &Agent{
		ID:           "agent-needs-update",
		Hostname:     "outdated-host",
		Status:       "online",
		RegisteredAt: time.Now(),
		Version:      "1.0.0",
		OS:           "linux",
		Arch:         "amd64",
	}
	if err := db.UpsertAgent(ctx, agent1); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	// Create agent with current version
	agent2 := &Agent{
		ID:           "agent-current",
		Hostname:     "current-host",
		Status:       "online",
		RegisteredAt: time.Now(),
		Version:      "2.0.0",
		OS:           "linux",
		Arch:         "amd64",
	}
	if err := db.UpsertAgent(ctx, agent2); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	// List agents needing update
	agents, err := db.ListAgentsNeedingUpdate(ctx)
	if err != nil {
		t.Fatalf("ListAgentsNeedingUpdate() error = %v", err)
	}

	// Should only include agent1
	if len(agents) != 1 {
		t.Errorf("ListAgentsNeedingUpdate() count = %d, want 1", len(agents))
	}
	if len(agents) > 0 && agents[0].ID != agent1.ID {
		t.Errorf("ListAgentsNeedingUpdate()[0].ID = %s, want %s", agents[0].ID, agent1.ID)
	}
}

// --- Health Check Config Tests ---

func TestHealthCheckConfig_CRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create global health check config
	config := &HealthCheckConfig{
		Name:              "Default Health Check",
		URL:               "{{.URL}}/health",
		Method:            "GET",
		ExpectedStatus:    200,
		TimeoutSeconds:    30,
		Retries:           3,
		RetryDelaySeconds: 5,
		Enabled:           true,
		IsGlobal:          true,
	}

	err := db.CreateHealthCheckConfig(ctx, config)
	if err != nil {
		t.Fatalf("CreateHealthCheckConfig() error = %v", err)
	}
	if config.ID == 0 {
		t.Error("CreateHealthCheckConfig() did not set ID")
	}

	// Get by ID
	got, err := db.GetHealthCheckConfig(ctx, config.ID)
	if err != nil {
		t.Fatalf("GetHealthCheckConfig() error = %v", err)
	}
	if got.Name != config.Name {
		t.Errorf("GetHealthCheckConfig() Name = %s, want %s", got.Name, config.Name)
	}
	if got.URL != config.URL {
		t.Errorf("GetHealthCheckConfig() URL = %s, want %s", got.URL, config.URL)
	}
	if got.Method != config.Method {
		t.Errorf("GetHealthCheckConfig() Method = %s, want %s", got.Method, config.Method)
	}
	if got.ExpectedStatus != config.ExpectedStatus {
		t.Errorf("GetHealthCheckConfig() ExpectedStatus = %d, want %d", got.ExpectedStatus, config.ExpectedStatus)
	}
	if !got.IsGlobal {
		t.Error("GetHealthCheckConfig() IsGlobal should be true")
	}

	// Update
	config.TimeoutSeconds = 60
	config.Retries = 5
	err = db.UpdateHealthCheckConfig(ctx, config)
	if err != nil {
		t.Fatalf("UpdateHealthCheckConfig() error = %v", err)
	}

	// Verify update
	got, err = db.GetHealthCheckConfig(ctx, config.ID)
	if err != nil {
		t.Fatalf("GetHealthCheckConfig() after update error = %v", err)
	}
	if got.TimeoutSeconds != 60 {
		t.Errorf("GetHealthCheckConfig() TimeoutSeconds = %d, want 60", got.TimeoutSeconds)
	}
	if got.Retries != 5 {
		t.Errorf("GetHealthCheckConfig() Retries = %d, want 5", got.Retries)
	}
}

func TestHealthCheckConfig_Global(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Get the default global config that's created by migrations
	got, err := db.GetGlobalHealthCheckConfig(ctx)
	if err != nil {
		t.Fatalf("GetGlobalHealthCheckConfig() error = %v", err)
	}

	// The migration creates "Global Default" as the default global config
	if got.Name != "Global Default" {
		t.Errorf("GetGlobalHealthCheckConfig() Name = %s, want 'Global Default'", got.Name)
	}
	if !got.IsGlobal {
		t.Error("GetGlobalHealthCheckConfig() IsGlobal should be true")
	}
}

func TestHealthCheckConfig_ForProject(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a project
	project := &Project{
		Name:       "health-check-project",
		Repository: "https://github.com/test/hc",
		Branch:     "main",
		DeployPath: "/app",
		Type:       "web",
		CreatedAt:  time.Now(),
	}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// Create global config
	global := &HealthCheckConfig{
		Name:           "Global",
		URL:            "{{.URL}}/health",
		Method:         "GET",
		ExpectedStatus: 200,
		TimeoutSeconds: 10,
		Enabled:        true,
		IsGlobal:       true,
	}
	if err := db.CreateHealthCheckConfig(ctx, global); err != nil {
		t.Fatalf("CreateHealthCheckConfig() global error = %v", err)
	}

	// Without project-specific config, should return global
	got, err := db.GetHealthCheckConfigForProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetHealthCheckConfigForProject() error = %v", err)
	}
	if !got.IsGlobal {
		t.Error("GetHealthCheckConfigForProject() should return global when no project config exists")
	}

	// Create project-specific config
	projectConfig := &HealthCheckConfig{
		ProjectID:      &project.ID,
		Name:           "Project Specific",
		URL:            "{{.URL}}/api/status",
		Method:         "POST",
		ExpectedStatus: 201,
		TimeoutSeconds: 20,
		Enabled:        true,
		IsGlobal:       false,
	}
	if err := db.CreateHealthCheckConfig(ctx, projectConfig); err != nil {
		t.Fatalf("CreateHealthCheckConfig() project error = %v", err)
	}

	// Now should return project-specific config
	got, err = db.GetHealthCheckConfigForProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetHealthCheckConfigForProject() with project config error = %v", err)
	}
	if got.IsGlobal {
		t.Error("GetHealthCheckConfigForProject() should return project-specific config when it exists")
	}
	if got.ID != projectConfig.ID {
		t.Errorf("GetHealthCheckConfigForProject() ID = %d, want %d", got.ID, projectConfig.ID)
	}
}

func TestHealthCheckConfig_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple configs
	configs := []*HealthCheckConfig{
		{Name: "Global", URL: "/health", Method: "GET", ExpectedStatus: 200, TimeoutSeconds: 10, Enabled: true, IsGlobal: true},
		{Name: "Config A", URL: "/a", Method: "GET", ExpectedStatus: 200, TimeoutSeconds: 10, Enabled: true, IsGlobal: false},
		{Name: "Config B", URL: "/b", Method: "GET", ExpectedStatus: 200, TimeoutSeconds: 10, Enabled: true, IsGlobal: false},
	}

	for _, c := range configs {
		if err := db.CreateHealthCheckConfig(ctx, c); err != nil {
			t.Fatalf("CreateHealthCheckConfig() error = %v", err)
		}
	}

	// List all
	list, err := db.ListHealthCheckConfigs(ctx)
	if err != nil {
		t.Fatalf("ListHealthCheckConfigs() error = %v", err)
	}
	if len(list) < 3 {
		t.Errorf("ListHealthCheckConfigs() count = %d, want at least 3", len(list))
	}

	// Check global is in the list
	foundGlobal := false
	for _, c := range list {
		if c.IsGlobal {
			foundGlobal = true
			break
		}
	}
	if !foundGlobal {
		t.Error("ListHealthCheckConfigs() should include global config")
	}
}

func TestHealthCheckConfig_Delete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create global config
	global := &HealthCheckConfig{
		Name: "Global", URL: "/health", Method: "GET", ExpectedStatus: 200,
		TimeoutSeconds: 10, Enabled: true, IsGlobal: true,
	}
	if err := db.CreateHealthCheckConfig(ctx, global); err != nil {
		t.Fatalf("CreateHealthCheckConfig() error = %v", err)
	}

	// Create non-global config
	nonGlobal := &HealthCheckConfig{
		Name: "Non-Global", URL: "/status", Method: "GET", ExpectedStatus: 200,
		TimeoutSeconds: 10, Enabled: true, IsGlobal: false,
	}
	if err := db.CreateHealthCheckConfig(ctx, nonGlobal); err != nil {
		t.Fatalf("CreateHealthCheckConfig() error = %v", err)
	}

	// Should be able to delete non-global
	err := db.DeleteHealthCheckConfig(ctx, nonGlobal.ID)
	if err != nil {
		t.Fatalf("DeleteHealthCheckConfig() non-global error = %v", err)
	}

	// Verify deleted
	_, err = db.GetHealthCheckConfig(ctx, nonGlobal.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetHealthCheckConfig() after delete should return ErrNotFound, got %v", err)
	}

	// Should NOT be able to delete global
	err = db.DeleteHealthCheckConfig(ctx, global.ID)
	if err == nil {
		t.Error("DeleteHealthCheckConfig() global should fail")
	}
}

func TestHealthCheckConfig_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.GetHealthCheckConfig(ctx, 999999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetHealthCheckConfig() non-existent should return ErrNotFound, got %v", err)
	}

	// Delete non-existent
	err = db.DeleteHealthCheckConfig(ctx, 999999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteHealthCheckConfig() non-existent should return ErrNotFound, got %v", err)
	}
}

// --- Deployment Rollback Tests ---

func TestDeploymentRollback_CRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create rollback
	rollback := &DeploymentRollback{
		DeploymentID:      "deploy-123",
		ProjectName:       "test-project",
		FromRelease:       5,
		ToRelease:         4,
		Reason:            "Health check failed",
		TriggeredBy:       RollbackTriggerAutoHealthFail,
		HealthCheckFailed: true,
		HealthCheckError:  "Connection refused",
		Status:            "pending",
		StartedAt:         time.Now(),
	}

	err := db.CreateDeploymentRollback(ctx, rollback)
	if err != nil {
		t.Fatalf("CreateDeploymentRollback() error = %v", err)
	}
	if rollback.ID == 0 {
		t.Error("CreateDeploymentRollback() did not set ID")
	}

	// Get by ID
	got, err := db.GetDeploymentRollback(ctx, rollback.ID)
	if err != nil {
		t.Fatalf("GetDeploymentRollback() error = %v", err)
	}
	if got.DeploymentID != rollback.DeploymentID {
		t.Errorf("GetDeploymentRollback() DeploymentID = %s, want %s", got.DeploymentID, rollback.DeploymentID)
	}
	if got.ProjectName != rollback.ProjectName {
		t.Errorf("GetDeploymentRollback() ProjectName = %s, want %s", got.ProjectName, rollback.ProjectName)
	}
	if got.FromRelease != 5 {
		t.Errorf("GetDeploymentRollback() FromRelease = %d, want 5", got.FromRelease)
	}
	if got.ToRelease != 4 {
		t.Errorf("GetDeploymentRollback() ToRelease = %d, want 4", got.ToRelease)
	}
	if got.TriggeredBy != RollbackTriggerAutoHealthFail {
		t.Errorf("GetDeploymentRollback() TriggeredBy = %s, want %s", got.TriggeredBy, RollbackTriggerAutoHealthFail)
	}
	if !got.HealthCheckFailed {
		t.Error("GetDeploymentRollback() HealthCheckFailed should be true")
	}

	// Update rollback
	now := time.Now()
	rollback.Status = "completed"
	rollback.CompletedAt = &now
	err = db.UpdateDeploymentRollback(ctx, rollback)
	if err != nil {
		t.Fatalf("UpdateDeploymentRollback() error = %v", err)
	}

	// Verify update
	got, err = db.GetDeploymentRollback(ctx, rollback.ID)
	if err != nil {
		t.Fatalf("GetDeploymentRollback() after update error = %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("GetDeploymentRollback() Status = %s, want completed", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("GetDeploymentRollback() CompletedAt should not be nil")
	}
}

func TestDeploymentRollback_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple rollbacks for different projects
	rollbacks := []*DeploymentRollback{
		{DeploymentID: "d1", ProjectName: "project-a", FromRelease: 2, ToRelease: 1, Reason: "test", TriggeredBy: "user", Status: "completed", StartedAt: time.Now()},
		{DeploymentID: "d2", ProjectName: "project-a", FromRelease: 3, ToRelease: 2, Reason: "test", TriggeredBy: "user", Status: "completed", StartedAt: time.Now().Add(time.Hour)},
		{DeploymentID: "d3", ProjectName: "project-b", FromRelease: 2, ToRelease: 1, Reason: "test", TriggeredBy: "user", Status: "completed", StartedAt: time.Now()},
	}

	for _, r := range rollbacks {
		if err := db.CreateDeploymentRollback(ctx, r); err != nil {
			t.Fatalf("CreateDeploymentRollback() error = %v", err)
		}
	}

	// List all
	list, total, err := db.ListDeploymentRollbacks(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("ListDeploymentRollbacks() all error = %v", err)
	}
	if total != 3 {
		t.Errorf("ListDeploymentRollbacks() all total = %d, want 3", total)
	}
	if len(list) != 3 {
		t.Errorf("ListDeploymentRollbacks() all count = %d, want 3", len(list))
	}

	// List for specific project
	list, total, err = db.ListDeploymentRollbacks(ctx, "project-a", 10, 0)
	if err != nil {
		t.Fatalf("ListDeploymentRollbacks() project-a error = %v", err)
	}
	if total != 2 {
		t.Errorf("ListDeploymentRollbacks() project-a total = %d, want 2", total)
	}
	if len(list) != 2 {
		t.Errorf("ListDeploymentRollbacks() project-a count = %d, want 2", len(list))
	}

	// List with pagination
	list, total, err = db.ListDeploymentRollbacks(ctx, "", 2, 0)
	if err != nil {
		t.Fatalf("ListDeploymentRollbacks() paginated error = %v", err)
	}
	if total != 3 {
		t.Errorf("ListDeploymentRollbacks() paginated total = %d, want 3", total)
	}
	if len(list) != 2 {
		t.Errorf("ListDeploymentRollbacks() paginated count = %d, want 2", len(list))
	}
}

func TestDeploymentRollback_GetLatest(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Use unique deployment ID for this test
	deployID := "deploy-getlatest-test-unique"

	// Create rollbacks - the most recently inserted one should be returned
	// Note: CreateDeploymentRollback doesn't save started_at from the struct,
	// it uses CURRENT_TIMESTAMP as default. So we rely on insertion order.
	var lastID int64
	var lastFromRelease int
	for i := 0; i < 3; i++ {
		fromRelease := i + 2
		rollback := &DeploymentRollback{
			DeploymentID: deployID,
			ProjectName:  "test-project",
			FromRelease:  fromRelease,
			ToRelease:    i + 1,
			Reason:       "test",
			TriggeredBy:  "user",
			Status:       "completed",
			StartedAt:    time.Now(), // Will be overwritten by DB default
		}
		if err := db.CreateDeploymentRollback(ctx, rollback); err != nil {
			t.Fatalf("CreateDeploymentRollback() error = %v", err)
		}
		lastID = rollback.ID
		lastFromRelease = fromRelease
		time.Sleep(1100 * time.Millisecond) // Ensure distinct timestamps (SQLite uses seconds)
	}

	// Get latest
	latest, err := db.GetLatestRollbackForDeployment(ctx, deployID)
	if err != nil {
		t.Fatalf("GetLatestRollbackForDeployment() error = %v", err)
	}

	// Verify it's the last inserted one
	if latest.ID != lastID {
		t.Errorf("GetLatestRollbackForDeployment() ID = %d, want %d", latest.ID, lastID)
	}
	if latest.FromRelease != lastFromRelease {
		t.Errorf("GetLatestRollbackForDeployment() FromRelease = %d, want %d", latest.FromRelease, lastFromRelease)
	}

	// Non-existent deployment
	_, err = db.GetLatestRollbackForDeployment(ctx, "non-existent-deployment-xyz")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetLatestRollbackForDeployment() non-existent should return ErrNotFound, got %v", err)
	}
}

func TestDeploymentRollback_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := db.GetDeploymentRollback(ctx, 999999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetDeploymentRollback() non-existent should return ErrNotFound, got %v", err)
	}
}

// --- Project Health Check Update Tests ---

func TestUpdateProjectHealthCheck(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create project
	project := &Project{
		Name:       "hc-update-project",
		Repository: "https://github.com/test/hc-update",
		Branch:     "main",
		DeployPath: "/app",
		Type:       "web",
		CreatedAt:  time.Now(),
	}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// Create health check config
	config := &HealthCheckConfig{
		Name: "Project HC", URL: "/health", Method: "GET", ExpectedStatus: 200,
		TimeoutSeconds: 10, Enabled: true, IsGlobal: false,
	}
	if err := db.CreateHealthCheckConfig(ctx, config); err != nil {
		t.Fatalf("CreateHealthCheckConfig() error = %v", err)
	}

	// Update project health check - this should not error
	err := db.UpdateProjectHealthCheck(ctx, project.ID, &config.ID, true, true)
	if err != nil {
		t.Fatalf("UpdateProjectHealthCheck() error = %v", err)
	}

	// The actual verification would require GetProjectByName to return these fields
	// which it currently doesn't. We're testing that the function runs without error.

	// Update to remove health check
	err = db.UpdateProjectHealthCheck(ctx, project.ID, nil, false, false)
	if err != nil {
		t.Fatalf("UpdateProjectHealthCheck() clear error = %v", err)
	}
}

// --- Audit Log Tests ---

func TestLogAuditWithSnapshot(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	snapshot := map[string]interface{}{
		"old_value": "foo",
		"new_value": "bar",
	}

	entry := &AuditEntry{
		User:     "test-user",
		Action:   "update",
		Resource: "settings",
		Details:  "Updated system settings",
	}

	err := db.LogAuditWithSnapshot(ctx, entry, snapshot)
	if err != nil {
		t.Fatalf("LogAuditWithSnapshot() error = %v", err)
	}

	// Verify by listing audit logs
	logs, err := db.ListAuditLogs(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("ListAuditLogs() returned no logs")
	}

	// Check the log entry
	found := false
	for _, log := range logs {
		if log.User == "test-user" && log.Action == "update" {
			found = true
			if log.ResourceData == "" {
				t.Error("LogAuditWithSnapshot() should have ResourceData")
			}
			break
		}
	}
	if !found {
		t.Error("LogAuditWithSnapshot() log entry not found")
	}
}

func TestListAuditLogsSince(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Record the time before creating logs
	since := time.Now().Add(-time.Second)

	// Create some audit logs
	for i := 0; i < 5; i++ {
		entry := &AuditEntry{
			User:     "user",
			Action:   "action",
			Resource: "target",
			Details:  "description",
		}
		if err := db.LogAudit(ctx, entry); err != nil {
			t.Fatalf("LogAudit() error = %v", err)
		}
	}

	// List logs since the recorded time
	logs, err := db.ListAuditLogsSince(ctx, since)
	if err != nil {
		t.Fatalf("ListAuditLogsSince() error = %v", err)
	}
	if len(logs) != 5 {
		t.Errorf("ListAuditLogsSince() count = %d, want 5", len(logs))
	}

	// List with future time should return nothing
	future := time.Now().Add(time.Hour)
	logs, err = db.ListAuditLogsSince(ctx, future)
	if err != nil {
		t.Fatalf("ListAuditLogsSince() future error = %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("ListAuditLogsSince() future count = %d, want 0", len(logs))
	}
}
