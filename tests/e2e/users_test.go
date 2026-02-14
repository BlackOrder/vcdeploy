//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestUsersAPI tests the users API CRUD operations.
func TestUsersAPI(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	t.Run("list users", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/users")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		ctx.Assertions.StatusOK(resp)

		result, err := testutil.DecodePaginatedJSON(resp)
		if err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		// Should have at least the admin user
		ctx.Assertions.True(len(result.Items) >= 1, "expected at least 1 user")
	})

	var createdUserID interface{}
	testUsername := "e2e-test-user"

	t.Run("create user", func(t *testing.T) {
		user := map[string]interface{}{
			"username": testUsername,
			"email":    "e2e-test@example.com",
			"password": "TestUser123!",
			"role":     "viewer",
		}
		resp, err := ctx.Client.Post("/api/v1/users", user)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// If user already exists, find it from the list
		if resp.StatusCode == http.StatusConflict {
			// Get users list and find our user
			listResp, err := ctx.Client.Get("/api/v1/users")
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			result, err := testutil.DecodePaginatedJSON(listResp)
			if err != nil {
				t.Fatalf("failed to decode users list: %v", err)
			}
			for _, u := range result.Items {
				if u["username"] == testUsername {
					createdUserID = u["id"]
					ctx.TrackResource("user", createdUserID)
					return
				}
			}
			t.Fatal("user exists but could not find in list")
			return
		}

		ctx.Assertions.StatusCreated(resp)

		var result map[string]interface{}
		if err := testutil.DecodeJSON(resp, &result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		createdUserID = result["id"]
		ctx.TrackResource("user", createdUserID)
	})

	t.Run("get user", func(t *testing.T) {
		if createdUserID == nil {
			t.Skip("no user created")
		}

		resp, err := ctx.Client.Get(fmt.Sprintf("/api/v1/users/%v", createdUserID))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		ctx.Assertions.StatusOK(resp)

		var user map[string]interface{}
		if err := testutil.DecodeJSON(resp, &user); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		ctx.Assertions.Equal(user["username"], testUsername)
		// Email might have been updated in a previous run, so just check it's not empty
		ctx.Assertions.True(user["email"] != nil && user["email"] != "", "user should have email")
	})

	t.Run("update user", func(t *testing.T) {
		if createdUserID == nil {
			t.Skip("no user created")
		}

		updates := map[string]interface{}{
			"email": "e2e-test-updated@example.com",
		}
		resp, err := ctx.Client.Put(fmt.Sprintf("/api/v1/users/%v", createdUserID), updates)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		ctx.Assertions.StatusOK(resp)
	})

	t.Run("delete user", func(t *testing.T) {
		if createdUserID == nil {
			t.Skip("no user created")
		}

		resp, err := ctx.Client.Delete(fmt.Sprintf("/api/v1/users/%v", createdUserID))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		ctx.Assertions.StatusOK(resp)
	})

	t.Run("create user with duplicate username", func(t *testing.T) {
		user := map[string]interface{}{
			"username": cfg.AdminUsername, // Already exists
			"email":    "duplicate@example.com",
			"password": "TestUser123!",
			"role":     "viewer",
		}
		resp, err := ctx.Client.Post("/api/v1/users", user)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		// Should fail with conflict or bad request
		if resp.StatusCode != 409 && resp.StatusCode != 400 {
			t.Errorf("expected 409 or 400, got %d", resp.StatusCode)
		}
	})

	t.Run("create user with invalid role", func(t *testing.T) {
		// Use a unique timestamp to avoid collision with existing users
		uniqueName := fmt.Sprintf("invalid-role-user-%d", time.Now().UnixNano())
		user := map[string]interface{}{
			"username": uniqueName,
			"email":    "invalid@example.com",
			"password": "TestUser123!",
			"role":     "superadmin", // Invalid role
		}
		resp, err := ctx.Client.Post("/api/v1/users", user)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// API validates roles - must be admin, user, or viewer
		ctx.Assertions.StatusBadRequest(resp)
	})

	t.Run("create user with weak password", func(t *testing.T) {
		user := map[string]interface{}{
			"username": "weak-pass-user",
			"email":    "weak@example.com",
			"password": "123", // Too weak
			"role":     "viewer",
		}
		resp, err := ctx.Client.Post("/api/v1/users", user)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		ctx.Assertions.StatusBadRequest(resp)
	})

	t.Cleanup(func() {
		ctx.CleanupResources()
	})
}

// TestUsersRBAC tests role-based access control for users API.
func TestUsersRBAC(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// First, login as admin and create a viewer user
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	viewerUser := map[string]interface{}{
		"username": "e2e-viewer-rbac",
		"email":    "viewer-rbac@example.com",
		"password": "ViewerPass123!",
		"role":     "viewer",
	}
	resp, err := ctx.Client.Post("/api/v1/users", viewerUser)
	if err != nil {
		t.Fatalf("failed to create viewer user: %v", err)
	}
	resp.Body.Close()

	var viewerResult *testutil.PaginatedResponse
	resp, _ = ctx.Client.Get("/api/v1/users")
	viewerResult, _ = testutil.DecodePaginatedJSON(resp)
	_ = viewerResult // Use result if needed

	// Now login as the viewer
	viewerCtx := testutil.NewAPITestContext(t)
	if err := viewerCtx.Login("e2e-viewer-rbac", "ViewerPass123!"); err != nil {
		t.Skipf("Could not login as viewer: %v", err)
	}

	t.Run("viewer cannot create users", func(t *testing.T) {
		newUser := map[string]interface{}{
			"username": "viewer-created-user",
			"email":    "created@example.com",
			"password": "Password123!",
			"role":     "viewer",
		}
		resp, err := viewerCtx.Client.Post("/api/v1/users", newUser)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		viewerCtx.Assertions.StatusForbidden(resp)
	})

	t.Run("viewer cannot delete users", func(t *testing.T) {
		resp, err := viewerCtx.Client.Delete("/api/v1/users/1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		viewerCtx.Assertions.StatusForbidden(resp)
	})

	// Cleanup: delete the viewer user
	t.Cleanup(func() {
		ctx.Cleanup.DeleteUser("e2e-viewer-rbac")
	})
}
