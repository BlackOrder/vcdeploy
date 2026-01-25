// Package users provides user management functionality.
package users

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.UserServicer = (*Service)(nil)

// Service handles user management.
type Service struct {
	db *storage.DB
}

// New creates a new users Service.
func New(db *storage.DB) *Service {
	return &Service{db: db}
}

// Create creates a new user with validated password.
func (s *Service) Create(ctx context.Context, username, password, email, role string) (*storage.User, error) {
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
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.db.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}

	return user, nil
}

// GetByID retrieves a user by ID.
func (s *Service) GetByID(ctx context.Context, id int64) (*storage.User, error) {
	user, err := s.db.GetUserByID(ctx, id)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}
	return user, nil
}

// GetByUsername retrieves a user by username.
func (s *Service) GetByUsername(ctx context.Context, username string) (*storage.User, error) {
	user, err := s.db.GetUserByUsername(ctx, username)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}
	return user, nil
}

// List returns all users.
func (s *Service) List(ctx context.Context) ([]*storage.User, error) {
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	return users, nil
}

// Update updates a user's information.
func (s *Service) Update(ctx context.Context, user *storage.User) error {
	if err := s.db.UpdateUserByID(ctx, user); err != nil {
		return fmt.Errorf("updating user: %w", err)
	}
	return nil
}

// Delete removes a user by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.db.DeleteUser(ctx, id); err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}
	return nil
}

// VerifyPassword verifies a username/password combination.
// Returns the user if valid, nil if user not found, or error if password wrong.
func (s *Service) VerifyPassword(ctx context.Context, username, password string) (*storage.User, error) {
	user, err := s.db.GetUserByUsername(ctx, username)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil // User not found
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
	user, err := s.db.GetUserByID(ctx, userID)
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

	if err := s.db.UpdateUserByID(ctx, user); err != nil {
		return fmt.Errorf("updating user: %w", err)
	}

	return nil
}

// SetTOTP configures TOTP for a user.
func (s *Service) SetTOTP(ctx context.Context, userID int64, secret string, enabled bool) error {
	user, err := s.db.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("getting user: %w", err)
	}

	user.TOTPSecret = secret
	user.TOTPEnabled = enabled

	if err := s.db.UpdateUserByID(ctx, user); err != nil {
		return fmt.Errorf("updating user TOTP: %w", err)
	}

	return nil
}

// Errors
var (
	ErrInvalidPassword = errors.New("invalid password")
	ErrUserNotFound    = errors.New("user not found")
)
