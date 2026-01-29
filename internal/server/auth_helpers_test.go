package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"go.uber.org/zap"
)

func TestAuthHelpers(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	t.Run("requireReadAccess denies without context", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		ctx := context.Background()

		// Should deny because no user in context
		result := server.requireReadAccess(ctx, rec)

		if result {
			t.Error("expected requireReadAccess to return false without user context")
		}
		if rec.Code == http.StatusOK {
			t.Error("expected non-OK status code")
		}
	})

	t.Run("requireWriteAccess denies without context", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		ctx := context.Background()

		result := server.requireWriteAccess(ctx, rec)

		if result {
			t.Error("expected requireWriteAccess to return false without user context")
		}
		if rec.Code == http.StatusOK {
			t.Error("expected non-OK status code")
		}
	})

	t.Run("requireAdminAccess denies without context", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		ctx := context.Background()

		result := server.requireAdminAccess(ctx, rec)

		if result {
			t.Error("expected requireAdminAccess to return false without user context")
		}
		if rec.Code == http.StatusOK {
			t.Error("expected non-OK status code")
		}
	})

	t.Run("requireReadAccessJSON returns JSON error", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		ctx := context.Background()

		result := server.requireReadAccessJSON(ctx, rec)

		if result {
			t.Error("expected requireReadAccessJSON to return false without user context")
		}
		if rec.Header().Get("Content-Type") != "application/json" {
			t.Error("expected JSON content type")
		}
	})

	t.Run("requireAdminAccessJSON returns JSON error", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		ctx := context.Background()

		result := server.requireAdminAccessJSON(ctx, rec)

		if result {
			t.Error("expected requireAdminAccessJSON to return false without user context")
		}
		if rec.Header().Get("Content-Type") != "application/json" {
			t.Error("expected JSON content type")
		}
	})

	t.Run("requireReadAccess allows with valid admin context", func(t *testing.T) {
		t.Parallel()

		testServer := newTestServer(t)
		rec := httptest.NewRecorder()
		userID := createTestAdminUser(t, testServer)
		ctx := context.WithValue(context.Background(), contextKeyUserID, userID)

		result := testServer.requireReadAccess(ctx, rec)

		if !result {
			t.Error("expected requireReadAccess to return true with admin context")
		}
	})

	t.Run("requireAdminAccess allows with valid admin context", func(t *testing.T) {
		t.Parallel()

		testServer := newTestServer(t)
		rec := httptest.NewRecorder()
		userID := createTestAdminUser(t, testServer)
		ctx := context.WithValue(context.Background(), contextKeyUserID, userID)

		result := testServer.requireAdminAccess(ctx, rec)

		if !result {
			t.Error("expected requireAdminAccess to return true with admin context")
		}
	})
}

func TestAuthHelpersEnforcement(t *testing.T) {
	t.Parallel()

	// Create server with enforcement enabled
	logger := zap.NewNop()
	cfg := &config.MasterConfig{
		Server: config.ServerConfig{
			Listen: ":0",
		},
	}

	server := newTestServer(t)

	// Create users with different roles
	ctx := context.Background()
	viewerUser, _ := server.userService.Create(ctx, "vieweruser", "TestPass123!", "viewer@test.com", "viewer")
	regularUser, _ := server.userService.Create(ctx, "regularuser", "TestPass123!", "regular@test.com", "user")
	adminUser, _ := server.userService.Create(ctx, "adminuser2", "TestPass123!", "admin2@test.com", "admin")

	_ = logger
	_ = cfg

	t.Run("viewer can read but not write", func(t *testing.T) {
		viewerCtx := context.WithValue(context.Background(), contextKeyUserID, viewerUser.ID)

		// Read should work
		rec := httptest.NewRecorder()
		if !server.requireReadAccess(viewerCtx, rec) {
			t.Error("viewer should have read access")
		}

		// Write should fail
		rec = httptest.NewRecorder()
		if server.requireWriteAccess(viewerCtx, rec) {
			t.Error("viewer should not have write access")
		}

		// Admin should fail
		rec = httptest.NewRecorder()
		if server.requireAdminAccess(viewerCtx, rec) {
			t.Error("viewer should not have admin access")
		}
	})

	t.Run("regular user can read and write but not admin", func(t *testing.T) {
		userCtx := context.WithValue(context.Background(), contextKeyUserID, regularUser.ID)

		// Read should work
		rec := httptest.NewRecorder()
		if !server.requireReadAccess(userCtx, rec) {
			t.Error("user should have read access")
		}

		// Write should work
		rec = httptest.NewRecorder()
		if !server.requireWriteAccess(userCtx, rec) {
			t.Error("user should have write access")
		}

		// Admin should fail
		rec = httptest.NewRecorder()
		if server.requireAdminAccess(userCtx, rec) {
			t.Error("user should not have admin access")
		}
	})

	t.Run("admin has all access", func(t *testing.T) {
		adminCtx := context.WithValue(context.Background(), contextKeyUserID, adminUser.ID)

		// Read should work
		rec := httptest.NewRecorder()
		if !server.requireReadAccess(adminCtx, rec) {
			t.Error("admin should have read access")
		}

		// Write should work
		rec = httptest.NewRecorder()
		if !server.requireWriteAccess(adminCtx, rec) {
			t.Error("admin should have write access")
		}

		// Admin should work
		rec = httptest.NewRecorder()
		if !server.requireAdminAccess(adminCtx, rec) {
			t.Error("admin should have admin access")
		}
	})
}
