// Package storage provides database operations for vcdeploy.
package storage

import "time"

// --- Certificate Authority Models ---

// CertificateAuthority represents a CA in the database.
// Old CAs are NEVER deleted, retained forever for backward compatibility.
type CertificateAuthority struct {
	ID             string     `json:"id"`
	Version        int        `json:"version"`
	CommonName     string     `json:"common_name"`
	CertificatePEM string     `json:"certificate_pem"`
	PrivateKeyEnc  []byte     `json:"-"` // KMS-encrypted, never expose in JSON
	NotBefore      time.Time  `json:"not_before"`
	NotAfter       time.Time  `json:"not_after"`
	Status         string     `json:"status"` // "active", "inactive"
	IsCurrent      bool       `json:"is_current"`
	CreatedAt      time.Time  `json:"created_at"`
	RotatedAt      *time.Time `json:"rotated_at,omitempty"`
}

// CAStatus constants for Certificate Authority lifecycle.
const (
	CAStatusActive   = "active"   // Current CA for signing
	CAStatusInactive = "inactive" // Can verify but not sign
)

// --- Agent Certificate Models ---

// AgentCertificate represents a certificate issued to an agent.
type AgentCertificate struct {
	ID               int64      `json:"id"`
	AgentID          string     `json:"agent_id"`
	CAID             string     `json:"ca_id"`
	SerialNumber     string     `json:"serial_number"`
	CertificatePEM   string     `json:"certificate_pem"`
	NotBefore        time.Time  `json:"not_before"`
	NotAfter         time.Time  `json:"not_after"`
	Status           string     `json:"status"` // "active", "expired", "revoked"
	IssuedAt         time.Time  `json:"issued_at"`
	RenewedAt        *time.Time `json:"renewed_at,omitempty"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	RevocationReason string     `json:"revocation_reason,omitempty"`
}

// CertStatus constants for certificate lifecycle.
const (
	CertStatusActive  = "active"
	CertStatusExpired = "expired"
	CertStatusRevoked = "revoked"
)

// --- Server Certificate Models ---

// ServerCertificate represents a TLS certificate for the server.
type ServerCertificate struct {
	ID             int64     `json:"id"`
	Hostname       string    `json:"hostname"`
	CertificatePEM string    `json:"certificate_pem"`
	PrivateKeyEnc  []byte    `json:"-"`    // KMS-encrypted, never expose in JSON
	SANs           string    `json:"sans"` // JSON array of Subject Alternative Names
	NotBefore      time.Time `json:"not_before"`
	NotAfter       time.Time `json:"not_after"`
	CAID           string    `json:"ca_id"` // Issued by CA
	CreatedAt      time.Time `json:"created_at"`
}

// --- Registration Token Models ---

// RegistrationToken represents a one-time token for agent registration.
type RegistrationToken struct {
	ID        int64      `json:"id"`
	Token     string     `json:"token"`
	AgentID   string     `json:"agent_id,omitempty"` // Pre-assigned or empty for any agent
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
}

// --- Source Credential Models ---

// SourceCredential represents credentials for accessing source repositories.
// Credentials are stored encrypted and never transmitted to agents.
type SourceCredential struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`        // "ssh_key", "https_token", "https_basic"
	URLPattern    string    `json:"url_pattern"` // Regex to match repo URLs
	CredentialEnc []byte    `json:"-"`           // KMS-encrypted, never expose in JSON
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SourceCredentialType constants.
const (
	SourceCredentialTypeSSHKey     = "ssh_key"
	SourceCredentialTypeHTTPSToken = "https_token"
	SourceCredentialTypeHTTPSBasic = "https_basic"
)

// --- Revoked Certificate Models ---

// RevokedCertificate represents an entry in the Certificate Revocation List.
type RevokedCertificate struct {
	ID           int64     `json:"id"`
	SerialNumber string    `json:"serial_number"`
	AgentID      string    `json:"agent_id,omitempty"`
	Reason       string    `json:"reason"`
	RevokedAt    time.Time `json:"revoked_at"`
	RevokedBy    string    `json:"revoked_by"`
}

// --- Encryption Key Models ---

// EncryptionKey represents a versioned encryption key.
// Keys are never deleted, only deactivated for decryption backward compatibility.
type EncryptionKey struct {
	ID                  string     `json:"id"`
	Version             int        `json:"version"`
	KeyMaterialEnc      []byte     `json:"-"` // Encrypted key material, never expose
	Algorithm           string     `json:"algorithm"`
	Status              string     `json:"status"` // "pending", "active", "inactive", "scheduled", "deleted"
	CreatedAt           time.Time  `json:"created_at"`
	ActivatedAt         *time.Time `json:"activated_at,omitempty"`
	DeactivatedAt       *time.Time `json:"deactivated_at,omitempty"`
	ScheduledDeletionAt *time.Time `json:"scheduled_deletion_at,omitempty"`
	DeletionCancelledAt *time.Time `json:"deletion_cancelled_at,omitempty"`
}

// KeyStatus constants for encryption key lifecycle.
const (
	KeyStatusPending   = "pending"   // Created but not yet active
	KeyStatusActive    = "active"    // Current key for encryption
	KeyStatusInactive  = "inactive"  // Can decrypt but not encrypt
	KeyStatusScheduled = "scheduled" // Scheduled for deletion (grace period)
	KeyStatusDeleted   = "deleted"   // Logically deleted (still retained)
)

// --- SSH Key Models ---

// SSHKey represents an SSH key pair for provisioning and git operations.
type SSHKey struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	PublicKey     string    `json:"public_key"`
	PrivateKeyEnc []byte    `json:"-"` // KMS-encrypted, never expose in JSON
	Fingerprint   string    `json:"fingerprint"`
	KeyType       string    `json:"key_type"` // "ed25519", "rsa", "ecdsa"
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}

// SSHKeyType constants.
const (
	SSHKeyTypeEd25519 = "ed25519"
	SSHKeyTypeRSA     = "rsa"
	SSHKeyTypeECDSA   = "ecdsa"
)

// --- Certificate Audit Event Models ---

// CertAuditEvent represents an audit log entry for certificate operations.
type CertAuditEvent struct {
	ID          int64     `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	EventType   string    `json:"event_type"` // "issued", "revoked", "renewed", "rejected"
	AgentID     string    `json:"agent_id,omitempty"`
	Hostname    string    `json:"hostname,omitempty"` // for server certs
	Serial      string    `json:"serial"`
	CAID        string    `json:"ca_id"`            // Issuing CA
	Reason      string    `json:"reason,omitempty"` // for revocation/rejection
	RequestedBy string    `json:"requested_by"`     // user or system
	ClientIP    string    `json:"client_ip,omitempty"`
}

// CertAuditEventType constants.
const (
	CertAuditEventIssued       = "issued"
	CertAuditEventRevoked      = "revoked"
	CertAuditEventRenewed      = "renewed"
	CertAuditEventRejected     = "rejected"
	CertAuditEventExpired      = "expired"
	CertAuditEventACMEObtained = "acme_obtained"
	CertAuditEventACMERenewed  = "acme_renewed"
	CertAuditEventACMEFailed   = "acme_failed"
)

// CertAuditFilter defines filters for querying certificate audit events.
type CertAuditFilter struct {
	AgentID   string
	EventType string
	Since     *time.Time
	Until     *time.Time
	Limit     int
	Offset    int
}

// ACMECertificate represents a stored ACME/Let's Encrypt certificate.
// Matches schema in migration 11: acme_certificates table.
type ACMECertificate struct {
	ID                  int64      `json:"id"`
	Domain              string     `json:"domain"`                // Primary domain (unique)
	CertificatePEM      string     `json:"certificate_pem"`       // Full certificate chain PEM
	PrivateKeyEncrypted []byte     `json:"private_key_encrypted"` // Private key (encrypted at rest)
	Issuer              string     `json:"issuer"`                // e.g., "Let's Encrypt"
	NotBefore           time.Time  `json:"not_before"`
	NotAfter            time.Time  `json:"not_after"`
	LastRenewal         *time.Time `json:"last_renewal,omitempty"` // Last renewal time
	AutoRenew           bool       `json:"auto_renew"`             // Whether to auto-renew
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// ACMEAccount represents an ACME account registration.
// Matches schema in migration 11: acme_accounts table.
type ACMEAccount struct {
	ID                  int64     `json:"id"`
	Email               string    `json:"email"`                 // Contact email
	AccountURL          string    `json:"account_url,omitempty"` // ACME account URL
	PrivateKeyEncrypted []byte    `json:"private_key_encrypted"` // Account private key (encrypted)
	DirectoryURL        string    `json:"directory_url"`         // ACME directory (Let's Encrypt prod/staging)
	CreatedAt           time.Time `json:"created_at"`
}

// --- Recovery Code Models ---

// RecoveryCode represents a one-time-use 2FA recovery code.
// Users are given 8 codes when enabling TOTP. Each code can only be used once.
type RecoveryCode struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	CodeHash  string     `json:"-"` // bcrypt hash, never exposed in JSON
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// RecoveryCodeCount is the number of recovery codes generated.
const RecoveryCodeCount = 8
