//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestProjectsAPI tests the projects API CRUD operations.
func TestProjectsAPI(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	t.Run("list projects", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/projects")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)

		var projects []map[string]interface{}
		if err := testutil.DecodeJSON(resp, &projects); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
	})

	var createdProjectID interface{}
	createdProjectName := "e2e-test-project"

	t.Run("create project", func(t *testing.T) {
		project := map[string]interface{}{
			"name":       createdProjectName,
			"repository": "https://github.com/test/repo.git",
			"branch":     "main",
			"deployPath": "/deploy/e2e-test",
			"type":       "nodejs",
		}

		resp, err := ctx.Client.Post("/api/v1/projects", project)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Accept 201 Created or 409 Conflict (already exists)
		if resp.StatusCode == http.StatusConflict {
			// Project already exists, list to find it
			listResp, err := ctx.Client.Get("/api/v1/projects")
			if err != nil {
				t.Fatalf("failed to list projects: %v", err)
			}
			defer listResp.Body.Close()

			var projects []map[string]interface{}
			if err := testutil.DecodeJSON(listResp, &projects); err != nil {
				t.Fatalf("failed to decode projects list: %v", err)
			}
			for _, p := range projects {
				if p["name"] == createdProjectName {
					createdProjectID = p["id"]
					break
				}
			}
			t.Log("project already exists, using existing")
		} else {
			ctx.Assertions.StatusCreated(resp)

			var result map[string]interface{}
			if err := testutil.DecodeJSON(resp, &result); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			createdProjectID = result["id"]
		}

		ctx.TrackResource("project", createdProjectID)
	})

	t.Run("get project", func(t *testing.T) {
		if createdProjectID == nil {
			t.Skip("no project created")
		}

		// Use project ID in the URL
		resp, err := ctx.Client.Get(fmt.Sprintf("/api/v1/projects/%v", createdProjectID))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)

		var project map[string]interface{}
		if err := testutil.DecodeJSON(resp, &project); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		ctx.Assertions.Equal(project["name"], "e2e-test-project")
		ctx.Assertions.Equal(project["branch"], "main")
	})

	t.Run("update project", func(t *testing.T) {
		if createdProjectID == nil {
			t.Skip("no project created")
		}

		updates := map[string]interface{}{
			"branch": "develop",
		}

		resp, err := ctx.Client.Put(fmt.Sprintf("/api/v1/projects/%v", createdProjectID), updates)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)

		// Verify the update
		getResp, _ := ctx.Client.Get(fmt.Sprintf("/api/v1/projects/%v", createdProjectID))
		defer getResp.Body.Close()

		var project map[string]interface{}
		testutil.DecodeJSON(getResp, &project)
		ctx.Assertions.Equal(project["branch"], "develop")
	})

	t.Run("get nonexistent project", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/projects/99999")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusNotFound(resp)
	})

	t.Run("create project with duplicate name", func(t *testing.T) {
		if createdProjectName == "" {
			t.Skip("no project created")
		}

		project := map[string]interface{}{
			"name":       "e2e-test-project", // Same name
			"repository": "https://github.com/test/other.git",
			"branch":     "main",
			"deployPath": "/deploy/other",
		}

		resp, err := ctx.Client.Post("/api/v1/projects", project)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should fail with conflict or bad request
		if resp.StatusCode != 409 && resp.StatusCode != 400 {
			t.Errorf("expected 409 or 400, got %d", resp.StatusCode)
		}
	})

	t.Run("create project with missing required fields", func(t *testing.T) {
		project := map[string]interface{}{
			"name": "incomplete-project-" + fmt.Sprint(time.Now().Unix()),
			// Missing repository, branch, deployPath
		}

		resp, err := ctx.Client.Post("/api/v1/projects", project)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// API requires repository - should return 400 Bad Request
		ctx.Assertions.StatusBadRequest(resp)
	})

	t.Run("create project with invalid repository URL", func(t *testing.T) {
		project := map[string]interface{}{
			"name":       "invalid-repo-project-" + fmt.Sprint(time.Now().Unix()),
			"repository": "not-a-valid-url",
			"branch":     "main",
			"deployPath": "/deploy/invalid",
		}

		resp, err := ctx.Client.Post("/api/v1/projects", project)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// API validates repository URL format - should return 400 Bad Request
		ctx.Assertions.StatusBadRequest(resp)
	})

	t.Run("delete project", func(t *testing.T) {
		if createdProjectID == nil {
			t.Skip("no project created")
		}

		resp, err := ctx.Client.Delete(fmt.Sprintf("/api/v1/projects/%v", createdProjectID))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Cleanup(func() {
		ctx.CleanupResources()
	})
}

// TestProjectDeployments tests project deployment operations.
func TestProjectDeployments(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	// Create a test project first
	projectName := "e2e-deploy-test-project"
	project := map[string]interface{}{
		"name":       projectName,
		"repository": "https://github.com/test/repo.git",
		"branch":     "main",
		"deployPath": "/deploy/deploy-test",
		"type":       "static",
	}

	resp, err := ctx.Client.Post("/api/v1/projects", project)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	defer resp.Body.Close()

	var projectID interface{}
	if resp.StatusCode == http.StatusConflict {
		// Find existing project ID
		listResp, _ := ctx.Client.Get("/api/v1/projects")
		defer listResp.Body.Close()
		var projects []map[string]interface{}
		testutil.DecodeJSON(listResp, &projects)
		for _, p := range projects {
			if p["name"] == projectName {
				projectID = p["id"]
				break
			}
		}
	} else {
		var result map[string]interface{}
		testutil.DecodeJSON(resp, &result)
		projectID = result["id"]
	}

	t.Run("trigger deployment", func(t *testing.T) {
		deployReq := map[string]interface{}{
			"branch": "main",
		}

		resp, err := ctx.Client.Post(fmt.Sprintf("/api/v1/projects/%v/deploy", projectID), deployReq)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Deployment may fail due to no agents (4xx) but should not have server errors (5xx)
		// NoServerError is appropriate here since we're testing the endpoint works, not the deployment itself
		ctx.Assertions.NoServerError(resp)
	})

	t.Cleanup(func() {
		if projectID != nil {
			ctx.Client.Delete(fmt.Sprintf("/api/v1/projects/%v", projectID))
		}
	})
}

// TestProjectHealthCheck tests project health check configuration.
func TestProjectHealthCheck(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	// Create a test project first
	projectName := "e2e-health-test-project"
	project := map[string]interface{}{
		"name":       projectName,
		"repository": "https://github.com/test/repo.git",
		"branch":     "main",
		"deployPath": "/deploy/health-test",
	}

	resp, err := ctx.Client.Post("/api/v1/projects", project)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	defer resp.Body.Close()

	var projectID interface{}
	if resp.StatusCode == http.StatusConflict {
		// Find existing project ID
		listResp, _ := ctx.Client.Get("/api/v1/projects")
		defer listResp.Body.Close()
		var projects []map[string]interface{}
		testutil.DecodeJSON(listResp, &projects)
		for _, p := range projects {
			if p["name"] == projectName {
				projectID = p["id"]
				break
			}
		}
	} else {
		var result map[string]interface{}
		testutil.DecodeJSON(resp, &result)
		projectID = result["id"]
	}

	t.Run("configure health check", func(t *testing.T) {
		healthConfig := map[string]interface{}{
			"enabled":              true,
			"url":                  "http://localhost:8080/health",
			"method":               "GET",
			"timeoutSeconds":       30,
			"retries":              3,
			"retryDelaySeconds":    5,
			"expectedStatus":       200,
			"autoRollbackEnabled":  true,
			"autoRollbackReleases": 1,
		}

		resp, err := ctx.Client.Put(fmt.Sprintf("/api/v1/projects/%v/health-config", projectID), healthConfig)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("get health check config", func(t *testing.T) {
		resp, err := ctx.Client.Get(fmt.Sprintf("/api/v1/projects/%v/health-config", projectID))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Cleanup(func() {
		if projectID != nil {
			ctx.Client.Delete(fmt.Sprintf("/api/v1/projects/%v", projectID))
		}
	})
}
