// Package testutil provides test data seeding for E2E, CLI, and integration tests.
package testutil

import (
	"fmt"
	"net/http"
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
		return nil, nil
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
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
		"name":        name,
		"repository":  repo,
		"branch":      branch,
		"deploy_path": deployPath,
		"type":        projectType,
	}

	resp, err := s.client.Post("/api/v1/projects", project)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}
	defer resp.Body.Close()

	// Handle duplicate gracefully for idempotent seeding
	if resp.StatusCode == http.StatusConflict {
		return nil, nil
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
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
		return nil, nil
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
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
		return nil, nil
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
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
	resp.Body.Close()
	return nil
}

// DeleteProject deletes a project by ID.
func (c *Cleanup) DeleteProject(id interface{}) error {
	resp, err := c.client.Delete(fmt.Sprintf("/api/v1/projects/%v", id))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// DeleteSecret deletes a secret by ID.
func (c *Cleanup) DeleteSecret(id interface{}) error {
	resp, err := c.client.Delete(fmt.Sprintf("/api/v1/secrets/%v", id))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// DeleteAPIKey deletes an API key by ID.
func (c *Cleanup) DeleteAPIKey(id interface{}) error {
	resp, err := c.client.Delete(fmt.Sprintf("/api/v1/api-keys/%v", id))
	if err != nil {
		return err
	}
	resp.Body.Close()
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
