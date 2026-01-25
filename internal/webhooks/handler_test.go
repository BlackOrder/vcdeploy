package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

// MockSecretStore implements SecretStore for testing
type MockSecretStore struct {
	Secrets        map[string]string
	RequireSecrets map[string]bool
	Err            error
}

func (m *MockSecretStore) GetWebhookSecret(projectID string) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	return m.Secrets[projectID], nil
}

func (m *MockSecretStore) IsSecretRequired(projectID string) bool {
	if m.RequireSecrets == nil {
		return false
	}
	return m.RequireSecrets[projectID]
}

// MockEventProcessor implements EventProcessor for testing
type MockEventProcessor struct {
	PushEvents []*PushEvent
	PREvents   []*PullRequestEvent
	TagEvents  []*TagEvent
	PushErr    error
	PRErr      error
	TagErr     error
}

func (m *MockEventProcessor) ProcessPush(event *PushEvent) error {
	m.PushEvents = append(m.PushEvents, event)
	return m.PushErr
}

func (m *MockEventProcessor) ProcessPullRequest(event *PullRequestEvent) error {
	m.PREvents = append(m.PREvents, event)
	return m.PRErr
}

func (m *MockEventProcessor) ProcessTag(event *TagEvent) error {
	m.TagEvents = append(m.TagEvents, event)
	return m.TagErr
}

// Helper to create HMAC signature for GitHub
func createGitHubSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newTestHandler(secrets *MockSecretStore, processor *MockEventProcessor) *Handler {
	logger, _ := zap.NewDevelopment()
	return NewHandler(logger, secrets, processor)
}

func TestValidateGitHubSignatureMethod(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	secrets := &MockSecretStore{Secrets: map[string]string{}}
	processor := &MockEventProcessor{}
	h := NewHandler(logger, secrets, processor)

	secret := "test-webhook-secret"
	payload := []byte(`{"action": "push"}`)

	tests := []struct {
		name          string
		signature     string
		secret        string
		requireSecret bool
		valid         bool
	}{
		{
			name:      "valid signature",
			signature: createGitHubSignature(secret, payload),
			secret:    secret,
			valid:     true,
		},
		{
			name:      "invalid signature",
			signature: "sha256=invalid",
			secret:    secret,
			valid:     false,
		},
		{
			name:      "wrong secret",
			signature: createGitHubSignature("wrong-secret", payload),
			secret:    secret,
			valid:     false,
		},
		{
			name:      "missing sha256 prefix",
			signature: "invalid",
			secret:    secret,
			valid:     false,
		},
		{
			name:      "empty signature with empty secret",
			signature: "",
			secret:    "",
			valid:     true, // No secret configured means no validation
		},
		{
			name:          "empty secret but required",
			signature:     "",
			secret:        "",
			requireSecret: true,
			valid:         false, // Secret required but not configured
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := h.validateGitHubSignature(payload, tt.signature, tt.secret, tt.requireSecret)
			if result != tt.valid {
				t.Errorf("validateGitHubSignature() = %v, want %v", result, tt.valid)
			}
		})
	}
}

func TestHandleGitHubPush(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "test-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"ref":    "refs/heads/main",
		"before": "0000000000000000000000000000000000000000",
		"after":  "abc123def456",
		"forced": false,
		"repository": map[string]interface{}{
			"full_name": "test/repo",
			"clone_url": "https://github.com/test/repo.git",
		},
		"head_commit": map[string]interface{}{
			"id":        "abc123def456",
			"message":   "Test commit",
			"timestamp": "2024-01-01T12:00:00Z",
			"url":       "https://github.com/test/repo/commit/abc123",
			"author": map[string]interface{}{
				"name":  "Test User",
				"email": "test@example.com",
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	signature := createGitHubSignature("test-secret", payloadBytes)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleGitHub() status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	if len(processor.PushEvents) != 1 {
		t.Fatalf("Expected 1 push event, got %d", len(processor.PushEvents))
	}

	event := processor.PushEvents[0]
	if event.ProjectID != "test-project" {
		t.Errorf("PushEvent.ProjectID = %v, want test-project", event.ProjectID)
	}
	if event.Branch != "main" {
		t.Errorf("PushEvent.Branch = %v, want main", event.Branch)
	}
	if event.Commit != "abc123def456" {
		t.Errorf("PushEvent.Commit = %v, want abc123def456", event.Commit)
	}
}

func TestHandleGitHubPushInvalidSignature(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "test-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := []byte(`{"ref":"refs/heads/main"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleGitHub() with invalid signature status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	if len(processor.PushEvents) != 0 {
		t.Error("HandleGitHub() should not process event with invalid signature")
	}
}

func TestHandleGitHubPushInvalidPayload(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "test-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := []byte(`{invalid json`)
	signature := createGitHubSignature("test-secret", payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleGitHub() with invalid JSON status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleGitHubPushProcessorError(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "test-secret",
		},
	}
	processor := &MockEventProcessor{
		PushErr: http.ErrHandlerTimeout,
	}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"ref":   "refs/heads/main",
		"after": "abc123",
		"repository": map[string]interface{}{
			"full_name": "test/repo",
			"clone_url": "https://github.com/test/repo.git",
		},
		"head_commit": map[string]interface{}{
			"id":      "abc123",
			"message": "Test",
			"author":  map[string]interface{}{"name": "Test", "email": "test@test.com"},
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	signature := createGitHubSignature("test-secret", payloadBytes)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("HandleGitHub() push processor error status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleGitHubPushTagRef(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "test-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"ref":   "refs/tags/v1.0.0",  // Tag push should be ignored
		"after": "abc123",
		"repository": map[string]interface{}{
			"full_name": "test/repo",
			"clone_url": "https://github.com/test/repo.git",
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	signature := createGitHubSignature("test-secret", payloadBytes)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleGitHub() tag ref status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Should not process as push event
	if len(processor.PushEvents) != 0 {
		t.Error("HandleGitHub() should not process tag ref as push event")
	}
}

func TestHandleGitHubPRInvalidPayload(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "test-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := []byte(`{invalid json`)
	signature := createGitHubSignature("test-secret", payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleGitHub() PR invalid JSON status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleGitHubPRProcessorError(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "test-secret",
		},
	}
	processor := &MockEventProcessor{
		PRErr: http.ErrHandlerTimeout,
	}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"action": "opened",
		"number": 123,
		"pull_request": map[string]interface{}{
			"title": "Test PR",
			"user":  map[string]interface{}{"login": "user"},
			"head":  map[string]interface{}{"ref": "feature"},
			"base":  map[string]interface{}{"ref": "main"},
		},
		"repository": map[string]interface{}{"full_name": "test/repo"},
	}

	payloadBytes, _ := json.Marshal(payload)
	signature := createGitHubSignature("test-secret", payloadBytes)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("HandleGitHub() PR processor error status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleGitHubTagInvalidPayload(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "test-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := []byte(`{invalid json`)
	signature := createGitHubSignature("test-secret", payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "create")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleGitHub() tag invalid JSON status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleGitHubTagProcessorError(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "test-secret",
		},
	}
	processor := &MockEventProcessor{
		TagErr: http.ErrHandlerTimeout,
	}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"ref_type": "tag",
		"ref":      "v1.0.0",
		"sender":   map[string]interface{}{"login": "user"},
		"repository": map[string]interface{}{"full_name": "test/repo"},
	}

	payloadBytes, _ := json.Marshal(payload)
	signature := createGitHubSignature("test-secret", payloadBytes)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "create")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("HandleGitHub() tag processor error status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleGitHubTagNotTag(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "test-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"ref_type": "branch",  // Not a tag
		"ref":      "feature",
		"sender":   map[string]interface{}{"login": "user"},
		"repository": map[string]interface{}{"full_name": "test/repo"},
	}

	payloadBytes, _ := json.Marshal(payload)
	signature := createGitHubSignature("test-secret", payloadBytes)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "create")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleGitHub() branch create status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Should not process as tag event
	if len(processor.TagEvents) != 0 {
		t.Error("HandleGitHub() should not process branch create as tag event")
	}
}

func TestHandleGitHubMissingProjectID(t *testing.T) {
	handler := newTestHandler(&MockSecretStore{}, &MockEventProcessor{})

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/", nil)
	req.Header.Set("X-GitHub-Event", "push")

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleGitHub() without project ID status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleGitHubPing(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "",
		},
	}
	handler := newTestHandler(secrets, &MockEventProcessor{})

	payload := []byte(`{"zen":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "ping")

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleGitHub() ping status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	if body != "pong" {
		t.Errorf("HandleGitHub() ping body = %v, want 'pong'", body)
	}
}

func TestHandleGitHubPullRequest(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "test-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"action": "opened",
		"number": 123,
		"pull_request": map[string]interface{}{
			"title": "Test PR",
			"user": map[string]interface{}{
				"login": "testuser",
			},
			"head": map[string]interface{}{
				"ref": "feature-branch",
			},
			"base": map[string]interface{}{
				"ref": "main",
			},
		},
		"repository": map[string]interface{}{
			"full_name": "test/repo",
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	signature := createGitHubSignature("test-secret", payloadBytes)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleGitHub() PR status = %d, want %d", rr.Code, http.StatusOK)
	}

	if len(processor.PREvents) != 1 {
		t.Fatalf("Expected 1 PR event, got %d", len(processor.PREvents))
	}

	event := processor.PREvents[0]
	if event.Number != 123 {
		t.Errorf("PREvent.Number = %d, want 123", event.Number)
	}
	if event.SourceBranch != "feature-branch" {
		t.Errorf("PREvent.SourceBranch = %v, want feature-branch", event.SourceBranch)
	}
}

func TestHandleGitHubTagCreate(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"test-project": "test-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"ref_type": "tag",
		"ref":      "v1.0.0",
		"sender": map[string]interface{}{
			"login": "testuser",
		},
		"repository": map[string]interface{}{
			"full_name": "test/repo",
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	signature := createGitHubSignature("test-secret", payloadBytes)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github/test-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "create")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handler.HandleGitHub(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleGitHub() tag create status = %d, want %d", rr.Code, http.StatusOK)
	}

	if len(processor.TagEvents) != 1 {
		t.Fatalf("Expected 1 tag event, got %d", len(processor.TagEvents))
	}

	event := processor.TagEvents[0]
	if event.Tag != "v1.0.0" {
		t.Errorf("TagEvent.Tag = %v, want v1.0.0", event.Tag)
	}
	if event.Deleted {
		t.Error("TagEvent.Deleted should be false for create event")
	}
}

func TestHandleGitLabPush(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"gitlab-project": "gitlab-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"ref":    "refs/heads/main",
		"before": "0000000000000000000000000000000000000000",
		"after":  "gitlab-commit-123",
		"project": map[string]interface{}{
			"path_with_namespace": "test/repo",
			"git_http_url":        "https://gitlab.com/test/repo.git",
		},
		"commits": []map[string]interface{}{
			{
				"id":        "gitlab-commit-123",
				"message":   "GitLab commit",
				"timestamp": "2024-01-01T12:00:00Z",
				"url":       "https://gitlab.com/test/repo/-/commit/gitlab-commit-123",
				"author": map[string]interface{}{
					"name":  "Test User",
					"email": "test@example.com",
				},
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/gitlab-project", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitlab-Event", "Push Hook")
	req.Header.Set("X-Gitlab-Token", "gitlab-secret")

	rr := httptest.NewRecorder()
	handler.HandleGitLab(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleGitLab() push status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	if len(processor.PushEvents) != 1 {
		t.Fatalf("Expected 1 push event, got %d", len(processor.PushEvents))
	}

	event := processor.PushEvents[0]
	if event.Provider != "gitlab" {
		t.Errorf("PushEvent.Provider = %v, want gitlab", event.Provider)
	}
}

func TestHandleGitLabInvalidToken(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"gitlab-project": "gitlab-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := []byte(`{"ref":"refs/heads/main"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/gitlab-project", bytes.NewReader(payload))
	req.Header.Set("X-Gitlab-Event", "Push Hook")
	req.Header.Set("X-Gitlab-Token", "wrong-token")

	rr := httptest.NewRecorder()
	handler.HandleGitLab(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleGitLab() with wrong token status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleGitLabPushInvalidPayload(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"gitlab-project": "gitlab-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := []byte(`{invalid json`)

	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/gitlab-project", bytes.NewReader(payload))
	req.Header.Set("X-Gitlab-Event", "Push Hook")
	req.Header.Set("X-Gitlab-Token", "gitlab-secret")

	rr := httptest.NewRecorder()
	handler.HandleGitLab(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleGitLab() push invalid JSON status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleGitLabPushProcessorError(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"gitlab-project": "gitlab-secret",
		},
	}
	processor := &MockEventProcessor{
		PushErr: http.ErrHandlerTimeout,
	}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"ref":    "refs/heads/main",
		"before": "0000000000000000000000000000000000000000",
		"after":  "gitlab-commit-123",
		"project": map[string]interface{}{
			"path_with_namespace": "test/repo",
			"git_http_url":        "https://gitlab.com/test/repo.git",
		},
		"commits": []map[string]interface{}{
			{
				"id":      "gitlab-commit-123",
				"message": "Test",
				"author":  map[string]interface{}{"name": "Test", "email": "test@test.com"},
			},
		},
	}

	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/gitlab-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-Gitlab-Event", "Push Hook")
	req.Header.Set("X-Gitlab-Token", "gitlab-secret")

	rr := httptest.NewRecorder()
	handler.HandleGitLab(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("HandleGitLab() push processor error status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleBitbucketPush(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bb-project": "",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"push": map[string]interface{}{
			"changes": []map[string]interface{}{
				{
					"new": map[string]interface{}{
						"type": "branch",
						"name": "main",
						"target": map[string]interface{}{
							"hash":    "bb-commit-123",
							"message": "Bitbucket commit",
							"date":    "2024-01-01T12:00:00Z",
							"author": map[string]interface{}{
								"raw": "Test User <test@example.com>",
							},
						},
					},
				},
			},
		},
		"repository": map[string]interface{}{
			"full_name": "test/repo",
			"links": map[string]interface{}{
				"html": map[string]interface{}{
					"href": "https://bitbucket.org/test/repo",
				},
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/bb-project", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-Key", "repo:push")

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleBitbucket() push status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleBitbucketPushInvalidPayload(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bb-project": "",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := []byte(`{invalid json`)

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/bb-project", bytes.NewReader(payload))
	req.Header.Set("X-Event-Key", "repo:push")

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleBitbucket() push invalid JSON status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleBitbucketPushProcessorError(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bb-project": "",
		},
	}
	processor := &MockEventProcessor{
		PushErr: http.ErrHandlerTimeout,
	}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"push": map[string]interface{}{
			"changes": []map[string]interface{}{
				{
					"new": map[string]interface{}{
						"type": "branch",
						"name": "main",
						"target": map[string]interface{}{
							"hash":    "bb-commit-123",
							"message": "Test",
							"date":    "2024-01-01T12:00:00Z",
							"author":  map[string]interface{}{"user": map[string]interface{}{"display_name": "Test"}},
						},
					},
				},
			},
		},
		"repository": map[string]interface{}{
			"full_name": "test/repo",
			"links": map[string]interface{}{
				"html": map[string]interface{}{"href": "https://bitbucket.org/test/repo"},
			},
		},
	}

	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/bb-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-Event-Key", "repo:push")

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("HandleBitbucket() push processor error status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleBitbucketPushTagChange(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bb-project": "",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"push": map[string]interface{}{
			"changes": []map[string]interface{}{
				{
					"new": map[string]interface{}{
						"type": "tag",  // Tag change - should be skipped
						"name": "v1.0.0",
						"target": map[string]interface{}{
							"hash":    "bb-commit-123",
							"message": "Test",
							"date":    "2024-01-01T12:00:00Z",
						},
					},
				},
			},
		},
		"repository": map[string]interface{}{
			"full_name": "test/repo",
			"links": map[string]interface{}{
				"html": map[string]interface{}{"href": "https://bitbucket.org/test/repo"},
			},
		},
	}

	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/bb-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-Event-Key", "repo:push")

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleBitbucket() tag change status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Should not process tag changes as push events
	if len(processor.PushEvents) != 0 {
		t.Error("HandleBitbucket() should not process tag change as push event")
	}
}

func TestHandleBitbucketMissingProjectID(t *testing.T) {
	handler := newTestHandler(&MockSecretStore{}, &MockEventProcessor{})

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/", nil)
	req.Header.Set("X-Event-Key", "repo:push")

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleBitbucket() without project ID status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleBitbucketMissingEventKey(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bb-project": "",
		},
	}
	handler := newTestHandler(secrets, &MockEventProcessor{})

	payload := []byte(`{"test": "data"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/bb-project", bytes.NewReader(payload))
	// No X-Event-Key header

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleBitbucket() without event key status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleBitbucketUnknownEventKey(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bb-project": "",
		},
	}
	handler := newTestHandler(secrets, &MockEventProcessor{})

	payload := []byte(`{"test": "data"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/bb-project", bytes.NewReader(payload))
	req.Header.Set("X-Event-Key", "unknown:event")

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleBitbucket() unknown event status = %d, want %d (should ignore)", rr.Code, http.StatusOK)
	}
}

func TestHandleGitLabMergeRequest(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"gitlab-project": "gitlab-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"object_attributes": map[string]interface{}{
			"action":        "open",
			"iid":           42,
			"title":         "Test MR",
			"source_branch": "feature-branch",
			"target_branch": "main",
		},
		"user": map[string]interface{}{
			"username": "testuser",
		},
		"project": map[string]interface{}{
			"path_with_namespace": "test/repo",
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/gitlab-project", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	req.Header.Set("X-Gitlab-Token", "gitlab-secret")

	rr := httptest.NewRecorder()
	handler.HandleGitLab(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleGitLab() MR status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	if len(processor.PREvents) != 1 {
		t.Fatalf("Expected 1 PR event, got %d", len(processor.PREvents))
	}

	event := processor.PREvents[0]
	if event.Provider != "gitlab" {
		t.Errorf("PREvent.Provider = %v, want gitlab", event.Provider)
	}
	if event.Number != 42 {
		t.Errorf("PREvent.Number = %d, want 42", event.Number)
	}
	if event.SourceBranch != "feature-branch" {
		t.Errorf("PREvent.SourceBranch = %v, want feature-branch", event.SourceBranch)
	}
	if event.TargetBranch != "main" {
		t.Errorf("PREvent.TargetBranch = %v, want main", event.TargetBranch)
	}
}

func TestHandleGitLabMergeRequestInvalidPayload(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"gitlab-project": "gitlab-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	// Invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/gitlab-project", bytes.NewReader([]byte(`{invalid`)))
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	req.Header.Set("X-Gitlab-Token", "gitlab-secret")

	rr := httptest.NewRecorder()
	handler.HandleGitLab(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleGitLab() invalid MR payload status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleGitLabMergeRequestProcessorError(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"gitlab-project": "gitlab-secret",
		},
	}
	processor := &MockEventProcessor{
		PRErr: http.ErrHandlerTimeout, // Simulate processor error
	}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"object_attributes": map[string]interface{}{
			"action":        "open",
			"iid":           42,
			"title":         "Test MR",
			"source_branch": "feature-branch",
			"target_branch": "main",
		},
		"user": map[string]interface{}{
			"username": "testuser",
		},
		"project": map[string]interface{}{
			"path_with_namespace": "test/repo",
		},
	}

	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/gitlab-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	req.Header.Set("X-Gitlab-Token", "gitlab-secret")

	rr := httptest.NewRecorder()
	handler.HandleGitLab(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("HandleGitLab() MR processor error status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleGitLabTagPush(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"gitlab-project": "gitlab-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"ref":       "refs/tags/v1.0.0",
		"before":    "0000000000000000000000000000000000000000",
		"after":     "abc123def456",
		"user_name": "testuser",
		"project": map[string]interface{}{
			"path_with_namespace": "test/repo",
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/gitlab-project", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitlab-Event", "Tag Push Hook")
	req.Header.Set("X-Gitlab-Token", "gitlab-secret")

	rr := httptest.NewRecorder()
	handler.HandleGitLab(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleGitLab() tag push status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	if len(processor.TagEvents) != 1 {
		t.Fatalf("Expected 1 tag event, got %d", len(processor.TagEvents))
	}

	event := processor.TagEvents[0]
	if event.Provider != "gitlab" {
		t.Errorf("TagEvent.Provider = %v, want gitlab", event.Provider)
	}
	if event.Tag != "v1.0.0" {
		t.Errorf("TagEvent.Tag = %v, want v1.0.0", event.Tag)
	}
	if event.Deleted {
		t.Error("TagEvent.Deleted should be false for create")
	}
}

func TestHandleGitLabTagDelete(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"gitlab-project": "gitlab-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"ref":       "refs/tags/v1.0.0",
		"before":    "abc123def456",
		"after":     "0000000000000000000000000000000000000000", // Zero hash = deleted
		"user_name": "testuser",
		"project": map[string]interface{}{
			"path_with_namespace": "test/repo",
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/gitlab-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-Gitlab-Event", "Tag Push Hook")
	req.Header.Set("X-Gitlab-Token", "gitlab-secret")

	rr := httptest.NewRecorder()
	handler.HandleGitLab(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleGitLab() tag delete status = %d, want %d", rr.Code, http.StatusOK)
	}

	if len(processor.TagEvents) != 1 {
		t.Fatalf("Expected 1 tag event, got %d", len(processor.TagEvents))
	}

	event := processor.TagEvents[0]
	if !event.Deleted {
		t.Error("TagEvent.Deleted should be true for tag deletion")
	}
}

func TestHandleGitLabTagInvalidPayload(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"gitlab-project": "gitlab-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/gitlab-project", bytes.NewReader([]byte(`{invalid`)))
	req.Header.Set("X-Gitlab-Event", "Tag Push Hook")
	req.Header.Set("X-Gitlab-Token", "gitlab-secret")

	rr := httptest.NewRecorder()
	handler.HandleGitLab(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleGitLab() invalid tag payload status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleGitLabTagProcessorError(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"gitlab-project": "gitlab-secret",
		},
	}
	processor := &MockEventProcessor{
		TagErr: http.ErrHandlerTimeout,
	}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"ref":       "refs/tags/v1.0.0",
		"before":    "0000000000000000000000000000000000000000",
		"after":     "abc123def456",
		"user_name": "testuser",
		"project": map[string]interface{}{
			"path_with_namespace": "test/repo",
		},
	}

	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/gitlab-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-Gitlab-Event", "Tag Push Hook")
	req.Header.Set("X-Gitlab-Token", "gitlab-secret")

	rr := httptest.NewRecorder()
	handler.HandleGitLab(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("HandleGitLab() tag processor error status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleBitbucketPullRequestCreated(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bb-project": "",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"pullrequest": map[string]interface{}{
			"id":    123,
			"title": "Test PR",
			"source": map[string]interface{}{
				"branch": map[string]interface{}{
					"name": "feature-branch",
				},
			},
			"destination": map[string]interface{}{
				"branch": map[string]interface{}{
					"name": "main",
				},
			},
			"author": map[string]interface{}{
				"display_name": "Test User",
			},
		},
		"repository": map[string]interface{}{
			"full_name": "test/repo",
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/bb-project", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-Key", "pullrequest:created")

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleBitbucket() PR status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	if len(processor.PREvents) != 1 {
		t.Fatalf("Expected 1 PR event, got %d", len(processor.PREvents))
	}

	event := processor.PREvents[0]
	if event.Provider != "bitbucket" {
		t.Errorf("PREvent.Provider = %v, want bitbucket", event.Provider)
	}
	if event.Action != "opened" {
		t.Errorf("PREvent.Action = %v, want opened", event.Action)
	}
	if event.Number != 123 {
		t.Errorf("PREvent.Number = %d, want 123", event.Number)
	}
}

func TestHandleBitbucketPullRequestUpdated(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bb-project": "",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"pullrequest": map[string]interface{}{
			"id":    123,
			"title": "Test PR",
			"source": map[string]interface{}{
				"branch": map[string]interface{}{
					"name": "feature-branch",
				},
			},
			"destination": map[string]interface{}{
				"branch": map[string]interface{}{
					"name": "main",
				},
			},
			"author": map[string]interface{}{
				"display_name": "Test User",
			},
		},
		"repository": map[string]interface{}{
			"full_name": "test/repo",
		},
	}

	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/bb-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-Event-Key", "pullrequest:updated")

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleBitbucket() PR updated status = %d, want %d", rr.Code, http.StatusOK)
	}

	if len(processor.PREvents) != 1 {
		t.Fatalf("Expected 1 PR event, got %d", len(processor.PREvents))
	}

	if processor.PREvents[0].Action != "synchronize" {
		t.Errorf("PREvent.Action = %v, want synchronize", processor.PREvents[0].Action)
	}
}

func TestHandleBitbucketPullRequestFulfilled(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bb-project": "",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"pullrequest": map[string]interface{}{
			"id":    123,
			"title": "Test PR",
			"source": map[string]interface{}{
				"branch": map[string]interface{}{
					"name": "feature-branch",
				},
			},
			"destination": map[string]interface{}{
				"branch": map[string]interface{}{
					"name": "main",
				},
			},
			"author": map[string]interface{}{
				"display_name": "Test User",
			},
		},
		"repository": map[string]interface{}{
			"full_name": "test/repo",
		},
	}

	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/bb-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-Event-Key", "pullrequest:fulfilled")

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleBitbucket() PR fulfilled status = %d, want %d", rr.Code, http.StatusOK)
	}

	if processor.PREvents[0].Action != "merged" {
		t.Errorf("PREvent.Action = %v, want merged", processor.PREvents[0].Action)
	}
}

func TestHandleBitbucketPullRequestRejected(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bb-project": "",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"pullrequest": map[string]interface{}{
			"id":    123,
			"title": "Test PR",
			"source": map[string]interface{}{
				"branch": map[string]interface{}{
					"name": "feature-branch",
				},
			},
			"destination": map[string]interface{}{
				"branch": map[string]interface{}{
					"name": "main",
				},
			},
			"author": map[string]interface{}{
				"display_name": "Test User",
			},
		},
		"repository": map[string]interface{}{
			"full_name": "test/repo",
		},
	}

	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/bb-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-Event-Key", "pullrequest:rejected")

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HandleBitbucket() PR rejected status = %d, want %d", rr.Code, http.StatusOK)
	}

	if processor.PREvents[0].Action != "closed" {
		t.Errorf("PREvent.Action = %v, want closed", processor.PREvents[0].Action)
	}
}

func TestHandleBitbucketPRInvalidPayload(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bb-project": "",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/bb-project", bytes.NewReader([]byte(`{invalid`)))
	req.Header.Set("X-Event-Key", "pullrequest:created")

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleBitbucket() invalid PR payload status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleBitbucketPRProcessorError(t *testing.T) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bb-project": "",
		},
	}
	processor := &MockEventProcessor{
		PRErr: http.ErrHandlerTimeout,
	}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"pullrequest": map[string]interface{}{
			"id":    123,
			"title": "Test PR",
			"source": map[string]interface{}{
				"branch": map[string]interface{}{
					"name": "feature-branch",
				},
			},
			"destination": map[string]interface{}{
				"branch": map[string]interface{}{
					"name": "main",
				},
			},
			"author": map[string]interface{}{
				"display_name": "Test User",
			},
		},
		"repository": map[string]interface{}{
			"full_name": "test/repo",
		},
	}

	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/bb-project", bytes.NewReader(payloadBytes))
	req.Header.Set("X-Event-Key", "pullrequest:created")

	rr := httptest.NewRecorder()
	handler.HandleBitbucket(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("HandleBitbucket() PR processor error status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestExtractProjectID(t *testing.T) {
	tests := []struct {
		path   string
		prefix string
		want   string
	}{
		{"/webhook/github/my-project", "/webhook/github/", "my-project"},
		{"/webhook/gitlab/other-project", "/webhook/gitlab/", "other-project"},
		{"/webhook/github/", "/webhook/github/", ""},
		{"/webhook/github", "/webhook/github/", ""},
		// Note: extractProjectID only returns the first path segment after prefix
		// This is intentional as project IDs should not contain slashes
		{"/webhook/github/project/with/slashes", "/webhook/github/", "project"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractProjectID(tt.path, tt.prefix)
			if got != tt.want {
				t.Errorf("extractProjectID(%q, %q) = %q, want %q", tt.path, tt.prefix, got, tt.want)
			}
		})
	}
}

// Benchmarks
func BenchmarkValidateGitHubSignature(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	secrets := &MockSecretStore{}
	processor := &MockEventProcessor{}
	h := NewHandler(logger, secrets, processor)

	secret := "benchmark-secret"
	payload := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"test/repo"}}`)
	signature := createGitHubSignature(secret, payload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.validateGitHubSignature(payload, signature, secret, false)
	}
}

func BenchmarkHandleGitHub(b *testing.B) {
	secrets := &MockSecretStore{
		Secrets: map[string]string{
			"bench-project": "bench-secret",
		},
	}
	processor := &MockEventProcessor{}
	handler := newTestHandler(secrets, processor)

	payload := map[string]interface{}{
		"ref":   "refs/heads/main",
		"after": "bench123",
		"repository": map[string]interface{}{
			"full_name": "test/repo",
			"clone_url": "https://github.com/test/repo.git",
		},
		"head_commit": map[string]interface{}{
			"id":      "bench123",
			"message": "Benchmark",
			"author":  map[string]interface{}{"name": "Test", "email": "test@test.com"},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		b.Fatalf("failed to marshal payload: %v", err)
	}
	signature := createGitHubSignature("bench-secret", payloadBytes)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/webhook/github/bench-project", bytes.NewReader(payloadBytes))
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-Hub-Signature-256", signature)

		rr := httptest.NewRecorder()
		handler.HandleGitHub(rr, req)
	}
}
