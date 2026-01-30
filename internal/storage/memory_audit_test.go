package storage

import (
	"context"
	"testing"
	"time"
)

// --- AuditEntry tests ---

func TestMemoryStore_LogAudit(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	entry := &AuditEntry{
		Source:   "server",
		User:     "admin",
		Action:   "project.create",
		Resource: "project",
	}
	err := s.LogAudit(ctx, entry)
	if err != nil {
		t.Fatalf("LogAudit() error = %v", err)
	}

	if entry.ID == 0 {
		t.Error("LogAudit() did not assign ID")
	}
	if entry.Timestamp.IsZero() {
		t.Error("LogAudit() did not set Timestamp")
	}
}

func TestMemoryStore_LogAuditWithSnapshot(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	resource := map[string]string{"name": "myproject", "status": "active"}
	entry := &AuditEntry{
		Action:   "project.delete",
		Resource: "project",
	}

	err := s.LogAuditWithSnapshot(ctx, entry, resource)
	if err != nil {
		t.Fatalf("LogAuditWithSnapshot() error = %v", err)
	}

	if entry.ResourceData == "" {
		t.Error("ResourceData was not set")
	}
}

func TestMemoryStore_ListAuditLogs(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.LogAudit(ctx, &AuditEntry{Action: "action1"})
	s.LogAudit(ctx, &AuditEntry{Action: "action2"})
	s.LogAudit(ctx, &AuditEntry{Action: "action3"})

	logs, err := s.ListAuditLogs(ctx, 2, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("len(logs) = %d, want 2", len(logs))
	}
	// Should be newest first
	if logs[0].Action != "action3" {
		t.Errorf("First action = %s, want action3 (newest first)", logs[0].Action)
	}
}

func TestMemoryStore_ListAuditLogs_Pagination(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.LogAudit(ctx, &AuditEntry{Action: "action"})
	}

	logs, _ := s.ListAuditLogs(ctx, 2, 2)
	if len(logs) != 2 {
		t.Errorf("len(logs) = %d, want 2", len(logs))
	}
}

func TestMemoryStore_ListAuditLogsSince(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	oldTime := time.Now().Add(-time.Hour)
	s.LogAudit(ctx, &AuditEntry{Action: "old", Timestamp: oldTime})
	time.Sleep(time.Millisecond)

	cutoff := time.Now().Add(-time.Minute)
	s.LogAudit(ctx, &AuditEntry{Action: "new1"})
	s.LogAudit(ctx, &AuditEntry{Action: "new2"})

	logs, err := s.ListAuditLogsSince(ctx, cutoff)
	if err != nil {
		t.Fatalf("ListAuditLogsSince() error = %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("len(logs) = %d, want 2", len(logs))
	}
}

// --- Setting tests ---

func TestMemoryStore_SetSetting_Create(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	err := s.SetSetting(ctx, "email", "smtp_host", "smtp.example.com", "string", false)
	if err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}

	setting, err := s.GetSetting(ctx, "email", "smtp_host")
	if err != nil {
		t.Fatalf("GetSetting() error = %v", err)
	}
	if setting.Value != "smtp.example.com" {
		t.Errorf("Value = %s, want smtp.example.com", setting.Value)
	}
}

func TestMemoryStore_SetSetting_Update(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.SetSetting(ctx, "email", "smtp_host", "old.example.com", "string", false)
	s.SetSetting(ctx, "email", "smtp_host", "new.example.com", "string", false)

	setting, _ := s.GetSetting(ctx, "email", "smtp_host")
	if setting.Value != "new.example.com" {
		t.Errorf("Value = %s, want new.example.com", setting.Value)
	}
}

func TestMemoryStore_GetSetting_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	_, err := s.GetSetting(context.Background(), "nonexistent", "key")
	if err != ErrNotFound {
		t.Errorf("GetSetting() error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_ListSettingsByCategory(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.SetSetting(ctx, "email", "smtp_host", "value1", "string", false)
	s.SetSetting(ctx, "email", "smtp_port", "value2", "string", false)
	s.SetSetting(ctx, "other", "key", "value3", "string", false)

	settings, err := s.ListSettingsByCategory(ctx, "email")
	if err != nil {
		t.Fatalf("ListSettingsByCategory() error = %v", err)
	}
	if len(settings) != 2 {
		t.Errorf("len(settings) = %d, want 2", len(settings))
	}
}

func TestMemoryStore_ListAllSettings(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.SetSetting(ctx, "email", "smtp_host", "value1", "string", false)
	s.SetSetting(ctx, "security", "key", "value2", "string", true)

	settings, err := s.ListAllSettings(ctx)
	if err != nil {
		t.Fatalf("ListAllSettings() error = %v", err)
	}
	if len(settings) != 2 {
		t.Errorf("len(settings) = %d, want 2", len(settings))
	}
}

func TestMemoryStore_DeleteSetting(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.SetSetting(ctx, "email", "delete_me", "value", "string", false)

	err := s.DeleteSetting(ctx, "email", "delete_me")
	if err != nil {
		t.Fatalf("DeleteSetting() error = %v", err)
	}

	_, err = s.GetSetting(ctx, "email", "delete_me")
	if err != ErrNotFound {
		t.Error("Setting still exists after delete")
	}
}

func TestMemoryStore_DeleteSetting_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	err := s.DeleteSetting(context.Background(), "nonexistent", "key")
	if err != ErrNotFound {
		t.Errorf("DeleteSetting() error = %v, want ErrNotFound", err)
	}
}
