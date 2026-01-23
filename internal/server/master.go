// Package server provides the master daemon HTTP and gRPC servers.
package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
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

	// Agent management
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

// NewMasterServer creates a new master server instance.
func NewMasterServer(cfg *config.MasterConfig, db *storage.DB, logger *zap.Logger) (*MasterServer, error) {
	s := &MasterServer{
		config:       cfg,
		db:           db,
		logger:       logger,
		agents:       make(map[string]*AgentConnection),
		shutdown:     make(chan struct{}),
		templatesDir: "/var/lib/vcdeploy/templates",
	}

	// Load templates from disk
	if err := s.loadTemplates(); err != nil {
		logger.Warn("Failed to load templates, using defaults", zap.Error(err))
	}

	return s, nil
}

func (s *MasterServer) loadTemplates() error {
	s.templates = template.New("").Funcs(s.templateFuncs())
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

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.startHTTP(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", zap.Error(err))
			errCh <- err
		}
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.startGRPC(); err != nil {
			s.logger.Error("gRPC server error", zap.Error(err))
			errCh <- err
		}
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runBackgroundTasks(ctx)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	}
}

func (s *MasterServer) startHTTP() error {
	mux := http.NewServeMux()

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("/var/lib/vcdeploy/static"))))

	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/projects", s.withAuth(s.handleProjects))
	mux.HandleFunc("/api/v1/projects/", s.withAuth(s.handleProject))
	mux.HandleFunc("/api/v1/deployments", s.withAuth(s.handleDeployments))
	mux.HandleFunc("/api/v1/deployments/", s.withAuth(s.handleDeployment))
	mux.HandleFunc("/api/v1/agents", s.withAuth(s.handleAgents))
	mux.HandleFunc("/api/v1/secrets", s.withAuth(s.handleSecrets))
	mux.HandleFunc("/api/v1/audit", s.withAuth(s.handleAuditLogs))

	mux.HandleFunc("/webhook/github/", s.handleGitHubWebhook)
	mux.HandleFunc("/webhook/gitlab/", s.handleGitLabWebhook)
	mux.HandleFunc("/webhook/bitbucket/", s.handleBitbucketWebhook)

	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/dashboard", s.withUIAuth(s.handleDashboard))
	mux.HandleFunc("/projects", s.withUIAuth(s.handleProjectsUI))
	mux.HandleFunc("/deployments", s.withUIAuth(s.handleDeploymentsUI))
	mux.HandleFunc("/agents", s.withUIAuth(s.handleAgentsUI))
	mux.HandleFunc("/settings", s.withUIAuth(s.handleSettingsUI))

	addr := s.config.Server.Listen
	if addr == "" {
		addr = ":8080"
	}
	s.logger.Info("Starting HTTP server", zap.String("addr", addr))

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.loggingMiddleware(mux),
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
	return s.grpcServer.Serve(lis)
}

func (s *MasterServer) runBackgroundTasks(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkAgentHealth()
			s.cleanupOldDeployments()
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

func (s *MasterServer) cleanupOldDeployments()       {}
func (s *MasterServer) processScheduledDeployments() {}

// Shutdown gracefully stops the server.
func (s *MasterServer) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down server")
	close(s.shutdown)

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
			if !s.validateAPIKey(token) {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
		} else {
			http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
			return
		}

		handler(w, r)
	}
}

func (s *MasterServer) withUIAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil || !s.validateSession(cookie.Value) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		handler(w, r)
	}
}

func (s *MasterServer) validateAPIKey(key string) bool {
	return key != ""
}

func (s *MasterServer) validateSession(token string) bool {
	return token != ""
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

func (s *MasterServer) handleSecrets(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, []interface{}{})
}

func (s *MasterServer) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, []interface{}{})
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
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		http.Error(w, "Missing signature", http.StatusUnauthorized)
		return
	}
	s.logger.Info("Received GitHub webhook")
	w.WriteHeader(http.StatusOK)
}

func (s *MasterServer) handleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := r.Header.Get("X-Gitlab-Token")
	if token == "" {
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}
	s.logger.Info("Received GitLab webhook")
	w.WriteHeader(http.StatusOK)
}

func (s *MasterServer) handleBitbucketWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.logger.Info("Received Bitbucket webhook")
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
		_ = password
		_ = totp
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    "TODO_GENERATE_SESSION_TOKEN",
			Path:     "/",
			HttpOnly: true,
			Secure:   s.config.Server.TLS.Enabled,
			MaxAge:   86400 * 7,
		})
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	s.renderTemplate(w, "login", nil)
}

func (s *MasterServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
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
