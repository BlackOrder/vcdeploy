// Package agent provides the agent daemon implementation.
package agent

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	_ "modernc.org/sqlite" // SQLite driver (pure Go, no CGO)
)

// AgentStore provides encrypted SQLite storage for the agent.
// All sensitive data is encrypted using a machine-derived key.
//
//nolint:revive // Type name stutters (agent.AgentStore) but is clearer than agent.Store
type AgentStore struct {
	db         *sql.DB
	machineKey []byte
	gcm        cipher.AEAD
}

// CertificateRecord represents a stored certificate.
type CertificateRecord struct {
	Type        string // "agent", "ca"
	Certificate []byte
	PrivateKey  []byte // nil for CA cert
	NotAfter    time.Time
	UpdatedAt   time.Time
}

// HMACSecret represents a stored HMAC secret for a master.
type HMACSecret struct {
	MasterHost string
	Secret     []byte
	CreatedAt  time.Time
}

// RevokedCertRecord represents a cached revoked certificate.
type RevokedCertRecord struct {
	SerialNumber string
	RevokedAt    time.Time
	SyncedAt     time.Time
}

// AuditEvent represents an audit log entry.
type AuditEvent struct {
	ID        int64
	Timestamp time.Time
	EventType string
	Details   string
	Metadata  map[string]interface{}
	Success   bool
}

// Audit event types.
const (
	AuditEventConnect        = "connect"
	AuditEventDisconnect     = "disconnect"
	AuditEventCertIssued     = "cert_issued"
	AuditEventCertRenewed    = "cert_renewed"
	AuditEventCertExpiring   = "cert_expiring"
	AuditEventReauth         = "reauth"
	AuditEventReauthFailed   = "reauth_failed"
	AuditEventDeployStart    = "deploy_start"
	AuditEventDeployComplete = "deploy_complete"
	AuditEventDeployFailed   = "deploy_failed"
	AuditEventDeployRollback = "deploy_rollback"
	AuditEventUpdateStart    = "update_start"
	AuditEventUpdateComplete = "update_complete"
	AuditEventUpdateFailed   = "update_failed"
	AuditEventConfigReload   = "config_reload"
	AuditEventHealthCheck    = "health_check"
)

// RepoCacheRecord represents a cached repository.
type RepoCacheRecord struct {
	RepoURL     string
	ArchivePath string
	Checksum    string
	CachedAt    time.Time
	LastUsed    time.Time
}

// dbSchemaVersion is incremented when the agent database schema changes incompatibly.
// When a version mismatch is detected, the old database is archived and a fresh one created.
const dbSchemaVersion = 1

// NewAgentStore creates a new agent store at the specified data directory.
func NewAgentStore(dataDir string) (*AgentStore, error) {
	dbPath := filepath.Join(dataDir, "agent.db")

	// Ensure directory exists
	if err := ensureDir(dataDir); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	// Open database with WAL mode and busy timeout
	db, err := sql.Open("sqlite", dbPath+"?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Check database compatibility and archive if needed
	db, err = checkAndArchiveIncompatible(db, dbPath)
	if err != nil {
		return nil, fmt.Errorf("database compatibility check: %w", err)
	}

	// Derive machine key
	machineKey, err := deriveMachineKey()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("derive machine key: %w", err)
	}

	// Create AES-GCM cipher
	block, err := aes.NewCipher(machineKey)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	store := &AgentStore{
		db:         db,
		machineKey: machineKey,
		gcm:        gcm,
	}

	return store, nil
}

// checkAndArchiveIncompatible checks the database schema version and archives
// the database if it's incompatible. Returns a fresh database connection.
func checkAndArchiveIncompatible(db *sql.DB, dbPath string) (*sql.DB, error) {
	var version int
	err := db.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		return nil, fmt.Errorf("read schema version: %w", err)
	}

	// New database or compatible version - set version and return
	if version == 0 || version == dbSchemaVersion {
		_, err = db.Exec(fmt.Sprintf("PRAGMA user_version = %d", dbSchemaVersion))
		if err != nil {
			return nil, fmt.Errorf("set schema version: %w", err)
		}
		return db, nil
	}

	// Incompatible version - archive and recreate
	log.Printf("Database version mismatch (have %d, need %d), archiving old database", version, dbSchemaVersion)
	db.Close()

	archivePath := fmt.Sprintf("%s.archived.%d", dbPath, time.Now().Unix())
	if err := os.Rename(dbPath, archivePath); err != nil {
		return nil, fmt.Errorf("archive old database: %w", err)
	}

	// Also archive WAL and SHM files if they exist
	for _, suffix := range []string{"-wal", "-shm"} {
		walPath := dbPath + suffix
		if _, err := os.Stat(walPath); err == nil {
			if renameErr := os.Rename(walPath, archivePath+suffix); renameErr != nil {
				log.Printf("Warning: failed to archive %s: %v", walPath, renameErr)
			}
		}
	}

	// Clean old archives (keep last 3)
	if err := cleanOldArchives(filepath.Dir(dbPath), 3); err != nil {
		// Log but don't fail - cleanup is best-effort
		log.Printf("Warning: failed to clean old archives: %v", err)
	}

	// Reopen with fresh database
	newDB, err := sql.Open("sqlite", dbPath+"?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("reopen database: %w", err)
	}

	_, err = newDB.Exec(fmt.Sprintf("PRAGMA user_version = %d", dbSchemaVersion))
	if err != nil {
		newDB.Close()
		return nil, fmt.Errorf("set schema version: %w", err)
	}

	return newDB, nil
}

// cleanOldArchives removes old database archives, keeping only the most recent ones.
func cleanOldArchives(dir string, keep int) error {
	pattern := filepath.Join(dir, "agent.db.archived.*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	// Filter out WAL and SHM files from the count
	var dbFiles []string
	for _, m := range matches {
		if !isWALOrSHM(m) {
			dbFiles = append(dbFiles, m)
		}
	}

	if len(dbFiles) <= keep {
		return nil
	}

	// Sort by filename (timestamp embedded) - oldest first
	sort.Strings(dbFiles)

	// Remove oldest, keep newest `keep` files
	for _, path := range dbFiles[:len(dbFiles)-keep] {
		// Remove main DB file
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		// Remove associated WAL and SHM files
		for _, suffix := range []string{"-wal", "-shm"} {
			os.Remove(path + suffix) // Ignore errors - files may not exist
		}
	}

	return nil
}

// isWALOrSHM checks if a path is a WAL or SHM file.
func isWALOrSHM(path string) bool {
	return filepath.Ext(path) == "-wal" || filepath.Ext(path) == "-shm" ||
		len(path) > 4 && (path[len(path)-4:] == "-wal" || path[len(path)-4:] == "-shm")
}

// InitSchema creates the database tables.
func (s *AgentStore) InitSchema(ctx context.Context) error {
	schema := `
	-- Agent state key-value store
	CREATE TABLE IF NOT EXISTS agent_state (
		key TEXT PRIMARY KEY,
		value BLOB NOT NULL,
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	-- Certificate storage
	CREATE TABLE IF NOT EXISTS certificates (
		type TEXT PRIMARY KEY,
		certificate BLOB NOT NULL,
		private_key BLOB,
		not_after TEXT NOT NULL,
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	-- HMAC secrets for re-authentication
	CREATE TABLE IF NOT EXISTS hmac_secrets (
		master_host TEXT PRIMARY KEY,
		secret BLOB NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	-- Cached revoked certificates (CRL)
	CREATE TABLE IF NOT EXISTS revoked_certs (
		serial_number TEXT PRIMARY KEY,
		revoked_at TEXT NOT NULL,
		synced_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	-- Audit log
	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TEXT NOT NULL DEFAULT (datetime('now')),
		event_type TEXT NOT NULL,
		details TEXT,
		metadata TEXT,
		success INTEGER NOT NULL DEFAULT 1
	);

	-- Repository cache metadata
	CREATE TABLE IF NOT EXISTS repo_cache (
		repo_url TEXT PRIMARY KEY,
		archive_path TEXT NOT NULL,
		checksum TEXT NOT NULL,
		cached_at TEXT NOT NULL DEFAULT (datetime('now')),
		last_used TEXT NOT NULL DEFAULT (datetime('now'))
	);

	-- Indexes
	CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp);
	CREATE INDEX IF NOT EXISTS idx_audit_log_event_type ON audit_log(event_type);
	CREATE INDEX IF NOT EXISTS idx_revoked_certs_synced ON revoked_certs(synced_at);
	`

	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// Close closes the database connection.
func (s *AgentStore) Close() error {
	return s.db.Close()
}

// encrypt encrypts data using AES-GCM.
func (s *AgentStore) encrypt(plaintext []byte) ([]byte, error) {
	if plaintext == nil {
		return nil, nil
	}

	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	return s.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt decrypts data using AES-GCM.
func (s *AgentStore) decrypt(ciphertext []byte) ([]byte, error) {
	if ciphertext == nil {
		return nil, nil
	}

	nonceSize := s.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return s.gcm.Open(nil, nonce, ciphertext, nil)
}

// ===== Agent State =====

// GetState retrieves an encrypted state value by key.
func (s *AgentStore) GetState(ctx context.Context, key string) ([]byte, error) {
	var encrypted []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM agent_state WHERE key = ?`, key).Scan(&encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return s.decrypt(encrypted)
}

// SetState stores an encrypted state value.
func (s *AgentStore) SetState(ctx context.Context, key string, value []byte) error {
	encrypted, err := s.encrypt(value)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO agent_state (key, value, updated_at) VALUES (?, ?, datetime('now'))`,
		key, encrypted)
	return err
}

// DeleteState removes a state value.
func (s *AgentStore) DeleteState(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agent_state WHERE key = ?`, key)
	return err
}

// ===== Certificates =====

// SaveCertificate stores a certificate (and optionally private key) encrypted.
func (s *AgentStore) SaveCertificate(ctx context.Context, certType string, cert, privateKey []byte, notAfter time.Time) error {
	encryptedCert, err := s.encrypt(cert)
	if err != nil {
		return fmt.Errorf("encrypt certificate: %w", err)
	}

	var encryptedKey []byte
	if privateKey != nil {
		encryptedKey, err = s.encrypt(privateKey)
		if err != nil {
			return fmt.Errorf("encrypt private key: %w", err)
		}
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO certificates (type, certificate, private_key, not_after, updated_at) 
		 VALUES (?, ?, ?, ?, datetime('now'))`,
		certType, encryptedCert, encryptedKey, notAfter.Format(time.RFC3339))
	return err
}

// GetCertificate retrieves a stored certificate.
func (s *AgentStore) GetCertificate(ctx context.Context, certType string) (*CertificateRecord, error) {
	var encryptedCert, encryptedKey []byte
	var notAfterStr, updatedAtStr string

	err := s.db.QueryRowContext(ctx,
		`SELECT certificate, private_key, not_after, updated_at FROM certificates WHERE type = ?`,
		certType).Scan(&encryptedCert, &encryptedKey, &notAfterStr, &updatedAtStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	cert, err := s.decrypt(encryptedCert)
	if err != nil {
		return nil, fmt.Errorf("decrypt certificate: %w", err)
	}

	var privateKey []byte
	if encryptedKey != nil {
		privateKey, err = s.decrypt(encryptedKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt private key: %w", err)
		}
	}

	notAfter, _ := time.Parse(time.RFC3339, notAfterStr)
	updatedAt, _ := time.Parse(time.RFC3339, updatedAtStr)

	return &CertificateRecord{
		Type:        certType,
		Certificate: cert,
		PrivateKey:  privateKey,
		NotAfter:    notAfter,
		UpdatedAt:   updatedAt,
	}, nil
}

// GetTLSCertificate loads a certificate as a tls.Certificate.
func (s *AgentStore) GetTLSCertificate(ctx context.Context, certType string) (*tls.Certificate, error) {
	record, err := s.GetCertificate(ctx, certType)
	if err != nil {
		return nil, err
	}

	if record.PrivateKey == nil {
		return nil, errors.New("certificate has no private key")
	}

	cert, err := tls.X509KeyPair(record.Certificate, record.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	return &cert, nil
}

// DeleteCertificate removes a stored certificate.
func (s *AgentStore) DeleteCertificate(ctx context.Context, certType string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM certificates WHERE type = ?`, certType)
	return err
}

// ListCertificates returns all stored certificates.
func (s *AgentStore) ListCertificates(ctx context.Context) ([]*CertificateRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT type, certificate, private_key, not_after, updated_at FROM certificates`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []*CertificateRecord
	for rows.Next() {
		var encryptedCert, encryptedKey []byte
		var certType, notAfterStr, updatedAtStr string

		if err := rows.Scan(&certType, &encryptedCert, &encryptedKey, &notAfterStr, &updatedAtStr); err != nil {
			return nil, err
		}

		cert, err := s.decrypt(encryptedCert)
		if err != nil {
			return nil, err
		}

		var privateKey []byte
		if encryptedKey != nil {
			privateKey, err = s.decrypt(encryptedKey)
			if err != nil {
				return nil, err
			}
		}

		notAfter, _ := time.Parse(time.RFC3339, notAfterStr)
		updatedAt, _ := time.Parse(time.RFC3339, updatedAtStr)

		certs = append(certs, &CertificateRecord{
			Type:        certType,
			Certificate: cert,
			PrivateKey:  privateKey,
			NotAfter:    notAfter,
			UpdatedAt:   updatedAt,
		})
	}

	return certs, rows.Err()
}

// ===== HMAC Secrets =====

// SaveHMACSecret stores an HMAC secret for a master host.
func (s *AgentStore) SaveHMACSecret(ctx context.Context, masterHost string, secret []byte) error {
	encrypted, err := s.encrypt(secret)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO hmac_secrets (master_host, secret, created_at) 
		 VALUES (?, ?, datetime('now'))`,
		masterHost, encrypted)
	return err
}

// GetHMACSecret retrieves an HMAC secret for a master host.
func (s *AgentStore) GetHMACSecret(ctx context.Context, masterHost string) (*HMACSecret, error) {
	var encrypted []byte
	var createdAtStr string

	err := s.db.QueryRowContext(ctx,
		`SELECT secret, created_at FROM hmac_secrets WHERE master_host = ?`,
		masterHost).Scan(&encrypted, &createdAtStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	secret, err := s.decrypt(encrypted)
	if err != nil {
		return nil, err
	}

	createdAt, _ := time.Parse(time.RFC3339, createdAtStr)

	return &HMACSecret{
		MasterHost: masterHost,
		Secret:     secret,
		CreatedAt:  createdAt,
	}, nil
}

// DeleteHMACSecret removes an HMAC secret.
func (s *AgentStore) DeleteHMACSecret(ctx context.Context, masterHost string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM hmac_secrets WHERE master_host = ?`, masterHost)
	return err
}

// ===== Revoked Certificates (CRL Cache) =====

// SaveRevokedCert caches a revoked certificate entry.
func (s *AgentStore) SaveRevokedCert(ctx context.Context, serialNumber string, revokedAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO revoked_certs (serial_number, revoked_at, synced_at) 
		 VALUES (?, ?, datetime('now'))`,
		serialNumber, revokedAt.Format(time.RFC3339))
	return err
}

// IsRevoked checks if a certificate serial number is in the revocation cache.
func (s *AgentStore) IsRevoked(ctx context.Context, serialNumber string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM revoked_certs WHERE serial_number = ?`,
		serialNumber).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListRevokedCerts returns all cached revoked certificates.
func (s *AgentStore) ListRevokedCerts(ctx context.Context) ([]*RevokedCertRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT serial_number, revoked_at, synced_at FROM revoked_certs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*RevokedCertRecord
	for rows.Next() {
		var serial, revokedAtStr, syncedAtStr string
		if err := rows.Scan(&serial, &revokedAtStr, &syncedAtStr); err != nil {
			return nil, err
		}

		revokedAt, _ := time.Parse(time.RFC3339, revokedAtStr)
		syncedAt, _ := time.Parse(time.RFC3339, syncedAtStr)

		records = append(records, &RevokedCertRecord{
			SerialNumber: serial,
			RevokedAt:    revokedAt,
			SyncedAt:     syncedAt,
		})
	}

	return records, rows.Err()
}

// SyncRevokedCerts replaces the local CRL cache with new entries.
func (s *AgentStore) SyncRevokedCerts(ctx context.Context, certs []*RevokedCertRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback() // Rollback is a no-op if already committed
	}()

	// Clear existing cache
	if _, err := tx.ExecContext(ctx, `DELETE FROM revoked_certs`); err != nil {
		return err
	}

	// Insert new entries
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO revoked_certs (serial_number, revoked_at, synced_at) VALUES (?, ?, datetime('now'))`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, cert := range certs {
		if _, err := stmt.ExecContext(ctx, cert.SerialNumber, cert.RevokedAt.Format(time.RFC3339)); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ===== Audit Logging =====

// LogAuditEvent records an audit event.
func (s *AgentStore) LogAuditEvent(ctx context.Context, eventType, details string, success bool) error {
	return s.LogAuditEventWithMetadata(ctx, eventType, details, success, nil)
}

// LogAuditEventWithMetadata records an audit event with structured metadata.
func (s *AgentStore) LogAuditEventWithMetadata(ctx context.Context, eventType, details string, success bool, metadata map[string]interface{}) error {
	successInt := 0
	if success {
		successInt = 1
	}

	var metadataJSON []byte
	var err error
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO audit_log (timestamp, event_type, details, metadata, success) 
		 VALUES (datetime('now'), ?, ?, ?, ?)`,
		eventType, details, metadataJSON, successInt)
	return err
}

// GetAuditEvents retrieves audit events with optional filtering.
func (s *AgentStore) GetAuditEvents(ctx context.Context, eventType string, limit int) ([]*AuditEvent, error) {
	var query string
	var args []any

	if eventType != "" {
		query = `SELECT id, timestamp, event_type, details, metadata, success FROM audit_log 
		         WHERE event_type = ? ORDER BY timestamp DESC LIMIT ?`
		args = []any{eventType, limit}
	} else {
		query = `SELECT id, timestamp, event_type, details, metadata, success FROM audit_log 
		         ORDER BY timestamp DESC LIMIT ?`
		args = []any{limit}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*AuditEvent
	for rows.Next() {
		var id int64
		var timestampStr, evType, details string
		var metadataJSON sql.NullString
		var success int

		if err := rows.Scan(&id, &timestampStr, &evType, &details, &metadataJSON, &success); err != nil {
			return nil, err
		}

		timestamp, _ := time.Parse("2006-01-02 15:04:05", timestampStr)

		event := &AuditEvent{
			ID:        id,
			Timestamp: timestamp,
			EventType: evType,
			Details:   details,
			Success:   success == 1,
		}

		// Parse metadata if present
		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &event.Metadata); err != nil {
				// Log but don't fail - metadata may be malformed
				event.Metadata = map[string]interface{}{"_parse_error": err.Error()}
			}
		}

		events = append(events, event)
	}

	return events, rows.Err()
}

// AuditLogFilter specifies criteria for querying audit logs.
type AuditLogFilter struct {
	EventTypes []string
	StartTime  *time.Time
	EndTime    *time.Time
	Success    *bool
	Limit      int
	Offset     int
}

// QueryAuditLogs retrieves audit logs matching the filter with advanced filtering.
func (s *AgentStore) QueryAuditLogs(ctx context.Context, filter AuditLogFilter) ([]*AuditEvent, error) {
	query := `SELECT id, timestamp, event_type, details, metadata, success FROM audit_log WHERE 1=1`
	args := []interface{}{}

	if len(filter.EventTypes) > 0 {
		placeholders := make([]string, len(filter.EventTypes))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		query += " AND event_type IN (" + strings.Join(placeholders, ",") + ")"
		for _, et := range filter.EventTypes {
			args = append(args, et)
		}
	}

	if filter.StartTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, filter.StartTime.Format("2006-01-02 15:04:05"))
	}

	if filter.EndTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, filter.EndTime.Format("2006-01-02 15:04:05"))
	}

	if filter.Success != nil {
		query += " AND success = ?"
		if *filter.Success {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
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

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*AuditEvent
	for rows.Next() {
		var id int64
		var timestampStr, evType, details string
		var metadataJSON sql.NullString
		var success int

		if err := rows.Scan(&id, &timestampStr, &evType, &details, &metadataJSON, &success); err != nil {
			return nil, err
		}

		timestamp, _ := time.Parse("2006-01-02 15:04:05", timestampStr)

		event := &AuditEvent{
			ID:        id,
			Timestamp: timestamp,
			EventType: evType,
			Details:   details,
			Success:   success == 1,
		}

		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &event.Metadata); err != nil {
				event.Metadata = map[string]interface{}{"_parse_error": err.Error()}
			}
		}

		events = append(events, event)
	}

	return events, rows.Err()
}

// CleanupOldAuditEvents removes audit events older than the specified duration.
func (s *AgentStore) CleanupOldAuditEvents(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM audit_log WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ===== Repository Cache =====

// SaveRepoCache stores repository cache metadata.
func (s *AgentStore) SaveRepoCache(ctx context.Context, repoURL, archivePath, checksum string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO repo_cache (repo_url, archive_path, checksum, cached_at, last_used) 
		 VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		repoURL, archivePath, checksum)
	return err
}

// GetRepoCache retrieves repository cache metadata.
func (s *AgentStore) GetRepoCache(ctx context.Context, repoURL string) (*RepoCacheRecord, error) {
	var archivePath, checksum, cachedAtStr, lastUsedStr string

	err := s.db.QueryRowContext(ctx,
		`SELECT archive_path, checksum, cached_at, last_used FROM repo_cache WHERE repo_url = ?`,
		repoURL).Scan(&archivePath, &checksum, &cachedAtStr, &lastUsedStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	cachedAt, _ := time.Parse("2006-01-02 15:04:05", cachedAtStr)
	lastUsed, _ := time.Parse("2006-01-02 15:04:05", lastUsedStr)

	return &RepoCacheRecord{
		RepoURL:     repoURL,
		ArchivePath: archivePath,
		Checksum:    checksum,
		CachedAt:    cachedAt,
		LastUsed:    lastUsed,
	}, nil
}

// TouchRepoCache updates the last_used timestamp for a cached repository.
func (s *AgentStore) TouchRepoCache(ctx context.Context, repoURL string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE repo_cache SET last_used = datetime('now') WHERE repo_url = ?`, repoURL)
	return err
}

// DeleteRepoCache removes a repository from the cache.
func (s *AgentStore) DeleteRepoCache(ctx context.Context, repoURL string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM repo_cache WHERE repo_url = ?`, repoURL)
	return err
}

// ListRepoCache returns all cached repositories.
func (s *AgentStore) ListRepoCache(ctx context.Context) ([]*RepoCacheRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT repo_url, archive_path, checksum, cached_at, last_used FROM repo_cache ORDER BY last_used DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*RepoCacheRecord
	for rows.Next() {
		var repoURL, archivePath, checksum, cachedAtStr, lastUsedStr string
		if err := rows.Scan(&repoURL, &archivePath, &checksum, &cachedAtStr, &lastUsedStr); err != nil {
			return nil, err
		}

		cachedAt, _ := time.Parse("2006-01-02 15:04:05", cachedAtStr)
		lastUsed, _ := time.Parse("2006-01-02 15:04:05", lastUsedStr)

		records = append(records, &RepoCacheRecord{
			RepoURL:     repoURL,
			ArchivePath: archivePath,
			Checksum:    checksum,
			CachedAt:    cachedAt,
			LastUsed:    lastUsed,
		})
	}

	return records, rows.Err()
}

// CleanupOldRepoCache removes repo cache entries not used since the specified duration.
func (s *AgentStore) CleanupOldRepoCache(ctx context.Context, olderThan time.Duration) ([]string, error) {
	cutoff := time.Now().Add(-olderThan).Format(time.RFC3339)

	// Get paths first so caller can clean up files
	rows, err := s.db.QueryContext(ctx,
		`SELECT archive_path FROM repo_cache WHERE last_used < ?`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Delete entries
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM repo_cache WHERE last_used < ?`, cutoff)
	if err != nil {
		return nil, err
	}

	return paths, nil
}

// ===== JSON State Helpers =====

// GetStateJSON retrieves and unmarshals a JSON state value.
func (s *AgentStore) GetStateJSON(ctx context.Context, key string, v any) error {
	data, err := s.GetState(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// SetStateJSON marshals and stores a JSON state value.
func (s *AgentStore) SetStateJSON(ctx context.Context, key string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.SetState(ctx, key, data)
}

// ensureDir creates a directory if it doesn't exist.
func ensureDir(dir string) error {
	return makeDir(dir, 0o700)
}
