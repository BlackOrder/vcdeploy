package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// mockUserService implements services.UserServicer for testing
type mockUserService struct {
	users map[int64]*storage.User
}

func newMockUserService() *mockUserService {
	return &mockUserService{
		users: make(map[int64]*storage.User),
	}
}

func (m *mockUserService) Create(ctx context.Context, username, password, email, role string) (*storage.User, error) {
	return nil, nil
}

func (m *mockUserService) GetByID(ctx context.Context, id int64) (*storage.User, error) {
	if user, ok := m.users[id]; ok {
		return user, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockUserService) GetByUsername(ctx context.Context, username string) (*storage.User, error) {
	return nil, nil
}

func (m *mockUserService) List(ctx context.Context) ([]*storage.User, error) {
	return nil, nil
}

func (m *mockUserService) Count(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockUserService) Update(ctx context.Context, user *storage.User) error {
	return nil
}

func (m *mockUserService) Delete(ctx context.Context, id int64) error {
	return nil
}

func (m *mockUserService) DeleteWithCleanup(ctx context.Context, id int64) error {
	return nil
}

func (m *mockUserService) VerifyPassword(ctx context.Context, username, password string) (*storage.User, error) {
	return nil, nil
}

func (m *mockUserService) UpdatePassword(ctx context.Context, userID int64, newPassword string) error {
	return nil
}

func (m *mockUserService) SetTOTP(ctx context.Context, userID int64, secret string, enabled bool) error {
	return nil
}

func (m *mockUserService) addUser(user *storage.User) {
	m.users[user.ID] = user
}

func TestRequireAPIEnabled(t *testing.T) {
	logger := zap.NewNop()

	t.Run("allows requests when API is enabled", func(t *testing.T) {
		cfg := &config.MasterConfig{
			API: config.APIConfig{Enabled: true},
		}
		mw := NewEnforcementMiddleware(cfg, nil, logger)

		handler := mw.RequireAPIEnabled(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/api/v1/projects", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("rejects requests when API is disabled", func(t *testing.T) {
		cfg := &config.MasterConfig{
			API: config.APIConfig{Enabled: false},
		}
		mw := NewEnforcementMiddleware(cfg, nil, logger)

		handler := mw.RequireAPIEnabled(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/api/v1/projects", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
		}
	})
}

func TestRequireAPIEnabledFunc(t *testing.T) {
	logger := zap.NewNop()

	t.Run("allows requests when API is enabled", func(t *testing.T) {
		cfg := &config.MasterConfig{
			API: config.APIConfig{Enabled: true},
		}
		mw := NewEnforcementMiddleware(cfg, nil, logger)

		handler := mw.RequireAPIEnabledFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/api/v1/projects", nil)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("rejects requests when API is disabled", func(t *testing.T) {
		cfg := &config.MasterConfig{
			API: config.APIConfig{Enabled: false},
		}
		mw := NewEnforcementMiddleware(cfg, nil, logger)

		handler := mw.RequireAPIEnabledFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/api/v1/projects", nil)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
		}
	})
}

func TestRequire2FAForAdmin(t *testing.T) {
	logger := zap.NewNop()

	t.Run("allows requests when 2FA not required", func(t *testing.T) {
		cfg := &config.MasterConfig{
			Security: config.SecurityConfig{Require2FAAdmin: false},
		}
		userSvc := newMockUserService()
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.Require2FAForAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("allows requests for non-admin users", func(t *testing.T) {
		cfg := &config.MasterConfig{
			Security: config.SecurityConfig{Require2FAAdmin: true},
		}
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "user", Role: "user", TOTPEnabled: false})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.Require2FAForAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("allows requests for admin with 2FA enabled", func(t *testing.T) {
		cfg := &config.MasterConfig{
			Security: config.SecurityConfig{Require2FAAdmin: true},
		}
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "admin", Role: "admin", TOTPEnabled: true})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.Require2FAForAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("rejects requests for admin without 2FA", func(t *testing.T) {
		cfg := &config.MasterConfig{
			Security: config.SecurityConfig{Require2FAAdmin: true},
		}
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "admin", Role: "admin", TOTPEnabled: false})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.Require2FAForAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status %d, got %d", http.StatusForbidden, rr.Code)
		}
	})

	t.Run("allows requests when no user in context", func(t *testing.T) {
		cfg := &config.MasterConfig{
			Security: config.SecurityConfig{Require2FAAdmin: true},
		}
		mw := NewEnforcementMiddleware(cfg, newMockUserService(), logger)

		handler := mw.Require2FAForAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})
}

func TestLogSizeEnforcer(t *testing.T) {
	logger := zap.NewNop()

	t.Run("allows content within size limit", func(t *testing.T) {
		enforcer := NewLogSizeEnforcer(1, logger) // 1MB limit

		content := "Hello, World!"
		if !enforcer.CheckSize(int64(len(content))) {
			t.Error("Expected content to be within size limit")
		}
	})

	t.Run("rejects content exceeding size limit", func(t *testing.T) {
		enforcer := NewLogSizeEnforcer(1, logger) // 1MB limit

		// Create content larger than 1MB
		largeContent := make([]byte, 2*1024*1024) // 2MB
		if enforcer.CheckSize(int64(len(largeContent))) {
			t.Error("Expected content to exceed size limit")
		}
	})

	t.Run("returns correct max size", func(t *testing.T) {
		enforcer := NewLogSizeEnforcer(10, logger) // 10MB limit
		expected := int64(10 * 1024 * 1024)

		if enforcer.MaxSize() != expected {
			t.Errorf("Expected max size %d, got %d", expected, enforcer.MaxSize())
		}
	})

	t.Run("uses default max size when zero", func(t *testing.T) {
		enforcer := NewLogSizeEnforcer(0, logger)
		expected := int64(100 * 1024 * 1024) // 100MB default

		if enforcer.MaxSize() != expected {
			t.Errorf("Expected default max size %d, got %d", expected, enforcer.MaxSize())
		}
	})

	t.Run("truncates large content", func(t *testing.T) {
		enforcer := NewLogSizeEnforcer(1, logger) // 1MB limit

		// Create content larger than 1MB
		largeContent := make([]byte, 2*1024*1024)
		for i := range largeContent {
			largeContent[i] = 'a'
		}

		truncated, wasTruncated := enforcer.TruncateLog(string(largeContent))

		if !wasTruncated {
			t.Error("Expected content to be truncated")
		}

		if int64(len(truncated)) > enforcer.MaxSize() {
			t.Errorf("Truncated content %d exceeds max size %d", len(truncated), enforcer.MaxSize())
		}

		if !containsString(truncated, "LOG TRUNCATED") {
			t.Error("Expected truncation marker in content")
		}
	})

	t.Run("does not truncate small content", func(t *testing.T) {
		enforcer := NewLogSizeEnforcer(1, logger) // 1MB limit

		content := "Small content"
		result, wasTruncated := enforcer.TruncateLog(content)

		if wasTruncated {
			t.Error("Expected content not to be truncated")
		}

		if result != content {
			t.Errorf("Expected content to be unchanged, got %q", result)
		}
	})
}

func TestChainMiddleware(t *testing.T) {
	var order []int

	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, 1)
			next.ServeHTTP(w, r)
		})
	}

	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, 2)
			next.ServeHTTP(w, r)
		})
	}

	mw3 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, 3)
			next.ServeHTTP(w, r)
		})
	}

	handler := ChainMiddleware(mw1, mw2, mw3)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, 0)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Middleware should be applied in order: mw1 -> mw2 -> mw3 -> handler
	expected := []int{1, 2, 3, 0}
	if len(order) != len(expected) {
		t.Fatalf("Expected %d middleware calls, got %d", len(expected), len(order))
	}

	for i, v := range expected {
		if order[i] != v {
			t.Errorf("Expected order[%d] = %d, got %d", i, v, order[i])
		}
	}
}

func TestUserContext(t *testing.T) {
	user := &storage.User{
		ID:       1,
		Username: "testuser",
		Role:     "admin",
	}

	ctx := WithUserContext(context.Background(), user)
	retrieved, ok := GetUserFromContext(ctx)

	if !ok {
		t.Fatal("Expected to retrieve user from context")
	}

	if retrieved.ID != user.ID {
		t.Errorf("Expected user ID %d, got %d", user.ID, retrieved.ID)
	}

	if retrieved.Username != user.Username {
		t.Errorf("Expected username %s, got %s", user.Username, retrieved.Username)
	}

	// Test empty context
	_, ok = GetUserFromContext(context.Background())
	if ok {
		t.Error("Expected no user in empty context")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Test Require2FAForAdminFunc (HandlerFunc version)
func TestRequire2FAForAdminFunc(t *testing.T) {
	logger := zap.NewNop()

	t.Run("skips check when Require2FAAdmin is false", func(t *testing.T) {
		cfg := &config.MasterConfig{
			Security: config.SecurityConfig{Require2FAAdmin: false},
		}
		userSvc := newMockUserService()
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handlerFunc := mw.Require2FAForAdminFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		handlerFunc(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("allows requests without user context", func(t *testing.T) {
		cfg := &config.MasterConfig{
			Security: config.SecurityConfig{Require2FAAdmin: true},
		}
		userSvc := newMockUserService()
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handlerFunc := mw.Require2FAForAdminFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		handlerFunc(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("allows non-admin users", func(t *testing.T) {
		cfg := &config.MasterConfig{
			Security: config.SecurityConfig{Require2FAAdmin: true},
		}
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "user", Role: "viewer", TOTPEnabled: false})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handlerFunc := mw.Require2FAForAdminFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handlerFunc(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("allows admin with 2FA enabled", func(t *testing.T) {
		cfg := &config.MasterConfig{
			Security: config.SecurityConfig{Require2FAAdmin: true},
		}
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "admin", Role: "admin", TOTPEnabled: true})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handlerFunc := mw.Require2FAForAdminFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handlerFunc(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("rejects admin without 2FA", func(t *testing.T) {
		cfg := &config.MasterConfig{
			Security: config.SecurityConfig{Require2FAAdmin: true},
		}
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "admin", Role: "admin", TOTPEnabled: false})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handlerFunc := mw.Require2FAForAdminFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/admin", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handlerFunc(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status %d, got %d", http.StatusForbidden, rr.Code)
		}
	})

	t.Run("handles user lookup error", func(t *testing.T) {
		cfg := &config.MasterConfig{
			Security: config.SecurityConfig{Require2FAAdmin: true},
		}
		userSvc := newMockUserService() // User not added, so lookup will fail
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handlerFunc := mw.Require2FAForAdminFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(999)) // Non-existent user
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handlerFunc(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, rr.Code)
		}
	})
}

// --- parseScopes and hasScope tests ---

func TestParseScopes(t *testing.T) {
	tests := []struct {
		name       string
		key        *storage.APIKey
		wantScopes []string
		wantErr    bool
	}{
		{
			name:       "nil key",
			key:        nil,
			wantScopes: nil,
			wantErr:    false,
		},
		{
			name:       "empty scopes",
			key:        &storage.APIKey{Scopes: ""},
			wantScopes: nil,
			wantErr:    false,
		},
		{
			name:       "valid single scope",
			key:        &storage.APIKey{Scopes: `["read"]`},
			wantScopes: []string{"read"},
			wantErr:    false,
		},
		{
			name:       "valid multiple scopes",
			key:        &storage.APIKey{Scopes: `["read","write","admin"]`},
			wantScopes: []string{"read", "write", "admin"},
			wantErr:    false,
		},
		{
			name:       "invalid JSON",
			key:        &storage.APIKey{Scopes: `not valid json`},
			wantScopes: nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scopes, err := parseScopes(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseScopes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(scopes) != len(tt.wantScopes) {
				t.Errorf("parseScopes() = %v, want %v", scopes, tt.wantScopes)
			}
			for i, s := range scopes {
				if s != tt.wantScopes[i] {
					t.Errorf("parseScopes()[%d] = %s, want %s", i, s, tt.wantScopes[i])
				}
			}
		})
	}
}

func TestHasScope(t *testing.T) {
	tests := []struct {
		name     string
		key      *storage.APIKey
		required APIScope
		want     bool
	}{
		{
			name:     "nil key means full access",
			key:      nil,
			required: ScopeRead,
			want:     true,
		},
		{
			name:     "empty scopes means full access",
			key:      &storage.APIKey{Scopes: "[]"},
			required: ScopeAdmin,
			want:     true,
		},
		{
			name:     "no scopes string means full access",
			key:      &storage.APIKey{Scopes: ""},
			required: ScopeWrite,
			want:     true,
		},
		{
			name:     "admin scope implies all others",
			key:      &storage.APIKey{Scopes: `["admin"]`},
			required: ScopeRead,
			want:     true,
		},
		{
			name:     "admin scope implies write",
			key:      &storage.APIKey{Scopes: `["admin"]`},
			required: ScopeWrite,
			want:     true,
		},
		{
			name:     "write scope implies read",
			key:      &storage.APIKey{Scopes: `["write"]`},
			required: ScopeRead,
			want:     true,
		},
		{
			name:     "write scope does not imply admin",
			key:      &storage.APIKey{Scopes: `["write"]`},
			required: ScopeAdmin,
			want:     false,
		},
		{
			name:     "read scope does not imply write",
			key:      &storage.APIKey{Scopes: `["read"]`},
			required: ScopeWrite,
			want:     false,
		},
		{
			name:     "exact scope match",
			key:      &storage.APIKey{Scopes: `["read"]`},
			required: ScopeRead,
			want:     true,
		},
		{
			name:     "invalid JSON returns false",
			key:      &storage.APIKey{Scopes: `invalid`},
			required: ScopeRead,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasScope(tt.key, tt.required)
			if got != tt.want {
				t.Errorf("hasScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- RequireRole tests ---

func TestRequireRole(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.MasterConfig{}

	t.Run("allows exact role match", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "admin", Role: "admin"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.RequireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/admin", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("rejects role mismatch", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "user", Role: "user"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.RequireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/admin", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status %d, got %d", http.StatusForbidden, rr.Code)
		}
	})

	t.Run("rejects no user in context", func(t *testing.T) {
		userSvc := newMockUserService()
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.RequireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/admin", nil)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("handles user lookup error", func(t *testing.T) {
		userSvc := newMockUserService() // No user added
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.RequireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/admin", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(999))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, rr.Code)
		}
	})
}

// --- RequireMinRole tests ---

func TestRequireMinRole(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.MasterConfig{}

	t.Run("admin can access admin-required", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "admin", Role: "admin"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.RequireMinRole("admin")(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/admin", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("admin can access user-required", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "admin", Role: "admin"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.RequireMinRole("user")(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/resource", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("user cannot access admin-required", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "user", Role: "user"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.RequireMinRole("admin")(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/admin", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status %d, got %d", http.StatusForbidden, rr.Code)
		}
	})

	t.Run("viewer can access viewer-required", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "viewer", Role: "viewer"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.RequireMinRole("viewer")(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/view", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("viewer cannot access user-required", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "viewer", Role: "viewer"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.RequireMinRole("user")(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/resource", nil)
		ctx := context.WithValue(req.Context(), contextKeyUserID, int64(1))
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status %d, got %d", http.StatusForbidden, rr.Code)
		}
	})

	t.Run("rejects no user in context", func(t *testing.T) {
		userSvc := newMockUserService()
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		handler := mw.RequireMinRole("user")(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/resource", nil)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})
}

// --- RequireScope tests ---

func TestRequireScope(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.MasterConfig{}
	userSvc := newMockUserService()
	mw := NewEnforcementMiddleware(cfg, userSvc, logger)

	t.Run("allows matching scope", func(t *testing.T) {
		handler := mw.RequireScope(ScopeRead)(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/api/v1/projects", nil)
		apiKey := &storage.APIKey{Scopes: `["read"]`}
		ctx := WithAPIKeyContext(req.Context(), apiKey)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("allows admin scope for any requirement", func(t *testing.T) {
		handler := mw.RequireScope(ScopeWrite)(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("POST", "/api/v1/projects", nil)
		apiKey := &storage.APIKey{Scopes: `["admin"]`}
		ctx := WithAPIKeyContext(req.Context(), apiKey)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("rejects insufficient scope", func(t *testing.T) {
		handler := mw.RequireScope(ScopeAdmin)(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("DELETE", "/api/v1/users/1", nil)
		apiKey := &storage.APIKey{KeyPrefix: "test_key", Scopes: `["read"]`}
		ctx := WithAPIKeyContext(req.Context(), apiKey)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status %d, got %d", http.StatusForbidden, rr.Code)
		}
	})

	t.Run("allows request without API key (session auth)", func(t *testing.T) {
		handler := mw.RequireScope(ScopeRead)(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/api/v1/projects", nil)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})
}

// --- RequireReadScope, RequireWriteScope, RequireAdminScope tests ---

func TestRequireScopeHelpers(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.MasterConfig{}
	userSvc := newMockUserService()
	mw := NewEnforcementMiddleware(cfg, userSvc, logger)

	t.Run("RequireReadScope allows read", func(t *testing.T) {
		handler := mw.RequireReadScope(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/api/v1/projects", nil)
		apiKey := &storage.APIKey{Scopes: `["read"]`}
		ctx := WithAPIKeyContext(req.Context(), apiKey)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("RequireWriteScope allows write", func(t *testing.T) {
		handler := mw.RequireWriteScope(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("POST", "/api/v1/projects", nil)
		apiKey := &storage.APIKey{Scopes: `["write"]`}
		ctx := WithAPIKeyContext(req.Context(), apiKey)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("RequireAdminScope allows admin", func(t *testing.T) {
		handler := mw.RequireAdminScope(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("DELETE", "/api/v1/users/1", nil)
		apiKey := &storage.APIKey{Scopes: `["admin"]`}
		ctx := WithAPIKeyContext(req.Context(), apiKey)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("RequireAdminScope rejects write-only", func(t *testing.T) {
		handler := mw.RequireAdminScope(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("DELETE", "/api/v1/users/1", nil)
		apiKey := &storage.APIKey{KeyPrefix: "test", Scopes: `["write"]`}
		ctx := WithAPIKeyContext(req.Context(), apiKey)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status %d, got %d", http.StatusForbidden, rr.Code)
		}
	})
}

// --- CheckScope tests ---

func TestCheckScope(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.MasterConfig{}
	userSvc := newMockUserService()
	mw := NewEnforcementMiddleware(cfg, userSvc, logger)

	t.Run("allows session auth without API key", func(t *testing.T) {
		ctx := context.Background()
		msg, status, ok := mw.CheckScope(ctx, ScopeRead)
		if !ok {
			t.Errorf("CheckScope() should allow when no API key present")
		}
		if msg != "" || status != 0 {
			t.Errorf("CheckScope() returned msg=%q, status=%d; want empty, 0", msg, status)
		}
	})

	t.Run("allows matching scope", func(t *testing.T) {
		apiKey := &storage.APIKey{Scopes: `["read"]`}
		ctx := WithAPIKeyContext(context.Background(), apiKey)
		msg, status, ok := mw.CheckScope(ctx, ScopeRead)
		if !ok {
			t.Errorf("CheckScope() should allow matching scope")
		}
		if msg != "" || status != 0 {
			t.Errorf("CheckScope() returned msg=%q, status=%d; want empty, 0", msg, status)
		}
	})

	t.Run("rejects insufficient scope", func(t *testing.T) {
		apiKey := &storage.APIKey{KeyPrefix: "test_key", Scopes: `["read"]`}
		ctx := WithAPIKeyContext(context.Background(), apiKey)
		msg, status, ok := mw.CheckScope(ctx, ScopeAdmin)
		if ok {
			t.Errorf("CheckScope() should reject insufficient scope")
		}
		if status != http.StatusForbidden {
			t.Errorf("CheckScope() status = %d; want %d", status, http.StatusForbidden)
		}
		if msg == "" {
			t.Errorf("CheckScope() should return error message")
		}
	})

	t.Run("admin scope implies all", func(t *testing.T) {
		apiKey := &storage.APIKey{Scopes: `["admin"]`}
		ctx := WithAPIKeyContext(context.Background(), apiKey)
		msg, status, ok := mw.CheckScope(ctx, ScopeWrite)
		if !ok {
			t.Errorf("CheckScope() admin should imply write")
		}
		if msg != "" || status != 0 {
			t.Errorf("CheckScope() returned msg=%q, status=%d; want empty, 0", msg, status)
		}
	})
}

// --- CheckWriteAccess tests ---

func TestCheckWriteAccess(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.MasterConfig{}

	t.Run("allows with write scope and user role", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "editor", Role: "user"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		apiKey := &storage.APIKey{Scopes: `["write"]`}
		ctx := WithAPIKeyContext(context.Background(), apiKey)
		ctx = context.WithValue(ctx, contextKeyUserID, int64(1))

		msg, status, ok := mw.CheckWriteAccess(ctx)
		if !ok {
			t.Errorf("CheckWriteAccess() should allow with write scope and user role")
		}
		if msg != "" || status != 0 {
			t.Errorf("CheckWriteAccess() returned msg=%q, status=%d; want empty, 0", msg, status)
		}
	})

	t.Run("rejects read-only API key", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "editor", Role: "user"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		apiKey := &storage.APIKey{KeyPrefix: "test", Scopes: `["read"]`}
		ctx := WithAPIKeyContext(context.Background(), apiKey)
		ctx = context.WithValue(ctx, contextKeyUserID, int64(1))

		msg, status, ok := mw.CheckWriteAccess(ctx)
		if ok {
			t.Errorf("CheckWriteAccess() should reject read-only API key")
		}
		if status != http.StatusForbidden {
			t.Errorf("CheckWriteAccess() status = %d; want %d", status, http.StatusForbidden)
		}
		if msg == "" {
			t.Errorf("CheckWriteAccess() should return error message")
		}
	})

	t.Run("rejects viewer role", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "viewer", Role: "viewer"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		apiKey := &storage.APIKey{Scopes: `["write"]`}
		ctx := WithAPIKeyContext(context.Background(), apiKey)
		ctx = context.WithValue(ctx, contextKeyUserID, int64(1))

		_, status, ok := mw.CheckWriteAccess(ctx)
		if ok {
			t.Errorf("CheckWriteAccess() should reject viewer role")
		}
		if status != http.StatusForbidden {
			t.Errorf("CheckWriteAccess() status = %d; want %d", status, http.StatusForbidden)
		}
	})
}

// --- CheckAdminAccess tests ---

func TestCheckAdminAccess(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.MasterConfig{}

	t.Run("allows with admin scope and admin role", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "admin", Role: "admin"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		apiKey := &storage.APIKey{Scopes: `["admin"]`}
		ctx := WithAPIKeyContext(context.Background(), apiKey)
		ctx = context.WithValue(ctx, contextKeyUserID, int64(1))

		msg, status, ok := mw.CheckAdminAccess(ctx)
		if !ok {
			t.Errorf("CheckAdminAccess() should allow with admin scope and admin role")
		}
		if msg != "" || status != 0 {
			t.Errorf("CheckAdminAccess() returned msg=%q, status=%d; want empty, 0", msg, status)
		}
	})

	t.Run("rejects write-only API key", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "admin", Role: "admin"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		apiKey := &storage.APIKey{KeyPrefix: "test", Scopes: `["write"]`}
		ctx := WithAPIKeyContext(context.Background(), apiKey)
		ctx = context.WithValue(ctx, contextKeyUserID, int64(1))

		_, status, ok := mw.CheckAdminAccess(ctx)
		if ok {
			t.Errorf("CheckAdminAccess() should reject write-only API key")
		}
		if status != http.StatusForbidden {
			t.Errorf("CheckAdminAccess() status = %d; want %d", status, http.StatusForbidden)
		}
	})

	t.Run("rejects non-admin user role", func(t *testing.T) {
		userSvc := newMockUserService()
		userSvc.addUser(&storage.User{ID: 1, Username: "user", Role: "user"})
		mw := NewEnforcementMiddleware(cfg, userSvc, logger)

		apiKey := &storage.APIKey{Scopes: `["admin"]`}
		ctx := WithAPIKeyContext(context.Background(), apiKey)
		ctx = context.WithValue(ctx, contextKeyUserID, int64(1))

		_, status, ok := mw.CheckAdminAccess(ctx)
		if ok {
			t.Errorf("CheckAdminAccess() should reject non-admin user role")
		}
		if status != http.StatusForbidden {
			t.Errorf("CheckAdminAccess() status = %d; want %d", status, http.StatusForbidden)
		}
	})
}
