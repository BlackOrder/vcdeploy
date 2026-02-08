package storage

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStore_LoadFromDB(t *testing.T) {
	ctx := context.Background()

	// Create a SQLite DB with test data
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create test data in SQLite
	user := &User{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hash123",
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	project := &Project{Name: "test-project"}
	if err := db.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if err := db.SetSetting(ctx, "test", "key1", "value1", "string", false); err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}

	// Create MemoryStore and load from DB
	memStore := NewMemoryStore(nil)
	defer memStore.Close()

	if err := memStore.LoadFromDB(ctx, db); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	// Verify data was loaded
	loadedUser, err := memStore.GetUserByUsername(ctx, "testuser")
	if err != nil {
		t.Errorf("GetUserByUsername() error = %v", err)
	}
	if loadedUser.Email != "test@example.com" {
		t.Errorf("User email = %s, want test@example.com", loadedUser.Email)
	}

	loadedProject, err := memStore.GetProjectByName(ctx, "test-project")
	if err != nil {
		t.Errorf("GetProjectByName() error = %v", err)
	}
	if loadedProject.ID != project.ID {
		t.Errorf("Project ID = %s, want %s", loadedProject.ID, project.ID)
	}

	loadedSetting, err := memStore.GetSetting(ctx, "test", "key1")
	if err != nil {
		t.Errorf("GetSetting() error = %v", err)
	}
	if loadedSetting.Value != "value1" {
		t.Errorf("Setting value = %s, want value1", loadedSetting.Value)
	}
}

func TestMemoryStore_LoadUsers(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create multiple users
	for i := 0; i < 5; i++ {
		user := &User{
			Username:     "user" + string(rune('0'+i)),
			Email:        "user" + string(rune('0'+i)) + "@example.com",
			PasswordHash: "hash",
		}
		db.CreateUser(ctx, user)
	}

	memStore := NewMemoryStore(nil)
	defer memStore.Close()

	if err := memStore.LoadFromDB(ctx, db); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	users, _ := memStore.ListUsers(ctx)
	if len(users) != 5 {
		t.Errorf("len(users) = %d, want 5", len(users))
	}
}

func TestMemoryStore_LoadProjects(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create project type and projects
	pt := &ProjectType{Name: "web", Description: "Web apps"}
	db.CreateProjectType(ctx, pt)

	for i := 0; i < 3; i++ {
		project := &Project{
			Name:       "project" + string(rune('0'+i)),
			TypeID:     strPtr(pt.Name),
			Repository: "git@example.com:test.git",
		}
		db.CreateProject(ctx, project)
	}

	memStore := NewMemoryStore(nil)
	defer memStore.Close()

	if err := memStore.LoadFromDB(ctx, db); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	projects, _ := memStore.ListProjects(ctx)
	if len(projects) != 3 {
		t.Errorf("len(projects) = %d, want 3", len(projects))
	}

	types, _ := memStore.ListProjectTypes(ctx)
	if len(types) != 2 {
		t.Errorf("len(types) = %d, want 2 (1 custom + 1 seeded generic)", len(types))
	}
}

func TestMemoryStore_LoadAgents(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create agents
	agent := &Agent{
		ID:       "agent-1",
		Hostname: "server1.example.com",
		Status:   "online",
		Labels:   map[string]string{"env": "prod"},
	}
	db.UpsertAgent(ctx, agent)

	memStore := NewMemoryStore(nil)
	defer memStore.Close()

	if err := memStore.LoadFromDB(ctx, db); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	loaded, err := memStore.GetAgent(ctx, "agent-1")
	if err != nil {
		t.Errorf("GetAgent() error = %v", err)
	}
	if loaded.Hostname != "server1.example.com" {
		t.Errorf("Hostname = %s, want server1.example.com", loaded.Hostname)
	}
	if loaded.Labels["env"] != "prod" {
		t.Errorf("Labels[env] = %s, want prod", loaded.Labels["env"])
	}
}

func TestMemoryStore_LoadDeployments(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create deployment
	dep := &DeploymentRecord{
		ID:        "deploy-1",
		Project:   "test",
		Status:    "success",
		StartedAt: time.Now(),
	}
	db.CreateDeployment(ctx, dep)

	// Create deployment log
	log := &DeploymentLog{
		DeploymentID: "deploy-1",
		Level:        "INFO",
		Message:      "Starting deployment",
	}
	db.CreateDeploymentLog(ctx, log)

	memStore := NewMemoryStore(nil)
	defer memStore.Close()

	if err := memStore.LoadFromDB(ctx, db); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	loaded, err := memStore.GetDeployment(ctx, "deploy-1")
	if err != nil {
		t.Errorf("GetDeployment() error = %v", err)
	}
	if loaded.Status != "success" {
		t.Errorf("Status = %s, want success", loaded.Status)
	}

	logs, _ := memStore.ListDeploymentLogs(ctx, "deploy-1")
	if len(logs) != 1 {
		t.Errorf("len(logs) = %d, want 1", len(logs))
	}
}

func TestMemoryStore_LoadAuditLogs(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create audit entries
	for i := 0; i < 3; i++ {
		entry := &AuditEntry{
			Action:    "test_action",
			User:      "admin",
			Timestamp: time.Now(),
		}
		db.LogAudit(ctx, entry)
	}

	memStore := NewMemoryStore(nil)
	defer memStore.Close()

	if err := memStore.LoadFromDB(ctx, db); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	logs, _ := memStore.ListAuditLogs(ctx, 100, 0)
	if len(logs) != 3 {
		t.Errorf("len(logs) = %d, want 3", len(logs))
	}
}

func TestMemoryStore_LoadSessions(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create user and session
	user := &User{Username: "testuser", Email: "test@example.com", PasswordHash: "hash"}
	db.CreateUser(ctx, user)

	session := &Session{
		ID:        "test-token-123", // DB uses ID as the key
		Token:     "test-token-123",
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	db.CreateSession(ctx, session)

	memStore := NewMemoryStore(nil)
	defer memStore.Close()

	if err := memStore.LoadFromDB(ctx, db); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	loaded, err := memStore.GetSessionByToken(ctx, "test-token-123")
	if err != nil {
		t.Errorf("GetSessionByToken() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded session is nil")
	}
	if loaded.UserID != user.ID {
		t.Errorf("UserID = %s, want %s", loaded.UserID, user.ID)
	}
}

func TestMemoryStore_LoadAPIKeys(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create user and API key
	user := &User{Username: "testuser", Email: "test@example.com", PasswordHash: "hash"}
	db.CreateUser(ctx, user)

	key := &APIKey{
		UserID:  user.ID,
		Name:    "test-key",
		KeyHash: "hash123abc",
	}
	db.CreateAPIKey(ctx, key)

	memStore := NewMemoryStore(nil)
	defer memStore.Close()

	if err := memStore.LoadFromDB(ctx, db); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	loaded, err := memStore.GetAPIKeyByHash(ctx, "hash123abc")
	if err != nil {
		t.Errorf("GetAPIKeyByHash() error = %v", err)
	}
	if loaded.Name != "test-key" {
		t.Errorf("Name = %s, want test-key", loaded.Name)
	}
}

func TestMemoryStore_LoadSettings(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create settings
	db.SetSetting(ctx, "app", "name", "vcdeploy", "string", false)
	db.SetSetting(ctx, "app", "debug", "true", "bool", false)

	memStore := NewMemoryStore(nil)
	defer memStore.Close()

	if err := memStore.LoadFromDB(ctx, db); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	name, _ := memStore.GetSetting(ctx, "app", "name")
	if name.Value != "vcdeploy" {
		t.Errorf("Setting value = %s, want vcdeploy", name.Value)
	}

	debug, _ := memStore.GetSetting(ctx, "app", "debug")
	if debug.Value != "true" {
		t.Errorf("Setting value = %s, want true", debug.Value)
	}
}

func TestMemoryStore_LoadNextIDs(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create multiple users to advance ID counter
	for i := 0; i < 10; i++ {
		user := &User{
			Username:     "user" + string(rune('a'+i)),
			Email:        "user" + string(rune('a'+i)) + "@example.com",
			PasswordHash: "hash",
		}
		db.CreateUser(ctx, user)
	}

	memStore := NewMemoryStore(nil)
	defer memStore.Close()

	if err := memStore.LoadFromDB(ctx, db); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}

	// Create a new user in memory store - should get next ID
	newUser := &User{
		Username:     "newuser",
		Email:        "new@example.com",
		PasswordHash: "hash",
	}
	memStore.CreateUser(ctx, newUser)

	// New user should have ID set
	if newUser.ID == "" {
		t.Error("New user ID should be set")
	}
}
