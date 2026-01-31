package storage

import (
	"context"
	"testing"
	"time"
)

// --- SSHHostKey tests ---

func TestMemoryStore_CreateSSHHostKey(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	key := &SSHHostKey{
		Hostname:    "server.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   "AAAAB3...",
		Fingerprint: "SHA256:abc123",
	}
	err := s.CreateSSHHostKey(ctx, key)
	if err != nil {
		t.Fatalf("CreateSSHHostKey() error = %v", err)
	}

	if key.ID == 0 {
		t.Error("CreateSSHHostKey() did not assign ID")
	}
}

func TestMemoryStore_CreateSSHHostKey_Duplicate(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateSSHHostKey(ctx, &SSHHostKey{Hostname: "server", Port: 22, KeyType: "ssh-rsa"})

	err := s.CreateSSHHostKey(ctx, &SSHHostKey{Hostname: "server", Port: 22, KeyType: "ssh-rsa"})
	if err != ErrDuplicate {
		t.Errorf("CreateSSHHostKey() error = %v, want ErrDuplicate", err)
	}
}

func TestMemoryStore_GetSSHHostKey(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateSSHHostKey(ctx, &SSHHostKey{Hostname: "server", Port: 22, KeyType: "ssh-rsa", Fingerprint: "abc"})

	key, err := s.GetSSHHostKey(ctx, "server", 22, "ssh-rsa")
	if err != nil {
		t.Fatalf("GetSSHHostKey() error = %v", err)
	}
	if key.Fingerprint != "abc" {
		t.Errorf("Fingerprint = %s, want abc", key.Fingerprint)
	}
}

func TestMemoryStore_GetSSHHostKeysByHost(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateSSHHostKey(ctx, &SSHHostKey{Hostname: "server", Port: 22, KeyType: "ssh-rsa"})
	s.CreateSSHHostKey(ctx, &SSHHostKey{Hostname: "server", Port: 22, KeyType: "ssh-ed25519"})
	s.CreateSSHHostKey(ctx, &SSHHostKey{Hostname: "other", Port: 22, KeyType: "ssh-rsa"})

	keys, err := s.GetSSHHostKeysByHost(ctx, "server", 22)
	if err != nil {
		t.Fatalf("GetSSHHostKeysByHost() error = %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("len(keys) = %d, want 2", len(keys))
	}
}

func TestMemoryStore_ListSSHHostKeys(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateSSHHostKey(ctx, &SSHHostKey{Hostname: "s1", Port: 22, KeyType: "ssh-rsa"})
	s.CreateSSHHostKey(ctx, &SSHHostKey{Hostname: "s2", Port: 22, KeyType: "ssh-rsa"})

	keys, err := s.ListSSHHostKeys(ctx)
	if err != nil {
		t.Fatalf("ListSSHHostKeys() error = %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("len(keys) = %d, want 2", len(keys))
	}
}

func TestMemoryStore_UpdateSSHHostKeyTrust(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	key := &SSHHostKey{Hostname: "server", Port: 22, KeyType: "ssh-rsa", Trusted: false}
	s.CreateSSHHostKey(ctx, key)

	err := s.UpdateSSHHostKeyTrust(ctx, key.ID, true, "admin")
	if err != nil {
		t.Fatalf("UpdateSSHHostKeyTrust() error = %v", err)
	}

	found, _ := s.GetSSHHostKey(ctx, "server", 22, "ssh-rsa")
	if !found.Trusted {
		t.Error("Trusted should be true")
	}
	if found.AddedBy != "admin" {
		t.Errorf("AddedBy = %s, want admin", found.AddedBy)
	}
}

func TestMemoryStore_DeleteSSHHostKey(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	key := &SSHHostKey{Hostname: "server", Port: 22, KeyType: "ssh-rsa"}
	s.CreateSSHHostKey(ctx, key)

	err := s.DeleteSSHHostKey(ctx, key.ID)
	if err != nil {
		t.Fatalf("DeleteSSHHostKey() error = %v", err)
	}

	_, err = s.GetSSHHostKey(ctx, "server", 22, "ssh-rsa")
	if err != ErrNotFound {
		t.Error("Key still exists after delete")
	}
}

func TestMemoryStore_DeleteSSHHostKeysByHost(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateSSHHostKey(ctx, &SSHHostKey{Hostname: "server", Port: 22, KeyType: "ssh-rsa"})
	s.CreateSSHHostKey(ctx, &SSHHostKey{Hostname: "server", Port: 22, KeyType: "ssh-ed25519"})

	count, err := s.DeleteSSHHostKeysByHost(ctx, "server", 22)
	if err != nil {
		t.Fatalf("DeleteSSHHostKeysByHost() error = %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	keys, _ := s.GetSSHHostKeysByHost(ctx, "server", 22)
	if len(keys) != 0 {
		t.Error("Keys still exist after delete")
	}
}

// --- SSHJumpServer tests ---

func TestMemoryStore_CreateJumpServer(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	js := &SSHJumpServer{
		Name:     "bastion1",
		Host:     "bastion.example.com",
		Port:     22,
		Username: "admin",
	}
	err := s.CreateJumpServer(ctx, js)
	if err != nil {
		t.Fatalf("CreateJumpServer() error = %v", err)
	}

	if js.ID == 0 {
		t.Error("CreateJumpServer() did not assign ID")
	}
}

func TestMemoryStore_CreateJumpServer_Duplicate(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateJumpServer(ctx, &SSHJumpServer{Name: "bastion"})

	err := s.CreateJumpServer(ctx, &SSHJumpServer{Name: "bastion"})
	if err != ErrDuplicate {
		t.Errorf("CreateJumpServer() error = %v, want ErrDuplicate", err)
	}
}

func TestMemoryStore_GetJumpServer(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	js := &SSHJumpServer{Name: "bastion", Host: "server.com"}
	s.CreateJumpServer(ctx, js)

	found, err := s.GetJumpServer(ctx, js.ID)
	if err != nil {
		t.Fatalf("GetJumpServer() error = %v", err)
	}
	if found.Host != "server.com" {
		t.Errorf("Host = %s, want server.com", found.Host)
	}
}

func TestMemoryStore_GetJumpServerByName(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateJumpServer(ctx, &SSHJumpServer{Name: "bastion", Host: "server.com"})

	found, err := s.GetJumpServerByName(ctx, "bastion")
	if err != nil {
		t.Fatalf("GetJumpServerByName() error = %v", err)
	}
	if found.Host != "server.com" {
		t.Errorf("Host = %s, want server.com", found.Host)
	}
}

func TestMemoryStore_ListJumpServers(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateJumpServer(ctx, &SSHJumpServer{Name: "js1"})
	s.CreateJumpServer(ctx, &SSHJumpServer{Name: "js2"})

	list, err := s.ListJumpServers(ctx)
	if err != nil {
		t.Fatalf("ListJumpServers() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
}

func TestMemoryStore_UpdateJumpServer(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	js := &SSHJumpServer{Name: "bastion", Host: "old.com"}
	s.CreateJumpServer(ctx, js)

	js.Host = "new.com"
	err := s.UpdateJumpServer(ctx, js)
	if err != nil {
		t.Fatalf("UpdateJumpServer() error = %v", err)
	}

	found, _ := s.GetJumpServer(ctx, js.ID)
	if found.Host != "new.com" {
		t.Errorf("Host = %s, want new.com", found.Host)
	}
}

func TestMemoryStore_DeleteJumpServer(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	js := &SSHJumpServer{Name: "bastion"}
	s.CreateJumpServer(ctx, js)

	err := s.DeleteJumpServer(ctx, js.ID)
	if err != nil {
		t.Fatalf("DeleteJumpServer() error = %v", err)
	}

	_, err = s.GetJumpServer(ctx, js.ID)
	if err != ErrNotFound {
		t.Error("JumpServer still exists after delete")
	}
}

// --- ProvisionJob tests ---

func TestMemoryStore_CreateProvisionJob(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	job := &ProvisionJob{
		ID:         "job-1",
		TargetHost: "server.example.com",
		TargetPort: 22,
		TargetUser: "root",
	}
	err := s.CreateProvisionJob(ctx, job)
	if err != nil {
		t.Fatalf("CreateProvisionJob() error = %v", err)
	}

	if job.Status != "pending" {
		t.Errorf("Status = %s, want pending", job.Status)
	}
}

func TestMemoryStore_CreateProvisionJob_Duplicate(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProvisionJob(ctx, &ProvisionJob{ID: "dup"})

	err := s.CreateProvisionJob(ctx, &ProvisionJob{ID: "dup"})
	if err != ErrDuplicate {
		t.Errorf("CreateProvisionJob() error = %v, want ErrDuplicate", err)
	}
}

func TestMemoryStore_GetProvisionJob(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProvisionJob(ctx, &ProvisionJob{ID: "job-1", TargetHost: "server"})

	job, err := s.GetProvisionJob(ctx, "job-1")
	if err != nil {
		t.Fatalf("GetProvisionJob() error = %v", err)
	}
	if job.TargetHost != "server" {
		t.Errorf("TargetHost = %s, want server", job.TargetHost)
	}
}

func TestMemoryStore_UpdateProvisionJobStatus(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProvisionJob(ctx, &ProvisionJob{ID: "job-1"})

	err := s.UpdateProvisionJobStatus(ctx, "job-1", "in_progress", "installing", "", 50)
	if err != nil {
		t.Fatalf("UpdateProvisionJobStatus() error = %v", err)
	}

	job, _ := s.GetProvisionJob(ctx, "job-1")
	if job.Status != "in_progress" {
		t.Errorf("Status = %s, want in_progress", job.Status)
	}
	if job.Progress != 50 {
		t.Errorf("Progress = %d, want 50", job.Progress)
	}
}

func TestMemoryStore_UpdateProvisionJobStatus_Completed(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProvisionJob(ctx, &ProvisionJob{ID: "job-1"})

	s.UpdateProvisionJobStatus(ctx, "job-1", "completed", "done", "", 100)

	job, _ := s.GetProvisionJob(ctx, "job-1")
	if job.CompletedAt == nil {
		t.Error("CompletedAt should be set when status is completed")
	}
}

func TestMemoryStore_ListPendingProvisionJobs(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateProvisionJob(ctx, &ProvisionJob{ID: "pending1", Status: "pending"})
	s.CreateProvisionJob(ctx, &ProvisionJob{ID: "running1", Status: "in_progress"})
	s.CreateProvisionJob(ctx, &ProvisionJob{ID: "done1", Status: "completed"})

	pending, err := s.ListPendingProvisionJobs(ctx)
	if err != nil {
		t.Fatalf("ListPendingProvisionJobs() error = %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("len(pending) = %d, want 2", len(pending))
	}
}

func TestMemoryStore_ListProvisionJobsByHost(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	s.CreateProvisionJob(ctx, &ProvisionJob{ID: "j1", TargetHost: "server1", StartedAt: now.Add(-time.Hour)})
	s.CreateProvisionJob(ctx, &ProvisionJob{ID: "j2", TargetHost: "server1", StartedAt: now})
	s.CreateProvisionJob(ctx, &ProvisionJob{ID: "j3", TargetHost: "server2"})

	list, total, err := s.ListProvisionJobsByHost(ctx, "server1", 10, 0)
	if err != nil {
		t.Fatalf("ListProvisionJobsByHost() error = %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	// Should be newest first
	if list[0].ID != "j2" {
		t.Errorf("First ID = %s, want j2 (newest first)", list[0].ID)
	}
}

// --- HealthCheckConfig tests ---

func TestMemoryStore_CreateHealthCheckConfig(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	config := &HealthCheckConfig{
		Name:           "Default Health Check",
		URL:            "{{.URL}}/health",
		Method:         "GET",
		ExpectedStatus: 200,
		TimeoutSeconds: 30,
		Retries:        3,
	}
	err := s.CreateHealthCheckConfig(ctx, config)
	if err != nil {
		t.Fatalf("CreateHealthCheckConfig() error = %v", err)
	}

	if config.ID == 0 {
		t.Error("CreateHealthCheckConfig() did not assign ID")
	}
}

func TestMemoryStore_GetHealthCheckConfig(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	config := &HealthCheckConfig{Name: "test"}
	s.CreateHealthCheckConfig(ctx, config)

	found, err := s.GetHealthCheckConfig(ctx, config.ID)
	if err != nil {
		t.Fatalf("GetHealthCheckConfig() error = %v", err)
	}
	if found.Name != "test" {
		t.Errorf("Name = %s, want test", found.Name)
	}
}

func TestMemoryStore_GetGlobalHealthCheckConfig(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateHealthCheckConfig(ctx, &HealthCheckConfig{Name: "project-config", IsGlobal: false})
	s.CreateHealthCheckConfig(ctx, &HealthCheckConfig{Name: "global-config", IsGlobal: true})

	global, err := s.GetGlobalHealthCheckConfig(ctx)
	if err != nil {
		t.Fatalf("GetGlobalHealthCheckConfig() error = %v", err)
	}
	if global.Name != "global-config" {
		t.Errorf("Name = %s, want global-config", global.Name)
	}
}

func TestMemoryStore_GetHealthCheckConfigForProject(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	projectID := int64(42)
	s.CreateHealthCheckConfig(ctx, &HealthCheckConfig{Name: "project-config", ProjectID: &projectID})

	config, err := s.GetHealthCheckConfigForProject(ctx, 42)
	if err != nil {
		t.Fatalf("GetHealthCheckConfigForProject() error = %v", err)
	}
	if config.Name != "project-config" {
		t.Errorf("Name = %s, want project-config", config.Name)
	}
}

func TestMemoryStore_UpdateHealthCheckConfig(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	config := &HealthCheckConfig{Name: "old", TimeoutSeconds: 10}
	s.CreateHealthCheckConfig(ctx, config)

	config.Name = "new"
	config.TimeoutSeconds = 30
	err := s.UpdateHealthCheckConfig(ctx, config)
	if err != nil {
		t.Fatalf("UpdateHealthCheckConfig() error = %v", err)
	}

	found, _ := s.GetHealthCheckConfig(ctx, config.ID)
	if found.TimeoutSeconds != 30 {
		t.Errorf("TimeoutSeconds = %d, want 30", found.TimeoutSeconds)
	}
}

func TestMemoryStore_ListHealthCheckConfigs(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateHealthCheckConfig(ctx, &HealthCheckConfig{Name: "config1"})
	s.CreateHealthCheckConfig(ctx, &HealthCheckConfig{Name: "config2"})

	list, err := s.ListHealthCheckConfigs(ctx)
	if err != nil {
		t.Fatalf("ListHealthCheckConfigs() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
}

func TestMemoryStore_DeleteHealthCheckConfig(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	config := &HealthCheckConfig{Name: "delete-me"}
	s.CreateHealthCheckConfig(ctx, config)

	err := s.DeleteHealthCheckConfig(ctx, config.ID)
	if err != nil {
		t.Fatalf("DeleteHealthCheckConfig() error = %v", err)
	}

	_, err = s.GetHealthCheckConfig(ctx, config.ID)
	if err != ErrNotFound {
		t.Error("Config still exists after delete")
	}
}

// --- HasSettings test ---

func TestMemoryStore_HasSettings(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	// No settings initially
	has, err := s.HasSettings(ctx)
	if err != nil {
		t.Fatalf("HasSettings() error = %v", err)
	}
	if has {
		t.Error("HasSettings() = true, want false")
	}

	// Add a setting
	s.SetSetting(ctx, "test", "key", "value", "string", false)

	has, _ = s.HasSettings(ctx)
	if !has {
		t.Error("HasSettings() = false, want true")
	}
}
