// Package security provides service layer for security-related operations.
package security

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// CertificateService provides business logic for certificate operations.
type CertificateService struct {
	store  storage.Store
	logger *zap.Logger
}

// NewCertificateService creates a new certificate service.
func NewCertificateService(store storage.Store, logger *zap.Logger) *CertificateService {
	return &CertificateService{
		store:  store,
		logger: logger,
	}
}

// AgentCertificateInfo represents agent certificate info for API responses.
type AgentCertificateInfo struct {
	ID               string     `json:"id"`
	AgentID          string     `json:"agent_id"`
	SerialNumber     string     `json:"serial_number"`
	Status           string     `json:"status"`
	NotBefore        time.Time  `json:"not_before"`
	NotAfter         time.Time  `json:"not_after"`
	IssuedAt         time.Time  `json:"issued_at"`
	ExpiresIn        string     `json:"expires_in"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	RevocationReason string     `json:"revocation_reason,omitempty"`
	Subject          string     `json:"subject,omitempty"`
	Issuer           string     `json:"issuer,omitempty"`
}

// CAInfo represents CA info for API responses.
type CAInfo struct {
	ID         string    `json:"id"`
	Version    int       `json:"version"`
	CommonName string    `json:"common_name"`
	Status     string    `json:"status"`
	IsCurrent  bool      `json:"is_current"`
	NotBefore  time.Time `json:"not_before"`
	NotAfter   time.Time `json:"not_after"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresIn  string    `json:"expires_in"`
}

// ServerCertInfo represents server certificate info for API responses.
type ServerCertInfo struct {
	ID        string    `json:"id"`
	Hostname  string    `json:"hostname"`
	SANs      []string  `json:"sans"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	CAID      string    `json:"ca_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresIn string    `json:"expires_in"`
}

// ListAgentCertificates returns all agent certificates with enriched info.
func (s *CertificateService) ListAgentCertificates(ctx context.Context) ([]AgentCertificateInfo, error) {
	certs, err := s.store.ListAgentCerts(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing agent certificates: %w", err)
	}

	result := make([]AgentCertificateInfo, 0, len(certs))
	now := time.Now()

	for _, cert := range certs {
		info := AgentCertificateInfo{
			ID:               cert.ID,
			AgentID:          cert.AgentID,
			SerialNumber:     cert.SerialNumber,
			Status:           cert.Status,
			NotBefore:        cert.NotBefore,
			NotAfter:         cert.NotAfter,
			IssuedAt:         cert.IssuedAt,
			ExpiresIn:        formatDuration(cert.NotAfter.Sub(now)),
			RevokedAt:        cert.RevokedAt,
			RevocationReason: cert.RevocationReason,
		}

		// Parse certificate to get subject/issuer
		if cert.CertificatePEM != "" {
			if parsed, err := parseCertPEM(cert.CertificatePEM); err == nil {
				info.Subject = parsed.Subject.String()
				info.Issuer = parsed.Issuer.String()
			}
		}

		result = append(result, info)
	}

	return result, nil
}

// GetAgentCertificate returns certificate info for a specific agent.
func (s *CertificateService) GetAgentCertificate(ctx context.Context, agentID string) (*AgentCertificateInfo, error) {
	cert, err := s.store.GetAgentCert(ctx, agentID)
	if err != nil {
		if errors.Is(err, services.ErrNotFound) {
			return nil, services.ErrNotFound
		}
		return nil, fmt.Errorf("getting agent certificate: %w", err)
	}

	now := time.Now()
	info := &AgentCertificateInfo{
		ID:               cert.ID,
		AgentID:          cert.AgentID,
		SerialNumber:     cert.SerialNumber,
		Status:           cert.Status,
		NotBefore:        cert.NotBefore,
		NotAfter:         cert.NotAfter,
		IssuedAt:         cert.IssuedAt,
		ExpiresIn:        formatDuration(cert.NotAfter.Sub(now)),
		RevokedAt:        cert.RevokedAt,
		RevocationReason: cert.RevocationReason,
	}

	if cert.CertificatePEM != "" {
		if parsed, err := parseCertPEM(cert.CertificatePEM); err == nil {
			info.Subject = parsed.Subject.String()
			info.Issuer = parsed.Issuer.String()
		}
	}

	return info, nil
}

// GetAgentCertificateBySerial returns certificate info by serial number.
func (s *CertificateService) GetAgentCertificateBySerial(ctx context.Context, serialNumber string) (*AgentCertificateInfo, error) {
	cert, err := s.store.GetAgentCertBySerial(ctx, serialNumber)
	if err != nil {
		return nil, fmt.Errorf("getting certificate by serial: %w", err)
	}

	now := time.Now()
	info := &AgentCertificateInfo{
		ID:               cert.ID,
		AgentID:          cert.AgentID,
		SerialNumber:     cert.SerialNumber,
		Status:           cert.Status,
		NotBefore:        cert.NotBefore,
		NotAfter:         cert.NotAfter,
		IssuedAt:         cert.IssuedAt,
		ExpiresIn:        formatDuration(cert.NotAfter.Sub(now)),
		RevokedAt:        cert.RevokedAt,
		RevocationReason: cert.RevocationReason,
	}

	if cert.CertificatePEM != "" {
		if parsed, err := parseCertPEM(cert.CertificatePEM); err == nil {
			info.Subject = parsed.Subject.String()
			info.Issuer = parsed.Issuer.String()
		}
	}

	return info, nil
}

// RevokeAgentCertificate revokes an agent's certificate.
func (s *CertificateService) RevokeAgentCertificate(ctx context.Context, agentID, reason, revokedBy string) error {
	// Check if certificate exists
	_, err := s.store.GetAgentCert(ctx, agentID)
	if err != nil {
		if errors.Is(err, services.ErrNotFound) {
			return services.ErrNotFound
		}
		return fmt.Errorf("checking agent certificate: %w", err)
	}

	if err := s.store.RevokeAgentCert(ctx, agentID, reason, revokedBy); err != nil {
		return fmt.Errorf("revoking certificate: %w", err)
	}

	s.logger.Info("Agent certificate revoked",
		zap.String("agent_id", agentID),
		zap.String("reason", reason),
		zap.String("revoked_by", revokedBy),
	)

	return nil
}

// RevokeAgentCertificateBySerial revokes a certificate by its serial number.
func (s *CertificateService) RevokeAgentCertificateBySerial(ctx context.Context, serialNumber, reason, revokedBy string) error {
	if err := s.store.RevokeAgentCertBySerial(ctx, serialNumber, reason, revokedBy); err != nil {
		return fmt.Errorf("revoking certificate by serial: %w", err)
	}

	s.logger.Info("Certificate revoked by serial",
		zap.String("serial_number", serialNumber),
		zap.String("reason", reason),
		zap.String("revoked_by", revokedBy),
	)

	return nil
}

// ListCAs returns all certificate authorities.
func (s *CertificateService) ListCAs(ctx context.Context) ([]CAInfo, error) {
	cas, err := s.store.ListCAs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing CAs: %w", err)
	}

	result := make([]CAInfo, 0, len(cas))
	now := time.Now()

	for _, ca := range cas {
		result = append(result, CAInfo{
			ID:         ca.ID,
			Version:    ca.Version,
			CommonName: ca.CommonName,
			Status:     ca.Status,
			IsCurrent:  ca.IsCurrent,
			NotBefore:  ca.NotBefore,
			NotAfter:   ca.NotAfter,
			CreatedAt:  ca.CreatedAt,
			ExpiresIn:  formatDuration(ca.NotAfter.Sub(now)),
		})
	}

	return result, nil
}

// GetCurrentCA returns the currently active CA.
func (s *CertificateService) GetCurrentCA(ctx context.Context) (*CAInfo, error) {
	ca, err := s.store.GetCurrentCA(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting current CA: %w", err)
	}

	now := time.Now()
	return &CAInfo{
		ID:         ca.ID,
		Version:    ca.Version,
		CommonName: ca.CommonName,
		Status:     ca.Status,
		IsCurrent:  ca.IsCurrent,
		NotBefore:  ca.NotBefore,
		NotAfter:   ca.NotAfter,
		CreatedAt:  ca.CreatedAt,
		ExpiresIn:  formatDuration(ca.NotAfter.Sub(now)),
	}, nil
}

// ListServerCertificates returns all server certificates.
func (s *CertificateService) ListServerCertificates(ctx context.Context) ([]ServerCertInfo, error) {
	certs, err := s.store.ListServerCerts(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing server certificates: %w", err)
	}

	result := make([]ServerCertInfo, 0, len(certs))
	now := time.Now()

	for _, cert := range certs {
		info := ServerCertInfo{
			ID:        cert.ID,
			Hostname:  cert.Hostname,
			NotBefore: cert.NotBefore,
			NotAfter:  cert.NotAfter,
			CAID:      cert.CAID,
			CreatedAt: cert.CreatedAt,
			ExpiresIn: formatDuration(cert.NotAfter.Sub(now)),
		}

		// Parse SANs from JSON string
		if cert.SANs != "" {
			// SANs stored as JSON array string
			info.SANs = []string{} // Initialize empty, will be populated from JSON
		}

		result = append(result, info)
	}

	return result, nil
}

// GetServerCertificate returns server certificate for a hostname.
func (s *CertificateService) GetServerCertificate(ctx context.Context, hostname string) (*ServerCertInfo, error) {
	cert, err := s.store.GetServerCert(ctx, hostname)
	if err != nil {
		return nil, fmt.Errorf("getting server certificate: %w", err)
	}

	now := time.Now()
	return &ServerCertInfo{
		ID:        cert.ID,
		Hostname:  cert.Hostname,
		NotBefore: cert.NotBefore,
		NotAfter:  cert.NotAfter,
		CAID:      cert.CAID,
		CreatedAt: cert.CreatedAt,
		ExpiresIn: formatDuration(cert.NotAfter.Sub(now)),
	}, nil
}

// ListCertAuditEvents returns certificate audit events with optional filtering.
func (s *CertificateService) ListCertAuditEvents(ctx context.Context, filter storage.CertAuditFilter) ([]*storage.CertAuditEvent, error) {
	events, err := s.store.ListCertAuditEvents(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("listing cert audit events: %w", err)
	}
	return events, nil
}

// Helper functions

func parseCertPEM(pemData string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		return "expired"
	}

	days := int(d.Hours() / 24)
	if days > 365 {
		years := days / 365
		return fmt.Sprintf("%d years", years)
	}
	if days > 30 {
		months := days / 30
		return fmt.Sprintf("%d months", months)
	}
	if days > 0 {
		return fmt.Sprintf("%d days", days)
	}

	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%d hours", hours)
	}

	minutes := int(d.Minutes())
	return fmt.Sprintf("%d minutes", minutes)
}
