//go:build e2e

// Package e2e provides end-to-end tests for vcdeploy.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestConfig holds E2E test configuration.
type TestConfig struct {
	MasterHTTPURL string
	MasterGRPCURL string
	TargetSSHHost string
	TargetSSHPort string
	GitServerURL  string
	APIToken      string
}

func getTestConfig() *TestConfig {
	return &TestConfig{
		MasterHTTPURL: getEnvOrDefault("E2E_MASTER_HTTP_URL", "http://localhost:18080"),
		MasterGRPCURL: getEnvOrDefault("E2E_MASTER_GRPC_URL", "localhost:19090"),
		TargetSSHHost: getEnvOrDefault("E2E_TARGET_SSH_HOST", "localhost"),
		TargetSSHPort: getEnvOrDefault("E2E_TARGET_SSH_PORT", "12222"),
		GitServerURL:  getEnvOrDefault("E2E_GIT_SERVER_URL", "http://localhost:13000"),
		APIToken:      getEnvOrDefault("E2E_API_TOKEN", "test-api-token"),
	}
}

// newAuthenticatedRequest creates an HTTP request with authentication header.
func newAuthenticatedRequest(method, url string, body io.Reader, token string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// TestHealthEndpoint verifies the master health endpoint is accessible.
func TestHealthEndpoint(t *testing.T) {
	cfg := getTestConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Wait for master to be ready
	err := waitForEndpoint(ctx, cfg.MasterHTTPURL+"/api/v1/health", 30*time.Second)
	if err != nil {
		t.Skipf("Master not available: %v", err)
	}

	resp, err := http.Get(cfg.MasterHTTPURL + "/api/v1/health")
	if err != nil {
		t.Fatalf("failed to get health endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var health map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Errorf("failed to decode health response: %v", err)
	}

	if status, ok := health["status"].(string); !ok || status != "healthy" {
		t.Errorf("expected healthy status, got: %v", health)
	}
}

// TestAPIProjects tests the projects API endpoints.
func TestAPIProjects(t *testing.T) {
	cfg := getTestConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := waitForEndpoint(ctx, cfg.MasterHTTPURL+"/api/v1/health", 30*time.Second)
	if err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list projects", func(t *testing.T) {
		req, err := newAuthenticatedRequest("GET", cfg.MasterHTTPURL+"/api/v1/projects", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to list projects: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("expected status 200, got %d: %s", resp.StatusCode, string(body))
		}
	})

	t.Run("create project", func(t *testing.T) {
		project := map[string]interface{}{
			"name":        "e2e-test-project",
			"repository":  "https://github.com/example/test.git",
			"branch":      "main",
			"deploy_path": "/deploy/test",
		}

		body, _ := json.Marshal(project)
		req, err := newAuthenticatedRequest("POST", cfg.MasterHTTPURL+"/api/v1/projects", bytes.NewReader(body), cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to create project: %v", err)
		}
		defer resp.Body.Close()

		// 201 Created or 200 OK are acceptable
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Errorf("expected status 201 or 200, got %d: %s", resp.StatusCode, string(respBody))
		}
	})
}

// TestAPIAgents tests the agents API endpoints.
func TestAPIAgents(t *testing.T) {
	cfg := getTestConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := waitForEndpoint(ctx, cfg.MasterHTTPURL+"/api/v1/health", 30*time.Second)
	if err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list agents", func(t *testing.T) {
		req, err := newAuthenticatedRequest("GET", cfg.MasterHTTPURL+"/api/v1/agents", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to list agents: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("expected status 200, got %d: %s", resp.StatusCode, string(body))
		}
	})
}

// TestAPIDeployments tests the deployments API endpoints.
func TestAPIDeployments(t *testing.T) {
	cfg := getTestConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := waitForEndpoint(ctx, cfg.MasterHTTPURL+"/api/v1/health", 30*time.Second)
	if err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list deployments", func(t *testing.T) {
		req, err := newAuthenticatedRequest("GET", cfg.MasterHTTPURL+"/api/v1/deployments", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to list deployments: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("expected status 200, got %d: %s", resp.StatusCode, string(body))
		}
	})
}

// TestWebhookEndpoints tests the webhook endpoints.
func TestWebhookEndpoints(t *testing.T) {
	cfg := getTestConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := waitForEndpoint(ctx, cfg.MasterHTTPURL+"/api/v1/health", 30*time.Second)
	if err != nil {
		t.Skipf("Master not available: %v", err)
	}

	tests := []struct {
		name     string
		endpoint string
		method   string
	}{
		{"github webhook exists", "/webhook/github/test", "POST"},
		{"gitlab webhook exists", "/webhook/gitlab/test", "POST"},
		{"bitbucket webhook exists", "/webhook/bitbucket/test", "POST"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, cfg.MasterHTTPURL+tt.endpoint, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("failed to call webhook: %v", err)
			}
			defer resp.Body.Close()

			// Should not be 404 (endpoint exists, might return 400 or 401 due to missing auth)
			if resp.StatusCode == http.StatusNotFound {
				t.Errorf("webhook endpoint %s returned 404", tt.endpoint)
			}
		})
	}
}

// waitForEndpoint waits until an HTTP endpoint becomes available.
func waitForEndpoint(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled waiting for endpoint %s", url)
		default:
			resp, err := http.Get(url)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	return fmt.Errorf("timeout waiting for endpoint %s", url)
}
