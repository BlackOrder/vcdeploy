// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// DB wraps the SQLite database connection.
type DB struct {
	conn   *sql.DB
	path   string
	logger *zap.Logger
}

// New creates a new database connection.
func New(path string, logger *zap.Logger) (*DB, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Configure connection pool
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	db := &DB{conn: conn, path: path, logger: logger}

	// Handle migration from legacy inline schema
	if err := db.migrateFromLegacy(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("legacy migration check: %w", err)
	}

	// Run versioned migrations
	if err := db.MigrateUp(context.Background()); err != nil {
		conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return db, nil
}

// Open is an alias for New
func Open(path string) (*DB, error) {
	return New(path, nil)
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying sql.DB connection.
// Use this when you need direct database access (e.g., for KMS initialization).
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// --- User operations ---

// User represents a user in the system.
type User struct {
	ID                 int64
	Username           string
	PasswordHash       string
	Email              string
	Role               string
	TOTPSecret         string
	TOTPEnabled        bool
	MustChangePassword bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// CreateUser creates a new user.
func (db *DB) CreateUser(ctx context.Context, user *User) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, role, must_change_password)
		VALUES (?, ?, ?, ?, ?)
	`, user.Username, user.PasswordHash, user.Email, user.Role, user.MustChangePassword)
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

// --- Agent operations ---

// Agent represents a connected agent.
type Agent struct {
	ID           string
	Hostname     string
	Labels       map[string]string
	Capabilities string // JSON string
	Status       string
	LastSeenAt   time.Time
	RegisteredAt time.Time
	Certificate  string
}

// UpsertAgent creates or updates an agent.
func (db *DB) UpsertAgent(ctx context.Context, agent *Agent) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO agents (id, hostname, labels, capabilities, status, last_seen_at, certificate)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			hostname = excluded.hostname,
			labels = excluded.labels,
			capabilities = excluded.capabilities,
			status = excluded.status,
			last_seen_at = excluded.last_seen_at,
			certificate = COALESCE(excluded.certificate, agents.certificate)
	`, agent.ID, agent.Hostname, mapToJSON(agent.Labels), agent.Capabilities,
		agent.Status, agent.LastSeenAt, agent.Certificate)
	return err
}

// GetAgent retrieves an agent by ID.
func (db *DB) GetAgent(ctx context.Context, id string) (*Agent, error) {
	var agent Agent
	var labels, capabilities sql.NullString
	var lastSeen sql.NullTime

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, hostname, labels, capabilities, status, last_seen_at, registered_at, certificate
		FROM agents WHERE id = ?
	`, id).Scan(
		&agent.ID, &agent.Hostname, &labels, &capabilities,
		&agent.Status, &lastSeen, &agent.RegisteredAt, &agent.Certificate,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying agent: %w", err)
	}

	agent.Labels = jsonToMap(labels.String)
	agent.LastSeenAt = lastSeen.Time
	return &agent, nil
}

// ListAgents returns all agents.
func (db *DB) ListAgents(ctx context.Context) ([]*Agent, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, hostname, labels, capabilities, status, last_seen_at, registered_at
		FROM agents ORDER BY hostname
	`)
	if err != nil {
		return nil, fmt.Errorf("querying agents: %w", err)
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		var agent Agent
		var labels, capabilities sql.NullString
		var lastSeen sql.NullTime

		if err := rows.Scan(&agent.ID, &agent.Hostname, &labels, &capabilities,
			&agent.Status, &lastSeen, &agent.RegisteredAt); err != nil {
			return nil, fmt.Errorf("scanning agent: %w", err)
		}

		agent.Labels = jsonToMap(labels.String)
		agent.LastSeenAt = lastSeen.Time
		agents = append(agents, &agent)
	}

	return agents, rows.Err()
}

// DeleteAgent removes an agent by ID.
func (db *DB) DeleteAgent(ctx context.Context, id string) error {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM agents WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting agent: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("agent not found: %s", id)
	}
	return nil
}

// --- Deployment operations ---

// Deployment represents a deployment record.
type Deployment struct {
	ID            string
	Project       string
	Target        string
	Branch        string
	CommitHash    string
	Status        string
	ReleaseNumber int
	StartedAt     time.Time
	CompletedAt   *time.Time
	TriggeredBy   string
	TriggerSource string
	ErrorMessage  string
}

// CreateDeployment creates a new deployment record.
func (db *DB) CreateDeployment(ctx context.Context, d *Deployment) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO deployments (id, project, target, branch, commit_hash, status, triggered_by, trigger_source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, d.ID, d.Project, d.Target, d.Branch, d.CommitHash, d.Status, d.TriggeredBy, d.TriggerSource)
	return err
}

// UpdateDeployment updates a deployment record.
func (db *DB) UpdateDeployment(ctx context.Context, d *Deployment) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE deployments SET
			status = ?, release_number = ?, completed_at = ?, error_message = ?
		WHERE id = ?
	`, d.Status, d.ReleaseNumber, d.CompletedAt, d.ErrorMessage, d.ID)
	return err
}

// GetDeployment retrieves a deployment by ID.
func (db *DB) GetDeployment(ctx context.Context, id string) (*Deployment, error) {
	var d Deployment
	var completedAt sql.NullTime
	var releaseNumber sql.NullInt64
	var commitHash, triggeredBy, triggerSource, errorMessage sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, project, target, branch, commit_hash, status, release_number,
		       started_at, completed_at, triggered_by, trigger_source, error_message
		FROM deployments WHERE id = ?
	`, id).Scan(
		&d.ID, &d.Project, &d.Target, &d.Branch, &commitHash, &d.Status, &releaseNumber,
		&d.StartedAt, &completedAt, &triggeredBy, &triggerSource, &errorMessage,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying deployment: %w", err)
	}

	if completedAt.Valid {
		d.CompletedAt = &completedAt.Time
	}
	if releaseNumber.Valid {
		d.ReleaseNumber = int(releaseNumber.Int64)
	}
	d.CommitHash = commitHash.String
	d.TriggeredBy = triggeredBy.String
	d.TriggerSource = triggerSource.String
	d.ErrorMessage = errorMessage.String
	return &d, nil
}

// --- Deployment log operations ---

// DeploymentLog represents a log entry for a deployment.
type DeploymentLog struct {
	ID           int64
	DeploymentID string
	Level        string
	Message      string
	Source       string
	CreatedAt    time.Time
}

// CreateDeploymentLog creates a deployment log entry.
func (db *DB) CreateDeploymentLog(ctx context.Context, log *DeploymentLog) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO deployment_logs (deployment_id, level, message, source, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, log.DeploymentID, log.Level, log.Message, log.Source, log.CreatedAt)
	return err
}

// ListDeploymentLogs returns logs for a deployment.
func (db *DB) ListDeploymentLogs(ctx context.Context, deploymentID string) ([]*DeploymentLog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, deployment_id, level, message, source, created_at
		FROM deployment_logs WHERE deployment_id = ? ORDER BY created_at
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("querying deployment logs: %w", err)
	}
	defer rows.Close()

	var logs []*DeploymentLog
	for rows.Next() {
		var log DeploymentLog
		if err := rows.Scan(&log.ID, &log.DeploymentID, &log.Level, &log.Message, &log.Source, &log.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning deployment log: %w", err)
		}
		logs = append(logs, &log)
	}
	return logs, rows.Err()
}

// ListDeploymentLogsAfter returns logs for a deployment after a specific log ID.
// This is used for streaming/polling new logs.
func (db *DB) ListDeploymentLogsAfter(ctx context.Context, deploymentID string, afterID int64) ([]*DeploymentLog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, deployment_id, level, message, source, created_at
		FROM deployment_logs WHERE deployment_id = ? AND id > ? ORDER BY created_at
	`, deploymentID, afterID)
	if err != nil {
		return nil, fmt.Errorf("querying deployment logs: %w", err)
	}
	defer rows.Close()

	var logs []*DeploymentLog
	for rows.Next() {
		var log DeploymentLog
		if err := rows.Scan(&log.ID, &log.DeploymentID, &log.Level, &log.Message, &log.Source, &log.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning deployment log: %w", err)
		}
		logs = append(logs, &log)
	}
	return logs, rows.Err()
}

// --- Audit log operations ---

// AuditEntry represents an audit log entry.
type AuditEntry struct {
	ID        int64
	Timestamp time.Time
	Source    string
	User      string
	Action    string
	Resource  string
	Details   string
	IPAddress string
	Result    string
}

// LogAudit creates an audit log entry.
func (db *DB) LogAudit(ctx context.Context, entry *AuditEntry) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO audit_logs (source, user, action, resource, details, ip_address, result)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, entry.Source, entry.User, entry.Action, entry.Resource, entry.Details, entry.IPAddress, entry.Result)
	return err
}

// ListAuditLogs returns audit log entries with optional filtering.
func (db *DB) ListAuditLogs(ctx context.Context, limit int, offset int) ([]*AuditEntry, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, timestamp, source, user, action, resource, details, ip_address, result
		FROM audit_logs ORDER BY timestamp DESC LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying audit logs: %w", err)
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		var entry AuditEntry
		if err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Source, &entry.User,
			&entry.Action, &entry.Resource, &entry.Details, &entry.IPAddress, &entry.Result); err != nil {
			return nil, fmt.Errorf("scanning audit entry: %w", err)
		}
		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// --- Secret operations ---

// Secret represents an encrypted secret.
type Secret struct {
	ID             int64
	Project        string
	Scope          string
	Key            string
	ValueEncrypted []byte
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SetSecretEncrypted creates or updates a secret with pre-encrypted value.
func (db *DB) SetSecretEncrypted(ctx context.Context, project, scope, key string, valueEncrypted []byte) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO secrets (project, scope, key, value_encrypted)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(project, scope, key) DO UPDATE SET
			value_encrypted = excluded.value_encrypted,
			updated_at = CURRENT_TIMESTAMP
	`, project, scope, key, valueEncrypted)
	return err
}

// GetSecret retrieves a secret.
func (db *DB) GetSecret(ctx context.Context, project, scope, key string) (*Secret, error) {
	var s Secret
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, project, scope, key, value_encrypted, created_at, updated_at
		FROM secrets WHERE project = ? AND scope = ? AND key = ?
	`, project, scope, key).Scan(
		&s.ID, &s.Project, &s.Scope, &s.Key, &s.ValueEncrypted, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying secret: %w", err)
	}
	return &s, nil
}

// SecretInfo represents secret metadata
type SecretInfo struct {
	Key       string
	Scope     string
	UpdatedAt time.Time
}

// ListSecrets returns secret metadata for a scope (CLI version)
func (db *DB) ListSecrets(scope string) ([]*SecretInfo, error) {
	rows, err := db.conn.Query(`
		SELECT key, scope, updated_at FROM secrets WHERE project = ? ORDER BY key
	`, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []*SecretInfo
	for rows.Next() {
		var s SecretInfo
		if err := rows.Scan(&s.Key, &s.Scope, &s.UpdatedAt); err != nil {
			return nil, err
		}
		secrets = append(secrets, &s)
	}
	return secrets, rows.Err()
}

// ListSecretsCtx returns all secrets for a project (without values) with context.
func (db *DB) ListSecretsCtx(ctx context.Context, project string) ([]*Secret, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project, scope, key, created_at, updated_at
		FROM secrets WHERE project = ? ORDER BY scope, key
	`, project)
	if err != nil {
		return nil, fmt.Errorf("querying secrets: %w", err)
	}
	defer rows.Close()

	var secrets []*Secret
	for rows.Next() {
		var s Secret
		if err := rows.Scan(&s.ID, &s.Project, &s.Scope, &s.Key, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning secret: %w", err)
		}
		secrets = append(secrets, &s)
	}

	return secrets, rows.Err()
}

// DeleteSecret deletes a secret (CLI version).
func (db *DB) DeleteSecret(scope, key string) error {
	_, err := db.conn.Exec(`DELETE FROM secrets WHERE project = ? AND key = ?`, scope, key)
	return err
}

// DeleteSecretCtx deletes a secret with context.
func (db *DB) DeleteSecretCtx(ctx context.Context, project, scope, key string) error {
	_, err := db.conn.ExecContext(ctx, `
		DELETE FROM secrets WHERE project = ? AND scope = ? AND key = ?
	`, project, scope, key)
	return err
}

// ListSecretsWithScope returns all secrets for a project and scope with encrypted values.
func (db *DB) ListSecretsWithScope(ctx context.Context, project, scope string) ([]*Secret, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project, scope, key, value_encrypted, created_at, updated_at
		FROM secrets WHERE project = ? AND scope = ? ORDER BY key
	`, project, scope)
	if err != nil {
		return nil, fmt.Errorf("querying secrets: %w", err)
	}
	defer rows.Close()

	var secrets []*Secret
	for rows.Next() {
		var s Secret
		if err := rows.Scan(&s.ID, &s.Project, &s.Scope, &s.Key, &s.ValueEncrypted, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning secret: %w", err)
		}
		secrets = append(secrets, &s)
	}
	return secrets, rows.Err()
}

// ListAllSecretsCtx returns all secrets from all projects (for re-encryption).
func (db *DB) ListAllSecretsCtx(ctx context.Context) ([]*Secret, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project, scope, key, value_encrypted, created_at, updated_at
		FROM secrets ORDER BY project, scope, key
	`)
	if err != nil {
		return nil, fmt.Errorf("querying all secrets: %w", err)
	}
	defer rows.Close()

	var secrets []*Secret
	for rows.Next() {
		var s Secret
		if err := rows.Scan(&s.ID, &s.Project, &s.Scope, &s.Key, &s.ValueEncrypted, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning secret: %w", err)
		}
		secrets = append(secrets, &s)
	}
	return secrets, rows.Err()
}

// --- Helper functions ---

func mapToJSON(m map[string]string) string {
	if len(m) == 0 { // nil map has len 0
		return "{}"
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func jsonToMap(s string) map[string]string {
	if s == "" || s == "{}" {
		return make(map[string]string)
	}
	result := make(map[string]string)
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return make(map[string]string)
	}
	return result
}

// Backup creates a backup of the database
func (db *DB) Backup(destPath string) error {
	// Close current operations and copy file
	src, err := os.Open(db.path)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy database: %w", err)
	}
	return nil
}

// --- Project operations ---

// Project represents a deployment project.
type Project struct {
	ID               int64
	Name             string
	Repository       string
	Branch           string
	DeployPath       string
	Type             string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	LastDeployAt     *time.Time
	LastDeployStatus string
}

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

// DeleteProject deletes a project.
func (db *DB) DeleteProject(name string) error {
	_, err := db.conn.Exec(`DELETE FROM projects WHERE name = ?`, name)
	return err
}

// --- Project Type operations ---

// ProjectType represents a project type template.
type ProjectType struct {
	ID           int64
	Name         string
	Description  string
	BuildCmd     string
	ProjectCount int
	CreatedAt    time.Time
}

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
	return err
}

// DeleteProjectType deletes a project type.
func (db *DB) DeleteProjectType(name string) error {
	_, err := db.conn.Exec(`DELETE FROM project_types WHERE name = ?`, name)
	return err
}

// --- Extended Deployment operations for CLI ---

// DeploymentCLI is a simplified deployment struct for CLI use
type DeploymentCLI struct {
	ID          string
	ProjectID   int64
	ProjectName string
	Target      string
	Status      string
	TriggeredBy string
	StartedAt   time.Time
	FinishedAt  *time.Time
}

// InsertDeployment creates a deployment record (CLI version - alternate method)
func (db *DB) InsertDeployment(d *DeploymentCLI) error {
	if d.ID == "" {
		d.ID = fmt.Sprintf("deploy-%d", time.Now().UnixNano())
	}
	_, err := db.conn.Exec(`
		INSERT INTO deployments (id, project, target, branch, status, triggered_by, started_at)
		VALUES (?, ?, ?, '', ?, ?, ?)
	`, d.ID, d.ProjectName, d.Target, d.Status, d.TriggeredBy, d.StartedAt)
	return err
}

// SaveDeployment updates a deployment record (CLI version - alternate method)
func (db *DB) SaveDeployment(d *DeploymentCLI) error {
	_, err := db.conn.Exec(`
		UPDATE deployments SET status = ?, completed_at = ? WHERE id = ?
	`, d.Status, d.FinishedAt, d.ID)
	return err
}

// ExportAllSecrets exports all secrets (for backup)
func (db *DB) ExportAllSecrets() (map[string]map[string]string, error) {
	rows, err := db.conn.Query(`SELECT project, key, value_encrypted FROM secrets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]map[string]string)
	for rows.Next() {
		var project, key string
		var value []byte
		if err := rows.Scan(&project, &key, &value); err != nil {
			return nil, err
		}
		if result[project] == nil {
			result[project] = make(map[string]string)
		}
		result[project][key] = string(value)
	}
	return result, rows.Err()
}

// --- Session Management ---

// Session represents a user session.
type Session struct {
	ID        string // TEXT primary key
	UserID    int64
	Token     string // Same as ID for sessions
	IPAddress string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
	LastUsed  time.Time
}

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
	return err
}

// DeleteExpiredSessions removes all expired sessions.
func (db *DB) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, time.Now())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteUserSessions deletes all sessions for a user.
func (db *DB) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// ListUserSessions returns all active sessions for a user.
func (db *DB) ListUserSessions(ctx context.Context, userID int64) ([]*Session, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, user_id, ip_address, user_agent, created_at, expires_at
		FROM sessions WHERE user_id = ? AND expires_at > ?
		ORDER BY created_at DESC
	`, userID, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var s Session
		var ipAddress, userAgent sql.NullString
		err := rows.Scan(&s.ID, &s.UserID, &ipAddress, &userAgent,
			&s.CreatedAt, &s.ExpiresAt)
		if err != nil {
			return nil, err
		}
		s.Token = s.ID
		s.IPAddress = ipAddress.String
		s.UserAgent = userAgent.String
		sessions = append(sessions, &s)
	}
	return sessions, rows.Err()
}

// --- API Key Management ---

// APIKey represents an API key.
type APIKey struct {
	ID         int64
	UserID     int64
	Name       string
	KeyHash    string // SHA-256 hash of the key
	KeyPrefix  string // First 8 characters of the key for identification
	Scopes     string // JSON array of scopes/permissions
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// CreateAPIKey creates a new API key.
func (db *DB) CreateAPIKey(ctx context.Context, key *APIKey) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO api_keys (user_id, name, key_hash, scopes, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, key.UserID, key.Name, key.KeyHash, key.Scopes, key.ExpiresAt, key.CreatedAt)
	if err != nil {
		return fmt.Errorf("create API key: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		db.logger.Warn("failed to get LastInsertId for API key", zap.Error(err))
	}
	key.ID = id
	return nil
}

// GetAPIKeyByHash retrieves an API key by its hash.
func (db *DB) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	var key APIKey
	var expiresAt, lastUsedAt sql.NullTime
	var scopes sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, user_id, name, key_hash, scopes, expires_at, last_used_at, created_at
		FROM api_keys WHERE key_hash = ?
	`, keyHash).Scan(
		&key.ID, &key.UserID, &key.Name, &key.KeyHash,
		&scopes, &expiresAt, &lastUsedAt, &key.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get API key: %w", err)
	}

	key.Scopes = scopes.String
	if expiresAt.Valid {
		key.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		key.LastUsedAt = &lastUsedAt.Time
	}

	return &key, nil
}

// UpdateAPIKeyUsage updates the last used timestamp for an API key.
func (db *DB) UpdateAPIKeyUsage(ctx context.Context, keyID int64) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE api_keys SET last_used_at = ? WHERE id = ?
	`, time.Now(), keyID)
	return err
}

// DeleteAPIKey permanently deletes an API key.
func (db *DB) DeleteAPIKey(ctx context.Context, keyID int64) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, keyID)
	return err
}

// ListAPIKeys returns all API keys for a user.
func (db *DB) ListAPIKeys(ctx context.Context, userID int64) ([]*APIKey, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, user_id, name, key_hash, scopes, expires_at, last_used_at, created_at
		FROM api_keys WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*APIKey
	for rows.Next() {
		var key APIKey
		var expiresAt, lastUsedAt sql.NullTime
		var scopes sql.NullString

		err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.KeyHash,
			&scopes, &expiresAt, &lastUsedAt, &key.CreatedAt)
		if err != nil {
			return nil, err
		}

		key.Scopes = scopes.String
		if expiresAt.Valid {
			key.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			key.LastUsedAt = &lastUsedAt.Time
		}

		keys = append(keys, &key)
	}
	return keys, rows.Err()
}

// IsAPIKeyValid checks if an API key is valid (not expired).
func (key *APIKey) IsValid() bool {
	if key == nil {
		return false
	}
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return false
	}
	return true
}

// --- Settings operations ---

// Setting represents a configuration setting.
type Setting struct {
	ID          int64
	Category    string
	Key         string
	Value       string
	ValueType   string
	Encrypted   bool
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// GetSetting retrieves a single setting.
func (db *DB) GetSetting(ctx context.Context, category, key string) (*Setting, error) {
	var s Setting
	var encrypted int
	var description sql.NullString
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, category, key, value, value_type, encrypted, description, created_at, updated_at
		FROM settings WHERE category = ? AND key = ?
	`, category, key).Scan(
		&s.ID, &s.Category, &s.Key, &s.Value, &s.ValueType, &encrypted, &description, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying setting: %w", err)
	}
	s.Encrypted = encrypted == 1
	s.Description = description.String
	return &s, nil
}

// SetSetting creates or updates a setting.
func (db *DB) SetSetting(ctx context.Context, category, key, value, valueType string, encrypted bool) error {
	encVal := 0
	if encrypted {
		encVal = 1
	}
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO settings (category, key, value, value_type, encrypted)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(category, key) DO UPDATE SET
			value = excluded.value,
			value_type = excluded.value_type,
			encrypted = excluded.encrypted,
			updated_at = CURRENT_TIMESTAMP
	`, category, key, value, valueType, encVal)
	return err
}

// ListSettingsByCategory retrieves all settings in a category.
func (db *DB) ListSettingsByCategory(ctx context.Context, category string) ([]*Setting, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, category, key, value, value_type, encrypted, description, created_at, updated_at
		FROM settings WHERE category = ? ORDER BY key
	`, category)
	if err != nil {
		return nil, fmt.Errorf("querying settings: %w", err)
	}
	defer rows.Close()

	var settings []*Setting
	for rows.Next() {
		var s Setting
		var encrypted int
		var description sql.NullString
		if err := rows.Scan(&s.ID, &s.Category, &s.Key, &s.Value, &s.ValueType, &encrypted, &description, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning setting: %w", err)
		}
		s.Encrypted = encrypted == 1
		s.Description = description.String
		settings = append(settings, &s)
	}
	return settings, rows.Err()
}

// ListAllSettings retrieves all settings.
func (db *DB) ListAllSettings(ctx context.Context) ([]*Setting, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, category, key, value, value_type, encrypted, description, created_at, updated_at
		FROM settings ORDER BY category, key
	`)
	if err != nil {
		return nil, fmt.Errorf("querying settings: %w", err)
	}
	defer rows.Close()

	var settings []*Setting
	for rows.Next() {
		var s Setting
		var encrypted int
		var description sql.NullString
		if err := rows.Scan(&s.ID, &s.Category, &s.Key, &s.Value, &s.ValueType, &encrypted, &description, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning setting: %w", err)
		}
		s.Encrypted = encrypted == 1
		s.Description = description.String
		settings = append(settings, &s)
	}
	return settings, rows.Err()
}

// DeleteSetting deletes a setting.
func (db *DB) DeleteSetting(ctx context.Context, category, key string) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM settings WHERE category = ? AND key = ?`, category, key)
	return err
}

// HasSettings checks if any settings exist (for init detection).
func (db *DB) HasSettings(ctx context.Context) (bool, error) {
	var count int
	err := db.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// --- Project Webhook operations ---

// ProjectWebhook represents a project-specific webhook configuration.
type ProjectWebhook struct {
	ID              int64
	ProjectID       int64
	Provider        string
	SecretEncrypted []byte
	Enabled         bool
	RequireSecret   bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

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
	return err
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
	return err
}

// --- Scheduled Deployment operations ---

// ScheduledDeployment represents a deployment scheduled for future execution.
type ScheduledDeployment struct {
	ID          string
	Project     string
	Target      string
	Branch      string
	ScheduledAt time.Time
	ScheduledBy string
	Status      string
}

// CreateScheduledDeployment creates a scheduled deployment.
func (db *DB) CreateScheduledDeployment(ctx context.Context, id, project, target, branch string, scheduledAt time.Time, scheduledBy string) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO deployments (id, project, target, branch, status, scheduled_at, scheduled_by, triggered_by)
		VALUES (?, ?, ?, ?, 'scheduled', ?, ?, ?)
	`, id, project, target, branch, scheduledAt, scheduledBy, scheduledBy)
	return err
}

// ListPendingScheduledDeployments returns deployments that are due to run.
func (db *DB) ListPendingScheduledDeployments(ctx context.Context) ([]*ScheduledDeployment, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project, target, branch, scheduled_at, scheduled_by, status
		FROM deployments 
		WHERE scheduled_at IS NOT NULL 
		  AND scheduled_at <= datetime('now') 
		  AND status = 'scheduled'
		ORDER BY scheduled_at
	`)
	if err != nil {
		return nil, fmt.Errorf("querying scheduled deployments: %w", err)
	}
	defer rows.Close()

	var deployments []*ScheduledDeployment
	for rows.Next() {
		var d ScheduledDeployment
		if err := rows.Scan(&d.ID, &d.Project, &d.Target, &d.Branch, &d.ScheduledAt, &d.ScheduledBy, &d.Status); err != nil {
			return nil, fmt.Errorf("scanning deployment: %w", err)
		}
		deployments = append(deployments, &d)
	}
	return deployments, rows.Err()
}

// CancelScheduledDeployment cancels a scheduled deployment.
func (db *DB) CancelScheduledDeployment(ctx context.Context, id string) error {
	result, err := db.conn.ExecContext(ctx, `
		UPDATE deployments SET status = 'cancelled', completed_at = datetime('now')
		WHERE id = ? AND status = 'scheduled'
	`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		db.logger.Warn("failed to get RowsAffected for cancel deployment", zap.Error(err))
	}
	if rows == 0 {
		return fmt.Errorf("deployment not found or not in scheduled status")
	}
	return nil
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
		UPDATE users SET email = ?, role = ?, password_hash = ?, updated_at = datetime('now')
		WHERE id = ?
	`, user.Email, user.Role, user.PasswordHash, user.ID)
	return err
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
	return err
}

// --- Additional Deployment operations ---

// ListDeploymentsRecent returns recent deployments.
func (db *DB) ListDeploymentsRecent(ctx context.Context, limit int) ([]*Deployment, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project, target, branch, commit_hash, status, release_number,
		       started_at, completed_at, triggered_by, trigger_source, error_message
		FROM deployments
		ORDER BY started_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying deployments: %w", err)
	}
	defer rows.Close()

	var deployments []*Deployment
	for rows.Next() {
		var d Deployment
		var completedAt sql.NullTime
		var releaseNumber sql.NullInt64
		var commitHash, triggeredBy, triggerSource, errorMessage sql.NullString

		if err := rows.Scan(
			&d.ID, &d.Project, &d.Target, &d.Branch, &commitHash, &d.Status, &releaseNumber,
			&d.StartedAt, &completedAt, &triggeredBy, &triggerSource, &errorMessage,
		); err != nil {
			return nil, fmt.Errorf("scanning deployment: %w", err)
		}

		if completedAt.Valid {
			d.CompletedAt = &completedAt.Time
		}
		if releaseNumber.Valid {
			d.ReleaseNumber = int(releaseNumber.Int64)
		}
		d.CommitHash = commitHash.String
		d.TriggeredBy = triggeredBy.String
		d.TriggerSource = triggerSource.String
		d.ErrorMessage = errorMessage.String
		deployments = append(deployments, &d)
	}
	return deployments, rows.Err()
}

// --- Additional Project operations ---

// UpdateProjectByName updates a project by name.
func (db *DB) UpdateProjectByName(ctx context.Context, p *Project) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE projects SET repository = ?, branch = ?, deploy_path = ?, type = ?
		WHERE name = ?
	`, p.Repository, p.Branch, p.DeployPath, p.Type, p.Name)
	return err
}

// --- Cleanup operations ---

// CleanupExpiredSessions removes sessions that expired before the cutoff time.
func (db *DB) CleanupExpiredSessions(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM sessions WHERE expires_at < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CleanupOldDeployments removes completed deployment records older than the cutoff.
func (db *DB) CleanupOldDeployments(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM deployments 
		WHERE completed_at IS NOT NULL 
		  AND completed_at < ?
		  AND status IN ('success', 'failed', 'cancelled')
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CleanupOldDeploymentLogs removes deployment logs older than the cutoff.
func (db *DB) CleanupOldDeploymentLogs(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM deployment_logs 
		WHERE created_at < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CleanupOldAuditLogs removes audit log entries older than the cutoff.
func (db *DB) CleanupOldAuditLogs(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM audit_log WHERE timestamp < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// MarkStaleAgents marks agents that haven't been seen since the cutoff as disconnected.
func (db *DB) MarkStaleAgents(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		UPDATE agents SET status = 'disconnected'
		WHERE status = 'connected' AND last_seen_at < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CleanupExpiredAPIKeys removes API keys that have expired before now.
func (db *DB) CleanupExpiredAPIKeys(ctx context.Context, now time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM api_keys 
		WHERE expires_at IS NOT NULL AND expires_at < ?
	`, now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CleanupOrphanedWebhooks removes webhook configs for projects that no longer exist.
func (db *DB) CleanupOrphanedWebhooks(ctx context.Context) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM project_webhooks 
		WHERE project_id NOT IN (SELECT id FROM projects)
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- SSH Host Key operations ---

// SSHHostKey represents a stored SSH host key.
type SSHHostKey struct {
	ID          int64
	Hostname    string
	Port        int
	KeyType     string
	PublicKey   string // Base64 encoded public key
	Fingerprint string // SHA256 fingerprint
	Trusted     bool
	AddedBy     string
	VerifiedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

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
