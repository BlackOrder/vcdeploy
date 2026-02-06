package storage

import (
	"context"
	"testing"
	"time"
)

// --- Project tests ---

func TestMemoryStore_CreateProject(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	project := &Project{Name: "myproject", Repository: "git@example.com:repo.git"}
	err := s.CreateProject(context.Background(), project)
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if project.ID == 0 {
		t.Error("CreateProject() did not assign ID")
	}
	if project.CreatedAt.IsZero() {
		t.Error("CreateProject() did not set CreatedAt")
	}
}

func TestMemoryStore_CreateProject_Duplicate(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	s.CreateProject(context.Background(), &Project{Name: "duplicate"})

	err := s.CreateProject(context.Background(), &Project{Name: "duplicate"})
	if err != ErrDuplicate {
		t.Errorf("CreateProject() error = %v, want ErrDuplicate", err)
	}
}

func TestMemoryStore_GetProjectByName(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProject(context.Background(), &Project{Name: "findme", Repository: "repo"})

	found, err := s.GetProjectByName(ctx, "findme")
	if err != nil {
		t.Fatalf("GetProjectByName() error = %v", err)
	}
	if found.Name != "findme" {
		t.Errorf("Name = %s, want findme", found.Name)
	}
}

func TestMemoryStore_GetProjectByName_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	_, err := s.GetProjectByName(context.Background(), "nonexistent")
	if err != ErrNotFound {
		t.Errorf("GetProjectByName() error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_ListProjects(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	s.CreateProject(context.Background(), &Project{Name: "project1"})
	s.CreateProject(context.Background(), &Project{Name: "project2"})
	s.CreateProject(context.Background(), &Project{Name: "project3"})

	projects, err := s.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects) != 3 {
		t.Errorf("len(projects) = %d, want 3", len(projects))
	}
}

func TestMemoryStore_UpdateProjectByName(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProject(context.Background(), &Project{Name: "update", Repository: "old"})

	updated := &Project{Name: "update", Repository: "new"}
	err := s.UpdateProjectByName(ctx, updated)
	if err != nil {
		t.Fatalf("UpdateProjectByName() error = %v", err)
	}

	found, _ := s.GetProjectByName(ctx, "update")
	if found.Repository != "new" {
		t.Errorf("Repository = %s, want new", found.Repository)
	}
}

func TestMemoryStore_UpdateProjectByName_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	err := s.UpdateProjectByName(context.Background(), &Project{Name: "nonexistent"})
	if err != ErrNotFound {
		t.Errorf("UpdateProjectByName() error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_DeleteProject(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProject(context.Background(), &Project{Name: "delete"})

	err := s.DeleteProject(context.Background(), "delete")
	if err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	_, err = s.GetProjectByName(ctx, "delete")
	if err != ErrNotFound {
		t.Error("Project still exists after delete")
	}
}

func TestMemoryStore_DeleteProject_CascadesWebhooks(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	project := &Project{Name: "withwebhook"}
	s.CreateProject(context.Background(), project)

	s.SetProjectWebhook(ctx, project.ID, "github", []byte("secret"), true, true)

	// Delete project should cascade
	s.DeleteProject(context.Background(), "withwebhook")

	webhooks, _ := s.ListProjectWebhooks(ctx, project.ID)
	if len(webhooks) != 0 {
		t.Error("Webhooks still exist after project delete")
	}
}

func TestMemoryStore_DeleteProject_CascadesSecrets(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProject(context.Background(), &Project{Name: "withsecret"})
	s.SetSecretEncrypted(ctx, "withsecret", "env", "API_KEY", []byte("encrypted"))

	s.DeleteProject(context.Background(), "withsecret")

	_, err := s.GetSecret(ctx, "withsecret", "env", "API_KEY")
	if err != ErrNotFound {
		t.Error("Secret still exists after project delete")
	}
}

func TestMemoryStore_DeleteProject_CascadesDeployments(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProject(context.Background(), &Project{Name: "withdeployment"})

	// Create a deployment for this project
	dep := &DeploymentRecord{
		ID:      "dep-123",
		Project: "withdeployment",
		Status:  "completed",
	}
	s.CreateDeployment(ctx, dep)

	// Add a deployment log
	s.CreateDeploymentLog(ctx, &DeploymentLog{
		ID:           1,
		DeploymentID: "dep-123",
		Message:      "Build started",
	})

	// Create scheduled deployment
	s.CreateScheduledDeployment(ctx, "sched-123", "withdeployment", "production", "main", time.Now().Add(time.Hour), "user@test.com")

	// Delete project should cascade
	s.DeleteProject(context.Background(), "withdeployment")

	// Verify deployment is deleted
	_, err := s.GetDeployment(ctx, "dep-123")
	if err != ErrNotFound {
		t.Error("Deployment still exists after project delete")
	}

	// Verify deployment logs are deleted
	logs, _ := s.ListDeploymentLogs(ctx, "dep-123")
	if len(logs) != 0 {
		t.Error("Deployment logs still exist after project delete")
	}

	// Verify scheduled deployment is deleted (would error if we try to cancel it)
	err = s.CancelScheduledDeployment(ctx, "sched-123")
	if err != ErrNotFound {
		t.Error("Scheduled deployment still exists after project delete")
	}
}

// --- Project Type tests ---

func TestMemoryStore_CreateProjectType(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	pt := &ProjectType{Name: "nodejs", Description: "Node.js projects"}
	err := s.CreateProjectType(ctx, pt)
	if err != nil {
		t.Fatalf("CreateProjectType() error = %v", err)
	}

	if pt.ID == 0 {
		t.Error("CreateProjectType() did not assign ID")
	}
}

func TestMemoryStore_CreateProjectType_Duplicate(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProjectType(ctx, &ProjectType{Name: "duplicate"})

	err := s.CreateProjectType(ctx, &ProjectType{Name: "duplicate"})
	if err != ErrDuplicate {
		t.Errorf("CreateProjectType() error = %v, want ErrDuplicate", err)
	}
}

func TestMemoryStore_GetProjectTypeByName(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProjectType(ctx, &ProjectType{Name: "golang", BuildCmd: "go build"})

	found, err := s.GetProjectTypeByName(ctx, "golang")
	if err != nil {
		t.Fatalf("GetProjectTypeByName() error = %v", err)
	}
	if found.BuildCmd != "go build" {
		t.Errorf("BuildCmd = %s, want 'go build'", found.BuildCmd)
	}
}

func TestMemoryStore_ListProjectTypes(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProjectType(ctx, &ProjectType{Name: "type1"})
	s.CreateProjectType(ctx, &ProjectType{Name: "type2"})

	types, err := s.ListProjectTypes(ctx)
	if err != nil {
		t.Fatalf("ListProjectTypes() error = %v", err)
	}
	if len(types) != 2 {
		t.Errorf("len(types) = %d, want 2", len(types))
	}
}

func TestMemoryStore_UpdateProjectTypeByName(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProjectType(ctx, &ProjectType{Name: "update", Description: "old"})

	err := s.UpdateProjectTypeByName(ctx, &ProjectType{Name: "update", Description: "new"})
	if err != nil {
		t.Fatalf("UpdateProjectTypeByName() error = %v", err)
	}

	found, _ := s.GetProjectTypeByName(ctx, "update")
	if found.Description != "new" {
		t.Errorf("Description = %s, want new", found.Description)
	}
}

func TestMemoryStore_DeleteProjectType(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProjectType(ctx, &ProjectType{Name: "delete"})

	err := s.DeleteProjectType(ctx, "delete")
	if err != nil {
		t.Fatalf("DeleteProjectType() error = %v", err)
	}

	_, err = s.GetProjectTypeByName(ctx, "delete")
	if err != ErrNotFound {
		t.Error("ProjectType still exists after delete")
	}
}

// --- Webhook tests ---

func TestMemoryStore_SetProjectWebhook_Create(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	project := &Project{Name: "webhooktest"}
	s.CreateProject(context.Background(), project)

	err := s.SetProjectWebhook(ctx, project.ID, "github", []byte("secret"), true, true)
	if err != nil {
		t.Fatalf("SetProjectWebhook() error = %v", err)
	}

	webhook, err := s.GetProjectWebhook(ctx, project.ID, "github")
	if err != nil {
		t.Fatalf("GetProjectWebhook() error = %v", err)
	}
	if !webhook.Enabled {
		t.Error("Webhook not enabled")
	}
}

func TestMemoryStore_SetProjectWebhook_Update(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	project := &Project{Name: "webhookupdate"}
	s.CreateProject(context.Background(), project)

	// Create
	s.SetProjectWebhook(ctx, project.ID, "github", []byte("old"), true, true)

	// Update
	s.SetProjectWebhook(ctx, project.ID, "github", []byte("new"), false, false)

	webhook, _ := s.GetProjectWebhook(ctx, project.ID, "github")
	if webhook.Enabled {
		t.Error("Webhook should be disabled after update")
	}
	if string(webhook.SecretEncrypted) != "new" {
		t.Errorf("SecretEncrypted = %s, want new", webhook.SecretEncrypted)
	}
}

func TestMemoryStore_ListProjectWebhooks(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	project := &Project{Name: "webhooklist"}
	s.CreateProject(context.Background(), project)

	s.SetProjectWebhook(ctx, project.ID, "github", nil, true, false)
	s.SetProjectWebhook(ctx, project.ID, "gitlab", nil, true, false)

	webhooks, err := s.ListProjectWebhooks(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListProjectWebhooks() error = %v", err)
	}
	if len(webhooks) != 2 {
		t.Errorf("len(webhooks) = %d, want 2", len(webhooks))
	}
}

func TestMemoryStore_DeleteProjectWebhook(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	project := &Project{Name: "webhookdelete"}
	s.CreateProject(context.Background(), project)
	s.SetProjectWebhook(ctx, project.ID, "github", nil, true, false)

	err := s.DeleteProjectWebhook(ctx, project.ID, "github")
	if err != nil {
		t.Fatalf("DeleteProjectWebhook() error = %v", err)
	}

	_, err = s.GetProjectWebhook(ctx, project.ID, "github")
	if err != ErrNotFound {
		t.Error("Webhook still exists after delete")
	}
}

// --- Secret tests ---

func TestMemoryStore_SetSecretEncrypted_Create(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	err := s.SetSecretEncrypted(ctx, "myproject", "env", "API_KEY", []byte("encrypted"))
	if err != nil {
		t.Fatalf("SetSecretEncrypted() error = %v", err)
	}

	secret, err := s.GetSecret(ctx, "myproject", "env", "API_KEY")
	if err != nil {
		t.Fatalf("GetSecret() error = %v", err)
	}
	if string(secret.ValueEncrypted) != "encrypted" {
		t.Errorf("ValueEncrypted = %s, want encrypted", secret.ValueEncrypted)
	}
}

func TestMemoryStore_SetSecretEncrypted_Update(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.SetSecretEncrypted(ctx, "myproject", "env", "API_KEY", []byte("old"))
	s.SetSecretEncrypted(ctx, "myproject", "env", "API_KEY", []byte("new"))

	secret, _ := s.GetSecret(ctx, "myproject", "env", "API_KEY")
	if string(secret.ValueEncrypted) != "new" {
		t.Errorf("ValueEncrypted = %s, want new", secret.ValueEncrypted)
	}
}

func TestMemoryStore_GetSecret_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	_, err := s.GetSecret(context.Background(), "project", "scope", "key")
	if err != ErrNotFound {
		t.Errorf("GetSecret() error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_ListSecretsCtx(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.SetSecretEncrypted(ctx, "project1", "env", "KEY1", []byte("val"))
	s.SetSecretEncrypted(ctx, "project1", "env", "KEY2", []byte("val"))
	s.SetSecretEncrypted(ctx, "project2", "env", "KEY1", []byte("val"))

	secrets, err := s.ListSecretsCtx(ctx, "project1")
	if err != nil {
		t.Fatalf("ListSecretsCtx() error = %v", err)
	}
	if len(secrets) != 2 {
		t.Errorf("len(secrets) for project1 = %d, want 2", len(secrets))
	}
}

func TestMemoryStore_ListSecretsWithScope(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.SetSecretEncrypted(ctx, "project", "env", "KEY1", []byte("val"))
	s.SetSecretEncrypted(ctx, "project", "deploy", "KEY2", []byte("val"))

	secrets, err := s.ListSecretsWithScope(ctx, "project", "env")
	if err != nil {
		t.Fatalf("ListSecretsWithScope() error = %v", err)
	}
	if len(secrets) != 1 {
		t.Errorf("len(secrets) = %d, want 1", len(secrets))
	}
}

func TestMemoryStore_DeleteSecretCtx(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.SetSecretEncrypted(ctx, "project", "env", "DELETE", []byte("val"))

	err := s.DeleteSecretCtx(ctx, "project", "env", "DELETE")
	if err != nil {
		t.Fatalf("DeleteSecretCtx() error = %v", err)
	}

	_, err = s.GetSecret(ctx, "project", "env", "DELETE")
	if err != ErrNotFound {
		t.Error("Secret still exists after delete")
	}
}

func TestMemoryStore_ExportAllSecrets(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.SetSecretEncrypted(ctx, "proj1", "env", "KEY1", []byte("val"))
	s.SetSecretEncrypted(ctx, "proj1", "env", "KEY2", []byte("val"))
	s.SetSecretEncrypted(ctx, "proj2", "deploy", "KEY3", []byte("val"))

	export, err := s.ExportAllSecrets(context.Background())
	if err != nil {
		t.Fatalf("ExportAllSecrets() error = %v", err)
	}

	if len(export) != 2 {
		t.Errorf("len(export) = %d, want 2 projects", len(export))
	}
	if len(export["proj1"]) != 2 {
		t.Errorf("len(export[proj1]) = %d, want 2", len(export["proj1"]))
	}
}
