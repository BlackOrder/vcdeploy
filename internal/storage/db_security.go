// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// --- SSH Host Key operations ---

// CreateSSHHostKey creates a new SSH host key record.
func (db *DB) CreateSSHHostKey(ctx context.Context, key *SSHHostKey) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO ssh_host_keys (hostname, port, key_type, public_key, fingerprint, trusted, added_by, verified_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, key.Hostname, key.Port, key.KeyType, key.PublicKey, key.Fingerprint, key.Trusted, key.AddedBy, key.VerifiedAt)
	if err != nil {
		return fmt.Errorf("creating ssh host key: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting last insert id: %w", err)
	}
	key.ID = id
	return nil
}

// GetSSHHostKey retrieves an SSH host key by hostname, port, and key type.
func (db *DB) GetSSHHostKey(ctx context.Context, hostname string, port int, keyType string) (*SSHHostKey, error) {
	key := &SSHHostKey{}
	var verifiedAt sql.NullTime
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, hostname, port, key_type, public_key, fingerprint, trusted, added_by, verified_at, created_at, updated_at
		FROM ssh_host_keys
		WHERE hostname = ? AND port = ? AND key_type = ?
	`, hostname, port, keyType).Scan(
		&key.ID, &key.Hostname, &key.Port, &key.KeyType, &key.PublicKey, &key.Fingerprint,
		&key.Trusted, &key.AddedBy, &verifiedAt, &key.CreatedAt, &key.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting ssh host key: %w", err)
	}
	if verifiedAt.Valid {
		key.VerifiedAt = &verifiedAt.Time
	}
	return key, nil
}

// GetSSHHostKeysByHost retrieves all SSH host keys for a hostname and port.
func (db *DB) GetSSHHostKeysByHost(ctx context.Context, hostname string, port int) ([]*SSHHostKey, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, hostname, port, key_type, public_key, fingerprint, trusted, added_by, verified_at, created_at, updated_at
		FROM ssh_host_keys
		WHERE hostname = ? AND port = ?
		ORDER BY key_type
	`, hostname, port)
	if err != nil {
		return nil, fmt.Errorf("listing ssh host keys: %w", err)
	}
	defer rows.Close()

	var keys []*SSHHostKey
	for rows.Next() {
		key := &SSHHostKey{}
		var verifiedAt sql.NullTime
		if err := rows.Scan(
			&key.ID, &key.Hostname, &key.Port, &key.KeyType, &key.PublicKey, &key.Fingerprint,
			&key.Trusted, &key.AddedBy, &verifiedAt, &key.CreatedAt, &key.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning ssh host key: %w", err)
		}
		if verifiedAt.Valid {
			key.VerifiedAt = &verifiedAt.Time
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// ListSSHHostKeys retrieves all SSH host keys.
func (db *DB) ListSSHHostKeys(ctx context.Context) ([]*SSHHostKey, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, hostname, port, key_type, public_key, fingerprint, trusted, added_by, verified_at, created_at, updated_at
		FROM ssh_host_keys
		ORDER BY hostname, port, key_type
	`)
	if err != nil {
		return nil, fmt.Errorf("listing ssh host keys: %w", err)
	}
	defer rows.Close()

	var keys []*SSHHostKey
	for rows.Next() {
		key := &SSHHostKey{}
		var verifiedAt sql.NullTime
		if err := rows.Scan(
			&key.ID, &key.Hostname, &key.Port, &key.KeyType, &key.PublicKey, &key.Fingerprint,
			&key.Trusted, &key.AddedBy, &verifiedAt, &key.CreatedAt, &key.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning ssh host key: %w", err)
		}
		if verifiedAt.Valid {
			key.VerifiedAt = &verifiedAt.Time
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// UpdateSSHHostKeyTrust updates the trust status of an SSH host key.
func (db *DB) UpdateSSHHostKeyTrust(ctx context.Context, id int64, trusted bool, verifiedBy string) error {
	now := time.Now()
	var verifiedAt *time.Time
	if trusted {
		verifiedAt = &now
	}
	_, err := db.conn.ExecContext(ctx, `
		UPDATE ssh_host_keys
		SET trusted = ?, added_by = ?, verified_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, trusted, verifiedBy, verifiedAt, id)
	if err != nil {
		return fmt.Errorf("updating ssh host key trust: %w", err)
	}
	return nil
}

// DeleteSSHHostKey deletes an SSH host key by ID.
func (db *DB) DeleteSSHHostKey(ctx context.Context, id int64) error {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM ssh_host_keys WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting ssh host key: %w", err)
	}
	// Note: SQLite's RowsAffected() never returns an error
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSSHHostKeysByHost deletes all SSH host keys for a hostname and port.
func (db *DB) DeleteSSHHostKeysByHost(ctx context.Context, hostname string, port int) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM ssh_host_keys WHERE hostname = ? AND port = ?
	`, hostname, port)
	if err != nil {
		return 0, fmt.Errorf("deleting ssh host keys: %w", err)
	}
	return result.RowsAffected()
}

// --- SSH Jump Server operations ---

// CreateJumpServer creates a new SSH jump server.
func (db *DB) CreateJumpServer(ctx context.Context, js *SSHJumpServer) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO ssh_jump_servers (name, host, port, username, ssh_key_id)
		VALUES (?, ?, ?, ?, ?)
	`, js.Name, js.Host, js.Port, js.Username, js.SSHKeyID)
	if err != nil {
		return fmt.Errorf("creating jump server: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting last insert id: %w", err)
	}
	js.ID = id
	return nil
}

// GetJumpServer retrieves a jump server by ID.
func (db *DB) GetJumpServer(ctx context.Context, id int64) (*SSHJumpServer, error) {
	js := &SSHJumpServer{}
	var sshKeyID sql.NullInt64
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, name, host, port, username, ssh_key_id, created_at
		FROM ssh_jump_servers
		WHERE id = ?
	`, id).Scan(&js.ID, &js.Name, &js.Host, &js.Port, &js.Username, &sshKeyID, &js.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting jump server: %w", err)
	}
	if sshKeyID.Valid {
		js.SSHKeyID = &sshKeyID.Int64
	}
	return js, nil
}

// GetJumpServerByName retrieves a jump server by name.
func (db *DB) GetJumpServerByName(ctx context.Context, name string) (*SSHJumpServer, error) {
	js := &SSHJumpServer{}
	var sshKeyID sql.NullInt64
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, name, host, port, username, ssh_key_id, created_at
		FROM ssh_jump_servers
		WHERE name = ?
	`, name).Scan(&js.ID, &js.Name, &js.Host, &js.Port, &js.Username, &sshKeyID, &js.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting jump server by name: %w", err)
	}
	if sshKeyID.Valid {
		js.SSHKeyID = &sshKeyID.Int64
	}
	return js, nil
}

// ListJumpServers retrieves all jump servers.
func (db *DB) ListJumpServers(ctx context.Context) ([]*SSHJumpServer, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, name, host, port, username, ssh_key_id, created_at
		FROM ssh_jump_servers
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("listing jump servers: %w", err)
	}
	defer rows.Close()

	var servers []*SSHJumpServer
	for rows.Next() {
		js := &SSHJumpServer{}
		var sshKeyID sql.NullInt64
		if err := rows.Scan(&js.ID, &js.Name, &js.Host, &js.Port, &js.Username, &sshKeyID, &js.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning jump server: %w", err)
		}
		if sshKeyID.Valid {
			js.SSHKeyID = &sshKeyID.Int64
		}
		servers = append(servers, js)
	}
	return servers, rows.Err()
}

// UpdateJumpServer updates an existing jump server.
func (db *DB) UpdateJumpServer(ctx context.Context, js *SSHJumpServer) error {
	result, err := db.conn.ExecContext(ctx, `
		UPDATE ssh_jump_servers
		SET name = ?, host = ?, port = ?, username = ?, ssh_key_id = ?
		WHERE id = ?
	`, js.Name, js.Host, js.Port, js.Username, js.SSHKeyID, js.ID)
	if err != nil {
		return fmt.Errorf("updating jump server: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteJumpServer deletes a jump server by ID.
func (db *DB) DeleteJumpServer(ctx context.Context, id int64) error {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM ssh_jump_servers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting jump server: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Blocked IP Methods ---

// BlockIP adds or updates a blocked IP record.
func (db *DB) BlockIP(ctx context.Context, block *BlockedIP) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO blocked_ips (ip_address, reason, blocked_at, expires_at, blocked_by)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(ip_address) DO UPDATE SET
			reason = excluded.reason,
			expires_at = excluded.expires_at,
			blocked_by = excluded.blocked_by
	`, block.IPAddress, block.Reason, block.BlockedAt, block.ExpiresAt, block.BlockedBy)
	if err != nil {
		return fmt.Errorf("blocking IP: %w", err)
	}
	return nil
}

// UnblockIP removes a blocked IP record.
func (db *DB) UnblockIP(ctx context.Context, ip string) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM blocked_ips WHERE ip_address = ?`, ip)
	if err != nil {
		return fmt.Errorf("unblocking IP: %w", err)
	}
	return nil
}

// IsIPBlocked checks if an IP is currently blocked.
func (db *DB) IsIPBlocked(ctx context.Context, ip string) (bool, error) {
	var count int
	err := db.conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM blocked_ips 
		WHERE ip_address = ? AND (expires_at IS NULL OR expires_at > ?)
	`, ip, time.Now()).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking blocked IP: %w", err)
	}
	return count > 0, nil
}

// GetBlockedIP retrieves a blocked IP record.
func (db *DB) GetBlockedIP(ctx context.Context, ip string) (*BlockedIP, error) {
	var block BlockedIP
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, ip_address, reason, blocked_at, expires_at, COALESCE(blocked_by, '')
		FROM blocked_ips WHERE ip_address = ?
	`, ip).Scan(&block.ID, &block.IPAddress, &block.Reason, &block.BlockedAt, &block.ExpiresAt, &block.BlockedBy)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting blocked IP: %w", err)
	}
	return &block, nil
}

// ListBlockedIPs returns all currently blocked IPs with pagination.
func (db *DB) ListBlockedIPs(ctx context.Context, limit, offset int) ([]*BlockedIP, int64, error) {
	// Get total count
	var total int64
	err := db.conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM blocked_ips WHERE expires_at IS NULL OR expires_at > ?
	`, time.Now()).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting blocked IPs: %w", err)
	}

	// Get paginated results
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, ip_address, reason, blocked_at, expires_at, COALESCE(blocked_by, '')
		FROM blocked_ips 
		WHERE expires_at IS NULL OR expires_at > ?
		ORDER BY blocked_at DESC
		LIMIT ? OFFSET ?
	`, time.Now(), limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing blocked IPs: %w", err)
	}
	defer rows.Close()

	var blocks []*BlockedIP
	for rows.Next() {
		var block BlockedIP
		if err := rows.Scan(&block.ID, &block.IPAddress, &block.Reason, &block.BlockedAt, &block.ExpiresAt, &block.BlockedBy); err != nil {
			return nil, 0, fmt.Errorf("scanning blocked IP: %w", err)
		}
		blocks = append(blocks, &block)
	}
	return blocks, total, rows.Err()
}

// CleanupExpiredBlockedIPs removes expired IP blocks.
func (db *DB) CleanupExpiredBlockedIPs(ctx context.Context) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM blocked_ips WHERE expires_at IS NOT NULL AND expires_at <= ?
	`, time.Now())
	if err != nil {
		return 0, fmt.Errorf("cleaning up expired blocked IPs: %w", err)
	}
	return result.RowsAffected()
}

// RecordRateLimitRequest records a rate limit request for tracking.
func (db *DB) RecordRateLimitRequest(ctx context.Context, key, bucket string, windowStart, windowEnd time.Time) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO rate_limits (key, bucket, count, window_start, window_end)
		VALUES (?, ?, 1, ?, ?)
		ON CONFLICT(key, bucket, window_start) DO UPDATE SET count = count + 1
	`, key, bucket, windowStart, windowEnd)
	if err != nil {
		return fmt.Errorf("recording rate limit request: %w", err)
	}
	return nil
}

// GetRateLimitCount returns the request count for a key in a time window.
func (db *DB) GetRateLimitCount(ctx context.Context, key, bucket string, since time.Time) (int64, error) {
	var total int64
	err := db.conn.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(count), 0) FROM rate_limits 
		WHERE key = ? AND bucket = ? AND window_end > ?
	`, key, bucket, since).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("getting rate limit count: %w", err)
	}
	return total, nil
}

// CleanupRateLimitRecords removes old rate limit records.
func (db *DB) CleanupRateLimitRecords(ctx context.Context, before time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM rate_limits WHERE window_end < ?
	`, before)
	if err != nil {
		return 0, fmt.Errorf("cleaning up rate limit records: %w", err)
	}
	return result.RowsAffected()
}

// --- ACME Certificate operations ---

// GetACMECertificate retrieves an ACME certificate by domain.
func (db *DB) GetACMECertificate(ctx context.Context, domain string) (*ACMECertificate, error) {
	var cert ACMECertificate
	var lastRenewal sql.NullTime
	var issuer sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, domain, certificate_pem, private_key_encrypted, issuer, 
			not_before, not_after, last_renewal, auto_renew, created_at, updated_at
		FROM acme_certificates WHERE domain = ?
	`, domain).Scan(&cert.ID, &cert.Domain, &cert.CertificatePEM, &cert.PrivateKeyEncrypted,
		&issuer, &cert.NotBefore, &cert.NotAfter, &lastRenewal, &cert.AutoRenew, &cert.CreatedAt, &cert.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting ACME certificate: %w", err)
	}

	cert.Issuer = issuer.String
	if lastRenewal.Valid {
		cert.LastRenewal = &lastRenewal.Time
	}
	return &cert, nil
}

// SaveACMECertificate creates or updates an ACME certificate.
func (db *DB) SaveACMECertificate(ctx context.Context, cert *ACMECertificate) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO acme_certificates (domain, certificate_pem, private_key_encrypted, issuer, 
			not_before, not_after, last_renewal, auto_renew, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(domain) DO UPDATE SET 
			certificate_pem = excluded.certificate_pem,
			private_key_encrypted = excluded.private_key_encrypted,
			issuer = excluded.issuer,
			not_before = excluded.not_before,
			not_after = excluded.not_after,
			last_renewal = excluded.last_renewal,
			auto_renew = excluded.auto_renew,
			updated_at = CURRENT_TIMESTAMP
	`, cert.Domain, cert.CertificatePEM, cert.PrivateKeyEncrypted, cert.Issuer,
		cert.NotBefore, cert.NotAfter, cert.LastRenewal, cert.AutoRenew)
	if err != nil {
		return fmt.Errorf("saving ACME certificate: %w", err)
	}

	if cert.ID == 0 {
		id, err := result.LastInsertId()
		if err == nil && id > 0 {
			cert.ID = id
		}
	}
	return nil
}

// DeleteACMECertificate removes an ACME certificate by domain.
func (db *DB) DeleteACMECertificate(ctx context.Context, domain string) error {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM acme_certificates WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("deleting ACME certificate: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ListACMECertificates returns all stored ACME certificates.
func (db *DB) ListACMECertificates(ctx context.Context) ([]*ACMECertificate, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, domain, certificate_pem, private_key_encrypted, issuer, 
			not_before, not_after, last_renewal, auto_renew, created_at, updated_at
		FROM acme_certificates ORDER BY domain ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing ACME certificates: %w", err)
	}
	defer rows.Close()

	var certs []*ACMECertificate
	for rows.Next() {
		var cert ACMECertificate
		var lastRenewal sql.NullTime
		var issuer sql.NullString

		if err := rows.Scan(&cert.ID, &cert.Domain, &cert.CertificatePEM, &cert.PrivateKeyEncrypted,
			&issuer, &cert.NotBefore, &cert.NotAfter, &lastRenewal, &cert.AutoRenew,
			&cert.CreatedAt, &cert.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning ACME certificate: %w", err)
		}
		cert.Issuer = issuer.String
		if lastRenewal.Valid {
			cert.LastRenewal = &lastRenewal.Time
		}
		certs = append(certs, &cert)
	}
	return certs, rows.Err()
}

// --- ACME Account operations ---

// GetACMEAccount retrieves an ACME account by email.
func (db *DB) GetACMEAccount(ctx context.Context, email string) (*ACMEAccount, error) {
	var account ACMEAccount
	var accountURL sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, email, account_url, private_key_encrypted, directory_url, created_at
		FROM acme_accounts WHERE email = ?
	`, email).Scan(&account.ID, &account.Email, &accountURL, &account.PrivateKeyEncrypted,
		&account.DirectoryURL, &account.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting ACME account: %w", err)
	}

	account.AccountURL = accountURL.String
	return &account, nil
}

// SaveACMEAccount creates or updates an ACME account.
func (db *DB) SaveACMEAccount(ctx context.Context, account *ACMEAccount) error {
	// Check if account exists by email (email isn't unique constraint, so manually handle)
	var existingID int64
	err := db.conn.QueryRowContext(ctx, `SELECT id FROM acme_accounts WHERE email = ?`, account.Email).Scan(&existingID)

	if errors.Is(err, sql.ErrNoRows) {
		// Insert new
		result, err := db.conn.ExecContext(ctx, `
			INSERT INTO acme_accounts (email, account_url, private_key_encrypted, directory_url, created_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		`, account.Email, account.AccountURL, account.PrivateKeyEncrypted, account.DirectoryURL)
		if err != nil {
			return fmt.Errorf("inserting ACME account: %w", err)
		}
		id, err := result.LastInsertId()
		if err == nil {
			account.ID = id
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking existing ACME account: %w", err)
	}

	// Update existing
	_, err = db.conn.ExecContext(ctx, `
		UPDATE acme_accounts SET account_url = ?, private_key_encrypted = ?, directory_url = ?
		WHERE id = ?
	`, account.AccountURL, account.PrivateKeyEncrypted, account.DirectoryURL, existingID)
	if err != nil {
		return fmt.Errorf("updating ACME account: %w", err)
	}
	account.ID = existingID
	return nil
}

// DeleteACMEAccount removes an ACME account by email.
func (db *DB) DeleteACMEAccount(ctx context.Context, email string) error {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM acme_accounts WHERE email = ?`, email)
	if err != nil {
		return fmt.Errorf("deleting ACME account: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
