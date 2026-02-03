// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"fmt"

	"go.uber.org/zap"
)

// --- Project operations ---

// CreateProject creates a new project.
func (db *DB) CreateProject(project *Project) error {
	result, err := db.conn.Exec(`
		INSERT INTO projects (name, repository, branch, deploy_path, type, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, project.Name, project.Repository, project.Branch, project.DeployPath, project.Type, project.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		db.logger.Warn("failed to get LastInsertId for project", zap.Error(err))
	}
	project.ID = id
	return nil
}

// GetProjectByName retrieves a project by name with context.
func (db *DB) GetProjectByName(ctx context.Context, name string) (*Project, error) {
	var p Project
	var lastDeploy sql.NullTime
	var lastDeployStatus sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, name, repository, branch, deploy_path, type, created_at, last_deploy_at, last_deploy_status
		FROM projects WHERE name = ?
	`, name).Scan(&p.ID, &p.Name, &p.Repository, &p.Branch, &p.DeployPath, &p.Type, &p.CreatedAt, &lastDeploy, &lastDeployStatus)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query project: %w", err)
	}

	if lastDeploy.Valid {
		p.LastDeployAt = &lastDeploy.Time
	}
	p.LastDeployStatus = lastDeployStatus.String
	return &p, nil
}

// ListProjects returns all projects.
func (db *DB) ListProjects() ([]*Project, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, repository, branch, deploy_path, type, created_at, last_deploy_at, last_deploy_status
		FROM projects ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		var p Project
		var lastDeploy sql.NullTime
		var lastDeployStatus sql.NullString

		if err := rows.Scan(&p.ID, &p.Name, &p.Repository, &p.Branch, &p.DeployPath, &p.Type, &p.CreatedAt, &lastDeploy, &lastDeployStatus); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		if lastDeploy.Valid {
			p.LastDeployAt = &lastDeploy.Time
		}
		p.LastDeployStatus = lastDeployStatus.String
		projects = append(projects, &p)
	}
	return projects, rows.Err()
}

// ListProjectsPaginated returns projects with pagination support.
func (db *DB) ListProjectsPaginated(ctx context.Context, limit, offset int) ([]*Project, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, name, repository, branch, deploy_path, type, created_at, last_deploy_at, last_deploy_status
		FROM projects ORDER BY name LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		var p Project
		var lastDeploy sql.NullTime
		var lastDeployStatus sql.NullString

		if err := rows.Scan(&p.ID, &p.Name, &p.Repository, &p.Branch, &p.DeployPath, &p.Type, &p.CreatedAt, &lastDeploy, &lastDeployStatus); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		if lastDeploy.Valid {
			p.LastDeployAt = &lastDeploy.Time
		}
		p.LastDeployStatus = lastDeployStatus.String
		projects = append(projects, &p)
	}
	return projects, rows.Err()
}

// CountProjects returns the total number of projects.
func (db *DB) CountProjects(ctx context.Context) (int64, error) {
	var count int64
	err := db.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting projects: %w", err)
	}
	return count, nil
}

// DeleteProject deletes a project.
func (db *DB) DeleteProject(name string) error {
	_, err := db.conn.Exec(`DELETE FROM projects WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("deleting project: %w", err)
	}
	return nil
}

// GetProjectByID retrieves a project by ID with context.
func (db *DB) GetProjectByID(ctx context.Context, id int64) (*Project, error) {
	var p Project
	var lastDeploy sql.NullTime
	var lastDeployStatus sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, name, repository, branch, deploy_path, type, created_at, last_deploy_at, last_deploy_status
		FROM projects WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Repository, &p.Branch, &p.DeployPath, &p.Type, &p.CreatedAt, &lastDeploy, &lastDeployStatus)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query project: %w", err)
	}

	if lastDeploy.Valid {
		p.LastDeployAt = &lastDeploy.Time
	}
	p.LastDeployStatus = lastDeployStatus.String
	return &p, nil
}

// UpdateProjectByID updates a project by ID.
func (db *DB) UpdateProjectByID(ctx context.Context, p *Project) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE projects SET name = ?, repository = ?, branch = ?, deploy_path = ?, type = ?
		WHERE id = ?
	`, p.Name, p.Repository, p.Branch, p.DeployPath, p.Type, p.ID)
	if err != nil {
		return fmt.Errorf("updating project: %w", err)
	}
	return nil
}

// DeleteProjectByID deletes a project by ID.
func (db *DB) DeleteProjectByID(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting project: %w", err)
	}
	return nil
}

// --- Project Type operations ---

// CreateProjectType creates a new project type.
func (db *DB) CreateProjectType(pt *ProjectType) error {
	result, err := db.conn.Exec(`
		INSERT INTO project_types (name, description, build_cmd, created_at)
		VALUES (?, ?, ?, ?)
	`, pt.Name, pt.Description, pt.BuildCmd, pt.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert project type: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		db.logger.Warn("failed to get LastInsertId for project type", zap.Error(err))
	}
	pt.ID = id
	return nil
}

// ListProjectTypes returns all project types.
func (db *DB) ListProjectTypes() ([]*ProjectType, error) {
	rows, err := db.conn.Query(`
		SELECT pt.id, pt.name, pt.description, pt.build_cmd, 
		       (SELECT COUNT(*) FROM projects WHERE type = pt.name) as project_count,
		       pt.created_at
		FROM project_types pt ORDER BY pt.name
	`)
	if err != nil {
		return nil, fmt.Errorf("query project types: %w", err)
	}
	defer rows.Close()

	var types []*ProjectType
	for rows.Next() {
		var pt ProjectType
		if err := rows.Scan(&pt.ID, &pt.Name, &pt.Description, &pt.BuildCmd, &pt.ProjectCount, &pt.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project type: %w", err)
		}
		types = append(types, &pt)
	}
	return types, rows.Err()
}

// GetProjectTypeByName retrieves a project type by name.
func (db *DB) GetProjectTypeByName(name string) (*ProjectType, error) {
	var pt ProjectType
	err := db.conn.QueryRow(`
		SELECT pt.id, pt.name, pt.description, pt.build_cmd, 
		       (SELECT COUNT(*) FROM projects WHERE type = pt.name) as project_count,
		       pt.created_at
		FROM project_types pt WHERE pt.name = ?
	`, name).Scan(&pt.ID, &pt.Name, &pt.Description, &pt.BuildCmd, &pt.ProjectCount, &pt.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project type not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("query project type: %w", err)
	}
	return &pt, nil
}

// UpdateProjectTypeByName updates a project type by name.
func (db *DB) UpdateProjectTypeByName(pt *ProjectType) error {
	_, err := db.conn.Exec(`
		UPDATE project_types SET description = ?, build_cmd = ?
		WHERE name = ?
	`, pt.Description, pt.BuildCmd, pt.Name)
	if err != nil {
		return fmt.Errorf("updating project type: %w", err)
	}
	return nil
}

// DeleteProjectType deletes a project type.
func (db *DB) DeleteProjectType(name string) error {
	_, err := db.conn.Exec(`DELETE FROM project_types WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("deleting project type: %w", err)
	}
	return nil
}

// ExportAllSecrets exports all secrets (for backup)
func (db *DB) ExportAllSecrets() (map[string]map[string]string, error) {
	rows, err := db.conn.Query(`SELECT project, key, value_encrypted FROM secrets`)
	if err != nil {
		return nil, fmt.Errorf("querying secrets for export: %w", err)
	}
	defer rows.Close()

	result := make(map[string]map[string]string)
	for rows.Next() {
		var project, key string
		var value []byte
		if err := rows.Scan(&project, &key, &value); err != nil {
			return nil, fmt.Errorf("scanning secret for export: %w", err)
		}
		if result[project] == nil {
			result[project] = make(map[string]string)
		}
		result[project][key] = string(value)
	}
	return result, rows.Err()
}

// --- Project Webhook operations ---

// GetProjectWebhook retrieves a webhook config for a project and provider.
func (db *DB) GetProjectWebhook(ctx context.Context, projectID int64, provider string) (*ProjectWebhook, error) {
	var w ProjectWebhook
	var enabled, requireSecret int
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, project_id, provider, secret_encrypted, enabled, COALESCE(require_secret, 0), created_at, updated_at
		FROM project_webhooks WHERE project_id = ? AND provider = ?
	`, projectID, provider).Scan(
		&w.ID, &w.ProjectID, &w.Provider, &w.SecretEncrypted, &enabled, &requireSecret, &w.CreatedAt, &w.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying project webhook: %w", err)
	}
	w.Enabled = enabled == 1
	w.RequireSecret = requireSecret == 1
	return &w, nil
}

// SetProjectWebhook creates or updates a webhook config.
func (db *DB) SetProjectWebhook(ctx context.Context, projectID int64, provider string, secretEncrypted []byte, enabled, requireSecret bool) error {
	enabledVal := 0
	if enabled {
		enabledVal = 1
	}
	requireSecretVal := 0
	if requireSecret {
		requireSecretVal = 1
	}
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO project_webhooks (project_id, provider, secret_encrypted, enabled, require_secret)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project_id, provider) DO UPDATE SET
			secret_encrypted = excluded.secret_encrypted,
			enabled = excluded.enabled,
			require_secret = excluded.require_secret,
			updated_at = CURRENT_TIMESTAMP
	`, projectID, provider, secretEncrypted, enabledVal, requireSecretVal)
	if err != nil {
		return fmt.Errorf("setting project webhook: %w", err)
	}
	return nil
}

// ListProjectWebhooks retrieves all webhooks for a project.
func (db *DB) ListProjectWebhooks(ctx context.Context, projectID int64) ([]*ProjectWebhook, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project_id, provider, secret_encrypted, enabled, COALESCE(require_secret, 0), created_at, updated_at
		FROM project_webhooks WHERE project_id = ? ORDER BY provider
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("querying project webhooks: %w", err)
	}
	defer rows.Close()

	var webhooks []*ProjectWebhook
	for rows.Next() {
		var w ProjectWebhook
		var enabled, requireSecret int
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.Provider, &w.SecretEncrypted, &enabled, &requireSecret, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning webhook: %w", err)
		}
		w.Enabled = enabled == 1
		w.RequireSecret = requireSecret == 1
		webhooks = append(webhooks, &w)
	}
	return webhooks, rows.Err()
}

// DeleteProjectWebhook deletes a webhook config.
func (db *DB) DeleteProjectWebhook(ctx context.Context, projectID int64, provider string) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM project_webhooks WHERE project_id = ? AND provider = ?`, projectID, provider)
	if err != nil {
		return fmt.Errorf("deleting project webhook: %w", err)
	}
	return nil
}

// --- Additional Project operations ---

// UpdateProjectByName updates a project by name.
func (db *DB) UpdateProjectByName(ctx context.Context, p *Project) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE projects SET repository = ?, branch = ?, deploy_path = ?, type = ?
		WHERE name = ?
	`, p.Repository, p.Branch, p.DeployPath, p.Type, p.Name)
	if err != nil {
		return fmt.Errorf("updating project: %w", err)
	}
	return nil
}
