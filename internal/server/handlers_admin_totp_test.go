package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// TestHandleAdminTOTPUsers tests the admin TOTP users list endpoint.
func TestHandleAdminTOTPUsers(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	// Create a user with TOTP enabled
	totpUser := &storage.User{
		Username:     "totpuser",
		PasswordHash: "test-hash",
		Email:        "totp@example.com",
		Role:         "user",
		TOTPEnabled:  true,
		TOTPSecret:   "JBSWY3DPEHPK3PXP",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, totpUser); err != nil {
		t.Fatalf("failed to create TOTP user: %v", err)
	}

	// Get the admin user for context
	adminUser, err := server.store.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get admin user: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/admin/totp/users", nil)
	ctx = WithUserContext(req.Context(), adminUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPUsers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var response struct {
		Users []struct {
			ID          int64  `json:"id"`
			Username    string `json:"username"`
			TOTPEnabled bool   `json:"totpEnabled"`
		} `json:"users"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should have one TOTP-enabled user
	found := false
	for _, u := range response.Users {
		if u.Username == "totpuser" {
			found = true
			if !u.TOTPEnabled {
				t.Error("expected totpEnabled to be true")
			}
		}
	}
	if !found {
		t.Error("expected to find totpuser in response")
	}
}

func TestHandleAdminTOTPUsers_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()
	adminUser, _ := server.store.GetUserByID(ctx, userID)

	req := httptest.NewRequest("POST", "/api/v1/admin/totp/users", nil)
	ctx = WithUserContext(req.Context(), adminUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPUsers(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleAdminTOTPUsers_RequiresAdmin(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	// Create a non-admin user
	regularUser := &storage.User{
		Username:     "regularuser",
		PasswordHash: "test-hash",
		Email:        "regular@example.com",
		Role:         "user",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, regularUser); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/admin/totp/users", nil)
	ctx = WithUserContext(req.Context(), regularUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPUsers(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

// TestHandleAdminTOTPStatus tests the admin TOTP status endpoint.
func TestHandleAdminTOTPStatus(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	// Create a user with TOTP enabled
	totpUser := &storage.User{
		Username:     "totpuser",
		PasswordHash: "test-hash",
		Email:        "totp@example.com",
		Role:         "user",
		TOTPEnabled:  true,
		TOTPSecret:   "JBSWY3DPEHPK3PXP",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, totpUser); err != nil {
		t.Fatalf("failed to create TOTP user: %v", err)
	}

	// Add recovery codes using GenerateRecoveryCodes
	_, hashes, err := security.GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("failed to generate recovery codes: %v", err)
	}
	var recoveryCodes []*storage.RecoveryCode
	for _, hash := range hashes[:3] { // Use 3 codes for testing
		recoveryCodes = append(recoveryCodes, &storage.RecoveryCode{
			UserID:    totpUser.ID,
			CodeHash:  hash,
			CreatedAt: time.Now(),
		})
	}
	if err := server.store.SaveRecoveryCodes(ctx, totpUser.ID, recoveryCodes); err != nil {
		t.Fatalf("failed to save recovery codes: %v", err)
	}

	// Get the admin user for context
	adminUser, err := server.store.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get admin user: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/admin/totp/status/totpuser", nil)
	ctx = WithUserContext(req.Context(), adminUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var response struct {
		UserID                 int64  `json:"user_id"`
		Username               string `json:"username"`
		TOTPEnabled            bool   `json:"totp_enabled"`
		RecoveryCodesRemaining int    `json:"recovery_codes_remaining"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Username != "totpuser" {
		t.Errorf("expected username 'totpuser', got '%s'", response.Username)
	}
	if !response.TOTPEnabled {
		t.Error("expected totp_enabled to be true")
	}
	if response.RecoveryCodesRemaining != 3 {
		t.Errorf("expected 3 recovery codes, got %d", response.RecoveryCodesRemaining)
	}
}

func TestHandleAdminTOTPStatus_UserNotFound(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()
	adminUser, _ := server.store.GetUserByID(ctx, userID)

	req := httptest.NewRequest("GET", "/api/v1/admin/totp/status/nonexistent", nil)
	ctx = WithUserContext(req.Context(), adminUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPStatus(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleAdminTOTPStatus_RequiresAdmin(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	regularUser := &storage.User{
		Username:     "regularuser",
		PasswordHash: "test-hash",
		Email:        "regular@example.com",
		Role:         "user",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, regularUser); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/admin/totp/status/someone", nil)
	ctx = WithUserContext(req.Context(), regularUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPStatus(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

// TestHandleAdminTOTPDisable tests the admin TOTP disable endpoint.
func TestHandleAdminTOTPDisable(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	// Create a user with TOTP enabled
	totpUser := &storage.User{
		Username:     "totpuser",
		PasswordHash: "test-hash",
		Email:        "totp@example.com",
		Role:         "user",
		TOTPEnabled:  true,
		TOTPSecret:   "JBSWY3DPEHPK3PXP",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, totpUser); err != nil {
		t.Fatalf("failed to create TOTP user: %v", err)
	}

	// Get the admin user for context (admin doesn't have TOTP)
	adminUser, err := server.store.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get admin user: %v", err)
	}

	body := bytes.NewBufferString(`{
		"username": "totpuser",
		"reason": "User locked out and contacted support"
	}`)

	req := httptest.NewRequest("POST", "/api/v1/admin/totp/disable", body)
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), adminUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPDisable(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	// Verify TOTP was disabled
	updatedUser, err := server.store.GetUserByUsername(ctx, "totpuser")
	if err != nil {
		t.Fatalf("failed to get updated user: %v", err)
	}
	if updatedUser.TOTPEnabled {
		t.Error("expected TOTP to be disabled")
	}
	if updatedUser.TOTPSecret != "" {
		t.Error("expected TOTP secret to be cleared")
	}
}

func TestHandleAdminTOTPDisable_RequiresAdminTOTP(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	// Enable TOTP for admin
	secret := "JBSWY3DPEHPK3PXP"
	if err := server.userService.SetTOTP(ctx, userID, secret, true); err != nil {
		t.Fatalf("failed to enable admin TOTP: %v", err)
	}

	// Create a user with TOTP enabled
	totpUser := &storage.User{
		Username:     "totpuser",
		PasswordHash: "test-hash",
		Email:        "totp@example.com",
		Role:         "user",
		TOTPEnabled:  true,
		TOTPSecret:   "ANOTHERSECRETKEY",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, totpUser); err != nil {
		t.Fatalf("failed to create TOTP user: %v", err)
	}

	// Get the admin user for context (now with TOTP)
	adminUser, err := server.store.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get admin user: %v", err)
	}

	// Attempt without providing admin TOTP code
	body := bytes.NewBufferString(`{
		"username": "totpuser",
		"reason": "User locked out and contacted support"
	}`)

	req := httptest.NewRequest("POST", "/api/v1/admin/totp/disable", body)
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), adminUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPDisable(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d: %s", http.StatusUnauthorized, w.Code, w.Body.String())
	}
}

func TestHandleAdminTOTPDisable_CannotSelfDisable(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	// Enable TOTP for admin
	secret := "JBSWY3DPEHPK3PXP"
	if err := server.userService.SetTOTP(ctx, userID, secret, true); err != nil {
		t.Fatalf("failed to enable admin TOTP: %v", err)
	}

	adminUser, err := server.store.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get admin user: %v", err)
	}

	// Attempt to disable own TOTP
	body := bytes.NewBufferString(`{
		"username": "testuser",
		"reason": "Testing self-disable prevention"
	}`)

	req := httptest.NewRequest("POST", "/api/v1/admin/totp/disable", body)
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), adminUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPDisable(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestHandleAdminTOTPDisable_ShortReason(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()
	adminUser, _ := server.store.GetUserByID(ctx, userID)

	body := bytes.NewBufferString(`{
		"username": "someone",
		"reason": "too short"
	}`)

	req := httptest.NewRequest("POST", "/api/v1/admin/totp/disable", body)
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), adminUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPDisable(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandleAdminTOTPDisable_UserNotFound(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()
	adminUser, _ := server.store.GetUserByID(ctx, userID)

	body := bytes.NewBufferString(`{
		"username": "nonexistent",
		"reason": "Testing nonexistent user"
	}`)

	req := httptest.NewRequest("POST", "/api/v1/admin/totp/disable", body)
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), adminUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPDisable(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleAdminTOTPDisable_UserHasNoTOTP(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	// Create a user without TOTP
	noTotpUser := &storage.User{
		Username:     "nototpuser",
		PasswordHash: "test-hash",
		Email:        "nototp@example.com",
		Role:         "user",
		TOTPEnabled:  false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, noTotpUser); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	adminUser, _ := server.store.GetUserByID(ctx, userID)

	body := bytes.NewBufferString(`{
		"username": "nototpuser",
		"reason": "User has no TOTP to disable"
	}`)

	req := httptest.NewRequest("POST", "/api/v1/admin/totp/disable", body)
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), adminUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPDisable(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandleAdminTOTPDisable_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()
	adminUser, _ := server.store.GetUserByID(ctx, userID)

	req := httptest.NewRequest("GET", "/api/v1/admin/totp/disable", nil)
	ctx = WithUserContext(req.Context(), adminUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPDisable(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleAdminTOTPDisable_RequiresAdmin(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	regularUser := &storage.User{
		Username:     "regularuser",
		PasswordHash: "test-hash",
		Email:        "regular@example.com",
		Role:         "user",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, regularUser); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	body := bytes.NewBufferString(`{
		"username": "someone",
		"reason": "Attempting unauthorized disable"
	}`)

	req := httptest.NewRequest("POST", "/api/v1/admin/totp/disable", body)
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), regularUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleAdminTOTPDisable(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}
