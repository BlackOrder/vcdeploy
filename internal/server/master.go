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
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// MasterServer is the main daemon server.
type MasterServer struct {
	config     *config.MasterConfig
	db         *storage.DB
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

// webhookSecretStoreAdapter implements webhooks.SecretStore using the DB.
type webhookSecretStoreAdapter struct {
	db     *storage.DB
	kms    *security.KMS
	logger *zap.Logger
}

func (a *webhookSecretStoreAdapter) GetWebhookSecret(projectID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Look up project by name (projectID in URL is the project name/slug)
	project, err := a.db.GetProjectByName(ctx, projectID)
	if errors.Is(err, storage.ErrNotFound) {
		return "", fmt.Errorf("project not found: %s", projectID)
	}
	if err != nil {
		return "", fmt.Errorf("looking up project: %w", err)
	}

	// Try each provider (github, gitlab, bitbucket)
	providers := []string{"github", "gitlab", "bitbucket"}
	for _, provider := range providers {
		webhook, err := a.db.GetProjectWebhook(ctx, project.ID, provider)
		if errors.Is(err, storage.ErrNotFound) {
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
	project, err := a.db.GetProjectByName(ctx, projectID)
	if err != nil {
		return false // If we can't find the project, don't require secret
	}

	// Check each provider for require_secret setting
	providers := []string{"github", "gitlab", "bitbucket"}
	for _, provider := range providers {
		webhook, err := a.db.GetProjectWebhook(ctx, project.ID, provider)
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
func NewMasterServer(cfg *config.MasterConfig, db *storage.DB, logger *zap.Logger) (*MasterServer, error) {
	sysCfg := config.MustGetSystemConfig()
	s := &MasterServer{
		config:       cfg,
		db:           db,
		logger:       logger,
		agents:       make(map[string]*AgentConnection),
		shutdown:     make(chan struct{}),
		templatesDir: sysCfg.TemplatesDir(),
	}

	// Initialize KMS for encryption services
	kms, kmsErr := security.NewKMS(db.Conn(), logger)
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
	s.userService = users.New(s.db)
	s.sessionService = sessions.New(s.db)
	s.apiKeyService = apikeys.New(s.db)
	s.projectService = projects.New(s.db)
	s.projectTypeService = projecttypes.New(s.db)
	s.deploymentService = deployments.New(s.db)
	s.agentService = agents.New(s.db)
	s.auditService = audit.New(s.db)
	s.hostKeyService = hostkeys.New(s.db)
	s.provisionService = provision.New(s.db)

	// Initialize KMS-dependent services
	if s.kms != nil {
		s.secretService = secrets.New(s.db, s.kms)
		s.settingsSvc = settings.New(s.db, s.kms)
		s.webhookService = webhooks.New(s.db, s.kms)
	}

	// Seed default admin user if no users exist
	if err := s.seedDefaultAdmin(); err != nil {
		logger.Warn("Failed to seed default admin", zap.Error(err))
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

	// Initialize rate limiter
	var err error
	s.rateLimiter, err = NewRateLimiter(nil, DefaultRateLimitConfig())
	if err != nil {
		logger.Warn("Failed to create rate limiter, continuing without it", zap.Error(err))
	}

	return s, nil
}

// SetCAManager sets the CA manager for agent certificate operations.
func (s *MasterServer) SetCAManager(ca *security.CAManager) {
	s.caManager = ca
	// Create the gRPC agent server with the CA manager
	s.agentServer = NewAgentServer(s.db, ca, s.logger)
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
	s.secretService = secrets.New(s.db, kms)
	s.settingsSvc = settings.New(s.db, kms)
	s.webhookService = webhooks.New(s.db, kms)

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

			// TODO: Create notifier from config when notification system is integrated
			s.alertManager = alerting.NewManager(nil, s.logger, thresholds)
			s.agentServer.SetAlertManager(s.alertManager)
			s.logger.Info("System alerting enabled", zap.Any("thresholds", thresholds))
		}
	}

	secretStore := &webhookSecretStoreAdapter{
		db:     s.db,
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

// seedDefaultAdmin creates a default admin user if no users exist.
func (s *MasterServer) seedDefaultAdmin() error {
	ctx := context.Background()

	// Check if any users exist
	users, err := s.userService.List(ctx)
	if err != nil {
		return fmt.Errorf("listing users: %w", err)
	}

	if len(users) > 0 {
		return nil // Users already exist, no need to seed
	}

	// Create default admin user with simple default credentials
	// User MUST change password on first login
	defaultPassword := "admin"
	user, err := s.userService.Create(ctx, "admin", defaultPassword, "admin@localhost", "admin")
	if err != nil {
		return fmt.Errorf("creating default admin: %w", err)
	}

	// Set MustChangePassword flag - admin must change password on first login
	user.MustChangePassword = true
	if err := s.userService.Update(ctx, user); err != nil {
		return fmt.Errorf("setting MustChangePassword flag: %w", err)
	}

	s.logger.Info("Created default admin user",
		zap.String("username", user.Username),
		zap.String("note", "Password change required on first login"))

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

	sysCfg := config.MustGetSystemConfig()
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

	// API Keys API
	mux.HandleFunc("/api/v1/apikeys", s.withAuth(s.handleAPIKeys))
	mux.HandleFunc("/api/v1/apikeys/", s.withAuth(s.handleAPIKey))

	// Audit API
	mux.HandleFunc("/api/v1/audit", s.withAuth(s.handleAuditLogs))

	// Host Keys API
	mux.HandleFunc("/api/v1/hostkeys", s.withAuth(s.handleHostKeys))
	mux.HandleFunc("/api/v1/hostkeys/", s.withAuth(s.handleHostKey))

	// Jump Servers API
	mux.HandleFunc("/api/v1/jumpservers", s.withAuth(s.handleJumpServers))
	mux.HandleFunc("/api/v1/jumpservers/", s.withAuth(s.handleJumpServer))

	// Blocked IPs API
	mux.HandleFunc("/api/v1/blocked", s.withAuth(s.handleBlockedIPs))
	mux.HandleFunc("/api/v1/blocked/", s.withAuth(s.handleBlockedIP))

	// Provision API
	mux.HandleFunc("/api/v1/provision", s.withAuth(s.handleProvisionJobs))
	mux.HandleFunc("/api/v1/provision/", s.withAuth(s.handleProvisionJob))

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
	mux.HandleFunc("/change-password", s.withUIAuth(s.handleChangePassword))
	mux.HandleFunc("/dashboard", s.withUIAuth(s.handleDashboard))
	mux.HandleFunc("/projects", s.withUIAuth(s.handleProjectsUI))
	mux.HandleFunc("/deployments", s.withUIAuth(s.handleDeploymentsUI))
	mux.HandleFunc("/agents", s.withUIAuth(s.handleAgentsUI))
	mux.HandleFunc("/secrets", s.withUIAuth(s.handleSecretsUI))
	mux.HandleFunc("/project-types", s.withUIAuth(s.handleProjectTypesUI))
	mux.HandleFunc("/audit", s.withUIAuth(s.handleAuditUI))
	mux.HandleFunc("/apikeys", s.withUIAuth(s.handleAPIKeysUI))
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
		dbBackupJob := scheduler.NewDatabaseBackupJob(s.db, &s.config.Backup.Database, s.logger)
		schedule := &scheduler.IntervalSchedule{Interval: s.config.Backup.Database.Interval}
		if err := s.scheduler.AddJob(dbBackupJob, schedule); err != nil {
			s.logger.Error("Failed to add database backup job", zap.Error(err))
		}
	}

	// Audit export job
	if s.config.Logs.Audit.Export.Enabled {
		auditExportJob := scheduler.NewAuditExportJob(s.db, &s.config.Logs.Audit.Export, s.logger)
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
	_, _ = rand.Read(random)
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
		// First try API key in Authorization header
		auth := r.Header.Get("Authorization")
		if auth != "" && strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			apiKey, userID, err := s.validateAPIKey(r.Context(), token)
			if err != nil {
				s.logger.Debug("API key validation failed", zap.Error(err))
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			// Add user ID and API key to context for downstream handlers
			ctx := context.WithValue(r.Context(), contextKeyUserID, userID)
			ctx = WithAPIKeyContext(ctx, apiKey)
			handler(w, r.WithContext(ctx))
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

		http.Error(w, "Unauthorized", http.StatusUnauthorized)
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
	if errors.Is(err, storage.ErrNotFound) {
		return nil, 0, fmt.Errorf("API key not found")
	}
	if err != nil {
		return nil, 0, fmt.Errorf("database error: %w", err)
	}

	// Check if valid
	if !apiKey.IsValid() {
		return nil, 0, fmt.Errorf("API key expired or revoked")
	}

	// Update last used timestamp (async to not slow down request)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("panic in API key usage update", zap.Any("panic", r))
			}
		}()
		if err := s.apiKeyService.UpdateUsage(context.Background(), apiKey.ID); err != nil {
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
	if errors.Is(err, storage.ErrNotFound) {
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

func (s *MasterServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	health := s.buildDetailedHealth(ctx)

	statusCode := http.StatusOK
	if health.Status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(health)
}

// HealthStatus represents detailed health information.
type HealthStatus struct {
	Status    string                 `json:"status"` // "healthy", "degraded", "unhealthy"
	Checks    map[string]CheckResult `json:"checks"`
	Version   string                 `json:"version,omitempty"`
	Uptime    string                 `json:"uptime,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// CheckResult represents the result of a single health check.
type CheckResult struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// handleHealthzLive handles /healthz and /livez (Kubernetes liveness probe).
// Returns 200 if the process is alive.
func (s *MasterServer) handleHealthzLive(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleHealthzReady handles /readyz (Kubernetes readiness probe).
// Returns 200 if the server can serve traffic, 503 otherwise.
func (s *MasterServer) handleHealthzReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Check database connectivity
	if err := s.db.Conn().PingContext(ctx); err != nil {
		s.logger.Warn("Readiness check failed: database not ready", zap.Error(err))
		http.Error(w, "database not ready", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// buildDetailedHealth builds a detailed health status.
func (s *MasterServer) buildDetailedHealth(ctx context.Context) HealthStatus {
	health := HealthStatus{
		Status:    "healthy",
		Checks:    make(map[string]CheckResult),
		Timestamp: time.Now().UTC(),
	}

	// Database check
	dbStart := time.Now()
	if err := s.db.Conn().PingContext(ctx); err != nil {
		health.Status = "unhealthy"
		health.Checks["database"] = CheckResult{
			Status:  "unhealthy",
			Message: err.Error(),
		}
	} else {
		health.Checks["database"] = CheckResult{
			Status:  "healthy",
			Latency: time.Since(dbStart).Round(time.Microsecond).String(),
		}
	}

	// gRPC server check
	if s.grpcServer != nil {
		health.Checks["grpc"] = CheckResult{Status: "healthy"}
	} else {
		health.Checks["grpc"] = CheckResult{
			Status:  "degraded",
			Message: "gRPC server not initialized",
		}
		if health.Status == "healthy" {
			health.Status = "degraded"
		}
	}

	// Agent connectivity summary
	s.agentsMu.RLock()
	connectedCount := 0
	totalCount := len(s.agents)
	for _, agent := range s.agents {
		if agent.Status == "connected" {
			connectedCount++
		}
	}
	s.agentsMu.RUnlock()

	agentStatus := "healthy"
	if totalCount > 0 && connectedCount == 0 {
		agentStatus = "degraded"
		if health.Status == "healthy" {
			health.Status = "degraded"
		}
	}
	health.Checks["agents"] = CheckResult{
		Status:  agentStatus,
		Message: fmt.Sprintf("%d/%d connected", connectedCount, totalCount),
	}

	return health
}

func (s *MasterServer) handleAgents(w http.ResponseWriter, r *http.Request) {
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

// handleSecrets handles GET/POST for /api/v1/secrets and per-project secrets.
func (s *MasterServer) handleSecrets(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Parse query params for filtering
	projectFilter := r.URL.Query().Get("project")
	scopeFilter := r.URL.Query().Get("scope")

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		if s.secretService == nil {
			s.jsonError(w, http.StatusInternalServerError, "Secret service not configured")
			return
		}

		var secretsList []services.SecretMetadata
		var err error

		if projectFilter != "" && scopeFilter != "" {
			// Get secrets for specific project/scope
			secretsList, err = s.secretService.List(ctx, projectFilter, scopeFilter)
		} else if projectFilter != "" {
			// Get all secrets for a project
			secretsList, err = s.secretService.ListByProject(ctx, projectFilter)
		} else {
			// Get all secrets (admin only)
			secretsList, err = s.secretService.ListAll(ctx)
		}

		if err != nil {
			s.logger.Error("Failed to list secrets", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to list secrets")
			return
		}

		// Return metadata only (no values for security)
		type secretResponse struct {
			ID        int64     `json:"id"`
			Project   string    `json:"project"`
			Scope     string    `json:"scope"`
			Key       string    `json:"key"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
		}

		result := make([]secretResponse, 0, len(secretsList))
		for _, sec := range secretsList {
			result = append(result, secretResponse{
				ID:        sec.ID,
				Project:   sec.Project,
				Scope:     sec.Scope,
				Key:       sec.Key,
				CreatedAt: sec.CreatedAt,
				UpdatedAt: sec.UpdatedAt,
			})
		}
		s.jsonResponse(w, result)

	case http.MethodPost:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		// Create or update a secret - limit body size to 1MB
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			Project string `json:"project"`
			Scope   string `json:"scope"`
			Key     string `json:"key"`
			Value   string `json:"value"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if req.Project == "" || req.Key == "" {
			s.jsonError(w, http.StatusBadRequest, "project and key are required")
			return
		}

		// Default scope to "default" if not provided
		if req.Scope == "" {
			req.Scope = "default"
		}

		// Use SecretService for encryption and storage
		if s.secretService == nil {
			s.jsonError(w, http.StatusInternalServerError, "Secret service not configured")
			return
		}

		if err := s.secretService.Set(ctx, req.Project, req.Scope, req.Key, req.Value); err != nil {
			s.logger.Error("Failed to store secret", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to store secret")
			return
		}

		s.logAudit(r, "create", "secret", fmt.Sprintf("project=%s scope=%s key=%s", req.Project, req.Scope, req.Key), "success")

		s.jsonResponse(w, map[string]string{
			"status":  "created",
			"project": req.Project,
			"scope":   req.Scope,
			"key":     req.Key,
		})

	case http.MethodDelete:
		// Delete a secret
		project := r.URL.Query().Get("project")
		scope := r.URL.Query().Get("scope")
		key := r.URL.Query().Get("key")

		if project == "" || key == "" {
			s.jsonError(w, http.StatusBadRequest, "project and key are required")
			return
		}
		if scope == "" {
			scope = "default"
		}

		if s.secretService == nil {
			s.jsonError(w, http.StatusInternalServerError, "Secret service not configured")
			return
		}

		if err := s.secretService.Delete(ctx, project, scope, key); err != nil {
			s.logger.Error("Failed to delete secret", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to delete secret")
			return
		}

		s.logAudit(r, "delete", "secret", fmt.Sprintf("project=%s scope=%s key=%s", project, scope, key), "success")

		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProjectTypes handles GET/POST for /api/v1/project-types.
func (s *MasterServer) handleProjectTypes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		types, err := s.projectTypeService.List(ctx)
		if err != nil {
			s.logger.Error("Failed to list project types", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to list project types")
			return
		}
		s.jsonResponse(w, types)

	case http.MethodPost:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		// Limit body size to 1MB
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			BuildCmd    string `json:"build_cmd"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if req.Name == "" {
			s.jsonError(w, http.StatusBadRequest, "name is required")
			return
		}

		pt, err := s.projectTypeService.Create(ctx, req.Name, req.Description, req.BuildCmd)
		if err != nil {
			s.logger.Error("Failed to create project type", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to create project type")
			return
		}

		s.logAudit(r, "create", "project_type", fmt.Sprintf("name=%s", req.Name), "success")
		s.jsonResponse(w, pt)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProjectType handles GET/PUT/DELETE for /api/v1/project-types/{name}.
func (s *MasterServer) handleProjectType(w http.ResponseWriter, r *http.Request) {
	// Extract name from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/project-types/")
	parts := strings.Split(path, "/")
	name := parts[0]

	if name == "" {
		s.jsonError(w, http.StatusBadRequest, "Project type name required")
		return
	}

	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		pt, err := s.projectTypeService.GetByName(ctx, name)
		if err != nil {
			s.logger.Error("Failed to get project type", zap.Error(err))
			s.jsonError(w, http.StatusNotFound, "Project type not found")
			return
		}
		s.jsonResponse(w, pt)

	case http.MethodPut:
		var req struct {
			Description string `json:"description"`
			BuildCmd    string `json:"build_cmd"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		pt := &storage.ProjectType{
			Name:        name,
			Description: req.Description,
			BuildCmd:    req.BuildCmd,
		}

		if err := s.projectTypeService.Update(ctx, pt); err != nil {
			s.logger.Error("Failed to update project type", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to update project type")
			return
		}

		s.logAudit(r, "update", "project_type", fmt.Sprintf("name=%s", name), "success")
		s.jsonResponse(w, map[string]string{"status": "updated"})

	case http.MethodDelete:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		if err := s.projectTypeService.Delete(ctx, name); err != nil {
			s.logger.Error("Failed to delete project type", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to delete project type")
			return
		}

		s.logAudit(r, "delete", "project_type", fmt.Sprintf("name=%s", name), "success")
		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *MasterServer) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Admin-only: viewing audit logs
	if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
		http.Error(w, msg, status)
		return
	}

	// Parse pagination params
	p := parsePagination(r)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	entries, err := s.auditService.List(ctx, p.Limit, p.Offset)
	if err != nil {
		s.logger.Error("Failed to list audit logs", zap.Error(err))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, entries)
}

// logAudit creates an audit log entry with request context.
func (s *MasterServer) logAudit(r *http.Request, action, resource, details, result string) {
	// Get username from context (set by auth middleware)
	user := "anonymous"
	if username, ok := r.Context().Value("username").(string); ok {
		user = username
	}

	// Get IP address
	ip := extractClientIP(r)

	entry := &storage.AuditEntry{
		Source:    "http",
		User:      user,
		Action:    action,
		Resource:  resource,
		Details:   details,
		IPAddress: ip,
		Result:    result,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.auditService.Log(ctx, entry); err != nil {
		s.logger.Error("Failed to write audit log", zap.Error(err))
	}
}

// logAuditWithSnapshot creates an audit log entry with request context and a resource snapshot.
// This is used for delete operations to capture the resource state before deletion.
func (s *MasterServer) logAuditWithSnapshot(r *http.Request, action, resource, resourceID string, snapshot any, details, result string) {
	// Get username from context (set by auth middleware)
	user := "anonymous"
	if username, ok := r.Context().Value("username").(string); ok {
		user = username
	}

	// Get IP address
	ip := extractClientIP(r)

	entry := &storage.AuditEntry{
		Source:     "http",
		User:       user,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Details:    details,
		IPAddress:  ip,
		Result:     result,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.auditService.LogWithSnapshot(ctx, entry, snapshot); err != nil {
		s.logger.Error("Failed to write audit log with snapshot", zap.Error(err))
	}
}

func (s *MasterServer) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (s *MasterServer) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Delegate to the webhooks handler if configured
	if s.webhookHandler != nil && s.webhookHandler.handler != nil {
		s.webhookHandler.handler.HandleGitHub(w, r)
		return
	}

	// Fallback: basic validation only
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		http.Error(w, "Missing signature", http.StatusUnauthorized)
		return
	}
	s.logger.Warn("Received GitHub webhook but no processor configured - webhook will not trigger deployment")
	w.WriteHeader(http.StatusOK)
}

func (s *MasterServer) handleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Delegate to the webhooks handler if configured
	if s.webhookHandler != nil && s.webhookHandler.handler != nil {
		s.webhookHandler.handler.HandleGitLab(w, r)
		return
	}

	// Fallback: basic validation only
	token := r.Header.Get("X-Gitlab-Token")
	if token == "" {
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}
	s.logger.Warn("Received GitLab webhook but no processor configured - webhook will not trigger deployment")
	w.WriteHeader(http.StatusOK)
}

func (s *MasterServer) handleBitbucketWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Delegate to the webhooks handler if configured
	if s.webhookHandler != nil && s.webhookHandler.handler != nil {
		s.webhookHandler.handler.HandleBitbucket(w, r)
		return
	}

	s.logger.Warn("Received Bitbucket webhook but no processor configured - webhook will not trigger deployment")
	w.WriteHeader(http.StatusOK)
}

// handleAPILogin handles JSON-based authentication for API clients.
// POST /api/v1/auth/login
// Request: {"username": "...", "password": "...", "totp": "..."}
// Response: {"token": "session_id", "user": {...}}
func (s *MasterServer) handleAPILogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTP     string `json:"totp,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.Username == "" || req.Password == "" {
		s.jsonError(w, http.StatusBadRequest, "username and password required")
		return
	}

	ctx := r.Context()

	// Look up user
	user, err := s.userService.GetByUsername(ctx, req.Username)
	if err != nil {
		if services.IsNotFound(err) {
			s.logger.Debug("API login failed: user not found", zap.String("username", req.Username))
			s.logAudit(r, "api_login", "session", "user not found: "+req.Username, "failure")
			s.jsonError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		s.logger.Error("Database error during API login", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal error")
		return
	}

	// Verify password
	if !verifyPassword(user.PasswordHash, req.Password) {
		s.logger.Debug("API login failed: invalid password", zap.String("username", req.Username))
		s.logAudit(r, "api_login", "session", "invalid password for: "+req.Username, "failure")
		s.jsonError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Check if user must change password
	if user.MustChangePassword {
		s.logger.Debug("API login blocked: password change required", zap.String("username", req.Username))
		s.logAudit(r, "api_login", "session", "password change required for: "+req.Username, "blocked")
		s.jsonError(w, http.StatusForbidden, "Password change required. Please login via web UI to change your password.")
		return
	}

	// Verify TOTP if enabled
	if user.TOTPEnabled {
		if req.TOTP == "" {
			s.jsonError(w, http.StatusUnauthorized, "TOTP required")
			return
		}
		if !verifyTOTP(user.TOTPSecret, req.TOTP) {
			s.logger.Debug("API login failed: invalid TOTP", zap.String("username", req.Username))
			s.logAudit(r, "api_login", "session", "invalid TOTP for: "+req.Username, "failure")
			s.jsonError(w, http.StatusUnauthorized, "Invalid verification code")
			return
		}
	}

	// Create session using service
	session, err := s.sessionService.Create(ctx, user.ID, extractClientIP(r), r.UserAgent(), 7*24*time.Hour)
	if err != nil {
		s.logger.Error("Failed to create session", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	// Log successful login
	s.logAudit(r, "api_login", "session", fmt.Sprintf("user: %s, IP: %s", req.Username, session.IPAddress), "success")

	s.logger.Info("User logged in via API",
		zap.String("username", req.Username),
		zap.String("ip", session.IPAddress))

	s.jsonResponse(w, map[string]interface{}{
		"token": session.ID,
		"user": map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
		},
	})
}

func (s *MasterServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *MasterServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")
		totp := r.FormValue("totp")
		s.logger.Debug("Login attempt", zap.String("username", username))

		ctx := r.Context()

		// Look up user
		user, err := s.userService.GetByUsername(ctx, username)
		if err != nil {
			if services.IsNotFound(err) {
				s.logger.Debug("Login failed: user not found", zap.String("username", username))
				s.logAudit(r, "login", "session", "user not found: "+username, "failure")
				s.renderTemplate(w, "login", map[string]interface{}{"Error": "Invalid credentials"})
				return
			}
			s.logger.Error("Database error during login", zap.Error(err))
			s.renderTemplate(w, "login", map[string]interface{}{"Error": "Internal error"})
			return
		}

		// Verify password
		if !verifyPassword(user.PasswordHash, password) {
			s.logger.Debug("Login failed: invalid password", zap.String("username", username))
			s.logAudit(r, "login", "session", "invalid password for: "+username, "failure")
			s.renderTemplate(w, "login", map[string]interface{}{"Error": "Invalid credentials"})
			return
		}

		// Verify TOTP if enabled
		if user.TOTPEnabled {
			if totp == "" {
				s.renderTemplate(w, "login", map[string]interface{}{
					"Username":  username,
					"NeedsTOTP": true,
				})
				return
			}
			if !verifyTOTP(user.TOTPSecret, totp) {
				s.logger.Debug("Login failed: invalid TOTP", zap.String("username", username))
				s.logAudit(r, "login", "session", "invalid TOTP for: "+username, "failure")
				s.renderTemplate(w, "login", map[string]interface{}{
					"Error":     "Invalid verification code",
					"Username":  username,
					"NeedsTOTP": true,
				})
				return
			}
		}

		// Create session using service (even if password change required, we need a session)
		session, err := s.sessionService.Create(ctx, user.ID, extractClientIP(r), r.UserAgent(), 7*24*time.Hour)
		if err != nil {
			s.logger.Error("Failed to create session", zap.Error(err))
			s.renderTemplate(w, "login", map[string]interface{}{"Error": "Internal error"})
			return
		}

		// Set session cookie first (needed for change-password page)
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    session.ID,
			Path:     "/",
			HttpOnly: true,
			Secure:   s.config.Server.TLS.Enabled,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   86400 * 7, // 7 days
		})

		// Check if user must change password
		if user.MustChangePassword {
			s.logger.Info("User must change password", zap.String("username", username))
			s.logAudit(r, "login", "session", fmt.Sprintf("user: %s, password change required", username), "success")
			http.Redirect(w, r, "/change-password", http.StatusSeeOther)
			return
		}

		// Log successful login
		s.logAudit(r, "login", "session", fmt.Sprintf("user: %s, IP: %s", username, session.IPAddress), "success")

		s.logger.Info("User logged in",
			zap.String("username", username),
			zap.String("ip", session.IPAddress))

		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	s.renderTemplate(w, "login", nil)
}

func (s *MasterServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Delete session from database
	if cookie, err := r.Cookie("session"); err == nil && cookie.Value != "" {
		ctx := r.Context()

		// Get user ID for audit log (if session exists) before deleting
		if session, err := s.sessionService.GetByToken(ctx, cookie.Value); err == nil {
			_ = s.auditService.Log(ctx, &storage.AuditEntry{
				Source:    "web",
				User:      fmt.Sprintf("user:%d", session.UserID),
				Action:    "logout",
				Resource:  "session",
				Result:    "success",
				Timestamp: time.Now(),
			})
		}

		if err := s.sessionService.Delete(ctx, cookie.Value); err != nil {
			s.logger.Debug("Failed to delete session", zap.Error(err))
		}
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// handleChangePassword handles the password change page for users who must change their password.
func (s *MasterServer) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := GetUserFromContext(r.Context())
	if !ok || user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodPost {
		currentPassword := r.FormValue("current_password")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")

		// Validate current password
		if !verifyPassword(user.PasswordHash, currentPassword) {
			s.renderTemplate(w, "change-password", map[string]interface{}{
				"Error":              "Current password is incorrect",
				"MustChangePassword": user.MustChangePassword,
			})
			return
		}

		// Validate new password matches confirmation
		if newPassword != confirmPassword {
			s.renderTemplate(w, "change-password", map[string]interface{}{
				"Error":              "New passwords do not match",
				"MustChangePassword": user.MustChangePassword,
			})
			return
		}

		// Validate new password is different from current
		if currentPassword == newPassword {
			s.renderTemplate(w, "change-password", map[string]interface{}{
				"Error":              "New password must be different from current password",
				"MustChangePassword": user.MustChangePassword,
			})
			return
		}

		// Update password using service (this clears MustChangePassword flag)
		ctx := r.Context()
		if err := s.userService.UpdatePassword(ctx, user.ID, newPassword); err != nil {
			s.logger.Error("Failed to update password", zap.Error(err))
			s.renderTemplate(w, "change-password", map[string]interface{}{
				"Error":              "Failed to update password: " + err.Error(),
				"MustChangePassword": user.MustChangePassword,
			})
			return
		}

		// Log password change
		s.logAudit(r, "password_change", "user", fmt.Sprintf("user: %s", user.Username), "success")

		s.logger.Info("User changed password",
			zap.String("username", user.Username),
			zap.Bool("was_forced", user.MustChangePassword))

		// Redirect to dashboard
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// GET request - show the change password form
	s.renderTemplate(w, "change-password", map[string]interface{}{
		"MustChangePassword": user.MustChangePassword,
	})
}

func (s *MasterServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Fetch stats
	stats := map[string]interface{}{
		"TotalProjects":    0,
		"DeploymentsToday": 0,
		"ConnectedAgents":  0,
		"SuccessRate":      0,
	}

	// Get projects count
	projects, err := s.projectService.List(ctx)
	if err == nil {
		stats["TotalProjects"] = len(projects)
	}

	// Get agents and count connected
	agents, err := s.agentService.List(ctx)
	var agentData []map[string]interface{}
	if err == nil {
		var connectedCount int
		for _, a := range agents {
			if a.Status == "connected" {
				connectedCount++
			}
			agentData = append(agentData, map[string]interface{}{
				"Name":     a.Hostname,
				"Hostname": a.Hostname,
				"Status":   a.Status,
			})
		}
		stats["ConnectedAgents"] = connectedCount
	}

	// Get recent deployments and calculate stats (limited to 5 for dashboard display)
	deployments, err := s.deploymentService.ListRecent(ctx, 5)
	var recentDeployments []map[string]interface{}
	if err == nil {
		var successCount, totalCount int
		today := time.Now().Truncate(24 * time.Hour)
		var deploymentsToday int

		for _, d := range deployments {
			if d.StartedAt.After(today) {
				deploymentsToday++
			}
			if d.Status == "success" {
				successCount++
			}
			totalCount++

			recentDeployments = append(recentDeployments, map[string]interface{}{
				"ID":          d.ID,
				"ProjectName": d.Project,
				"Branch":      d.Branch,
				"Status":      d.Status,
				"CreatedAt":   d.StartedAt,
			})
		}

		stats["DeploymentsToday"] = deploymentsToday
		if totalCount > 0 {
			stats["SuccessRate"] = (successCount * 100) / totalCount
		}
	}

	// Get recent audit logs (limited to 5 for dashboard)
	auditLogs, err := s.auditService.List(ctx, 5, 0)
	var recentActivity []map[string]interface{}
	if err == nil {
		for _, log := range auditLogs {
			recentActivity = append(recentActivity, map[string]interface{}{
				"Action":    log.Action,
				"Username":  log.User,
				"CreatedAt": log.Timestamp,
			})
		}
	}

	data := s.withCommonData(r, map[string]interface{}{
		"Title":             "Dashboard",
		"Active":            "dashboard",
		"Stats":             stats,
		"RecentDeployments": recentDeployments,
		"Agents":            agentData,
		"RecentActivity":    recentActivity,
	})

	s.renderTemplate(w, "dashboard", data)
}

func (s *MasterServer) handleProjectsUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get projects
	var projectsList []map[string]interface{}
	projects, err := s.projectService.List(ctx)
	if err != nil {
		s.logger.Error("Failed to list projects", zap.Error(err))
	} else {
		for _, p := range projects {
			projectsList = append(projectsList, map[string]interface{}{
				"ID":         p.ID,
				"Name":       p.Name,
				"Type":       p.Type,
				"Repository": p.Repository,
				"Branch":     p.Branch,
				"Path":       p.DeployPath,
				"CreatedAt":  p.CreatedAt,
			})
		}
	}

	s.renderTemplate(w, "projects", s.withCommonData(r, map[string]interface{}{
		"Title":    "Projects",
		"Active":   "projects",
		"Projects": projectsList,
	}))
}

func (s *MasterServer) handleDeploymentsUI(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "deployments", s.withCommonData(r, map[string]interface{}{"Title": "Deployments", "Active": "deployments"}))
}

func (s *MasterServer) handleAgentsUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get agents from database
	var agentsList []map[string]interface{}
	agents, err := s.agentService.List(ctx)
	if err != nil {
		s.logger.Error("Failed to list agents", zap.Error(err))
	} else {
		for _, a := range agents {
			agentsList = append(agentsList, map[string]interface{}{
				"ID":         a.ID,
				"Hostname":   a.Hostname,
				"Status":     a.Status,
				"Version":    a.Version,
				"OS":         a.OS,
				"Arch":       a.Arch,
				"LastSeenAt": a.LastSeenAt,
			})
		}
	}

	s.renderTemplate(w, "agents", s.withCommonData(r, map[string]interface{}{
		"Title":  "Agents",
		"Active": "agents",
		"Agents": agentsList,
	}))
}

func (s *MasterServer) handleSettingsUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Build settings map for template
	settings := map[string]interface{}{}

	if s.settingsSvc != nil {
		// Server settings
		settings["MasterURL"], _ = s.settingsSvc.GetString(ctx, "server", "master_url", "")
		settings["LogLevel"], _ = s.settingsSvc.GetString(ctx, "logs", "app_level", "info")
		settings["RetentionDays"], _ = s.settingsSvc.GetInt(ctx, "logs", "deploy_retention", 90)
		settings["MaxConcurrent"], _ = s.settingsSvc.GetInt(ctx, "deployments", "max_concurrent", 10)

		// Security settings
		settings["Require2FA"], _ = s.settingsSvc.GetBool(ctx, "security", "require_2fa", false)
		settings["ForceHTTPS"], _ = s.settingsSvc.GetBool(ctx, "security", "force_https", false)
		settings["AuditLog"], _ = s.settingsSvc.GetBool(ctx, "security", "audit_log", true)
		settings["SessionTimeout"], _ = s.settingsSvc.GetInt(ctx, "security", "session_timeout", 60)

		// Notification settings
		settings["SlackWebhook"], _ = s.settingsSvc.GetString(ctx, "notifications", "slack_webhook", "")
		settings["SlackChannel"], _ = s.settingsSvc.GetString(ctx, "notifications", "slack_channel", "")
		settings["SMTPHost"], _ = s.settingsSvc.GetString(ctx, "notifications", "smtp_host", "")
		settings["SMTPPort"], _ = s.settingsSvc.GetInt(ctx, "notifications", "smtp_port", 587)
		settings["SMTPUser"], _ = s.settingsSvc.GetString(ctx, "notifications", "smtp_user", "")
		settings["SMTPFrom"], _ = s.settingsSvc.GetString(ctx, "notifications", "smtp_from", "")

		// Appearance settings
		settings["DarkMode"], _ = s.settingsSvc.GetBool(ctx, "appearance", "dark_mode", true)
		settings["ThemeColor"], _ = s.settingsSvc.GetString(ctx, "appearance", "theme_color", "green")
	}

	data := s.withCommonData(r, map[string]interface{}{
		"Title":  "Settings",
		"Active": "settings",
	})
	data["Settings"] = settings
	s.renderTemplate(w, "settings", data)
}

func (s *MasterServer) handleSecretsUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get projects for filter dropdown
	projects, err := s.projectService.List(ctx)
	if err != nil {
		s.logger.Error("Failed to list projects for secrets UI", zap.Error(err))
		projects = nil
	}

	// Get secrets
	var secretsList []map[string]interface{}
	if s.secretService != nil {
		secrets, err := s.secretService.ListAll(ctx)
		if err != nil {
			s.logger.Error("Failed to list secrets", zap.Error(err))
		} else {
			for _, sec := range secrets {
				secretsList = append(secretsList, map[string]interface{}{
					"ID":        sec.ID,
					"Project":   sec.Project,
					"Scope":     sec.Scope,
					"Key":       sec.Key,
					"CreatedAt": sec.CreatedAt,
				})
			}
		}
	}

	data := s.withCommonData(r, map[string]interface{}{
		"Title":   "Secrets",
		"Active":  "secrets",
		"Secrets": secretsList,
	})
	data["Projects"] = projects
	s.renderTemplate(w, "secrets", data)
}

func (s *MasterServer) handleProjectTypesUI(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "project-types", s.withCommonData(r, map[string]interface{}{
		"Title":  "Project Types",
		"Active": "project-types",
	}))
}

func (s *MasterServer) handleAuditUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get audit logs
	var auditLogs []map[string]interface{}
	entries, err := s.auditService.List(ctx, 100, 0)
	if err != nil {
		s.logger.Error("Failed to list audit logs", zap.Error(err))
	} else {
		for _, entry := range entries {
			auditLogs = append(auditLogs, map[string]interface{}{
				"ID":        entry.ID,
				"Timestamp": entry.Timestamp,
				"User":      entry.User,
				"Action":    entry.Action,
				"Resource":  entry.Resource,
				"Details":   entry.Details,
				"IPAddress": entry.IPAddress,
				"Result":    entry.Result,
			})
		}
	}

	s.renderTemplate(w, "audit", s.withCommonData(r, map[string]interface{}{
		"Title":     "Audit Log",
		"Active":    "audit",
		"AuditLogs": auditLogs,
	}))
}

func (s *MasterServer) handleAPIKeysUI(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "apikeys", s.withCommonData(r, map[string]interface{}{
		"Title":  "API Keys",
		"Active": "apikeys",
	}))
}

// withCommonData adds common template data like ShowNav, Username, CSPNonce for authenticated pages
func (s *MasterServer) withCommonData(r *http.Request, data map[string]interface{}) map[string]interface{} {
	if data == nil {
		data = make(map[string]interface{})
	}
	data["ShowNav"] = true

	// Add CSP nonce for inline scripts
	if nonce := GetCSPNonce(r); nonce != "" {
		data["CSPNonce"] = nonce
	}

	// Get username from context
	if userID, ok := GetUserIDFromContext(r.Context()); ok {
		user, err := s.userService.GetByID(r.Context(), userID)
		if err == nil && user != nil {
			data["Username"] = user.Username
		}
	}

	return data
}

func (s *MasterServer) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	if s.templates == nil {
		http.Error(w, "Templates not loaded", http.StatusInternalServerError)
		return
	}

	tmpl, ok := s.templates[name+".html"]
	if !ok {
		s.logger.Error("Template not found", zap.String("template", name))
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	// Convert data to a map and add common theme settings
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		dataMap = map[string]interface{}{}
	}

	// Add appearance settings if settings service is available
	if s.settingsSvc != nil {
		ctx := context.Background()
		darkMode, _ := s.settingsSvc.GetBool(ctx, "appearance", "dark_mode", true)
		themeColor, _ := s.settingsSvc.GetString(ctx, "appearance", "theme_color", "green")
		theme, _ := s.settingsSvc.GetString(ctx, "appearance", "theme", "dark")

		dataMap["DarkMode"] = darkMode
		dataMap["ThemeColor"] = themeColor
		dataMap["Theme"] = theme
	} else {
		// Defaults if settings service not available
		dataMap["DarkMode"] = true
		dataMap["ThemeColor"] = "green"
		dataMap["Theme"] = "dark"
	}

	// Execute the page template (which includes base via {{template "base" .}})
	if err := tmpl.ExecuteTemplate(w, name+".html", dataMap); err != nil {
		s.logger.Error("Template render error", zap.String("template", name), zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// --- Helper Functions ---

// verifyPassword verifies a password against a bcrypt hash.
func verifyPassword(hash, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	// Use bcrypt for password verification
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// verifyTOTP verifies a TOTP code against a secret.
func verifyTOTP(secret, code string) bool {
	if secret == "" || code == "" {
		return false
	}
	return security.ValidateTOTP(secret, code, security.DefaultTOTPConfig())
}

// extractClientIP extracts the client IP from a request, handling proxies.
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (first IP in chain is the client)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
