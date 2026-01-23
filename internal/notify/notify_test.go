package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	logger, _ := zap.NewDevelopment()
	manager := NewManager(logger)

	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
}

func TestManagerRegister(t *testing.T) {
	logger, _ := zap.NewDevelopment()
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
	logger, _ := zap.NewDevelopment()
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
	logger, _ := zap.NewDevelopment()
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
	logger, _ := zap.NewDevelopment()
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

func TestSlackConfig(t *testing.T) {
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
	// Create test server
	var receivedPayload []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
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

func TestWebhookConfig(t *testing.T) {
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
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		decoder.Decode(&receivedPayload)
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
	notifier.Send(ctx, event)

	if receivedSignature == "" {
		t.Error("Expected X-VCDeploy-Signature header")
	}
}

func TestWebhookNotifierWithCustomHeaders(t *testing.T) {
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
	notifier.Send(ctx, event)

	if receivedHeader != "custom-value" {
		t.Errorf("Custom header = %v, want custom-value", receivedHeader)
	}
}

func TestWebhookNotifierSendError(t *testing.T) {
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
	logger, _ := zap.NewDevelopment()
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
		json.Marshal(event)
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
