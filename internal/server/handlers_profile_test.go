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

func TestHandleTOTPSetup(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	// Get the test user (created without TOTP)
	user, err := server.store.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}

	// Create authenticated request
	req := httptest.NewRequest(http.MethodPost, "/api/v1/totp/setup", nil)
	ctx = WithUserContext(req.Context(), user)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleTOTPSetup(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Secret string `json:"secret"`
		URI    string `json:"uri"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.Secret == "" {
		t.Error("Expected secret to be non-empty")
	}
	if resp.URI == "" {
		t.Error("Expected URI to be non-empty")
	}
	if len(resp.Secret) < 16 {
		t.Error("Secret should be at least 16 characters")
	}
}

func TestHandleTOTPSetup_AlreadyEnabled(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	// Create user with TOTP enabled
	user := &storage.User{
		Username:     "totpuser2",
		PasswordHash: "test-hash",
		Email:        "totp2@example.com",
		Role:         "user",
		TOTPEnabled:  true,
		TOTPSecret:   "TESTSECRET123456",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/totp/setup", nil)
	ctx = WithUserContext(req.Context(), user)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleTOTPSetup(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestHandleTOTPSetup_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()
	user, _ := server.store.GetUserByID(ctx, userID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/totp/setup", nil)
	ctx = WithUserContext(req.Context(), user)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleTOTPSetup(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestHandleTOTPEnable(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	// Create a user without TOTP
	user := &storage.User{
		Username:     "totpenableuser",
		PasswordHash: "test-hash",
		Email:        "totpenable@example.com",
		Role:         "user",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Generate a secret
	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("Failed to generate secret: %v", err)
	}

	// Generate valid TOTP code
	totpCode := security.GenerateTOTPCode(secret, time.Now().Unix(), security.DefaultTOTPConfig())

	body, _ := json.Marshal(map[string]string{
		"secret":    secret,
		"totp_code": totpCode,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/totp/enable", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), user)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleTOTPEnable(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Message       string   `json:"message"`
		RecoveryCodes []string `json:"recovery_codes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(resp.RecoveryCodes) != 8 {
		t.Errorf("Expected 8 recovery codes, got %d", len(resp.RecoveryCodes))
	}

	// Verify recovery code format (XXXX-XXXX)
	for _, code := range resp.RecoveryCodes {
		if len(code) != 9 || code[4] != '-' {
			t.Errorf("Recovery code should be XXXX-XXXX format, got: %s", code)
		}
	}
}

func TestHandleTOTPEnable_InvalidCode(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	user := &storage.User{
		Username:     "totpinvaliduser",
		PasswordHash: "test-hash",
		Email:        "totpinvalid@example.com",
		Role:         "user",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	secret, _ := security.GenerateTOTPSecret()

	body, _ := json.Marshal(map[string]string{
		"secret":    secret,
		"totp_code": "000000", // Invalid code
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/totp/enable", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), user)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleTOTPEnable(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestHandleTOTPEnable_MissingFields(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	user := &storage.User{
		Username:     "totpmissinguser",
		PasswordHash: "test-hash",
		Email:        "totpmissing@example.com",
		Role:         "user",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	tests := []struct {
		name string
		body map[string]string
	}{
		{"missing secret", map[string]string{"totp_code": "123456"}},
		{"missing code", map[string]string{"secret": "TESTSECRET"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/totp/enable", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx = WithUserContext(req.Context(), user)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			server.handleTOTPEnable(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}
		})
	}
}

func TestHandleTOTPDisable(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	// Create user with TOTP enabled
	secret, _ := security.GenerateTOTPSecret()
	user := &storage.User{
		Username:     "totpdisableuser",
		PasswordHash: "test-hash",
		Email:        "totpdisable@example.com",
		Role:         "user",
		TOTPEnabled:  true,
		TOTPSecret:   secret,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Generate valid TOTP code
	totpCode := security.GenerateTOTPCode(secret, time.Now().Unix(), security.DefaultTOTPConfig())

	body, _ := json.Marshal(map[string]string{
		"totp_code": totpCode,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/totp/disable", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), user)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleTOTPDisable(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTOTPDisable_NotEnabled(t *testing.T) {
	t.Parallel()

	server, _, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()
	user, _ := server.store.GetUserByID(ctx, userID)

	body, _ := json.Marshal(map[string]string{
		"totp_code": "123456",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/totp/disable", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), user)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleTOTPDisable(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestHandleTOTPDisable_InvalidCode(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	secret, _ := security.GenerateTOTPSecret()
	user := &storage.User{
		Username:     "totpdisableinvaliduser",
		PasswordHash: "test-hash",
		Email:        "totpdisableinvalid@example.com",
		Role:         "user",
		TOTPEnabled:  true,
		TOTPSecret:   secret,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"totp_code": "000000",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/totp/disable", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), user)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleTOTPDisable(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestHandleTOTPDisable_MissingCode(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	ctx := context.Background()

	user := &storage.User{
		Username:     "totpdisablemissinguser",
		PasswordHash: "test-hash",
		Email:        "totpdisablemissing@example.com",
		Role:         "user",
		TOTPEnabled:  true,
		TOTPSecret:   "TESTSECRET",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	body, _ := json.Marshal(map[string]string{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/totp/disable", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx = WithUserContext(req.Context(), user)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleTOTPDisable(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}
