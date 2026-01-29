package sessions

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services/testutil"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func newTestService(t *testing.T) (*Service, *storage.DB) {
	t.Helper()

	db, cleanup := testutil.NewTestDB(t)
	t.Cleanup(cleanup)

	return New(db), db
}

func createTestUser(t *testing.T, db *storage.DB) int64 {
	t.Helper()
	ctx := context.Background()

	user := &storage.User{
		Username:     "testuser",
		PasswordHash: "testhash",
		Email:        "test@example.com",
		Role:         "user",
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	return user.ID
}

func TestService_Create(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	session, err := svc.Create(ctx, userID, "127.0.0.1", "Mozilla/5.0", time.Hour)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if session.ID == "" {
		t.Error("Create() did not set session ID")
	}
	if session.Token == "" {
		t.Error("Create() did not set token")
	}
	if session.UserID != userID {
		t.Errorf("Create() userID = %v, want %v", session.UserID, userID)
	}
	if session.IPAddress != "127.0.0.1" {
		t.Errorf("Create() IPAddress = %v, want %v", session.IPAddress, "127.0.0.1")
	}
	if session.UserAgent != "Mozilla/5.0" {
		t.Errorf("Create() UserAgent = %v, want %v", session.UserAgent, "Mozilla/5.0")
	}
	if session.ExpiresAt.Before(time.Now()) {
		t.Error("Create() ExpiresAt should be in the future")
	}
}

func TestService_GetByToken(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	created, err := svc.Create(ctx, userID, "127.0.0.1", "Mozilla/5.0", time.Hour)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	session, err := svc.GetByToken(ctx, created.Token)
	if err != nil {
		t.Fatalf("GetByToken() error = %v", err)
	}
	if session == nil {
		t.Fatal("GetByToken() returned nil")
	}
	if session.Token != created.Token {
		t.Errorf("GetByToken() token = %v, want %v", session.Token, created.Token)
	}
}

func TestService_GetByToken_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.GetByToken(ctx, "nonexistent-token")
	if err == nil {
		t.Error("GetByToken() expected error for nonexistent token")
	}
}

func TestService_Delete(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	session, err := svc.Create(ctx, userID, "127.0.0.1", "Mozilla/5.0", time.Hour)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := svc.Delete(ctx, session.Token); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = svc.GetByToken(ctx, session.Token)
	if err == nil {
		t.Error("GetByToken() expected error after delete")
	}
}

func TestService_DeleteAllForUser(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	for i := 0; i < 3; i++ {
		_, err := svc.Create(ctx, userID, "127.0.0.1", "Mozilla/5.0", time.Hour)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	sessions, err := svc.ListForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListForUser() error = %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("ListForUser() count = %v, want 3", len(sessions))
	}

	if err := svc.DeleteAllForUser(ctx, userID); err != nil {
		t.Fatalf("DeleteAllForUser() error = %v", err)
	}

	sessions, err = svc.ListForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListForUser() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("ListForUser() after delete count = %v, want 0", len(sessions))
	}
}

func TestService_DeleteExpired(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	expiredSession := &storage.Session{
		ID:        "expired-token",
		Token:     "expired-token",
		UserID:    userID,
		IPAddress: "127.0.0.1",
		UserAgent: "Mozilla/5.0",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	if err := db.CreateSession(ctx, expiredSession); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	_, err := svc.Create(ctx, userID, "127.0.0.1", "Mozilla/5.0", time.Hour)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	count, err := svc.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}
	if count != 1 {
		t.Errorf("DeleteExpired() count = %v, want 1", count)
	}

	sessions, err := svc.ListForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListForUser() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("ListForUser() count = %v, want 1", len(sessions))
	}
}

func TestService_ListForUser(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	for i := 0; i < 5; i++ {
		_, err := svc.Create(ctx, userID, "127.0.0.1", "Mozilla/5.0", time.Hour)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	sessions, err := svc.ListForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListForUser() error = %v", err)
	}
	if len(sessions) != 5 {
		t.Errorf("ListForUser() count = %v, want 5", len(sessions))
	}

	for _, s := range sessions {
		if s.UserID != userID {
			t.Errorf("ListForUser() session userID = %v, want %v", s.UserID, userID)
		}
	}
}

func TestService_ListForUser_Empty(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	sessions, err := svc.ListForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListForUser() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("ListForUser() count = %v, want 0", len(sessions))
	}
}
