package storage

import (
	"context"
	"time"
)

// --- Project operations ---

// CreateProject creates a new project in memory and queues persistence.
func (s *MemoryStore) CreateProject(project *Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate name
	if _, exists := s.projectsByName[project.Name]; exists {
		return ErrDuplicate
	}

	project.ID = nextID(&s.nextProjectID)
	now := time.Now()
	project.CreatedAt = now
	project.UpdatedAt = now

	// Store a copy
	stored := *project
	s.projects[project.ID] = &stored
	s.projectsByName[project.Name] = &stored

	// Queue persistence
	s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpInsert, "projects", &stored))

	return nil
}

// GetProjectByID retrieves a project by ID from memory.
func (s *MemoryStore) GetProjectByID(ctx context.Context, id int64) (*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	project, ok := s.projects[id]
	if !ok {
		return nil, ErrNotFound
	}

	// Return a copy
	copied := *project
	return &copied, nil
}

// GetProjectByName retrieves a project by name from memory.
func (s *MemoryStore) GetProjectByName(ctx context.Context, name string) (*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	project, ok := s.projectsByName[name]
	if !ok {
		return nil, ErrNotFound
	}

	// Return a copy
	copied := *project
	return &copied, nil
}

// ListProjects returns all projects from memory.
func (s *MemoryStore) ListProjects() ([]*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	projects := make([]*Project, 0, len(s.projects))
	for _, project := range s.projects {
		copied := *project
		projects = append(projects, &copied)
	}

	return projects, nil
}

// UpdateProjectByID updates a project by ID in memory and queues persistence.
func (s *MemoryStore) UpdateProjectByID(ctx context.Context, p *Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.projects[p.ID]
	if !ok {
		return ErrNotFound
	}

	// Preserve immutable fields
	p.CreatedAt = existing.CreatedAt
	p.UpdatedAt = time.Now()

	// Handle name change - update projectsByName map
	if existing.Name != p.Name {
		delete(s.projectsByName, existing.Name)
	}

	// Store a copy
	stored := *p
	s.projects[p.ID] = &stored
	s.projectsByName[p.Name] = &stored

	// Queue persistence
	s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpUpdate, "projects", &stored))

	return nil
}

// UpdateProjectByName updates a project in memory and queues persistence.
func (s *MemoryStore) UpdateProjectByName(ctx context.Context, p *Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.projectsByName[p.Name]
	if !ok {
		return ErrNotFound
	}

	p.ID = existing.ID
	p.CreatedAt = existing.CreatedAt
	p.UpdatedAt = time.Now()

	// Store a copy
	stored := *p
	s.projects[p.ID] = &stored
	s.projectsByName[p.Name] = &stored

	// Queue persistence
	s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpUpdate, "projects", &stored))

	return nil
}

// UpdateProjectHealthCheck updates health check settings for a project.
func (s *MemoryStore) UpdateProjectHealthCheck(ctx context.Context, projectID int64, healthCheckID *int64, autoRollback, rollbackOnHealthFail bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	project, ok := s.projects[projectID]
	if !ok {
		return ErrNotFound
	}

	project.HealthCheckID = healthCheckID
	project.AutoRollbackEnabled = autoRollback
	project.RollbackOnHealthFail = rollbackOnHealthFail
	project.UpdatedAt = time.Now()

	// Queue persistence
	s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpUpdate, "projects", map[string]any{
		"id":                      projectID,
		"health_check_id":         healthCheckID,
		"auto_rollback_enabled":   autoRollback,
		"rollback_on_health_fail": rollbackOnHealthFail,
	}))

	return nil
}

// DeleteProjectByID removes a project by ID from memory and queues persistence.
func (s *MemoryStore) DeleteProjectByID(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	project, ok := s.projects[id]
	if !ok {
		return nil // Not an error to delete non-existent
	}

	delete(s.projects, id)
	delete(s.projectsByName, project.Name)

	// Cascade delete webhooks for this project
	for webhookID, webhook := range s.webhooks {
		if webhook.ProjectID == id {
			delete(s.webhooks, webhookID)
		}
	}

	// Cascade delete deployments for this project
	for depID, dep := range s.deployments {
		if dep.Project == project.Name || (dep.ProjectID != nil && *dep.ProjectID == id) {
			delete(s.deployments, depID)
			// Also delete associated logs
			delete(s.deploymentLogs, depID)
		}
	}

	// Cascade delete scheduled deployments for this project
	for schedID, sched := range s.scheduledDeploys {
		if sched.Project == project.Name {
			delete(s.scheduledDeploys, schedID)
		}
	}

	// Cascade delete secrets for this project
	for key, secret := range s.secrets {
		if secret.Project == project.Name {
			delete(s.secrets, key)
		}
	}

	// Queue persistence
	s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpDelete, "projects", id))

	return nil
}

// DeleteProject removes a project from memory and queues persistence.
func (s *MemoryStore) DeleteProject(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	project, ok := s.projectsByName[name]
	if !ok {
		return nil // Not an error to delete non-existent
	}

	delete(s.projects, project.ID)
	delete(s.projectsByName, name)

	// Cascade delete webhooks for this project
	for id, webhook := range s.webhooks {
		if webhook.ProjectID == project.ID {
			delete(s.webhooks, id)
		}
	}

	// Cascade delete secrets for this project
	for key, secret := range s.secrets {
		if secret.Project == name {
			delete(s.secrets, key)
		}
	}

	// Cascade delete deployments for this project
	for depID, dep := range s.deployments {
		if dep.Project == name || (dep.ProjectID != nil && *dep.ProjectID == project.ID) {
			delete(s.deployments, depID)
			// Also delete associated logs
			delete(s.deploymentLogs, depID)
		}
	}

	// Cascade delete scheduled deployments for this project
	for schedID, sched := range s.scheduledDeploys {
		if sched.Project == name {
			delete(s.scheduledDeploys, schedID)
		}
	}

	// Queue persistence
	s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpDelete, "projects", map[string]string{"name": name}))

	return nil
}

// --- Project Type operations ---

// CreateProjectType creates a new project type in memory and queues persistence.
func (s *MemoryStore) CreateProjectType(pt *ProjectType) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate name
	for _, existing := range s.projectTypes {
		if existing.Name == pt.Name {
			return ErrDuplicate
		}
	}

	pt.ID = nextID(&s.nextProjectTypeID)
	pt.CreatedAt = time.Now()

	// Store a copy
	stored := *pt
	s.projectTypes[pt.ID] = &stored

	// Queue persistence
	s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpInsert, "project_types", &stored))

	return nil
}

// ListProjectTypes returns all project types from memory.
func (s *MemoryStore) ListProjectTypes() ([]*ProjectType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	types := make([]*ProjectType, 0, len(s.projectTypes))
	for _, pt := range s.projectTypes {
		copied := *pt
		types = append(types, &copied)
	}

	return types, nil
}

// GetProjectTypeByName retrieves a project type by name from memory.
func (s *MemoryStore) GetProjectTypeByName(name string) (*ProjectType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, pt := range s.projectTypes {
		if pt.Name == name {
			copied := *pt
			return &copied, nil
		}
	}

	return nil, ErrNotFound
}

// UpdateProjectTypeByName updates a project type in memory and queues persistence.
func (s *MemoryStore) UpdateProjectTypeByName(pt *ProjectType) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, existing := range s.projectTypes {
		if existing.Name != pt.Name {
			continue
		}
		pt.ID = id
		pt.CreatedAt = existing.CreatedAt

		stored := *pt
		s.projectTypes[id] = &stored

		// Queue persistence
		s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpUpdate, "project_types", &stored))

		return nil
	}

	return ErrNotFound
}

// DeleteProjectType removes a project type from memory and queues persistence.
func (s *MemoryStore) DeleteProjectType(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, pt := range s.projectTypes {
		if pt.Name == name {
			delete(s.projectTypes, id)

			// Queue persistence
			s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpDelete, "project_types", map[string]string{"name": name}))

			return nil
		}
	}

	return nil // Not an error to delete non-existent
}

// --- Project Webhook operations ---

// GetProjectWebhook retrieves a webhook for a project and provider from memory.
func (s *MemoryStore) GetProjectWebhook(ctx context.Context, projectID int64, provider string) (*ProjectWebhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, webhook := range s.webhooks {
		if webhook.ProjectID == projectID && webhook.Provider == provider {
			copied := *webhook
			return &copied, nil
		}
	}

	return nil, ErrNotFound
}

// SetProjectWebhook creates or updates a webhook in memory and queues persistence.
func (s *MemoryStore) SetProjectWebhook(ctx context.Context, projectID int64, provider string, secretEncrypted []byte, enabled, requireSecret bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Check if webhook already exists
	for _, webhook := range s.webhooks {
		if webhook.ProjectID != projectID || webhook.Provider != provider {
			continue
		}
		// Update existing
		webhook.SecretEncrypted = secretEncrypted
		webhook.Enabled = enabled
		webhook.RequireSecret = requireSecret
		webhook.UpdatedAt = now

		// Queue persistence
		s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpUpdate, "project_webhooks", webhook))

		return nil
	}

	// Create new
	webhook := &ProjectWebhook{
		ID:              nextID(&s.nextWebhookID),
		ProjectID:       projectID,
		Provider:        provider,
		SecretEncrypted: secretEncrypted,
		Enabled:         enabled,
		RequireSecret:   requireSecret,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	s.webhooks[webhook.ID] = webhook

	// Queue persistence
	s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpInsert, "project_webhooks", webhook))

	return nil
}

// ListProjectWebhooks returns all webhooks for a project from memory.
func (s *MemoryStore) ListProjectWebhooks(ctx context.Context, projectID int64) ([]*ProjectWebhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var webhooks []*ProjectWebhook
	for _, webhook := range s.webhooks {
		if webhook.ProjectID == projectID {
			copied := *webhook
			webhooks = append(webhooks, &copied)
		}
	}

	return webhooks, nil
}

// DeleteProjectWebhook removes a webhook from memory and queues persistence.
func (s *MemoryStore) DeleteProjectWebhook(ctx context.Context, projectID int64, provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, webhook := range s.webhooks {
		if webhook.ProjectID == projectID && webhook.Provider == provider {
			delete(s.webhooks, id)

			// Queue persistence
			s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpDelete, "project_webhooks", map[string]any{
				"project_id": projectID,
				"provider":   provider,
			}))

			return nil
		}
	}

	return nil // Not an error to delete non-existent
}

// --- Secret operations ---

// SetSecretEncrypted creates or updates a secret in memory and queues persistence.
func (s *MemoryStore) SetSecretEncrypted(ctx context.Context, project, scope, key string, valueEncrypted []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	mapKey := secretKey(project, scope, key)
	now := time.Now()

	existing, ok := s.secrets[mapKey]
	if ok {
		// Update existing
		existing.ValueEncrypted = valueEncrypted
		existing.UpdatedAt = now

		// Queue persistence
		s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpUpdate, "secrets", existing))

		return nil
	}

	// Create new
	secret := &Secret{
		ID:             nextID(&s.nextSecretID),
		Project:        project,
		Scope:          scope,
		Key:            key,
		ValueEncrypted: valueEncrypted,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	s.secrets[mapKey] = secret

	// Queue persistence
	s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpInsert, "secrets", secret))

	return nil
}

// GetSecret retrieves a secret by project, scope, and key from memory.
func (s *MemoryStore) GetSecret(ctx context.Context, project, scope, key string) (*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	mapKey := secretKey(project, scope, key)
	secret, ok := s.secrets[mapKey]
	if !ok {
		return nil, ErrNotFound
	}

	// Return a copy
	copied := *secret
	return &copied, nil
}

// ListSecrets returns secret metadata for a scope (legacy method without context).
func (s *MemoryStore) ListSecrets(scope string) ([]*SecretInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var infos []*SecretInfo
	for _, secret := range s.secrets {
		if secret.Scope == scope {
			infos = append(infos, &SecretInfo{
				Key:       secret.Key,
				Scope:     secret.Scope,
				UpdatedAt: secret.UpdatedAt,
			})
		}
	}

	return infos, nil
}

// ListSecretsCtx returns all secrets for a project from memory.
func (s *MemoryStore) ListSecretsCtx(ctx context.Context, project string) ([]*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var secrets []*Secret
	for _, secret := range s.secrets {
		if secret.Project == project {
			copied := *secret
			secrets = append(secrets, &copied)
		}
	}

	return secrets, nil
}

// ListSecretsWithScope returns secrets for a project and scope from memory.
func (s *MemoryStore) ListSecretsWithScope(ctx context.Context, project, scope string) ([]*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var secrets []*Secret
	for _, secret := range s.secrets {
		if secret.Project == project && secret.Scope == scope {
			copied := *secret
			secrets = append(secrets, &copied)
		}
	}

	return secrets, nil
}

// ListAllSecretsCtx returns all secrets from memory.
func (s *MemoryStore) ListAllSecretsCtx(ctx context.Context) ([]*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	secrets := make([]*Secret, 0, len(s.secrets))
	for _, secret := range s.secrets {
		copied := *secret
		secrets = append(secrets, &copied)
	}

	return secrets, nil
}

// DeleteSecret removes a secret from memory (legacy method without project).
func (s *MemoryStore) DeleteSecret(scope, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find and delete secrets matching scope and key
	for mapKey, secret := range s.secrets {
		if secret.Scope == scope && secret.Key == key {
			delete(s.secrets, mapKey)

			// Queue persistence
			s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpDelete, "secrets", map[string]string{
				"scope": scope,
				"key":   key,
			}))

			return nil
		}
	}

	return nil // Not an error to delete non-existent
}

// DeleteSecretCtx removes a secret from memory.
func (s *MemoryStore) DeleteSecretCtx(ctx context.Context, project, scope, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	mapKey := secretKey(project, scope, key)
	delete(s.secrets, mapKey)

	// Queue persistence
	s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpDelete, "secrets", map[string]string{
		"project": project,
		"scope":   scope,
		"key":     key,
	}))

	return nil
}

// ExportAllSecrets exports all secrets grouped by project/scope.
// Returns map[project]map[key]scope
func (s *MemoryStore) ExportAllSecrets() (map[string]map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]map[string]string)
	for _, secret := range s.secrets {
		if result[secret.Project] == nil {
			result[secret.Project] = make(map[string]string)
		}
		result[secret.Project][secret.Key] = secret.Scope
	}

	return result, nil
}
