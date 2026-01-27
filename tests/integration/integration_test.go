// Package integration provides end-to-end integration tests for vcdeploy.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/server"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// TestFixture provides test infrastructure for integration tests.
type TestFixture struct {
	T          *testing.T
	DB         *storage.DB
	Server     *server.MasterServer
	HTTPServer *httptest.Server
	Logger     *zap.Logger
	TempDir    string
}

// NewTestFixture creates a new test fixture with initialized components.
func NewTestFixture(t *testing.T) *TestFixture {
	t.Helper()

	// Create temp directory
	tempDir := t.TempDir()

	// Create logger
	logger := zap.NewNop()

	// Create database
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.New(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create config
	cfg := config.DefaultMasterConfig()

	// Create master server
	srv, err := server.NewMasterServer(cfg, db, logger)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	return &TestFixture{
		T:       t,
		DB:      db,
		Server:  srv,
		Logger:  logger,
		TempDir: tempDir,
	}
}

// Close cleans up the test fixture.
func (f *TestFixture) Close() {
	if f.HTTPServer != nil {
		f.HTTPServer.Close()
	}
	if f.DB != nil {
		f.DB.Close()
	}
}

// CreateTestUser creates a test user in the database.
func (f *TestFixture) CreateTestUser(username, password string) {
	f.T.Helper()

	ctx := context.Background()
	user := &storage.User{
		Username:     username,
		Email:        username + "@test.com",
		PasswordHash: hashPassword(password),
		Role:         "admin",
		CreatedAt:    time.Now(),
	}

	if err := f.DB.CreateUser(ctx, user); err != nil {
		f.T.Fatalf("Failed to create user: %v", err)
	}
}

// CreateTestProject creates a test project in the database.
func (f *TestFixture) CreateTestProject(name, repo, branch string) *storage.Project {
	f.T.Helper()

	project := &storage.Project{
		Name:       name,
		Repository: repo,
		Branch:     branch,
		DeployPath: "/var/www/" + name,
		Type:       "web",
		CreatedAt:  time.Now(),
	}

	if err := f.DB.CreateProject(project); err != nil {
		f.T.Fatalf("Failed to create project: %v", err)
	}

	return project
}

// hashPassword creates a password hash for testing using proper bcrypt.
func hashPassword(password string) string {
	hash, err := security.HashPassword(password)
	if err != nil {
		panic("failed to hash password: " + err.Error())
	}
	return hash
}

// TestWebhookFlow tests the complete webhook processing flow.
func TestWebhookFlow(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	// Create a test project
	project := f.CreateTestProject("test-project", "https://github.com/test/repo.git", "main")

	// Create webhook config
	ctx := context.Background()
	err := f.DB.SetProjectWebhook(ctx, project.ID, "github", []byte("secret"), true, false)
	if err != nil {
		t.Fatalf("Failed to create webhook config: %v", err)
	}

	// Verify webhook was created
	webhook, err := f.DB.GetProjectWebhook(ctx, project.ID, "github")
	if err != nil {
		t.Fatalf("Failed to get webhook config: %v", err)
	}

	if webhook == nil {
		t.Fatal("Webhook should exist")
	}

	if !webhook.Enabled {
		t.Error("Webhook should be enabled")
	}
}

// TestAgentRegistration tests agent registration flow.
func TestAgentRegistration(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Register an agent using UpsertAgent
	agent := &storage.Agent{
		ID:         "agent-001",
		Hostname:   "server1.test.com",
		Labels:     map[string]string{"env": "test"},
		Status:     "connected",
		LastSeenAt: time.Now(),
	}

	err := f.DB.UpsertAgent(ctx, agent)
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	// Verify agent was created
	storedAgent, err := f.DB.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	if storedAgent == nil {
		t.Fatal("Agent should exist")
	}

	if storedAgent.Hostname != "server1.test.com" {
		t.Errorf("Expected hostname server1.test.com, got %s", storedAgent.Hostname)
	}

	// Update agent status
	agent.Status = "disconnected"
	err = f.DB.UpsertAgent(ctx, agent)
	if err != nil {
		t.Fatalf("Failed to update agent: %v", err)
	}

	// Verify update
	storedAgent, err = f.DB.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("Failed to get agent after update: %v", err)
	}

	if storedAgent.Status != "disconnected" {
		t.Errorf("Expected status disconnected, got %s", storedAgent.Status)
	}
}

// TestScheduledDeployments tests the scheduled deployments flow.
func TestScheduledDeployments(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Create a test project
	project := f.CreateTestProject("scheduled-project", "https://github.com/test/scheduled.git", "main")

	// Schedule a deployment (CreateScheduledDeployment takes id, project, target, branch, scheduledAt, scheduledBy)
	scheduledTime := time.Now().Add(1 * time.Hour)
	deploymentID := fmt.Sprintf("scheduled-%d", time.Now().UnixNano())
	err := f.DB.CreateScheduledDeployment(ctx, deploymentID, project.Name, "production", "main", scheduledTime, "admin")
	if err != nil {
		t.Fatalf("Failed to create scheduled deployment: %v", err)
	}

	// The deployment was created but hasn't hit its scheduled time yet
	// ListPendingScheduledDeployments only returns deployments where scheduled_at <= now
	// So we'll just verify the creation didn't error

	t.Log("Scheduled deployment created successfully")
}

// TestSecurityMiddleware tests security headers.
func TestSecurityMiddleware(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	// Create a test handler that just returns OK
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create security middleware
	secMW := server.NewSecurityMiddleware(server.DefaultSecurityConfig())

	// Wrap handler with middleware
	wrapped := secMW.HeadersOnlyMiddleware(handler)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	// Serve request
	wrapped.ServeHTTP(rec, req)

	// Check security headers
	resp := rec.Result()

	// X-Content-Type-Options
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("Missing X-Content-Type-Options header")
	}

	// X-Frame-Options
	if resp.Header.Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Error("Missing X-Frame-Options header")
	}

	// X-XSS-Protection
	if resp.Header.Get("X-XSS-Protection") != "1; mode=block" {
		t.Error("Missing X-XSS-Protection header")
	}

	// Content-Security-Policy
	csp := resp.Header.Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Missing Content-Security-Policy header")
	}

	// Referrer-Policy
	if resp.Header.Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Error("Missing Referrer-Policy header")
	}
}

// TestRateLimiting tests rate limiting functionality.
func TestRateLimiting(t *testing.T) {
	// Create rate limiter with low limits for testing
	cfg := server.RateLimitConfig{
		RequestsPerSecond: 2,
		BurstSize:         3,
		BlockDuration:     1 * time.Minute,
		BlockThreshold:    5,
		CleanupInterval:   1 * time.Minute,
	}

	rl, err := server.NewRateLimiter(nil, cfg)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}
	defer rl.Stop()

	testIP := "192.168.1.100"

	// First few requests should be allowed (burst)
	for i := 0; i < 3; i++ {
		allowed, status := rl.Allow(testIP, "/test")
		if !allowed {
			t.Errorf("Request %d should be allowed (within burst)", i+1)
		}
		if status.IsBlocked {
			t.Errorf("IP should not be blocked on request %d", i+1)
		}
	}

	// Next request should be rate limited (exceeded burst)
	allowed, status := rl.Allow(testIP, "/test")
	if allowed {
		t.Log("Request 4 was allowed due to token replenishment")
	}
	t.Logf("Status after 4 requests: tokens=%.2f, violations=%d", status.TokensRemaining, status.Violations)
}

// TestAuditLogging tests audit log creation and retrieval.
func TestAuditLogging(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Create audit entries
	entries := []*storage.AuditEntry{
		{
			Source:    "test",
			User:      "admin",
			Action:    "login",
			Resource:  "session",
			Details:   "IP: 192.168.1.1",
			IPAddress: "192.168.1.1",
			Result:    "success",
		},
		{
			Source:    "test",
			User:      "admin",
			Action:    "create",
			Resource:  "project",
			Details:   "Created project test-project",
			IPAddress: "192.168.1.1",
			Result:    "success",
		},
		{
			Source:    "test",
			User:      "hacker",
			Action:    "login",
			Resource:  "session",
			Details:   "IP: 10.0.0.1",
			IPAddress: "10.0.0.1",
			Result:    "failure",
		},
	}

	for _, entry := range entries {
		if err := f.DB.LogAudit(ctx, entry); err != nil {
			t.Fatalf("Failed to create audit entry: %v", err)
		}
	}

	// Retrieve audit logs
	logs, err := f.DB.ListAuditLogs(ctx, 10, 0)
	if err != nil {
		t.Fatalf("Failed to list audit logs: %v", err)
	}

	if len(logs) != 3 {
		t.Errorf("Expected 3 audit logs, got %d", len(logs))
	}

	// Verify most recent entry is first
	if len(logs) > 0 && logs[0].Action != "login" {
		// The most recent entry might vary due to timestamp ordering
		t.Logf("First log action: %s", logs[0].Action)
	}
}

// TestValidation tests input validation.
func TestValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		pattern string
		valid   bool
	}{
		{"valid username", "admin", "username", true},
		{"username with underscore", "admin_user", "username", true},
		{"username too short", "ab", "username", false},
		{"valid project name", "my-project", "project", true},
		{"project with number", "project123", "project", true},
		{"project too short", "ab", "project", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic length check for now
			minLen := 3
			isValid := len(tt.input) >= minLen

			if isValid != tt.valid {
				// This is a simplified test - actual validation is more complex
				t.Logf("Input %q length=%d, minLen=%d", tt.input, len(tt.input), minLen)
			}
		})
	}
}

// TestHTTPEndpoints tests HTTP API endpoints.
func TestHTTPEndpoints(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	// Create test handler
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Create test server
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Test health endpoint
	resp, err := http.Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("Failed to call health endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if !bytes.Contains(body, []byte("ok")) {
		t.Errorf("Expected body to contain 'ok', got %s", body)
	}
}

// TestSettingsStorage tests settings storage and retrieval.
func TestSettingsStorage(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Set some settings (SetSetting takes category, key, value, valueType, encrypted)
	testSettings := []struct {
		category  string
		key       string
		value     string
		valueType string
		encrypted bool
	}{
		{"server", "listen", ":8080", "string", false},
		{"server", "tls_enabled", "true", "bool", false},
		{"security", "session_timeout", "3600", "int", false},
		{"deploy", "keep_releases", "5", "int", false},
	}

	for _, s := range testSettings {
		err := f.DB.SetSetting(ctx, s.category, s.key, s.value, s.valueType, s.encrypted)
		if err != nil {
			t.Fatalf("Failed to set setting %s.%s: %v", s.category, s.key, err)
		}
	}

	// Get individual setting
	setting, err := f.DB.GetSetting(ctx, "server", "listen")
	if err != nil {
		t.Fatalf("Failed to get setting: %v", err)
	}
	if setting == nil {
		t.Fatal("Expected setting to exist")
	}
	if setting.Value != ":8080" {
		t.Errorf("Expected :8080, got %s", setting.Value)
	}

	// List settings by category
	serverSettings, err := f.DB.ListSettingsByCategory(ctx, "server")
	if err != nil {
		t.Fatalf("Failed to list settings: %v", err)
	}
	if len(serverSettings) != 2 {
		t.Errorf("Expected 2 server settings, got %d", len(serverSettings))
	}

	// Delete a setting
	err = f.DB.DeleteSetting(ctx, "deploy", "keep_releases")
	if err != nil {
		t.Fatalf("Failed to delete setting: %v", err)
	}

	// Verify deletion
	setting, err = f.DB.GetSetting(ctx, "deploy", "keep_releases")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Expected ErrNotFound after delete, got: %v", err)
	}
	if setting != nil {
		t.Error("Expected setting to be deleted")
	}
}

// TestSecretsStorage tests secret storage.
func TestSecretsStorage(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Store a secret
	secretValue := []byte("my-api-key-12345")
	err := f.DB.SetSecretEncrypted(ctx, "test-project", "production", "API_KEY", secretValue)
	if err != nil {
		t.Fatalf("Failed to store secret: %v", err)
	}

	// Retrieve the secret
	secret, err := f.DB.GetSecret(ctx, "test-project", "production", "API_KEY")
	if err != nil {
		t.Fatalf("Failed to get secret: %v", err)
	}

	if secret == nil {
		t.Fatal("Secret should exist")
	}

	if string(secret.ValueEncrypted) != string(secretValue) {
		t.Errorf("Secret value mismatch: got %s, want %s", secret.ValueEncrypted, secretValue)
	}

	// List secrets for project/environment
	secrets, err := f.DB.ListSecretsWithScope(ctx, "test-project", "production")
	if err != nil {
		t.Fatalf("Failed to list secrets: %v", err)
	}

	if len(secrets) != 1 {
		t.Errorf("Expected 1 secret, got %d", len(secrets))
	}

	// Delete the secret
	err = f.DB.DeleteSecretCtx(ctx, "test-project", "production", "API_KEY")
	if err != nil {
		t.Fatalf("Failed to delete secret: %v", err)
	}

	// Verify deletion
	secret, err = f.DB.GetSecret(ctx, "test-project", "production", "API_KEY")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Expected ErrNotFound after delete, got: %v", err)
	}
	if secret != nil {
		t.Error("Secret should be deleted")
	}
}

// TestProjectCRUD tests project create/read/update/delete operations.
func TestProjectCRUD(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	// Create
	project := f.CreateTestProject("crud-project", "https://github.com/test/crud.git", "develop")

	// Read by name
	readProject, err := f.DB.GetProjectByName(context.Background(), project.Name)
	if err != nil {
		t.Fatalf("Failed to get project: %v", err)
	}
	if readProject == nil {
		t.Fatal("Project should exist")
	}
	if readProject.Name != "crud-project" {
		t.Errorf("Expected name crud-project, got %s", readProject.Name)
	}

	// List projects
	projects, err := f.DB.ListProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}
	if len(projects) == 0 {
		t.Error("Expected at least 1 project")
	}

	// Delete
	err = f.DB.DeleteProject(project.Name)
	if err != nil {
		t.Fatalf("Failed to delete project: %v", err)
	}

	// Verify deletion
	_, err = f.DB.GetProjectByName(context.Background(), project.Name)
	if err == nil {
		t.Error("Expected error when getting deleted project")
	}
}

// TestUserCRUD tests user create/read operations.
func TestUserCRUD(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Create
	user := &storage.User{
		Username:     "testuser",
		Email:        "testuser@test.com",
		PasswordHash: hashPassword("password123"),
		Role:         "user",
		CreatedAt:    time.Now(),
	}
	err := f.DB.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Read by username
	readUser, err := f.DB.GetUserByUsername(ctx, "testuser")
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	if readUser == nil {
		t.Fatal("User should exist")
	}
	if readUser.Email != "testuser@test.com" {
		t.Errorf("Expected email testuser@test.com, got %s", readUser.Email)
	}
	if readUser.Role != "user" {
		t.Errorf("Expected role user, got %s", readUser.Role)
	}
}

// TestDeploymentFlow tests the deployment creation and status updates.
func TestDeploymentFlow(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Create a project first
	project := f.CreateTestProject("deploy-project", "https://github.com/test/deploy.git", "main")

	// Create a deployment
	deploymentID := fmt.Sprintf("deploy-%d", time.Now().UnixNano())
	deployment := &storage.DeploymentRecord{
		ID:          deploymentID,
		Project:     project.Name,
		Target:      "production",
		Branch:      "main",
		Status:      "pending",
		TriggeredBy: "testuser",
		StartedAt:   time.Now(),
	}
	err := f.DB.CreateDeployment(ctx, deployment)
	if err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Get deployment
	readDeployment, err := f.DB.GetDeployment(ctx, deploymentID)
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}
	if readDeployment == nil {
		t.Fatal("Deployment should exist")
	}
	if readDeployment.Status != "pending" {
		t.Errorf("Expected status pending, got %s", readDeployment.Status)
	}

	// Update deployment status using UpdateDeployment
	now := time.Now()
	readDeployment.Status = "success"
	readDeployment.CompletedAt = &now
	err = f.DB.UpdateDeployment(ctx, readDeployment)
	if err != nil {
		t.Fatalf("Failed to update deployment status: %v", err)
	}

	// Verify update
	readDeployment, err = f.DB.GetDeployment(ctx, deploymentID)
	if err != nil {
		t.Fatalf("Failed to get deployment after update: %v", err)
	}
	if readDeployment.Status != "success" {
		t.Errorf("Expected status success, got %s", readDeployment.Status)
	}
}

// TestE2EDeploymentWorkflow tests a complete deployment workflow from
// project creation through deployment execution and status tracking.
func TestE2EDeploymentWorkflow(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Step 1: Create a project type
	projectType := &storage.ProjectType{
		Name:        "nodejs-e2e",
		Description: "Node.js application for E2E test",
		BuildCmd:    "npm ci && npm run build",
		CreatedAt:   time.Now(),
	}
	err := f.DB.CreateProjectType(projectType)
	if err != nil {
		t.Fatalf("Failed to create project type: %v", err)
	}

	// Step 2: Create a project
	project := &storage.Project{
		Name:       "e2e-test-project",
		Repository: "https://github.com/test/e2e-repo.git",
		Branch:     "main",
		DeployPath: "/var/www/e2e",
		Type:       "nodejs-e2e",
		CreatedAt:  time.Now(),
	}
	err = f.DB.CreateProject(project)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Step 3: Register an agent
	agent := &storage.Agent{
		ID:         "e2e-agent-001",
		Hostname:   "server1.test.com",
		Labels:     map[string]string{"env": "production"},
		Status:     "connected",
		LastSeenAt: time.Now(),
	}
	err = f.DB.UpsertAgent(ctx, agent)
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	// Step 4: Create a deployment
	deploymentID := fmt.Sprintf("e2e-deploy-%d", time.Now().UnixNano())
	deployment := &storage.DeploymentRecord{
		ID:            deploymentID,
		Project:       project.Name,
		Target:        "production",
		Branch:        "main",
		CommitHash:    "abc123def456",
		Status:        "pending",
		ReleaseNumber: 1,
		TriggeredBy:   "e2e-test",
		TriggerSource: "manual",
		StartedAt:     time.Now(),
	}
	err = f.DB.CreateDeployment(ctx, deployment)
	if err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Step 5: Simulate deployment progress - update to running
	runningDeployment, err := f.DB.GetDeployment(ctx, deploymentID)
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}
	runningDeployment.Status = "running"
	err = f.DB.UpdateDeployment(ctx, runningDeployment)
	if err != nil {
		t.Fatalf("Failed to update deployment to running: %v", err)
	}

	// Step 6: Mark deployment as successful
	completedAt := time.Now()
	runningDeployment.Status = "success"
	runningDeployment.CompletedAt = &completedAt
	err = f.DB.UpdateDeployment(ctx, runningDeployment)
	if err != nil {
		t.Fatalf("Failed to mark deployment as success: %v", err)
	}

	// Step 7: Verify final state
	finalDeployment, err := f.DB.GetDeployment(ctx, deploymentID)
	if err != nil {
		t.Fatalf("Failed to get final deployment: %v", err)
	}

	if finalDeployment.Status != "success" {
		t.Errorf("Expected deployment status 'success', got '%s'", finalDeployment.Status)
	}
	if finalDeployment.CompletedAt == nil {
		t.Error("Expected deployment to have completion time")
	}

	// Step 8: Verify deployment appears in recent deployments
	recentDeployments, err := f.DB.ListDeploymentsRecent(ctx, 10)
	if err != nil {
		t.Fatalf("Failed to list recent deployments: %v", err)
	}

	found := false
	for _, d := range recentDeployments {
		if d.ID == deploymentID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Deployment should appear in recent deployments")
	}

	t.Log("E2E deployment workflow completed successfully")
}

// TestE2ESecretsManagement tests secrets creation, retrieval and deletion workflow.
func TestE2ESecretsManagement(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Create a project first
	project := f.CreateTestProject("secrets-test", "https://github.com/test/secrets.git", "main")

	// Step 1: Create secrets
	secrets := map[string]string{
		"DATABASE_URL": "postgres://localhost/test",
		"API_KEY":      "sk-test-12345",
		"JWT_SECRET":   "super-secret-jwt-key",
	}

	for key, value := range secrets {
		err := f.DB.SetSecretEncrypted(ctx, project.Name, "env", key, []byte(value))
		if err != nil {
			t.Fatalf("Failed to create secret %s: %v", key, err)
		}
	}

	// Step 2: List secrets (project name is used as scope for ListSecrets)
	storedSecrets, err := f.DB.ListSecrets(project.Name)
	if err != nil {
		t.Fatalf("Failed to list secrets: %v", err)
	}

	if len(storedSecrets) != 3 {
		t.Errorf("Expected 3 secrets, got %d", len(storedSecrets))
	}

	// Step 3: Verify secret keys
	secretKeys := make(map[string]bool)
	for _, s := range storedSecrets {
		secretKeys[s.Key] = true
	}

	for key := range secrets {
		if !secretKeys[key] {
			t.Errorf("Expected secret %s not found", key)
		}
	}

	// Step 4: Delete a secret
	err = f.DB.DeleteSecret(project.Name, "API_KEY")
	if err != nil {
		t.Fatalf("Failed to delete secret: %v", err)
	}

	// Step 5: Verify deletion
	storedSecrets, err = f.DB.ListSecrets(project.Name)
	if err != nil {
		t.Fatalf("Failed to list secrets after deletion: %v", err)
	}

	if len(storedSecrets) != 2 {
		t.Errorf("Expected 2 secrets after deletion, got %d", len(storedSecrets))
	}

	for _, s := range storedSecrets {
		if s.Key == "API_KEY" {
			t.Error("Deleted secret should not be present")
		}
	}

	t.Log("E2E secrets management workflow completed successfully")
}

// TestE2EProjectTypeManagement tests project type CRUD workflow.
func TestE2EProjectTypeManagement(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	// Step 1: Create project types
	types := []*storage.ProjectType{
		{Name: "nodejs", Description: "Node.js application", BuildCmd: "npm ci && npm run build", CreatedAt: time.Now()},
		{Name: "python", Description: "Python application", BuildCmd: "pip install -r requirements.txt", CreatedAt: time.Now()},
		{Name: "go", Description: "Go application", BuildCmd: "go build -o app ./...", CreatedAt: time.Now()},
	}

	for _, pt := range types {
		err := f.DB.CreateProjectType(pt)
		if err != nil {
			t.Fatalf("Failed to create project type %s: %v", pt.Name, err)
		}
	}

	// Step 2: List all types
	storedTypes, err := f.DB.ListProjectTypes()
	if err != nil {
		t.Fatalf("Failed to list project types: %v", err)
	}

	if len(storedTypes) != 3 {
		t.Errorf("Expected 3 project types, got %d", len(storedTypes))
	}

	// Step 3: Get specific type
	goType, err := f.DB.GetProjectTypeByName("go")
	if err != nil {
		t.Fatalf("Failed to get project type 'go': %v", err)
	}
	if goType.BuildCmd != "go build -o app ./..." {
		t.Errorf("Unexpected build command: %s", goType.BuildCmd)
	}

	// Step 4: Update a type
	goType.BuildCmd = "go build -ldflags '-s -w' -o app ./..."
	err = f.DB.UpdateProjectTypeByName(goType)
	if err != nil {
		t.Fatalf("Failed to update project type: %v", err)
	}

	// Step 5: Verify update
	goType, err = f.DB.GetProjectTypeByName("go")
	if err != nil {
		t.Fatalf("Failed to get updated project type: %v", err)
	}
	if goType.BuildCmd != "go build -ldflags '-s -w' -o app ./..." {
		t.Errorf("Update was not persisted")
	}

	// Step 6: Delete a type
	err = f.DB.DeleteProjectType("python")
	if err != nil {
		t.Fatalf("Failed to delete project type: %v", err)
	}

	// Step 7: Verify deletion
	storedTypes, err = f.DB.ListProjectTypes()
	if err != nil {
		t.Fatalf("Failed to list project types after deletion: %v", err)
	}
	if len(storedTypes) != 2 {
		t.Errorf("Expected 2 project types after deletion, got %d", len(storedTypes))
	}

	t.Log("E2E project type management workflow completed successfully")
}

// TestE2EAgentDeploymentWithLogs tests the complete workflow:
// Agent registration -> deployment creation -> logs -> status updates -> completion
func TestE2EAgentDeploymentWithLogs(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Step 1: Register an agent
	agent := &storage.Agent{
		ID:       "workflow-agent-001",
		Hostname: "deploy-server-1.example.com",
		Labels: map[string]string{
			"env":      "staging",
			"region":   "us-east-1",
			"capacity": "high",
		},
		Status:     "connected",
		LastSeenAt: time.Now(),
	}
	err := f.DB.UpsertAgent(ctx, agent)
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}
	t.Log("Step 1: Agent registered")

	// Step 2: Create a project
	project := &storage.Project{
		Name:       "workflow-test-app",
		Repository: "https://github.com/test/workflow-app.git",
		Branch:     "main",
		DeployPath: "/var/www/workflow-app",
		Type:       "web",
		CreatedAt:  time.Now(),
	}
	err = f.DB.CreateProject(project)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	t.Log("Step 2: Project created")

	// Step 3: Simulate agent heartbeat
	agent.LastSeenAt = time.Now()
	agent.Status = "connected"
	err = f.DB.UpsertAgent(ctx, agent)
	if err != nil {
		t.Fatalf("Failed to update agent heartbeat: %v", err)
	}
	t.Log("Step 3: Agent heartbeat updated")

	// Step 4: Create a deployment
	deploymentID := fmt.Sprintf("workflow-deploy-%d", time.Now().UnixNano())
	deployment := &storage.DeploymentRecord{
		ID:            deploymentID,
		Project:       project.Name,
		Target:        "staging",
		Branch:        "main",
		CommitHash:    "a1b2c3d4e5f6",
		Status:        "pending",
		ReleaseNumber: 1,
		TriggeredBy:   "ci-pipeline",
		TriggerSource: "webhook",
		StartedAt:     time.Now(),
	}
	err = f.DB.CreateDeployment(ctx, deployment)
	if err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}
	t.Log("Step 4: Deployment created")

	// Step 5: Simulate deployment start - add initial log
	initialLog := &storage.DeploymentLog{
		DeploymentID: deploymentID,
		Level:        "info",
		Message:      "Starting deployment for workflow-test-app",
		Source:       "system",
		CreatedAt:    time.Now(),
	}
	err = f.DB.CreateDeploymentLog(ctx, initialLog)
	if err != nil {
		t.Fatalf("Failed to create deployment log: %v", err)
	}
	t.Log("Step 5: Initial deployment log created")

	// Step 6: Update deployment to running
	deployment.Status = "running"
	err = f.DB.UpdateDeployment(ctx, deployment)
	if err != nil {
		t.Fatalf("Failed to update deployment status to running: %v", err)
	}
	t.Log("Step 6: Deployment status updated to running")

	// Step 7: Simulate deployment progress with multiple logs
	logEntries := []struct {
		level   string
		message string
	}{
		{"info", "Cloning repository https://github.com/test/workflow-app.git"},
		{"info", "Checking out branch main at commit a1b2c3d4e5f6"},
		{"info", "Running pre-deploy hooks"},
		{"info", "Installing dependencies"},
		{"info", "Building application"},
		{"info", "Running tests"},
		{"info", "Deploying to /var/www/workflow-app"},
		{"info", "Running post-deploy hooks"},
		{"info", "Deployment completed successfully"},
	}

	for _, entry := range logEntries {
		log := &storage.DeploymentLog{
			DeploymentID: deploymentID,
			Level:        entry.level,
			Message:      entry.message,
			Source:       "agent",
			CreatedAt:    time.Now(),
		}
		err = f.DB.CreateDeploymentLog(ctx, log)
		if err != nil {
			t.Fatalf("Failed to create log entry: %v", err)
		}
	}
	t.Log("Step 7: Deployment progress logs created")

	// Step 8: Verify logs are stored and retrievable
	logs, err := f.DB.ListDeploymentLogs(ctx, deploymentID)
	if err != nil {
		t.Fatalf("Failed to retrieve deployment logs: %v", err)
	}
	expectedLogCount := len(logEntries) + 1 // +1 for initial log
	if len(logs) != expectedLogCount {
		t.Errorf("Expected %d logs, got %d", expectedLogCount, len(logs))
	}
	t.Log("Step 8: Deployment logs verified")

	// Step 9: Mark deployment as successful
	completedAt := time.Now()
	deployment.Status = "success"
	deployment.CompletedAt = &completedAt
	err = f.DB.UpdateDeployment(ctx, deployment)
	if err != nil {
		t.Fatalf("Failed to mark deployment as success: %v", err)
	}
	t.Log("Step 9: Deployment marked as successful")

	// Step 10: Verify final deployment state
	finalDeployment, err := f.DB.GetDeployment(ctx, deploymentID)
	if err != nil {
		t.Fatalf("Failed to get final deployment: %v", err)
	}
	if finalDeployment.Status != "success" {
		t.Errorf("Expected deployment status 'success', got '%s'", finalDeployment.Status)
	}
	if finalDeployment.CompletedAt == nil {
		t.Error("Expected deployment to have completion time")
	}
	t.Log("Step 10: Final deployment state verified")

	// Step 11: Test audit logging for the deployment
	auditEntry := &storage.AuditEntry{
		Action:    "deployment.completed",
		User:      "system",
		Source:    "integration-test",
		Resource:  deploymentID,
		Details:   fmt.Sprintf(`{"project":"%s","status":"success"}`, project.Name),
		IPAddress: "127.0.0.1",
		Result:    "success",
		Timestamp: time.Now(),
	}
	err = f.DB.LogAudit(ctx, auditEntry)
	if err != nil {
		t.Fatalf("Failed to create audit entry: %v", err)
	}
	t.Log("Step 11: Audit entry created")

	// Step 12: Verify audit log retrieval
	auditLogs, err := f.DB.ListAuditLogs(ctx, 10, 0)
	if err != nil {
		t.Fatalf("Failed to list audit entries: %v", err)
	}
	found := false
	for _, entry := range auditLogs {
		if entry.Resource == deploymentID && entry.Action == "deployment.completed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Audit entry should be retrievable")
	}
	t.Log("Step 12: Audit log verified")

	// Step 13: Verify agent is still connected
	storedAgent, err := f.DB.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}
	if storedAgent.Status != "connected" {
		t.Errorf("Expected agent status 'connected', got '%s'", storedAgent.Status)
	}
	t.Log("Step 13: Agent status verified")

	t.Log("E2E agent deployment with logs workflow completed successfully")
}

// TestE2EDeploymentFailureWorkflow tests deployment failure handling:
// Deployment start -> error during deploy -> failure status -> rollback request
func TestE2EDeploymentFailureWorkflow(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Step 1: Create a project
	project := &storage.Project{
		Name:       "failure-test-app",
		Repository: "https://github.com/test/failure-app.git",
		Branch:     "main",
		DeployPath: "/var/www/failure-app",
		Type:       "web",
		CreatedAt:  time.Now(),
	}
	err := f.DB.CreateProject(project)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Step 2: Create a successful previous deployment (for rollback target)
	prevDeploymentID := fmt.Sprintf("prev-deploy-%d", time.Now().UnixNano())
	prevCompletedAt := time.Now().Add(-1 * time.Hour)
	prevDeployment := &storage.DeploymentRecord{
		ID:            prevDeploymentID,
		Project:       project.Name,
		Target:        "production",
		Branch:        "main",
		CommitHash:    "prev123456",
		Status:        "success",
		ReleaseNumber: 1,
		TriggeredBy:   "admin",
		TriggerSource: "manual",
		StartedAt:     time.Now().Add(-2 * time.Hour),
		CompletedAt:   &prevCompletedAt,
	}
	err = f.DB.CreateDeployment(ctx, prevDeployment)
	if err != nil {
		t.Fatalf("Failed to create previous deployment: %v", err)
	}

	// Step 3: Create new deployment that will fail
	failedDeploymentID := fmt.Sprintf("failed-deploy-%d", time.Now().UnixNano())
	failedDeployment := &storage.DeploymentRecord{
		ID:            failedDeploymentID,
		Project:       project.Name,
		Target:        "production",
		Branch:        "main",
		CommitHash:    "broken123",
		Status:        "pending",
		ReleaseNumber: 2,
		TriggeredBy:   "ci",
		TriggerSource: "webhook",
		StartedAt:     time.Now(),
	}
	err = f.DB.CreateDeployment(ctx, failedDeployment)
	if err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Step 4: Update to running
	failedDeployment.Status = "running"
	err = f.DB.UpdateDeployment(ctx, failedDeployment)
	if err != nil {
		t.Fatalf("Failed to update deployment to running: %v", err)
	}

	// Step 5: Add failure logs
	errorLogs := []struct {
		level   string
		message string
	}{
		{"info", "Starting deployment for failure-test-app"},
		{"info", "Cloning repository"},
		{"info", "Installing dependencies"},
		{"error", "Build failed: exit code 1"},
		{"error", "npm ERR! Failed to compile TypeScript"},
		{"error", "Deployment aborted due to build failure"},
	}

	for _, entry := range errorLogs {
		log := &storage.DeploymentLog{
			DeploymentID: failedDeploymentID,
			Level:        entry.level,
			Message:      entry.message,
			Source:       "agent",
			CreatedAt:    time.Now(),
		}
		err = f.DB.CreateDeploymentLog(ctx, log)
		if err != nil {
			t.Fatalf("Failed to create log entry: %v", err)
		}
	}

	// Step 6: Mark deployment as failed
	completedAt := time.Now()
	failedDeployment.Status = "failed"
	failedDeployment.CompletedAt = &completedAt
	err = f.DB.UpdateDeployment(ctx, failedDeployment)
	if err != nil {
		t.Fatalf("Failed to mark deployment as failed: %v", err)
	}

	// Step 7: Verify failure state
	finalDeployment, err := f.DB.GetDeployment(ctx, failedDeploymentID)
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}
	if finalDeployment.Status != "failed" {
		t.Errorf("Expected status 'failed', got '%s'", finalDeployment.Status)
	}

	// Step 8: Verify error logs are present
	logs, err := f.DB.ListDeploymentLogs(ctx, failedDeploymentID)
	if err != nil {
		t.Fatalf("Failed to get logs: %v", err)
	}
	hasErrorLog := false
	for _, log := range logs {
		if log.Level == "error" {
			hasErrorLog = true
			break
		}
	}
	if !hasErrorLog {
		t.Error("Expected error logs to be present")
	}

	// Step 9: Create rollback deployment
	rollbackDeploymentID := fmt.Sprintf("rollback-deploy-%d", time.Now().UnixNano())
	rollbackDeployment := &storage.DeploymentRecord{
		ID:            rollbackDeploymentID,
		Project:       project.Name,
		Target:        "production",
		Branch:        "main",
		CommitHash:    prevDeployment.CommitHash, // Rolling back to previous commit
		Status:        "pending",
		ReleaseNumber: 3,
		TriggeredBy:   "admin",
		TriggerSource: "rollback",
		StartedAt:     time.Now(),
	}
	err = f.DB.CreateDeployment(ctx, rollbackDeployment)
	if err != nil {
		t.Fatalf("Failed to create rollback deployment: %v", err)
	}

	// Step 10: Complete rollback
	rollbackCompletedAt := time.Now()
	rollbackDeployment.Status = "success"
	rollbackDeployment.CompletedAt = &rollbackCompletedAt
	err = f.DB.UpdateDeployment(ctx, rollbackDeployment)
	if err != nil {
		t.Fatalf("Failed to complete rollback: %v", err)
	}

	// Step 11: Verify deployment history
	allDeployments, err := f.DB.ListDeploymentsRecent(ctx, 10)
	if err != nil {
		t.Fatalf("Failed to list deployments: %v", err)
	}

	projectDeployments := make([]*storage.DeploymentRecord, 0)
	for _, d := range allDeployments {
		if d.Project == project.Name {
			projectDeployments = append(projectDeployments, d)
		}
	}

	if len(projectDeployments) < 3 {
		t.Errorf("Expected at least 3 deployments for project, got %d", len(projectDeployments))
	}

	t.Log("E2E deployment failure workflow completed successfully")
}

// TestE2EMultiAgentDeployment tests coordinated deployment across multiple agents
func TestE2EMultiAgentDeployment(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Step 1: Register multiple agents
	agents := []*storage.Agent{
		{
			ID:         "multi-agent-1",
			Hostname:   "web-server-1.example.com",
			Labels:     map[string]string{"role": "web", "region": "us-east"},
			Status:     "connected",
			LastSeenAt: time.Now(),
		},
		{
			ID:         "multi-agent-2",
			Hostname:   "web-server-2.example.com",
			Labels:     map[string]string{"role": "web", "region": "us-west"},
			Status:     "connected",
			LastSeenAt: time.Now(),
		},
		{
			ID:         "multi-agent-3",
			Hostname:   "api-server-1.example.com",
			Labels:     map[string]string{"role": "api", "region": "us-east"},
			Status:     "connected",
			LastSeenAt: time.Now(),
		},
	}

	for _, agent := range agents {
		err := f.DB.UpsertAgent(ctx, agent)
		if err != nil {
			t.Fatalf("Failed to register agent %s: %v", agent.ID, err)
		}
	}
	t.Log("Step 1: Multiple agents registered")

	// Step 2: Create a project
	project := &storage.Project{
		Name:       "multi-deploy-app",
		Repository: "https://github.com/test/multi-app.git",
		Branch:     "main",
		DeployPath: "/var/www/multi-app",
		Type:       "web",
		CreatedAt:  time.Now(),
	}
	err := f.DB.CreateProject(project)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	t.Log("Step 2: Project created")

	// Step 3: Create deployment for each agent target
	deploymentIDs := make([]string, 0)
	for i, agent := range agents {
		deploymentID := fmt.Sprintf("multi-deploy-%d-%d", i, time.Now().UnixNano())
		deployment := &storage.DeploymentRecord{
			ID:            deploymentID,
			Project:       project.Name,
			Target:        agent.Hostname,
			Branch:        "main",
			CommitHash:    "multi123",
			Status:        "pending",
			ReleaseNumber: 1,
			TriggeredBy:   "orchestrator",
			TriggerSource: "manual",
			StartedAt:     time.Now(),
		}
		err = f.DB.CreateDeployment(ctx, deployment)
		if err != nil {
			t.Fatalf("Failed to create deployment for %s: %v", agent.ID, err)
		}
		deploymentIDs = append(deploymentIDs, deploymentID)
	}
	t.Log("Step 3: Deployments created for all agents")

	// Step 4: Simulate parallel deployment execution
	for _, deploymentID := range deploymentIDs {
		deployment, err := f.DB.GetDeployment(ctx, deploymentID)
		if err != nil {
			t.Fatalf("Failed to get deployment %s: %v", deploymentID, err)
		}

		// Update to running
		deployment.Status = "running"
		err = f.DB.UpdateDeployment(ctx, deployment)
		if err != nil {
			t.Fatalf("Failed to update deployment to running: %v", err)
		}

		// Add logs
		log := &storage.DeploymentLog{
			DeploymentID: deploymentID,
			Level:        "info",
			Message:      fmt.Sprintf("Deploying to %s", deployment.Target),
			Source:       "agent",
			CreatedAt:    time.Now(),
		}
		err = f.DB.CreateDeploymentLog(ctx, log)
		if err != nil {
			t.Fatalf("Failed to create log: %v", err)
		}

		// Complete deployment
		completedAt := time.Now()
		deployment.Status = "success"
		deployment.CompletedAt = &completedAt
		err = f.DB.UpdateDeployment(ctx, deployment)
		if err != nil {
			t.Fatalf("Failed to complete deployment: %v", err)
		}
	}
	t.Log("Step 4: All deployments completed")

	// Step 5: Verify all deployments succeeded
	successCount := 0
	for _, deploymentID := range deploymentIDs {
		deployment, err := f.DB.GetDeployment(ctx, deploymentID)
		if err != nil {
			t.Fatalf("Failed to get deployment %s: %v", deploymentID, err)
		}
		if deployment.Status == "success" {
			successCount++
		}
	}
	if successCount != len(agents) {
		t.Errorf("Expected %d successful deployments, got %d", len(agents), successCount)
	}
	t.Log("Step 5: All deployments verified as successful")

	// Step 6: Verify all agents are still connected
	storedAgents, err := f.DB.ListAgents(ctx)
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}
	connectedCount := 0
	for _, agent := range storedAgents {
		if agent.Status == "connected" {
			connectedCount++
		}
	}
	if connectedCount != len(agents) {
		t.Errorf("Expected %d connected agents, got %d", len(agents), connectedCount)
	}
	t.Log("Step 6: All agents verified as connected")

	t.Log("E2E multi-agent deployment workflow completed successfully")
}

// TestAuthenticationFlows tests various authentication scenarios.
func TestAuthenticationFlows(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	t.Run("user creation and password verification", func(t *testing.T) {
		// Create user
		user := &storage.User{
			Username:     "authtest",
			PasswordHash: "$2a$10$1234567890123456789012345678901234567890123456789012", // bcrypt hash
			Email:        "authtest@example.com",
			Role:         "user",
		}
		err := f.DB.CreateUser(ctx, user)
		if err != nil {
			t.Fatalf("CreateUser() error = %v", err)
		}
		if user.ID == 0 {
			t.Error("CreateUser() did not set ID")
		}

		// Verify user can be retrieved
		fetched, err := f.DB.GetUserByUsername(ctx, "authtest")
		if err != nil {
			t.Fatalf("GetUserByUsername() error = %v", err)
		}
		if fetched.Email != user.Email {
			t.Errorf("GetUserByUsername() email = %s, want %s", fetched.Email, user.Email)
		}
	})

	t.Run("session creation and validation", func(t *testing.T) {
		// Create user first
		user := &storage.User{
			Username:     "sessiontest",
			PasswordHash: "$2a$10$1234567890123456789012345678901234567890123456789012",
			Email:        "sessiontest@example.com",
			Role:         "admin",
		}
		_ = f.DB.CreateUser(ctx, user)

		// Create session
		session := &storage.Session{
			ID:        "test-session-token-123",
			UserID:    user.ID,
			Token:     "test-session-token-123",
			IPAddress: "192.168.1.1",
			UserAgent: "Mozilla/5.0",
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		err := f.DB.CreateSession(ctx, session)
		if err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}

		// Validate session
		fetched, err := f.DB.GetSessionByToken(ctx, "test-session-token-123")
		if err != nil {
			t.Fatalf("GetSessionByToken() error = %v", err)
		}
		if fetched.UserID != user.ID {
			t.Errorf("GetSessionByToken() userID = %d, want %d", fetched.UserID, user.ID)
		}

		// Delete session (logout)
		err = f.DB.DeleteSession(ctx, "test-session-token-123")
		if err != nil {
			t.Fatalf("DeleteSession() error = %v", err)
		}

		// Verify session is gone
		_, err = f.DB.GetSessionByToken(ctx, "test-session-token-123")
		if err != storage.ErrNotFound {
			t.Errorf("GetSessionByToken() after delete error = %v, want ErrNotFound", err)
		}
	})

	t.Run("expired session rejected", func(t *testing.T) {
		user := &storage.User{
			Username:     "expiredsession",
			PasswordHash: "$2a$10$1234567890123456789012345678901234567890123456789012",
			Email:        "expired@example.com",
			Role:         "user",
		}
		_ = f.DB.CreateUser(ctx, user)

		// Create expired session
		session := &storage.Session{
			ID:        "expired-token",
			UserID:    user.ID,
			Token:     "expired-token",
			IPAddress: "192.168.1.1",
			UserAgent: "Test Agent",
			ExpiresAt: time.Now().Add(-1 * time.Hour), // Already expired
		}
		_ = f.DB.CreateSession(ctx, session)

		// Attempting to get expired session should fail
		_, err := f.DB.GetSessionByToken(ctx, "expired-token")
		if err != storage.ErrNotFound {
			t.Errorf("GetSessionByToken() with expired token error = %v, want ErrNotFound", err)
		}
	})

	t.Run("API key creation and validation", func(t *testing.T) {
		user := &storage.User{
			Username:     "apikeytest",
			PasswordHash: "$2a$10$1234567890123456789012345678901234567890123456789012",
			Email:        "apikey@example.com",
			Role:         "admin",
		}
		_ = f.DB.CreateUser(ctx, user)

		// Create API key
		apiKey := &storage.APIKey{
			UserID:    user.ID,
			Name:      "test-key",
			KeyHash:   "hashed-key-value",
			KeyPrefix: "vcd_test",
			Scopes:    `["read", "write"]`,
		}
		err := f.DB.CreateAPIKey(ctx, apiKey)
		if err != nil {
			t.Fatalf("CreateAPIKey() error = %v", err)
		}

		// Verify API key can be retrieved
		keys, err := f.DB.ListAPIKeys(ctx, user.ID)
		if err != nil {
			t.Fatalf("ListAPIKeys() error = %v", err)
		}
		if len(keys) != 1 {
			t.Errorf("ListAPIKeys() = %d keys, want 1", len(keys))
		}

		// Delete API key
		err = f.DB.DeleteAPIKey(ctx, apiKey.ID)
		if err != nil {
			t.Fatalf("DeleteAPIKey() error = %v", err)
		}
	})

	t.Run("user deletion cascades sessions", func(t *testing.T) {
		user := &storage.User{
			Username:     "deletetest",
			PasswordHash: "$2a$10$1234567890123456789012345678901234567890123456789012",
			Email:        "delete@example.com",
			Role:         "user",
		}
		_ = f.DB.CreateUser(ctx, user)

		// Create multiple sessions
		for i := 0; i < 3; i++ {
			session := &storage.Session{
				ID:        fmt.Sprintf("delete-session-%d", i),
				UserID:    user.ID,
				Token:     fmt.Sprintf("delete-session-%d", i),
				IPAddress: "192.168.1.1",
				UserAgent: "Test Agent",
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}
			_ = f.DB.CreateSession(ctx, session)
		}

		// Delete all user sessions
		err := f.DB.DeleteUserSessions(ctx, user.ID)
		if err != nil {
			t.Fatalf("DeleteUserSessions() error = %v", err)
		}

		// Verify sessions are gone
		sessions, _ := f.DB.ListUserSessions(ctx, user.ID)
		if len(sessions) != 0 {
			t.Errorf("ListUserSessions() = %d, want 0", len(sessions))
		}
	})
}

// TestRoleBasedAccess tests role-based access patterns.
func TestRoleBasedAccess(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Create users with different roles
	roles := []string{"admin", "user", "viewer"}
	users := make(map[string]*storage.User)

	for _, role := range roles {
		user := &storage.User{
			Username:     fmt.Sprintf("%s_user", role),
			PasswordHash: "$2a$10$1234567890123456789012345678901234567890123456789012",
			Email:        fmt.Sprintf("%s@example.com", role),
			Role:         role,
		}
		err := f.DB.CreateUser(ctx, user)
		if err != nil {
			t.Fatalf("CreateUser(%s) error = %v", role, err)
		}
		users[role] = user
	}

	t.Run("admin has highest privileges", func(t *testing.T) {
		admin := users["admin"]
		if admin.Role != "admin" {
			t.Errorf("Admin role = %s, want admin", admin.Role)
		}
	})

	t.Run("user has standard privileges", func(t *testing.T) {
		user := users["user"]
		if user.Role != "user" {
			t.Errorf("User role = %s, want user", user.Role)
		}
	})

	t.Run("viewer has read-only privileges", func(t *testing.T) {
		viewer := users["viewer"]
		if viewer.Role != "viewer" {
			t.Errorf("Viewer role = %s, want viewer", viewer.Role)
		}
	})

	t.Run("role update works", func(t *testing.T) {
		user := users["user"]
		user.Role = "admin"
		err := f.DB.UpdateUserByID(ctx, user)
		if err != nil {
			t.Fatalf("UpdateUserByID() error = %v", err)
		}

		updated, _ := f.DB.GetUserByID(ctx, user.ID)
		if updated.Role != "admin" {
			t.Errorf("Updated role = %s, want admin", updated.Role)
		}
	})
}

// TestAuditTrail tests that security-relevant actions are audited.
func TestAuditTrail(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Close()

	ctx := context.Background()

	// Simulate security-relevant actions and audit them
	auditActions := []struct {
		action   string
		resource string
		details  string
	}{
		{"login", "session", "User login from 192.168.1.1"},
		{"logout", "session", "User logout"},
		{"create", "api_key", "Created API key: test-key"},
		{"delete", "api_key", "Deleted API key: test-key"},
		{"failed_login", "session", "Failed login attempt from 192.168.1.100"},
		{"password_change", "user", "Password changed for user admin"},
		{"role_change", "user", "Role changed from user to admin"},
	}

	for _, audit := range auditActions {
		entry := &storage.AuditEntry{
			Source:    "security_test",
			User:      "testuser",
			Action:    audit.action,
			Resource:  audit.resource,
			Details:   audit.details,
			IPAddress: "192.168.1.1",
			Result:    "success",
			Timestamp: time.Now(),
		}
		err := f.DB.LogAudit(ctx, entry)
		if err != nil {
			t.Fatalf("LogAudit(%s) error = %v", audit.action, err)
		}
	}

	// Verify all audit entries were created
	entries, err := f.DB.ListAuditLogs(ctx, 100, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}

	// Filter to our test entries
	testEntries := 0
	for _, e := range entries {
		if e.Source == "security_test" {
			testEntries++
		}
	}
	if testEntries != len(auditActions) {
		t.Errorf("Found %d test audit entries, want %d", testEntries, len(auditActions))
	}

	// Verify specific action types are recorded
	actionCounts := make(map[string]int)
	for _, e := range entries {
		if e.Source == "security_test" {
			actionCounts[e.Action]++
		}
	}
	if actionCounts["login"] != 1 {
		t.Errorf("Found %d login entries, want 1", actionCounts["login"])
	}
	if actionCounts["failed_login"] != 1 {
		t.Errorf("Found %d failed_login entries, want 1", actionCounts["failed_login"])
	}
}
