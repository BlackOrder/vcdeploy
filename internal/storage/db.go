// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection.
type DB struct {
	conn *sql.DB
	path string
}

// New creates a new database connection.
func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Test connection
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	db := &DB{conn: conn, path: path}

	// Initialize schema
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	// Run additional migrations
	if err := db.AdditionalMigrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("additional migration: %w", err)
	}

	return db, nil
}

// Open is an alias for New
func Open(path string) (*DB, error) {
	return New(path)
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// migrate creates or updates the database schema.
func (db *DB) migrate() error {
	schema := `
	-- Users table
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		email TEXT,
		role TEXT NOT NULL DEFAULT 'viewer',
		totp_secret TEXT,
		totp_enabled INTEGER DEFAULT 0,
		must_change_password INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Sessions table
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		ip_address TEXT,
		user_agent TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	-- API keys table
	CREATE TABLE IF NOT EXISTS api_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		key_hash TEXT UNIQUE NOT NULL,
		scopes TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_used_at DATETIME,
		expires_at DATETIME,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	-- Agents table
	CREATE TABLE IF NOT EXISTS agents (
		id TEXT PRIMARY KEY,
		hostname TEXT,
		labels TEXT,
		capabilities TEXT,
		status TEXT DEFAULT 'offline',
		last_seen_at DATETIME,
		registered_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		certificate TEXT
	);

	-- Secrets table
	CREATE TABLE IF NOT EXISTS secrets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project TEXT NOT NULL,
		scope TEXT NOT NULL,
		key TEXT NOT NULL,
		value_encrypted BLOB NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(project, scope, key)
	);

	-- Deployments table
	CREATE TABLE IF NOT EXISTS deployments (
		id TEXT PRIMARY KEY,
		project TEXT NOT NULL,
		target TEXT NOT NULL,
		branch TEXT NOT NULL,
		commit_hash TEXT,
		status TEXT NOT NULL,
		release_number INTEGER,
		started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		completed_at DATETIME,
		triggered_by TEXT,
		trigger_source TEXT,
		error_message TEXT
	);

	-- Deployment logs table
	CREATE TABLE IF NOT EXISTS deployment_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		deployment_id TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		level TEXT NOT NULL,
		message TEXT NOT NULL,
		source TEXT,
		FOREIGN KEY (deployment_id) REFERENCES deployments(id) ON DELETE CASCADE
	);

	-- Audit logs table
	CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		source TEXT NOT NULL,
		user TEXT NOT NULL,
		action TEXT NOT NULL,
		resource TEXT,
		details TEXT,
		ip_address TEXT,
		result TEXT NOT NULL
	);

	-- Webhook secrets table
	CREATE TABLE IF NOT EXISTS webhook_secrets (
		provider TEXT PRIMARY KEY,
		secret_encrypted BLOB NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Master key metadata table
	CREATE TABLE IF NOT EXISTS master_key_meta (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		key_id TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		rotated_at DATETIME,
		previous_key_id TEXT,
		previous_key_expires_at DATETIME
	);

	-- Create indexes
	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
	CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
	CREATE INDEX IF NOT EXISTS idx_secrets_project_scope ON secrets(project, scope);
	CREATE INDEX IF NOT EXISTS idx_deployments_project ON deployments(project);
	CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
	CREATE INDEX IF NOT EXISTS idx_deployments_started_at ON deployments(started_at);
	CREATE INDEX IF NOT EXISTS idx_deployment_logs_deployment_id ON deployment_logs(deployment_id);
	CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp);
	CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
	CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(user);
	`

	_, err := db.conn.Exec(schema)
	return err
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
		return nil, nil
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
		return nil, nil
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

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, project, target, branch, commit_hash, status, release_number,
		       started_at, completed_at, triggered_by, trigger_source, error_message
		FROM deployments WHERE id = ?
	`, id).Scan(
		&d.ID, &d.Project, &d.Target, &d.Branch, &d.CommitHash, &d.Status, &d.ReleaseNumber,
		&d.StartedAt, &completedAt, &d.TriggeredBy, &d.TriggerSource, &d.ErrorMessage,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying deployment: %w", err)
	}

	if completedAt.Valid {
		d.CompletedAt = &completedAt.Time
	}
	return &d, nil
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

// SetSecret sets a secret with plain string value (encrypts internally for CLI use).
func (db *DB) SetSecret(scope, key, value string) error {
	// In production, would encrypt the value
	encrypted := []byte(value)
	_, err := db.conn.Exec(`
		INSERT INTO secrets (project, scope, key, value_encrypted)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(project, scope, key) DO UPDATE SET
			value_encrypted = excluded.value_encrypted,
			updated_at = CURRENT_TIMESTAMP
	`, scope, scope, key, encrypted)
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
		return nil, nil
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

// --- Helper functions ---

func mapToJSON(m map[string]string) string {
	if m == nil {
		return "{}"
	}
	// Simple JSON encoding without external dependency
	result := "{"
	first := true
	for k, v := range m {
		if !first {
			result += ","
		}
		result += fmt.Sprintf(`"%s":"%s"`, k, v)
		first = false
	}
	result += "}"
	return result
}

func jsonToMap(s string) map[string]string {
	// Simple implementation - in production use encoding/json
	return make(map[string]string)
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

	id, _ := result.LastInsertId()
	project.ID = id
	return nil
}

// GetProject retrieves a project by name.
func (db *DB) GetProject(name string) (*Project, error) {
	var p Project
	var lastDeploy sql.NullTime

	err := db.conn.QueryRow(`
		SELECT id, name, repository, branch, deploy_path, type, created_at, last_deploy_at, last_deploy_status
		FROM projects WHERE name = ?
	`, name).Scan(&p.ID, &p.Name, &p.Repository, &p.Branch, &p.DeployPath, &p.Type, &p.CreatedAt, &lastDeploy, &p.LastDeployStatus)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("query project: %w", err)
	}

	if lastDeploy.Valid {
		p.LastDeployAt = &lastDeploy.Time
	}
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

		if err := rows.Scan(&p.ID, &p.Name, &p.Repository, &p.Branch, &p.DeployPath, &p.Type, &p.CreatedAt, &lastDeploy, &p.LastDeployStatus); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		if lastDeploy.Valid {
			p.LastDeployAt = &lastDeploy.Time
		}
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

	id, _ := result.LastInsertId()
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

// --- Extended Deployment operations ---

// DeploymentExt is an extended deployment struct for CLI
type DeploymentExt struct {
	ID          string
	ProjectID   int64
	ProjectName string
	Target      string
	Status      string
	TriggeredBy string
	StartedAt   time.Time
	FinishedAt  *time.Time
}

// CreateDeploymentExt creates a deployment (extended version for CLI)
func (db *DB) CreateDeploymentExt(d *DeploymentExt) error {
	_, err := db.conn.Exec(`
		INSERT INTO deployments (id, project, target, branch, status, triggered_by, started_at)
		VALUES (?, ?, ?, '', ?, ?, ?)
	`, d.ID, d.ProjectName, d.Target, d.Status, d.TriggeredBy, d.StartedAt)
	return err
}

// UpdateDeploymentExt updates a deployment (extended version for CLI)
func (db *DB) UpdateDeploymentExt(d *DeploymentExt) error {
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

// Ensure we have the projects and project_types tables
func init() {
	// Additional schema for projects (will be added to migrate())
}

// AdditionalMigration adds additional tables
func (db *DB) AdditionalMigrate() error {
	additionalSchema := `
	-- Projects table
	CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		repository TEXT,
		branch TEXT DEFAULT 'main',
		deploy_path TEXT,
		type TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_deploy_at DATETIME,
		last_deploy_status TEXT
	);

	-- Project types table
	CREATE TABLE IF NOT EXISTS project_types (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		build_cmd TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_projects_name ON projects(name);
	CREATE INDEX IF NOT EXISTS idx_projects_type ON projects(type);
	`
	_, err := db.conn.Exec(additionalSchema)
	return err
}
