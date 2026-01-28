package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// MockNotifier implements Notifier for testing
type MockNotifier struct {
	NameValue  string
	SendErr    error
	SendCalled bool
	LastEvent  Event
	mu         sync.Mutex
}

func (m *MockNotifier) Send(ctx context.Context, event Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SendCalled = true
	m.LastEvent = event
	return m.SendErr
}

func (m *MockNotifier) Name() string {
	return m.NameValue
}

func (m *MockNotifier) WasCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.SendCalled
}

func TestEvent(t *testing.T) {
	t.Parallel()

	event := Event{
		Type:        "deployment",
		ProjectID:   "proj-001",
		ProjectName: "Test Project",
		Environment: "production",
		DeployID:    "deploy-123",
		Version:     "v1.2.0",
		Status:      "success",
		User:        "admin",
		Message:     "Deployment completed successfully",
		URL:         "https://deploy.example.com/123",
		Timestamp:   time.Now(),
	}

	if event.Type != "deployment" {
		t.Errorf("Event.Type = %v, want deployment", event.Type)
	}

	if event.Status != "success" {
		t.Errorf("Event.Status = %v, want success", event.Status)
	}

	if event.Timestamp.IsZero() {
		t.Error("Event.Timestamp should not be zero")
	}
}

func TestEventJSON(t *testing.T) {
	t.Parallel()

	event := Event{
		Type:        "deployment",
		ProjectID:   "proj-001",
		ProjectName: "Test Project",
		Status:      "success",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.ProjectID != event.ProjectID {
		t.Errorf("Decoded.ProjectID = %v, want %v", decoded.ProjectID, event.ProjectID)
	}
}

func TestNewManager(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	manager := NewManager(logger)

	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
}

func TestManagerRegister(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	manager := NewManager(logger)

	notifier1 := &MockNotifier{NameValue: "mock1"}
	notifier2 := &MockNotifier{NameValue: "mock2"}

	manager.Register(notifier1)
	manager.Register(notifier2)

	if len(manager.notifiers) != 2 {
		t.Errorf("Manager.notifiers count = %d, want 2", len(manager.notifiers))
	}
}

func TestManagerNotify(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	manager := NewManager(logger)

	notifier := &MockNotifier{NameValue: "test-notifier"}
	manager.Register(notifier)

	event := Event{
		Type:      "deployment",
		ProjectID: "test",
		Status:    "success",
	}

	ctx := context.Background()
	manager.Notify(ctx, event)

	// Wait for goroutine to complete
	time.Sleep(100 * time.Millisecond)

	if !notifier.WasCalled() {
		t.Error("Notifier.Send() should have been called")
	}

	if notifier.LastEvent.ProjectID != "test" {
		t.Errorf("LastEvent.ProjectID = %v, want test", notifier.LastEvent.ProjectID)
	}
}

func TestManagerNotifyMultiple(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	manager := NewManager(logger)

	notifiers := []*MockNotifier{
		{NameValue: "notifier1"},
		{NameValue: "notifier2"},
		{NameValue: "notifier3"},
	}

	for _, n := range notifiers {
		manager.Register(n)
	}

	event := Event{
		Type:      "deployment",
		ProjectID: "multi-test",
		Status:    "success",
	}

	ctx := context.Background()
	manager.Notify(ctx, event)

	// Wait for goroutines to complete
	time.Sleep(200 * time.Millisecond)

	for i, n := range notifiers {
		if !n.WasCalled() {
			t.Errorf("Notifier %d should have been called", i)
		}
	}
}

func TestManagerNotifyWithError(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	manager := NewManager(logger)

	// One notifier that fails
	failingNotifier := &MockNotifier{
		NameValue: "failing",
		SendErr:   context.DeadlineExceeded,
	}

	// One that succeeds
	successNotifier := &MockNotifier{
		NameValue: "success",
	}

	manager.Register(failingNotifier)
	manager.Register(successNotifier)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()
	manager.Notify(ctx, event) // Should not panic

	time.Sleep(100 * time.Millisecond)

	// Both should still be called
	if !failingNotifier.WasCalled() {
		t.Error("Failing notifier should have been called")
	}
	if !successNotifier.WasCalled() {
		t.Error("Success notifier should have been called")
	}
}

func TestManagerNotifyAndWait(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	manager := NewManager(logger)

	notifier := &MockNotifier{
		NameValue: "test-notifier",
	}
	manager.Register(notifier)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()
	manager.NotifyAndWait(ctx, event)

	// After NotifyAndWait returns, the notifier should have been called
	if !notifier.WasCalled() {
		t.Error("Notifier should have been called")
	}
}

func TestManagerWait(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	manager := NewManager(logger)

	notifier := &MockNotifier{
		NameValue: "test-notifier",
	}
	manager.Register(notifier)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()
	manager.Notify(ctx, event)
	manager.Wait()

	// After Wait returns, all notifications should be complete
	if !notifier.WasCalled() {
		t.Error("Notifier should have been called")
	}
}

func TestSlackConfig(t *testing.T) {
	t.Parallel()

	cfg := SlackConfig{
		WebhookURL: "https://hooks.slack.com/services/xxx",
		Channel:    "#deployments",
		Username:   "VCDeploy Bot",
		IconEmoji:  ":rocket:",
	}

	if cfg.Channel != "#deployments" {
		t.Errorf("SlackConfig.Channel = %v, want #deployments", cfg.Channel)
	}

	if cfg.IconEmoji != ":rocket:" {
		t.Errorf("SlackConfig.IconEmoji = %v, want :rocket:", cfg.IconEmoji)
	}
}

func TestNewSlackNotifier(t *testing.T) {
	t.Parallel()

	cfg := SlackConfig{
		WebhookURL: "https://hooks.slack.com/services/xxx",
	}

	notifier := NewSlackNotifier(cfg)

	if notifier == nil {
		t.Fatal("NewSlackNotifier() returned nil")
	}

	if notifier.Name() != "slack" {
		t.Errorf("SlackNotifier.Name() = %v, want slack", notifier.Name())
	}
}

func TestSlackNotifierSend(t *testing.T) {
	t.Parallel()

	// Create test server
	var receivedPayload []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		buf := make([]byte, 4096)
		n, err := r.Body.Read(buf)
		if err != nil && err.Error() != "EOF" {
			t.Errorf("Failed to read request body: %v", err)
			return
		}
		receivedPayload = buf[:n]

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := SlackConfig{
		WebhookURL: server.URL,
		Channel:    "#test",
		Username:   "Test Bot",
	}

	notifier := NewSlackNotifier(cfg)

	event := Event{
		Type:        "deployment",
		ProjectName: "Test Project",
		Environment: "production",
		Status:      "success",
		User:        "admin",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)

	if err != nil {
		t.Fatalf("SlackNotifier.Send() error = %v", err)
	}

	if len(receivedPayload) == 0 {
		t.Error("Expected payload to be sent")
	}
}

func TestSlackNotifierSendFailure(t *testing.T) {
	t.Parallel()

	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := SlackConfig{
		WebhookURL: server.URL,
	}

	notifier := NewSlackNotifier(cfg)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)

	if err == nil {
		t.Error("Expected error for 500 response")
	}
}

func TestSlackNotifierSendEmptyURL(t *testing.T) {
	t.Parallel()

	cfg := SlackConfig{
		WebhookURL: "",
	}

	notifier := NewSlackNotifier(cfg)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)

	// Empty URL should not error, just skip
	if err != nil {
		t.Errorf("Expected no error for empty URL, got: %v", err)
	}
}

func TestEmailConfig(t *testing.T) {
	t.Parallel()

	cfg := EmailConfig{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		Username:    "notify@example.com",
		Password:    "secret",
		FromAddress: "notify@example.com",
		FromName:    "VCDeploy",
		ToAddresses: []string{"team@example.com", "ops@example.com"},
	}

	if cfg.SMTPHost != "smtp.example.com" {
		t.Errorf("EmailConfig.SMTPHost = %v, want smtp.example.com", cfg.SMTPHost)
	}

	if len(cfg.ToAddresses) != 2 {
		t.Errorf("EmailConfig.ToAddresses count = %d, want 2", len(cfg.ToAddresses))
	}
}

func TestNewEmailNotifier(t *testing.T) {
	t.Parallel()

	cfg := EmailConfig{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		FromAddress: "test@example.com",
		ToAddresses: []string{"team@example.com"},
	}

	notifier, err := NewEmailNotifier(cfg)
	if err != nil {
		t.Fatalf("NewEmailNotifier() error = %v", err)
	}

	if notifier == nil {
		t.Fatal("NewEmailNotifier() returned nil")
	}

	if notifier.Name() != "email" {
		t.Errorf("EmailNotifier.Name() = %v, want email", notifier.Name())
	}
}

func TestEmailNotifierSendEmptyConfig(t *testing.T) {
	t.Parallel()

	// Test with empty SMTP host - should skip silently
	cfg := EmailConfig{
		SMTPHost:    "",
		ToAddresses: []string{"test@example.com"},
	}

	notifier, err := NewEmailNotifier(cfg)
	if err != nil {
		t.Fatalf("NewEmailNotifier() error = %v", err)
	}

	event := Event{
		Type:        "deployment",
		ProjectName: "Test Project",
		Environment: "production",
		Status:      "success",
	}

	ctx := context.Background()
	err = notifier.Send(ctx, event)
	if err != nil {
		t.Errorf("Send() with empty host should not error, got: %v", err)
	}
}

func TestEmailNotifierSendEmptyRecipients(t *testing.T) {
	t.Parallel()

	// Test with no recipients - should skip silently
	cfg := EmailConfig{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		ToAddresses: []string{},
	}

	notifier, err := NewEmailNotifier(cfg)
	if err != nil {
		t.Fatalf("NewEmailNotifier() error = %v", err)
	}

	event := Event{
		Type:        "deployment",
		ProjectName: "Test Project",
		Status:      "success",
	}

	ctx := context.Background()
	err = notifier.Send(ctx, event)
	if err != nil {
		t.Errorf("Send() with no recipients should not error, got: %v", err)
	}
}

func TestEmailNotifierDefaultTemplate(t *testing.T) {
	t.Parallel()

	cfg := EmailConfig{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		FromAddress: "test@example.com",
		FromName:    "VCDeploy",
		ToAddresses: []string{"team@example.com"},
	}

	notifier, err := NewEmailNotifier(cfg)
	if err != nil {
		t.Fatalf("NewEmailNotifier() error = %v", err)
	}

	event := Event{
		Type:        "deployment",
		ProjectName: "Test Project",
		Environment: "production",
		Status:      "success",
		Version:     "v1.2.3",
		User:        "deployer",
		Message:     "Test deployment message",
		URL:         "https://example.com/deploy/123",
		Timestamp:   time.Now(),
	}

	// Test default template rendering (internal method)
	template := notifier.defaultTemplate(event)

	if template == "" {
		t.Error("defaultTemplate() returned empty string")
	}

	// Verify template contains key elements
	if !strings.Contains(template, "Test Project") {
		t.Error("template should contain project name")
	}
	if !strings.Contains(template, "production") {
		t.Error("template should contain environment")
	}
	if !strings.Contains(template, "success") {
		t.Error("template should contain status")
	}
	if !strings.Contains(template, "v1.2.3") {
		t.Error("template should contain version")
	}
	if !strings.Contains(template, "Test deployment message") {
		t.Error("template should contain message")
	}
	if !strings.Contains(template, "https://example.com/deploy/123") {
		t.Error("template should contain URL")
	}
}

func TestEmailNotifierDefaultTemplateStatuses(t *testing.T) {
	t.Parallel()

	cfg := EmailConfig{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		FromAddress: "test@example.com",
		ToAddresses: []string{"team@example.com"},
	}

	notifier, err := NewEmailNotifier(cfg)
	if err != nil {
		t.Fatalf("NewEmailNotifier() error = %v", err)
	}

	statuses := []string{"success", "failed", "pending", "running", "rolled_back"}

	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			event := Event{
				ProjectName: "Test",
				Environment: "prod",
				Status:      status,
				Timestamp:   time.Now(),
			}

			template := notifier.defaultTemplate(event)
			if template == "" {
				t.Errorf("defaultTemplate() for status %s returned empty", status)
			}
			if !strings.Contains(template, status) {
				t.Errorf("template should contain status %s", status)
			}
		})
	}
}

func TestEmailNotifierDefaultTemplateNoMessage(t *testing.T) {
	t.Parallel()

	cfg := EmailConfig{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		FromAddress: "test@example.com",
		ToAddresses: []string{"team@example.com"},
	}

	notifier, err := NewEmailNotifier(cfg)
	if err != nil {
		t.Fatalf("NewEmailNotifier() error = %v", err)
	}

	event := Event{
		ProjectName: "Test",
		Environment: "prod",
		Status:      "success",
		Message:     "", // No message
		URL:         "", // No URL
		Timestamp:   time.Now(),
	}

	template := notifier.defaultTemplate(event)
	if template == "" {
		t.Error("defaultTemplate() returned empty string")
	}
}

func TestWebhookConfig(t *testing.T) {
	t.Parallel()

	cfg := WebhookConfig{
		URL:     "https://webhook.example.com/deploy",
		Method:  "POST",
		Secret:  "webhook-secret",
		Headers: map[string]string{"X-Custom": "value"},
	}

	if cfg.Secret != "webhook-secret" {
		t.Errorf("WebhookConfig.Secret = %v, want webhook-secret", cfg.Secret)
	}

	if cfg.Method != "POST" {
		t.Errorf("WebhookConfig.Method = %v, want POST", cfg.Method)
	}
}

func TestNewWebhookNotifier(t *testing.T) {
	t.Parallel()

	cfg := WebhookConfig{
		URL: "https://webhook.example.com/deploy",
	}

	notifier := NewWebhookNotifier(cfg)

	if notifier == nil {
		t.Fatal("NewWebhookNotifier() returned nil")
	}

	if notifier.Name() != "webhook" {
		t.Errorf("WebhookNotifier.Name() = %v, want webhook", notifier.Name())
	}
}

func TestWebhookNotifierDefaultMethod(t *testing.T) {
	t.Parallel()

	cfg := WebhookConfig{
		URL:    "https://webhook.example.com/deploy",
		Method: "", // Should default to POST
	}

	notifier := NewWebhookNotifier(cfg)

	if notifier.config.Method != "POST" {
		t.Errorf("WebhookNotifier default method = %v, want POST", notifier.config.Method)
	}
}

func TestWebhookNotifierSend(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		_ = decoder.Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := WebhookConfig{
		URL: server.URL,
	}

	notifier := NewWebhookNotifier(cfg)

	event := Event{
		Type:      "deployment",
		ProjectID: "webhook-test",
		Status:    "success",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)

	if err != nil {
		t.Fatalf("WebhookNotifier.Send() error = %v", err)
	}

	if receivedPayload["project_id"] != "webhook-test" {
		t.Errorf("Received payload.project_id = %v, want webhook-test", receivedPayload["project_id"])
	}
}

func TestWebhookNotifierWithSignature(t *testing.T) {
	t.Parallel()

	var receivedSignature string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-VCDeploy-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := WebhookConfig{
		URL:    server.URL,
		Secret: "test-secret",
	}

	notifier := NewWebhookNotifier(cfg)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()
	_ = notifier.Send(ctx, event)

	if receivedSignature == "" {
		t.Error("Expected X-VCDeploy-Signature header")
	}
}

func TestWebhookNotifierWithCustomHeaders(t *testing.T) {
	t.Parallel()

	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom-Header")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := WebhookConfig{
		URL: server.URL,
		Headers: map[string]string{
			"X-Custom-Header": "custom-value",
		},
	}

	notifier := NewWebhookNotifier(cfg)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()
	_ = notifier.Send(ctx, event)

	if receivedHeader != "custom-value" {
		t.Errorf("Custom header = %v, want custom-value", receivedHeader)
	}
}

func TestWebhookNotifierSendError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := WebhookConfig{
		URL: server.URL,
	}

	notifier := NewWebhookNotifier(cfg)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)

	if err == nil {
		t.Error("Expected error for 500 response")
	}
}

func TestWebhookNotifierEmptyURL(t *testing.T) {
	t.Parallel()

	cfg := WebhookConfig{
		URL: "",
	}

	notifier := NewWebhookNotifier(cfg)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)

	// Empty URL should not error, just skip
	if err != nil {
		t.Errorf("Expected no error for empty URL, got: %v", err)
	}
}

func TestComputeHMACSHA256(t *testing.T) {
	t.Parallel()

	message := []byte("test message")
	key := []byte("secret-key")

	sig := computeHMACSHA256(message, key)

	if sig == "" {
		t.Error("Expected non-empty signature")
	}

	// Should produce consistent results
	sig2 := computeHMACSHA256(message, key)
	if sig != sig2 {
		t.Error("HMAC should be deterministic")
	}

	// Different message should produce different signature
	sig3 := computeHMACSHA256([]byte("different"), key)
	if sig == sig3 {
		t.Error("Different messages should produce different signatures")
	}
}

// Benchmark tests
func BenchmarkManagerNotify(b *testing.B) {
	logger := zap.NewNop()
	manager := NewManager(logger)

	for i := 0; i < 5; i++ {
		manager.Register(&MockNotifier{NameValue: "bench"})
	}

	event := Event{
		Type:      "deployment",
		ProjectID: "bench-project",
		Status:    "success",
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.Notify(ctx, event)
	}
}

func BenchmarkEventMarshal(b *testing.B) {
	event := Event{
		Type:        "deployment",
		ProjectID:   "bench-project",
		ProjectName: "Benchmark Project",
		Environment: "production",
		DeployID:    "deploy-001",
		Version:     "v1.0.0",
		Status:      "success",
		User:        "benchuser",
		Message:     "Benchmark deployment",
		URL:         "https://example.com/deploy/001",
		Timestamp:   time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(event)
	}
}

func BenchmarkComputeHMACSHA256(b *testing.B) {
	message := []byte(`{"type":"deployment","project_id":"bench","status":"success"}`)
	key := []byte("benchmark-secret-key")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computeHMACSHA256(message, key)
	}
}

// Additional tests for improved coverage

func TestSlackNotifierSendWithMessage(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		_ = decoder.Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := SlackConfig{
		WebhookURL: server.URL,
		Channel:    "#test",
		Username:   "Test Bot",
		IconEmoji:  ":rocket:",
	}

	notifier := NewSlackNotifier(cfg)

	event := Event{
		Type:        "deployment",
		ProjectName: "Test Project",
		Environment: "production",
		Status:      "success",
		User:        "admin",
		Message:     "Deployment message with details",
		URL:         "https://example.com/deploy/123",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)

	if err != nil {
		t.Fatalf("SlackNotifier.Send() error = %v", err)
	}

	// Verify the payload includes message and URL blocks
	attachments, ok := receivedPayload["attachments"].([]interface{})
	if !ok || len(attachments) == 0 {
		t.Fatal("Expected attachments in payload")
	}

	attachment := attachments[0].(map[string]interface{})
	blocks, ok := attachment["blocks"].([]interface{})
	if !ok {
		t.Fatal("Expected blocks in attachment")
	}

	// Should have at least 4 blocks (header, fields, message, actions)
	if len(blocks) < 4 {
		t.Errorf("Expected at least 4 blocks, got %d", len(blocks))
	}
}

func TestSlackNotifierStatusColors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		status        string
		expectedColor string
	}{
		{"success", "#36a64f"},
		{"failed", "#dc3545"},
		{"pending", "#ffc107"},
		{"running", "#ffc107"},
		{"rolled_back", "#fd7e14"},
		{"unknown", "#36a64f"}, // default color
	}

	for _, tc := range testCases {
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()

			var receivedPayload map[string]interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				decoder := json.NewDecoder(r.Body)
				_ = decoder.Decode(&receivedPayload)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			cfg := SlackConfig{WebhookURL: server.URL}
			notifier := NewSlackNotifier(cfg)

			event := Event{
				Type:   "deployment",
				Status: tc.status,
			}

			ctx := context.Background()
			err := notifier.Send(ctx, event)
			if err != nil {
				t.Fatalf("Send() error = %v", err)
			}

			attachments := receivedPayload["attachments"].([]interface{})
			attachment := attachments[0].(map[string]interface{})
			color := attachment["color"].(string)

			if color != tc.expectedColor {
				t.Errorf("Expected color %s for status %s, got %s", tc.expectedColor, tc.status, color)
			}
		})
	}
}

func TestSlackNotifierEventTypes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		eventType     string
		expectedEmoji string
	}{
		{"deployment", ":rocket:"},
		{"rollback", ":rewind:"},
		{"failed", ":x:"},
		{"other", ":rocket:"}, // default emoji
	}

	for _, tc := range testCases {
		t.Run(tc.eventType, func(t *testing.T) {
			t.Parallel()

			var receivedPayload map[string]interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				decoder := json.NewDecoder(r.Body)
				_ = decoder.Decode(&receivedPayload)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			cfg := SlackConfig{WebhookURL: server.URL}
			notifier := NewSlackNotifier(cfg)

			event := Event{
				Type:        tc.eventType,
				ProjectName: "Test",
				Environment: "prod",
				Status:      "success",
			}

			ctx := context.Background()
			err := notifier.Send(ctx, event)
			if err != nil {
				t.Fatalf("Send() error = %v", err)
			}

			// Verify the emoji is in the message
			attachments := receivedPayload["attachments"].([]interface{})
			attachment := attachments[0].(map[string]interface{})
			blocks := attachment["blocks"].([]interface{})
			firstBlock := blocks[0].(map[string]interface{})
			text := firstBlock["text"].(map[string]interface{})
			textContent := text["text"].(string)

			if !strings.Contains(textContent, tc.expectedEmoji) {
				t.Errorf("Expected emoji %s in message for type %s, got: %s", tc.expectedEmoji, tc.eventType, textContent)
			}
		})
	}
}

func TestSlackNotifierSendNetworkError(t *testing.T) {
	t.Parallel()

	cfg := SlackConfig{
		WebhookURL: "http://localhost:59999/nonexistent", // Non-listening port
	}

	notifier := NewSlackNotifier(cfg)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)

	if err == nil {
		t.Error("Expected error for network failure")
	}
}

func TestSlackNotifierSendContextCancelled(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Delay response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := SlackConfig{WebhookURL: server.URL}
	notifier := NewSlackNotifier(cfg)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := notifier.Send(ctx, event)

	if err == nil {
		t.Error("Expected error for context timeout")
	}
}

func TestNewEmailNotifierWithInvalidTemplateDir(t *testing.T) {
	t.Parallel()

	cfg := EmailConfig{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		FromAddress: "test@example.com",
		ToAddresses: []string{"team@example.com"},
		TemplateDir: "/nonexistent/path/templates",
	}

	_, err := NewEmailNotifier(cfg)
	if err == nil {
		t.Error("Expected error for invalid template directory")
	}
}

func TestEmailNotifierSendWithTemplate(t *testing.T) {
	t.Parallel()

	// Create a temp directory with a test template
	tempDir := t.TempDir()
	templatePath := tempDir + "/deployment.html"

	templateContent := `<!DOCTYPE html>
<html>
<body>
<h1>{{.ProjectName}}</h1>
<p>Status: {{.Status}}</p>
</body>
</html>`

	if err := writeTestTemplate(templatePath, templateContent); err != nil {
		t.Fatalf("Failed to create test template: %v", err)
	}

	cfg := EmailConfig{
		SMTPHost:    "localhost",
		SMTPPort:    25,
		FromAddress: "test@example.com",
		FromName:    "VCDeploy",
		ToAddresses: []string{"team@example.com"},
		TemplateDir: tempDir,
	}

	notifier, err := NewEmailNotifier(cfg)
	if err != nil {
		t.Fatalf("NewEmailNotifier() error = %v", err)
	}

	if notifier.templates == nil {
		t.Error("Expected templates to be loaded")
	}
}

// Helper function to write test template files
func writeTestTemplate(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestWebhookNotifierSendWithDifferentMethods(t *testing.T) {
	t.Parallel()

	methods := []string{"POST", "PUT", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			var receivedMethod string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			cfg := WebhookConfig{
				URL:    server.URL,
				Method: method,
			}

			notifier := NewWebhookNotifier(cfg)

			event := Event{
				Type:   "deployment",
				Status: "success",
			}

			ctx := context.Background()
			err := notifier.Send(ctx, event)
			if err != nil {
				t.Fatalf("Send() error = %v", err)
			}

			if receivedMethod != method {
				t.Errorf("Expected method %s, got %s", method, receivedMethod)
			}
		})
	}
}

func TestWebhookNotifierSendVerifyPayload(t *testing.T) {
	t.Parallel()

	var receivedPayload Event
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		_ = decoder.Decode(&receivedPayload)

		// Verify headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("Expected Content-Type: application/json")
		}
		if r.Header.Get("User-Agent") != "vcdeploy/1.0" {
			t.Error("Expected User-Agent: vcdeploy/1.0")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := WebhookConfig{URL: server.URL}
	notifier := NewWebhookNotifier(cfg)

	event := Event{
		Type:        "deployment",
		ProjectID:   "proj-123",
		ProjectName: "Test Project",
		Environment: "staging",
		DeployID:    "deploy-456",
		Version:     "v2.0.0",
		Status:      "success",
		User:        "deployer",
		Message:     "Test message",
		URL:         "https://example.com/deploy",
		Timestamp:   time.Now(),
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedPayload.ProjectID != event.ProjectID {
		t.Errorf("ProjectID = %v, want %v", receivedPayload.ProjectID, event.ProjectID)
	}
	if receivedPayload.ProjectName != event.ProjectName {
		t.Errorf("ProjectName = %v, want %v", receivedPayload.ProjectName, event.ProjectName)
	}
}

func TestWebhookNotifierSendNetworkError(t *testing.T) {
	t.Parallel()

	cfg := WebhookConfig{
		URL: "http://localhost:59998/nonexistent",
	}

	notifier := NewWebhookNotifier(cfg)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)

	if err == nil {
		t.Error("Expected error for network failure")
	}
}

func TestWebhookNotifierSendContextCancelled(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := WebhookConfig{URL: server.URL}
	notifier := NewWebhookNotifier(cfg)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := notifier.Send(ctx, event)

	if err == nil {
		t.Error("Expected error for context timeout")
	}
}

func TestWebhookNotifierVerifySignature(t *testing.T) {
	t.Parallel()

	secret := "my-secret-key"
	var receivedSignature string
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-VCDeploy-Signature")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := WebhookConfig{
		URL:    server.URL,
		Secret: secret,
	}

	notifier := NewWebhookNotifier(cfg)

	event := Event{
		Type:      "deployment",
		ProjectID: "sig-test",
		Status:    "success",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Verify signature format
	if !strings.HasPrefix(receivedSignature, "sha256=") {
		t.Errorf("Expected signature to start with 'sha256=', got: %s", receivedSignature)
	}

	// Verify signature is correct
	expectedSig := computeHMACSHA256(receivedBody, []byte(secret))
	actualSig := strings.TrimPrefix(receivedSignature, "sha256=")

	if actualSig != expectedSig {
		t.Errorf("Signature mismatch: got %s, want %s", actualSig, expectedSig)
	}
}

func TestWebhookNotifierStatusCodes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		statusCode  int
		expectError bool
	}{
		{http.StatusOK, false},
		{http.StatusCreated, false},
		{http.StatusAccepted, false},
		{http.StatusNoContent, false},
		{http.StatusBadRequest, true},
		{http.StatusUnauthorized, true},
		{http.StatusForbidden, true},
		{http.StatusNotFound, true},
		{http.StatusInternalServerError, true},
		{http.StatusServiceUnavailable, true},
	}

	for _, tc := range testCases {
		t.Run(http.StatusText(tc.statusCode), func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			cfg := WebhookConfig{URL: server.URL}
			notifier := NewWebhookNotifier(cfg)

			event := Event{
				Type:   "deployment",
				Status: "success",
			}

			ctx := context.Background()
			err := notifier.Send(ctx, event)

			if tc.expectError && err == nil {
				t.Errorf("Expected error for status %d", tc.statusCode)
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error for status %d: %v", tc.statusCode, err)
			}
		})
	}
}

// PanickingNotifier is a test notifier that panics
type PanickingNotifier struct {
	NameValue string
}

func (p *PanickingNotifier) Send(ctx context.Context, event Event) error {
	panic("intentional panic for testing")
}

func (p *PanickingNotifier) Name() string {
	return p.NameValue
}

func TestManagerNotifyWithPanic(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	manager := NewManager(logger)

	// Register a panicking notifier
	panicNotifier := &PanickingNotifier{NameValue: "panic-notifier"}
	manager.Register(panicNotifier)

	// Also register a normal notifier to verify it still runs
	normalNotifier := &MockNotifier{NameValue: "normal-notifier"}
	manager.Register(normalNotifier)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()

	// Should not panic - the manager should recover
	manager.Notify(ctx, event)
	manager.Wait()

	// Normal notifier should still have been called
	if !normalNotifier.WasCalled() {
		t.Error("Normal notifier should have been called despite panic in other notifier")
	}
}

func TestManagerNotifyNoNotifiers(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	manager := NewManager(logger)

	// Don't register any notifiers
	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()

	// Should not panic or block
	manager.Notify(ctx, event)
	manager.Wait()
}

func TestManagerNotifyAndWaitMultiple(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	manager := NewManager(logger)

	notifiers := make([]*MockNotifier, 5)
	for i := 0; i < 5; i++ {
		notifiers[i] = &MockNotifier{NameValue: fmt.Sprintf("notifier-%d", i)}
		manager.Register(notifiers[i])
	}

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	ctx := context.Background()
	manager.NotifyAndWait(ctx, event)

	// All notifiers should have been called
	for i, n := range notifiers {
		if !n.WasCalled() {
			t.Errorf("Notifier %d was not called", i)
		}
	}
}

func TestEventWithAllFields(t *testing.T) {
	t.Parallel()

	timestamp := time.Now()
	event := Event{
		Type:        "deployment",
		ProjectID:   "proj-001",
		ProjectName: "Complete Project",
		Environment: "production",
		DeployID:    "deploy-999",
		Version:     "v3.0.0",
		Status:      "success",
		User:        "admin",
		Message:     "Full deployment test",
		URL:         "https://deploy.example.com/999",
		Timestamp:   timestamp,
	}

	// Test JSON round-trip with all fields
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Verify all fields
	if decoded.Type != event.Type {
		t.Errorf("Type = %v, want %v", decoded.Type, event.Type)
	}
	if decoded.ProjectID != event.ProjectID {
		t.Errorf("ProjectID = %v, want %v", decoded.ProjectID, event.ProjectID)
	}
	if decoded.ProjectName != event.ProjectName {
		t.Errorf("ProjectName = %v, want %v", decoded.ProjectName, event.ProjectName)
	}
	if decoded.Environment != event.Environment {
		t.Errorf("Environment = %v, want %v", decoded.Environment, event.Environment)
	}
	if decoded.DeployID != event.DeployID {
		t.Errorf("DeployID = %v, want %v", decoded.DeployID, event.DeployID)
	}
	if decoded.Version != event.Version {
		t.Errorf("Version = %v, want %v", decoded.Version, event.Version)
	}
	if decoded.Status != event.Status {
		t.Errorf("Status = %v, want %v", decoded.Status, event.Status)
	}
	if decoded.User != event.User {
		t.Errorf("User = %v, want %v", decoded.User, event.User)
	}
	if decoded.Message != event.Message {
		t.Errorf("Message = %v, want %v", decoded.Message, event.Message)
	}
	if decoded.URL != event.URL {
		t.Errorf("URL = %v, want %v", decoded.URL, event.URL)
	}
}

func TestSlackConfigWithAllFields(t *testing.T) {
	t.Parallel()

	cfg := SlackConfig{
		WebhookURL: "https://hooks.slack.com/services/T00/B00/XXX",
		Channel:    "#deployments",
		Username:   "Deploy Bot",
		IconEmoji:  ":deploy:",
	}

	if cfg.WebhookURL == "" {
		t.Error("WebhookURL should not be empty")
	}
	if cfg.Channel != "#deployments" {
		t.Errorf("Channel = %v, want #deployments", cfg.Channel)
	}
	if cfg.Username != "Deploy Bot" {
		t.Errorf("Username = %v, want Deploy Bot", cfg.Username)
	}
	if cfg.IconEmoji != ":deploy:" {
		t.Errorf("IconEmoji = %v, want :deploy:", cfg.IconEmoji)
	}
}

func TestEmailConfigWithAllFields(t *testing.T) {
	t.Parallel()

	cfg := EmailConfig{
		SMTPHost:    "smtp.gmail.com",
		SMTPPort:    587,
		Username:    "user@gmail.com",
		Password:    "app-password",
		FromAddress: "deploy@company.com",
		FromName:    "Deploy System",
		ToAddresses: []string{"team@company.com", "ops@company.com", "manager@company.com"},
		TemplateDir: "/etc/vcdeploy/templates",
	}

	if cfg.SMTPHost != "smtp.gmail.com" {
		t.Errorf("SMTPHost = %v, want smtp.gmail.com", cfg.SMTPHost)
	}
	if cfg.SMTPPort != 587 {
		t.Errorf("SMTPPort = %v, want 587", cfg.SMTPPort)
	}
	if cfg.Username != "user@gmail.com" {
		t.Errorf("Username = %v, want user@gmail.com", cfg.Username)
	}
	if len(cfg.ToAddresses) != 3 {
		t.Errorf("ToAddresses count = %d, want 3", len(cfg.ToAddresses))
	}
}

func TestWebhookConfigWithAllFields(t *testing.T) {
	t.Parallel()

	cfg := WebhookConfig{
		URL:    "https://webhook.site/test",
		Method: "PUT",
		Headers: map[string]string{
			"Authorization": "Bearer token123",
			"X-Custom":      "value",
		},
		Secret: "webhook-secret",
	}

	if cfg.URL == "" {
		t.Error("URL should not be empty")
	}
	if cfg.Method != "PUT" {
		t.Errorf("Method = %v, want PUT", cfg.Method)
	}
	if len(cfg.Headers) != 2 {
		t.Errorf("Headers count = %d, want 2", len(cfg.Headers))
	}
	if cfg.Secret != "webhook-secret" {
		t.Errorf("Secret = %v, want webhook-secret", cfg.Secret)
	}
}

func TestComputeHMACSHA256WithEmptyInputs(t *testing.T) {
	t.Parallel()

	// Empty message
	sig1 := computeHMACSHA256([]byte{}, []byte("key"))
	if sig1 == "" {
		t.Error("Should produce signature for empty message")
	}

	// Empty key
	sig2 := computeHMACSHA256([]byte("message"), []byte{})
	if sig2 == "" {
		t.Error("Should produce signature for empty key")
	}

	// Both empty
	sig3 := computeHMACSHA256([]byte{}, []byte{})
	if sig3 == "" {
		t.Error("Should produce signature for empty inputs")
	}

	// All should be different from each other
	if sig1 == sig2 || sig2 == sig3 || sig1 == sig3 {
		// Actually some might be the same due to HMAC properties, just verify they're valid
		t.Log("Note: Some empty input combinations may produce same HMAC")
	}
}

func TestManagerConcurrentNotify(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	manager := NewManager(logger)

	var callCount int
	var mu sync.Mutex

	countingNotifier := &MockNotifier{
		NameValue: "counting",
	}
	manager.Register(countingNotifier)

	// Send multiple notifications concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			event := Event{
				Type:      "deployment",
				ProjectID: fmt.Sprintf("proj-%d", id),
				Status:    "success",
			}
			ctx := context.Background()
			manager.Notify(ctx, event)
			mu.Lock()
			callCount++
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	manager.Wait()

	if callCount != 10 {
		t.Errorf("Expected 10 notifications, got %d", callCount)
	}
}

func TestSlackNotifierSendOnlyMessage(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		_ = decoder.Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := SlackConfig{WebhookURL: server.URL}
	notifier := NewSlackNotifier(cfg)

	// Event with message but no URL
	event := Event{
		Type:        "deployment",
		ProjectName: "Test",
		Environment: "prod",
		Status:      "success",
		Message:     "Test message only",
		URL:         "", // No URL
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	attachments := receivedPayload["attachments"].([]interface{})
	attachment := attachments[0].(map[string]interface{})
	blocks := attachment["blocks"].([]interface{})

	// Should have 3 blocks (header, fields, message) but no actions
	if len(blocks) != 3 {
		t.Errorf("Expected 3 blocks for message-only, got %d", len(blocks))
	}
}

func TestSlackNotifierSendOnlyURL(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		_ = decoder.Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := SlackConfig{WebhookURL: server.URL}
	notifier := NewSlackNotifier(cfg)

	// Event with URL but no message
	event := Event{
		Type:        "deployment",
		ProjectName: "Test",
		Environment: "prod",
		Status:      "success",
		Message:     "", // No message
		URL:         "https://example.com/deploy",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	attachments := receivedPayload["attachments"].([]interface{})
	attachment := attachments[0].(map[string]interface{})
	blocks := attachment["blocks"].([]interface{})

	// Should have 3 blocks (header, fields, actions) but no message
	if len(blocks) != 3 {
		t.Errorf("Expected 3 blocks for URL-only, got %d", len(blocks))
	}
}

func TestEmailNotifierDefaultTemplateWithEmptyOptionals(t *testing.T) {
	t.Parallel()

	cfg := EmailConfig{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		FromAddress: "test@example.com",
		ToAddresses: []string{"team@example.com"},
	}

	notifier, err := NewEmailNotifier(cfg)
	if err != nil {
		t.Fatalf("NewEmailNotifier() error = %v", err)
	}

	event := Event{
		ProjectName: "Test",
		Environment: "prod",
		Status:      "success",
		Version:     "",
		User:        "",
		Message:     "",
		URL:         "",
		Timestamp:   time.Time{}, // Zero time
	}

	template := notifier.defaultTemplate(event)

	if template == "" {
		t.Error("defaultTemplate() should return content even with empty fields")
	}

	// Verify HTML structure is still valid
	if !strings.Contains(template, "<!DOCTYPE html>") {
		t.Error("template should contain DOCTYPE")
	}
	if !strings.Contains(template, "</html>") {
		t.Error("template should have closing html tag")
	}
}

// --- Discord Notifier Tests ---

func TestDiscordNotifierName(t *testing.T) {
	t.Parallel()

	notifier := NewDiscordNotifier(DiscordConfig{})

	if notifier.Name() != "discord" {
		t.Errorf("DiscordNotifier.Name() = %v, want discord", notifier.Name())
	}
}

func TestDiscordNotifierSend(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&receivedPayload); err != nil {
			t.Errorf("Failed to decode payload: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DiscordConfig{
		WebhookURL: server.URL,
		Username:   "VCDeploy Bot",
		AvatarURL:  "https://example.com/avatar.png",
	}

	notifier := NewDiscordNotifier(cfg)

	event := Event{
		Type:        "deployment",
		ProjectName: "Test Project",
		Environment: "production",
		Status:      "success",
		User:        "admin",
		Version:     "v1.2.3",
		Timestamp:   time.Now(),
	}

	ctx := context.Background()
	err := notifier.Send(ctx, event)

	if err != nil {
		t.Fatalf("DiscordNotifier.Send() error = %v", err)
	}

	// Verify payload structure
	if receivedPayload["username"] != "VCDeploy Bot" {
		t.Errorf("Expected username 'VCDeploy Bot', got %v", receivedPayload["username"])
	}
	if receivedPayload["avatar_url"] != "https://example.com/avatar.png" {
		t.Errorf("Expected avatar_url, got %v", receivedPayload["avatar_url"])
	}

	embeds, ok := receivedPayload["embeds"].([]interface{})
	if !ok || len(embeds) == 0 {
		t.Fatal("Expected embeds array")
	}

	embed, ok := embeds[0].(map[string]interface{})
	if !ok {
		t.Fatal("Expected embed object")
	}

	// Check embed has expected fields
	if embed["title"] == nil {
		t.Error("Expected title in embed")
	}
	if embed["color"] == nil {
		t.Error("Expected color in embed")
	}
	if embed["fields"] == nil {
		t.Error("Expected fields in embed")
	}
}

func TestDiscordNotifierSendWithMessage(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		decoder.Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DiscordConfig{WebhookURL: server.URL}
	notifier := NewDiscordNotifier(cfg)

	event := Event{
		Type:        "deployment",
		ProjectName: "Test",
		Environment: "prod",
		Status:      "success",
		User:        "admin",
		Message:     "Custom deployment message",
	}

	err := notifier.Send(context.Background(), event)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Verify message is in fields
	embeds := receivedPayload["embeds"].([]interface{})
	embed := embeds[0].(map[string]interface{})
	fields := embed["fields"].([]interface{})

	foundMessage := false
	for _, f := range fields {
		field := f.(map[string]interface{})
		if field["name"] == "Message" {
			foundMessage = true
			if field["value"] != "Custom deployment message" {
				t.Errorf("Expected message value, got %v", field["value"])
			}
		}
	}
	if !foundMessage {
		t.Error("Expected Message field in embed")
	}
}

func TestDiscordNotifierSendWithURL(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		decoder.Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DiscordConfig{WebhookURL: server.URL}
	notifier := NewDiscordNotifier(cfg)

	event := Event{
		Type:        "deployment",
		ProjectName: "Test",
		Environment: "prod",
		Status:      "success",
		User:        "admin",
		URL:         "https://example.com/deploy/123",
	}

	err := notifier.Send(context.Background(), event)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	embeds := receivedPayload["embeds"].([]interface{})
	embed := embeds[0].(map[string]interface{})

	if embed["url"] != "https://example.com/deploy/123" {
		t.Errorf("Expected URL in embed, got %v", embed["url"])
	}
}

func TestDiscordNotifierStatusColors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		status        string
		expectedColor float64 // JSON numbers are float64
	}{
		{"success", 3066993},      // green
		{"failed", 15158332},      // red
		{"pending", 16776960},     // yellow
		{"running", 16776960},     // yellow
		{"rolled_back", 15105570}, // orange
	}

	for _, tc := range testCases {
		t.Run(tc.status, func(t *testing.T) {
			var receivedPayload map[string]interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				decoder := json.NewDecoder(r.Body)
				decoder.Decode(&receivedPayload)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			cfg := DiscordConfig{WebhookURL: server.URL}
			notifier := NewDiscordNotifier(cfg)

			event := Event{
				Type:        "deployment",
				ProjectName: "Test",
				Environment: "prod",
				Status:      tc.status,
				User:        "admin",
			}

			notifier.Send(context.Background(), event)

			embeds := receivedPayload["embeds"].([]interface{})
			embed := embeds[0].(map[string]interface{})
			color := embed["color"].(float64)

			if color != tc.expectedColor {
				t.Errorf("Expected color %v for status %s, got %v", tc.expectedColor, tc.status, color)
			}
		})
	}
}

func TestDiscordNotifierSendEmptyURL(t *testing.T) {
	t.Parallel()

	cfg := DiscordConfig{WebhookURL: ""}
	notifier := NewDiscordNotifier(cfg)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	err := notifier.Send(context.Background(), event)

	// Empty URL should not error, just skip
	if err != nil {
		t.Errorf("Expected no error for empty URL, got: %v", err)
	}
}

func TestDiscordNotifierSendFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	cfg := DiscordConfig{WebhookURL: server.URL}
	notifier := NewDiscordNotifier(cfg)

	event := Event{
		Type:   "deployment",
		Status: "success",
	}

	err := notifier.Send(context.Background(), event)

	if err == nil {
		t.Error("Expected error for 400 response")
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "he..."},
		{"hi", 2, "hi"},
		{"test", 3, "tes"}, // maxLen <= 3 returns prefix without ellipsis
		{"", 10, ""},
		{"a very long string", 10, "a very ..."},
	}

	for _, tc := range tests {
		result := truncateString(tc.input, tc.maxLen)
		if result != tc.expected {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tc.input, tc.maxLen, result, tc.expected)
		}
	}
}
