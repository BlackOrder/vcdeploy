// Package users provides user management functionality.
package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.UserServicer = (*Service)(nil)

// Service handles user management.
type Service struct {
	store storage.Store
}

// New creates a new users Service.
func New(store storage.Store) *Service {
	return &Service{store: store}
}

// Create creates a new user with validated password.
func (s *Service) Create(ctx context.Context, username, password, email, role string, opts ...services.CreateUserOption) (*storage.User, error) {
	// Apply options
	var options services.CreateUserOptions
	for _, opt := range opts {
		opt(&options)
	}
	// Validate password complexity
	if err := security.ValidatePassword(password); err != nil {
		return nil, fmt.Errorf("password validation failed: %w", err)
	}

	// Hash password
	hash, err := security.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	// Set default role if not provided
	if role == "" {
		role = "user"
	}

	user := &storage.User{
		Username:     username,
		PasswordHash: hash,
		Email:        email,
		Role:         role,
		TOTPEnabled:  options.TOTPEnabled,
		TOTPSecret:   options.TOTPSecret,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.store.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}

	return user, nil
}

// GetByID retrieves a user by ID.
func (s *Service) GetByID(ctx context.Context, id int64) (*storage.User, error) {
	user, err := s.store.GetUserByID(ctx, id)
	if services.IsNotFound(err) {
		return nil, services.NotFound("users.GetByID", "user", strconv.FormatInt(id, 10))
	}
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}
	return user, nil
}

// GetByUsername retrieves a user by username.
func (s *Service) GetByUsername(ctx context.Context, username string) (*storage.User, error) {
	user, err := s.store.GetUserByUsername(ctx, username)
	if services.IsNotFound(err) {
		return nil, services.NotFound("users.GetByUsername", "user", username)
	}
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}
	return user, nil
}

// List returns all users.
func (s *Service) List(ctx context.Context) ([]*storage.User, error) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	return users, nil
}

// ListPaginated returns users with pagination support (H6).
func (s *Service) ListPaginated(ctx context.Context, p services.Pagination) (*services.ListResult[*storage.User], error) {
	users, err := s.store.ListUsersPaginated(ctx, p.Limit, p.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("counting users: %w", err)
	}
	return &services.ListResult[*storage.User]{
		Items:      users,
		TotalCount: count,
		Pagination: p,
	}, nil
}

// Update updates a user's information.
func (s *Service) Update(ctx context.Context, user *storage.User) error {
	if err := s.store.UpdateUserByID(ctx, user); err != nil {
		return fmt.Errorf("updating user: %w", err)
	}
	return nil
}

// Delete removes a user by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.store.DeleteUser(ctx, id); err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}
	return nil
}

// VerifyPassword verifies a username/password combination.
// Returns the user if valid, services.ErrNotFound if user not found, or ErrInvalidPassword if password wrong.
func (s *Service) VerifyPassword(ctx context.Context, username, password string) (*storage.User, error) {
	user, err := s.store.GetUserByUsername(ctx, username)
	if services.IsNotFound(err) {
		return nil, services.NotFound("users.VerifyPassword", "user", username)
	}
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}

	if !security.VerifyPassword(password, user.PasswordHash) {
		return nil, ErrInvalidPassword
	}

	return user, nil
}

// UpdatePassword updates a user's password with validation.
func (s *Service) UpdatePassword(ctx context.Context, userID int64, newPassword string) error {
	// Validate password complexity
	if err := security.ValidatePassword(newPassword); err != nil {
		return fmt.Errorf("password validation failed: %w", err)
	}

	// Get user
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("getting user: %w", err)
	}

	// Hash new password
	hash, err := security.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	user.PasswordHash = hash
	user.MustChangePassword = false

	if err := s.store.UpdateUserByID(ctx, user); err != nil {
		return fmt.Errorf("updating user: %w", err)
	}

	return nil
}

// SetTOTP configures TOTP for a user.
func (s *Service) SetTOTP(ctx context.Context, userID int64, secret string, enabled bool) error {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("getting user: %w", err)
	}

	user.TOTPSecret = secret
	user.TOTPEnabled = enabled

	if err := s.store.UpdateUserByID(ctx, user); err != nil {
		return fmt.Errorf("updating user TOTP: %w", err)
	}

	return nil
}

// Count returns the total number of users.
func (s *Service) Count(ctx context.Context) (int64, error) {
	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return count, nil
}

// DeleteWithCleanup deletes a user and all associated data (sessions, API keys) in a transaction.
func (s *Service) DeleteWithCleanup(ctx context.Context, userID int64) error {
	return s.store.RunInTransaction(ctx, func(tx *sql.Tx) error {
		// Delete all user's sessions
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
			return fmt.Errorf("deleting user sessions: %w", err)
		}

		// Delete all user's API keys
		if _, err := tx.ExecContext(ctx, `DELETE FROM api_keys WHERE user_id = ?`, userID); err != nil {
			return fmt.Errorf("deleting user API keys: %w", err)
		}

		// Delete the user
		result, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
		if err != nil {
			return fmt.Errorf("deleting user: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("checking rows affected: %w", err)
		}
		if rowsAffected == 0 {
			return ErrUserNotFound
		}

		return nil
	})
}

// Errors
var (
	ErrInvalidPassword = errors.New("invalid password")
	ErrUserNotFound    = errors.New("user not found")
)
