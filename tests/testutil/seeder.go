// Package testutil provides test data seeding for E2E, CLI, and integration tests.
package testutil

import (
	"fmt"
	"net/http"
)

// Seeder provides test data seeding capabilities.
type Seeder struct {
	client *HTTPClient
}

// NewSeeder creates a new test data seeder.
func NewSeeder(client *HTTPClient) *Seeder {
	return &Seeder{client: client}
}

// SeedUser creates a test user and returns the user data.
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
	AdminUser    string
	AdminPass    string
	ViewerUser   string
	ViewerPass   string
	OperatorUser string
	OperatorPass string

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
	OperatorUser:    "test-operator",
	OperatorPass:    "TestOperator123!",
	TestProject1:    "e2e-test-project-1",
	TestProject2:    "e2e-test-project-2",
	TestRepo:        "https://github.com/test/repo.git",
	TestBranch:      "main",
	TestPath:        "/deploy/test",
	TestSecretKey:   "E2E_TEST_SECRET",
	TestSecretValue: "test-secret-value",
}
