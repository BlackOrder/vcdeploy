// Package testutil provides test data seeding for E2E, CLI, and integration tests.
package testutil

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// SeedResult contains all created test entity IDs from SeedAll.
type SeedResult struct {
	AdminUserID   interface{}
	ViewerUserID  interface{}
	RegularUserID interface{}
	ProjectID     interface{}
	SecretID      interface{}
	APIKeyID      interface{}
	APIKey        string // The actual API key value
}

// Seeder provides test data seeding capabilities.
type Seeder struct {
	client *HTTPClient
}

// NewSeeder creates a new test data seeder.
func NewSeeder(client *HTTPClient) *Seeder {
	return &Seeder{client: client}
}

// SeedAll creates a complete test environment with all common entities.
// Returns a SeedResult with all created entity IDs or an error.
func (s *Seeder) SeedAll() (*SeedResult, error) {
	result := &SeedResult{}

	// Create admin user
	adminUser, err := s.SeedUser(TestData.AdminUser, TestData.AdminUser+"@test.com", TestData.AdminPass, "admin")
	if err != nil {
		// Check if already exists
		if adminUser == nil {
			return nil, fmt.Errorf("failed to seed admin user: %w", err)
		}
	}
	if adminUser != nil {
		result.AdminUserID = adminUser["id"]
	}

	// Create viewer user
	viewerUser, err := s.SeedUser(TestData.ViewerUser, TestData.ViewerUser+"@test.com", TestData.ViewerPass, "viewer")
	if err != nil {
		if viewerUser == nil {
			return nil, fmt.Errorf("failed to seed viewer user: %w", err)
		}
	}
	if viewerUser != nil {
		result.ViewerUserID = viewerUser["id"]
	}

	// Create regular user (with 'user' role)
	regularUser, err := s.SeedUser(TestData.RegularUser, TestData.RegularUser+"@test.com", TestData.RegularPass, "user")
	if err != nil {
		if regularUser == nil {
			return nil, fmt.Errorf("failed to seed regular user: %w", err)
		}
	}
	if regularUser != nil {
		result.RegularUserID = regularUser["id"]
	}

	// Create test project
	project, err := s.SeedProject(TestData.TestProject1, TestData.TestRepo, TestData.TestBranch, TestData.TestPath, "generic")
	if err != nil {
		if project == nil {
			return nil, fmt.Errorf("failed to seed project: %w", err)
		}
	}
	if project != nil {
		result.ProjectID = project["id"]
	}

	// Create test secret (requires project)
	if result.ProjectID != nil {
		secret, err := s.SeedSecret(TestData.TestProject1, "env", TestData.TestSecretKey, TestData.TestSecretValue)
		if err != nil {
			if secret == nil {
				return nil, fmt.Errorf("failed to seed secret: %w", err)
			}
		}
		if secret != nil {
			result.SecretID = secret["id"]
		}
	}

	// Create API key
	apiKey, err := s.SeedAPIKey("test-seeder-key", []string{"read", "write", "admin"})
	if err != nil {
		if apiKey == nil {
			return nil, fmt.Errorf("failed to seed API key: %w", err)
		}
	}
	if apiKey != nil {
		result.APIKeyID = apiKey["id"]
		if key, ok := apiKey["key"].(string); ok {
			result.APIKey = key
		}
	}

	return result, nil
}

// SeedUser creates a test user and returns the user data.
// If the user already exists (409 Conflict), returns nil user without error for idempotency.
func (s *Seeder) SeedUser(username, email, password, role string) (map[string]interface{}, error) {
	user := map[string]interface{}{
		"username": username,
		"email":    email,
		"password": password,
		"role":     role,
	}

	resp, err := s.client.Post("/api/v1/users", user)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	defer resp.Body.Close()

	// Handle duplicate gracefully for idempotent seeding
	if resp.StatusCode == http.StatusConflict {
		log.Printf("[seeder] User %q already exists (conflict), skipping creation", username)
		return nil, nil
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to create user %s: status %d: %s", username, resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := DecodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SeedProject creates a test project and returns the project data.
// If the project already exists (409 Conflict), returns nil project without error for idempotency.
func (s *Seeder) SeedProject(name, repo, branch, deployPath, projectType string) (map[string]interface{}, error) {
	project := map[string]interface{}{
		"name":       name,
		"repository": repo,
		"branch":     branch,
		"deployPath": deployPath,
		"type":       projectType,
	}

	resp, err := s.client.Post("/api/v1/projects", project)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}
	defer resp.Body.Close()

	// Handle duplicate gracefully for idempotent seeding
	if resp.StatusCode == http.StatusConflict {
		log.Printf("[seeder] Project %q already exists (conflict), skipping creation", name)
		return nil, nil
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to create project %s: status %d: %s", name, resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := DecodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SeedSecret creates a test secret and returns the secret data.
// If the secret already exists (409 Conflict), returns nil secret without error for idempotency.
func (s *Seeder) SeedSecret(project, scope, key, value string) (map[string]interface{}, error) {
	secret := map[string]interface{}{
		"project": project,
		"scope":   scope,
		"key":     key,
		"value":   value,
	}

	resp, err := s.client.Post("/api/v1/secrets", secret)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}
	defer resp.Body.Close()

	// Handle duplicate gracefully for idempotent seeding
	if resp.StatusCode == http.StatusConflict {
		log.Printf("[seeder] Secret %q (project=%s, scope=%s) already exists (conflict), skipping creation", key, project, scope)
		return nil, nil
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to create secret %s: status %d: %s", key, resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := DecodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SeedAPIKey creates a test API key and returns the key data.
// If the API key already exists (409 Conflict), returns nil without error for idempotency.
func (s *Seeder) SeedAPIKey(name string, permissions []string) (map[string]interface{}, error) {
	apiKey := map[string]interface{}{
		"name":        name,
		"permissions": permissions,
	}

	resp, err := s.client.Post("/api/v1/api-keys", apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}
	defer resp.Body.Close()

	// Handle duplicate gracefully for idempotent seeding
	if resp.StatusCode == http.StatusConflict {
		log.Printf("[seeder] API key %q already exists (conflict), skipping creation", name)
		return nil, nil
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to create API key %s: status %d: %s", name, resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := DecodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Cleanup removes test data.
type Cleanup struct {
	client *HTTPClient
}

// NewCleanup creates a new cleanup helper.
func NewCleanup(client *HTTPClient) *Cleanup {
	return &Cleanup{client: client}
}

// DeleteUser deletes a user by ID.
func (c *Cleanup) DeleteUser(id interface{}) error {
	resp, err := c.client.Delete(fmt.Sprintf("/api/v1/users/%v", id))
	if err != nil {
		return err
	}
	_ = resp.Body.Close() // #nosec G104 - best effort cleanup in test
	return nil
}

// DeleteProject deletes a project by ID.
func (c *Cleanup) DeleteProject(id interface{}) error {
	resp, err := c.client.Delete(fmt.Sprintf("/api/v1/projects/%v", id))
	if err != nil {
		return err
	}
	_ = resp.Body.Close() // #nosec G104 - best effort cleanup in test
	return nil
}

// DeleteSecret deletes a secret by ID.
func (c *Cleanup) DeleteSecret(id interface{}) error {
	resp, err := c.client.Delete(fmt.Sprintf("/api/v1/secrets/%v", id))
	if err != nil {
		return err
	}
	_ = resp.Body.Close() // #nosec G104 - best effort cleanup in test
	return nil
}

// DeleteAPIKey deletes an API key by ID.
func (c *Cleanup) DeleteAPIKey(id interface{}) error {
	resp, err := c.client.Delete(fmt.Sprintf("/api/v1/api-keys/%v", id))
	if err != nil {
		return err
	}
	_ = resp.Body.Close() // #nosec G104 - best effort cleanup in test
	return nil
}

// TestData holds common test data values.
var TestData = struct {
	// Users
	AdminUser   string
	AdminPass   string
	ViewerUser  string
	ViewerPass  string
	RegularUser string
	RegularPass string

	// Projects
	TestProject1 string
	TestProject2 string
	TestRepo     string
	TestBranch   string
	TestPath     string

	// Secrets
	TestSecretKey   string
	TestSecretValue string
}{
	AdminUser:       "test-admin",
	AdminPass:       "TestAdmin123!",
	ViewerUser:      "test-viewer",
	ViewerPass:      "TestViewer123!",
	RegularUser:     "test-user",
	RegularPass:     "TestUser123!",
	TestProject1:    "e2e-test-project-1",
	TestProject2:    "e2e-test-project-2",
	TestRepo:        "https://github.com/test/repo.git",
	TestBranch:      "main",
	TestPath:        "/deploy/test",
	TestSecretKey:   "E2E_TEST_SECRET",
	TestSecretValue: "test-secret-value",
}

// ========================================
// Deployment Lifecycle Helpers (Step 1)
// ========================================

// TriggerDeployment triggers a deployment for a project.
// Returns the deployment data or an error.
func (s *Seeder) TriggerDeployment(projectID, branch, target string) (map[string]interface{}, error) {
	deployment := map[string]interface{}{
		"projectId": projectID,
		"branch":    branch,
	}
	if target != "" {
		deployment["target"] = target
	}

	resp, err := s.client.Post("/api/v1/deployments", deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to trigger deployment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to trigger deployment: status %d: %s", resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := DecodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// TriggerDeploymentForProject triggers a deployment using the project deploy endpoint.
// This is an alternative to TriggerDeployment using POST /api/v1/projects/{id}/deploy.
func (s *Seeder) TriggerDeploymentForProject(projectID, branch string) (map[string]interface{}, error) {
	deployment := map[string]interface{}{
		"branch": branch,
	}

	resp, err := s.client.Post(fmt.Sprintf("/api/v1/projects/%s/deploy", projectID), deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to trigger deployment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to trigger deployment for project %s: status %d: %s", projectID, resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := DecodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// WaitForDeploymentStatus polls the deployment status until it matches the expected status or times out.
// Valid statuses: pending, running, success, failed, cancelled
func (s *Seeder) WaitForDeploymentStatus(deploymentID, expectedStatus string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		resp, err := s.client.Get(fmt.Sprintf("/api/v1/deployments/%s", deploymentID))
		if err != nil {
			return fmt.Errorf("failed to get deployment status: %w", err)
		}

		var deployment map[string]interface{}
		if err := DecodeJSON(resp, &deployment); err != nil {
			_ = resp.Body.Close() // #nosec G104 - best effort cleanup in test
			return fmt.Errorf("failed to decode deployment: %w", err)
		}
		_ = resp.Body.Close() // #nosec G104 - best effort cleanup in test

		status, ok := deployment["status"].(string)
		if !ok {
			return fmt.Errorf("deployment has no status field")
		}

		if status == expectedStatus {
			return nil
		}

		// Check for terminal states that won't transition further
		if expectedStatus != "failed" && expectedStatus != "cancelled" && expectedStatus != "success" {
			if status == "failed" || status == "cancelled" || status == "success" {
				return fmt.Errorf("deployment reached terminal status %q while waiting for %q", status, expectedStatus)
			}
		}

		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for deployment %s to reach status %s", deploymentID, expectedStatus)
}

// WaitForDeploymentComplete waits for deployment to reach any terminal state (success, failed, cancelled).
// Returns the final status and deployment data.
func (s *Seeder) WaitForDeploymentComplete(deploymentID string, timeout time.Duration) (string, map[string]interface{}, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		resp, err := s.client.Get(fmt.Sprintf("/api/v1/deployments/%s", deploymentID))
		if err != nil {
			return "", nil, fmt.Errorf("failed to get deployment status: %w", err)
		}

		var deployment map[string]interface{}
		if err := DecodeJSON(resp, &deployment); err != nil {
			_ = resp.Body.Close() // #nosec G104 - best effort cleanup in test
			return "", nil, fmt.Errorf("failed to decode deployment: %w", err)
		}
		_ = resp.Body.Close() // #nosec G104 - best effort cleanup in test

		status, ok := deployment["status"].(string)
		if !ok {
			return "", nil, fmt.Errorf("deployment has no status field")
		}

		// Terminal states
		if status == "success" || status == "failed" || status == "cancelled" || status == "completed" {
			return status, deployment, nil
		}

		time.Sleep(pollInterval)
	}

	return "", nil, fmt.Errorf("timeout waiting for deployment %s to complete", deploymentID)
}

// GetDeploymentLogs fetches the logs for a deployment.
func (s *Seeder) GetDeploymentLogs(deploymentID string) ([]string, error) {
	resp, err := s.client.Get(fmt.Sprintf("/api/v1/deployments/%s/logs", deploymentID))
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to get deployment logs: status %d: %s", resp.StatusCode, body)
	}

	// Try to decode as JSON array of log lines or as object with logs field
	var logs []string
	body, err := ReadBody(resp)
	if err != nil {
		return nil, err
	}

	// Try array format first
	if err := json.Unmarshal([]byte(body), &logs); err == nil {
		return logs, nil
	}

	// Try object with logs/lines field
	var logObj map[string]interface{}
	if err := json.Unmarshal([]byte(body), &logObj); err == nil {
		if logLines, ok := logObj["logs"].([]interface{}); ok {
			for _, line := range logLines {
				if str, ok := line.(string); ok {
					logs = append(logs, str)
				}
			}
			return logs, nil
		}
		if logLines, ok := logObj["lines"].([]interface{}); ok {
			for _, line := range logLines {
				if str, ok := line.(string); ok {
					logs = append(logs, str)
				}
			}
			return logs, nil
		}
		// If there's a "content" field (raw logs)
		if content, ok := logObj["content"].(string); ok {
			return []string{content}, nil
		}
	}

	// Return raw body as single log line
	return []string{body}, nil
}

// CancelDeployment cancels a running deployment.
func (s *Seeder) CancelDeployment(deploymentID string) error {
	resp, err := s.client.Post(fmt.Sprintf("/api/v1/deployments/%s/cancel", deploymentID), nil)
	if err != nil {
		return fmt.Errorf("failed to cancel deployment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		body, _ := ReadBody(resp)
		return fmt.Errorf("failed to cancel deployment: status %d: %s", resp.StatusCode, body)
	}

	return nil
}

// TriggerRollback triggers a rollback to a previous deployment.
func (s *Seeder) TriggerRollback(deploymentID string) (map[string]interface{}, error) {
	resp, err := s.client.Post(fmt.Sprintf("/api/v1/deployments/%s/rollback", deploymentID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to trigger rollback: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to trigger rollback: status %d: %s", resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := DecodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteDeployment deletes a deployment record.
func (s *Seeder) DeleteDeployment(deploymentID string) error {
	resp, err := s.client.Delete(fmt.Sprintf("/api/v1/deployments/%s", deploymentID))
	if err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := ReadBody(resp)
		return fmt.Errorf("failed to delete deployment: status %d: %s", resp.StatusCode, body)
	}

	return nil
}

// ========================================
// Webhook Helpers (Step 2)
// ========================================

// SeedWebhook configures a webhook for a project.
func (s *Seeder) SeedWebhook(projectID, provider, webhookURL, secret string) (map[string]interface{}, error) {
	webhook := map[string]interface{}{
		"provider": provider,
		"url":      webhookURL,
		"secret":   secret,
		"active":   true,
	}

	resp, err := s.client.Post(fmt.Sprintf("/api/v1/projects/%s/webhooks", projectID), webhook)
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook: %w", err)
	}
	defer resp.Body.Close()

	// Handle duplicate gracefully
	if resp.StatusCode == http.StatusConflict {
		log.Printf("[seeder] Webhook already exists for project %s, skipping creation", projectID)
		return nil, nil
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to create webhook: status %d: %s", resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := DecodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetProjectWebhooks gets all webhooks for a project.
func (s *Seeder) GetProjectWebhooks(projectID string) ([]map[string]interface{}, error) {
	resp, err := s.client.Get(fmt.Sprintf("/api/v1/projects/%s/webhooks", projectID))
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to get webhooks: status %d: %s", resp.StatusCode, body)
	}

	var webhooks []map[string]interface{}
	if err := DecodeJSON(resp, &webhooks); err != nil {
		// Try decoding as paginated response
		var paginatedResp struct {
			Items []map[string]interface{} `json:"items"`
		}
		resp2, _ := s.client.Get(fmt.Sprintf("/api/v1/projects/%s/webhooks", projectID))
		if err2 := DecodeJSON(resp2, &paginatedResp); err2 == nil {
			return paginatedResp.Items, nil
		}
		return nil, err
	}
	return webhooks, nil
}

// DeleteWebhook deletes a webhook by ID.
func (s *Seeder) DeleteWebhook(projectID, webhookID string) error {
	resp, err := s.client.Delete(fmt.Sprintf("/api/v1/projects/%s/webhooks/%s", projectID, webhookID))
	if err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := ReadBody(resp)
		return fmt.Errorf("failed to delete webhook: status %d: %s", resp.StatusCode, body)
	}

	return nil
}

// GenerateWebhookSignature generates a real HMAC signature for webhook payloads.
// Supports GitHub (sha256), GitLab (token), and Bitbucket (sha256) providers.
func GenerateWebhookSignature(payload []byte, secret, provider string) string {
	switch provider {
	case "github":
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		return "sha256=" + hex.EncodeToString(mac.Sum(nil))
	case "gitlab":
		// GitLab uses X-Gitlab-Token header with the secret directly
		return secret
	case "bitbucket":
		// Bitbucket uses HMAC-SHA256
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		return hex.EncodeToString(mac.Sum(nil))
	default:
		// Default to GitHub style
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		return "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}
}

// SendWebhookEvent sends a properly signed webhook event to a URL.
func SendWebhookEvent(url, provider string, payload map[string]interface{}, secret string) (*http.Response, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Set provider-specific headers
	switch provider {
	case "github":
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-GitHub-Delivery", fmt.Sprintf("test-%d", time.Now().UnixNano()))
		req.Header.Set("X-Hub-Signature-256", GenerateWebhookSignature(payloadBytes, secret, "github"))
	case "gitlab":
		req.Header.Set("X-Gitlab-Event", "Push Hook")
		req.Header.Set("X-Gitlab-Token", secret)
	case "bitbucket":
		req.Header.Set("X-Event-Key", "repo:push")
		req.Header.Set("X-Hub-Signature", GenerateWebhookSignature(payloadBytes, secret, "bitbucket"))
	}

	return http.DefaultClient.Do(req)
}

// GitHubPushPayload creates a standard GitHub push webhook payload.
func GitHubPushPayload(repo, branch, commitSHA, commitMessage string) map[string]interface{} {
	return map[string]interface{}{
		"ref": "refs/heads/" + branch,
		"repository": map[string]interface{}{
			"full_name": repo,
			"clone_url": "https://github.com/" + repo + ".git",
		},
		"head_commit": map[string]interface{}{
			"id":      commitSHA,
			"message": commitMessage,
		},
		"pusher": map[string]interface{}{
			"name":  "test-user",
			"email": "test@example.com",
		},
	}
}

// GitLabPushPayload creates a standard GitLab push webhook payload.
func GitLabPushPayload(repo, branch, commitSHA, commitMessage string) map[string]interface{} {
	return map[string]interface{}{
		"object_kind": "push",
		"ref":         "refs/heads/" + branch,
		"project": map[string]interface{}{
			"path_with_namespace": repo,
		},
		"commits": []map[string]interface{}{
			{
				"id":      commitSHA,
				"message": commitMessage,
			},
		},
	}
}

// BitbucketPushPayload creates a standard Bitbucket push webhook payload.
func BitbucketPushPayload(repo, branch, commitSHA string) map[string]interface{} {
	return map[string]interface{}{
		"push": map[string]interface{}{
			"changes": []map[string]interface{}{
				{
					"new": map[string]interface{}{
						"name": branch,
						"target": map[string]interface{}{
							"hash": commitSHA,
						},
					},
				},
			},
		},
		"repository": map[string]interface{}{
			"full_name": repo,
		},
	}
}

// ========================================
// Agent Helpers (Step 3)
// ========================================

// GetFirstAgent retrieves the first available agent from the list.
// This is useful for tests that need a real agent to interact with.
func (s *Seeder) GetFirstAgent() (map[string]interface{}, error) {
	resp, err := s.client.Get("/api/v1/agents")
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to list agents: status %d: %s", resp.StatusCode, body)
	}

	result, err := DecodePaginatedJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to decode agents response: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("no agents available")
	}

	return result.Items[0], nil
}

// GetAgent retrieves an agent by ID.
func (s *Seeder) GetAgent(agentID string) (map[string]interface{}, error) {
	resp, err := s.client.Get(fmt.Sprintf("/api/v1/agents/%s", agentID))
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to get agent: status %d: %s", resp.StatusCode, body)
	}

	var agent map[string]interface{}
	if err := DecodeJSON(resp, &agent); err != nil {
		return nil, err
	}
	return agent, nil
}

// DeleteAgent deletes an agent by ID.
func (s *Seeder) DeleteAgent(agentID string) error {
	resp, err := s.client.Delete(fmt.Sprintf("/api/v1/agents/%s", agentID))
	if err != nil {
		return fmt.Errorf("failed to delete agent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := ReadBody(resp)
		return fmt.Errorf("failed to delete agent: status %d: %s", resp.StatusCode, body)
	}

	return nil
}

// GenerateAgentToken generates a registration token for an agent.
func (s *Seeder) GenerateAgentToken(agentID string) (string, error) {
	resp, err := s.client.Post(fmt.Sprintf("/api/v1/agents/%s/token", agentID), nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate agent token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := ReadBody(resp)
		return "", fmt.Errorf("failed to generate agent token: status %d: %s", resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := DecodeJSON(resp, &result); err != nil {
		return "", err
	}

	if token, ok := result["token"].(string); ok {
		return token, nil
	}

	return "", fmt.Errorf("token not found in response")
}

// UpdateAgentLabels updates the labels for an agent.
func (s *Seeder) UpdateAgentLabels(agentID string, labels map[string]string) error {
	update := map[string]interface{}{
		"labels": labels,
	}

	resp, err := s.client.Put(fmt.Sprintf("/api/v1/agents/%s", agentID), update)
	if err != nil {
		return fmt.Errorf("failed to update agent labels: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := ReadBody(resp)
		return fmt.Errorf("failed to update agent labels: status %d: %s", resp.StatusCode, body)
	}

	return nil
}

// UpdateAgentStatus updates the status of an agent (e.g., "active", "maintenance").
func (s *Seeder) UpdateAgentStatus(agentID, status string) error {
	update := map[string]interface{}{
		"status": status,
	}

	resp, err := s.client.Put(fmt.Sprintf("/api/v1/agents/%s", agentID), update)
	if err != nil {
		return fmt.Errorf("failed to update agent status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := ReadBody(resp)
		return fmt.Errorf("failed to update agent status: status %d: %s", resp.StatusCode, body)
	}

	return nil
}

// ListAgents returns all agents.
func (s *Seeder) ListAgents() ([]map[string]interface{}, error) {
	resp, err := s.client.Get("/api/v1/agents")
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ReadBody(resp)
		return nil, fmt.Errorf("failed to list agents: status %d: %s", resp.StatusCode, body)
	}

	result, err := DecodePaginatedJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to decode agents response: %w", err)
	}

	return result.Items, nil
}
