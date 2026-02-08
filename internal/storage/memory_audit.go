package storage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/xid"
)

// --- Audit methods ---

// LogAudit creates a new audit log entry.
func (s *MemoryStore) LogAudit(ctx context.Context, entry *AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry.ID = nextID(&s.nextAuditID)
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Copy-on-store
	stored := *entry
	s.auditLogs = append(s.auditLogs, &stored)

	s.queueWrite(s.auditWrites, NewWriteOp(WriteOpInsert, "audit_logs", &stored))
	return nil
}

// LogAuditWithSnapshot creates an audit log entry with a resource snapshot.
func (s *MemoryStore) LogAuditWithSnapshot(ctx context.Context, entry *AuditEntry, resourceSnapshot any) error {
	if resourceSnapshot != nil {
		data, err := json.Marshal(resourceSnapshot)
		if err == nil {
			entry.ResourceData = string(data)
		}
	}
	return s.LogAudit(ctx, entry)
}

// ListAuditLogs returns audit logs with pagination.
func (s *MemoryStore) ListAuditLogs(ctx context.Context, limit int, offset int) ([]*AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Make a copy sorted by newest first
	all := make([]*AuditEntry, len(s.auditLogs))
	for i, e := range s.auditLogs {
		cp := *e
		all[len(s.auditLogs)-1-i] = &cp // Reverse order (newest first)
	}

	// Apply pagination
	if offset >= len(all) {
		return []*AuditEntry{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	return all[offset:end], nil
}

// ListAuditLogsSince returns audit logs since a given time.
func (s *MemoryStore) ListAuditLogsSince(ctx context.Context, since time.Time) ([]*AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*AuditEntry
	for _, e := range s.auditLogs {
		if e.Timestamp.After(since) || e.Timestamp.Equal(since) {
			cp := *e
			result = append(result, &cp)
		}
	}

	// Sort by timestamp ascending for since queries
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Timestamp.Before(result[i].Timestamp) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result, nil
}

// --- Setting methods ---

// GetSetting retrieves a setting by category and key.
func (s *MemoryStore) GetSetting(ctx context.Context, category, key string) (*Setting, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	setting, ok := s.settings[settingKey(category, key)]
	if !ok {
		return nil, ErrNotFound
	}

	result := *setting
	return &result, nil
}

// SetSetting creates or updates a setting.
func (s *MemoryStore) SetSetting(ctx context.Context, category, key, value, valueType string, encrypted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := settingKey(category, key)
	now := time.Now()

	existing, exists := s.settings[k]
	if exists {
		existing.Value = value
		existing.ValueType = valueType
		existing.Encrypted = encrypted
		existing.UpdatedAt = now

		s.queueWrite(s.coreWrites, NewWriteOp(WriteOpUpdate, "settings", existing))
		return nil
	}

	setting := &Setting{
		UID:       xid.New().String(),
		ID:        nextID(&s.nextSettingID),
		Category:  category,
		Key:       key,
		Value:     value,
		ValueType: valueType,
		Encrypted: encrypted,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.settings[k] = setting
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpInsert, "settings", setting))
	return nil
}

// InitSetting seeds a setting only if it does not already exist.
// Used for runtime settings where user edits should survive server restarts.
func (s *MemoryStore) InitSetting(ctx context.Context, category, key, value, valueType string, encrypted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := settingKey(category, key)
	if _, exists := s.settings[k]; exists {
		return nil // Already exists, do not overwrite
	}

	now := time.Now()
	setting := &Setting{
		UID:       xid.New().String(),
		ID:        nextID(&s.nextSettingID),
		Category:  category,
		Key:       key,
		Value:     value,
		ValueType: valueType,
		Encrypted: encrypted,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.settings[k] = setting
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpInsert, "settings", setting))
	return nil
}

// ListSettingsByCategory returns all settings for a category.
func (s *MemoryStore) ListSettingsByCategory(ctx context.Context, category string) ([]*Setting, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Setting
	for _, setting := range s.settings {
		if setting.Category == category {
			cp := *setting
			result = append(result, &cp)
		}
	}

	return result, nil
}

// ListAllSettings returns all settings.
func (s *MemoryStore) ListAllSettings(ctx context.Context) ([]*Setting, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Setting, 0, len(s.settings))
	for _, setting := range s.settings {
		cp := *setting
		result = append(result, &cp)
	}

	return result, nil
}

// DeleteSetting removes a setting.
func (s *MemoryStore) DeleteSetting(ctx context.Context, category, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := settingKey(category, key)
	if _, ok := s.settings[k]; !ok {
		return ErrNotFound
	}

	delete(s.settings, k)
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "settings", k))
	return nil
}
