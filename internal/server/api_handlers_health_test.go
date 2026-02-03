package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func TestHandleHealthCheckConfigs(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	t.Run("GET - list configs", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/health-checks", http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleHealthCheckConfigs(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify paginated response structure
		if _, ok := resp["items"]; !ok {
			t.Error("Response missing 'items' field")
		}
		if _, ok := resp["totalCount"]; !ok {
			t.Error("Response missing 'totalCount' field")
		}

		items, ok := resp["items"].([]interface{})
		if !ok {
			t.Fatalf("items is not an array")
		}
		// Should include the default global config from migration
		if len(items) < 1 {
			t.Errorf("Expected at least 1 config, got %d", len(items))
		}
	})

	t.Run("POST - create config", func(t *testing.T) {
		config := &storage.HealthCheckConfig{
			ProjectID:         nil,
			Name:              "test-config",
			URL:               "http://localhost:8080/health",
			Method:            "GET",
			ExpectedStatus:    200,
			TimeoutSeconds:    10,
			Retries:           3,
			RetryDelaySeconds: 5,
			Enabled:           true,
			IsGlobal:          false,
		}

		body, _ := json.Marshal(config)
		req := requestWithAdminContext(httptest.NewRequest("POST", "/api/v1/health-checks", bytes.NewReader(body)), userID)
		rr := httptest.NewRecorder()
		s.handleHealthCheckConfigs(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d: %s", http.StatusCreated, rr.Code, rr.Body.String())
		}

		var created storage.HealthCheckConfig
		if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if created.Name != "test-config" {
			t.Errorf("Expected name 'test-config', got '%s'", created.Name)
		}
		if created.ID == 0 {
			t.Error("Expected config ID to be set")
		}
	})

	t.Run("POST - invalid config (no URL)", func(t *testing.T) {
		config := &storage.HealthCheckConfig{
			Name: "invalid-config",
		}

		body, _ := json.Marshal(config)
		req := requestWithAdminContext(httptest.NewRequest("POST", "/api/v1/health-checks", bytes.NewReader(body)), userID)
		rr := httptest.NewRecorder()
		s.handleHealthCheckConfigs(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d: %s", http.StatusBadRequest, rr.Code, rr.Body.String())
		}
	})
}

func TestHandleHealthCheckConfig(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	// Create a test config first
	ctx := context.Background()
	config := &storage.HealthCheckConfig{
		Name:              "single-test",
		URL:               "http://localhost:9000/health",
		Method:            "GET",
		ExpectedStatus:    200,
		TimeoutSeconds:    10,
		Retries:           3,
		RetryDelaySeconds: 5,
		Enabled:           true,
	}
	if err := s.store.CreateHealthCheckConfig(ctx, config); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	t.Run("GET - get config by ID", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", fmt.Sprintf("/api/v1/health-checks/%d", config.ID), http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleHealthCheckConfig(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var got storage.HealthCheckConfig
		if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if got.Name != "single-test" {
			t.Errorf("Expected name 'single-test', got '%s'", got.Name)
		}
	})

	t.Run("PUT - update config", func(t *testing.T) {
		// Use pointer-based partial update
		name := "updated-test"
		timeout := 20

		update := struct {
			Name           *string `json:"name"`
			TimeoutSeconds *int    `json:"timeoutSeconds"`
		}{
			Name:           &name,
			TimeoutSeconds: &timeout,
		}

		body, _ := json.Marshal(update)
		req := requestWithAdminContext(httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/health-checks/%d", config.ID), bytes.NewReader(body)), userID)
		rr := httptest.NewRecorder()
		s.handleHealthCheckConfig(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var updated storage.HealthCheckConfig
		if err := json.NewDecoder(rr.Body).Decode(&updated); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if updated.Name != "updated-test" {
			t.Errorf("Expected name 'updated-test', got '%s'", updated.Name)
		}
		if updated.TimeoutSeconds != 20 {
			t.Errorf("Expected timeout 20, got %d", updated.TimeoutSeconds)
		}
	})

	t.Run("DELETE - delete config", func(t *testing.T) {
		// Create another config to delete
		toDelete := &storage.HealthCheckConfig{
			Name:              "to-delete",
			URL:               "http://localhost/delete",
			Method:            "GET",
			ExpectedStatus:    200,
			TimeoutSeconds:    10,
			Retries:           1,
			RetryDelaySeconds: 1,
			Enabled:           true,
		}
		if err := s.store.CreateHealthCheckConfig(ctx, toDelete); err != nil {
			t.Fatalf("Failed to create config to delete: %v", err)
		}

		req := requestWithAdminContext(httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/health-checks/%d", toDelete.ID), http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleHealthCheckConfig(rr, req)

		// DELETE returns 204 No Content
		if rr.Code != http.StatusNoContent {
			t.Errorf("Expected status %d, got %d: %s", http.StatusNoContent, rr.Code, rr.Body.String())
		}

		// Verify deletion
		_, err := s.store.GetHealthCheckConfig(ctx, toDelete.ID)
		if err == nil {
			t.Error("Expected error when getting deleted config")
		}
	})

	t.Run("GET - not found", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/health-checks/99999", http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleHealthCheckConfig(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d: %s", http.StatusNotFound, rr.Code, rr.Body.String())
		}
	})

	t.Run("DELETE - not found", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("DELETE", "/api/v1/health-checks/99999", http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleHealthCheckConfig(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d: %s", http.StatusNotFound, rr.Code, rr.Body.String())
		}
	})

	t.Run("DELETE - cannot delete global config", func(t *testing.T) {
		// Get the global config ID
		globalConfig, err := s.store.GetGlobalHealthCheckConfig(ctx)
		if err != nil {
			t.Skipf("No global config to test: %v", err)
		}

		req := requestWithAdminContext(httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/health-checks/%d", globalConfig.ID), http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleHealthCheckConfig(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d: %s", http.StatusBadRequest, rr.Code, rr.Body.String())
		}
	})
}

func TestHandleGlobalHealthCheck(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	t.Run("GET - get global config", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/health-checks/global", http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleGlobalHealthCheck(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var config storage.HealthCheckConfig
		if err := json.NewDecoder(rr.Body).Decode(&config); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if !config.IsGlobal {
			t.Error("Expected global config, got non-global")
		}
	})
}

func TestHandleProjectHealthConfig(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	// Create a test project first
	ctx := context.Background()
	project := &storage.Project{
		Name:                 "health-test-project",
		Repository:           "https://github.com/test/repo",
		Branch:               "main",
		DeployPath:           "/var/www/test",
		AutoRollbackEnabled:  false,
		RollbackOnHealthFail: false,
	}
	if err := s.store.CreateProject(project); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	t.Run("GET - get project health config", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/projects/health-test-project/health-config", http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleProjectHealthConfig(rr, req, project.ID)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		// The response is the health check config, which may be global or project-specific
		var config storage.HealthCheckConfig
		if err := json.NewDecoder(rr.Body).Decode(&config); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Should return the global config since no project-specific one is set
		if !config.IsGlobal {
			t.Log("Got project-specific config instead of global")
		}
	})

	t.Run("PUT - update project health config", func(t *testing.T) {
		// Create a health check config to link
		healthConfig := &storage.HealthCheckConfig{
			ProjectID:         &project.ID,
			Name:              "project-health-check",
			URL:               "http://localhost:8080/health",
			Method:            "GET",
			ExpectedStatus:    200,
			TimeoutSeconds:    15,
			Retries:           3,
			RetryDelaySeconds: 5,
			Enabled:           true,
		}
		if err := s.store.CreateHealthCheckConfig(ctx, healthConfig); err != nil {
			t.Fatalf("Failed to create health config: %v", err)
		}

		autoRollback := true
		rollbackOnHealth := true
		update := struct {
			HealthCheckID        *int64 `json:"healthCheckId"`
			AutoRollbackEnabled  *bool  `json:"autoRollbackEnabled"`
			RollbackOnHealthFail *bool  `json:"rollbackOnHealthFail"`
		}{
			HealthCheckID:        &healthConfig.ID,
			AutoRollbackEnabled:  &autoRollback,
			RollbackOnHealthFail: &rollbackOnHealth,
		}

		body, _ := json.Marshal(update)
		req := requestWithAdminContext(httptest.NewRequest("PUT", "/api/v1/projects/health-test-project/health-config", bytes.NewReader(body)), userID)
		rr := httptest.NewRecorder()
		s.handleProjectHealthConfig(rr, req, project.ID)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		// Verify the response indicates success
		var response map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if response["auto_rollback_enabled"] != true {
			t.Errorf("Expected auto_rollback_enabled=true, got %v", response["auto_rollback_enabled"])
		}
		if response["rollback_on_health_fail"] != true {
			t.Errorf("Expected rollback_on_health_fail=true, got %v", response["rollback_on_health_fail"])
		}
	})
}

func TestHandleRollbackRecords(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	// Create test project and deployment first
	ctx := context.Background()
	project := &storage.Project{
		Name:       "rollback-test-project",
		Repository: "https://github.com/test/repo",
		Branch:     "main",
		DeployPath: "/var/www/test",
	}
	if err := s.store.CreateProject(project); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	deployment := &storage.DeploymentRecord{
		ID:            "test-deploy-rb1",
		Project:       project.Name,
		Target:        "production",
		Branch:        "main",
		Status:        "running",
		ReleaseNumber: 1,
		StartedAt:     time.Now(),
		TriggeredBy:   "test",
	}
	if err := s.store.CreateDeployment(ctx, deployment); err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	// Create a rollback record
	rollback := &storage.DeploymentRollback{
		DeploymentID:      deployment.ID,
		ProjectName:       project.Name,
		FromRelease:       2,
		ToRelease:         1,
		Reason:            "Health check failed",
		TriggeredBy:       storage.RollbackTriggerAutoHealthFail,
		HealthCheckFailed: true,
		HealthCheckError:  "Status 500",
		Status:            "completed",
		StartedAt:         time.Now(),
	}
	if err := s.store.CreateDeploymentRollback(ctx, rollback); err != nil {
		t.Fatalf("Failed to create test rollback: %v", err)
	}

	t.Run("GET - list rollbacks", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/rollbacks", http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleRollbackRecords(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var response struct {
			Items  []*storage.DeploymentRollback `json:"items"`
			Total  int                           `json:"total"`
			Limit  int                           `json:"limit"`
			Offset int                           `json:"offset"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(response.Items) != 1 {
			t.Errorf("Expected 1 rollback, got %d", len(response.Items))
		}
	})

	t.Run("GET - filter by project", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/rollbacks?project="+project.Name, http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleRollbackRecords(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var response struct {
			Items  []*storage.DeploymentRollback `json:"items"`
			Total  int                           `json:"total"`
			Limit  int                           `json:"limit"`
			Offset int                           `json:"offset"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(response.Items) != 1 {
			t.Errorf("Expected 1 rollback, got %d", len(response.Items))
		}
	})

	t.Run("GET - filter by non-existent project", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/rollbacks?project=nonexistent", http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleRollbackRecords(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var response struct {
			Items  []*storage.DeploymentRollback `json:"items"`
			Total  int                           `json:"total"`
			Limit  int                           `json:"limit"`
			Offset int                           `json:"offset"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(response.Items) != 0 {
			t.Errorf("Expected 0 rollbacks, got %d", len(response.Items))
		}
	})
}

func TestHandleRollbackRecord(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	// Create test project and deployment first
	ctx := context.Background()
	project := &storage.Project{
		Name:       "rollback-record-test",
		Repository: "https://github.com/test/repo",
		Branch:     "main",
		DeployPath: "/var/www/test",
	}
	if err := s.store.CreateProject(project); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	deployment := &storage.DeploymentRecord{
		ID:            "test-deploy-rb2",
		Project:       project.Name,
		Target:        "production",
		Branch:        "main",
		Status:        "success",
		ReleaseNumber: 1,
		StartedAt:     time.Now(),
		TriggeredBy:   "test",
	}
	if err := s.store.CreateDeployment(ctx, deployment); err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	// Create a rollback record
	rollback := &storage.DeploymentRollback{
		DeploymentID:      deployment.ID,
		ProjectName:       project.Name,
		FromRelease:       3,
		ToRelease:         2,
		Reason:            "Manual rollback",
		TriggeredBy:       storage.RollbackTriggerUser,
		HealthCheckFailed: false,
		Status:            "completed",
		StartedAt:         time.Now(),
	}
	if err := s.store.CreateDeploymentRollback(ctx, rollback); err != nil {
		t.Fatalf("Failed to create test rollback: %v", err)
	}

	t.Run("GET - get rollback by ID", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", fmt.Sprintf("/api/v1/rollbacks/%d", rollback.ID), http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleRollbackRecord(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var got storage.DeploymentRollback
		if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if got.ProjectName != project.Name {
			t.Errorf("Expected project name '%s', got '%s'", project.Name, got.ProjectName)
		}
		if got.FromRelease != 3 {
			t.Errorf("Expected from_release 3, got %d", got.FromRelease)
		}
	})

	t.Run("GET - not found", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/rollbacks/99999", http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleRollbackRecord(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d: %s", http.StatusNotFound, rr.Code, rr.Body.String())
		}
	})
}

func TestHandleTestHealthCheck(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	// Create a test HTTP server to check against
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer testServer.Close()

	// Create a health config that points to our test server
	ctx := context.Background()
	config := &storage.HealthCheckConfig{
		Name:              "test-server-health",
		URL:               testServer.URL + "/health",
		Method:            "GET",
		ExpectedStatus:    200,
		TimeoutSeconds:    10,
		Retries:           1,
		RetryDelaySeconds: 1,
		Enabled:           true,
	}
	if err := s.store.CreateHealthCheckConfig(ctx, config); err != nil {
		t.Fatalf("Failed to create health config: %v", err)
	}

	t.Run("POST - test health check success", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("POST", fmt.Sprintf("/api/v1/health-checks/%d/test", config.ID), http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleHealthCheckConfig(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var result storage.HealthCheckResult
		if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if !result.Success {
			t.Errorf("Expected success=true, got false. Error: %s", result.ErrorMessage)
		}
		if result.StatusCode != 200 {
			t.Errorf("Expected status code 200, got %d", result.StatusCode)
		}
	})

	// Create a config that will fail
	failConfig := &storage.HealthCheckConfig{
		Name:              "fail-test",
		URL:               testServer.URL + "/nonexistent",
		Method:            "GET",
		ExpectedStatus:    200,
		TimeoutSeconds:    10,
		Retries:           1,
		RetryDelaySeconds: 1,
		Enabled:           true,
	}
	if err := s.store.CreateHealthCheckConfig(ctx, failConfig); err != nil {
		t.Fatalf("Failed to create fail config: %v", err)
	}

	t.Run("POST - test health check failure", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("POST", fmt.Sprintf("/api/v1/health-checks/%d/test", failConfig.ID), http.NoBody), userID)
		rr := httptest.NewRecorder()
		s.handleHealthCheckConfig(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var result storage.HealthCheckResult
		if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result.Success {
			t.Error("Expected success=false, got true")
		}
		if result.StatusCode != 404 {
			t.Errorf("Expected status code 404, got %d", result.StatusCode)
		}
	})
}
