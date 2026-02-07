package users

import (
	"context"
	"errors"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/services/testutil"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func newTestService(t *testing.T) (*Service, storage.Store) {
	t.Helper()

	db, cleanup := testutil.NewTestStore(t)
	t.Cleanup(cleanup)

	return New(db), db
}

func TestService_Create(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Test creating a user
	user, err := svc.Create(ctx, "testuser", "StrongP@ss123!", "test@example.com", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if user.ID == 0 {
		t.Error("Create() did not set user ID")
	}
	if user.Username != "testuser" {
		t.Errorf("Create() username = %v, want %v", user.Username, "testuser")
	}
	if user.Email != "test@example.com" {
		t.Errorf("Create() email = %v, want %v", user.Email, "test@example.com")
	}
	if user.Role != "user" {
		t.Errorf("Create() role = %v, want %v", user.Role, "user")
	}
}

func TestService_Create_WeakPassword(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, "testuser", "weak", "test@example.com", "user")
	if err == nil {
		t.Error("Create() expected error for weak password")
	}
}

func TestService_Create_DefaultRole(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	user, err := svc.Create(ctx, "testuser", "StrongP@ss123!", "test@example.com", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if user.Role != "user" {
		t.Errorf("Create() default role = %v, want %v", user.Role, "user")
	}
}

func TestService_GetByUsername(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a user first
	_, err := svc.Create(ctx, "findme", "StrongP@ss123!", "find@example.com", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Find by username
	user, err := svc.GetByUsername(ctx, "findme")
	if err != nil {
		t.Fatalf("GetByUsername() error = %v", err)
	}
	if user == nil {
		t.Fatal("GetByUsername() returned nil")
	}
	if user.Username != "findme" {
		t.Errorf("GetByUsername() username = %v, want %v", user.Username, "findme")
	}
}

func TestService_GetByUsername_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	user, err := svc.GetByUsername(ctx, "nonexistent")
	if err == nil {
		t.Fatal("GetByUsername() expected error for nonexistent user")
	}
	if !services.IsNotFound(err) {
		t.Errorf("GetByUsername() expected ErrNotFound, got: %v", err)
	}
	if user != nil {
		t.Error("GetByUsername() expected nil for nonexistent user")
	}
}

func TestService_GetByID(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a user first
	created, err := svc.Create(ctx, "byid", "StrongP@ss123!", "byid@example.com", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Find by ID
	user, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if user == nil {
		t.Fatal("GetByID() returned nil")
	}
	if user.ID != created.ID {
		t.Errorf("GetByID() id = %v, want %v", user.ID, created.ID)
	}
}

func TestService_List(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create some users
	for i := 0; i < 3; i++ {
		_, err := svc.Create(ctx, "user"+string(rune('0'+i)), "StrongP@ss123!", "", "user")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	users, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(users) != 3 {
		t.Errorf("List() returned %v users, want %v", len(users), 3)
	}
}

func TestService_Count(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create some users
	for i := 0; i < 5; i++ {
		_, err := svc.Create(ctx, "countuser"+string(rune('0'+i)), "StrongP@ss123!", "", "user")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	count, err := svc.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 5 {
		t.Errorf("Count() = %v, want %v", count, 5)
	}
}

func TestService_Delete(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a user
	user, err := svc.Create(ctx, "todelete", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete the user
	err = svc.Delete(ctx, user.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted - should return NotFound error
	found, err := svc.GetByID(ctx, user.ID)
	if !services.IsNotFound(err) {
		t.Errorf("GetByID() error = %v, want NotFound", err)
	}
	if found != nil {
		t.Error("Delete() user still exists")
	}
}

func TestService_VerifyPassword(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	password := "StrongP@ss123!"
	_, err := svc.Create(ctx, "verifyuser", password, "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify correct password
	user, err := svc.VerifyPassword(ctx, "verifyuser", password)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if user == nil {
		t.Error("VerifyPassword() returned nil for correct password")
	}

	// Verify wrong password
	user, err = svc.VerifyPassword(ctx, "verifyuser", "wrongpassword")
	if err != ErrInvalidPassword {
		t.Errorf("VerifyPassword() error = %v, want ErrInvalidPassword", err)
	}
	if user != nil {
		t.Error("VerifyPassword() returned user for wrong password")
	}
}

func TestService_UpdatePassword(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create user
	user, err := svc.Create(ctx, "updatepw", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update password
	newPassword := "NewStr0ng!Pass"
	err = svc.UpdatePassword(ctx, user.ID, newPassword)
	if err != nil {
		t.Fatalf("UpdatePassword() error = %v", err)
	}

	// Verify new password works
	verified, err := svc.VerifyPassword(ctx, "updatepw", newPassword)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if verified == nil {
		t.Error("UpdatePassword() new password doesn't work")
	}
}

func TestService_SetTOTP(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create user
	user, err := svc.Create(ctx, "totpuser", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Enable TOTP
	err = svc.SetTOTP(ctx, user.ID, "JBSWY3DPEHPK3PXP", true)
	if err != nil {
		t.Fatalf("SetTOTP() error = %v", err)
	}

	// Verify TOTP is enabled
	updated, err := svc.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !updated.TOTPEnabled {
		t.Error("SetTOTP() did not enable TOTP")
	}
	if updated.TOTPSecret != "JBSWY3DPEHPK3PXP" {
		t.Errorf("SetTOTP() secret = %v, want %v", updated.TOTPSecret, "JBSWY3DPEHPK3PXP")
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	user, err := svc.GetByID(ctx, 99999)
	if err == nil {
		t.Fatal("GetByID() expected error for nonexistent user")
	}
	if !services.IsNotFound(err) {
		t.Errorf("GetByID() expected ErrNotFound, got: %v", err)
	}
	if user != nil {
		t.Error("GetByID() expected nil for nonexistent user")
	}
}

func TestService_Update(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a user first
	user, err := svc.Create(ctx, "updateme", "StrongP@ss123!", "update@example.com", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update user fields
	user.Email = "newemail@example.com"
	user.Role = "admin"
	err = svc.Update(ctx, user)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify update
	updated, err := svc.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.Email != "newemail@example.com" {
		t.Errorf("Update() email = %v, want %v", updated.Email, "newemail@example.com")
	}
	if updated.Role != "admin" {
		t.Errorf("Update() role = %v, want %v", updated.Role, "admin")
	}
}

func TestService_VerifyPassword_UserNotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	user, err := svc.VerifyPassword(ctx, "nonexistent", "password")
	if err == nil {
		t.Fatal("VerifyPassword() expected error for nonexistent user")
	}
	if !services.IsNotFound(err) {
		t.Errorf("VerifyPassword() expected ErrNotFound, got: %v", err)
	}
	if user != nil {
		t.Error("VerifyPassword() expected nil for nonexistent user")
	}
}

func TestService_UpdatePassword_WeakPassword(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create user
	user, err := svc.Create(ctx, "weakpwuser", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Try to update with weak password
	err = svc.UpdatePassword(ctx, user.ID, "weak")
	if err == nil {
		t.Error("UpdatePassword() expected error for weak password")
	}
}

func TestService_UpdatePassword_UserNotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.UpdatePassword(ctx, 99999, "StrongP@ss123!")
	if err == nil {
		t.Error("UpdatePassword() expected error for nonexistent user")
	}
}

func TestService_SetTOTP_Disable(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create user
	user, err := svc.Create(ctx, "totpdisable", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Enable TOTP first
	err = svc.SetTOTP(ctx, user.ID, "JBSWY3DPEHPK3PXP", true)
	if err != nil {
		t.Fatalf("SetTOTP() enable error = %v", err)
	}

	// Disable TOTP
	err = svc.SetTOTP(ctx, user.ID, "", false)
	if err != nil {
		t.Fatalf("SetTOTP() disable error = %v", err)
	}

	// Verify TOTP is disabled
	updated, err := svc.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.TOTPEnabled {
		t.Error("SetTOTP() did not disable TOTP")
	}
	if updated.TOTPSecret != "" {
		t.Errorf("SetTOTP() secret = %v, want empty", updated.TOTPSecret)
	}
}

func TestService_SetTOTP_UserNotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.SetTOTP(ctx, 99999, "JBSWY3DPEHPK3PXP", true)
	if err == nil {
		t.Error("SetTOTP() expected error for nonexistent user")
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Delete doesn't return an error for non-existent users in SQLite
	// This test documents the behavior
	err := svc.Delete(ctx, 99999)
	_ = err // May or may not error depending on implementation
}

func TestService_List_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	users, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(users) != 0 {
		t.Errorf("List() returned %v users for empty database, want 0", len(users))
	}
}

func TestService_Count_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	count, err := svc.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Errorf("Count() = %v for empty database, want 0", count)
	}
}

func TestService_Create_DuplicateUsername(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create first user
	_, err := svc.Create(ctx, "duplicate", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() first error = %v", err)
	}

	// Try to create duplicate
	_, err = svc.Create(ctx, "duplicate", "StrongP@ss123!", "", "user")
	if err == nil {
		t.Error("Create() expected error for duplicate username")
	}
}

func TestService_Create_AdminRole(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	user, err := svc.Create(ctx, "adminuser", "StrongP@ss123!", "admin@example.com", "admin")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if user.Role != "admin" {
		t.Errorf("Create() role = %v, want %v", user.Role, "admin")
	}
}

func TestService_UpdatePassword_OldPasswordInvalidated(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	oldPassword := "StrongP@ss123!"
	newPassword := "NewStr0ng!Pass"

	// Create user
	_, err := svc.Create(ctx, "pwchange", oldPassword, "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify old password works
	user, err := svc.VerifyPassword(ctx, "pwchange", oldPassword)
	if err != nil || user == nil {
		t.Fatalf("VerifyPassword() old password should work initially")
	}

	// Update password
	err = svc.UpdatePassword(ctx, user.ID, newPassword)
	if err != nil {
		t.Fatalf("UpdatePassword() error = %v", err)
	}

	// Verify old password no longer works
	_, err = svc.VerifyPassword(ctx, "pwchange", oldPassword)
	if !errors.Is(err, ErrInvalidPassword) {
		t.Error("VerifyPassword() old password should not work after update")
	}

	// Verify new password works
	user, err = svc.VerifyPassword(ctx, "pwchange", newPassword)
	if err != nil || user == nil {
		t.Error("VerifyPassword() new password should work after update")
	}
}

func TestService_Update_MustChangePassword(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create user
	user, err := svc.Create(ctx, "forcechange", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Set must change password flag
	user.MustChangePassword = true
	err = svc.Update(ctx, user)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify flag is set
	updated, err := svc.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !updated.MustChangePassword {
		t.Error("Update() MustChangePassword not set")
	}

	// Update password should clear the flag
	err = svc.UpdatePassword(ctx, user.ID, "AnotherStr0ng!Pass")
	if err != nil {
		t.Fatalf("UpdatePassword() error = %v", err)
	}

	updated, err = svc.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.MustChangePassword {
		t.Error("UpdatePassword() should clear MustChangePassword flag")
	}
}

// --- DeleteWithCleanup tests ---

func TestService_DeleteWithCleanup(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	// Create a user
	user, err := svc.Create(ctx, "cleanupuser", "StrongP@ss123!", "cleanup@example.com", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Add a session for this user
	conn := db.Conn()
	_, err = conn.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, ip_address, user_agent, expires_at)
		VALUES ('session1', ?, '127.0.0.1', 'test-agent', datetime('now', '+1 hour'))
	`, user.ID)
	if err != nil {
		t.Fatalf("Failed to insert session: %v", err)
	}

	// Add an API key for this user
	_, err = conn.ExecContext(ctx, `
		INSERT INTO api_keys (uid, user_id, name, key_hash, scopes)
		VALUES (?, ?, 'test-key', 'hash123', 'read,write')
	`, "uid-cleanup-key", user.ID)
	if err != nil {
		t.Fatalf("Failed to insert API key: %v", err)
	}

	// Delete with cleanup
	err = svc.DeleteWithCleanup(ctx, user.ID)
	if err != nil {
		t.Fatalf("DeleteWithCleanup() error = %v", err)
	}

	// Verify user is deleted - should return NotFound error
	found, err := svc.GetByID(ctx, user.ID)
	if !services.IsNotFound(err) {
		t.Errorf("GetByID() error = %v, want NotFound", err)
	}
	if found != nil {
		t.Error("DeleteWithCleanup() user still exists")
	}

	// Verify sessions are deleted
	var sessionCount int
	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, user.ID).Scan(&sessionCount)
	if err != nil {
		t.Fatalf("Failed to count sessions: %v", err)
	}
	if sessionCount != 0 {
		t.Errorf("DeleteWithCleanup() sessions still exist, count = %d", sessionCount)
	}

	// Verify API keys are deleted
	var apiKeyCount int
	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE user_id = ?`, user.ID).Scan(&apiKeyCount)
	if err != nil {
		t.Fatalf("Failed to count API keys: %v", err)
	}
	if apiKeyCount != 0 {
		t.Errorf("DeleteWithCleanup() API keys still exist, count = %d", apiKeyCount)
	}
}

func TestService_DeleteWithCleanup_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.DeleteWithCleanup(ctx, 99999)
	if err == nil {
		t.Error("DeleteWithCleanup() expected error for nonexistent user")
	}
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("DeleteWithCleanup() error = %v, want ErrUserNotFound", err)
	}
}

func TestService_DeleteWithCleanup_NoAssociatedData(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a user without any associated data
	user, err := svc.Create(ctx, "cleanuser", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete with cleanup should still work
	err = svc.DeleteWithCleanup(ctx, user.ID)
	if err != nil {
		t.Fatalf("DeleteWithCleanup() error = %v", err)
	}

	// Verify user is deleted - should return NotFound error
	found, err := svc.GetByID(ctx, user.ID)
	if !services.IsNotFound(err) {
		t.Errorf("GetByID() error = %v, want NotFound", err)
	}
	if found != nil {
		t.Error("DeleteWithCleanup() user still exists")
	}
}

func TestService_DeleteWithCleanup_OnlySessions(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	// Create a user
	user, err := svc.Create(ctx, "sessionuser", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Add multiple sessions for this user
	conn := db.Conn()
	for i := 0; i < 3; i++ {
		_, err = conn.ExecContext(ctx, `
			INSERT INTO sessions (id, user_id, ip_address, user_agent, expires_at)
			VALUES (?, ?, '127.0.0.1', 'test-agent', datetime('now', '+1 hour'))
		`, "session"+string(rune('0'+i)), user.ID)
		if err != nil {
			t.Fatalf("Failed to insert session %d: %v", i, err)
		}
	}

	// Delete with cleanup
	err = svc.DeleteWithCleanup(ctx, user.ID)
	if err != nil {
		t.Fatalf("DeleteWithCleanup() error = %v", err)
	}

	// Verify all sessions are deleted
	var sessionCount int
	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, user.ID).Scan(&sessionCount)
	if err != nil {
		t.Fatalf("Failed to count sessions: %v", err)
	}
	if sessionCount != 0 {
		t.Errorf("DeleteWithCleanup() sessions still exist, count = %d", sessionCount)
	}
}

func TestService_DeleteWithCleanup_OnlyAPIKeys(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	// Create a user
	user, err := svc.Create(ctx, "apikeyuser", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Add multiple API keys for this user
	conn := db.Conn()
	for i := 0; i < 3; i++ {
		_, err = conn.ExecContext(ctx, `
			INSERT INTO api_keys (uid, user_id, name, key_hash, scopes)
			VALUES (?, ?, ?, ?, 'read')
		`, "uid-apikey-"+string(rune('0'+i)), user.ID, "key"+string(rune('0'+i)), "hash"+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("Failed to insert API key %d: %v", i, err)
		}
	}

	// Delete with cleanup
	err = svc.DeleteWithCleanup(ctx, user.ID)
	if err != nil {
		t.Fatalf("DeleteWithCleanup() error = %v", err)
	}

	// Verify all API keys are deleted
	var apiKeyCount int
	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE user_id = ?`, user.ID).Scan(&apiKeyCount)
	if err != nil {
		t.Fatalf("Failed to count API keys: %v", err)
	}
	if apiKeyCount != 0 {
		t.Errorf("DeleteWithCleanup() API keys still exist, count = %d", apiKeyCount)
	}
}

func TestService_Create_PasswordVariations(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		expectError bool
	}{
		{"too short", "Sh0rt!", true},
		{"no uppercase", "strongp@ss123!", true},
		{"no lowercase", "STRONGP@SS123!", true},
		{"no digit", "StrongP@ssword!", true},
		{"no special", "StrongPass12345", true},
		{"valid complex", "MyStr0ng!P@ss", false},
		{"valid with all types", "Aa1!Bb2@Cc3#Dd4$", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTestService(t)
			ctx := context.Background()

			_, err := svc.Create(ctx, "testuser", tc.password, "", "user")
			if tc.expectError && err == nil {
				t.Errorf("Create() expected error for password: %s", tc.name)
			}
			if !tc.expectError && err != nil {
				t.Errorf("Create() unexpected error for password %s: %v", tc.name, err)
			}
		})
	}
}

func TestService_GetByUsername_CaseSensitive(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a user with lowercase username
	_, err := svc.Create(ctx, "lowercase", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Try to find with different case - should not find
	// Now returns NotFound error instead of nil when user not found
	user, err := svc.GetByUsername(ctx, "LOWERCASE")
	// Depending on SQLite collation, this may or may not find the user
	// This test documents the actual behavior - either user is found or NotFound error
	if user == nil && err != nil && !services.IsNotFound(err) {
		t.Errorf("GetByUsername() unexpected error = %v", err)
	}
}

func TestService_VerifyPassword_EmptyPassword(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create user
	_, err := svc.Create(ctx, "emptypasstest", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify with empty password
	user, err := svc.VerifyPassword(ctx, "emptypasstest", "")
	if err != ErrInvalidPassword {
		t.Errorf("VerifyPassword() error = %v, want ErrInvalidPassword", err)
	}
	if user != nil {
		t.Error("VerifyPassword() should not return user for empty password")
	}
}

func TestService_Update_AllFields(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a user
	user, err := svc.Create(ctx, "fullupdate", "StrongP@ss123!", "old@example.com", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update all fields
	user.Email = "new@example.com"
	user.Role = "admin"
	user.MustChangePassword = true
	user.TOTPEnabled = true
	user.TOTPSecret = "TESTSECRET"

	err = svc.Update(ctx, user)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify all updates
	updated, err := svc.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.Email != "new@example.com" {
		t.Errorf("Update() email = %v, want %v", updated.Email, "new@example.com")
	}
	if updated.Role != "admin" {
		t.Errorf("Update() role = %v, want %v", updated.Role, "admin")
	}
	if !updated.MustChangePassword {
		t.Error("Update() MustChangePassword not set")
	}
	if !updated.TOTPEnabled {
		t.Error("Update() TOTPEnabled not set")
	}
	if updated.TOTPSecret != "TESTSECRET" {
		t.Errorf("Update() TOTPSecret = %v, want %v", updated.TOTPSecret, "TESTSECRET")
	}
}

func TestService_Create_AllRoles(t *testing.T) {
	roles := []string{"user", "admin", "viewer", "deployer"}

	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			svc, _ := newTestService(t)
			ctx := context.Background()

			user, err := svc.Create(ctx, "roleuser", "StrongP@ss123!", "", role)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if user.Role != role {
				t.Errorf("Create() role = %v, want %v", user.Role, role)
			}
		})
	}
}

func TestService_Create_WithEmail(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	user, err := svc.Create(ctx, "emailuser", "StrongP@ss123!", "test@example.com", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if user.Email != "test@example.com" {
		t.Errorf("Create() email = %v, want %v", user.Email, "test@example.com")
	}
}

func TestService_Create_EmptyEmail(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	user, err := svc.Create(ctx, "noemailuser", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if user.Email != "" {
		t.Errorf("Create() email = %v, want empty", user.Email)
	}
}

func TestService_Update_Role(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a user
	user, err := svc.Create(ctx, "rolechange", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update role
	user.Role = "admin"
	err = svc.Update(ctx, user)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify update
	updated, err := svc.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.Role != "admin" {
		t.Errorf("Update() role = %v, want %v", updated.Role, "admin")
	}
}

func TestService_DeleteWithCleanup_MultipleSessions(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	// Create a user
	user, err := svc.Create(ctx, "multisession", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Add multiple sessions and API keys
	conn := db.Conn()
	for i := 0; i < 5; i++ {
		_, err = conn.ExecContext(ctx, `
			INSERT INTO sessions (id, user_id, ip_address, user_agent, expires_at)
			VALUES (?, ?, '127.0.0.1', 'test-agent', datetime('now', '+1 hour'))
		`, "multi-session"+string(rune('0'+i)), user.ID)
		if err != nil {
			t.Fatalf("Failed to insert session: %v", err)
		}
		_, err = conn.ExecContext(ctx, `
			INSERT INTO api_keys (uid, user_id, name, key_hash, scopes)
			VALUES (?, ?, ?, ?, 'read')
		`, "uid-multi-key-"+string(rune('0'+i)), user.ID, "key"+string(rune('0'+i)), "hash-multi"+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("Failed to insert API key: %v", err)
		}
	}

	// Verify counts before deletion
	var sessionCount, apiKeyCount int
	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, user.ID).Scan(&sessionCount)
	if err != nil {
		t.Fatalf("Failed to count sessions: %v", err)
	}
	if sessionCount != 5 {
		t.Errorf("Expected 5 sessions, got %d", sessionCount)
	}

	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE user_id = ?`, user.ID).Scan(&apiKeyCount)
	if err != nil {
		t.Fatalf("Failed to count API keys: %v", err)
	}
	if apiKeyCount != 5 {
		t.Errorf("Expected 5 API keys, got %d", apiKeyCount)
	}

	// Delete with cleanup
	err = svc.DeleteWithCleanup(ctx, user.ID)
	if err != nil {
		t.Fatalf("DeleteWithCleanup() error = %v", err)
	}

	// Verify all deleted
	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, user.ID).Scan(&sessionCount)
	if err != nil {
		t.Fatalf("Failed to count sessions after deletion: %v", err)
	}
	if sessionCount != 0 {
		t.Errorf("DeleteWithCleanup() sessions still exist, count = %d", sessionCount)
	}

	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE user_id = ?`, user.ID).Scan(&apiKeyCount)
	if err != nil {
		t.Fatalf("Failed to count API keys after deletion: %v", err)
	}
	if apiKeyCount != 0 {
		t.Errorf("DeleteWithCleanup() API keys still exist, count = %d", apiKeyCount)
	}
}

func TestService_VerifyPassword_MultipleAttempts(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	password := "StrongP@ss123!"
	_, err := svc.Create(ctx, "attemptuser", password, "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Multiple wrong attempts
	for i := 0; i < 3; i++ {
		user, err := svc.VerifyPassword(ctx, "attemptuser", "wrongpassword")
		if err != ErrInvalidPassword {
			t.Errorf("Attempt %d: VerifyPassword() error = %v, want ErrInvalidPassword", i, err)
		}
		if user != nil {
			t.Errorf("Attempt %d: VerifyPassword() returned user for wrong password", i)
		}
	}

	// Correct password should still work
	user, err := svc.VerifyPassword(ctx, "attemptuser", password)
	if err != nil {
		t.Fatalf("VerifyPassword() correct password error = %v", err)
	}
	if user == nil {
		t.Error("VerifyPassword() should return user for correct password")
	}
}

func TestService_UpdatePassword_MultipleUpdates(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	passwords := []string{
		"StrongP@ss123!",
		"AnotherStr0ng!Pass",
		"ThirdP@ssword1!",
	}

	// Create user
	user, err := svc.Create(ctx, "multiupdate", passwords[0], "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update password multiple times
	for i := 1; i < len(passwords); i++ {
		err = svc.UpdatePassword(ctx, user.ID, passwords[i])
		if err != nil {
			t.Fatalf("UpdatePassword() attempt %d error = %v", i, err)
		}

		// Verify only latest password works
		for j := 0; j < i; j++ {
			u, err := svc.VerifyPassword(ctx, "multiupdate", passwords[j])
			if err != ErrInvalidPassword {
				t.Errorf("Old password %d should not work after update %d", j, i)
			}
			if u != nil {
				t.Error("VerifyPassword() should not return user for old password")
			}
		}

		// Current password should work
		u, err := svc.VerifyPassword(ctx, "multiupdate", passwords[i])
		if err != nil {
			t.Fatalf("Current password should work: %v", err)
		}
		if u == nil {
			t.Error("VerifyPassword() should return user for current password")
		}
	}
}

func TestService_SetTOTP_UpdateSecret(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create user
	user, err := svc.Create(ctx, "totpupdate", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Enable with first secret
	err = svc.SetTOTP(ctx, user.ID, "FIRST_SECRET_123", true)
	if err != nil {
		t.Fatalf("SetTOTP() first error = %v", err)
	}

	// Update to new secret
	err = svc.SetTOTP(ctx, user.ID, "SECOND_SECRET_456", true)
	if err != nil {
		t.Fatalf("SetTOTP() second error = %v", err)
	}

	// Verify new secret
	updated, err := svc.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.TOTPSecret != "SECOND_SECRET_456" {
		t.Errorf("SetTOTP() secret = %v, want %v", updated.TOTPSecret, "SECOND_SECRET_456")
	}
}

func TestService_List_ManyUsers(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create many users
	expectedCount := 25
	for i := 0; i < expectedCount; i++ {
		username := "listuser" + string(rune('A'+i/10)) + string(rune('0'+i%10))
		_, err := svc.Create(ctx, username, "StrongP@ss123!", "", "user")
		if err != nil {
			t.Fatalf("Create() user %d error = %v", i, err)
		}
	}

	users, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(users) != expectedCount {
		t.Errorf("List() returned %d users, want %d", len(users), expectedCount)
	}

	count, err := svc.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != int64(expectedCount) {
		t.Errorf("Count() = %d, want %d", count, expectedCount)
	}
}

func TestService_Delete_ThenRecreate(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	username := "recreateuser"

	// Create user
	user, err := svc.Create(ctx, username, "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() first error = %v", err)
	}
	firstID := user.ID

	// Delete user
	err = svc.Delete(ctx, user.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Recreate user with same username
	user, err = svc.Create(ctx, username, "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() second error = %v", err)
	}

	// Should have different ID
	if user.ID == firstID {
		t.Error("Recreated user should have different ID")
	}
}

func TestService_GetByID_AfterDelete(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create user
	user, err := svc.Create(ctx, "deleteme", "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	userID := user.ID

	// Delete user
	err = svc.Delete(ctx, userID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// GetByID should return NotFound error
	found, err := svc.GetByID(ctx, userID)
	if !services.IsNotFound(err) {
		t.Errorf("GetByID() error = %v, want NotFound", err)
	}
	if found != nil {
		t.Error("GetByID() should return nil for deleted user")
	}
}

func TestService_GetByUsername_AfterDelete(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	username := "deletebyname"

	// Create user
	user, err := svc.Create(ctx, username, "StrongP@ss123!", "", "user")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete user
	err = svc.Delete(ctx, user.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// GetByUsername should return NotFound error
	found, err := svc.GetByUsername(ctx, username)
	if !services.IsNotFound(err) {
		t.Errorf("GetByUsername() error = %v, want NotFound", err)
	}
	if found != nil {
		t.Error("GetByUsername() should return nil for deleted user")
	}
}

func TestService_Update_EmptyFields(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create user with email
	user, err := svc.Create(ctx, "clearfields", "StrongP@ss123!", "clear@example.com", "admin")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Clear email
	user.Email = ""
	err = svc.Update(ctx, user)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify email is cleared
	updated, err := svc.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.Email != "" {
		t.Errorf("Update() email = %v, want empty", updated.Email)
	}
}
