package storage

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStore_CreateUser(t *testing.T) {
	tests := []struct {
		name    string
		user    *User
		wantErr bool
	}{
		{
			name:    "valid user",
			user:    &User{Username: "testuser", Email: "test@example.com", Role: "admin"},
			wantErr: false,
		},
		{
			name:    "empty username",
			user:    &User{Username: "", Email: "test@example.com"},
			wantErr: false, // Username validation is done elsewhere
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewMemoryStore(nil)
			defer s.Close()

			err := s.CreateUser(context.Background(), tt.user)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if tt.user.ID == "" {
					t.Error("CreateUser() did not assign ID")
				}
				if tt.user.CreatedAt.IsZero() {
					t.Error("CreateUser() did not set CreatedAt")
				}
			}
		})
	}
}

func TestMemoryStore_CreateUser_Duplicate(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	ctx := context.Background()

	// Create first user
	user1 := &User{Username: "duplicate", Email: "test1@example.com"}
	if err := s.CreateUser(ctx, user1); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	// Attempt to create duplicate
	user2 := &User{Username: "duplicate", Email: "test2@example.com"}
	err := s.CreateUser(ctx, user2)
	if err != ErrDuplicate {
		t.Errorf("CreateUser() error = %v, want ErrDuplicate", err)
	}
}

func TestMemoryStore_GetUserByUsername(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	// Create a user
	user := &User{Username: "findme", Email: "find@example.com", Role: "user"}
	s.CreateUser(ctx, user)

	// Find the user
	found, err := s.GetUserByUsername(ctx, "findme")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	if found.Username != "findme" {
		t.Errorf("Username = %s, want findme", found.Username)
	}

	// Verify it's a copy (modifying shouldn't affect stored)
	found.Email = "modified@example.com"
	original, _ := s.GetUserByUsername(ctx, "findme")
	if original.Email == "modified@example.com" {
		t.Error("GetUserByUsername returned reference instead of copy")
	}
}

func TestMemoryStore_GetUserByUsername_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	_, err := s.GetUserByUsername(context.Background(), "nonexistent")
	if err != ErrNotFound {
		t.Errorf("GetUserByUsername() error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_GetUserByID(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	user := &User{Username: "byid", Email: "byid@example.com"}
	s.CreateUser(ctx, user)

	found, err := s.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if found.ID != user.ID {
		t.Errorf("ID = %s, want %s", found.ID, user.ID)
	}
}

func TestMemoryStore_ListUsers(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	// Create some users
	s.CreateUser(ctx, &User{Username: "user1"})
	s.CreateUser(ctx, &User{Username: "user2"})
	s.CreateUser(ctx, &User{Username: "user3"})

	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(users) != 3 {
		t.Errorf("len(users) = %d, want 3", len(users))
	}
}

func TestMemoryStore_CountUsers(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	count, _ := s.CountUsers(ctx)
	if count != 0 {
		t.Errorf("initial count = %d, want 0", count)
	}

	s.CreateUser(ctx, &User{Username: "user1"})
	s.CreateUser(ctx, &User{Username: "user2"})

	count, _ = s.CountUsers(ctx)
	if count != 2 {
		t.Errorf("count after creates = %d, want 2", count)
	}
}

func TestMemoryStore_UpdateUserByID(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	user := &User{Username: "original", Email: "orig@example.com"}
	s.CreateUser(ctx, user)

	// Update the user
	user.Email = "updated@example.com"
	err := s.UpdateUserByID(ctx, user)
	if err != nil {
		t.Fatalf("UpdateUserByID() error = %v", err)
	}

	// Verify update
	found, _ := s.GetUserByID(ctx, user.ID)
	if found.Email != "updated@example.com" {
		t.Errorf("Email = %s, want updated@example.com", found.Email)
	}
}

func TestMemoryStore_UpdateUserByID_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	err := s.UpdateUserByID(context.Background(), &User{ID: "nonexistent"})
	if err != ErrNotFound {
		t.Errorf("UpdateUserByID() error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_UpdateUserByID_ChangeUsername(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	user := &User{Username: "oldname"}
	s.CreateUser(ctx, user)

	// Change username
	user.Username = "newname"
	err := s.UpdateUserByID(ctx, user)
	if err != nil {
		t.Fatalf("UpdateUserByID() error = %v", err)
	}

	// Old username should not find
	_, err = s.GetUserByUsername(ctx, "oldname")
	if err != ErrNotFound {
		t.Error("Old username still exists")
	}

	// New username should find
	found, err := s.GetUserByUsername(ctx, "newname")
	if err != nil {
		t.Fatalf("GetUserByUsername(newname) error = %v", err)
	}
	if found.ID != user.ID {
		t.Error("New username points to wrong user")
	}
}

func TestMemoryStore_DeleteUser(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	user := &User{Username: "todelete"}
	s.CreateUser(ctx, user)

	err := s.DeleteUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	// User should not exist
	_, err = s.GetUserByID(ctx, user.ID)
	if err != ErrNotFound {
		t.Error("User still exists after delete")
	}
}

func TestMemoryStore_DeleteUser_CascadesSessions(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	user := &User{Username: "withsession"}
	s.CreateUser(ctx, user)

	session := &Session{
		Token:     "test-token",
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	s.CreateSession(ctx, session)

	// Delete user should cascade to session
	s.DeleteUser(ctx, user.ID)

	_, err := s.GetSessionByToken(ctx, "test-token")
	if err != ErrNotFound {
		t.Error("Session still exists after user delete")
	}
}

// --- Session tests ---

func TestMemoryStore_CreateSession(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	session := &Session{
		Token:     "session-token",
		UserID:    "user-1",
		ExpiresAt: time.Now().Add(time.Hour),
	}

	err := s.CreateSession(ctx, session)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if session.ID != "session-token" {
		t.Errorf("ID = %s, want session-token", session.ID)
	}
}

func TestMemoryStore_CreateSession_EmptyToken(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	err := s.CreateSession(context.Background(), &Session{Token: ""})
	if err != ErrValidation {
		t.Errorf("CreateSession() error = %v, want ErrValidation", err)
	}
}

func TestMemoryStore_GetSessionByToken_Expired(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	session := &Session{
		Token:     "expired-token",
		UserID:    "user-1",
		ExpiresAt: time.Now().Add(-time.Hour), // Already expired
	}
	s.CreateSession(ctx, session)

	_, err := s.GetSessionByToken(ctx, "expired-token")
	if err != ErrNotFound {
		t.Errorf("GetSessionByToken() for expired session error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_DeleteUserSessions(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	// Create sessions for two users
	s.CreateSession(ctx, &Session{Token: "user1-1", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)})
	s.CreateSession(ctx, &Session{Token: "user1-2", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)})
	s.CreateSession(ctx, &Session{Token: "user2-1", UserID: "user-2", ExpiresAt: time.Now().Add(time.Hour)})

	// Delete user 1's sessions
	err := s.DeleteUserSessions(ctx, "user-1")
	if err != nil {
		t.Fatalf("DeleteUserSessions() error = %v", err)
	}

	// User 1's sessions should be gone
	sessions, _ := s.ListUserSessions(ctx, "user-1")
	if len(sessions) != 0 {
		t.Errorf("user 1 sessions = %d, want 0", len(sessions))
	}

	// User 2's session should still exist
	sessions, _ = s.ListUserSessions(ctx, "user-2")
	if len(sessions) != 1 {
		t.Errorf("user 2 sessions = %d, want 1", len(sessions))
	}
}

// --- API Key tests ---

func TestMemoryStore_CreateAPIKey(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	key := &APIKey{
		UserID:    "user-1",
		Name:      "test-key",
		KeyHash:   "hash123",
		KeyPrefix: "vcd_",
	}

	err := s.CreateAPIKey(ctx, key)
	if err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	if key.ID == "" {
		t.Error("CreateAPIKey() did not assign ID")
	}
}

func TestMemoryStore_CreateAPIKey_EmptyHash(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	err := s.CreateAPIKey(context.Background(), &APIKey{KeyHash: ""})
	if err != ErrValidation {
		t.Errorf("CreateAPIKey() error = %v, want ErrValidation", err)
	}
}

func TestMemoryStore_GetAPIKeyByHash(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	key := &APIKey{UserID: "user-1", KeyHash: "findhash", Name: "findme"}
	s.CreateAPIKey(ctx, key)

	found, err := s.GetAPIKeyByHash(ctx, "findhash")
	if err != nil {
		t.Fatalf("GetAPIKeyByHash() error = %v", err)
	}
	if found.Name != "findme" {
		t.Errorf("Name = %s, want findme", found.Name)
	}
}

func TestMemoryStore_GetAPIKeyByHash_Expired(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	expiredTime := time.Now().Add(-time.Hour)
	key := &APIKey{UserID: "user-1", KeyHash: "expiredhash", ExpiresAt: &expiredTime}
	s.CreateAPIKey(ctx, key)

	_, err := s.GetAPIKeyByHash(ctx, "expiredhash")
	if err != ErrNotFound {
		t.Errorf("GetAPIKeyByHash() for expired key error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_ListAPIKeys(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateAPIKey(ctx, &APIKey{UserID: "user-1", KeyHash: "key1", Name: "key1"})
	s.CreateAPIKey(ctx, &APIKey{UserID: "user-1", KeyHash: "key2", Name: "key2"})
	s.CreateAPIKey(ctx, &APIKey{UserID: "user-2", KeyHash: "key3", Name: "key3"})

	keys, err := s.ListAPIKeys(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListAPIKeys() error = %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("len(keys) for user 1 = %d, want 2", len(keys))
	}
}

func TestMemoryStore_DeleteAPIKey(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	key := &APIKey{UserID: "user-1", KeyHash: "deletehash", Name: "todelete"}
	s.CreateAPIKey(ctx, key)

	err := s.DeleteAPIKey(ctx, key.ID)
	if err != nil {
		t.Fatalf("DeleteAPIKey() error = %v", err)
	}

	_, err = s.GetAPIKeyByHash(ctx, "deletehash")
	if err != ErrNotFound {
		t.Error("API key still exists after delete")
	}
}

func TestMemoryStore_UpdateAPIKeyUsage(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	key := &APIKey{UserID: "user-1", KeyHash: "usagehash", Name: "usage"}
	s.CreateAPIKey(ctx, key)

	err := s.UpdateAPIKeyUsage(ctx, key.ID)
	if err != nil {
		t.Fatalf("UpdateAPIKeyUsage() error = %v", err)
	}

	found, _ := s.GetAPIKeyByHash(ctx, "usagehash")
	if found.LastUsedAt == nil {
		t.Error("LastUsedAt not updated")
	}
}
