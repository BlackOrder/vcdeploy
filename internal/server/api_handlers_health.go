package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/validation"
	"go.uber.org/zap"
)

// handleHealthCheckConfigs handles /api/v1/health-checks
func (s *MasterServer) handleHealthCheckConfigs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListHealthCheckConfigs(w, r)
	case http.MethodPost:
		s.handleCreateHealthCheckConfig(w, r)
	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleHealthCheckConfig handles /api/v1/health-checks/{id}
func (s *MasterServer) handleHealthCheckConfig(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/health-checks/")
	idStr := strings.Split(path, "/")[0]
	id := idStr
	if id == "" {
		s.jsonError(w, http.StatusBadRequest, "invalid health check config ID")
		return
	}

	// Check for sub-path
	if strings.Contains(path, "/test") {
		if r.Method == http.MethodPost {
			s.handleTestHealthCheck(w, r, id)
			return
		}
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetHealthCheckConfig(w, r, id)
	case http.MethodPut:
		s.handleUpdateHealthCheckConfig(w, r, id)
	case http.MethodDelete:
		s.handleDeleteHealthCheckConfig(w, r, id)
	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleListHealthCheckConfigs lists all health check configurations.
func (s *MasterServer) handleListHealthCheckConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	configs, err := s.store.ListHealthCheckConfigs(ctx)
	if err != nil {
		s.logger.Error("Failed to list health check configs", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to list health check configs")
		return
	}

	// Apply pagination
	p := parsePagination(r)
	totalCount := len(configs)

	// Apply offset
	if p.Offset >= totalCount {
		configs = []*storage.HealthCheckConfig{}
	} else {
		configs = configs[p.Offset:]
		// Apply limit
		if p.Limit > 0 && p.Limit < len(configs) {
			configs = configs[:p.Limit]
		}
	}

	s.jsonResponse(w, PaginatedResponse{
		Items:      configs,
		TotalCount: int64(totalCount),
		Limit:      p.Limit,
		Offset:     p.Offset,
	})
}

// handleCreateHealthCheckConfig creates a new health check configuration.
func (s *MasterServer) handleCreateHealthCheckConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var config storage.HealthCheckConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&config); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate required fields
	if config.Name == "" {
		s.jsonError(w, http.StatusBadRequest, "name is required")
		return
	}
	if config.URL == "" {
		s.jsonError(w, http.StatusBadRequest, "URL is required")
		return
	}

	// Set defaults
	if config.Method == "" {
		config.Method = "GET"
	}
	if config.ExpectedStatus == 0 {
		config.ExpectedStatus = 200
	}
	if config.TimeoutSeconds == 0 {
		config.TimeoutSeconds = 10
	}
	if config.Retries == 0 {
		config.Retries = 3
	}
	if config.RetryDelaySeconds == 0 {
		config.RetryDelaySeconds = 5
	}
	config.Enabled = true

	// Can't create another global config
	if config.IsGlobal {
		s.jsonError(w, http.StatusBadRequest, "cannot create another global config; update the existing one")
		return
	}

	if err := s.store.CreateHealthCheckConfig(ctx, &config); err != nil {
		s.logger.Error("Failed to create health check config", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to create health check config")
		return
	}

	s.writeJSON(w, http.StatusCreated, config)
}

// handleGetHealthCheckConfig retrieves a health check configuration.
func (s *MasterServer) handleGetHealthCheckConfig(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	config, err := s.store.GetHealthCheckConfig(ctx, id)
	if services.IsNotFound(err) {
		s.jsonError(w, http.StatusNotFound, "health check config not found")
		return
	}
	if err != nil {
		s.logger.Error("Failed to get health check config", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to get health check config")
		return
	}

	s.jsonResponse(w, config)
}

// handleUpdateHealthCheckConfig updates a health check configuration.
func (s *MasterServer) handleUpdateHealthCheckConfig(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	// Get existing config
	existing, err := s.store.GetHealthCheckConfig(ctx, id)
	if services.IsNotFound(err) {
		s.jsonError(w, http.StatusNotFound, "health check config not found")
		return
	}
	if err != nil {
		s.logger.Error("Failed to get health check config", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to get health check config")
		return
	}

	// Parse request body
	var updates struct {
		Name              *string `json:"name"`
		URL               *string `json:"url"`
		Method            *string `json:"method"`
		ExpectedStatus    *int    `json:"expectedStatus"`
		TimeoutSeconds    *int    `json:"timeoutSeconds"`
		Retries           *int    `json:"retries"`
		RetryDelaySeconds *int    `json:"retryDelaySeconds"`
		Headers           *string `json:"headers"`
		Body              *string `json:"body"`
		BodyContains      *string `json:"bodyContains"`
		Enabled           *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&updates); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Apply updates
	if updates.Name != nil {
		existing.Name = *updates.Name
	}
	if updates.URL != nil {
		existing.URL = *updates.URL
	}
	if updates.Method != nil {
		// M8 FIX: Validate HTTP method
		method := strings.ToUpper(*updates.Method)
		validMethods := map[string]bool{"GET": true, "POST": true, "PUT": true, "HEAD": true, "OPTIONS": true}
		if !validMethods[method] {
			s.jsonError(w, http.StatusBadRequest, "invalid method: must be GET, POST, PUT, HEAD, or OPTIONS")
			return
		}
		existing.Method = method
	}
	if updates.ExpectedStatus != nil {
		// M8 FIX: Validate HTTP status code range
		status := *updates.ExpectedStatus
		if status < 100 || status >= 600 {
			s.jsonError(w, http.StatusBadRequest, "invalid expectedStatus: must be between 100 and 599")
			return
		}
		existing.ExpectedStatus = status
	}
	if updates.TimeoutSeconds != nil {
		// M8 FIX: Validate timeout range
		timeout := *updates.TimeoutSeconds
		if timeout < 1 || timeout > 300 {
			s.jsonError(w, http.StatusBadRequest, "invalid timeoutSeconds: must be between 1 and 300")
			return
		}
		existing.TimeoutSeconds = timeout
	}
	if updates.Retries != nil {
		// M8 FIX: Validate retries range
		retries := *updates.Retries
		if retries < 0 || retries > 10 {
			s.jsonError(w, http.StatusBadRequest, "invalid retries: must be between 0 and 10")
			return
		}
		existing.Retries = retries
	}
	if updates.RetryDelaySeconds != nil {
		// M8 FIX: Validate retry delay range
		delay := *updates.RetryDelaySeconds
		if delay < 0 || delay > 60 {
			s.jsonError(w, http.StatusBadRequest, "invalid retryDelaySeconds: must be between 0 and 60")
			return
		}
		existing.RetryDelaySeconds = delay
	}
	if updates.Headers != nil {
		existing.Headers = *updates.Headers
	}
	if updates.Body != nil {
		existing.Body = *updates.Body
	}
	if updates.BodyContains != nil {
		existing.BodyContains = *updates.BodyContains
	}
	if updates.Enabled != nil {
		existing.Enabled = *updates.Enabled
	}

	if err := s.store.UpdateHealthCheckConfig(ctx, existing); err != nil {
		s.logger.Error("Failed to update health check config", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to update health check config")
		return
	}

	s.jsonResponse(w, existing)
}

// handleDeleteHealthCheckConfig deletes a health check configuration.
func (s *MasterServer) handleDeleteHealthCheckConfig(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	if err := s.store.DeleteHealthCheckConfig(ctx, id); err != nil {
		if err.Error() == "cannot delete global health check config" {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "health check config not found")
			return
		}
		s.logger.Error("Failed to delete health check config", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to delete health check config")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleGlobalHealthCheck handles /api/v1/health-checks/global
func (s *MasterServer) handleGlobalHealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	config, err := s.store.GetGlobalHealthCheckConfig(ctx)
	if services.IsNotFound(err) {
		s.jsonError(w, http.StatusNotFound, "global health check config not found")
		return
	}
	if err != nil {
		s.logger.Error("Failed to get global health check config", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to get global health check config")
		return
	}

	s.jsonResponse(w, config)
}

// handleProjectHealthConfig handles /api/v1/projects/{id}/health-config
func (s *MasterServer) handleProjectHealthConfig(w http.ResponseWriter, r *http.Request, projectID string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// Get health check config for project (uses global if none set)
		config, err := s.store.GetHealthCheckConfigForProject(ctx, projectID)
		if services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "no health check config found")
			return
		}
		if err != nil {
			s.logger.Error("Failed to get project health config", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "failed to get project health config")
			return
		}

		s.jsonResponse(w, config)

	case http.MethodPut:
		var req struct {
			HealthCheckID        *string `json:"healthCheckId"`
			AutoRollbackEnabled  *bool   `json:"autoRollbackEnabled"`
			RollbackOnHealthFail *bool   `json:"rollbackOnHealthFail"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		// Validate health check ID if provided
		if req.HealthCheckID != nil && *req.HealthCheckID != "" {
			_, err := s.store.GetHealthCheckConfig(ctx, *req.HealthCheckID)
			if services.IsNotFound(err) {
				s.jsonError(w, http.StatusBadRequest, "health check config not found")
				return
			}
			if err != nil {
				s.logger.Error("Failed to validate health check config", zap.Error(err))
				s.jsonError(w, http.StatusInternalServerError, "failed to validate health check config")
				return
			}
		}

		// Get current project settings for defaults
		autoRollback := true
		rollbackOnHealthFail := true
		if req.AutoRollbackEnabled != nil {
			autoRollback = *req.AutoRollbackEnabled
		}
		if req.RollbackOnHealthFail != nil {
			rollbackOnHealthFail = *req.RollbackOnHealthFail
		}

		if err := s.store.UpdateProjectHealthCheck(ctx, projectID, req.HealthCheckID, autoRollback, rollbackOnHealthFail); err != nil {
			s.logger.Error("Failed to update project health config", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "failed to update project health config")
			return
		}

		s.jsonResponse(w, ProjectHealthConfigResponse{
			HealthCheckID:        req.HealthCheckID,
			AutoRollbackEnabled:  autoRollback,
			RollbackOnHealthFail: rollbackOnHealthFail,
		})

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleRollbackRecords handles /api/v1/rollbacks
func (s *MasterServer) handleRollbackRecords(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	projectName := r.URL.Query().Get("project")
	p := parsePaginationWithDefaults(r, 20)

	rollbacks, total, err := s.store.ListDeploymentRollbacks(ctx, projectName, p.Limit, p.Offset)
	if err != nil {
		s.logger.Error("Failed to list deployment rollbacks", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to list deployment rollbacks")
		return
	}

	s.jsonResponse(w, RollbackListResponse{
		Items:      rollbacks,
		TotalCount: total,
		Limit:      p.Limit,
		Offset:     p.Offset,
	})
}

// handleRollbackRecord handles /api/v1/rollbacks/{id}
func (s *MasterServer) handleRollbackRecord(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/rollbacks/")
	idStr := strings.Split(path, "/")[0]
	id := idStr
	if id == "" {
		s.jsonError(w, http.StatusBadRequest, "invalid rollback ID")
		return
	}

	ctx := r.Context()

	rollback, err := s.store.GetDeploymentRollback(ctx, id)
	if services.IsNotFound(err) {
		s.jsonError(w, http.StatusNotFound, "rollback not found")
		return
	}
	if err != nil {
		s.logger.Error("Failed to get deployment rollback", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to get deployment rollback")
		return
	}

	s.jsonResponse(w, rollback)
}

// handleTestHealthCheck handles POST /api/v1/health-checks/{id}/test
// This allows testing a health check configuration without triggering a deployment.
func (s *MasterServer) handleTestHealthCheck(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	config, err := s.store.GetHealthCheckConfig(ctx, id)
	if services.IsNotFound(err) {
		s.jsonError(w, http.StatusNotFound, "health check config not found")
		return
	}
	if err != nil {
		s.logger.Error("Failed to get health check config", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to get health check config")
		return
	}

	// Parse optional URL override from request
	var reqBody struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&reqBody); err == nil && reqBody.URL != "" {
		config.URL = reqBody.URL
	}

	// Perform the health check directly from the master (for testing purposes)
	result := s.performHealthCheck(ctx, config)

	s.jsonResponse(w, result)
}

// performHealthCheck performs a health check from the master server (for testing).
func (s *MasterServer) performHealthCheck(ctx context.Context, config *storage.HealthCheckConfig) *storage.HealthCheckResult {
	result := &storage.HealthCheckResult{
		ConfigID:  config.ID,
		CheckedAt: time.Now(),
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: time.Duration(config.TimeoutSeconds) * time.Second,
	}

	// Determine method
	method := config.Method
	if method == "" {
		method = "GET"
	}

	// Create request body
	var bodyReader io.Reader
	if config.Body != "" {
		bodyReader = strings.NewReader(config.Body)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, config.URL, bodyReader)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to create request: %v", err)
		return result
	}

	// Add headers
	if config.Headers != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(config.Headers), &headers); err == nil {
			for key, value := range headers {
				req.Header.Set(key, value)
			}
		}
	}

	// Perform request
	start := time.Now()
	resp, err := client.Do(req)
	result.ResponseTimeMs = time.Since(start).Milliseconds()

	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	// Read body
	body, _ := io.ReadAll(resp.Body)

	// Check expected status
	expectedStatus := config.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = 200
	}
	if resp.StatusCode != expectedStatus {
		result.ErrorMessage = fmt.Sprintf("Expected status %d, got %d", expectedStatus, resp.StatusCode)
		return result
	}

	// Check body contains
	if config.BodyContains != "" && !strings.Contains(string(body), config.BodyContains) {
		result.ErrorMessage = fmt.Sprintf("Response body does not contain '%s'", config.BodyContains)
		return result
	}

	result.Success = true
	return result
}
