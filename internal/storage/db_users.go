// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"go.uber.org/zap"
	"time"
)

// --- User operations ---

// CreateUser creates a new user.
func (db *DB) CreateUser(ctx context.Context, user *User) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, role, must_change_password, totp_secret, totp_enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, user.Username, user.PasswordHash, user.Email, user.Role, user.MustChangePassword, user.TOTPSecret, user.TOTPEnabled)
	if err != nil {
		return fmt.Errorf("inserting user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting user id: %w", err)
	}
	user.ID = id

	return nil
}

// GetUserByUsername retrieves a user by username.
func (db *DB) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	var totpSecret sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, username, password_hash, email, role, totp_secret, totp_enabled, 
		       must_change_password, created_at, updated_at
		FROM users WHERE username = ?
	`, username).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.Role,
		&totpSecret, &user.TOTPEnabled, &user.MustChangePassword,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}

	user.TOTPSecret = totpSecret.String
	return &user, nil
}

// --- Session Management ---

// CreateSession creates a new session.
func (db *DB) CreateSession(ctx context.Context, session *Session) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, ip_address, user_agent, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, session.ID, session.UserID, session.IPAddress, session.UserAgent,
		session.CreatedAt, session.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetSessionByToken retrieves a session by token (ID).
func (db *DB) GetSessionByToken(ctx context.Context, token string) (*Session, error) {
	var session Session
	var ipAddress, userAgent sql.NullString
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, user_id, ip_address, user_agent, created_at, expires_at
		FROM sessions WHERE id = ? AND expires_at > ?
	`, token, time.Now()).Scan(
		&session.ID, &session.UserID, &ipAddress, &userAgent,
		&session.CreatedAt, &session.ExpiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	session.Token = session.ID
	session.IPAddress = ipAddress.String
	session.UserAgent = userAgent.String
	return &session, nil
}

// DeleteSession deletes a session by token (ID).
func (db *DB) DeleteSession(ctx context.Context, token string) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, token)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes all expired sessions.
func (db *DB) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, time.Now())
	if err != nil {
		return 0, fmt.Errorf("deleting expired sessions: %w", err)
	}
	return result.RowsAffected()
}

// DeleteUserSessions deletes all sessions for a user.
func (db *DB) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("deleting user sessions: %w", err)
	}
	return nil
}

// ListUserSessions returns all active sessions for a user.
func (db *DB) ListUserSessions(ctx context.Context, userID int64) ([]*Session, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, user_id, ip_address, user_agent, created_at, expires_at
		FROM sessions WHERE user_id = ? AND expires_at > ?
		ORDER BY created_at DESC
	`, userID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("querying user sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var s Session
		var ipAddress, userAgent sql.NullString
		err := rows.Scan(&s.ID, &s.UserID, &ipAddress, &userAgent,
			&s.CreatedAt, &s.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		s.Token = s.ID
		s.IPAddress = ipAddress.String
		s.UserAgent = userAgent.String
		sessions = append(sessions, &s)
	}
	return sessions, rows.Err()
}

// --- Additional User operations ---

// ListUsers returns all users.
func (db *DB) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, username, password_hash, email, role, totp_secret, totp_enabled, 
		       must_change_password, created_at, updated_at
		FROM users ORDER BY username
	`)
	if err != nil {
		return nil, fmt.Errorf("querying users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var user User
		var totpSecret sql.NullString
		if err := rows.Scan(
			&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.Role,
			&totpSecret, &user.TOTPEnabled, &user.MustChangePassword,
			&user.CreatedAt, &user.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		user.TOTPSecret = totpSecret.String
		users = append(users, &user)
	}
	return users, rows.Err()
}

// ListUsersPaginated returns users with pagination support (H6).
func (db *DB) ListUsersPaginated(ctx context.Context, limit, offset int) ([]*User, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, username, password_hash, email, role, totp_secret, totp_enabled, 
		       must_change_password, created_at, updated_at
		FROM users ORDER BY username LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var user User
		var totpSecret sql.NullString
		if err := rows.Scan(
			&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.Role,
			&totpSecret, &user.TOTPEnabled, &user.MustChangePassword,
			&user.CreatedAt, &user.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		user.TOTPSecret = totpSecret.String
		users = append(users, &user)
	}
	return users, rows.Err()
}

// CountUsers returns the total number of users.
func (db *DB) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	err := db.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return count, nil
}

// GetUserByID retrieves a user by ID.
func (db *DB) GetUserByID(ctx context.Context, id int64) (*User, error) {
	var user User
	var totpSecret sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, username, password_hash, email, role, totp_secret, totp_enabled,
		       must_change_password, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.Role,
		&totpSecret, &user.TOTPEnabled, &user.MustChangePassword,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}

	user.TOTPSecret = totpSecret.String
	return &user, nil
}

// UpdateUserByID updates a user's information.
func (db *DB) UpdateUserByID(ctx context.Context, user *User) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE users SET email = ?, role = ?, password_hash = ?, 
		       totp_secret = ?, totp_enabled = ?, must_change_password = ?,
		       updated_at = datetime('now')
		WHERE id = ?
	`, user.Email, user.Role, user.PasswordHash, user.TOTPSecret, user.TOTPEnabled, user.MustChangePassword, user.ID)
	if err != nil {
		return fmt.Errorf("updating user: %w", err)
	}
	return nil
}

// DeleteUser deletes a user by ID.
func (db *DB) DeleteUser(ctx context.Context, id int64) error {
	// Also delete associated sessions and API keys
	if _, err := db.conn.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		db.logger.Warn("failed to delete user sessions", zap.Int64("userID", id), zap.Error(err))
	}
	if _, err := db.conn.ExecContext(ctx, `DELETE FROM api_keys WHERE user_id = ?`, id); err != nil {
		db.logger.Warn("failed to delete user API keys", zap.Int64("userID", id), zap.Error(err))
	}
	_, err := db.conn.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}
	return nil
}

// --- Recovery Code Operations ---

// SaveRecoveryCodes saves a set of recovery codes for a user (replaces any existing).
func (db *DB) SaveRecoveryCodes(ctx context.Context, userID int64, codes []*RecoveryCode) error {
	return db.RunInTransaction(ctx, func(tx *sql.Tx) error {
		// Delete existing codes for this user
		if _, err := tx.ExecContext(ctx, `DELETE FROM recovery_codes WHERE user_id = ?`, userID); err != nil {
			return fmt.Errorf("deleting existing codes: %w", err)
		}

		// Insert new codes
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO recovery_codes (user_id, code_hash, created_at)
			VALUES (?, ?, CURRENT_TIMESTAMP)
		`)
		if err != nil {
			return fmt.Errorf("preparing statement: %w", err)
		}
		defer stmt.Close()

		for _, code := range codes {
			result, err := stmt.ExecContext(ctx, userID, code.CodeHash)
			if err != nil {
				return fmt.Errorf("inserting recovery code: %w", err)
			}
			id, err := result.LastInsertId()
			if err == nil {
				code.ID = id
			}
		}
		return nil
	})
}

// GetRecoveryCodes returns all recovery codes for a user.
func (db *DB) GetRecoveryCodes(ctx context.Context, userID int64) ([]*RecoveryCode, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, user_id, code_hash, used_at, created_at
		FROM recovery_codes
		WHERE user_id = ?
		ORDER BY id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying recovery codes: %w", err)
	}
	defer rows.Close()

	var codes []*RecoveryCode
	for rows.Next() {
		var code RecoveryCode
		if err := rows.Scan(&code.ID, &code.UserID, &code.CodeHash, &code.UsedAt, &code.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning recovery code: %w", err)
		}
		codes = append(codes, &code)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating recovery codes: %w", err)
	}
	return codes, nil
}

// UseRecoveryCode marks a recovery code as used.
func (db *DB) UseRecoveryCode(ctx context.Context, codeID int64) error {
	result, err := db.conn.ExecContext(ctx, `
		UPDATE recovery_codes SET used_at = CURRENT_TIMESTAMP
		WHERE id = ? AND used_at IS NULL
	`, codeID)
	if err != nil {
		return fmt.Errorf("marking recovery code as used: %w", err)
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

// DeleteRecoveryCodes removes all recovery codes for a user.
func (db *DB) DeleteRecoveryCodes(ctx context.Context, userID int64) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM recovery_codes WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("deleting recovery codes: %w", err)
	}
	return nil
}

// CountUnusedRecoveryCodes returns the count of unused codes for a user.
func (db *DB) CountUnusedRecoveryCodes(ctx context.Context, userID int64) (int, error) {
	var count int
	err := db.conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM recovery_codes
		WHERE user_id = ? AND used_at IS NULL
	`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting unused recovery codes: %w", err)
	}
	return count, nil
}
