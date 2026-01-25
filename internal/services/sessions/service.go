// Package sessions provides session management functionality.
package sessions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.SessionServicer = (*Service)(nil)

// Service handles session management.
type Service struct {
	db *storage.DB
}

// New creates a new sessions Service.
func New(db *storage.DB) *Service {
	return &Service{db: db}
}

// Create creates a new session for a user.
func (s *Service) Create(ctx context.Context, userID int64, ipAddress, userAgent string, duration time.Duration) (*storage.Session, error) {
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}

	session := &storage.Session{
		ID:        token,
		Token:     token,
		UserID:    userID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(duration),
	}

	if err := s.db.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	return session, nil
}

// GetByToken retrieves a session by its token.
func (s *Service) GetByToken(ctx context.Context, token string) (*storage.Session, error) {
	session, err := s.db.GetSessionByToken(ctx, token)
	if err != nil {
		return nil, err // Returns ErrNotFound if not found
	}
	return session, nil
}

// Delete removes a session by token.
func (s *Service) Delete(ctx context.Context, token string) error {
	return s.db.DeleteSession(ctx, token)
}

// DeleteAllForUser removes all sessions for a user.
func (s *Service) DeleteAllForUser(ctx context.Context, userID int64) error {
	return s.db.DeleteUserSessions(ctx, userID)
}

// DeleteExpired removes all expired sessions.
func (s *Service) DeleteExpired(ctx context.Context) (int64, error) {
	return s.db.DeleteExpiredSessions(ctx)
}

// ListForUser returns all active sessions for a user.
func (s *Service) ListForUser(ctx context.Context, userID int64) ([]*storage.Session, error) {
	return s.db.ListUserSessions(ctx, userID)
}

// generateToken creates a cryptographically secure session token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
