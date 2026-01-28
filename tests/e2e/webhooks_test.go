//go:build e2e

package e2e

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestWebhookAvailability tests webhook endpoint availability.
func TestWebhookAvailability(t *testing.T) {
	ctx := setupTest(t)

	webhooks := []struct {
		name     string
		endpoint string
	}{
		{"GitHub", "/webhook/github/test-project"},
		{"GitLab", "/webhook/gitlab/test-project"},
		{"Bitbucket", "/webhook/bitbucket/test-project"},
	}

	for _, wh := range webhooks {
		t.Run(wh.name+" webhook exists", func(t *testing.T) {
			// Webhooks don't require auth, but need proper payload
			req, err := http.NewRequest("POST", ctx.Config.MasterHTTPURL+wh.endpoint, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			// Should not be 404 (endpoint exists, might return 400 due to missing payload)
			if resp.StatusCode == http.StatusNotFound {
				t.Errorf("webhook endpoint %s returned 404", wh.endpoint)
			}
		})
	}
}

// TestGitHubWebhook tests GitHub webhook payload handling.
func TestGitHubWebhook(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// First create a project to receive webhooks
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	project := map[string]interface{}{
		"name":           "webhook-test-project",
		"repository":     "https://github.com/test/webhook-repo.git",
		"branch":         "main",
		"deploy_path":    "/deploy/webhook-test",
		"webhook_secret": "test-webhook-secret",
	}

	resp, _ := ctx.Client.Post("/api/v1/projects", project)
	resp.Body.Close()

	t.Run("GitHub push event", func(t *testing.T) {
		payload := `{
			"ref": "refs/heads/main",
			"repository": {
				"full_name": "test/webhook-repo",
				"clone_url": "https://github.com/test/webhook-repo.git"
			},
			"head_commit": {
				"id": "abc123",
				"message": "Test commit"
			}
		}`

		req, err := http.NewRequest("POST",
			ctx.Config.MasterHTTPURL+"/webhook/github/webhook-test-project",
			strings.NewReader(payload))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		// Add GitHub headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-GitHub-Delivery", "test-delivery-123")

		// Calculate HMAC signature
		mac := hmac.New(sha256.New, []byte("test-webhook-secret"))
		mac.Write([]byte(payload))
		signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Hub-Signature-256", signature)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should accept or reject based on config, not 404
		ctx.Assertions.NoServerError(resp)
	})

	t.Run("GitHub ping event", func(t *testing.T) {
		payload := `{
			"zen": "Mind your words, they are important.",
			"hook_id": 123456
		}`

		req, _ := http.NewRequest("POST",
			ctx.Config.MasterHTTPURL+"/webhook/github/webhook-test-project",
			strings.NewReader(payload))

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "ping")

		resp, _ := http.DefaultClient.Do(req)
		defer resp.Body.Close()

		ctx.Assertions.NoServerError(resp)
	})

	t.Run("invalid signature", func(t *testing.T) {
		payload := `{"ref": "refs/heads/main"}`

		req, _ := http.NewRequest("POST",
			ctx.Config.MasterHTTPURL+"/webhook/github/webhook-test-project",
			strings.NewReader(payload))

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-Hub-Signature-256", "sha256=invalidsignature")

		resp, _ := http.DefaultClient.Do(req)
		defer resp.Body.Close()

		// Should reject invalid signature
		if resp.StatusCode == http.StatusOK {
			t.Error("should reject invalid signature")
		}
	})

	t.Cleanup(func() {
		ctx.Cleanup.DeleteProject("webhook-test-project")
	})
}

// TestGitLabWebhook tests GitLab webhook payload handling.
func TestGitLabWebhook(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Create a project
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	project := map[string]interface{}{
		"name":           "gitlab-webhook-project",
		"repository":     "https://gitlab.com/test/repo.git",
		"branch":         "main",
		"deploy_path":    "/deploy/gitlab-test",
		"webhook_secret": "gitlab-secret-token",
	}

	resp, _ := ctx.Client.Post("/api/v1/projects", project)
	resp.Body.Close()

	t.Run("GitLab push event", func(t *testing.T) {
		payload := `{
			"object_kind": "push",
			"ref": "refs/heads/main",
			"project": {
				"path_with_namespace": "test/repo"
			},
			"commits": [{
				"id": "abc123",
				"message": "Test commit"
			}]
		}`

		req, _ := http.NewRequest("POST",
			ctx.Config.MasterHTTPURL+"/webhook/gitlab/gitlab-webhook-project",
			strings.NewReader(payload))

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Gitlab-Event", "Push Hook")
		req.Header.Set("X-Gitlab-Token", "gitlab-secret-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.NoServerError(resp)
	})

	t.Cleanup(func() {
		ctx.Cleanup.DeleteProject("gitlab-webhook-project")
	})
}

// TestBitbucketWebhook tests Bitbucket webhook payload handling.
func TestBitbucketWebhook(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Create a project
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	project := map[string]interface{}{
		"name":        "bitbucket-webhook-project",
		"repository":  "https://bitbucket.org/test/repo.git",
		"branch":      "main",
		"deploy_path": "/deploy/bitbucket-test",
	}

	resp, _ := ctx.Client.Post("/api/v1/projects", project)
	resp.Body.Close()

	t.Run("Bitbucket push event", func(t *testing.T) {
		payload := `{
			"push": {
				"changes": [{
					"new": {
						"name": "main",
						"target": {
							"hash": "abc123"
						}
					}
				}]
			},
			"repository": {
				"full_name": "test/repo"
			}
		}`

		req, _ := http.NewRequest("POST",
			ctx.Config.MasterHTTPURL+"/webhook/bitbucket/bitbucket-webhook-project",
			strings.NewReader(payload))

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Event-Key", "repo:push")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.NoServerError(resp)
	})

	t.Cleanup(func() {
		ctx.Cleanup.DeleteProject("bitbucket-webhook-project")
	})
}

// TestWebhookMethodNotAllowed tests that webhooks reject non-POST methods.
func TestWebhookMethodNotAllowed(t *testing.T) {
	ctx := setupTest(t)

	methods := []string{"GET", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		t.Run(method+" request", func(t *testing.T) {
			req, _ := http.NewRequest(method,
				ctx.Config.MasterHTTPURL+"/webhook/github/test",
				nil)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			io.ReadAll(resp.Body) // Drain body

			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("expected 405, got %d", resp.StatusCode)
			}
		})
	}
}
