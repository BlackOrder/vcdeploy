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
