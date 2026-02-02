// Package server provides HTTP and gRPC server implementations.
package server

import (
	"html/template"
	"net/http"
	"path/filepath"
	"time"

	securitysvc "github.com/BlackOrder/vcdeploy/internal/services/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// CertificateStats holds statistics for the certificates page.
type CertificateStats struct {
	Active   int
	Expiring int
	Expired  int
	Revoked  int
}

// handleCertificatesUI renders the certificate management page.
func (s *MasterServer) handleCertificatesUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get certificate service
	certService := securitysvc.NewCertificateService(s.store, s.logger)

	// Get agent certificates to compute stats
	certs, err := certService.ListAgentCertificates(ctx)
	if err != nil {
		s.logger.Error("Failed to list agent certificates", zap.Error(err))
	}

	// Calculate stats
	stats := CertificateStats{}
	now := time.Now()
	thirtyDays := 30 * 24 * time.Hour

	for _, cert := range certs {
		if cert.RevokedAt != nil {
			stats.Revoked++
		} else if cert.NotAfter.Before(now) {
			stats.Expired++
		} else if cert.NotAfter.Sub(now) < thirtyDays {
			stats.Expiring++
		} else {
			stats.Active++
		}
	}

	// Get CAs for display
	cas, err := certService.ListCAs(ctx)
	if err != nil {
		s.logger.Error("Failed to list CAs", zap.Error(err))
	}

	s.renderTemplate(w, "certificates", s.withCommonData(r, map[string]interface{}{
		"Title":        "Certificates",
		"Active":       "certificates",
		"Stats":        stats,
		"CAs":          cas,
		"Certificates": certs,
	}))
}

// handleCAsPartial renders the CA list partial for HTMX.
func (s *MasterServer) handleCAsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	certService := securitysvc.NewCertificateService(s.store, s.logger)
	cas, err := certService.ListCAs(ctx)
	if err != nil {
		s.logger.Error("Failed to list CAs", zap.Error(err))
		http.Error(w, "Failed to load CAs", http.StatusInternalServerError)
		return
	}

	// Parse and render the partial template
	partialPath := filepath.Join(s.templatesDir, "partials", "certificates_cas.html")
	tmpl, err := template.New("certificates_cas.html").Funcs(s.templateFuncs()).ParseFiles(partialPath)
	if err != nil {
		s.logger.Error("Failed to parse partial template", zap.Error(err))
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"CAs": cas,
	}

	if err := tmpl.Execute(w, data); err != nil {
		s.logger.Error("Failed to render partial", zap.Error(err))
		http.Error(w, "Render error", http.StatusInternalServerError)
	}
}

// handleAgentCertsPartial renders the agent certificates partial for HTMX.
func (s *MasterServer) handleAgentCertsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	certService := securitysvc.NewCertificateService(s.store, s.logger)
	certs, err := certService.ListAgentCertificates(ctx)
	if err != nil {
		s.logger.Error("Failed to list agent certificates", zap.Error(err))
		http.Error(w, "Failed to load certificates", http.StatusInternalServerError)
		return
	}

	// Enrich certificates with computed status fields
	now := time.Now()
	thirtyDays := 30 * 24 * time.Hour
	type CertWithStatus struct {
		securitysvc.AgentCertificateInfo
		IsExpired      bool
		IsExpiringSoon bool
	}

	enrichedCerts := make([]CertWithStatus, 0, len(certs))
	for _, cert := range certs {
		enriched := CertWithStatus{
			AgentCertificateInfo: cert,
			IsExpired:            cert.NotAfter.Before(now),
			IsExpiringSoon:       !cert.NotAfter.Before(now) && cert.NotAfter.Sub(now) < thirtyDays,
		}
		enrichedCerts = append(enrichedCerts, enriched)
	}

	// Parse and render the partial template
	partialPath := filepath.Join(s.templatesDir, "partials", "certificates_agents.html")
	tmpl, err := template.New("certificates_agents.html").Funcs(s.templateFuncs()).ParseFiles(partialPath)
	if err != nil {
		s.logger.Error("Failed to parse partial template", zap.Error(err))
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Certificates": enrichedCerts,
	}

	if err := tmpl.Execute(w, data); err != nil {
		s.logger.Error("Failed to render partial", zap.Error(err))
		http.Error(w, "Render error", http.StatusInternalServerError)
	}
}

// handleCredentialsUI renders the source credentials management page.
func (s *MasterServer) handleCredentialsUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get all source credentials
	creds, err := s.store.ListSourceCredentials(ctx)
	if err != nil {
		s.logger.Error("Failed to list source credentials", zap.Error(err))
		creds = nil
	}

	s.renderTemplate(w, "security/credentials", s.withCommonData(r, map[string]interface{}{
		"Title":       "Source Credentials",
		"Active":      "security",
		"Credentials": creds,
	}))
}

// handleSSHKeysUI renders the SSH keys management page.
func (s *MasterServer) handleSSHKeysUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get all SSH keys
	keys, err := s.store.ListSSHKeys(ctx)
	if err != nil {
		s.logger.Error("Failed to list SSH keys", zap.Error(err))
		keys = nil
	}

	// Get SSH host keys
	hostKeys, err := s.store.ListSSHHostKeys(ctx)
	if err != nil {
		s.logger.Error("Failed to list SSH host keys", zap.Error(err))
		hostKeys = nil
	}

	// Get jump servers
	jumpServers, err := s.store.ListJumpServers(ctx)
	if err != nil {
		s.logger.Error("Failed to list jump servers", zap.Error(err))
		jumpServers = nil
	}

	s.renderTemplate(w, "security/ssh-keys", s.withCommonData(r, map[string]interface{}{
		"Title":       "SSH Keys",
		"Active":      "security",
		"SSHKeys":     keys,
		"HostKeys":    hostKeys,
		"JumpServers": jumpServers,
	}))
}

// handleProvisionUI renders the agent provisioning page.
func (s *MasterServer) handleProvisionUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get pending provision jobs
	pendingJobs, err := s.store.ListPendingProvisionJobs(ctx)
	if err != nil {
		s.logger.Error("Failed to list pending provision jobs", zap.Error(err))
		pendingJobs = nil
	}

	// Get registration tokens
	tokens, err := s.store.ListRegistrationTokens(ctx)
	if err != nil {
		s.logger.Error("Failed to list registration tokens", zap.Error(err))
		tokens = nil
	}

	// Get agent binaries
	binaries, err := s.store.ListAgentBinaries(ctx)
	if err != nil {
		s.logger.Error("Failed to list agent binaries", zap.Error(err))
		binaries = nil
	}

	s.renderTemplate(w, "security/provision", s.withCommonData(r, map[string]interface{}{
		"Title":              "Agent Provisioning",
		"Active":             "security",
		"PendingJobs":        pendingJobs,
		"RegistrationTokens": tokens,
		"AgentBinaries":      binaries,
	}))
}

// handleSecurityAuditUI renders the security audit log page.
func (s *MasterServer) handleSecurityAuditUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get filter parameters
	eventType := r.URL.Query().Get("event_type")
	agentID := r.URL.Query().Get("agent_id")

	// Build filter
	filter := storage.CertAuditFilter{
		EventType: eventType,
		AgentID:   agentID,
		Limit:     100,
	}

	events, err := s.store.ListCertAuditEvents(ctx, filter)
	if err != nil {
		s.logger.Error("Failed to list audit events", zap.Error(err))
		events = nil
	}

	s.renderTemplate(w, "security/audit", s.withCommonData(r, map[string]interface{}{
		"Title":  "Security Audit Log",
		"Active": "security",
		"Events": events,
		"Filter": map[string]string{
			"event_type": eventType,
			"agent_id":   agentID,
		},
	}))
}

// handleTLSSettingsUI renders the TLS settings page.
func (s *MasterServer) handleTLSSettingsUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get ACME certificates
	acmeCerts, err := s.store.ListACMECertificates(ctx)
	if err != nil {
		s.logger.Error("Failed to list ACME certificates", zap.Error(err))
		acmeCerts = nil
	}

	// Build directory URL for display
	directoryURL := "https://acme-v02.api.letsencrypt.org/directory"
	if s.config.Server.TLS.ACME.Staging {
		directoryURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
	}

	// Build TLS config display
	tlsConfig := map[string]interface{}{
		"Mode":             string(s.config.Server.TLS.Mode),
		"CertFile":         s.config.Server.TLS.CertFile,
		"KeyFile":          s.config.Server.TLS.KeyFile,
		"ACMEDomains":      joinDomains(s.config.Server.TLS.ACME.Domains),
		"ACMEEmail":        s.config.Server.TLS.ACME.Email,
		"ACMEDirectoryURL": directoryURL,
		"ACMEAcceptTOS":    true, // Implied by configuration
	}

	s.renderTemplate(w, "settings_tls", s.withCommonData(r, map[string]interface{}{
		"Title":            "TLS Settings",
		"Active":           "settings",
		"TLSConfig":        tlsConfig,
		"ACMECertificates": acmeCerts,
	}))
}

// joinDomains joins slice of domains into comma-separated string
func joinDomains(domains []string) string {
	if len(domains) == 0 {
		return ""
	}
	result := domains[0]
	for i := 1; i < len(domains); i++ {
		result += ", " + domains[i]
	}
	return result
}
