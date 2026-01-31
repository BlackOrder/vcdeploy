package storage

import (
	"context"
	"testing"
	"time"
)

func TestNewTestMemoryStore(t *testing.T) {
	store := NewTestMemoryStore(t)
	
	if store == nil {
		t.Fatal("NewTestMemoryStore() returned nil")
	}
	if store.MemoryStore == nil {
		t.Fatal("NewTestMemoryStore().MemoryStore is nil")
	}
	if store.t != t {
		t.Fatal("NewTestMemoryStore().t is incorrect")
	}
	
	// Verify store is functional by creating a user
	user := store.MustCreateUser("pingtest", "ping@example.com", "hash")
	if user.ID == 0 {
		t.Error("store should be functional")
	}
}

func TestTestMemoryStore_MustCreateUser(t *testing.T) {
	store := NewTestMemoryStore(t)
	
	user := store.MustCreateUser("testuser", "test@example.com", "passhash")
	
	if user.ID == 0 {
		t.Error("user.ID should be assigned")
	}
	if user.Username != "testuser" {
		t.Errorf("user.Username = %q, want %q", user.Username, "testuser")
	}
	
	// Verify it was stored
	got, err := store.GetUserByUsername(context.Background(), "testuser")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("GetUserByUsername().ID = %d, want %d", got.ID, user.ID)
	}
}

func TestTestMemoryStore_MustCreateProject(t *testing.T) {
	store := NewTestMemoryStore(t)
	
	project := store.MustCreateProject("test-project")
	
	if project.ID == 0 {
		t.Error("project.ID should be assigned")
	}
	if project.Name != "test-project" {
		t.Errorf("project.Name = %q, want %q", project.Name, "test-project")
	}
	
	// Verify it was stored
	got, err := store.GetProjectByName(context.Background(), "test-project")
	if err != nil {
		t.Fatalf("GetProjectByName() error = %v", err)
	}
	if got.ID != project.ID {
		t.Errorf("GetProjectByName().ID = %d, want %d", got.ID, project.ID)
	}
}

func TestTestMemoryStore_MustCreateSession(t *testing.T) {
	store := NewTestMemoryStore(t)
	
	user := store.MustCreateUser("sessionuser", "session@example.com", "hash")
	expires := time.Now().Add(time.Hour)
	session := store.MustCreateSession(user.ID, "test-token", expires)
	
	if session.ID == "" {
		t.Error("session.ID should be assigned")
	}
	if session.UserID != user.ID {
		t.Errorf("session.UserID = %d, want %d", session.UserID, user.ID)
	}
	
	// Verify it was stored
	got, err := store.GetSessionByToken(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("GetSessionByToken() error = %v", err)
	}
	if got.ID != session.ID {
		t.Errorf("GetSessionByToken().ID = %s, want %s", got.ID, session.ID)
	}
}

func TestTestMemoryStore_MustCreateAPIKey(t *testing.T) {
	store := NewTestMemoryStore(t)
	
	user := store.MustCreateUser("apikeyuser", "apikey@example.com", "hash")
	key := store.MustCreateAPIKey(user.ID, "test-key", "key-hash-123")
	
	if key.ID == 0 {
		t.Error("key.ID should be assigned")
	}
	if key.Name != "test-key" {
		t.Errorf("key.Name = %q, want %q", key.Name, "test-key")
	}
	
	// Verify it was stored
	got, err := store.GetAPIKeyByHash(context.Background(), "key-hash-123")
	if err != nil {
		t.Fatalf("GetAPIKeyByHash() error = %v", err)
	}
	if got.ID != key.ID {
		t.Errorf("GetAPIKeyByHash().ID = %d, want %d", got.ID, key.ID)
	}
}

func TestTestMemoryStore_MustUpsertAgent(t *testing.T) {
	store := NewTestMemoryStore(t)
	
	agent := store.MustUpsertAgent("agent-test-1", "host1.example.com", "online")
	
	if agent.ID != "agent-test-1" {
		t.Errorf("agent.ID = %q, want %q", agent.ID, "agent-test-1")
	}
	if agent.Hostname != "host1.example.com" {
		t.Errorf("agent.Hostname = %q, want %q", agent.Hostname, "host1.example.com")
	}
	
	// Verify it was stored
	got, err := store.GetAgent(context.Background(), "agent-test-1")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if got.Hostname != agent.Hostname {
		t.Errorf("GetAgent().Hostname = %q, want %q", got.Hostname, agent.Hostname)
	}
}

func TestTestMemoryStore_MustCreateDeployment(t *testing.T) {
	store := NewTestMemoryStore(t)
	
	project := store.MustCreateProject("deploy-project")
	deployment := store.MustCreateDeployment("deploy-test-1", project.Name, "running")
	
	if deployment.ID != "deploy-test-1" {
		t.Errorf("deployment.ID = %q, want %q", deployment.ID, "deploy-test-1")
	}
	if deployment.Status != "running" {
		t.Errorf("deployment.Status = %q, want %q", deployment.Status, "running")
	}
	
	// Verify it was stored
	got, err := store.GetDeployment(context.Background(), "deploy-test-1")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}
	if got.Project != deployment.Project {
		t.Errorf("GetDeployment().Project = %q, want %q", got.Project, deployment.Project)
	}
}

func TestTestMemoryStore_MustSetSetting(t *testing.T) {
	store := NewTestMemoryStore(t)
	
	store.MustSetSetting("test", "key1", "value1")
	
	// Verify it was stored
	got, err := store.GetSetting(context.Background(), "test", "key1")
	if err != nil {
		t.Fatalf("GetSetting() error = %v", err)
	}
	if got.Value != "value1" {
		t.Errorf("GetSetting().Value = %q, want %q", got.Value, "value1")
	}
}

func TestTestMemoryStore_MustBlockIP(t *testing.T) {
	store := NewTestMemoryStore(t)
	
	expires := time.Now().Add(time.Hour)
	blocked := store.MustBlockIP("192.168.1.100", "testing", expires)
	
	if blocked.ID == 0 {
		t.Error("blocked.ID should be assigned")
	}
	if blocked.IPAddress != "192.168.1.100" {
		t.Errorf("blocked.IPAddress = %q, want %q", blocked.IPAddress, "192.168.1.100")
	}
	
	// Verify it was stored
	isBlocked, err := store.IsIPBlocked(context.Background(), "192.168.1.100")
	if err != nil {
		t.Fatalf("IsIPBlocked() error = %v", err)
	}
	if !isBlocked {
		t.Error("IP should be blocked")
	}
}

func TestTestMemoryStore_MustLogAudit(t *testing.T) {
	store := NewTestMemoryStore(t)
	
	entry := store.MustLogAudit("test_action", "testuser", "test_resource")
	
	if entry.ID == 0 {
		t.Error("entry.ID should be assigned")
	}
	if entry.Action != "test_action" {
		t.Errorf("entry.Action = %q, want %q", entry.Action, "test_action")
	}
	
	// Verify it was stored
	entries, err := store.ListAuditLogs(context.Background(), 100, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].ID != entry.ID {
		t.Errorf("entries[0].ID = %d, want %d", entries[0].ID, entry.ID)
	}
}

func TestTestMemoryStore_SeedTestData(t *testing.T) {
	store := NewTestMemoryStore(t)
	
	data := store.SeedTestData()
	
	// Verify all data was created
	if data.AdminUser == nil || data.AdminUser.ID == 0 {
		t.Error("AdminUser should be created")
	}
	if data.RegularUser == nil || data.RegularUser.ID == 0 {
		t.Error("RegularUser should be created")
	}
	if data.Project1 == nil || data.Project1.ID == 0 {
		t.Error("Project1 should be created")
	}
	if data.Project2 == nil || data.Project2.ID == 0 {
		t.Error("Project2 should be created")
	}
	if data.AdminSession == nil || data.AdminSession.ID == "" {
		t.Error("AdminSession should be created")
	}
	if data.UserSession == nil || data.UserSession.ID == "" {
		t.Error("UserSession should be created")
	}
	if data.AdminAPIKey == nil || data.AdminAPIKey.ID == 0 {
		t.Error("AdminAPIKey should be created")
	}
	if data.Agent1 == nil || data.Agent1.ID == "" {
		t.Error("Agent1 should be created")
	}
	if data.Agent2 == nil || data.Agent2.ID == "" {
		t.Error("Agent2 should be created")
	}
	if data.Deployment1 == nil || data.Deployment1.ID == "" {
		t.Error("Deployment1 should be created")
	}
	if data.Deployment2 == nil || data.Deployment2.ID == "" {
		t.Error("Deployment2 should be created")
	}
	
	// Verify settings
	setting, err := store.GetSetting(context.Background(), "app", "name")
	if err != nil {
		t.Fatalf("GetSetting() error = %v", err)
	}
	if setting.Value != "vcdeploy" {
		t.Errorf("app.name = %q, want %q", setting.Value, "vcdeploy")
	}
}

func TestTestMemoryStore_SeedTestData_DataRelationships(t *testing.T) {
	store := NewTestMemoryStore(t)
	
	data := store.SeedTestData()
	
	// Verify session belongs to correct user
	session, err := store.GetSessionByToken(context.Background(), "admin-session-token")
	if err != nil {
		t.Fatalf("GetSessionByToken() error = %v", err)
	}
	if session.UserID != data.AdminUser.ID {
		t.Errorf("AdminSession.UserID = %d, want %d", session.UserID, data.AdminUser.ID)
	}
	
	// Verify API key belongs to correct user
	apiKey, err := store.GetAPIKeyByHash(context.Background(), "admin-key-hash")
	if err != nil {
		t.Fatalf("GetAPIKeyByHash() error = %v", err)
	}
	if apiKey.UserID != data.AdminUser.ID {
		t.Errorf("AdminAPIKey.UserID = %d, want %d", apiKey.UserID, data.AdminUser.ID)
	}
	
	// Verify deployment belongs to correct project
	deployment, err := store.GetDeployment(context.Background(), data.Deployment1.ID)
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}
	if deployment.Project != data.Project1.Name {
		t.Errorf("Deployment1.Project = %q, want %q", deployment.Project, data.Project1.Name)
	}
}
