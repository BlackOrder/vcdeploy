// Package security provides security services for vcdeploy.
package security

import (
	"context"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// CertAuditor provides audit logging for certificate operations.
type CertAuditor struct {
	store storage.Store
}

// NewCertAuditor creates a new certificate auditor.
func NewCertAuditor(store storage.Store) *CertAuditor {
	return &CertAuditor{store: store}
}

// LogIssued logs a certificate issuance event.
func (a *CertAuditor) LogIssued(ctx context.Context, agentID, serial, caID, requestedBy, clientIP string) error {
	event := &storage.CertAuditEvent{
		Timestamp:   time.Now(),
		EventType:   storage.CertAuditEventIssued,
		AgentID:     agentID,
		Serial:      serial,
		CAID:        caID,
		RequestedBy: requestedBy,
		ClientIP:    clientIP,
	}
	return a.store.SaveCertAuditEvent(ctx, event)
}

// LogServerCertIssued logs a server certificate issuance event.
func (a *CertAuditor) LogServerCertIssued(ctx context.Context, hostname, serial, caID, requestedBy string) error {
	event := &storage.CertAuditEvent{
		Timestamp:   time.Now(),
		EventType:   storage.CertAuditEventIssued,
		Hostname:    hostname,
		Serial:      serial,
		CAID:        caID,
		RequestedBy: requestedBy,
	}
	return a.store.SaveCertAuditEvent(ctx, event)
}

// LogRevoked logs a certificate revocation event.
func (a *CertAuditor) LogRevoked(ctx context.Context, agentID, serial, caID, reason, requestedBy string) error {
	event := &storage.CertAuditEvent{
		Timestamp:   time.Now(),
		EventType:   storage.CertAuditEventRevoked,
		AgentID:     agentID,
		Serial:      serial,
		CAID:        caID,
		Reason:      reason,
		RequestedBy: requestedBy,
	}
	return a.store.SaveCertAuditEvent(ctx, event)
}

// LogRenewed logs a certificate renewal event.
func (a *CertAuditor) LogRenewed(ctx context.Context, agentID, serial, caID, requestedBy, clientIP string) error {
	event := &storage.CertAuditEvent{
		Timestamp:   time.Now(),
		EventType:   storage.CertAuditEventRenewed,
		AgentID:     agentID,
		Serial:      serial,
		CAID:        caID,
		RequestedBy: requestedBy,
		ClientIP:    clientIP,
	}
	return a.store.SaveCertAuditEvent(ctx, event)
}

// LogRejected logs a certificate request rejection event.
func (a *CertAuditor) LogRejected(ctx context.Context, agentID, caID, reason, requestedBy, clientIP string) error {
	event := &storage.CertAuditEvent{
		Timestamp:   time.Now(),
		EventType:   storage.CertAuditEventRejected,
		AgentID:     agentID,
		Serial:      "", // No serial for rejected requests
		CAID:        caID,
		Reason:      reason,
		RequestedBy: requestedBy,
		ClientIP:    clientIP,
	}
	return a.store.SaveCertAuditEvent(ctx, event)
}

// LogExpired logs a certificate expiration event.
func (a *CertAuditor) LogExpired(ctx context.Context, agentID, serial, caID string) error {
	event := &storage.CertAuditEvent{
		Timestamp:   time.Now(),
		EventType:   storage.CertAuditEventExpired,
		AgentID:     agentID,
		Serial:      serial,
		CAID:        caID,
		RequestedBy: "system",
	}
	return a.store.SaveCertAuditEvent(ctx, event)
}

// GetEvents retrieves audit events with optional filtering.
func (a *CertAuditor) GetEvents(ctx context.Context, filter storage.CertAuditFilter) ([]*storage.CertAuditEvent, error) {
	return a.store.ListCertAuditEvents(ctx, filter)
}

// GetEventsByAgent retrieves audit events for a specific agent.
func (a *CertAuditor) GetEventsByAgent(ctx context.Context, agentID string, limit int) ([]*storage.CertAuditEvent, error) {
	filter := storage.CertAuditFilter{
		AgentID: agentID,
		Limit:   limit,
	}
	return a.store.ListCertAuditEvents(ctx, filter)
}

// GetRecentEvents retrieves recent audit events.
func (a *CertAuditor) GetRecentEvents(ctx context.Context, since time.Time, limit int) ([]*storage.CertAuditEvent, error) {
	filter := storage.CertAuditFilter{
		Since: &since,
		Limit: limit,
	}
	return a.store.ListCertAuditEvents(ctx, filter)
}
