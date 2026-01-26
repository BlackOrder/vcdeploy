package audit

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

func newTestService(t *testing.T) *Service {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := zap.NewNop()
	db, err := storage.New(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return New(db)
}

func TestService_Log(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	entry := &storage.AuditEntry{
		Source:    "test",
		User:      "testuser",
		Action:    "login",
		Resource:  "auth",
		Details:   "User logged in",
		IPAddress: "127.0.0.1",
		Result:    "success",
	}

	if err := svc.Log(ctx, entry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	entries, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List() count = %v, want 1", len(entries))
	}

	logged := entries[0]
	if logged.Source != "test" {
		t.Errorf("Log() source = %v, want %v", logged.Source, "test")
	}
	if logged.User != "testuser" {
		t.Errorf("Log() user = %v, want %v", logged.User, "testuser")
	}
	if logged.Action != "login" {
		t.Errorf("Log() action = %v, want %v", logged.Action, "login")
	}
}

func TestService_Log_SetsTimestamp(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	entry := &storage.AuditEntry{
		Source: "test",
		User:   "testuser",
		Action: "test",
	}

	if err := svc.Log(ctx, entry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	entries, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if entries[0].Timestamp.IsZero() {
		t.Error("Log() should set timestamp if not provided")
	}
}

func TestService_LogAction(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	err := svc.LogAction(ctx, "web", "admin", "delete", "user:123", "Deleted user", "192.168.1.1", "success")
	if err != nil {
		t.Fatalf("LogAction() error = %v", err)
	}

	entries, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List() count = %v, want 1", len(entries))
	}

	entry := entries[0]
	if entry.Source != "web" {
		t.Errorf("LogAction() source = %v, want %v", entry.Source, "web")
	}
	if entry.User != "admin" {
		t.Errorf("LogAction() user = %v, want %v", entry.User, "admin")
	}
	if entry.Action != "delete" {
		t.Errorf("LogAction() action = %v, want %v", entry.Action, "delete")
	}
	if entry.Resource != "user:123" {
		t.Errorf("LogAction() resource = %v, want %v", entry.Resource, "user:123")
	}
	if entry.Details != "Deleted user" {
		t.Errorf("LogAction() details = %v, want %v", entry.Details, "Deleted user")
	}
	if entry.IPAddress != "192.168.1.1" {
		t.Errorf("LogAction() ipAddress = %v, want %v", entry.IPAddress, "192.168.1.1")
	}
	if entry.Result != "success" {
		t.Errorf("LogAction() result = %v, want %v", entry.Result, "success")
	}
}

func TestService_List(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		entry := &storage.AuditEntry{
			Source: "test",
			User:   "testuser",
			Action: "action",
		}
		if err := svc.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	entries, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 5 {
		t.Errorf("List() count = %v, want 5", len(entries))
	}
}

func TestService_List_Pagination(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		entry := &storage.AuditEntry{
			Source: "test",
			User:   "testuser",
			Action: "action",
		}
		if err := svc.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	entries, err := svc.List(ctx, 3, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("List() with limit count = %v, want 3", len(entries))
	}

	entries, err = svc.List(ctx, 3, 3)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("List() with offset count = %v, want 3", len(entries))
	}
}

func TestService_List_DefaultLimit(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("List() with zero limit error = %v", err)
	}

	_, err = svc.List(ctx, -1, 0)
	if err != nil {
		t.Fatalf("List() with negative limit error = %v", err)
	}
}

func TestService_List_NegativeOffset(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.List(ctx, 10, -5)
	if err != nil {
		t.Fatalf("List() with negative offset error = %v", err)
	}
}

func TestService_Cleanup(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	oldEntry := &storage.AuditEntry{
		Source:    "test",
		User:      "testuser",
		Action:    "old-action",
		Timestamp: time.Now().Add(-48 * time.Hour),
	}
	if err := svc.Log(ctx, oldEntry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	newEntry := &storage.AuditEntry{
		Source: "test",
		User:   "testuser",
		Action: "new-action",
	}
	if err := svc.Log(ctx, newEntry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	count, err := svc.Cleanup(ctx, cutoff)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if count != 1 {
		t.Errorf("Cleanup() count = %v, want 1", count)
	}

	entries, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("List() after cleanup count = %v, want 1", len(entries))
	}
	if entries[0].Action != "new-action" {
		t.Errorf("Cleanup() removed wrong entry, action = %v", entries[0].Action)
	}
}

func TestService_Cleanup_NoOldEntries(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	entry := &storage.AuditEntry{
		Source: "test",
		User:   "testuser",
		Action: "action",
	}
	if err := svc.Log(ctx, entry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	count, err := svc.Cleanup(ctx, cutoff)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if count != 0 {
		t.Errorf("Cleanup() count = %v, want 0", count)
	}
}
