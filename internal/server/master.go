// Package server provides the master daemon HTTP and gRPC servers.
package server

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
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

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/proto"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/webhooks"
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

	// Webhook handling
	webhookHandler *webhookHandlerAdapter

	// Security middleware
	securityMiddleware *SecurityMiddleware
	rateLimiter        *RateLimiter

	// Agent management (for HTTP API, synced from gRPC service)
	agents   map[string]*AgentConnection
	agentsMu sync.RWMutex

	// Templates (loaded from disk)
	templates    *template.Template
	templatesDir string

	// Shutdown handling
	shutdown chan struct{}
	wg       sync.WaitGroup
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
	handler *webhooks.Handler
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

	// Load templates from disk
	if err := s.loadTemplates(); err != nil {
		logger.Warn("Failed to load templates, using defaults", zap.Error(err))
	}

	// Initialize security middleware
	s.securityMiddleware = NewSecurityMiddleware(DefaultSecurityConfig())

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
func (s *MasterServer) SetWebhookHandler(kms *security.KMS, processor webhooks.EventProcessor) {
	// Also set KMS on the server for secrets API
	s.kms = kms

	secretStore := &webhookSecretStoreAdapter{
		db:     s.db,
		kms:    kms,
		logger: s.logger,
	}

	s.webhookHandler = &webhookHandlerAdapter{
		handler: webhooks.NewHandler(s.logger, secretStore, processor),
	}
}

// encryptSecret encrypts a secret value using the KMS.
func (s *MasterServer) encryptSecret(plaintext []byte) ([]byte, error) {
	if s.kms == nil {
		return nil, fmt.Errorf("KMS not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Encrypt returns a versioned string (v1:keyid:base64)
	encrypted, err := s.kms.Encrypt(ctx, plaintext)
	if err != nil {
		return nil, fmt.Errorf("encrypting secret: %w", err)
	}

	return []byte(encrypted), nil
}

// decryptSecret decrypts a secret value using the KMS.
func (s *MasterServer) decryptSecret(ciphertext []byte) ([]byte, error) {
	if s.kms == nil {
		return nil, fmt.Errorf("KMS not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	plaintext, err := s.kms.Decrypt(ctx, string(ciphertext))
	if err != nil {
		return nil, fmt.Errorf("decrypting secret: %w", err)
	}

	return plaintext, nil
}

// GetAgentServer returns the gRPC agent server.
func (s *MasterServer) GetAgentServer() *AgentServer {
	return s.agentServer
}

func (s *MasterServer) loadTemplates() error {
	s.templates = template.New("").Funcs(s.templateFuncs())

	// Check if templates directory exists
	if _, err := os.Stat(s.templatesDir); os.IsNotExist(err) {
		// No templates directory - using defaults is fine
		return nil
	}

	// Load all template files from the templates directory
	pattern := filepath.Join(s.templatesDir, "*.html")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("globbing templates: %w", err)
	}

	if len(files) == 0 {
		// No template files found - using defaults
		return nil
	}

	// Parse all found template files
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", file, err)
		}

		name := filepath.Base(file)
		_, err = s.templates.New(name).Parse(string(content))
		if err != nil {
			return fmt.Errorf("parsing template %s: %w", name, err)
		}
	}

	return nil
}

func (s *MasterServer) templateFuncs() template.FuncMap {
	return template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"json": func(v interface{}) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
	}
}

// Start starts the master server.
func (s *MasterServer) Start(ctx context.Context) error {
	errCh := make(chan error, 2)

	s.wg.Go(func() {
		if err := s.startHTTP(); err != nil && err != http.ErrServerClosed {
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

	// Webhooks
	mux.HandleFunc("/webhook/github/", s.handleGitHubWebhook)
	mux.HandleFunc("/webhook/gitlab/", s.handleGitLabWebhook)
	mux.HandleFunc("/webhook/bitbucket/", s.handleBitbucketWebhook)

	// UI Routes
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
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

	// Build middleware chain: logging -> security headers -> rate limiting -> handler
	var handler http.Handler = mux
	handler = s.loggingMiddleware(handler)

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

	if s.config.Server.TLS.Enabled {
		cert, err := tls.LoadX509KeyPair(s.config.Server.TLS.Cert, s.config.Server.TLS.Key)
		if err != nil {
			return fmt.Errorf("loading TLS cert: %w", err)
		}
		creds := credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cert}})
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
	cleanupTask := NewCleanupTask(s.db, s.logger, cleanupConfig)
	cleanupTask.Start()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	defer cleanupTask.Stop()

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
	deployments, err := s.db.ListPendingScheduledDeployments(ctx)
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
		deployment := &storage.Deployment{
			ID:          d.ID,
			Project:     d.Project,
			Target:      d.Target,
			Branch:      d.Branch,
			Status:      "running",
			TriggeredBy: d.ScheduledBy,
			StartedAt:   time.Now(),
		}
		if err := s.db.UpdateDeployment(ctx, deployment); err != nil {
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
			s.db.UpdateDeployment(ctx, deployment)
			continue
		}

		// Note: actual completion status will be updated when agent reports back
		s.logger.Info("Deployment triggered on agent", zap.String("deployment_id", d.ID))
	}
}

// triggerDeploymentOnAgent sends a deployment command to the appropriate agent.
func (s *MasterServer) triggerDeploymentOnAgent(ctx context.Context, deployment *storage.Deployment) error {
	// Get project details
	project, err := s.db.GetProjectByName(ctx, deployment.Project)
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
func (s *MasterServer) Shutdown(ctx context.Context) error {
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
		s.httpServer.Shutdown(ctx)
	}
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	s.wg.Wait()
	return nil
}

// Stop is an alias for Shutdown
func (s *MasterServer) Stop(ctx context.Context) error {
	return s.Shutdown(ctx)
}

func (s *MasterServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(wrapped, r)
		s.logger.Debug("HTTP request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", wrapped.status),
			zap.Duration("duration", time.Since(start)),
		)
	})
}

func (s *MasterServer) withAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			userID, err := s.validateAPIKey(r.Context(), token)
			if err != nil {
				s.logger.Debug("API key validation failed", zap.Error(err))
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			// Add user ID to context for downstream handlers
			ctx := context.WithValue(r.Context(), contextKeyUserID, userID)
			handler(w, r.WithContext(ctx))
			return
		} else {
			http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
			return
		}
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

// validateAPIKey validates an API key and returns the user ID if valid.
func (s *MasterServer) validateAPIKey(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, fmt.Errorf("empty API key")
	}

	// Hash the key for lookup
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	// Look up in database
	apiKey, err := s.db.GetAPIKeyByHash(ctx, keyHash)
	if errors.Is(err, storage.ErrNotFound) {
		return 0, fmt.Errorf("API key not found")
	}
	if err != nil {
		return 0, fmt.Errorf("database error: %w", err)
	}

	// Check if valid
	if !apiKey.IsValid() {
		return 0, fmt.Errorf("API key expired or revoked")
	}

	// Update last used timestamp (async to not slow down request)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("panic in API key usage update", zap.Any("panic", r))
			}
		}()
		if err := s.db.UpdateAPIKeyUsage(context.Background(), apiKey.ID); err != nil {
			s.logger.Debug("Failed to update API key usage", zap.Error(err))
		}
	}()

	return apiKey.UserID, nil
}

// validateSession validates a session token and returns the user ID if valid.
func (s *MasterServer) validateSession(ctx context.Context, token string) (int64, error) {
	if token == "" {
		return 0, fmt.Errorf("empty session token")
	}

	// Look up in database
	session, err := s.db.GetSessionByToken(ctx, token)
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
	})
}

func (s *MasterServer) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.jsonResponse(w, []interface{}{})
	case http.MethodPost:
		s.jsonResponse(w, map[string]string{"status": "created"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *MasterServer) handleProject(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *MasterServer) handleDeployments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.jsonResponse(w, []interface{}{})
	case http.MethodPost:
		s.jsonResponse(w, map[string]string{"status": "queued"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *MasterServer) handleDeployment(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]string{"status": "ok"})
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
		var secrets []*storage.Secret
		var err error

		if projectFilter != "" && scopeFilter != "" {
			// Get secrets for specific project/scope
			secrets, err = s.db.ListSecretsWithScope(ctx, projectFilter, scopeFilter)
		} else if projectFilter != "" {
			// Get all secrets for a project
			secrets, err = s.db.ListSecretsCtx(ctx, projectFilter)
		} else {
			// Get all secrets (admin only)
			secrets, err = s.db.ListAllSecretsCtx(ctx)
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

		result := make([]secretResponse, 0, len(secrets))
		for _, sec := range secrets {
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
		// Create or update a secret
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

		// Encrypt the value before storing
		encrypted, err := s.encryptSecret([]byte(req.Value))
		if err != nil {
			s.logger.Error("Failed to encrypt secret", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to encrypt secret")
			return
		}

		if err := s.db.SetSecretEncrypted(ctx, req.Project, req.Scope, req.Key, encrypted); err != nil {
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

		if err := s.db.DeleteSecretCtx(ctx, project, scope, key); err != nil {
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
	switch r.Method {
	case http.MethodGet:
		types, err := s.db.ListProjectTypes()
		if err != nil {
			s.logger.Error("Failed to list project types", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to list project types")
			return
		}
		s.jsonResponse(w, types)

	case http.MethodPost:
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

		pt := &storage.ProjectType{
			Name:        req.Name,
			Description: req.Description,
			BuildCmd:    req.BuildCmd,
			CreatedAt:   time.Now(),
		}

		if err := s.db.CreateProjectType(pt); err != nil {
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

	switch r.Method {
	case http.MethodGet:
		pt, err := s.db.GetProjectTypeByName(name)
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

		pt := &storage.ProjectType{
			Name:        name,
			Description: req.Description,
			BuildCmd:    req.BuildCmd,
		}

		if err := s.db.UpdateProjectTypeByName(pt); err != nil {
			s.logger.Error("Failed to update project type", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to update project type")
			return
		}

		s.logAudit(r, "update", "project_type", fmt.Sprintf("name=%s", name), "success")
		s.jsonResponse(w, map[string]string{"status": "updated"})

	case http.MethodDelete:
		if err := s.db.DeleteProjectType(name); err != nil {
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

	// Parse pagination params
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50
	offset := 0

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	entries, err := s.db.ListAuditLogs(ctx, limit, offset)
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

	if err := s.db.LogAudit(ctx, entry); err != nil {
		s.logger.Error("Failed to write audit log", zap.Error(err))
	}
}

func (s *MasterServer) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
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
	s.logger.Info("Received GitHub webhook (no processor configured)")
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
	s.logger.Info("Received GitLab webhook (no processor configured)")
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

	s.logger.Info("Received Bitbucket webhook (no processor configured)")
	w.WriteHeader(http.StatusOK)
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
		user, err := s.db.GetUserByUsername(ctx, username)
		if errors.Is(err, storage.ErrNotFound) {
			s.logger.Debug("Login failed: user not found", zap.String("username", username))
			s.logAudit(r, "login", "session", "user not found: "+username, "failure")
			s.renderTemplate(w, "login", map[string]interface{}{"Error": "Invalid credentials"})
			return
		}
		if err != nil {
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

		// Generate session token
		sessionToken, err := security.GenerateSecureToken(32)
		if err != nil {
			s.logger.Error("Failed to generate session token", zap.Error(err))
			s.renderTemplate(w, "login", map[string]interface{}{"Error": "Internal error"})
			return
		}

		// Create session in database
		session := &storage.Session{
			ID:        sessionToken,
			UserID:    user.ID,
			IPAddress: extractClientIP(r),
			UserAgent: r.UserAgent(),
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour), // 7 days
		}
		if err := s.db.CreateSession(ctx, session); err != nil {
			s.logger.Error("Failed to create session", zap.Error(err))
			s.renderTemplate(w, "login", map[string]interface{}{"Error": "Internal error"})
			return
		}

		// Log successful login
		s.logAudit(r, "login", "session", fmt.Sprintf("user: %s, IP: %s", username, session.IPAddress), "success")

		s.logger.Info("User logged in",
			zap.String("username", username),
			zap.String("ip", session.IPAddress))

		// Set session cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    sessionToken,
			Path:     "/",
			HttpOnly: true,
			Secure:   s.config.Server.TLS.Enabled,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   86400 * 7, // 7 days
		})
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
		if session, err := s.db.GetSessionByToken(ctx, cookie.Value); err == nil {
			s.db.LogAudit(ctx, &storage.AuditEntry{
				Source:    "web",
				User:      fmt.Sprintf("user:%d", session.UserID),
				Action:    "logout",
				Resource:  "session",
				Result:    "success",
				Timestamp: time.Now(),
			})
		}

		if err := s.db.DeleteSession(ctx, cookie.Value); err != nil {
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

func (s *MasterServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "dashboard", map[string]interface{}{"Title": "Dashboard"})
}

func (s *MasterServer) handleProjectsUI(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "projects", map[string]interface{}{"Title": "Projects"})
}

func (s *MasterServer) handleDeploymentsUI(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "deployments", map[string]interface{}{"Title": "Deployments"})
}

func (s *MasterServer) handleAgentsUI(w http.ResponseWriter, r *http.Request) {
	s.agentsMu.RLock()
	agents := make([]map[string]interface{}, 0)
	for _, a := range s.agents {
		agents = append(agents, map[string]interface{}{
			"id":     a.ID,
			"name":   a.Name,
			"status": a.Status,
		})
	}
	s.agentsMu.RUnlock()
	s.renderTemplate(w, "agents", map[string]interface{}{"Title": "Agents", "Agents": agents})
}

func (s *MasterServer) handleSettingsUI(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "settings", map[string]interface{}{"Title": "Settings"})
}

func (s *MasterServer) handleSecretsUI(w http.ResponseWriter, r *http.Request) {
	projects, _ := s.db.ListProjects()
	s.renderTemplate(w, "secrets", map[string]interface{}{
		"Title":    "Secrets",
		"Active":   "secrets",
		"Projects": projects,
	})
}

func (s *MasterServer) handleProjectTypesUI(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "project-types", map[string]interface{}{
		"Title":  "Project Types",
		"Active": "project-types",
	})
}

func (s *MasterServer) handleAuditUI(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "audit", map[string]interface{}{
		"Title":  "Audit Log",
		"Active": "audit",
	})
}

func (s *MasterServer) handleAPIKeysUI(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "apikeys", map[string]interface{}{
		"Title":  "API Keys",
		"Active": "apikeys",
	})
}

func (s *MasterServer) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	if s.templates == nil {
		http.Error(w, "Templates not loaded", http.StatusInternalServerError)
		return
	}
	if err := s.templates.ExecuteTemplate(w, name+".html", data); err != nil {
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
