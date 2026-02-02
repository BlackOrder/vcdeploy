// Package server provides the master daemon HTTP and gRPC servers.
package server

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/alerting"
	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/metrics"
	"github.com/BlackOrder/vcdeploy/internal/notify"
	"github.com/BlackOrder/vcdeploy/internal/proto"
	"github.com/BlackOrder/vcdeploy/internal/scheduler"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/services/agents"
	"github.com/BlackOrder/vcdeploy/internal/services/apikeys"
	"github.com/BlackOrder/vcdeploy/internal/services/audit"
	"github.com/BlackOrder/vcdeploy/internal/services/deployments"
	"github.com/BlackOrder/vcdeploy/internal/services/hostkeys"
	"github.com/BlackOrder/vcdeploy/internal/services/projects"
	"github.com/BlackOrder/vcdeploy/internal/services/projecttypes"
	"github.com/BlackOrder/vcdeploy/internal/services/provision"
	"github.com/BlackOrder/vcdeploy/internal/services/secrets"
	"github.com/BlackOrder/vcdeploy/internal/services/sessions"
	"github.com/BlackOrder/vcdeploy/internal/services/settings"
	"github.com/BlackOrder/vcdeploy/internal/services/users"
	"github.com/BlackOrder/vcdeploy/internal/services/webhooks"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	webhookshandler "github.com/BlackOrder/vcdeploy/internal/webhooks"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// createNotifyManager creates a notification manager from config with registered notifiers.
// It validates configuration for each enabled provider, logs warnings for misconfigurations,
// and gracefully skips providers that fail to initialize.
func createNotifyManager(cfg config.NotificationsConfig, logger *zap.Logger) *notify.Manager {
	mgr := notify.NewManager(logger)

	// Slack notifications
	if cfg.Providers.Slack.Enabled {
		if cfg.Providers.Slack.WebhookURL == "" {
			logger.Warn("Slack notifications enabled but webhook_url not configured")
		} else {
			mgr.Register(notify.NewSlackNotifier(notify.SlackConfig{
				WebhookURL: cfg.Providers.Slack.WebhookURL,
				Channel:    cfg.Providers.Slack.Channel,
				Username:   cfg.Providers.Slack.Username,
				IconEmoji:  cfg.Providers.Slack.IconEmoji,
			}))
			logger.Info("Slack notification provider registered")
		}
	}

	// Email notifications
	if cfg.Providers.Email.Enabled {
		smtp := cfg.Providers.Email.SMTP
		if smtp.Host == "" {
			logger.Warn("Email notifications enabled but SMTP host not configured")
		} else if smtp.FromAddress == "" {
			logger.Warn("Email notifications enabled but from_address not configured")
		} else if len(smtp.ToAddresses) == 0 {
			logger.Warn("Email notifications enabled but no to_addresses configured")
		} else {
			notifier, err := notify.NewEmailNotifier(notify.EmailConfig{
				SMTPHost:    smtp.Host,
				SMTPPort:    smtp.Port,
				Username:    smtp.User,
				Password:    smtp.Password,
				FromAddress: smtp.FromAddress,
				FromName:    smtp.FromName,
				ToAddresses: smtp.ToAddresses,
			})
			if err != nil {
				logger.Warn("Failed to create email notifier", zap.Error(err))
			} else {
				mgr.Register(notifier)
				logger.Info("Email notification provider registered")
			}
		}
	}

	// Webhook notifications
	if cfg.Providers.Webhook.Enabled {
		if cfg.Providers.Webhook.URL == "" {
			logger.Warn("Webhook notifications enabled but URL not configured")
		} else {
			mgr.Register(notify.NewWebhookNotifier(notify.WebhookConfig{
				URL:     cfg.Providers.Webhook.URL,
				Method:  cfg.Providers.Webhook.Method,
				Headers: cfg.Providers.Webhook.Headers,
				Secret:  cfg.Providers.Webhook.Secret,
			}))
			logger.Info("Webhook notification provider registered")
		}
	}

	// Discord notifications
	if cfg.Providers.Discord.Enabled {
		if cfg.Providers.Discord.WebhookURL == "" {
			logger.Warn("Discord notifications enabled but webhook_url not configured")
		} else {
			mgr.Register(notify.NewDiscordNotifier(notify.DiscordConfig{
				WebhookURL: cfg.Providers.Discord.WebhookURL,
				Username:   cfg.Providers.Discord.Username,
				AvatarURL:  cfg.Providers.Discord.AvatarURL,
			}))
			logger.Info("Discord notification provider registered")
		}
	}

	return mgr
}

// MasterServer is the main daemon server.
type MasterServer struct {
	config     *config.MasterConfig
	store      storage.Store
	logger     *zap.Logger
	httpServer *http.Server
	grpcServer *grpc.Server

	// gRPC agent service
	agentServer *AgentServer
	caManager   *security.CAManager

	// KMS for secret encryption/decryption
	kms *security.KMS

	// Service layer - new architecture
	secretService      services.SecretServicer
	settingsSvc        services.SettingsServicer
	userService        services.UserServicer
	sessionService     services.SessionServicer
	apiKeyService      services.APIKeyServicer
	projectService     services.ProjectServicer
	projectTypeService services.ProjectTypeServicer
	webhookService     services.WebhookServicer
	deploymentService  services.DeploymentServicer
	agentService       services.AgentServicer
	auditService       services.AuditServicer
	hostKeyService     services.HostKeyServicer
	provisionService   services.ProvisionServicer

	// Webhook handling
	webhookHandler *webhookHandlerAdapter

	// Alerting
	alertManager *alerting.Manager

	// Security middleware
	securityMiddleware    *SecurityMiddleware
	cspMiddleware         *CSPMiddleware
	rateLimiter           *RateLimiter
	enforcementMiddleware *EnforcementMiddleware
	logSizeEnforcer       *LogSizeEnforcer

	// Agent management (for HTTP API, synced from gRPC service)
	agents   map[string]*AgentConnection
	agentsMu sync.RWMutex

	// Templates (loaded from disk) - map of page name to compiled template
	templates    map[string]*template.Template
	templatesDir string

	// Shutdown handling
	shutdown     chan struct{}
	shutdownOnce sync.Once
	wg           sync.WaitGroup

	// Scheduled jobs
	scheduler *scheduler.Scheduler

	// Setup state - true when no users exist and env credentials not provided
	requiresSetup bool
}

// AgentConnection tracks a connected agent.
type AgentConnection struct {
	ID          string
	Name        string
	Tags        []string
	ConnectedAt time.Time
	LastPing    time.Time
	Status      string
	Stream      interface{} // gRPC stream
}

// webhookHandlerAdapter wraps the webhooks.Handler for MasterServer.
type webhookHandlerAdapter struct {
	handler *webhookshandler.Handler
}

// webhookSecretStoreAdapter implements webhooks.SecretStore using the Store.
type webhookSecretStoreAdapter struct {
	store  storage.Store
	kms    *security.KMS
	logger *zap.Logger
}

func (a *webhookSecretStoreAdapter) GetWebhookSecret(projectID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Look up project by name (projectID in URL is the project name/slug)
	project, err := a.store.GetProjectByName(ctx, projectID)
	if services.IsNotFound(err) {
		return "", fmt.Errorf("project not found: %s", projectID)
	}
	if err != nil {
		return "", fmt.Errorf("looking up project: %w", err)
	}

	// Try each provider (github, gitlab, bitbucket)
	providers := []string{"github", "gitlab", "bitbucket"}
	for _, provider := range providers {
		webhook, err := a.store.GetProjectWebhook(ctx, project.ID, provider)
		if services.IsNotFound(err) {
			continue
		}
		if err != nil {
			continue
		}
		if webhook != nil && webhook.Enabled && len(webhook.SecretEncrypted) > 0 {
			// Decrypt the secret using KMS
			if a.kms != nil {
				// The secret is stored as base64-encoded encrypted data
				encoded := string(webhook.SecretEncrypted)
				secret, err := a.kms.DecryptString(ctx, encoded)
				if err != nil {
					a.logger.Error("Failed to decrypt webhook secret", zap.Error(err))
					continue
				}
				return secret, nil
			}
		}
	}

	// No secret configured
	return "", nil
}

// IsSecretRequired checks if a webhook secret is required for a project.
func (a *webhookSecretStoreAdapter) IsSecretRequired(projectID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Look up project by name
	project, err := a.store.GetProjectByName(ctx, projectID)
	if err != nil {
		return false // If we can't find the project, don't require secret
	}

	// Check each provider for require_secret setting
	providers := []string{"github", "gitlab", "bitbucket"}
	for _, provider := range providers {
		webhook, err := a.store.GetProjectWebhook(ctx, project.ID, provider)
		if err != nil {
			continue
		}
		if webhook != nil && webhook.Enabled && webhook.RequireSecret {
			return true
		}
	}

	return false
}

// NewMasterServer creates a new master server instance.
func NewMasterServer(cfg *config.MasterConfig, store storage.Store, logger *zap.Logger) (*MasterServer, error) {
	sysCfg, err := config.GetSystemConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load system config: %w", err)
	}
	s := &MasterServer{
		config:       cfg,
		store:        store,
		logger:       logger,
		agents:       make(map[string]*AgentConnection),
		shutdown:     make(chan struct{}),
		templatesDir: sysCfg.TemplatesDir(),
	}

	// Initialize KMS for encryption services (using the store for persistence)
	kms, kmsErr := security.NewKMS(context.Background(), store, logger)
	if kmsErr != nil {
		logger.Warn("Failed to create KMS, some features will be unavailable", zap.Error(kmsErr))
	} else {
		// Initialize KMS with a key if none exists
		if initErr := kms.Initialize(context.Background()); initErr != nil {
			logger.Warn("Failed to initialize KMS", zap.Error(initErr))
		} else {
			s.kms = kms
		}
	}

	// Initialize service layer (services that don't need KMS)
	s.userService = users.New(s.store)
	s.sessionService = sessions.New(s.store)
	s.apiKeyService = apikeys.New(s.store)
	s.projectService = projects.New(s.store)
	s.projectTypeService = projecttypes.New(s.store)
	s.deploymentService = deployments.New(s.store)
	s.agentService = agents.New(s.store)
	s.auditService = audit.New(s.store)
	s.hostKeyService = hostkeys.New(s.store)
	s.provisionService = provision.New(s.store)

	// Initialize KMS-dependent services
	if s.kms != nil {
		s.secretService = secrets.New(s.store, s.kms)
		s.settingsSvc = settings.New(s.store, s.kms)
		s.webhookService = webhooks.New(s.store, s.kms)

		// Initialize CA manager for agent certificate operations (now uses store)
		ca, caErr := security.NewCAManager(store, s.kms, logger)
		if caErr != nil {
			logger.Warn("Failed to create CA manager, agent gRPC will be unavailable", zap.Error(caErr))
		} else {
			// Initialize CA if none exists
			caConfig := security.DefaultCAConfig()
			if initErr := ca.Initialize(context.Background(), caConfig); initErr != nil {
				logger.Warn("Failed to initialize CA", zap.Error(initErr))
			} else {
				s.caManager = ca
				// Create the gRPC agent server with the CA manager
				s.agentServer = NewAgentServer(s.store, ca, s.logger)
				s.agentServer.SetServices(s.agentService, s.deploymentService)

				// Enable auto-registration in test mode
				if os.Getenv("VCDEPLOY_TEST_MODE") == "true" {
					s.agentServer.SetAllowAutoRegister(true)
				}

				logger.Info("Agent gRPC server initialized")
			}
		}
	}

	// Sync admin credentials from env or mark system as requiring setup
	if err := s.syncAdminCredentials(); err != nil {
		logger.Warn("Failed to sync admin credentials", zap.Error(err))
	}

	// Load templates from disk
	if err := s.loadTemplates(); err != nil {
		logger.Warn("Failed to load templates, using defaults", zap.Error(err))
	}

	// Initialize security middleware
	s.securityMiddleware = NewSecurityMiddleware(DefaultSecurityConfig())

	// Initialize CSP middleware with nonces enabled
	cspConfig := DefaultCSPConfigWithUnsafeInline()
	cspConfig.EnableNonces = true
	s.cspMiddleware = NewCSPMiddleware(cspConfig)

	// Initialize enforcement middleware
	s.enforcementMiddleware = NewEnforcementMiddleware(cfg, s.userService, logger)

	// Initialize log size enforcer
	s.logSizeEnforcer = NewLogSizeEnforcer(cfg.Logs.Deployment.MaxSizeMB, logger)

	// Initialize rate limiter only if enabled in config
	if cfg.API.RateLimit.Enabled {
		var err error
		rateLimitConfig := DefaultRateLimitConfig()
		if cfg.API.RateLimit.RequestsPerSecond > 0 {
			rateLimitConfig.RequestsPerSecond = cfg.API.RateLimit.RequestsPerSecond
		}
		if cfg.API.RateLimit.BurstSize > 0 {
			rateLimitConfig.BurstSize = cfg.API.RateLimit.BurstSize
		}
		s.rateLimiter, err = NewRateLimiter(nil, rateLimitConfig)
		if err != nil {
			logger.Warn("Failed to create rate limiter, continuing without it", zap.Error(err))
		}
	} else {
		logger.Info("Rate limiting disabled in configuration")
	}

	return s, nil
}

// SetCAManager sets the CA manager for agent certificate operations.
func (s *MasterServer) SetCAManager(ca *security.CAManager) {
	s.caManager = ca
	// Create the gRPC agent server with the CA manager
	s.agentServer = NewAgentServer(s.store, ca, s.logger)

	// Enable auto-registration in test mode
	if os.Getenv("VCDEPLOY_TEST_MODE") == "true" {
		s.agentServer.SetAllowAutoRegister(true)
	}
}

// SetKMS configures the KMS for secret encryption/decryption.
func (s *MasterServer) SetKMS(kms *security.KMS) {
	s.kms = kms
}

// SetWebhookHandler configures the webhook handler with KMS for secret decryption.
func (s *MasterServer) SetWebhookHandler(kms *security.KMS, processor webhookshandler.EventProcessor) {
	// Also set KMS on the server for secrets API
	s.kms = kms

	// Initialize KMS-dependent services
	s.secretService = secrets.New(s.store, kms)
	s.settingsSvc = settings.New(s.store, kms)
	s.webhookService = webhooks.New(s.store, kms)

	// Inject services into AgentServer if it exists
	if s.agentServer != nil {
		s.agentServer.SetServices(s.agentService, s.deploymentService)

		// Initialize alerting if enabled
		if s.config.Alerting.Enabled {
			thresholds := alerting.DefaultThresholds()
			if s.config.Alerting.DiskWarningPercent > 0 {
				thresholds.DiskWarningPercent = s.config.Alerting.DiskWarningPercent
			}
			if s.config.Alerting.DiskCriticalPercent > 0 {
				thresholds.DiskCriticalPercent = s.config.Alerting.DiskCriticalPercent
			}
			if s.config.Alerting.MemoryWarningPercent > 0 {
				thresholds.MemoryWarningPercent = s.config.Alerting.MemoryWarningPercent
			}
			if s.config.Alerting.CPUWarningPercent > 0 {
				thresholds.CPUWarningPercent = s.config.Alerting.CPUWarningPercent
			}
			if s.config.Alerting.DeploymentTimeout > 0 {
				thresholds.DeploymentTimeout = s.config.Alerting.DeploymentTimeout
			}
			if s.config.Alerting.AlertCooldown > 0 {
				thresholds.AlertCooldown = s.config.Alerting.AlertCooldown
			}

			notifyManager := createNotifyManager(s.config.Notifications, s.logger)
			s.alertManager = alerting.NewManager(notifyManager, s.logger, thresholds)
			s.agentServer.SetAlertManager(s.alertManager)
			s.logger.Info("System alerting enabled", zap.Any("thresholds", thresholds))
		}
	}

	secretStore := &webhookSecretStoreAdapter{
		store:  s.store,
		kms:    kms,
		logger: s.logger,
	}

	s.webhookHandler = &webhookHandlerAdapter{
		handler: webhookshandler.NewHandler(s.logger, secretStore, processor),
	}
}

// GetAgentServer returns the gRPC agent server.
func (s *MasterServer) GetAgentServer() *AgentServer {
	return s.agentServer
}

func (s *MasterServer) loadTemplates() error {
	s.templates = make(map[string]*template.Template)

	// Check if templates directory exists
	if _, err := os.Stat(s.templatesDir); os.IsNotExist(err) {
		// No templates directory - using defaults is fine
		return nil
	}

	// Find base template
	basePath := filepath.Join(s.templatesDir, "base.html")
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return fmt.Errorf("base template not found: %s", basePath)
	}

	// Load all page template files from the templates directory
	pattern := filepath.Join(s.templatesDir, "*.html")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("globbing templates: %w", err)
	}

	// Parse each page template together with base template
	// This creates separate template instances so {{define "content"}} doesn't conflict
	for _, file := range files {
		name := filepath.Base(file)
		if name == "base.html" {
			continue // Skip base template, it's included with each page
		}

		// Create a new template for this page, parsing base first then the page
		tmpl, err := template.New("").Funcs(s.templateFuncs()).ParseFiles(basePath, file)
		if err != nil {
			return fmt.Errorf("parsing template %s: %w", name, err)
		}
		s.templates[name] = tmpl
	}

	return nil
}

// syncAdminCredentials syncs admin credentials from environment variables or marks system as requiring setup.
// Environment variables:
//   - VCDEPLOY_ADMIN_PASSWORD: Admin password (required for env-based setup)
//   - VCDEPLOY_ADMIN_USERNAME: Admin username (default: "admin")
//   - VCDEPLOY_ADMIN_EMAIL: Admin email (default: "admin@localhost")
//
// Behavior:
//   - If VCDEPLOY_ADMIN_PASSWORD is set: create or update admin user on every startup
//   - If not set and no users exist: mark system as requiring setup (s.requiresSetup = true)
//   - If not set and users exist: normal operation
func (s *MasterServer) syncAdminCredentials() error {
	ctx := context.Background()

	// Read environment variables
	envPassword := os.Getenv("VCDEPLOY_ADMIN_PASSWORD")
	envUsername := os.Getenv("VCDEPLOY_ADMIN_USERNAME")
	envEmail := os.Getenv("VCDEPLOY_ADMIN_EMAIL")

	// Set defaults for username and email
	if envUsername == "" {
		envUsername = "admin"
	}
	if envEmail == "" {
		envEmail = "admin@localhost"
	}

	// If password is set via env, sync credentials
	if envPassword != "" {
		return s.syncAdminFromEnv(ctx, envUsername, envPassword, envEmail)
	}

	// No env password - check if any users exist
	count, err := s.userService.Count(ctx)
	if err != nil {
		return fmt.Errorf("counting users: %w", err)
	}

	if count == 0 {
		// No users and no env credentials - require setup
		s.requiresSetup = true
		s.logger.Info("No users found and VCDEPLOY_ADMIN_PASSWORD not set - setup required",
			zap.String("hint", "Visit /setup in browser or use 'vcdeploy admin' CLI command"))
	}

	return nil
}

// syncAdminFromEnv creates or updates admin user from environment variables.
func (s *MasterServer) syncAdminFromEnv(ctx context.Context, username, password, email string) error {
	// Try to find existing admin user by username
	existingUser, err := s.userService.GetByUsername(ctx, username)
	if err == nil && existingUser != nil {
		// User exists - update password and email
		existingUser.Email = email
		if err := s.userService.Update(ctx, existingUser); err != nil {
			return fmt.Errorf("updating admin email: %w", err)
		}

		// Update password (this handles hashing internally)
		if err := s.userService.UpdatePassword(ctx, existingUser.ID, password); err != nil {
			return fmt.Errorf("updating admin password: %w", err)
		}

		s.logger.Info("Admin credentials synced from environment variables",
			zap.String("username", username),
			zap.String("email", email))
		return nil
	}

	// User doesn't exist - create new admin
	user, err := s.userService.Create(ctx, username, password, email, "admin")
	if err != nil {
		return fmt.Errorf("creating admin user: %w", err)
	}

	// Don't set MustChangePassword for env-managed credentials (they're intentional)
	s.logger.Info("Admin user created from environment variables",
		zap.String("username", user.Username),
		zap.String("email", user.Email))

	return nil
}

func (s *MasterServer) templateFuncs() template.FuncMap {
	return template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"json": func(v interface{}) string {
			b, err := json.Marshal(v)
			if err != nil {
				return "{}"
			}
			return string(b)
		},
	}
}

// Start starts the master server.
func (s *MasterServer) Start(ctx context.Context) error {
	errCh := make(chan error, 2)

	s.wg.Go(func() {
		if err := s.startHTTP(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP server error", zap.Error(err))
			errCh <- err
		}
	})

	s.wg.Go(func() {
		if err := s.startGRPC(); err != nil {
			s.logger.Error("gRPC server error", zap.Error(err))
			errCh <- err
		}
	})

	s.wg.Go(func() {
		s.runBackgroundTasks(ctx)
	})

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	}
}

func (s *MasterServer) startHTTP() error {
	mux := http.NewServeMux()

	sysCfg, err := config.GetSystemConfig()
	if err != nil {
		return fmt.Errorf("failed to load system config: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(sysCfg.StaticDir()))))

	// Health check
	mux.HandleFunc("/api/v1/health", s.handleHealth)

	// Kubernetes-style health endpoints (no auth)
	mux.HandleFunc("/healthz", s.handleHealthzLive)
	mux.HandleFunc("/livez", s.handleHealthzLive)
	mux.HandleFunc("/readyz", s.handleHealthzReady)

	// Metrics endpoint (no auth - typically filtered at network level)
	mux.Handle("/metrics", promhttp.Handler())

	// Auth API (for programmatic login)
	mux.HandleFunc("/api/v1/auth/login", s.handleAPILogin)
	mux.HandleFunc("/api/v1/auth/me", s.withAuth(s.handleAPICurrentUser))

	// Stats endpoint
	mux.HandleFunc("/api/v1/stats", s.withAuth(s.handleStats))

	// Users API
	mux.HandleFunc("/api/v1/users", s.withAuth(s.handleUsers))
	mux.HandleFunc("/api/v1/users/", s.withAuth(s.handleUser))

	// Settings API
	mux.HandleFunc("/api/v1/settings/export", s.withAuth(s.handleSettingsExport))
	mux.HandleFunc("/api/v1/settings/import", s.withAuth(s.handleSettingsImport))
	mux.HandleFunc("/api/v1/settings/", s.withAuth(s.handleSettingsCategory))

	// Projects API
	mux.HandleFunc("/api/v1/projects", s.withAuth(s.handleProjectsAPI))
	mux.HandleFunc("/api/v1/projects/", s.withAuth(s.handleProjectAPI))

	// Deployments API
	mux.HandleFunc("/api/v1/deployments", s.withAuth(s.handleDeploymentsAPI))
	mux.HandleFunc("/api/v1/deployments/", s.withAuth(s.handleDeploymentAPI))

	// Agents API
	mux.HandleFunc("/api/v1/agents", s.withAuth(s.handleAgentsAPI))
	mux.HandleFunc("/api/v1/agents/", s.withAuth(s.handleAgentAPI))

	// Secrets API
	mux.HandleFunc("/api/v1/secrets", s.withAuth(s.handleSecrets))

	// Project Types API
	mux.HandleFunc("/api/v1/project-types", s.withAuth(s.handleProjectTypes))
	mux.HandleFunc("/api/v1/project-types/", s.withAuth(s.handleProjectType))

	// API Keys API (canonical kebab-case only)
	mux.HandleFunc("/api/v1/api-keys", s.withAuth(s.handleAPIKeys))
	mux.HandleFunc("/api/v1/api-keys/", s.withAuth(s.handleAPIKey))

	// Audit API
	mux.HandleFunc("/api/v1/audit", s.withAuth(s.handleAuditLogs))

	// Host Keys API (canonical kebab-case only)
	mux.HandleFunc("/api/v1/host-keys", s.withAuth(s.handleHostKeys))
	mux.HandleFunc("/api/v1/host-keys/", s.withAuth(s.handleHostKey))

	// Jump Servers API (canonical kebab-case only)
	mux.HandleFunc("/api/v1/jump-servers", s.withAuth(s.handleJumpServers))
	mux.HandleFunc("/api/v1/jump-servers/", s.withAuth(s.handleJumpServer))

	// Blocked IPs API
	mux.HandleFunc("/api/v1/blocked", s.withAuth(s.handleBlockedIPs))
	mux.HandleFunc("/api/v1/blocked/", s.withAuth(s.handleBlockedIP))

	// Provision API
	mux.HandleFunc("/api/v1/provision", s.withAuth(s.handleProvisionJobs))
	mux.HandleFunc("/api/v1/provision/", s.withAuth(s.handleProvisionJob))

	// Security - Certificates API
	mux.HandleFunc("/api/v1/certificates/agents", s.withAuth(s.handleCertificates))
	mux.HandleFunc("/api/v1/certificates/agents/", s.withAuth(s.handleCertificates))
	mux.HandleFunc("/api/v1/certificates/cas", s.withAuth(s.handleCAs))
	mux.HandleFunc("/api/v1/certificates/cas/", s.withAuth(s.handleCAs))
	mux.HandleFunc("/api/v1/certificates/server", s.withAuth(s.handleServerCertificate))
	mux.HandleFunc("/api/v1/certificates/server/", s.withAuth(s.handleServerCertificate))
	mux.HandleFunc("/api/v1/certificates/audit", s.withAuth(s.handleCertAudit))

	// Security - Credentials API
	mux.HandleFunc("/api/v1/credentials", s.withAuth(s.handleCredentials))
	mux.HandleFunc("/api/v1/credentials/", s.withAuth(s.handleCredentials))

	// Security - SSH Keys API
	mux.HandleFunc("/api/v1/ssh-keys", s.withAuth(s.handleSSHKeys))
	mux.HandleFunc("/api/v1/ssh-keys/", s.withAuth(s.handleSSHKeys))

	// Security - Agent Provisioning API
	mux.HandleFunc("/api/v1/agents/provision", s.withAuth(s.handleProvisionAgent))
	mux.HandleFunc("/api/v1/agents/provision/", s.withAuth(s.handleProvisionAgent))

	// Agent Binaries API
	mux.HandleFunc("/api/v1/binaries", s.withAuth(s.handleAgentBinaries))
	mux.HandleFunc("/api/v1/binaries/latest", s.withAuth(s.handleAgentBinaryLatest))
	mux.HandleFunc("/api/v1/binaries/", s.withAuth(s.handleAgentBinary))

	// Health Check Configuration API
	mux.HandleFunc("/api/v1/health-checks", s.withAuth(s.handleHealthCheckConfigs))
	mux.HandleFunc("/api/v1/health-checks/global", s.withAuth(s.handleGlobalHealthCheck))
	mux.HandleFunc("/api/v1/health-checks/", s.withAuth(s.handleHealthCheckConfig))

	// Rollback Records API
	mux.HandleFunc("/api/v1/rollbacks", s.withAuth(s.handleRollbackRecords))
	mux.HandleFunc("/api/v1/rollbacks/", s.withAuth(s.handleRollbackRecord))

	// Webhooks
	mux.HandleFunc("/webhook/github/", s.handleGitHubWebhook)
	mux.HandleFunc("/webhook/gitlab/", s.handleGitLabWebhook)
	mux.HandleFunc("/webhook/bitbucket/", s.handleBitbucketWebhook)

	// UI Routes
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/setup", s.handleSetup)
	mux.HandleFunc("/change-password", s.withUIAuth(s.handleChangePassword))
	mux.HandleFunc("/dashboard", s.withUIAuth(s.handleDashboard))
	mux.HandleFunc("/projects", s.withUIAuth(s.handleProjectsUI))
	mux.HandleFunc("/deployments", s.withUIAuth(s.handleDeploymentsUI))
	mux.HandleFunc("/agents", s.withUIAuth(s.handleAgentsUI))
	mux.HandleFunc("/secrets", s.withUIAuth(s.handleSecretsUI))
	mux.HandleFunc("/project-types", s.withUIAuth(s.handleProjectTypesUI))
	mux.HandleFunc("/audit", s.withUIAuth(s.handleAuditUI))
	mux.HandleFunc("/api-keys", s.withUIAuth(s.handleAPIKeysUI))
	mux.HandleFunc("/settings", s.withUIAuth(s.handleSettingsUI))

	addr := s.config.Server.Listen
	if addr == "" {
		addr = ":8080"
	}
	s.logger.Info("Starting HTTP server", zap.String("addr", addr))

	// Build middleware chain: otel -> request ID -> logging -> CSP -> security headers -> rate limiting -> handler
	var handler http.Handler = mux
	handler = s.loggingMiddleware(handler)
	handler = s.requestIDMiddleware(handler) // Add request ID first (outermost)

	// Add OpenTelemetry HTTP instrumentation (outermost for full request tracing)
	handler = otelhttp.NewHandler(handler, "vcdeploy-http",
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
	)

	// Add CSP middleware (must be before security headers to set CSP header)
	if s.cspMiddleware != nil {
		handler = s.cspMiddleware.Handler(handler)
	}

	// Add security headers middleware
	if s.securityMiddleware != nil {
		handler = s.securityMiddleware.HeadersOnlyMiddleware(handler)
	}

	// Add rate limiting (if enabled)
	if s.rateLimiter != nil {
		handler = s.rateLimiter.Middleware(handler)
	}

	// Add setup required middleware (redirects to /setup when system needs initial configuration)
	handler = s.setupRequiredMiddleware(handler)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if s.config.Server.TLS.Enabled {
		return s.httpServer.ListenAndServeTLS(s.config.Server.TLS.Cert, s.config.Server.TLS.Key)
	}
	return s.httpServer.ListenAndServe()
}

func (s *MasterServer) startGRPC() error {
	addr := s.config.GRPC.Listen
	if addr == "" {
		addr = ":9090"
	}
	s.logger.Info("Starting gRPC server", zap.String("addr", addr))

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	var opts []grpc.ServerOption

	// Add OpenTelemetry gRPC instrumentation
	opts = append(opts, grpc.StatsHandler(otelgrpc.NewServerHandler()))

	if s.config.Server.TLS.Enabled {
		cert, err := tls.LoadX509KeyPair(s.config.Server.TLS.Cert, s.config.Server.TLS.Key)
		if err != nil {
			return fmt.Errorf("loading TLS cert: %w", err)
		}
		creds := credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		})
		opts = append(opts, grpc.Creds(creds))
	}

	s.grpcServer = grpc.NewServer(opts...)

	// Register agent service if available
	if s.agentServer != nil {
		proto.RegisterAgentServiceServer(s.grpcServer, s.agentServer)
		s.logger.Info("Registered AgentService gRPC handler")
	}

	return s.grpcServer.Serve(lis)
}

func (s *MasterServer) runBackgroundTasks(ctx context.Context) {
	// Create and start the cleanup task
	cleanupConfig := DefaultCleanupConfig()
	cleanupConfig.Interval = time.Minute // Check every minute for demo, but actual cleanup runs less often

	cleanupServices := CleanupServices{
		SessionService:    s.sessionService,
		DeploymentService: s.deploymentService,
		AuditService:      s.auditService,
		AgentService:      s.agentService,
		APIKeyService:     s.apiKeyService,
		WebhookService:    s.webhookService,
	}
	cleanupTask := NewCleanupTask(cleanupServices, s.logger, cleanupConfig)
	cleanupTask.Start()

	// Set up scheduled jobs
	s.setupScheduledJobs()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	defer cleanupTask.Stop()
	defer func() {
		if s.scheduler != nil {
			s.scheduler.Stop()
		}
	}()

	for {
		select {
		case <-ticker.C:
			s.checkAgentHealth()
			s.processScheduledDeployments()
		case <-s.shutdown:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *MasterServer) checkAgentHealth() {
	s.agentsMu.Lock()
	defer s.agentsMu.Unlock()

	staleThreshold := time.Now().Add(-2 * time.Minute)
	for id, agent := range s.agents {
		if agent.LastPing.Before(staleThreshold) {
			s.logger.Warn("Agent stale", zap.String("agent", id))
			agent.Status = "stale"
		}
	}
}

func (s *MasterServer) processScheduledDeployments() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get pending scheduled deployments that are due
	deployments, err := s.deploymentService.ListPendingScheduled(ctx)
	if err != nil {
		s.logger.Error("Failed to list scheduled deployments", zap.Error(err))
		return
	}

	for _, d := range deployments {
		s.logger.Info("Processing scheduled deployment",
			zap.String("id", d.ID),
			zap.String("project", d.Project),
			zap.Time("scheduledAt", d.ScheduledAt),
		)

		// Update deployment status to running
		deployment := &storage.DeploymentRecord{
			ID:          d.ID,
			Project:     d.Project,
			Target:      d.Target,
			Branch:      d.Branch,
			Status:      "running",
			TriggeredBy: d.ScheduledBy,
			StartedAt:   time.Now(),
		}
		if err := s.deploymentService.Update(ctx, deployment); err != nil {
			s.logger.Error("Failed to update scheduled deployment", zap.Error(err))
			continue
		}

		// Trigger actual deployment via agent
		if err := s.triggerDeploymentOnAgent(ctx, deployment); err != nil {
			s.logger.Error("Failed to trigger deployment on agent",
				zap.String("deployment_id", d.ID),
				zap.Error(err),
			)
			// Mark deployment as failed
			deployment.Status = "failed"
			now := time.Now()
			deployment.CompletedAt = &now
			_ = s.deploymentService.Update(ctx, deployment)
			continue
		}

		// Note: actual completion status will be updated when agent reports back
		s.logger.Info("Deployment triggered on agent", zap.String("deployment_id", d.ID))
	}
}

// setupScheduledJobs initializes and starts the scheduler with configured jobs.
func (s *MasterServer) setupScheduledJobs() {
	s.scheduler = scheduler.New(s.logger)

	// Key rotation job
	if s.config.Security.KeyRotation.Enabled {
		keyRotationJob := scheduler.NewKeyRotationJob(s.kms, &s.config.Security.KeyRotation, s.logger)
		schedule := &scheduler.IntervalSchedule{Interval: s.config.Security.KeyRotation.Interval}
		if err := s.scheduler.AddJob(keyRotationJob, schedule); err != nil {
			s.logger.Error("Failed to add key rotation job", zap.Error(err))
		}
	}

	// Database backup job
	if s.config.Backup.Database.Enabled {
		dbBackupJob := scheduler.NewDatabaseBackupJob(s.store, &s.config.Backup.Database, s.logger)
		schedule := &scheduler.IntervalSchedule{Interval: s.config.Backup.Database.Interval}
		if err := s.scheduler.AddJob(dbBackupJob, schedule); err != nil {
			s.logger.Error("Failed to add database backup job", zap.Error(err))
		}
	}

	// Audit export job
	if s.config.Logs.Audit.Export.Enabled {
		auditExportJob := scheduler.NewAuditExportJob(s.store, &s.config.Logs.Audit.Export, s.logger)
		var schedule scheduler.Schedule
		if s.config.Logs.Audit.Export.Schedule != "" {
			var err error
			schedule, err = scheduler.ParseSchedule(s.config.Logs.Audit.Export.Schedule)
			if err != nil {
				s.logger.Error("Invalid audit export schedule, using daily default", zap.Error(err))
				schedule = &scheduler.DailySchedule{Hour: 2, Minute: 0}
			}
		} else {
			schedule = &scheduler.DailySchedule{Hour: 2, Minute: 0}
		}
		if err := s.scheduler.AddJob(auditExportJob, schedule); err != nil {
			s.logger.Error("Failed to add audit export job", zap.Error(err))
		}
	}

	// Log rotation job
	if s.config.Logs.Rotation.Schedule != "" {
		rotationCfg := scheduler.LogRotationConfig{
			Enabled:   true,
			LogDir:    "/var/log/vcdeploy",
			MaxSizeMB: s.config.Logs.Deployment.MaxSizeMB,
			Retention: s.config.Logs.Deployment.Retention,
		}
		logRotationJob := scheduler.NewLogRotationJob(rotationCfg, s.logger)

		schedule, err := scheduler.ParseSchedule(s.config.Logs.Rotation.Schedule)
		if err != nil {
			s.logger.Error("Invalid log rotation schedule, using daily default", zap.Error(err))
			schedule = &scheduler.DailySchedule{Hour: 3, Minute: 0}
		}
		if err := s.scheduler.AddJob(logRotationJob, schedule); err != nil {
			s.logger.Error("Failed to add log rotation job", zap.Error(err))
		}
	}

	s.scheduler.Start()
	s.logger.Info("Scheduled jobs initialized")
}

// triggerDeploymentOnAgent sends a deployment command to the appropriate agent.
func (s *MasterServer) triggerDeploymentOnAgent(ctx context.Context, deployment *storage.DeploymentRecord) error {
	// Get project details
	project, err := s.projectService.GetByName(ctx, deployment.Project)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	// Determine which agent to use
	agentID := deployment.Target
	if agentID == "" {
		// Use default agent or first connected agent
		if s.agentServer == nil {
			return fmt.Errorf("no agent server configured")
		}
		connectedAgents := s.agentServer.GetConnectedAgents()
		if len(connectedAgents) == 0 {
			return fmt.Errorf("no agents connected")
		}
		agentID = connectedAgents[0]
	}

	// Check if agent is connected
	if s.agentServer == nil || !s.agentServer.IsAgentConnected(agentID) {
		return fmt.Errorf("agent %s not connected", agentID)
	}

	// Build deployment command
	deployCmd := &proto.DeployCommand{
		DeploymentId: deployment.ID,
		Project:      deployment.Project,
		Target:       deployment.Target,
		Repository:   project.Repository,
		Branch:       deployment.Branch,
		Path:         project.DeployPath,
		Settings: &proto.DeploymentSettings{
			Strategy:     "rolling",
			KeepReleases: 5,
		},
	}

	// Send command to agent
	if err := s.agentServer.SendDeployCommand(agentID, deployCmd); err != nil {
		return fmt.Errorf("send deploy command: %w", err)
	}

	return nil
}

// Shutdown gracefully stops the server.
// Shutdown is idempotent and can be called multiple times safely.
func (s *MasterServer) Shutdown(ctx context.Context) error {
	s.shutdownOnce.Do(func() {
		s.logger.Info("Shutting down server")
		close(s.shutdown)

		// Stop background goroutines in middleware and rate limiter
		if s.securityMiddleware != nil {
			s.securityMiddleware.Stop()
		}
		if s.rateLimiter != nil {
			s.rateLimiter.Stop()
		}

		if s.httpServer != nil {
			_ = s.httpServer.Shutdown(ctx)
		}
		if s.grpcServer != nil {
			s.grpcServer.GracefulStop()
		}
	})

	s.wg.Wait()
	return nil
}

// Stop is an alias for Shutdown
func (s *MasterServer) Stop(ctx context.Context) error {
	return s.Shutdown(ctx)
}

// RequiresSetup returns whether the server requires initial setup.
// This is true when no users exist and VCDEPLOY_ADMIN_PASSWORD was not set.
func (s *MasterServer) RequiresSetup() bool {
	return s.requiresSetup
}

// SetRequiresSetup sets the requiresSetup state. Used for testing.
func (s *MasterServer) SetRequiresSetup(v bool) {
	s.requiresSetup = v
}

// Request ID context key and header
type contextKeyType string

const (
	contextKeyRequestID contextKeyType = "request_id"
	RequestIDHeader     string         = "X-Request-ID"
)

// GetRequestID retrieves the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(contextKeyRequestID).(string); ok {
		return id
	}
	return ""
}

// requestIDMiddleware adds a unique request ID to each request.
// It will use an existing X-Request-ID header if present, otherwise generate a new one.
func (s *MasterServer) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			requestID = generateRequestID()
		}

		// Add to response header
		w.Header().Set(RequestIDHeader, requestID)

		// Add to context
		ctx := context.WithValue(r.Context(), contextKeyRequestID, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateRequestID creates a unique request ID using timestamp and random suffix.
func generateRequestID() string {
	// Use timestamp in microseconds + random suffix for uniqueness
	// Format: timestamp_random (e.g., "1706529600123456_a1b2c3d4")
	timestamp := time.Now().UnixMicro()
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		// C3 FIX: Fallback to timestamp-only ID if crypto/rand fails
		// This is extremely unlikely but we must handle it to avoid predictable IDs
		return fmt.Sprintf("%d_%d", timestamp, time.Now().UnixNano()%100000000)
	}
	return fmt.Sprintf("%d_%x", timestamp, random)
}

func (s *MasterServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip metrics for /metrics endpoint to avoid recursive counting
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		metrics.HTTPRequestsInFlight.Inc()
		defer metrics.HTTPRequestsInFlight.Dec()

		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		statusStr := strconv.Itoa(wrapped.status)

		// Normalize path for metrics (remove IDs to reduce cardinality)
		path := normalizePath(r.URL.Path)

		// Record metrics
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, path, statusStr).Observe(duration.Seconds())
		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, path, statusStr).Inc()

		// Include request ID in logs for correlation
		requestID := GetRequestID(r.Context())
		s.logger.Debug("HTTP request",
			zap.String("request_id", requestID),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", wrapped.status),
			zap.Duration("duration", duration),
		)
	})
}

// normalizePath normalizes URL paths for metrics by replacing IDs with placeholders.
// This reduces metric cardinality while preserving useful grouping.
func normalizePath(path string) string {
	// Simple normalization: replace numeric segments and UUIDs with :id
	parts := strings.Split(path, "/")
	for i, part := range parts {
		// Replace numeric IDs
		if _, err := strconv.ParseInt(part, 10, 64); err == nil {
			parts[i] = ":id"
			continue
		}
		// Replace UUIDs (simple check for 36-char strings with dashes)
		if len(part) == 36 && strings.Count(part, "-") == 4 {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func (s *MasterServer) withAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// First try Authorization header (Bearer token)
		auth := r.Header.Get("Authorization")
		if auth != "" && strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")

			// First try as API key
			apiKey, userID, err := s.validateAPIKey(r.Context(), token)
			if err == nil {
				// Valid API key - add user ID and API key to context
				ctx := context.WithValue(r.Context(), contextKeyUserID, userID)
				ctx = WithAPIKeyContext(ctx, apiKey)
				handler(w, r.WithContext(ctx))
				return
			}
			s.logger.Debug("API key validation failed, trying session", zap.Error(err))

			// If not an API key, try as session token (for API login flow)
			userID, err = s.validateSession(r.Context(), token)
			if err == nil {
				ctx := context.WithValue(r.Context(), contextKeyUserID, userID)
				handler(w, r.WithContext(ctx))
				return
			}
			s.logger.Debug("Session token validation failed", zap.Error(err))

			s.jsonError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		// Fall back to session cookie (for htmx requests from browser)
		cookie, err := r.Cookie("session")
		if err == nil {
			userID, err := s.validateSession(r.Context(), cookie.Value)
			if err == nil {
				ctx := context.WithValue(r.Context(), contextKeyUserID, userID)
				handler(w, r.WithContext(ctx))
				return
			}
			s.logger.Debug("Session validation failed", zap.Error(err))
		}

		s.jsonError(w, http.StatusUnauthorized, "Unauthorized")
	}
}

func (s *MasterServer) withUIAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		userID, err := s.validateSession(r.Context(), cookie.Value)
		if err != nil {
			s.logger.Debug("Session validation failed", zap.Error(err))
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Add user ID to context for downstream handlers
		ctx := context.WithValue(r.Context(), contextKeyUserID, userID)

		// Also load and set the full user object for handlers that need it
		user, err := s.userService.GetByID(ctx, userID)
		if err != nil {
			s.logger.Debug("Failed to load user", zap.Error(err))
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx = WithUserContext(ctx, user)

		handler(w, r.WithContext(ctx))
	}
}

// Context key for user ID
type contextKey string

const contextKeyUserID contextKey = "userID"

// GetUserIDFromContext extracts the user ID from the request context.
func GetUserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(contextKeyUserID).(int64)
	return userID, ok
}

// validateAPIKey validates an API key and returns the API key object and user ID if valid.
func (s *MasterServer) validateAPIKey(ctx context.Context, key string) (*storage.APIKey, int64, error) {
	if key == "" {
		return nil, 0, fmt.Errorf("empty API key")
	}

	// Look up using API key service
	apiKey, err := s.apiKeyService.GetByRawKey(ctx, key)
	if services.IsNotFound(err) {
		return nil, 0, fmt.Errorf("API key not found")
	}
	if err != nil {
		return nil, 0, fmt.Errorf("database error: %w", err)
	}

	// Check if valid
	if !apiKey.IsValid() {
		return nil, 0, fmt.Errorf("API key expired or revoked")
	}

	// Update last used timestamp asynchronously
	// With MemoryStore, this is non-blocking as updates go to memory immediately
	// and are batched to SQLite in the background
	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.apiKeyService.UpdateUsage(updateCtx, apiKey.ID); err != nil {
			// Don't fail the request if usage update fails - it's not critical
			s.logger.Debug("Failed to update API key usage", zap.Error(err))
		}
	}()

	return apiKey, apiKey.UserID, nil
}

// validateSession validates a session token and returns the user ID if valid.
func (s *MasterServer) validateSession(ctx context.Context, token string) (int64, error) {
	if token == "" {
		return 0, fmt.Errorf("empty session token")
	}

	// Look up using session service
	session, err := s.sessionService.GetByToken(ctx, token)
	if services.IsNotFound(err) {
		return 0, fmt.Errorf("session not found or expired")
	}
	if err != nil {
		return 0, fmt.Errorf("database error: %w", err)
	}

	return session.UserID, nil
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (s *MasterServer) handleAgents(w http.ResponseWriter, _ *http.Request) {
	s.agentsMu.RLock()
	agents := make([]map[string]interface{}, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, map[string]interface{}{
			"id":          a.ID,
			"name":        a.Name,
			"tags":        a.Tags,
			"status":      a.Status,
			"connectedAt": a.ConnectedAt,
			"lastPing":    a.LastPing,
		})
	}
	s.agentsMu.RUnlock()
	s.jsonResponse(w, agents)
}

// writeJSON encodes data as JSON and writes to the response.
// H12 FIX: Properly handles encoder errors instead of ignoring them.
func (s *MasterServer) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode JSON response",
			zap.Error(err),
			zap.Any("data", data),
		)
	}
}

func (s *MasterServer) jsonResponse(w http.ResponseWriter, data interface{}) {
	s.writeJSON(w, http.StatusOK, data)
}
