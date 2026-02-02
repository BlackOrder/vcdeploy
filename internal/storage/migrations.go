// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Migration represents a database migration.
type Migration struct {
	Version     int
	Description string
	Up          func(tx *sql.Tx) error
	Down        func(tx *sql.Tx) error
}

// MigrationRecord represents a record in the schema_migrations table.
type MigrationRecord struct {
	Version   int
	AppliedAt time.Time
}

// migrations is the ordered list of all migrations.
// New migrations must be appended with incrementing version numbers.
var migrations = []Migration{
	{
		Version:     1,
		Description: "Initial schema - users, sessions, api_keys",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
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
					key_prefix TEXT,
					scopes TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					last_used_at DATETIME,
					expires_at DATETIME,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				);

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
				CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
				CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS api_keys;
				DROP TABLE IF EXISTS sessions;
				DROP TABLE IF EXISTS users;
			`)
			return err
		},
	},
	{
		Version:     2,
		Description: "Agents and secrets tables",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
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
					project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
					scope TEXT NOT NULL,
					key TEXT NOT NULL,
					value_encrypted BLOB NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					UNIQUE(project, scope, key)
				);

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_secrets_project_scope ON secrets(project, scope);
				CREATE INDEX IF NOT EXISTS idx_secrets_project_id ON secrets(project_id);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS secrets;
				DROP TABLE IF EXISTS agents;
			`)
			return err
		},
	},
	{
		Version:     3,
		Description: "Deployments and deployment logs",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				-- Deployments table
				CREATE TABLE IF NOT EXISTS deployments (
					id TEXT PRIMARY KEY,
					project TEXT NOT NULL,
					project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
					target TEXT NOT NULL,
					branch TEXT NOT NULL,
					commit_hash TEXT,
					status TEXT NOT NULL,
					release_number INTEGER,
					started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					completed_at DATETIME,
					triggered_by TEXT,
					trigger_source TEXT,
					error_message TEXT,
					scheduled_at DATETIME,
					scheduled_by TEXT
				);

				-- Deployment logs table
				CREATE TABLE IF NOT EXISTS deployment_logs (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					deployment_id TEXT NOT NULL,
					level TEXT NOT NULL,
					message TEXT NOT NULL,
					source TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (deployment_id) REFERENCES deployments(id) ON DELETE CASCADE
				);

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_deployments_project ON deployments(project);
				CREATE INDEX IF NOT EXISTS idx_deployments_project_id ON deployments(project_id);
				CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
				CREATE INDEX IF NOT EXISTS idx_deployments_started_at ON deployments(started_at);
				CREATE INDEX IF NOT EXISTS idx_deployments_triggered_by ON deployments(triggered_by);
				CREATE INDEX IF NOT EXISTS idx_deployments_scheduled ON deployments(scheduled_at) WHERE scheduled_at IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_deployment_logs_deployment_id ON deployment_logs(deployment_id);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS deployment_logs;
				DROP TABLE IF EXISTS deployments;
			`)
			return err
		},
	},
	{
		Version:     4,
		Description: "Audit logs and webhook secrets",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				-- Audit logs table
				CREATE TABLE IF NOT EXISTS audit_logs (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
					source TEXT NOT NULL,
					user TEXT NOT NULL,
					action TEXT NOT NULL,
					resource TEXT,
					resource_id TEXT,
					resource_data TEXT,
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

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp);
				CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
				CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(user);
				CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource, resource_id);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS master_key_meta;
				DROP TABLE IF EXISTS webhook_secrets;
				DROP TABLE IF EXISTS audit_logs;
			`)
			return err
		},
	},
	{
		Version:     5,
		Description: "Projects and project types",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
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

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_projects_name ON projects(name);
				CREATE INDEX IF NOT EXISTS idx_projects_type ON projects(type);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS project_types;
				DROP TABLE IF EXISTS projects;
			`)
			return err
		},
	},
	{
		Version:     6,
		Description: "SSH keys and known hosts for agent provisioning",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				-- SSH keys table - Ed25519 keys stored in database
				CREATE TABLE IF NOT EXISTS ssh_keys (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT UNIQUE NOT NULL,
					public_key TEXT NOT NULL,
					private_key_encrypted BLOB NOT NULL,
					key_type TEXT NOT NULL DEFAULT 'ed25519',
					fingerprint TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					last_used_at DATETIME
				);

				-- Known hosts table - SSH host key verification
				CREATE TABLE IF NOT EXISTS known_hosts (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					hostname TEXT NOT NULL,
					port INTEGER NOT NULL DEFAULT 22,
					key_type TEXT NOT NULL,
					public_key TEXT NOT NULL,
					fingerprint TEXT NOT NULL,
					added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					last_verified_at DATETIME,
					UNIQUE(hostname, port, key_type)
				);

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_ssh_keys_name ON ssh_keys(name);
				CREATE INDEX IF NOT EXISTS idx_ssh_keys_fingerprint ON ssh_keys(fingerprint);
				CREATE INDEX IF NOT EXISTS idx_known_hosts_hostname ON known_hosts(hostname, port);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS known_hosts;
				DROP TABLE IF EXISTS ssh_keys;
			`)
			return err
		},
	},
	{
		Version:     7,
		Description: "KMS-style encryption keys with versioning",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				-- Encryption keys table - KMS-style key management
				-- Keys are NEVER deleted, only marked as inactive for decryption
				CREATE TABLE IF NOT EXISTS encryption_keys (
					id TEXT PRIMARY KEY,
					version INTEGER NOT NULL,
					key_material_encrypted BLOB NOT NULL,
					algorithm TEXT NOT NULL DEFAULT 'AES-256-GCM',
					status TEXT NOT NULL DEFAULT 'active',
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					activated_at DATETIME,
					deactivated_at DATETIME,
					scheduled_deletion_at DATETIME,
					deletion_cancelled_at DATETIME,
					UNIQUE(version)
				);

				-- Key usage audit
				CREATE TABLE IF NOT EXISTS encryption_key_usage (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					key_id TEXT NOT NULL,
					operation TEXT NOT NULL,
					resource_type TEXT,
					resource_id TEXT,
					timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (key_id) REFERENCES encryption_keys(id)
				);

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_encryption_keys_status ON encryption_keys(status);
				CREATE INDEX IF NOT EXISTS idx_encryption_keys_version ON encryption_keys(version);
				CREATE INDEX IF NOT EXISTS idx_encryption_key_usage_key_id ON encryption_key_usage(key_id);
				CREATE INDEX IF NOT EXISTS idx_encryption_key_usage_timestamp ON encryption_key_usage(timestamp);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS encryption_key_usage;
				DROP TABLE IF EXISTS encryption_keys;
			`)
			return err
		},
	},
	{
		Version:     8,
		Description: "Multi-CA trust system for agent mTLS",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				-- Certificate authorities table - Multi-CA trust
				-- Old CAs are NEVER deleted, retained forever for backward compatibility
				CREATE TABLE IF NOT EXISTS certificate_authorities (
					id TEXT PRIMARY KEY,
					version INTEGER NOT NULL,
					common_name TEXT NOT NULL,
					certificate_pem TEXT NOT NULL,
					private_key_encrypted BLOB NOT NULL,
					not_before DATETIME NOT NULL,
					not_after DATETIME NOT NULL,
					status TEXT NOT NULL DEFAULT 'active',
					is_current INTEGER NOT NULL DEFAULT 0,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					rotated_at DATETIME,
					UNIQUE(version)
				);

				-- Agent certificates table
				CREATE TABLE IF NOT EXISTS agent_certificates (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					agent_id TEXT NOT NULL,
					ca_id TEXT NOT NULL,
					serial_number TEXT UNIQUE NOT NULL,
					certificate_pem TEXT NOT NULL,
					not_before DATETIME NOT NULL,
					not_after DATETIME NOT NULL,
					status TEXT NOT NULL DEFAULT 'active',
					issued_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					renewed_at DATETIME,
					revoked_at DATETIME,
					revocation_reason TEXT,
					FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE,
					FOREIGN KEY (ca_id) REFERENCES certificate_authorities(id)
				);

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_ca_status ON certificate_authorities(status);
				CREATE INDEX IF NOT EXISTS idx_ca_is_current ON certificate_authorities(is_current);
				CREATE INDEX IF NOT EXISTS idx_agent_certs_agent_id ON agent_certificates(agent_id);
				CREATE INDEX IF NOT EXISTS idx_agent_certs_ca_id ON agent_certificates(ca_id);
				CREATE INDEX IF NOT EXISTS idx_agent_certs_status ON agent_certificates(status);
				CREATE INDEX IF NOT EXISTS idx_agent_certs_not_after ON agent_certificates(not_after);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS agent_certificates;
				DROP TABLE IF EXISTS certificate_authorities;
			`)
			return err
		},
	},
	{
		Version:     9,
		Description: "Rate limiting and IP blocking",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				-- Rate limits table - track request counts
				CREATE TABLE IF NOT EXISTS rate_limits (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					key TEXT NOT NULL,
					bucket TEXT NOT NULL,
					count INTEGER NOT NULL DEFAULT 1,
					window_start DATETIME NOT NULL,
					window_end DATETIME NOT NULL,
					UNIQUE(key, bucket, window_start)
				);

				-- Blocked IPs table
				CREATE TABLE IF NOT EXISTS blocked_ips (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					ip_address TEXT UNIQUE NOT NULL,
					reason TEXT NOT NULL,
					blocked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					expires_at DATETIME,
					blocked_by TEXT
				);

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_rate_limits_key_bucket ON rate_limits(key, bucket);
				CREATE INDEX IF NOT EXISTS idx_rate_limits_window ON rate_limits(window_end);
				CREATE INDEX IF NOT EXISTS idx_blocked_ips_expires ON blocked_ips(expires_at);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS blocked_ips;
				DROP TABLE IF EXISTS rate_limits;
			`)
			return err
		},
	},
	{
		Version:     10,
		Description: "Agent provisioning jobs and binary management",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				-- Agent binaries table - track available agent versions
				CREATE TABLE IF NOT EXISTS agent_binaries (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					version TEXT NOT NULL,
					os TEXT NOT NULL,
					arch TEXT NOT NULL,
					path TEXT NOT NULL,
					checksum_sha256 TEXT NOT NULL,
					size_bytes INTEGER NOT NULL,
					uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					is_current INTEGER NOT NULL DEFAULT 0,
					UNIQUE(version, os, arch)
				);

				-- Agent provision jobs table - track provisioning with rollback support
				CREATE TABLE IF NOT EXISTS agent_provision_jobs (
					id TEXT PRIMARY KEY,
					target_host TEXT NOT NULL,
					target_port INTEGER NOT NULL DEFAULT 22,
					target_user TEXT NOT NULL,
					ssh_key_id INTEGER,
					agent_binary_id INTEGER,
					status TEXT NOT NULL DEFAULT 'pending',
					stage TEXT,
					progress INTEGER DEFAULT 0,
					error_message TEXT,
					rollback_data TEXT,
					started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					completed_at DATETIME,
					FOREIGN KEY (ssh_key_id) REFERENCES ssh_keys(id),
					FOREIGN KEY (agent_binary_id) REFERENCES agent_binaries(id)
				);

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_agent_binaries_current ON agent_binaries(is_current);
				CREATE INDEX IF NOT EXISTS idx_agent_binaries_version ON agent_binaries(version, os, arch);
				CREATE INDEX IF NOT EXISTS idx_provision_jobs_status ON agent_provision_jobs(status);
				CREATE INDEX IF NOT EXISTS idx_provision_jobs_target ON agent_provision_jobs(target_host);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS agent_provision_jobs;
				DROP TABLE IF EXISTS agent_binaries;
			`)
			return err
		},
	},
	{
		Version:     11,
		Description: "ACME/Let's Encrypt certificate storage",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				-- ACME certificates table - Let's Encrypt cert storage
				CREATE TABLE IF NOT EXISTS acme_certificates (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					domain TEXT UNIQUE NOT NULL,
					certificate_pem TEXT NOT NULL,
					private_key_encrypted BLOB NOT NULL,
					issuer TEXT,
					not_before DATETIME NOT NULL,
					not_after DATETIME NOT NULL,
					last_renewal DATETIME,
					auto_renew INTEGER NOT NULL DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				-- ACME account table
				CREATE TABLE IF NOT EXISTS acme_accounts (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					email TEXT NOT NULL,
					account_url TEXT,
					private_key_encrypted BLOB NOT NULL,
					directory_url TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_acme_certs_domain ON acme_certificates(domain);
				CREATE INDEX IF NOT EXISTS idx_acme_certs_expiry ON acme_certificates(not_after);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS acme_accounts;
				DROP TABLE IF EXISTS acme_certificates;
			`)
			return err
		},
	},
	{
		Version:     12,
		Description: "Settings storage and project webhooks",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				-- Settings table - stores all configuration as key/value pairs
				CREATE TABLE IF NOT EXISTS settings (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					category TEXT NOT NULL,
					key TEXT NOT NULL,
					value TEXT NOT NULL,
					value_type TEXT NOT NULL DEFAULT 'string',
					encrypted INTEGER DEFAULT 0,
					description TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					UNIQUE(category, key)
				);

				-- SSH Jump servers table
				CREATE TABLE IF NOT EXISTS ssh_jump_servers (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT UNIQUE NOT NULL,
					host TEXT NOT NULL,
					port INTEGER DEFAULT 22,
					username TEXT NOT NULL,
					ssh_key_id INTEGER,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (ssh_key_id) REFERENCES ssh_keys(id)
				);

				-- Project webhooks table - per-project webhook configuration
				CREATE TABLE IF NOT EXISTS project_webhooks (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					project_id INTEGER NOT NULL,
					provider TEXT NOT NULL,
					secret_encrypted BLOB,
					enabled INTEGER DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
					UNIQUE(project_id, provider)
				);

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_settings_category ON settings(category);
				CREATE INDEX IF NOT EXISTS idx_project_webhooks_project ON project_webhooks(project_id);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS project_webhooks;
				DROP TABLE IF EXISTS ssh_jump_servers;
				DROP TABLE IF EXISTS settings;
			`)
			return err
		},
	},
	{
		Version:     13,
		Description: "Add require_secret column to project_webhooks",
		Up: func(tx *sql.Tx) error {
			// Check if column already exists (SQLite doesn't have IF NOT EXISTS for columns)
			var count int
			err := tx.QueryRow(`
				SELECT COUNT(*) FROM pragma_table_info('project_webhooks') WHERE name='require_secret'
			`).Scan(&count)
			if err != nil {
				return err
			}
			if count > 0 {
				// Column already exists
				return nil
			}
			_, err = tx.Exec(`
				ALTER TABLE project_webhooks ADD COLUMN require_secret INTEGER DEFAULT 0
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			// SQLite doesn't support DROP COLUMN, we'd need to recreate the table
			// For simplicity, this is a no-op since the column is optional
			return nil
		},
	},
	{
		Version:     14,
		Description: "SSH host keys table for secure host key verification",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				-- SSH host keys table for database-backed host key verification
				CREATE TABLE IF NOT EXISTS ssh_host_keys (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					hostname TEXT NOT NULL,
					port INTEGER NOT NULL DEFAULT 22,
					key_type TEXT NOT NULL,
					public_key TEXT NOT NULL,
					fingerprint TEXT NOT NULL,
					trusted INTEGER NOT NULL DEFAULT 0,
					added_by TEXT,
					verified_at DATETIME,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					UNIQUE(hostname, port, key_type)
				);

				-- Index for quick lookups
				CREATE INDEX IF NOT EXISTS idx_ssh_host_keys_hostname_port ON ssh_host_keys(hostname, port);
				CREATE INDEX IF NOT EXISTS idx_ssh_host_keys_fingerprint ON ssh_host_keys(fingerprint);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE IF EXISTS ssh_host_keys`)
			return err
		},
	},
	{
		Version:     15,
		Description: "Agent self-update configuration and version tracking",
		Up: func(tx *sql.Tx) error {
			// Add update configuration columns to agents table one at a time
			// SQLite requires separate statements for each ALTER TABLE
			alterStatements := []string{
				`ALTER TABLE agents ADD COLUMN version TEXT DEFAULT ''`,
				`ALTER TABLE agents ADD COLUMN os TEXT DEFAULT ''`,
				`ALTER TABLE agents ADD COLUMN arch TEXT DEFAULT ''`,
				`ALTER TABLE agents ADD COLUMN update_policy TEXT DEFAULT 'immediate'`,
				`ALTER TABLE agents ADD COLUMN update_window_start TEXT DEFAULT ''`,
				`ALTER TABLE agents ADD COLUMN update_window_end TEXT DEFAULT ''`,
				`ALTER TABLE agents ADD COLUMN last_update_at DATETIME`,
				`ALTER TABLE agents ADD COLUMN last_update_error TEXT DEFAULT ''`,
			}

			for _, stmt := range alterStatements {
				// Check if column already exists to make migration idempotent
				// We'll just try to add it and ignore "duplicate column name" errors
				_, err := tx.Exec(stmt)
				if err != nil {
					// SQLite error for duplicate column contains "duplicate column name"
					if !strings.Contains(err.Error(), "duplicate column name") {
						return fmt.Errorf("executing %s: %w", stmt, err)
					}
				}
			}

			// Agent update history table
			_, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS agent_update_history (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					agent_id TEXT NOT NULL,
					from_version TEXT NOT NULL,
					to_version TEXT NOT NULL,
					status TEXT NOT NULL,
					error_message TEXT DEFAULT '',
					started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					completed_at DATETIME,
					rolled_back INTEGER DEFAULT 0,
					FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
				)
			`)
			if err != nil {
				return fmt.Errorf("creating agent_update_history table: %w", err)
			}

			// Indexes
			_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_update_history_agent ON agent_update_history(agent_id)`)
			if err != nil {
				return fmt.Errorf("creating agent index: %w", err)
			}

			_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_update_history_status ON agent_update_history(status)`)
			if err != nil {
				return fmt.Errorf("creating status index: %w", err)
			}

			return nil
		},
		Down: func(tx *sql.Tx) error {
			// SQLite doesn't support DROP COLUMN, so we just drop the new table
			_, err := tx.Exec(`DROP TABLE IF EXISTS agent_update_history`)
			return err
		},
	},
	{
		Version:     16,
		Description: "Health check configuration and deployment rollback tracking",
		Up: func(tx *sql.Tx) error {
			// Health check configurations - both global and per-project
			_, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS health_check_configs (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					project_id INTEGER,
					name TEXT NOT NULL,
					url TEXT NOT NULL,
					method TEXT NOT NULL DEFAULT 'GET',
					expected_status INTEGER DEFAULT 200,
					timeout_seconds INTEGER DEFAULT 10,
					retries INTEGER DEFAULT 3,
					retry_delay_seconds INTEGER DEFAULT 5,
					headers TEXT,
					body TEXT,
					body_contains TEXT,
					enabled INTEGER DEFAULT 1,
					is_global INTEGER DEFAULT 0,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
				)
			`)
			if err != nil {
				return fmt.Errorf("creating health_check_configs table: %w", err)
			}

			// Add health check reference to projects
			alterStatements := []string{
				`ALTER TABLE projects ADD COLUMN health_check_id INTEGER REFERENCES health_check_configs(id)`,
				`ALTER TABLE projects ADD COLUMN auto_rollback_enabled INTEGER DEFAULT 1`,
				`ALTER TABLE projects ADD COLUMN rollback_on_health_fail INTEGER DEFAULT 1`,
			}

			for _, stmt := range alterStatements {
				_, err := tx.Exec(stmt)
				if err != nil {
					if !strings.Contains(err.Error(), "duplicate column name") {
						return fmt.Errorf("executing %s: %w", stmt, err)
					}
				}
			}

			// Deployment rollbacks table to track rollback events
			_, err = tx.Exec(`
				CREATE TABLE IF NOT EXISTS deployment_rollbacks (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					deployment_id TEXT NOT NULL,
					project_name TEXT NOT NULL,
					from_release INTEGER NOT NULL,
					to_release INTEGER NOT NULL,
					reason TEXT NOT NULL,
					triggered_by TEXT NOT NULL,
					health_check_failed INTEGER DEFAULT 0,
					health_check_error TEXT,
					status TEXT NOT NULL DEFAULT 'pending',
					error_message TEXT,
					started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					completed_at DATETIME,
					FOREIGN KEY (deployment_id) REFERENCES deployments(id) ON DELETE CASCADE
				)
			`)
			if err != nil {
				return fmt.Errorf("creating deployment_rollbacks table: %w", err)
			}

			// Indexes
			_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_health_check_configs_project ON health_check_configs(project_id)`)
			if err != nil {
				return fmt.Errorf("creating health check index: %w", err)
			}

			_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_health_check_configs_global ON health_check_configs(is_global)`)
			if err != nil {
				return fmt.Errorf("creating global health check index: %w", err)
			}

			_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_deployment_rollbacks_deployment ON deployment_rollbacks(deployment_id)`)
			if err != nil {
				return fmt.Errorf("creating deployment rollbacks index: %w", err)
			}

			_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_deployment_rollbacks_project ON deployment_rollbacks(project_name)`)
			if err != nil {
				return fmt.Errorf("creating project rollbacks index: %w", err)
			}

			// Insert default global health check configuration
			_, err = tx.Exec(`
				INSERT OR IGNORE INTO health_check_configs (
					name, url, method, expected_status, timeout_seconds, retries, 
					retry_delay_seconds, enabled, is_global
				) VALUES (
					'Global Default', '{{.URL}}', 'GET', 200, 10, 3, 5, 1, 1
				)
			`)
			if err != nil {
				return fmt.Errorf("inserting default global health check: %w", err)
			}

			return nil
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				DROP TABLE IF EXISTS deployment_rollbacks;
				DROP TABLE IF EXISTS health_check_configs;
			`)
			return err
		},
	},
	{
		Version:     17,
		Description: "Security tables for certificate authorities, encryption keys, and SSH keys",
		Up: func(tx *sql.Tx) error {
			// Certificate authorities table
			_, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS certificate_authorities (
					id TEXT PRIMARY KEY,
					version INTEGER NOT NULL,
					common_name TEXT NOT NULL,
					certificate_pem TEXT NOT NULL,
					private_key_encrypted BLOB NOT NULL,
					not_before DATETIME NOT NULL,
					not_after DATETIME NOT NULL,
					status TEXT NOT NULL DEFAULT 'active',
					is_current INTEGER NOT NULL DEFAULT 0,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					rotated_at DATETIME,
					UNIQUE(version)
				)
			`)
			if err != nil {
				return fmt.Errorf("creating certificate_authorities table: %w", err)
			}

			// Agent certificates table
			_, err = tx.Exec(`
				CREATE TABLE IF NOT EXISTS agent_certificates (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					agent_id TEXT NOT NULL,
					ca_id TEXT NOT NULL,
					serial_number TEXT UNIQUE NOT NULL,
					certificate_pem TEXT NOT NULL,
					not_before DATETIME NOT NULL,
					not_after DATETIME NOT NULL,
					status TEXT NOT NULL DEFAULT 'active',
					issued_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					renewed_at DATETIME,
					revoked_at DATETIME,
					revocation_reason TEXT,
					FOREIGN KEY (ca_id) REFERENCES certificate_authorities(id)
				)
			`)
			if err != nil {
				return fmt.Errorf("creating agent_certificates table: %w", err)
			}

			// Server certificates table (for master server TLS)
			_, err = tx.Exec(`
				CREATE TABLE IF NOT EXISTS server_certificates (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					hostname TEXT NOT NULL,
					certificate_pem TEXT NOT NULL,
					private_key_encrypted BLOB NOT NULL,
					sans TEXT,
					not_before DATETIME NOT NULL,
					not_after DATETIME NOT NULL,
					ca_id TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (ca_id) REFERENCES certificate_authorities(id)
				)
			`)
			if err != nil {
				return fmt.Errorf("creating server_certificates table: %w", err)
			}

			// Registration tokens for secure agent registration
			_, err = tx.Exec(`
				CREATE TABLE IF NOT EXISTS registration_tokens (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					token TEXT UNIQUE NOT NULL,
					agent_id TEXT,
					expires_at DATETIME,
					used_at DATETIME,
					created_by TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`)
			if err != nil {
				return fmt.Errorf("creating registration_tokens table: %w", err)
			}

			// Source credentials (git repo credentials)
			_, err = tx.Exec(`
				CREATE TABLE IF NOT EXISTS source_credentials (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT UNIQUE NOT NULL,
					type TEXT NOT NULL,
					url_pattern TEXT NOT NULL,
					credential_encrypted BLOB,
					created_by TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`)
			if err != nil {
				return fmt.Errorf("creating source_credentials table: %w", err)
			}

			// Revoked certificates tracking
			_, err = tx.Exec(`
				CREATE TABLE IF NOT EXISTS revoked_certificates (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					serial_number TEXT UNIQUE NOT NULL,
					agent_id TEXT,
					reason TEXT,
					revoked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					revoked_by TEXT NOT NULL
				)
			`)
			if err != nil {
				return fmt.Errorf("creating revoked_certificates table: %w", err)
			}

			// Encryption keys for KMS
			_, err = tx.Exec(`
				CREATE TABLE IF NOT EXISTS encryption_keys (
					id TEXT PRIMARY KEY,
					version INTEGER NOT NULL,
					key_material_encrypted BLOB NOT NULL,
					algorithm TEXT NOT NULL DEFAULT 'AES-256-GCM',
					status TEXT NOT NULL DEFAULT 'active',
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					activated_at DATETIME,
					deactivated_at DATETIME,
					scheduled_deletion_at DATETIME,
					deletion_cancelled_at DATETIME,
					UNIQUE(version)
				)
			`)
			if err != nil {
				return fmt.Errorf("creating encryption_keys table: %w", err)
			}

			// Encryption key usage logs
			_, err = tx.Exec(`
				CREATE TABLE IF NOT EXISTS encryption_key_usage (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					key_id TEXT NOT NULL,
					operation TEXT NOT NULL,
					resource_type TEXT,
					resource_id TEXT,
					timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`)
			if err != nil {
				return fmt.Errorf("creating encryption_key_usage table: %w", err)
			}

			// Add created_by column to ssh_keys table (created in migration 6)
			// This column tracks who created the key for audit purposes
			_, err = tx.Exec(`ALTER TABLE ssh_keys ADD COLUMN created_by TEXT DEFAULT 'system'`)
			if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
				return fmt.Errorf("adding created_by to ssh_keys: %w", err)
			}

			// Certificate audit events table
			_, err = tx.Exec(`
				CREATE TABLE IF NOT EXISTS cert_audit_events (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
					event_type TEXT NOT NULL,
					agent_id TEXT,
					hostname TEXT,
					serial TEXT NOT NULL,
					ca_id TEXT NOT NULL,
					reason TEXT,
					requested_by TEXT NOT NULL,
					client_ip TEXT
				)
			`)
			if err != nil {
				return fmt.Errorf("creating cert_audit_events table: %w", err)
			}

			// Create indexes for security tables
			// Note: ssh_keys and known_hosts indexes already exist from migration 6
			indexes := []string{
				`CREATE INDEX IF NOT EXISTS idx_agent_certificates_agent_id ON agent_certificates(agent_id)`,
				`CREATE INDEX IF NOT EXISTS idx_agent_certificates_ca_id ON agent_certificates(ca_id)`,
				`CREATE INDEX IF NOT EXISTS idx_agent_certificates_status ON agent_certificates(status)`,
				`CREATE INDEX IF NOT EXISTS idx_server_certificates_ca_id ON server_certificates(ca_id)`,
				`CREATE INDEX IF NOT EXISTS idx_server_certificates_hostname ON server_certificates(hostname)`,
				`CREATE INDEX IF NOT EXISTS idx_registration_tokens_expires ON registration_tokens(expires_at)`,
				`CREATE INDEX IF NOT EXISTS idx_revoked_certificates_serial ON revoked_certificates(serial_number)`,
				`CREATE INDEX IF NOT EXISTS idx_encryption_keys_status ON encryption_keys(status)`,
				`CREATE INDEX IF NOT EXISTS idx_encryption_keys_version ON encryption_keys(version)`,
				`CREATE INDEX IF NOT EXISTS idx_cert_audit_events_event ON cert_audit_events(event_type)`,
				`CREATE INDEX IF NOT EXISTS idx_cert_audit_events_created ON cert_audit_events(timestamp)`,
			}

			for _, idx := range indexes {
				if _, err := tx.Exec(idx); err != nil {
					return fmt.Errorf("creating index: %w", err)
				}
			}

			return nil
		},
		Down: func(tx *sql.Tx) error {
			// Note: ssh_keys and known_hosts were created in migration 6, not here
			// We only added created_by column to ssh_keys which can't be easily removed in SQLite
			tables := []string{
				"cert_audit_events",
				"encryption_key_usage",
				"encryption_keys",
				"revoked_certificates",
				"source_credentials",
				"registration_tokens",
				"server_certificates",
				"agent_certificates",
				"certificate_authorities",
			}
			for _, table := range tables {
				if _, err := tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		Version:     18,
		Description: "Add HMAC secret to agents table for re-authentication",
		Up: func(tx *sql.Tx) error {
			// Add hmac_secret column to agents table for HMAC-based re-authentication
			_, err := tx.Exec(`ALTER TABLE agents ADD COLUMN hmac_secret BLOB`)
			if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
				return fmt.Errorf("adding hmac_secret to agents: %w", err)
			}

			// Add reauth_address to server configuration (stored in settings)
			// This is handled by config, not database

			return nil
		},
		Down: func(tx *sql.Tx) error {
			// SQLite doesn't support DROP COLUMN directly
			// Would need to recreate table, but for greenfield this is fine
			return nil
		},
	},
	{
		Version:     19,
		Description: "Add provision_logs table for provisioning job logs",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS provision_logs (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					job_id TEXT NOT NULL,
					timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					level TEXT NOT NULL,
					message TEXT NOT NULL,
					FOREIGN KEY (job_id) REFERENCES agent_provision_jobs(id)
				);

				CREATE INDEX IF NOT EXISTS idx_provision_logs_job_id ON provision_logs(job_id);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE IF EXISTS provision_logs`)
			return err
		},
	},
	{
		Version:     20,
		Description: "Add recovery_codes table for TOTP recovery codes",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS recovery_codes (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					user_id INTEGER NOT NULL,
					code_hash TEXT NOT NULL,
					used_at DATETIME,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				);

				CREATE INDEX IF NOT EXISTS idx_recovery_codes_user_id ON recovery_codes(user_id);
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE IF EXISTS recovery_codes`)
			return err
		},
	},
}

// MigrateUp runs all pending migrations.
func (db *DB) MigrateUp(ctx context.Context) error {
	// Ensure schema_migrations table exists
	if err := db.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	// Get current version
	currentVersion, err := db.getCurrentVersion()
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	// Sort migrations by version
	sortedMigrations := make([]Migration, len(migrations))
	copy(sortedMigrations, migrations)
	sort.Slice(sortedMigrations, func(i, j int) bool {
		return sortedMigrations[i].Version < sortedMigrations[j].Version
	})

	// Apply pending migrations
	for _, m := range sortedMigrations {
		if m.Version <= currentVersion {
			continue
		}

		if err := db.applyMigration(ctx, m, true); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.Version, m.Description, err)
		}
	}

	return nil
}

// MigrateDown rolls back the last n migrations.
func (db *DB) MigrateDown(ctx context.Context, steps int) error {
	if steps <= 0 {
		return fmt.Errorf("steps must be positive")
	}

	// Ensure schema_migrations table exists
	if err := db.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	// Get applied migrations in reverse order
	applied, err := db.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("get applied migrations: %w", err)
	}

	// Sort in reverse order (newest first)
	sort.Slice(applied, func(i, j int) bool {
		return applied[i].Version > applied[j].Version
	})

	// Rollback requested number of steps
	count := 0
	for _, record := range applied {
		if count >= steps {
			break
		}

		// Find the migration
		var migration *Migration
		for i := range migrations {
			if migrations[i].Version == record.Version {
				migration = &migrations[i]
				break
			}
		}

		if migration == nil {
			return fmt.Errorf("migration version %d not found in code", record.Version)
		}

		if err := db.applyMigration(ctx, *migration, false); err != nil {
			return fmt.Errorf("rollback migration %d (%s): %w", migration.Version, migration.Description, err)
		}

		count++
	}

	return nil
}

// MigrateTo migrates to a specific version (up or down).
func (db *DB) MigrateTo(ctx context.Context, targetVersion int) error {
	// Ensure schema_migrations table exists
	if err := db.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	currentVersion, err := db.getCurrentVersion()
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	if targetVersion == currentVersion {
		return nil // Already at target
	}

	if targetVersion > currentVersion {
		// Migrate up
		for _, m := range migrations {
			if m.Version > currentVersion && m.Version <= targetVersion {
				if err := db.applyMigration(ctx, m, true); err != nil {
					return fmt.Errorf("migration %d: %w", m.Version, err)
				}
			}
		}
	} else {
		// Migrate down
		// Get applied migrations in reverse order
		applied, err := db.getAppliedMigrations()
		if err != nil {
			return fmt.Errorf("get applied migrations: %w", err)
		}

		sort.Slice(applied, func(i, j int) bool {
			return applied[i].Version > applied[j].Version
		})

		for _, record := range applied {
			if record.Version <= targetVersion {
				break
			}

			var migration *Migration
			for i := range migrations {
				if migrations[i].Version == record.Version {
					migration = &migrations[i]
					break
				}
			}

			if migration == nil {
				return fmt.Errorf("migration version %d not found", record.Version)
			}

			if err := db.applyMigration(ctx, *migration, false); err != nil {
				return fmt.Errorf("rollback migration %d: %w", migration.Version, err)
			}
		}
	}

	return nil
}

// GetMigrationStatus returns the status of all migrations.
func (db *DB) GetMigrationStatus() ([]MigrationStatus, error) {
	if err := db.ensureMigrationsTable(); err != nil {
		return nil, err
	}

	applied, err := db.getAppliedMigrations()
	if err != nil {
		return nil, err
	}

	appliedMap := make(map[int]time.Time)
	for _, a := range applied {
		appliedMap[a.Version] = a.AppliedAt
	}

	result := make([]MigrationStatus, len(migrations))
	for i, m := range migrations {
		status := MigrationStatus{
			Version:     m.Version,
			Description: m.Description,
			Applied:     false,
		}
		if appliedAt, ok := appliedMap[m.Version]; ok {
			status.Applied = true
			status.AppliedAt = appliedAt
		}
		result[i] = status
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})

	return result, nil
}

// MigrationStatus represents the status of a migration.
type MigrationStatus struct {
	Version     int
	Description string
	Applied     bool
	AppliedAt   time.Time
}

// ensureMigrationsTable creates the schema_migrations table if it doesn't exist.
func (db *DB) ensureMigrationsTable() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}
	return nil
}

// getCurrentVersion returns the highest applied migration version.
func (db *DB) getCurrentVersion() (int, error) {
	var version sql.NullInt64
	err := db.conn.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("query max version: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}

// getAppliedMigrations returns all applied migrations.
func (db *DB) getAppliedMigrations() ([]MigrationRecord, error) {
	rows, err := db.conn.Query(`SELECT version, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("query migrations: %w", err)
	}
	defer rows.Close()

	var records []MigrationRecord
	for rows.Next() {
		var r MigrationRecord
		if err := rows.Scan(&r.Version, &r.AppliedAt); err != nil {
			return nil, fmt.Errorf("scan migration record: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migrations: %w", err)
	}
	return records, nil
}

// applyMigration applies a single migration (up or down) within a transaction.
func (db *DB) applyMigration(ctx context.Context, m Migration, up bool) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if up {
		if err := m.Up(tx); err != nil {
			return fmt.Errorf("up: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, m.Version); err != nil {
			return fmt.Errorf("record migration: %w", err)
		}
	} else {
		if m.Down == nil {
			return fmt.Errorf("migration %d has no down function", m.Version)
		}
		if err := m.Down(tx); err != nil {
			return fmt.Errorf("down: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM schema_migrations WHERE version = ?`, m.Version); err != nil {
			return fmt.Errorf("remove migration record: %w", err)
		}
	}

	return tx.Commit()
}

// migrateFromLegacy handles migration from the old inline schema approach.
// It checks if tables exist but schema_migrations doesn't, and marks
// the appropriate migrations as applied.
func (db *DB) migrateFromLegacy() error {
	// Check if schema_migrations exists
	var count int
	err := db.conn.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master 
		WHERE type='table' AND name='schema_migrations'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check migrations table: %w", err)
	}

	if count > 0 {
		// Already using new migration system
		return nil
	}

	// Check if we have legacy tables
	err = db.conn.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master 
		WHERE type='table' AND name='users'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check users table: %w", err)
	}

	if count == 0 {
		// Fresh database, no legacy migration needed
		return nil
	}

	// Create migrations table
	if err := db.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	// Check which tables exist and mark corresponding migrations as applied
	tablesToMigration := map[string]int{
		"users":                   1,
		"agents":                  2,
		"deployments":             3,
		"audit_logs":              4,
		"projects":                5,
		"ssh_keys":                6,
		"encryption_keys":         7,
		"certificate_authorities": 8,
		"rate_limits":             9,
		"agent_provision_jobs":    10,
		"acme_certificates":       11,
	}

	maxApplied := 0
	for table, version := range tablesToMigration {
		err = db.conn.QueryRow(`
			SELECT COUNT(*) FROM sqlite_master 
			WHERE type='table' AND name=?
		`, table).Scan(&count)
		if err != nil {
			return fmt.Errorf("check table %s: %w", table, err)
		}
		if count > 0 && version > maxApplied {
			maxApplied = version
		}
	}

	// Mark all migrations up to maxApplied as applied
	for i := 1; i <= maxApplied; i++ {
		_, err = db.conn.Exec(`
			INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)
		`, i)
		if err != nil {
			return fmt.Errorf("mark migration %d applied: %w", i, err)
		}
	}

	return nil
}
