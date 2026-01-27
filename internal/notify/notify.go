package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"go.uber.org/zap"
)

// Event represents a deployment event for notifications
type Event struct {
	Type        string    `json:"type"`
	ProjectID   string    `json:"project_id"`
	ProjectName string    `json:"project_name"`
	Environment string    `json:"environment"`
	DeployID    string    `json:"deploy_id"`
	Version     string    `json:"version"`
	Status      string    `json:"status"`
	User        string    `json:"user"`
	Message     string    `json:"message"`
	URL         string    `json:"url"`
	Timestamp   time.Time `json:"timestamp"`
}

// Notifier interface for different notification channels
type Notifier interface {
	Send(ctx context.Context, event Event) error
	Name() string
}

// Manager handles multiple notification channels.
// Thread-safety: Register should only be called during initialization before
// any calls to Notify. If dynamic registration is needed, external synchronization
// is required.
type Manager struct {
	logger    *zap.Logger
	mu        sync.RWMutex // Protects notifiers slice
	notifiers []Notifier
	wg        sync.WaitGroup
}

// NewManager creates a notification manager
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		logger:    logger,
		notifiers: make([]Notifier, 0),
	}
}

// Register adds a notifier to the manager.
// Note: Register should be called during initialization only.
func (m *Manager) Register(n Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifiers = append(m.notifiers, n)
	m.logger.Info("registered notifier", zap.String("name", n.Name()))
}

// Notify sends an event to all registered notifiers asynchronously.
// Use NotifyAndWait if you need to wait for all notifications to complete.
func (m *Manager) Notify(ctx context.Context, event Event) {
	event.Timestamp = time.Now()

	m.mu.RLock()
	notifiers := m.notifiers // Copy slice reference under lock
	m.mu.RUnlock()

	for _, n := range notifiers {
		m.wg.Add(1)
		go func(notifier Notifier) {
			defer m.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("panic in notifier",
						zap.String("notifier", notifier.Name()),
						zap.Any("panic", r),
					)
				}
			}()
			if err := notifier.Send(ctx, event); err != nil {
				m.logger.Error("notification failed",
					zap.String("notifier", notifier.Name()),
					zap.Error(err),
				)
			}
		}(n)
	}
}

// NotifyAndWait sends an event to all registered notifiers and waits for completion.
// Use this during shutdown to ensure all notifications are sent.
func (m *Manager) NotifyAndWait(ctx context.Context, event Event) {
	m.Notify(ctx, event)
	m.Wait()
}

// Wait blocks until all in-flight notifications complete.
func (m *Manager) Wait() {
	m.wg.Wait()
}

// SlackConfig holds Slack notification settings
type SlackConfig struct {
	WebhookURL string
	Channel    string
	Username   string
	IconEmoji  string
}

// SlackNotifier sends notifications to Slack
type SlackNotifier struct {
	config SlackConfig
	client *http.Client
}

// NewSlackNotifier creates a Slack notifier
func NewSlackNotifier(cfg SlackConfig) *SlackNotifier {
	return &SlackNotifier{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name returns the notifier name
func (s *SlackNotifier) Name() string {
	return "slack"
}

// Send sends a notification to Slack
func (s *SlackNotifier) Send(ctx context.Context, event Event) error {
	if s.config.WebhookURL == "" {
		return nil
	}

	color := "#36a64f" // green
	switch event.Status {
	case "failed":
		color = "#dc3545"
	case "pending", "running":
		color = "#ffc107"
	case "rolled_back":
		color = "#fd7e14"
	}

	emoji := ":rocket:"
	switch event.Type {
	case "rollback":
		emoji = ":rewind:"
	case "failed":
		emoji = ":x:"
	}

	payload := map[string]interface{}{
		"channel":    s.config.Channel,
		"username":   s.config.Username,
		"icon_emoji": s.config.IconEmoji,
		"attachments": []map[string]interface{}{
			{
				"color": color,
				"blocks": []map[string]interface{}{
					{
						"type": "section",
						"text": map[string]string{
							"type": "mrkdwn",
							"text": fmt.Sprintf("%s *%s* deployment to *%s*", emoji, event.ProjectName, event.Environment),
						},
					},
					{
						"type": "section",
						"fields": []map[string]string{
							{"type": "mrkdwn", "text": fmt.Sprintf("*Status:*\n%s", event.Status)},
							{"type": "mrkdwn", "text": fmt.Sprintf("*Version:*\n%s", event.Version)},
							{"type": "mrkdwn", "text": fmt.Sprintf("*Triggered by:*\n%s", event.User)},
							{"type": "mrkdwn", "text": fmt.Sprintf("*Time:*\n%s", event.Timestamp.Format(time.RFC822))},
						},
					},
				},
			},
		},
	}

	if event.Message != "" {
		attachments := payload["attachments"].([]map[string]interface{})
		blocks := attachments[0]["blocks"].([]map[string]interface{})
		blocks = append(blocks, map[string]interface{}{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*Message:*\n%s", event.Message),
			},
		})
		attachments[0]["blocks"] = blocks
	}

	if event.URL != "" {
		attachments := payload["attachments"].([]map[string]interface{})
		blocks := attachments[0]["blocks"].([]map[string]interface{})
		blocks = append(blocks, map[string]interface{}{
			"type": "actions",
			"elements": []map[string]interface{}{
				{
					"type": "button",
					"text": map[string]string{
						"type": "plain_text",
						"text": "View Deployment",
					},
					"url": event.URL,
				},
			},
		})
		attachments[0]["blocks"] = blocks
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	return nil
}

// EmailConfig holds email notification settings
type EmailConfig struct {
	SMTPHost    string
	SMTPPort    int
	Username    string
	Password    string
	FromAddress string
	FromName    string
	ToAddresses []string
	TemplateDir string
}

// EmailNotifier sends notifications via email
type EmailNotifier struct {
	config    EmailConfig
	templates *template.Template
}

// NewEmailNotifier creates an email notifier
func NewEmailNotifier(cfg EmailConfig) (*EmailNotifier, error) {
	n := &EmailNotifier{config: cfg}

	if cfg.TemplateDir != "" {
		tmpl, err := template.ParseGlob(cfg.TemplateDir + "/*.html")
		if err != nil {
			return nil, fmt.Errorf("parse templates: %w", err)
		}
		n.templates = tmpl
	}

	return n, nil
}

// Name returns the notifier name
func (e *EmailNotifier) Name() string {
	return "email"
}

// Send sends a notification via email
func (e *EmailNotifier) Send(ctx context.Context, event Event) error {
	if e.config.SMTPHost == "" || len(e.config.ToAddresses) == 0 {
		return nil
	}

	subject := fmt.Sprintf("[vcdeploy] %s - %s deployment %s",
		event.ProjectName, event.Environment, event.Status)

	var body bytes.Buffer
	if e.templates != nil {
		if err := e.templates.ExecuteTemplate(&body, "deployment.html", event); err != nil {
			return fmt.Errorf("execute template: %w", err)
		}
	} else {
		body.WriteString(e.defaultTemplate(event))
	}

	msg := fmt.Sprintf("From: %s <%s>\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=utf-8\r\n"+
		"\r\n"+
		"%s",
		e.config.FromName,
		e.config.FromAddress,
		strings.Join(e.config.ToAddresses, ", "),
		subject,
		body.String(),
	)

	addr := net.JoinHostPort(e.config.SMTPHost, strconv.Itoa(e.config.SMTPPort))
	var auth smtp.Auth
	if e.config.Username != "" {
		auth = smtp.PlainAuth("", e.config.Username, e.config.Password, e.config.SMTPHost)
	}

	err := smtp.SendMail(addr, auth, e.config.FromAddress, e.config.ToAddresses, []byte(msg))
	if err != nil {
		return fmt.Errorf("send mail: %w", err)
	}

	return nil
}

func (e *EmailNotifier) defaultTemplate(event Event) string {
	statusColor := "#28a745"
	switch event.Status {
	case "failed":
		statusColor = "#dc3545"
	case "pending", "running":
		statusColor = "#ffc107"
	case "rolled_back":
		statusColor = "#fd7e14"
	}

	messageSection := ""
	if event.Message != "" {
		messageSection = fmt.Sprintf("<dt>Message</dt><dd>%s</dd>", event.Message)
	}

	buttonSection := ""
	if event.URL != "" {
		buttonSection = fmt.Sprintf(`<p><a href="%s" class="button">View Deployment</a></p>`, event.URL)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #1a1a2e; color: white; padding: 20px; border-radius: 8px 8px 0 0; }
        .content { background: #f8f9fa; padding: 20px; border-radius: 0 0 8px 8px; }
        .status { display: inline-block; padding: 4px 12px; border-radius: 4px; color: white; background: %s; }
        .details { margin: 20px 0; }
        .details dt { color: #666; font-size: 12px; margin-top: 12px; }
        .details dd { margin: 4px 0 0 0; font-weight: 500; }
        .button { display: inline-block; padding: 10px 20px; background: #28a745; color: white; text-decoration: none; border-radius: 4px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1 style="margin: 0;">🚀 Deployment Notification</h1>
        </div>
        <div class="content">
            <p><strong>%s</strong> deployment to <strong>%s</strong></p>
            <p>Status: <span class="status">%s</span></p>
            
            <dl class="details">
                <dt>Version</dt>
                <dd>%s</dd>
                
                <dt>Triggered by</dt>
                <dd>%s</dd>
                
                <dt>Time</dt>
                <dd>%s</dd>
                
                %s
            </dl>
            
            %s
        </div>
    </div>
</body>
</html>`,
		statusColor,
		event.ProjectName,
		event.Environment,
		event.Status,
		event.Version,
		event.User,
		event.Timestamp.Format(time.RFC1123),
		messageSection,
		buttonSection,
	)
}

// WebhookConfig holds webhook notification settings
type WebhookConfig struct {
	URL     string
	Method  string
	Headers map[string]string
	Secret  string
}

// WebhookNotifier sends notifications to custom webhooks
type WebhookNotifier struct {
	config WebhookConfig
	client *http.Client
}

// NewWebhookNotifier creates a webhook notifier
func NewWebhookNotifier(cfg WebhookConfig) *WebhookNotifier {
	if cfg.Method == "" {
		cfg.Method = "POST"
	}
	return &WebhookNotifier{
		config: cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name returns the notifier name
func (w *WebhookNotifier) Name() string {
	return "webhook"
}

// Send sends a notification to the configured webhook
func (w *WebhookNotifier) Send(ctx context.Context, event Event) error {
	if w.config.URL == "" {
		return nil
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, w.config.Method, w.config.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "vcdeploy/1.0")

	for k, v := range w.config.Headers {
		req.Header.Set(k, v)
	}

	// Add HMAC signature if secret is configured
	if w.config.Secret != "" {
		sig := computeHMACSHA256(body, []byte(w.config.Secret))
		req.Header.Set("X-VCDeploy-Signature", "sha256="+sig)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// computeHMACSHA256 computes HMAC-SHA256 and returns hex string
func computeHMACSHA256(message, key []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(message)
	return hex.EncodeToString(h.Sum(nil))
}

// --- Discord Notifier ---

// DiscordConfig holds Discord notification settings
type DiscordConfig struct {
	WebhookURL string
	Username   string
	AvatarURL  string
}

// DiscordNotifier sends notifications to Discord
type DiscordNotifier struct {
	config DiscordConfig
	client *http.Client
}

// NewDiscordNotifier creates a Discord notifier
func NewDiscordNotifier(cfg DiscordConfig) *DiscordNotifier {
	return &DiscordNotifier{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name returns the notifier name
func (d *DiscordNotifier) Name() string {
	return "discord"
}

// Send sends a notification to Discord
func (d *DiscordNotifier) Send(ctx context.Context, event Event) error {
	if d.config.WebhookURL == "" {
		return nil
	}

	// Map status to color (Discord uses decimal colors)
	color := 3066993 // green (#2ECC71)
	switch event.Status {
	case "failed":
		color = 15158332 // red (#E74C3C)
	case "pending", "running":
		color = 16776960 // yellow (#FFFF00)
	case "rolled_back":
		color = 15105570 // orange (#E67E22)
	}

	emoji := "🚀"
	switch event.Type {
	case "rollback":
		emoji = "⏪"
	case "failed":
		emoji = "❌"
	}

	// Build Discord embed
	embed := map[string]interface{}{
		"title":       fmt.Sprintf("%s %s Deployment", emoji, event.ProjectName),
		"description": fmt.Sprintf("Deployment to **%s**", event.Environment),
		"color":       color,
		"fields": []map[string]interface{}{
			{
				"name":   "Status",
				"value":  event.Status,
				"inline": true,
			},
			{
				"name":   "Version",
				"value":  event.Version,
				"inline": true,
			},
			{
				"name":   "Triggered By",
				"value":  event.User,
				"inline": true,
			},
		},
		"timestamp": event.Timestamp.Format(time.RFC3339),
		"footer": map[string]string{
			"text": "VCDeploy",
		},
	}

	if event.Message != "" {
		fields := embed["fields"].([]map[string]interface{})
		fields = append(fields, map[string]interface{}{
			"name":   "Message",
			"value":  truncateString(event.Message, 1024),
			"inline": false,
		})
		embed["fields"] = fields
	}

	if event.URL != "" {
		embed["url"] = event.URL
	}

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{embed},
	}

	if d.config.Username != "" {
		payload["username"] = d.config.Username
	}
	if d.config.AvatarURL != "" {
		payload["avatar_url"] = d.config.AvatarURL
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", d.config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "vcdeploy/1.0")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord returned status %d", resp.StatusCode)
	}

	return nil
}

// truncateString truncates a string to the specified max length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
