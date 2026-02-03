// Package server provides HTTP and gRPC servers for vcdeploy.
package server

import (
	"crypto/x509"
	"net/http"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
)

// TLSStatus represents the current TLS configuration status.
type TLSStatus struct {
	Mode          string           `json:"mode"`
	Enabled       bool             `json:"enabled"`
	ForceHTTPS    bool             `json:"force_https"`
	Certificate   *CertificateInfo `json:"certificate,omitempty"`
	ACMEStatus    *ACMEStatusInfo  `json:"acme_status,omitempty"`
	UsingFallback bool             `json:"using_fallback,omitempty"`
}

// CertificateInfo contains details about the current server certificate.
type CertificateInfo struct {
	Subject   string    `json:"subject"`
	Issuer    string    `json:"issuer"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	Serial    string    `json:"serial"`
	DNSNames  []string  `json:"dns_names"`
}

// ACMEStatusInfo contains ACME-specific status information.
type ACMEStatusInfo struct {
	Domains    []string `json:"domains"`
	Email      string   `json:"email"`
	Staging    bool     `json:"staging"`
	TestMode   bool     `json:"test_mode"`
	LastError  string   `json:"last_error,omitempty"`
	NeedsRenew bool     `json:"needs_renewal"`
	DaysLeft   int      `json:"days_remaining"`
}

// handleGetTLSStatus returns the current TLS configuration status.
func (s *MasterServer) handleGetTLSStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	status := &TLSStatus{
		Mode:          string(s.config.Server.TLS.Mode),
		Enabled:       s.config.Server.TLS.Mode != config.TLSModeDisabled,
		ForceHTTPS:    s.config.Server.TLS.ForceHTTPS,
		UsingFallback: s.acmeFallback,
	}

	// Get current certificate info if TLS is enabled
	if s.tlsConfig != nil {
		certInfo := s.getCurrentCertInfo()
		if certInfo != nil {
			status.Certificate = certInfo
		}
	}

	// Get ACME status if applicable
	if s.config.Server.TLS.Mode == config.TLSModeACME && s.acmeClient != nil {
		acmeStatus := s.acmeClient.GetStatus()
		status.ACMEStatus = &ACMEStatusInfo{
			Domains:    s.config.Server.TLS.ACME.Domains,
			Email:      s.config.Server.TLS.ACME.Email,
			Staging:    s.config.Server.TLS.ACME.Staging,
			TestMode:   acmeStatus.TestMode,
			NeedsRenew: acmeStatus.NeedsRenewal,
			DaysLeft:   acmeStatus.DaysRemaining,
		}
	}

	s.jsonResponse(w, status)
}

// handleForceACMERenewal triggers an immediate ACME certificate renewal.
func (s *MasterServer) handleForceACMERenewal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if s.config.Server.TLS.Mode != config.TLSModeACME {
		s.jsonError(w, http.StatusBadRequest, "ACME not enabled")
		return
	}

	if s.acmeClient == nil {
		s.jsonError(w, http.StatusServiceUnavailable, "ACME client not initialized")
		return
	}

	// For autocert-based ACME, certificates are renewed automatically on GetCertificate
	// We can trigger a status check which helps ensure renewal happens
	status := s.acmeClient.GetStatus()

	s.jsonResponse(w, ACMERenewalResponse{
		Status:        "renewal_check_initiated",
		NeedsRenewal:  status.NeedsRenewal,
		DaysRemaining: status.DaysRemaining,
		Message:       "ACME certificates are renewed automatically when needed",
	})
}

// getCurrentCertInfo extracts certificate information from the current TLS config.
func (s *MasterServer) getCurrentCertInfo() *CertificateInfo {
	if s.tlsConfig == nil {
		return nil
	}

	// Try to get certificate through GetCertificate callback
	if s.tlsConfig.GetCertificate != nil {
		// Create a minimal ClientHelloInfo to get the certificate
		cert, err := s.tlsConfig.GetCertificate(nil)
		if err != nil || cert == nil || len(cert.Certificate) == 0 {
			return nil
		}

		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil
		}

		return &CertificateInfo{
			Subject:   x509Cert.Subject.CommonName,
			Issuer:    x509Cert.Issuer.CommonName,
			NotBefore: x509Cert.NotBefore,
			NotAfter:  x509Cert.NotAfter,
			Serial:    x509Cert.SerialNumber.String(),
			DNSNames:  x509Cert.DNSNames,
		}
	}

	// Try static certificates if available
	if len(s.tlsConfig.Certificates) > 0 && len(s.tlsConfig.Certificates[0].Certificate) > 0 {
		x509Cert, err := x509.ParseCertificate(s.tlsConfig.Certificates[0].Certificate[0])
		if err != nil {
			return nil
		}

		return &CertificateInfo{
			Subject:   x509Cert.Subject.CommonName,
			Issuer:    x509Cert.Issuer.CommonName,
			NotBefore: x509Cert.NotBefore,
			NotAfter:  x509Cert.NotAfter,
			Serial:    x509Cert.SerialNumber.String(),
			DNSNames:  x509Cert.DNSNames,
		}
	}

	return nil
}

// TLSSettingsUpdate represents a request to update TLS settings.
// Note: TLS settings changes require server restart to take effect.
type TLSSettingsUpdate struct {
	Mode       string   `json:"mode"`        // "disabled", "static", "acme"
	CertFile   string   `json:"cert_file"`   // For static mode
	KeyFile    string   `json:"key_file"`    // For static mode
	Domains    []string `json:"domains"`     // For ACME mode
	Email      string   `json:"email"`       // For ACME mode
	Staging    bool     `json:"staging"`     // Use Let's Encrypt staging
	ForceHTTPS bool     `json:"force_https"` // Redirect HTTP to HTTPS
}

// handleUpdateTLSSettings updates TLS configuration.
// Note: Changes require server restart to take effect.
func (s *MasterServer) handleUpdateTLSSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// TLS settings are typically managed via config file and require restart.
	// This endpoint returns an informational message.
	s.jsonResponse(w, TLSSettingsInfoResponse{
		Status:  "info",
		Message: "TLS settings are managed via configuration file. Changes require server restart.",
		Current: TLSConfigInfo{
			Mode:       string(s.config.Server.TLS.Mode),
			ForceHTTPS: s.config.Server.TLS.ForceHTTPS,
		},
	})
}

// handleTLSStatusPartial returns an HTML partial for TLS status (used by HTMX).
func (s *MasterServer) handleTLSStatusPartial(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := &TLSStatus{
		Mode:          string(s.config.Server.TLS.Mode),
		Enabled:       s.config.Server.TLS.Mode != config.TLSModeDisabled,
		ForceHTTPS:    s.config.Server.TLS.ForceHTTPS,
		UsingFallback: s.acmeFallback,
	}

	if s.tlsConfig != nil {
		status.Certificate = s.getCurrentCertInfo()
	}

	if s.config.Server.TLS.Mode == config.TLSModeACME && s.acmeClient != nil {
		acmeStatus := s.acmeClient.GetStatus()
		status.ACMEStatus = &ACMEStatusInfo{
			Domains:    s.config.Server.TLS.ACME.Domains,
			Email:      s.config.Server.TLS.ACME.Email,
			Staging:    s.config.Server.TLS.ACME.Staging,
			TestMode:   acmeStatus.TestMode,
			NeedsRenew: acmeStatus.NeedsRenewal,
			DaysLeft:   acmeStatus.DaysRemaining,
		}
	}

	// Return as JSON - the UI can format it
	s.jsonResponse(w, status)
}
