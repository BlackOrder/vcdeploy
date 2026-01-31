package storage

import (
	"context"
	"time"
)

// --- User operations ---

// CreateUser creates a new user in memory and queues persistence.
func (s *MemoryStore) CreateUser(ctx context.Context, user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate username
	if _, exists := s.usersByName[user.Username]; exists {
		return ErrDuplicate
	}

	// Assign ID
	user.ID = nextID(&s.nextUserID)
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	// Store a copy in memory to prevent external mutation
	stored := *user
	s.users[user.ID] = &stored
	s.usersByName[user.Username] = &stored

	// Queue persistence
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpInsert, "users", &stored))

	return nil
}

// GetUserByUsername retrieves a user by username from memory.
func (s *MemoryStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.usersByName[username]
	if !ok {
		return nil, ErrNotFound
	}

	// Return a copy to prevent mutation
	copied := *user
	return &copied, nil
}

// GetUserByID retrieves a user by ID from memory.
func (s *MemoryStore) GetUserByID(ctx context.Context, id int64) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[id]
	if !ok {
		return nil, ErrNotFound
	}

	// Return a copy to prevent mutation
	copied := *user
	return &copied, nil
}

// ListUsers returns all users from memory.
func (s *MemoryStore) ListUsers(ctx context.Context) ([]*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*User, 0, len(s.users))
	for _, user := range s.users {
		copied := *user
		users = append(users, &copied)
	}

	return users, nil
}

// CountUsers returns the total number of users.
func (s *MemoryStore) CountUsers(ctx context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return int64(len(s.users)), nil
}

// UpdateUserByID updates a user in memory and queues persistence.
func (s *MemoryStore) UpdateUserByID(ctx context.Context, user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.users[user.ID]
	if !ok {
		return ErrNotFound
	}

	// If username changed, update the name index
	if existing.Username != user.Username {
		// Check for duplicate new username
		if _, exists := s.usersByName[user.Username]; exists {
			return ErrDuplicate
		}
		delete(s.usersByName, existing.Username)
	}

	user.UpdatedAt = time.Now()

	// Store a copy to prevent external mutation
	stored := *user
	s.users[user.ID] = &stored
	s.usersByName[user.Username] = &stored

	// Queue persistence
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpUpdate, "users", &stored))

	return nil
}

// DeleteUser removes a user from memory and queues persistence.
func (s *MemoryStore) DeleteUser(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[id]
	if !ok {
		// Not an error to delete non-existent user
		return nil
	}

	delete(s.users, id)
	delete(s.usersByName, user.Username)

	// Also delete user's sessions and API keys
	for token, session := range s.sessions {
		if session.UserID == id {
			delete(s.sessions, token)
		}
	}
	delete(s.apiKeysByUser, id)
	for hash, key := range s.apiKeys {
		if key.UserID == id {
			delete(s.apiKeys, hash)
		}
	}

	// Queue persistence
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "users", map[string]int64{"id": id}))

	return nil
}

// --- Session operations ---

// CreateSession creates a new session in memory and queues persistence.
func (s *MemoryStore) CreateSession(ctx context.Context, session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Token is used as ID
	if session.Token == "" {
		return ErrValidation
	}

	session.ID = session.Token
	session.CreatedAt = time.Now()
	session.LastUsed = session.CreatedAt

	s.sessions[session.Token] = session

	// Queue persistence
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpInsert, "sessions", session))

	return nil
}

// GetSessionByToken retrieves a session by token from memory.
func (s *MemoryStore) GetSessionByToken(ctx context.Context, token string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[token]
	if !ok {
		return nil, ErrNotFound
	}

	// Check if expired
	if time.Now().After(session.ExpiresAt) {
		return nil, ErrNotFound
	}

	// Return a copy
	copied := *session
	return &copied, nil
}

// DeleteSession removes a session from memory and queues persistence.
func (s *MemoryStore) DeleteSession(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, token)

	// Queue persistence
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "sessions", map[string]string{"token": token}))

	return nil
}

// DeleteExpiredSessions removes expired sessions from memory and queues persistence.
func (s *MemoryStore) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var deleted int64

	for token, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, token)
			deleted++
		}
	}

	if deleted > 0 {
		// Queue persistence
		s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "sessions", map[string]time.Time{"expired_before": now}))
	}

	return deleted, nil
}

// DeleteUserSessions removes all sessions for a user from memory and queues persistence.
func (s *MemoryStore) DeleteUserSessions(ctx context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for token, session := range s.sessions {
		if session.UserID == userID {
			delete(s.sessions, token)
		}
	}

	// Queue persistence
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "sessions", map[string]int64{"user_id": userID}))

	return nil
}

// ListUserSessions returns all sessions for a user from memory.
func (s *MemoryStore) ListUserSessions(ctx context.Context, userID int64) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sessions []*Session
	for _, session := range s.sessions {
		if session.UserID == userID {
			copied := *session
			sessions = append(sessions, &copied)
		}
	}

	return sessions, nil
}

// --- API Key operations ---

// CreateAPIKey creates a new API key in memory and queues persistence.
func (s *MemoryStore) CreateAPIKey(ctx context.Context, key *APIKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key.KeyHash == "" {
		return ErrValidation
	}

	// Check for duplicate hash
	if _, exists := s.apiKeys[key.KeyHash]; exists {
		return ErrDuplicate
	}

	key.ID = nextID(&s.nextAPIKeyID)
	key.CreatedAt = time.Now()

	s.apiKeys[key.KeyHash] = key
	s.apiKeysByUser[key.UserID] = append(s.apiKeysByUser[key.UserID], key)

	// Queue persistence
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpInsert, "api_keys", key))

	return nil
}

// GetAPIKeyByHash retrieves an API key by hash from memory.
func (s *MemoryStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, ok := s.apiKeys[keyHash]
	if !ok {
		return nil, ErrNotFound
	}

	// Check if expired
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return nil, ErrNotFound
	}

	// Return a copy
	copied := *key
	return &copied, nil
}

// UpdateAPIKeyUsage updates the last used timestamp for an API key.
func (s *MemoryStore) UpdateAPIKeyUsage(ctx context.Context, keyID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range s.apiKeys {
		if key.ID == keyID {
			now := time.Now()
			key.LastUsedAt = &now

			// Queue persistence
			s.queueWrite(s.coreWrites, NewWriteOp(WriteOpUpdate, "api_keys", map[string]any{
				"id":           keyID,
				"last_used_at": now,
			}))

			return nil
		}
	}

	return ErrNotFound
}

// DeleteAPIKey removes an API key from memory and queues persistence.
func (s *MemoryStore) DeleteAPIKey(ctx context.Context, keyID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var keyHash string
	var userID int64

	for hash, key := range s.apiKeys {
		if key.ID == keyID {
			keyHash = hash
			userID = key.UserID
			break
		}
	}

	if keyHash == "" {
		return nil // Not found is not an error for delete
	}

	delete(s.apiKeys, keyHash)

	// Remove from user's keys
	userKeys := s.apiKeysByUser[userID]
	for i, key := range userKeys {
		if key.ID == keyID {
			s.apiKeysByUser[userID] = append(userKeys[:i], userKeys[i+1:]...)
			break
		}
	}

	// Queue persistence
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "api_keys", map[string]int64{"id": keyID}))

	return nil
}

// ListAPIKeys returns all API keys for a user from memory.
func (s *MemoryStore) ListAPIKeys(ctx context.Context, userID int64) ([]*APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := s.apiKeysByUser[userID]
	result := make([]*APIKey, len(keys))
	for i, key := range keys {
		copied := *key
		result[i] = &copied
	}

	return result, nil
}

// ErrDuplicate is returned when attempting to create a duplicate record.
var ErrDuplicate = errDuplicate{}

type errDuplicate struct{}

func (errDuplicate) Error() string { return "duplicate" }

// ErrValidation is returned when validation fails.
var ErrValidation = errValidation{}

type errValidation struct{}

func (errValidation) Error() string { return "validation error" }
