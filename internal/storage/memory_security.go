package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// --- Certificate Authority methods ---

// GetCA returns a CA by ID.
func (s *MemoryStore) GetCA(ctx context.Context, id string) (*CertificateAuthority, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ca, ok := s.certificateAuthorities[id]
	if !ok {
		return nil, ErrNotFound
	}

	// Copy-on-read
	result := *ca
	return &result, nil
}

// GetCurrentCA returns the currently active CA.
func (s *MemoryStore) GetCurrentCA(ctx context.Context) (*CertificateAuthority, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ca := range s.certificateAuthorities {
		if ca.IsCurrent {
			result := *ca
			return &result, nil
		}
	}

	return nil, ErrNotFound
}

// ListCAs returns all certificate authorities.
func (s *MemoryStore) ListCAs(ctx context.Context) ([]*CertificateAuthority, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cas := make([]*CertificateAuthority, 0, len(s.certificateAuthorities))
	for _, ca := range s.certificateAuthorities {
		cp := *ca
		cas = append(cas, &cp)
	}

	return cas, nil
}

// SaveCA creates or updates a certificate authority.
func (s *MemoryStore) SaveCA(ctx context.Context, ca *CertificateAuthority) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy-on-store
	stored := *ca
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = time.Now()
	}

	_, exists := s.certificateAuthorities[ca.ID]
	s.certificateAuthorities[ca.ID] = &stored

	opType := WriteOpInsert
	if exists {
		opType = WriteOpUpdate
	}
	s.queueWrite(s.coreWrites, NewWriteOp(opType, "certificate_authorities", &stored))

	return nil
}

// SetCurrentCA marks a CA as current and deactivates others.
func (s *MemoryStore) SetCurrentCA(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	targetCA, ok := s.certificateAuthorities[id]
	if !ok {
		return ErrNotFound
	}

	now := time.Now()

	// Deactivate all other CAs
	for _, ca := range s.certificateAuthorities {
		if ca.ID != id && ca.IsCurrent {
			ca.IsCurrent = false
			ca.Status = CAStatusInactive
			ca.RotatedAt = &now
			s.queueWrite(s.coreWrites, NewWriteOp(WriteOpUpdate, "certificate_authorities", ca))
		}
	}

	// Activate target CA
	targetCA.IsCurrent = true
	targetCA.Status = CAStatusActive
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpUpdate, "certificate_authorities", targetCA))

	return nil
}

// --- Agent Certificate methods ---

// GetAgentCert returns the active certificate for an agent.
func (s *MemoryStore) GetAgentCert(ctx context.Context, agentID string) (*AgentCertificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, cert := range s.agentCertificates {
		if cert.AgentID == agentID && cert.Status == CertStatusActive {
			result := *cert
			return &result, nil
		}
	}

	return nil, ErrNotFound
}

// GetAgentCertBySerial returns a certificate by serial number.
func (s *MemoryStore) GetAgentCertBySerial(ctx context.Context, serialNumber string) (*AgentCertificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cert, ok := s.agentCertificates[serialNumber]
	if !ok {
		return nil, ErrNotFound
	}

	result := *cert
	return &result, nil
}

// ListAgentCerts returns all agent certificates.
func (s *MemoryStore) ListAgentCerts(ctx context.Context) ([]*AgentCertificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	certs := make([]*AgentCertificate, 0, len(s.agentCertificates))
	for _, cert := range s.agentCertificates {
		cp := *cert
		certs = append(certs, &cp)
	}

	return certs, nil
}

// ListAgentCertsByAgent returns all certificates for an agent.
func (s *MemoryStore) ListAgentCertsByAgent(ctx context.Context, agentID string) ([]*AgentCertificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var certs []*AgentCertificate
	for _, cert := range s.agentCertificates {
		if cert.AgentID == agentID {
			cp := *cert
			certs = append(certs, &cp)
		}
	}

	return certs, nil
}

// SaveAgentCert creates or updates an agent certificate.
func (s *MemoryStore) SaveAgentCert(ctx context.Context, cert *AgentCertificate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy-on-store
	stored := *cert
	if stored.ID == 0 {
		stored.ID = s.nextAgentCertID.Add(1)
	}
	if stored.IssuedAt.IsZero() {
		stored.IssuedAt = time.Now()
	}

	_, exists := s.agentCertificates[cert.SerialNumber]
	s.agentCertificates[cert.SerialNumber] = &stored

	opType := WriteOpInsert
	if exists {
		opType = WriteOpUpdate
	}
	s.queueWrite(s.coreWrites, NewWriteOp(opType, "agent_certificates", &stored))

	// Update the input cert's ID if it was newly assigned
	cert.ID = stored.ID

	return nil
}

// RevokeAgentCert revokes an agent's active certificate.
func (s *MemoryStore) RevokeAgentCert(ctx context.Context, agentID, reason, revokedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	found := false

	for _, cert := range s.agentCertificates {
		if cert.AgentID == agentID && cert.Status == CertStatusActive {
			cert.Status = CertStatusRevoked
			cert.RevokedAt = &now
			cert.RevocationReason = reason
			s.queueWrite(s.coreWrites, NewWriteOp(WriteOpUpdate, "agent_certificates", cert))

			// Add to revocation list
			revoked := &RevokedCertificate{
				ID:           s.nextRevokedCertID.Add(1),
				SerialNumber: cert.SerialNumber,
				AgentID:      agentID,
				Reason:       reason,
				RevokedAt:    now,
				RevokedBy:    revokedBy,
			}
			s.revokedCertificates[cert.SerialNumber] = revoked
			s.queueWrite(s.coreWrites, NewWriteOp(WriteOpInsert, "revoked_certificates", revoked))

			found = true
		}
	}

	if !found {
		return ErrNotFound
	}

	return nil
}

// RevokeAgentCertBySerial revokes a specific certificate by serial.
func (s *MemoryStore) RevokeAgentCertBySerial(ctx context.Context, serialNumber, reason, revokedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cert, ok := s.agentCertificates[serialNumber]
	if !ok {
		return ErrNotFound
	}

	now := time.Now()
	cert.Status = CertStatusRevoked
	cert.RevokedAt = &now
	cert.RevocationReason = reason
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpUpdate, "agent_certificates", cert))

	// Add to revocation list
	revoked := &RevokedCertificate{
		ID:           s.nextRevokedCertID.Add(1),
		SerialNumber: serialNumber,
		AgentID:      cert.AgentID,
		Reason:       reason,
		RevokedAt:    now,
		RevokedBy:    revokedBy,
	}
	s.revokedCertificates[serialNumber] = revoked
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpInsert, "revoked_certificates", revoked))

	return nil
}

// UpdateAgentCertStatus updates certificate status (e.g., mark expired).
func (s *MemoryStore) UpdateAgentCertStatus(ctx context.Context, serialNumber, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cert, ok := s.agentCertificates[serialNumber]
	if !ok {
		return ErrNotFound
	}

	cert.Status = status
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpUpdate, "agent_certificates", cert))

	return nil
}

// --- Server Certificate methods ---

// GetServerCert returns the certificate for a hostname.
func (s *MemoryStore) GetServerCert(ctx context.Context, hostname string) (*ServerCertificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cert, ok := s.serverCertificates[hostname]
	if !ok {
		return nil, ErrNotFound
	}

	result := *cert
	return &result, nil
}

// ListServerCerts returns all server certificates.
func (s *MemoryStore) ListServerCerts(ctx context.Context) ([]*ServerCertificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	certs := make([]*ServerCertificate, 0, len(s.serverCertificates))
	for _, cert := range s.serverCertificates {
		cp := *cert
		certs = append(certs, &cp)
	}

	return certs, nil
}

// SaveServerCert creates or updates a server certificate.
func (s *MemoryStore) SaveServerCert(ctx context.Context, cert *ServerCertificate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy-on-store
	stored := *cert
	if stored.ID == 0 {
		stored.ID = s.nextServerCertID.Add(1)
	}
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = time.Now()
	}

	_, exists := s.serverCertificates[cert.Hostname]
	s.serverCertificates[cert.Hostname] = &stored

	opType := WriteOpInsert
	if exists {
		opType = WriteOpUpdate
	}
	s.queueWrite(s.coreWrites, NewWriteOp(opType, "server_certificates", &stored))

	cert.ID = stored.ID

	return nil
}

// DeleteServerCert removes a server certificate.
func (s *MemoryStore) DeleteServerCert(ctx context.Context, hostname string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cert, ok := s.serverCertificates[hostname]
	if !ok {
		return ErrNotFound
	}

	delete(s.serverCertificates, hostname)
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "server_certificates", cert))

	return nil
}

// --- Registration Token methods ---

// GetRegistrationToken returns a token by its value.
func (s *MemoryStore) GetRegistrationToken(ctx context.Context, token string) (*RegistrationToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rt, ok := s.registrationTokens[token]
	if !ok {
		return nil, ErrNotFound
	}

	result := *rt
	return &result, nil
}

// ListRegistrationTokens returns all registration tokens.
func (s *MemoryStore) ListRegistrationTokens(ctx context.Context) ([]*RegistrationToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tokens := make([]*RegistrationToken, 0, len(s.registrationTokens))
	for _, rt := range s.registrationTokens {
		cp := *rt
		tokens = append(tokens, &cp)
	}

	return tokens, nil
}

// SaveRegistrationToken creates or updates a registration token.
func (s *MemoryStore) SaveRegistrationToken(ctx context.Context, rt *RegistrationToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy-on-store
	stored := *rt
	if stored.ID == 0 {
		stored.ID = s.nextRegTokenID.Add(1)
	}
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = time.Now()
	}

	_, exists := s.registrationTokens[rt.Token]
	s.registrationTokens[rt.Token] = &stored

	opType := WriteOpInsert
	if exists {
		opType = WriteOpUpdate
	}
	s.queueWrite(s.coreWrites, NewWriteOp(opType, "registration_tokens", &stored))

	rt.ID = stored.ID

	return nil
}

// DeleteRegistrationToken removes a registration token.
func (s *MemoryStore) DeleteRegistrationToken(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rt, ok := s.registrationTokens[token]
	if !ok {
		return ErrNotFound
	}

	delete(s.registrationTokens, token)
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "registration_tokens", rt))

	return nil
}

// MarkTokenUsed marks a token as used.
func (s *MemoryStore) MarkTokenUsed(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rt, ok := s.registrationTokens[token]
	if !ok {
		return ErrNotFound
	}

	now := time.Now()
	rt.UsedAt = &now
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpUpdate, "registration_tokens", rt))

	return nil
}

// CleanupExpiredTokens removes expired tokens.
func (s *MemoryStore) CleanupExpiredTokens(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var deleted int64

	for token, rt := range s.registrationTokens {
		if rt.ExpiresAt.Before(now) {
			delete(s.registrationTokens, token)
			s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "registration_tokens", rt))
			deleted++
		}
	}

	return deleted, nil
}

// --- Source Credential methods ---

// GetSourceCredential returns a credential by ID.
func (s *MemoryStore) GetSourceCredential(ctx context.Context, id int64) (*SourceCredential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cred, ok := s.sourceCredentials[id]
	if !ok {
		return nil, ErrNotFound
	}

	result := *cred
	return &result, nil
}

// GetSourceCredentialByName returns a credential by name.
func (s *MemoryStore) GetSourceCredentialByName(ctx context.Context, name string) (*SourceCredential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, cred := range s.sourceCredentials {
		if cred.Name == name {
			result := *cred
			return &result, nil
		}
	}

	return nil, ErrNotFound
}

// ListSourceCredentials returns all source credentials.
func (s *MemoryStore) ListSourceCredentials(ctx context.Context) ([]*SourceCredential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	creds := make([]*SourceCredential, 0, len(s.sourceCredentials))
	for _, cred := range s.sourceCredentials {
		cp := *cred
		creds = append(creds, &cp)
	}

	return creds, nil
}

// SaveSourceCredential creates or updates a source credential.
func (s *MemoryStore) SaveSourceCredential(ctx context.Context, cred *SourceCredential) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy-on-store
	stored := *cred
	isNew := stored.ID == 0
	if isNew {
		stored.ID = s.nextSourceCredID.Add(1)
	}
	now := time.Now()
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now
	}
	stored.UpdatedAt = now

	s.sourceCredentials[stored.ID] = &stored

	opType := WriteOpInsert
	if !isNew {
		opType = WriteOpUpdate
	}
	s.queueWrite(s.coreWrites, NewWriteOp(opType, "source_credentials", &stored))

	cred.ID = stored.ID

	return nil
}

// DeleteSourceCredential removes a source credential.
func (s *MemoryStore) DeleteSourceCredential(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cred, ok := s.sourceCredentials[id]
	if !ok {
		return ErrNotFound
	}

	delete(s.sourceCredentials, id)
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "source_credentials", cred))

	return nil
}

// --- Revoked Certificate methods ---

// IsRevoked checks if a certificate serial is revoked.
func (s *MemoryStore) IsRevoked(ctx context.Context, serialNumber string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.revokedCertificates[serialNumber]
	return ok, nil
}

// ListRevokedCerts returns all revoked certificates.
func (s *MemoryStore) ListRevokedCerts(ctx context.Context) ([]*RevokedCertificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	certs := make([]*RevokedCertificate, 0, len(s.revokedCertificates))
	for _, cert := range s.revokedCertificates {
		cp := *cert
		certs = append(certs, &cp)
	}

	return certs, nil
}

// SaveRevokedCert adds a certificate to the revocation list.
func (s *MemoryStore) SaveRevokedCert(ctx context.Context, revoked *RevokedCertificate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy-on-store
	stored := *revoked
	if stored.ID == 0 {
		stored.ID = s.nextRevokedCertID.Add(1)
	}
	if stored.RevokedAt.IsZero() {
		stored.RevokedAt = time.Now()
	}

	_, exists := s.revokedCertificates[revoked.SerialNumber]
	s.revokedCertificates[revoked.SerialNumber] = &stored

	opType := WriteOpInsert
	if exists {
		opType = WriteOpUpdate
	}
	s.queueWrite(s.coreWrites, NewWriteOp(opType, "revoked_certificates", &stored))

	revoked.ID = stored.ID

	return nil
}

// --- Encryption Key methods ---

// GetEncryptionKey returns a key by ID.
func (s *MemoryStore) GetEncryptionKey(ctx context.Context, id string) (*EncryptionKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, ok := s.encryptionKeys[id]
	if !ok {
		return nil, ErrNotFound
	}

	result := *key
	return &result, nil
}

// GetCurrentEncryptionKey returns the currently active encryption key.
func (s *MemoryStore) GetCurrentEncryptionKey(ctx context.Context) (*EncryptionKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, key := range s.encryptionKeys {
		if key.Status == KeyStatusActive {
			result := *key
			return &result, nil
		}
	}

	return nil, ErrNotFound
}

// ListEncryptionKeys returns all encryption keys.
func (s *MemoryStore) ListEncryptionKeys(ctx context.Context) ([]*EncryptionKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]*EncryptionKey, 0, len(s.encryptionKeys))
	for _, key := range s.encryptionKeys {
		cp := *key
		keys = append(keys, &cp)
	}

	return keys, nil
}

// SaveEncryptionKey creates or updates an encryption key.
func (s *MemoryStore) SaveEncryptionKey(ctx context.Context, key *EncryptionKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy-on-store
	stored := *key
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = time.Now()
	}

	_, exists := s.encryptionKeys[key.ID]
	s.encryptionKeys[key.ID] = &stored

	opType := WriteOpInsert
	if exists {
		opType = WriteOpUpdate
	}
	s.queueWrite(s.coreWrites, NewWriteOp(opType, "encryption_keys", &stored))

	return nil
}

// UpdateEncryptionKeyStatus updates a key's status.
func (s *MemoryStore) UpdateEncryptionKeyStatus(ctx context.Context, id string, status string, scheduledDeletion *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, ok := s.encryptionKeys[id]
	if !ok {
		return ErrNotFound
	}

	key.Status = status
	now := time.Now()

	switch status {
	case KeyStatusActive:
		key.ActivatedAt = &now
	case KeyStatusInactive:
		key.DeactivatedAt = &now
	case KeyStatusScheduled:
		key.ScheduledDeletionAt = scheduledDeletion
	case KeyStatusDeleted:
		// Mark as deleted but retain
	}

	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpUpdate, "encryption_keys", key))

	return nil
}

// --- SSH Key methods ---

// GetSSHKey returns an SSH key by ID.
func (s *MemoryStore) GetSSHKey(ctx context.Context, id int64) (*SSHKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, ok := s.sshKeys[id]
	if !ok {
		return nil, ErrNotFound
	}

	result := *key
	return &result, nil
}

// GetSSHKeyByName returns an SSH key by name.
func (s *MemoryStore) GetSSHKeyByName(ctx context.Context, name string) (*SSHKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, key := range s.sshKeys {
		if key.Name == name {
			result := *key
			return &result, nil
		}
	}

	return nil, ErrNotFound
}

// ListSSHKeys returns all SSH keys.
func (s *MemoryStore) ListSSHKeys(ctx context.Context) ([]*SSHKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]*SSHKey, 0, len(s.sshKeys))
	for _, key := range s.sshKeys {
		cp := *key
		keys = append(keys, &cp)
	}

	return keys, nil
}

// SaveSSHKey creates or updates an SSH key.
func (s *MemoryStore) SaveSSHKey(ctx context.Context, key *SSHKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy-on-store
	stored := *key
	isNew := stored.ID == 0
	if isNew {
		stored.ID = s.nextSSHKeyID.Add(1)
	}
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = time.Now()
	}

	s.sshKeys[stored.ID] = &stored

	opType := WriteOpInsert
	if !isNew {
		opType = WriteOpUpdate
	}
	s.queueWrite(s.coreWrites, NewWriteOp(opType, "ssh_keys", &stored))

	key.ID = stored.ID

	return nil
}

// DeleteSSHKey removes an SSH key.
func (s *MemoryStore) DeleteSSHKey(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, ok := s.sshKeys[id]
	if !ok {
		return ErrNotFound
	}

	delete(s.sshKeys, id)
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "ssh_keys", key))

	return nil
}

// --- Certificate Audit Event methods ---

// SaveCertAuditEvent logs a certificate audit event.
func (s *MemoryStore) SaveCertAuditEvent(ctx context.Context, event *CertAuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy-on-store
	stored := *event
	stored.ID = s.nextCertAuditID.Add(1)
	if stored.Timestamp.IsZero() {
		stored.Timestamp = time.Now()
	}

	s.certAuditEvents = append(s.certAuditEvents, &stored)
	s.queueWrite(s.auditWrites, NewWriteOp(WriteOpInsert, "cert_audit_events", &stored))

	event.ID = stored.ID

	return nil
}

// ListCertAuditEvents returns certificate audit events with optional filtering.
func (s *MemoryStore) ListCertAuditEvents(ctx context.Context, filter CertAuditFilter) ([]*CertAuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var events []*CertAuditEvent

	for _, event := range s.certAuditEvents {
		// Apply filters
		if filter.AgentID != "" && event.AgentID != filter.AgentID {
			continue
		}
		if filter.EventType != "" && event.EventType != filter.EventType {
			continue
		}
		if filter.Since != nil && event.Timestamp.Before(*filter.Since) {
			continue
		}
		if filter.Until != nil && event.Timestamp.After(*filter.Until) {
			continue
		}

		cp := *event
		events = append(events, &cp)
	}

	// Apply offset and limit
	if filter.Offset > 0 {
		if filter.Offset >= len(events) {
			return nil, nil
		}
		events = events[filter.Offset:]
	}
	if filter.Limit > 0 && len(events) > filter.Limit {
		events = events[:filter.Limit]
	}

	return events, nil
}

// --- DB implementation stubs for security stores ---
// These implement the interface but use direct SQL (to be migrated to MemoryStore)

// GetCA returns a CA by ID.
func (db *DB) GetCA(ctx context.Context, id string) (*CertificateAuthority, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, version, common_name, certificate_pem, private_key_encrypted, 
		       not_before, not_after, status, is_current, created_at, rotated_at
		FROM certificate_authorities
		WHERE id = ?
	`, id)

	return scanCA(row)
}

// GetCurrentCA returns the currently active CA.
func (db *DB) GetCurrentCA(ctx context.Context) (*CertificateAuthority, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, version, common_name, certificate_pem, private_key_encrypted, 
		       not_before, not_after, status, is_current, created_at, rotated_at
		FROM certificate_authorities
		WHERE is_current = 1
		LIMIT 1
	`)

	return scanCA(row)
}

// ListCAs returns all certificate authorities.
func (db *DB) ListCAs(ctx context.Context) ([]*CertificateAuthority, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, version, common_name, certificate_pem, private_key_encrypted, 
		       not_before, not_after, status, is_current, created_at, rotated_at
		FROM certificate_authorities
		ORDER BY version DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query CAs: %w", err)
	}
	defer rows.Close()

	var cas []*CertificateAuthority
	for rows.Next() {
		ca, err := scanCARow(rows)
		if err != nil {
			return nil, err
		}
		cas = append(cas, ca)
	}

	return cas, rows.Err()
}

// SaveCA creates or updates a certificate authority.
func (db *DB) SaveCA(ctx context.Context, ca *CertificateAuthority) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT OR REPLACE INTO certificate_authorities 
		(id, version, common_name, certificate_pem, private_key_encrypted, not_before, not_after, status, is_current, created_at, rotated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ca.ID, ca.Version, ca.CommonName, ca.CertificatePEM, ca.PrivateKeyEnc,
		ca.NotBefore, ca.NotAfter, ca.Status, ca.IsCurrent, ca.CreatedAt, ca.RotatedAt)
	return err
}

// SetCurrentCA marks a CA as current and deactivates others.
func (db *DB) SetCurrentCA(ctx context.Context, id string) error {
	return db.RunInTransaction(ctx, func(tx *sql.Tx) error {
		now := time.Now()

		// Deactivate all other CAs
		_, err := tx.ExecContext(ctx, `
			UPDATE certificate_authorities 
			SET is_current = 0, status = ?, rotated_at = ?
			WHERE is_current = 1 AND id != ?
		`, CAStatusInactive, now, id)
		if err != nil {
			return err
		}

		// Activate target CA
		_, err = tx.ExecContext(ctx, `
			UPDATE certificate_authorities 
			SET is_current = 1, status = ?
			WHERE id = ?
		`, CAStatusActive, id)
		return err
	})
}

func scanCA(row *sql.Row) (*CertificateAuthority, error) {
	ca := &CertificateAuthority{}
	var rotatedAt sql.NullTime
	err := row.Scan(
		&ca.ID, &ca.Version, &ca.CommonName, &ca.CertificatePEM, &ca.PrivateKeyEnc,
		&ca.NotBefore, &ca.NotAfter, &ca.Status, &ca.IsCurrent, &ca.CreatedAt, &rotatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan CA: %w", err)
	}
	if rotatedAt.Valid {
		ca.RotatedAt = &rotatedAt.Time
	}
	return ca, nil
}

func scanCARow(rows *sql.Rows) (*CertificateAuthority, error) {
	ca := &CertificateAuthority{}
	var rotatedAt sql.NullTime
	err := rows.Scan(
		&ca.ID, &ca.Version, &ca.CommonName, &ca.CertificatePEM, &ca.PrivateKeyEnc,
		&ca.NotBefore, &ca.NotAfter, &ca.Status, &ca.IsCurrent, &ca.CreatedAt, &rotatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan CA: %w", err)
	}
	if rotatedAt.Valid {
		ca.RotatedAt = &rotatedAt.Time
	}
	return ca, nil
}

// GetAgentCert returns the active certificate for an agent.
func (db *DB) GetAgentCert(ctx context.Context, agentID string) (*AgentCertificate, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, agent_id, ca_id, serial_number, certificate_pem, not_before, not_after, 
		       status, issued_at, renewed_at, revoked_at, revocation_reason
		FROM agent_certificates
		WHERE agent_id = ? AND status = ?
		ORDER BY issued_at DESC
		LIMIT 1
	`, agentID, CertStatusActive)

	return scanAgentCert(row)
}

// GetAgentCertBySerial returns a certificate by serial number.
func (db *DB) GetAgentCertBySerial(ctx context.Context, serialNumber string) (*AgentCertificate, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, agent_id, ca_id, serial_number, certificate_pem, not_before, not_after, 
		       status, issued_at, renewed_at, revoked_at, revocation_reason
		FROM agent_certificates
		WHERE serial_number = ?
	`, serialNumber)

	return scanAgentCert(row)
}

// ListAgentCerts returns all agent certificates.
func (db *DB) ListAgentCerts(ctx context.Context) ([]*AgentCertificate, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, agent_id, ca_id, serial_number, certificate_pem, not_before, not_after, 
		       status, issued_at, renewed_at, revoked_at, revocation_reason
		FROM agent_certificates
		ORDER BY issued_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query certs: %w", err)
	}
	defer rows.Close()

	var certs []*AgentCertificate
	for rows.Next() {
		cert, err := scanAgentCertRow(rows)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}

	return certs, rows.Err()
}

// ListAgentCertsByAgent returns all certificates for an agent.
func (db *DB) ListAgentCertsByAgent(ctx context.Context, agentID string) ([]*AgentCertificate, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, agent_id, ca_id, serial_number, certificate_pem, not_before, not_after, 
		       status, issued_at, renewed_at, revoked_at, revocation_reason
		FROM agent_certificates
		WHERE agent_id = ?
		ORDER BY issued_at DESC
	`, agentID)
	if err != nil {
		return nil, fmt.Errorf("query certs: %w", err)
	}
	defer rows.Close()

	var certs []*AgentCertificate
	for rows.Next() {
		cert, err := scanAgentCertRow(rows)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}

	return certs, rows.Err()
}

// SaveAgentCert creates or updates an agent certificate.
func (db *DB) SaveAgentCert(ctx context.Context, cert *AgentCertificate) error {
	if cert.ID == 0 {
		result, err := db.conn.ExecContext(ctx, `
			INSERT INTO agent_certificates 
			(agent_id, ca_id, serial_number, certificate_pem, not_before, not_after, status, issued_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, cert.AgentID, cert.CAID, cert.SerialNumber, cert.CertificatePEM,
			cert.NotBefore, cert.NotAfter, cert.Status, cert.IssuedAt)
		if err != nil {
			return err
		}
		id, _ := result.LastInsertId()
		cert.ID = id
		return nil
	}

	_, err := db.conn.ExecContext(ctx, `
		UPDATE agent_certificates 
		SET agent_id = ?, ca_id = ?, serial_number = ?, certificate_pem = ?, 
		    not_before = ?, not_after = ?, status = ?, issued_at = ?,
		    renewed_at = ?, revoked_at = ?, revocation_reason = ?
		WHERE id = ?
	`, cert.AgentID, cert.CAID, cert.SerialNumber, cert.CertificatePEM,
		cert.NotBefore, cert.NotAfter, cert.Status, cert.IssuedAt,
		cert.RenewedAt, cert.RevokedAt, cert.RevocationReason, cert.ID)
	return err
}

// RevokeAgentCert revokes an agent's certificate.
func (db *DB) RevokeAgentCert(ctx context.Context, agentID, reason, revokedBy string) error {
	now := time.Now()
	result, err := db.conn.ExecContext(ctx, `
		UPDATE agent_certificates 
		SET status = ?, revoked_at = ?, revocation_reason = ?
		WHERE agent_id = ? AND status = ?
	`, CertStatusRevoked, now, reason, agentID, CertStatusActive)
	if err != nil {
		return err
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

// RevokeAgentCertBySerial revokes a specific certificate by serial.
func (db *DB) RevokeAgentCertBySerial(ctx context.Context, serialNumber, reason, revokedBy string) error {
	now := time.Now()
	result, err := db.conn.ExecContext(ctx, `
		UPDATE agent_certificates 
		SET status = ?, revoked_at = ?, revocation_reason = ?
		WHERE serial_number = ?
	`, CertStatusRevoked, now, reason, serialNumber)
	if err != nil {
		return err
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdateAgentCertStatus updates certificate status.
func (db *DB) UpdateAgentCertStatus(ctx context.Context, serialNumber, status string) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE agent_certificates SET status = ? WHERE serial_number = ?
	`, status, serialNumber)
	return err
}

func scanAgentCert(row *sql.Row) (*AgentCertificate, error) {
	cert := &AgentCertificate{}
	var renewedAt, revokedAt sql.NullTime
	var revocationReason sql.NullString
	err := row.Scan(
		&cert.ID, &cert.AgentID, &cert.CAID, &cert.SerialNumber, &cert.CertificatePEM,
		&cert.NotBefore, &cert.NotAfter, &cert.Status, &cert.IssuedAt,
		&renewedAt, &revokedAt, &revocationReason,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan cert: %w", err)
	}
	if renewedAt.Valid {
		cert.RenewedAt = &renewedAt.Time
	}
	if revokedAt.Valid {
		cert.RevokedAt = &revokedAt.Time
	}
	if revocationReason.Valid {
		cert.RevocationReason = revocationReason.String
	}
	return cert, nil
}

func scanAgentCertRow(rows *sql.Rows) (*AgentCertificate, error) {
	cert := &AgentCertificate{}
	var renewedAt, revokedAt sql.NullTime
	var revocationReason sql.NullString
	err := rows.Scan(
		&cert.ID, &cert.AgentID, &cert.CAID, &cert.SerialNumber, &cert.CertificatePEM,
		&cert.NotBefore, &cert.NotAfter, &cert.Status, &cert.IssuedAt,
		&renewedAt, &revokedAt, &revocationReason,
	)
	if err != nil {
		return nil, fmt.Errorf("scan cert: %w", err)
	}
	if renewedAt.Valid {
		cert.RenewedAt = &renewedAt.Time
	}
	if revokedAt.Valid {
		cert.RevokedAt = &revokedAt.Time
	}
	if revocationReason.Valid {
		cert.RevocationReason = revocationReason.String
	}
	return cert, nil
}

// GetServerCert returns the certificate for a hostname.
func (db *DB) GetServerCert(ctx context.Context, hostname string) (*ServerCertificate, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, hostname, certificate_pem, private_key_encrypted, sans, not_before, not_after, ca_id, created_at
		FROM server_certificates
		WHERE hostname = ?
	`, hostname)

	cert := &ServerCertificate{}
	err := row.Scan(
		&cert.ID, &cert.Hostname, &cert.CertificatePEM, &cert.PrivateKeyEnc,
		&cert.SANs, &cert.NotBefore, &cert.NotAfter, &cert.CAID, &cert.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan server cert: %w", err)
	}
	return cert, nil
}

// ListServerCerts returns all server certificates.
func (db *DB) ListServerCerts(ctx context.Context) ([]*ServerCertificate, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, hostname, certificate_pem, private_key_encrypted, sans, not_before, not_after, ca_id, created_at
		FROM server_certificates
	`)
	if err != nil {
		return nil, fmt.Errorf("query server certs: %w", err)
	}
	defer rows.Close()

	var certs []*ServerCertificate
	for rows.Next() {
		cert := &ServerCertificate{}
		err := rows.Scan(
			&cert.ID, &cert.Hostname, &cert.CertificatePEM, &cert.PrivateKeyEnc,
			&cert.SANs, &cert.NotBefore, &cert.NotAfter, &cert.CAID, &cert.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan server cert: %w", err)
		}
		certs = append(certs, cert)
	}

	return certs, rows.Err()
}

// SaveServerCert creates or updates a server certificate.
func (db *DB) SaveServerCert(ctx context.Context, cert *ServerCertificate) error {
	if cert.ID == 0 {
		result, err := db.conn.ExecContext(ctx, `
			INSERT INTO server_certificates 
			(hostname, certificate_pem, private_key_encrypted, sans, not_before, not_after, ca_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, cert.Hostname, cert.CertificatePEM, cert.PrivateKeyEnc,
			cert.SANs, cert.NotBefore, cert.NotAfter, cert.CAID, cert.CreatedAt)
		if err != nil {
			return err
		}
		id, _ := result.LastInsertId()
		cert.ID = id
		return nil
	}

	_, err := db.conn.ExecContext(ctx, `
		UPDATE server_certificates 
		SET certificate_pem = ?, private_key_encrypted = ?, sans = ?, 
		    not_before = ?, not_after = ?, ca_id = ?
		WHERE id = ?
	`, cert.CertificatePEM, cert.PrivateKeyEnc, cert.SANs,
		cert.NotBefore, cert.NotAfter, cert.CAID, cert.ID)
	return err
}

// DeleteServerCert removes a server certificate.
func (db *DB) DeleteServerCert(ctx context.Context, hostname string) error {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM server_certificates WHERE hostname = ?`, hostname)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetRegistrationToken returns a token by its value.
func (db *DB) GetRegistrationToken(ctx context.Context, token string) (*RegistrationToken, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, token, agent_id, expires_at, used_at, created_by, created_at
		FROM registration_tokens
		WHERE token = ?
	`, token)

	rt := &RegistrationToken{}
	var usedAt sql.NullTime
	var agentID sql.NullString
	err := row.Scan(&rt.ID, &rt.Token, &agentID, &rt.ExpiresAt, &usedAt, &rt.CreatedBy, &rt.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan token: %w", err)
	}
	if usedAt.Valid {
		rt.UsedAt = &usedAt.Time
	}
	if agentID.Valid {
		rt.AgentID = agentID.String
	}
	return rt, nil
}

// ListRegistrationTokens returns all registration tokens.
func (db *DB) ListRegistrationTokens(ctx context.Context) ([]*RegistrationToken, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, token, agent_id, expires_at, used_at, created_by, created_at
		FROM registration_tokens
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*RegistrationToken
	for rows.Next() {
		rt := &RegistrationToken{}
		var usedAt sql.NullTime
		var agentID sql.NullString
		err := rows.Scan(&rt.ID, &rt.Token, &agentID, &rt.ExpiresAt, &usedAt, &rt.CreatedBy, &rt.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		if usedAt.Valid {
			rt.UsedAt = &usedAt.Time
		}
		if agentID.Valid {
			rt.AgentID = agentID.String
		}
		tokens = append(tokens, rt)
	}

	return tokens, rows.Err()
}

// SaveRegistrationToken creates or updates a registration token.
func (db *DB) SaveRegistrationToken(ctx context.Context, rt *RegistrationToken) error {
	if rt.ID == 0 {
		result, err := db.conn.ExecContext(ctx, `
			INSERT INTO registration_tokens (token, agent_id, expires_at, created_by, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, rt.Token, nullString(rt.AgentID), rt.ExpiresAt, rt.CreatedBy, rt.CreatedAt)
		if err != nil {
			return err
		}
		id, _ := result.LastInsertId()
		rt.ID = id
		return nil
	}

	_, err := db.conn.ExecContext(ctx, `
		UPDATE registration_tokens 
		SET agent_id = ?, expires_at = ?, used_at = ?
		WHERE id = ?
	`, nullString(rt.AgentID), rt.ExpiresAt, rt.UsedAt, rt.ID)
	return err
}

// DeleteRegistrationToken removes a registration token.
func (db *DB) DeleteRegistrationToken(ctx context.Context, token string) error {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM registration_tokens WHERE token = ?`, token)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkTokenUsed marks a token as used.
func (db *DB) MarkTokenUsed(ctx context.Context, token string) error {
	now := time.Now()
	_, err := db.conn.ExecContext(ctx, `
		UPDATE registration_tokens SET used_at = ? WHERE token = ?
	`, now, token)
	return err
}

// CleanupExpiredTokens removes expired tokens.
func (db *DB) CleanupExpiredTokens(ctx context.Context) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM registration_tokens WHERE expires_at < ?
	`, time.Now())
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

// GetSourceCredential returns a credential by ID.
func (db *DB) GetSourceCredential(ctx context.Context, id int64) (*SourceCredential, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, name, type, url_pattern, credential_encrypted, created_by, created_at, updated_at
		FROM source_credentials
		WHERE id = ?
	`, id)

	cred := &SourceCredential{}
	err := row.Scan(&cred.ID, &cred.Name, &cred.Type, &cred.URLPattern, &cred.CredentialEnc,
		&cred.CreatedBy, &cred.CreatedAt, &cred.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan credential: %w", err)
	}
	return cred, nil
}

// GetSourceCredentialByName returns a credential by name.
func (db *DB) GetSourceCredentialByName(ctx context.Context, name string) (*SourceCredential, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, name, type, url_pattern, credential_encrypted, created_by, created_at, updated_at
		FROM source_credentials
		WHERE name = ?
	`, name)

	cred := &SourceCredential{}
	err := row.Scan(&cred.ID, &cred.Name, &cred.Type, &cred.URLPattern, &cred.CredentialEnc,
		&cred.CreatedBy, &cred.CreatedAt, &cred.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan credential: %w", err)
	}
	return cred, nil
}

// ListSourceCredentials returns all source credentials.
func (db *DB) ListSourceCredentials(ctx context.Context) ([]*SourceCredential, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, name, type, url_pattern, credential_encrypted, created_by, created_at, updated_at
		FROM source_credentials
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("query credentials: %w", err)
	}
	defer rows.Close()

	var creds []*SourceCredential
	for rows.Next() {
		cred := &SourceCredential{}
		err := rows.Scan(&cred.ID, &cred.Name, &cred.Type, &cred.URLPattern, &cred.CredentialEnc,
			&cred.CreatedBy, &cred.CreatedAt, &cred.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan credential: %w", err)
		}
		creds = append(creds, cred)
	}

	return creds, rows.Err()
}

// SaveSourceCredential creates or updates a source credential.
func (db *DB) SaveSourceCredential(ctx context.Context, cred *SourceCredential) error {
	now := time.Now()
	if cred.ID == 0 {
		cred.CreatedAt = now
		cred.UpdatedAt = now
		result, err := db.conn.ExecContext(ctx, `
			INSERT INTO source_credentials (name, type, url_pattern, credential_encrypted, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, cred.Name, cred.Type, cred.URLPattern, cred.CredentialEnc, cred.CreatedBy, cred.CreatedAt, cred.UpdatedAt)
		if err != nil {
			return err
		}
		id, _ := result.LastInsertId()
		cred.ID = id
		return nil
	}

	cred.UpdatedAt = now
	_, err := db.conn.ExecContext(ctx, `
		UPDATE source_credentials 
		SET name = ?, type = ?, url_pattern = ?, credential_encrypted = ?, updated_at = ?
		WHERE id = ?
	`, cred.Name, cred.Type, cred.URLPattern, cred.CredentialEnc, cred.UpdatedAt, cred.ID)
	return err
}

// DeleteSourceCredential removes a source credential.
func (db *DB) DeleteSourceCredential(ctx context.Context, id int64) error {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM source_credentials WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// IsRevoked checks if a certificate serial is revoked.
func (db *DB) IsRevoked(ctx context.Context, serialNumber string) (bool, error) {
	var count int
	err := db.conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM revoked_certificates WHERE serial_number = ?
	`, serialNumber).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListRevokedCerts returns all revoked certificates.
func (db *DB) ListRevokedCerts(ctx context.Context) ([]*RevokedCertificate, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, serial_number, agent_id, reason, revoked_at, revoked_by
		FROM revoked_certificates
		ORDER BY revoked_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query revoked: %w", err)
	}
	defer rows.Close()

	var certs []*RevokedCertificate
	for rows.Next() {
		cert := &RevokedCertificate{}
		var agentID sql.NullString
		err := rows.Scan(&cert.ID, &cert.SerialNumber, &agentID, &cert.Reason, &cert.RevokedAt, &cert.RevokedBy)
		if err != nil {
			return nil, fmt.Errorf("scan revoked: %w", err)
		}
		if agentID.Valid {
			cert.AgentID = agentID.String
		}
		certs = append(certs, cert)
	}

	return certs, rows.Err()
}

// SaveRevokedCert adds a certificate to the revocation list.
func (db *DB) SaveRevokedCert(ctx context.Context, revoked *RevokedCertificate) error {
	if revoked.RevokedAt.IsZero() {
		revoked.RevokedAt = time.Now()
	}
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO revoked_certificates (serial_number, agent_id, reason, revoked_at, revoked_by)
		VALUES (?, ?, ?, ?, ?)
	`, revoked.SerialNumber, nullString(revoked.AgentID), revoked.Reason, revoked.RevokedAt, revoked.RevokedBy)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	revoked.ID = id
	return nil
}

// GetEncryptionKey returns a key by ID.
func (db *DB) GetEncryptionKey(ctx context.Context, id string) (*EncryptionKey, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, version, key_material_encrypted, algorithm, status, created_at, 
		       activated_at, deactivated_at, scheduled_deletion_at, deletion_cancelled_at
		FROM encryption_keys
		WHERE id = ?
	`, id)

	return scanEncryptionKey(row)
}

// GetCurrentEncryptionKey returns the currently active encryption key.
func (db *DB) GetCurrentEncryptionKey(ctx context.Context) (*EncryptionKey, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, version, key_material_encrypted, algorithm, status, created_at, 
		       activated_at, deactivated_at, scheduled_deletion_at, deletion_cancelled_at
		FROM encryption_keys
		WHERE status = ?
		LIMIT 1
	`, KeyStatusActive)

	return scanEncryptionKey(row)
}

// ListEncryptionKeys returns all encryption keys.
func (db *DB) ListEncryptionKeys(ctx context.Context) ([]*EncryptionKey, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, version, key_material_encrypted, algorithm, status, created_at, 
		       activated_at, deactivated_at, scheduled_deletion_at, deletion_cancelled_at
		FROM encryption_keys
		ORDER BY version DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query keys: %w", err)
	}
	defer rows.Close()

	var keys []*EncryptionKey
	for rows.Next() {
		key, err := scanEncryptionKeyRow(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	return keys, rows.Err()
}

// SaveEncryptionKey creates or updates an encryption key.
func (db *DB) SaveEncryptionKey(ctx context.Context, key *EncryptionKey) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT OR REPLACE INTO encryption_keys 
		(id, version, key_material_encrypted, algorithm, status, created_at, activated_at, deactivated_at, scheduled_deletion_at, deletion_cancelled_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, key.ID, key.Version, key.KeyMaterialEnc, key.Algorithm, key.Status, key.CreatedAt,
		key.ActivatedAt, key.DeactivatedAt, key.ScheduledDeletionAt, key.DeletionCancelledAt)
	return err
}

// UpdateEncryptionKeyStatus updates a key's status.
func (db *DB) UpdateEncryptionKeyStatus(ctx context.Context, id string, status string, scheduledDeletion *time.Time) error {
	now := time.Now()

	var err error
	switch status {
	case KeyStatusActive:
		_, err = db.conn.ExecContext(ctx, `
			UPDATE encryption_keys SET status = ?, activated_at = ? WHERE id = ?
		`, status, now, id)
	case KeyStatusInactive:
		_, err = db.conn.ExecContext(ctx, `
			UPDATE encryption_keys SET status = ?, deactivated_at = ? WHERE id = ?
		`, status, now, id)
	case KeyStatusScheduled:
		_, err = db.conn.ExecContext(ctx, `
			UPDATE encryption_keys SET status = ?, scheduled_deletion_at = ? WHERE id = ?
		`, status, scheduledDeletion, id)
	default:
		_, err = db.conn.ExecContext(ctx, `
			UPDATE encryption_keys SET status = ? WHERE id = ?
		`, status, id)
	}
	return err
}

func scanEncryptionKey(row *sql.Row) (*EncryptionKey, error) {
	key := &EncryptionKey{}
	var activatedAt, deactivatedAt, scheduledDeletionAt, deletionCancelledAt sql.NullTime
	err := row.Scan(
		&key.ID, &key.Version, &key.KeyMaterialEnc, &key.Algorithm, &key.Status, &key.CreatedAt,
		&activatedAt, &deactivatedAt, &scheduledDeletionAt, &deletionCancelledAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan key: %w", err)
	}
	if activatedAt.Valid {
		key.ActivatedAt = &activatedAt.Time
	}
	if deactivatedAt.Valid {
		key.DeactivatedAt = &deactivatedAt.Time
	}
	if scheduledDeletionAt.Valid {
		key.ScheduledDeletionAt = &scheduledDeletionAt.Time
	}
	if deletionCancelledAt.Valid {
		key.DeletionCancelledAt = &deletionCancelledAt.Time
	}
	return key, nil
}

func scanEncryptionKeyRow(rows *sql.Rows) (*EncryptionKey, error) {
	key := &EncryptionKey{}
	var activatedAt, deactivatedAt, scheduledDeletionAt, deletionCancelledAt sql.NullTime
	err := rows.Scan(
		&key.ID, &key.Version, &key.KeyMaterialEnc, &key.Algorithm, &key.Status, &key.CreatedAt,
		&activatedAt, &deactivatedAt, &scheduledDeletionAt, &deletionCancelledAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan key: %w", err)
	}
	if activatedAt.Valid {
		key.ActivatedAt = &activatedAt.Time
	}
	if deactivatedAt.Valid {
		key.DeactivatedAt = &deactivatedAt.Time
	}
	if scheduledDeletionAt.Valid {
		key.ScheduledDeletionAt = &scheduledDeletionAt.Time
	}
	if deletionCancelledAt.Valid {
		key.DeletionCancelledAt = &deletionCancelledAt.Time
	}
	return key, nil
}

// GetSSHKey returns an SSH key by ID.
func (db *DB) GetSSHKey(ctx context.Context, id int64) (*SSHKey, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, name, public_key, private_key_encrypted, fingerprint, key_type, created_by, created_at
		FROM ssh_keys
		WHERE id = ?
	`, id)

	key := &SSHKey{}
	err := row.Scan(&key.ID, &key.Name, &key.PublicKey, &key.PrivateKeyEnc, &key.Fingerprint,
		&key.KeyType, &key.CreatedBy, &key.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan SSH key: %w", err)
	}
	return key, nil
}

// GetSSHKeyByName returns an SSH key by name.
func (db *DB) GetSSHKeyByName(ctx context.Context, name string) (*SSHKey, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, name, public_key, private_key_encrypted, fingerprint, key_type, created_by, created_at
		FROM ssh_keys
		WHERE name = ?
	`, name)

	key := &SSHKey{}
	err := row.Scan(&key.ID, &key.Name, &key.PublicKey, &key.PrivateKeyEnc, &key.Fingerprint,
		&key.KeyType, &key.CreatedBy, &key.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan SSH key: %w", err)
	}
	return key, nil
}

// ListSSHKeys returns all SSH keys.
func (db *DB) ListSSHKeys(ctx context.Context) ([]*SSHKey, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, name, public_key, private_key_encrypted, fingerprint, key_type, created_by, created_at
		FROM ssh_keys
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("query SSH keys: %w", err)
	}
	defer rows.Close()

	var keys []*SSHKey
	for rows.Next() {
		key := &SSHKey{}
		err := rows.Scan(&key.ID, &key.Name, &key.PublicKey, &key.PrivateKeyEnc, &key.Fingerprint,
			&key.KeyType, &key.CreatedBy, &key.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan SSH key: %w", err)
		}
		keys = append(keys, key)
	}

	return keys, rows.Err()
}

// SaveSSHKey creates or updates an SSH key.
func (db *DB) SaveSSHKey(ctx context.Context, key *SSHKey) error {
	if key.ID == 0 {
		if key.CreatedAt.IsZero() {
			key.CreatedAt = time.Now()
		}
		result, err := db.conn.ExecContext(ctx, `
			INSERT INTO ssh_keys (name, public_key, private_key_encrypted, fingerprint, key_type, created_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, key.Name, key.PublicKey, key.PrivateKeyEnc, key.Fingerprint, key.KeyType, key.CreatedBy, key.CreatedAt)
		if err != nil {
			return err
		}
		id, _ := result.LastInsertId()
		key.ID = id
		return nil
	}

	_, err := db.conn.ExecContext(ctx, `
		UPDATE ssh_keys 
		SET name = ?, public_key = ?, private_key_encrypted = ?, fingerprint = ?, key_type = ?
		WHERE id = ?
	`, key.Name, key.PublicKey, key.PrivateKeyEnc, key.Fingerprint, key.KeyType, key.ID)
	return err
}

// DeleteSSHKey removes an SSH key.
func (db *DB) DeleteSSHKey(ctx context.Context, id int64) error {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM ssh_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// SaveCertAuditEvent logs a certificate audit event.
func (db *DB) SaveCertAuditEvent(ctx context.Context, event *CertAuditEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO cert_audit_events (timestamp, event_type, agent_id, hostname, serial, ca_id, reason, requested_by, client_ip)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.Timestamp, event.EventType, nullString(event.AgentID), nullString(event.Hostname),
		event.Serial, event.CAID, nullString(event.Reason), event.RequestedBy, nullString(event.ClientIP))
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	event.ID = id
	return nil
}

// ListCertAuditEvents returns certificate audit events with optional filtering.
func (db *DB) ListCertAuditEvents(ctx context.Context, filter CertAuditFilter) ([]*CertAuditEvent, error) {
	query := `
		SELECT id, timestamp, event_type, agent_id, hostname, serial, ca_id, reason, requested_by, client_ip
		FROM cert_audit_events
		WHERE 1=1
	`
	var args []any

	if filter.AgentID != "" {
		query += " AND agent_id = ?"
		args = append(args, filter.AgentID)
	}
	if filter.EventType != "" {
		query += " AND event_type = ?"
		args = append(args, filter.EventType)
	}
	if filter.Since != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.Since)
	}
	if filter.Until != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.Until)
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit events: %w", err)
	}
	defer rows.Close()

	var events []*CertAuditEvent
	for rows.Next() {
		event := &CertAuditEvent{}
		var agentID, hostname, reason, clientIP sql.NullString
		err := rows.Scan(&event.ID, &event.Timestamp, &event.EventType, &agentID, &hostname,
			&event.Serial, &event.CAID, &reason, &event.RequestedBy, &clientIP)
		if err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		if agentID.Valid {
			event.AgentID = agentID.String
		}
		if hostname.Valid {
			event.Hostname = hostname.String
		}
		if reason.Valid {
			event.Reason = reason.String
		}
		if clientIP.Valid {
			event.ClientIP = clientIP.String
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

// nullString converts a string to sql.NullString, returning NULL for empty strings.
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// --- ACME Certificate methods ---

// GetACMECertificate retrieves an ACME certificate by domain.
func (s *MemoryStore) GetACMECertificate(ctx context.Context, domain string) (*ACMECertificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cert, ok := s.acmeCertificates[domain]
	if !ok {
		return nil, ErrNotFound
	}

	// Copy-on-read
	result := *cert
	return &result, nil
}

// SaveACMECertificate creates or updates an ACME certificate.
func (s *MemoryStore) SaveACMECertificate(ctx context.Context, cert *ACMECertificate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	stored := *cert

	existing, exists := s.acmeCertificates[cert.Domain]
	if !exists {
		stored.ID = s.nextACMECertID.Add(1)
		stored.CreatedAt = now
		stored.UpdatedAt = now
	} else {
		stored.ID = existing.ID
		stored.CreatedAt = existing.CreatedAt
		stored.UpdatedAt = now
	}

	s.acmeCertificates[cert.Domain] = &stored
	cert.ID = stored.ID

	opType := WriteOpInsert
	if exists {
		opType = WriteOpUpdate
	}
	s.queueWrite(s.coreWrites, NewWriteOp(opType, "acme_certificates", &stored))

	return nil
}

// DeleteACMECertificate removes an ACME certificate by domain.
func (s *MemoryStore) DeleteACMECertificate(ctx context.Context, domain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cert, ok := s.acmeCertificates[domain]
	if !ok {
		return ErrNotFound
	}

	delete(s.acmeCertificates, domain)
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "acme_certificates", cert))

	return nil
}

// ListACMECertificates returns all stored ACME certificates.
func (s *MemoryStore) ListACMECertificates(ctx context.Context) ([]*ACMECertificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	certs := make([]*ACMECertificate, 0, len(s.acmeCertificates))
	for _, cert := range s.acmeCertificates {
		cp := *cert
		certs = append(certs, &cp)
	}

	return certs, nil
}

// --- ACME Account methods ---

// GetACMEAccount retrieves an ACME account by email.
func (s *MemoryStore) GetACMEAccount(ctx context.Context, email string) (*ACMEAccount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	account, ok := s.acmeAccounts[email]
	if !ok {
		return nil, ErrNotFound
	}

	// Copy-on-read
	result := *account
	return &result, nil
}

// SaveACMEAccount creates or updates an ACME account.
func (s *MemoryStore) SaveACMEAccount(ctx context.Context, account *ACMEAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	stored := *account

	existing, exists := s.acmeAccounts[account.Email]
	if !exists {
		stored.ID = s.nextACMEAccountID.Add(1)
		stored.CreatedAt = now
	} else {
		stored.ID = existing.ID
		stored.CreatedAt = existing.CreatedAt
	}

	s.acmeAccounts[account.Email] = &stored
	account.ID = stored.ID

	opType := WriteOpInsert
	if exists {
		opType = WriteOpUpdate
	}
	s.queueWrite(s.coreWrites, NewWriteOp(opType, "acme_accounts", &stored))

	return nil
}

// DeleteACMEAccount removes an ACME account by email.
func (s *MemoryStore) DeleteACMEAccount(ctx context.Context, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	account, ok := s.acmeAccounts[email]
	if !ok {
		return ErrNotFound
	}

	delete(s.acmeAccounts, email)
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "acme_accounts", account))

	return nil
}

// --- Recovery Code methods ---

// SaveRecoveryCodes saves a set of recovery codes for a user (replaces any existing).
func (s *MemoryStore) SaveRecoveryCodes(ctx context.Context, userID int64, codes []*RecoveryCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Delete existing codes for this user first
	if existing, ok := s.recoveryCodes[userID]; ok {
		for _, code := range existing {
			s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "recovery_codes", code))
		}
	}

	// Save new codes
	newCodes := make([]*RecoveryCode, len(codes))
	for i, code := range codes {
		stored := *code
		stored.ID = s.nextRecoveryCodeID.Add(1)
		stored.UserID = userID
		stored.CreatedAt = now
		newCodes[i] = &stored
		codes[i].ID = stored.ID
		s.queueWrite(s.coreWrites, NewWriteOp(WriteOpInsert, "recovery_codes", &stored))
	}

	s.recoveryCodes[userID] = newCodes
	return nil
}

// GetRecoveryCodes returns all recovery codes for a user.
func (s *MemoryStore) GetRecoveryCodes(ctx context.Context, userID int64) ([]*RecoveryCode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	codes, ok := s.recoveryCodes[userID]
	if !ok {
		return []*RecoveryCode{}, nil
	}

	// Copy-on-read
	result := make([]*RecoveryCode, len(codes))
	for i, c := range codes {
		cp := *c
		result[i] = &cp
	}
	return result, nil
}

// UseRecoveryCode marks a recovery code as used.
func (s *MemoryStore) UseRecoveryCode(ctx context.Context, codeID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Find the code across all users
	for userID, codes := range s.recoveryCodes {
		for i, code := range codes {
			if code.ID == codeID {
				if code.UsedAt != nil {
					return fmt.Errorf("recovery code already used")
				}
				code.UsedAt = &now
				s.recoveryCodes[userID][i] = code
				s.queueWrite(s.coreWrites, NewWriteOp(WriteOpUpdate, "recovery_codes", code))
				return nil
			}
		}
	}

	return ErrNotFound
}

// DeleteRecoveryCodes removes all recovery codes for a user.
func (s *MemoryStore) DeleteRecoveryCodes(ctx context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	codes, ok := s.recoveryCodes[userID]
	if !ok {
		return nil // No codes to delete
	}

	for _, code := range codes {
		s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "recovery_codes", code))
	}

	delete(s.recoveryCodes, userID)
	return nil
}

// CountUnusedRecoveryCodes returns the count of unused codes for a user.
func (s *MemoryStore) CountUnusedRecoveryCodes(ctx context.Context, userID int64) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	codes, ok := s.recoveryCodes[userID]
	if !ok {
		return 0, nil
	}

	count := 0
	for _, code := range codes {
		if code.UsedAt == nil {
			count++
		}
	}
	return count, nil
}
