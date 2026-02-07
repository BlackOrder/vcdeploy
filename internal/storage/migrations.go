// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
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

// MigrationStatus represents the status of a migration.
type MigrationStatus struct {
	Version     int
	Description string
	Applied     bool
	AppliedAt   time.Time
}

// migrations is the ordered list of all migrations.
// CONSOLIDATED: All tables in their final form with no ALTER TABLE statements.
var migrations = []Migration{
	{
		Version:     1,
		Description: "Consolidated schema - all tables in final form",
		Up: func(tx *sql.Tx) error {
			// ============================================================
			// CORE TABLES: Users, Sessions, API Keys, Recovery Codes
			// ============================================================
			_, err := tx.Exec(`
				-- ============================================================
				-- TABLE: users
				-- ============================================================
				CREATE TABLE IF NOT EXISTS users (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
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
				CREATE INDEX IF NOT EXISTS idx_users_uid ON users(uid);

				-- ============================================================
				-- TABLE: sessions
				-- ============================================================
				CREATE TABLE IF NOT EXISTS sessions (
					id TEXT PRIMARY KEY,
					user_id INTEGER NOT NULL,
					ip_address TEXT,
					user_agent TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					expires_at DATETIME NOT NULL,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				);
				CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
				CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

				-- ============================================================
				-- TABLE: api_keys
				-- ============================================================
				CREATE TABLE IF NOT EXISTS api_keys (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
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
				CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);

				-- ============================================================
				-- TABLE: recovery_codes (TOTP recovery)
				-- ============================================================
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
			if err != nil {
				return fmt.Errorf("creating core tables: %w", err)
			}

			// ============================================================
			// PROJECT TABLES: Projects, Project Types, Webhooks, Secrets
			// ============================================================
			_, err = tx.Exec(`
				-- ============================================================
				-- TABLE: project_types
				-- ============================================================
				CREATE TABLE IF NOT EXISTS project_types (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					name TEXT UNIQUE NOT NULL,
					description TEXT,
					build_cmd TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);
				CREATE INDEX IF NOT EXISTS idx_project_types_uid ON project_types(uid);

				-- ============================================================
				-- TABLE: health_check_configs (referenced by projects)
				-- ============================================================
				CREATE TABLE IF NOT EXISTS health_check_configs (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
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
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);
				CREATE INDEX IF NOT EXISTS idx_health_check_configs_project ON health_check_configs(project_id);
				CREATE INDEX IF NOT EXISTS idx_health_check_configs_global ON health_check_configs(is_global);

				-- ============================================================
				-- TABLE: projects (with consolidated columns from migration 16)
				-- ============================================================
				CREATE TABLE IF NOT EXISTS projects (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					name TEXT UNIQUE NOT NULL,
					repository TEXT,
					branch TEXT DEFAULT 'main',
					deploy_path TEXT,
					type TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					last_deploy_at DATETIME,
					last_deploy_status TEXT,
					-- Consolidated from migration 16:
					health_check_id INTEGER REFERENCES health_check_configs(id),
					auto_rollback_enabled INTEGER DEFAULT 1,
					rollback_on_health_fail INTEGER DEFAULT 1
				);
				CREATE INDEX IF NOT EXISTS idx_projects_name ON projects(name);
				CREATE INDEX IF NOT EXISTS idx_projects_type ON projects(type);

				-- Add foreign key for health_check_configs.project_id after projects exists
				-- (SQLite doesn't enforce foreign keys on table creation order, but this is cleaner)

				-- ============================================================
				-- TABLE: project_webhooks (with require_secret from migration 13)
				-- ============================================================
				CREATE TABLE IF NOT EXISTS project_webhooks (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					project_id INTEGER NOT NULL,
					provider TEXT NOT NULL,
					secret_encrypted BLOB,
					enabled INTEGER DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					-- Consolidated from migration 13:
					require_secret INTEGER DEFAULT 0,
					FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
					UNIQUE(project_id, provider)
				);
				CREATE INDEX IF NOT EXISTS idx_project_webhooks_project ON project_webhooks(project_id);

				-- ============================================================
				-- TABLE: secrets
				-- ============================================================
				CREATE TABLE IF NOT EXISTS secrets (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					project TEXT NOT NULL,
					project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
					scope TEXT NOT NULL,
					key TEXT NOT NULL,
					value_encrypted BLOB NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					UNIQUE(project, scope, key)
				);
				CREATE INDEX IF NOT EXISTS idx_secrets_project_scope ON secrets(project, scope);
				CREATE INDEX IF NOT EXISTS idx_secrets_project_id ON secrets(project_id);
			`)
			if err != nil {
				return fmt.Errorf("creating project tables: %w", err)
			}

			// ============================================================
			// DEPLOYMENT TABLES: Deployments, Logs, Rollbacks
			// ============================================================
			_, err = tx.Exec(`
				-- ============================================================
				-- TABLE: deployments
				-- ============================================================
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
				CREATE INDEX IF NOT EXISTS idx_deployments_project ON deployments(project);
				CREATE INDEX IF NOT EXISTS idx_deployments_project_id ON deployments(project_id);
				CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
				CREATE INDEX IF NOT EXISTS idx_deployments_started_at ON deployments(started_at);
				CREATE INDEX IF NOT EXISTS idx_deployments_triggered_by ON deployments(triggered_by);
				CREATE INDEX IF NOT EXISTS idx_deployments_scheduled ON deployments(scheduled_at) WHERE scheduled_at IS NOT NULL;

				-- ============================================================
				-- TABLE: deployment_logs
				-- ============================================================
				CREATE TABLE IF NOT EXISTS deployment_logs (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					deployment_id TEXT NOT NULL,
					level TEXT NOT NULL,
					message TEXT NOT NULL,
					source TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (deployment_id) REFERENCES deployments(id) ON DELETE CASCADE
				);
				CREATE INDEX IF NOT EXISTS idx_deployment_logs_deployment_id ON deployment_logs(deployment_id);

				-- ============================================================
				-- TABLE: deployment_rollbacks
				-- ============================================================
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
				);
				CREATE INDEX IF NOT EXISTS idx_deployment_rollbacks_deployment ON deployment_rollbacks(deployment_id);
				CREATE INDEX IF NOT EXISTS idx_deployment_rollbacks_project ON deployment_rollbacks(project_name);

				-- ============================================================
				-- TABLE: deployment_agents (NEW - for multi-agent deployments)
				-- ============================================================
				CREATE TABLE IF NOT EXISTS deployment_agents (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					deployment_id TEXT NOT NULL,
					agent_id TEXT NOT NULL,
					status TEXT NOT NULL DEFAULT 'pending',
					started_at DATETIME,
					completed_at DATETIME,
					error_message TEXT,
					FOREIGN KEY (deployment_id) REFERENCES deployments(id) ON DELETE CASCADE,
					FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE,
					UNIQUE(deployment_id, agent_id)
				);
				CREATE INDEX IF NOT EXISTS idx_deployment_agents_deployment ON deployment_agents(deployment_id);
				CREATE INDEX IF NOT EXISTS idx_deployment_agents_agent ON deployment_agents(agent_id);
				CREATE INDEX IF NOT EXISTS idx_deployment_agents_status ON deployment_agents(status);
			`)
			if err != nil {
				return fmt.Errorf("creating deployment tables: %w", err)
			}

			// ============================================================
			// AGENT TABLES: Agents, Certificates, Binaries, Provisioning
			// ============================================================
			_, err = tx.Exec(`
				-- ============================================================
				-- TABLE: agents (with ALL columns consolidated from migrations 15, 18)
				-- ============================================================
				CREATE TABLE IF NOT EXISTS agents (
					id TEXT PRIMARY KEY,
					hostname TEXT,
					labels TEXT,
					capabilities TEXT,
					status TEXT DEFAULT 'offline',
					last_seen_at DATETIME,
					registered_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					certificate TEXT,
					-- Consolidated from migration 15:
					version TEXT DEFAULT '',
					os TEXT DEFAULT '',
					arch TEXT DEFAULT '',
					update_policy TEXT DEFAULT 'immediate',
					update_window_start TEXT DEFAULT '',
					update_window_end TEXT DEFAULT '',
					last_update_at DATETIME,
					last_update_error TEXT DEFAULT '',
					-- Consolidated from migration 18:
					hmac_secret BLOB
				);
				CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
				CREATE INDEX IF NOT EXISTS idx_agents_hostname ON agents(hostname);

				-- ============================================================
				-- TABLE: agent_update_history
				-- ============================================================
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
				);
				CREATE INDEX IF NOT EXISTS idx_agent_update_history_agent ON agent_update_history(agent_id);
				CREATE INDEX IF NOT EXISTS idx_agent_update_history_status ON agent_update_history(status);

				-- ============================================================
				-- TABLE: agent_binaries
				-- ============================================================
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
				CREATE INDEX IF NOT EXISTS idx_agent_binaries_current ON agent_binaries(is_current);
				CREATE INDEX IF NOT EXISTS idx_agent_binaries_version ON agent_binaries(version, os, arch);

				-- ============================================================
				-- TABLE: agent_provision_jobs
				-- ============================================================
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
				CREATE INDEX IF NOT EXISTS idx_provision_jobs_status ON agent_provision_jobs(status);
				CREATE INDEX IF NOT EXISTS idx_provision_jobs_target ON agent_provision_jobs(target_host);

				-- ============================================================
				-- TABLE: provision_logs
				-- ============================================================
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
			if err != nil {
				return fmt.Errorf("creating agent tables: %w", err)
			}

			// ============================================================
			// SECURITY TABLES: CA, Certificates, Encryption Keys, SSH
			// ============================================================
			_, err = tx.Exec(`
				-- ============================================================
				-- TABLE: certificate_authorities
				-- ============================================================
				CREATE TABLE IF NOT EXISTS certificate_authorities (
					id TEXT PRIMARY KEY,
					uid TEXT UNIQUE NOT NULL,
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
				CREATE INDEX IF NOT EXISTS idx_ca_status ON certificate_authorities(status);
				CREATE INDEX IF NOT EXISTS idx_ca_is_current ON certificate_authorities(is_current);

				-- ============================================================
				-- TABLE: agent_certificates
				-- ============================================================
				CREATE TABLE IF NOT EXISTS agent_certificates (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
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
				CREATE INDEX IF NOT EXISTS idx_agent_certs_agent_id ON agent_certificates(agent_id);
				CREATE INDEX IF NOT EXISTS idx_agent_certs_ca_id ON agent_certificates(ca_id);
				CREATE INDEX IF NOT EXISTS idx_agent_certs_status ON agent_certificates(status);
				CREATE INDEX IF NOT EXISTS idx_agent_certs_not_after ON agent_certificates(not_after);

				-- ============================================================
				-- TABLE: server_certificates
				-- ============================================================
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
				);
				CREATE INDEX IF NOT EXISTS idx_server_certificates_ca_id ON server_certificates(ca_id);
				CREATE INDEX IF NOT EXISTS idx_server_certificates_hostname ON server_certificates(hostname);

				-- ============================================================
				-- TABLE: revoked_certificates
				-- ============================================================
				CREATE TABLE IF NOT EXISTS revoked_certificates (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					serial_number TEXT UNIQUE NOT NULL,
					agent_id TEXT,
					reason TEXT,
					revoked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					revoked_by TEXT NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_revoked_certificates_serial ON revoked_certificates(serial_number);

				-- ============================================================
				-- TABLE: registration_tokens
				-- ============================================================
				CREATE TABLE IF NOT EXISTS registration_tokens (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					token TEXT UNIQUE NOT NULL,
					agent_id TEXT,
					expires_at DATETIME,
					used_at DATETIME,
					created_by TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);
				CREATE INDEX IF NOT EXISTS idx_registration_tokens_expires ON registration_tokens(expires_at);

				-- ============================================================
				-- TABLE: encryption_keys (KMS-style)
				-- ============================================================
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
				CREATE INDEX IF NOT EXISTS idx_encryption_keys_status ON encryption_keys(status);
				CREATE INDEX IF NOT EXISTS idx_encryption_keys_version ON encryption_keys(version);

				-- ============================================================
				-- TABLE: encryption_key_usage
				-- ============================================================
				CREATE TABLE IF NOT EXISTS encryption_key_usage (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					key_id TEXT NOT NULL,
					operation TEXT NOT NULL,
					resource_type TEXT,
					resource_id TEXT,
					timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (key_id) REFERENCES encryption_keys(id)
				);
				CREATE INDEX IF NOT EXISTS idx_encryption_key_usage_key_id ON encryption_key_usage(key_id);
				CREATE INDEX IF NOT EXISTS idx_encryption_key_usage_timestamp ON encryption_key_usage(timestamp);

				-- ============================================================
				-- TABLE: cert_audit_events
				-- ============================================================
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
				);
				CREATE INDEX IF NOT EXISTS idx_cert_audit_events_event ON cert_audit_events(event_type);
				CREATE INDEX IF NOT EXISTS idx_cert_audit_events_created ON cert_audit_events(timestamp);
			`)
			if err != nil {
				return fmt.Errorf("creating security tables: %w", err)
			}

			// ============================================================
			// SSH TABLES: Keys, Known Hosts, Host Keys, Jump Servers
			// ============================================================
			_, err = tx.Exec(`
				-- ============================================================
				-- TABLE: ssh_keys (with created_by from migration 17)
				-- ============================================================
				CREATE TABLE IF NOT EXISTS ssh_keys (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					name TEXT UNIQUE NOT NULL,
					public_key TEXT NOT NULL,
					private_key_encrypted BLOB NOT NULL,
					key_type TEXT NOT NULL DEFAULT 'ed25519',
					fingerprint TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					last_used_at DATETIME,
					-- Consolidated from migration 17:
					created_by TEXT DEFAULT 'system'
				);
				CREATE INDEX IF NOT EXISTS idx_ssh_keys_name ON ssh_keys(name);
				CREATE INDEX IF NOT EXISTS idx_ssh_keys_fingerprint ON ssh_keys(fingerprint);

				-- ============================================================
				-- TABLE: known_hosts
				-- ============================================================
				CREATE TABLE IF NOT EXISTS known_hosts (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					hostname TEXT NOT NULL,
					port INTEGER NOT NULL DEFAULT 22,
					key_type TEXT NOT NULL,
					public_key TEXT NOT NULL,
					fingerprint TEXT NOT NULL,
					added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					last_verified_at DATETIME,
					UNIQUE(hostname, port, key_type)
				);
				CREATE INDEX IF NOT EXISTS idx_known_hosts_hostname ON known_hosts(hostname, port);

				-- ============================================================
				-- TABLE: ssh_host_keys
				-- ============================================================
				CREATE TABLE IF NOT EXISTS ssh_host_keys (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
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
				CREATE INDEX IF NOT EXISTS idx_ssh_host_keys_hostname_port ON ssh_host_keys(hostname, port);
				CREATE INDEX IF NOT EXISTS idx_ssh_host_keys_fingerprint ON ssh_host_keys(fingerprint);

				-- ============================================================
				-- TABLE: ssh_jump_servers
				-- ============================================================
				CREATE TABLE IF NOT EXISTS ssh_jump_servers (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					name TEXT UNIQUE NOT NULL,
					host TEXT NOT NULL,
					port INTEGER DEFAULT 22,
					username TEXT NOT NULL,
					ssh_key_id INTEGER,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (ssh_key_id) REFERENCES ssh_keys(id)
				);

				-- ============================================================
				-- TABLE: source_credentials (git credentials)
				-- ============================================================
				CREATE TABLE IF NOT EXISTS source_credentials (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					name TEXT UNIQUE NOT NULL,
					type TEXT NOT NULL,
					url_pattern TEXT NOT NULL,
					credential_encrypted BLOB,
					created_by TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);
			`)
			if err != nil {
				return fmt.Errorf("creating SSH tables: %w", err)
			}

			// ============================================================
			// INFRASTRUCTURE TABLES: Settings, Rate Limits, Audit Logs
			// ============================================================
			_, err = tx.Exec(`
				-- ============================================================
				-- TABLE: settings
				-- ============================================================
				CREATE TABLE IF NOT EXISTS settings (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
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
				CREATE INDEX IF NOT EXISTS idx_settings_category ON settings(category);

				-- ============================================================
				-- TABLE: rate_limits
				-- ============================================================
				CREATE TABLE IF NOT EXISTS rate_limits (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					key TEXT NOT NULL,
					bucket TEXT NOT NULL,
					count INTEGER NOT NULL DEFAULT 1,
					window_start DATETIME NOT NULL,
					window_end DATETIME NOT NULL,
					UNIQUE(key, bucket, window_start)
				);
				CREATE INDEX IF NOT EXISTS idx_rate_limits_key_bucket ON rate_limits(key, bucket);
				CREATE INDEX IF NOT EXISTS idx_rate_limits_window ON rate_limits(window_end);

				-- ============================================================
				-- TABLE: blocked_ips
				-- ============================================================
				CREATE TABLE IF NOT EXISTS blocked_ips (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					ip_address TEXT UNIQUE NOT NULL,
					reason TEXT NOT NULL,
					blocked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					expires_at DATETIME,
					blocked_by TEXT
				);
				CREATE INDEX IF NOT EXISTS idx_blocked_ips_expires ON blocked_ips(expires_at);

				-- ============================================================
				-- TABLE: audit_logs
				-- ============================================================
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
				CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp);
				CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
				CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(user);
				CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource, resource_id);

				-- ============================================================
				-- TABLE: webhook_secrets
				-- ============================================================
				CREATE TABLE IF NOT EXISTS webhook_secrets (
					provider TEXT PRIMARY KEY,
					uid TEXT UNIQUE NOT NULL,
					secret_encrypted BLOB NOT NULL,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				-- ============================================================
				-- TABLE: master_key_meta
				-- ============================================================
				CREATE TABLE IF NOT EXISTS master_key_meta (
					id INTEGER PRIMARY KEY CHECK (id = 1),
					key_id TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					rotated_at DATETIME,
					previous_key_id TEXT,
					previous_key_expires_at DATETIME
				);
			`)
			if err != nil {
				return fmt.Errorf("creating infrastructure tables: %w", err)
			}

			// ============================================================
			// ACME TABLES: Certificates, Accounts
			// ============================================================
			_, err = tx.Exec(`
				-- ============================================================
				-- TABLE: acme_certificates
				-- ============================================================
				CREATE TABLE IF NOT EXISTS acme_certificates (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
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
				CREATE INDEX IF NOT EXISTS idx_acme_certs_domain ON acme_certificates(domain);
				CREATE INDEX IF NOT EXISTS idx_acme_certs_expiry ON acme_certificates(not_after);

				-- ============================================================
				-- TABLE: acme_accounts
				-- ============================================================
				CREATE TABLE IF NOT EXISTS acme_accounts (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					email TEXT NOT NULL,
					account_url TEXT,
					private_key_encrypted BLOB NOT NULL,
					directory_url TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);
			`)
			if err != nil {
				return fmt.Errorf("creating ACME tables: %w", err)
			}

			// ============================================================
			// RECIPE TABLES: Components, Playbooks, Activations, Approvals
			// ============================================================
			_, err = tx.Exec(`
				-- ============================================================
				-- TABLE: recipe_components
				-- ============================================================
				CREATE TABLE IF NOT EXISTS recipe_components (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					namespace TEXT NOT NULL CHECK(namespace IN ('seed', 'user')),
					slug TEXT NOT NULL,
					version TEXT NOT NULL,
					name TEXT NOT NULL,
					description TEXT,
					component_type TEXT NOT NULL CHECK(component_type IN ('hook', 'command', 'service_reload', 'file_op')),
					content JSON NOT NULL,
					variables JSON,
					is_seed BOOLEAN NOT NULL DEFAULT 0,
					is_raw BOOLEAN NOT NULL DEFAULT 0,
					is_deprecated BOOLEAN NOT NULL DEFAULT 0,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					UNIQUE(namespace, slug, version)
				);
				CREATE INDEX IF NOT EXISTS idx_recipe_components_namespace ON recipe_components(namespace);
				CREATE INDEX IF NOT EXISTS idx_recipe_components_slug ON recipe_components(namespace, slug);
				CREATE INDEX IF NOT EXISTS idx_recipe_components_type ON recipe_components(component_type);

				-- ============================================================
				-- TABLE: playbooks
				-- ============================================================
				CREATE TABLE IF NOT EXISTS playbooks (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					namespace TEXT NOT NULL CHECK(namespace IN ('seed', 'user')),
					slug TEXT NOT NULL,
					version TEXT NOT NULL,
					name TEXT NOT NULL,
					description TEXT,
					framework_type TEXT,
					steps JSON NOT NULL,
					shared_dirs JSON,
					shared_files JSON,
					writable_dirs JSON,
					keep_releases INTEGER NOT NULL DEFAULT 5,
					validation_rules JSON,
					is_seed BOOLEAN NOT NULL DEFAULT 0,
					is_deprecated BOOLEAN NOT NULL DEFAULT 0,
					parent_id INTEGER,
					parent_version TEXT,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					UNIQUE(namespace, slug, version),
					FOREIGN KEY (parent_id) REFERENCES playbooks(id) ON DELETE SET NULL
				);
				CREATE INDEX IF NOT EXISTS idx_playbooks_namespace ON playbooks(namespace);
				CREATE INDEX IF NOT EXISTS idx_playbooks_slug ON playbooks(namespace, slug);
				CREATE INDEX IF NOT EXISTS idx_playbooks_framework ON playbooks(framework_type);
				CREATE INDEX IF NOT EXISTS idx_playbooks_parent ON playbooks(parent_id);

				-- ============================================================
				-- TABLE: playbook_activations
				-- ============================================================
				CREATE TABLE IF NOT EXISTS playbook_activations (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					project_id INTEGER NOT NULL,
					playbook_id INTEGER NOT NULL,
					activated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					activated_by INTEGER,
					FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
					FOREIGN KEY (playbook_id) REFERENCES playbooks(id) ON DELETE RESTRICT,
					FOREIGN KEY (activated_by) REFERENCES users(id) ON DELETE SET NULL
				);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_playbook_activations_project ON playbook_activations(project_id);
				CREATE INDEX IF NOT EXISTS idx_playbook_activations_playbook ON playbook_activations(playbook_id);

				-- ============================================================
				-- TABLE: playbook_variable_bindings
				-- ============================================================
				CREATE TABLE IF NOT EXISTS playbook_variable_bindings (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					activation_id INTEGER NOT NULL,
					variable_name TEXT NOT NULL,
					source_type TEXT NOT NULL CHECK(source_type IN ('literal', 'env', 'secret')),
					source_ref TEXT,
					literal_value TEXT,
					FOREIGN KEY (activation_id) REFERENCES playbook_activations(id) ON DELETE CASCADE,
					UNIQUE(activation_id, variable_name)
				);
				CREATE INDEX IF NOT EXISTS idx_variable_bindings_activation ON playbook_variable_bindings(activation_id);
				CREATE INDEX IF NOT EXISTS idx_variable_bindings_source ON playbook_variable_bindings(source_type, source_ref);

				-- ============================================================
				-- TABLE: raw_command_approvals
				-- ============================================================
				CREATE TABLE IF NOT EXISTS raw_command_approvals (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					uid TEXT UNIQUE NOT NULL,
					component_id INTEGER NOT NULL,
					approved_by INTEGER NOT NULL,
					approved_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					approval_note TEXT,
					FOREIGN KEY (component_id) REFERENCES recipe_components(id) ON DELETE CASCADE,
					FOREIGN KEY (approved_by) REFERENCES users(id) ON DELETE RESTRICT
				);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_raw_approvals_component ON raw_command_approvals(component_id);
			`)
			if err != nil {
				return fmt.Errorf("creating recipe tables: %w", err)
			}

			// Insert default global health check configuration
			_, err = tx.Exec(`
				INSERT OR IGNORE INTO health_check_configs (
					uid, name, url, method, expected_status, timeout_seconds, retries, 
					retry_delay_seconds, enabled, is_global
				) VALUES (
					'default-global-hc', 'Global Default', '{{.URL}}', 'GET', 200, 10, 3, 5, 1, 1
				)
			`)
			if err != nil {
				return fmt.Errorf("inserting default health check config: %w", err)
			}

			return nil
		},
		Down: func(tx *sql.Tx) error {
			// Drop all tables in reverse dependency order
			tables := []string{
				// Recipe tables
				"raw_command_approvals",
				"playbook_variable_bindings",
				"playbook_activations",
				"playbooks",
				"recipe_components",
				// ACME tables
				"acme_accounts",
				"acme_certificates",
				// Infrastructure tables
				"master_key_meta",
				"webhook_secrets",
				"audit_logs",
				"blocked_ips",
				"rate_limits",
				"settings",
				// SSH tables
				"source_credentials",
				"ssh_jump_servers",
				"ssh_host_keys",
				"known_hosts",
				"ssh_keys",
				// Security tables
				"cert_audit_events",
				"encryption_key_usage",
				"encryption_keys",
				"registration_tokens",
				"revoked_certificates",
				"server_certificates",
				"agent_certificates",
				"certificate_authorities",
				// Agent tables
				"provision_logs",
				"agent_provision_jobs",
				"agent_binaries",
				"agent_update_history",
				"agents",
				// Deployment tables
				"deployment_agents",
				"deployment_rollbacks",
				"deployment_logs",
				"deployments",
				// Project tables
				"secrets",
				"project_webhooks",
				"projects",
				"health_check_configs",
				"project_types",
				// Core tables
				"recovery_codes",
				"api_keys",
				"sessions",
				"users",
			}
			for _, table := range tables {
				if _, err := tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
					return fmt.Errorf("dropping table %s: %w", table, err)
				}
			}
			return nil
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

// migrateFromLegacy checks for legacy databases using the old multi-migration
// schema (versions 1-21) and marks them as migrated to the consolidated version 1.
// This ensures existing databases continue working after the schema consolidation.
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
		// Check if already on new consolidated schema (version 1 is present
		// and has the deployment_agents table which is new in consolidated schema)
		var hasDeploymentAgents int
		err = db.conn.QueryRow(`
			SELECT COUNT(*) FROM sqlite_master 
			WHERE type='table' AND name='deployment_agents'
		`).Scan(&hasDeploymentAgents)
		if err != nil {
			return fmt.Errorf("check deployment_agents table: %w", err)
		}

		if hasDeploymentAgents > 0 {
			// Already on consolidated schema
			return nil
		}

		// Legacy database with old migration versions - needs upgrade path
		// This would require data migration which is handled separately
		return nil
	}

	if count == 0 {
		// No schema_migrations table - check for existing tables
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
	}

	// Create migrations table if it doesn't exist
	if err := db.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	// For legacy databases, we need to check if core tables exist
	// and mark the consolidated migration as applied
	var hasUsers int
	err = db.conn.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master 
		WHERE type='table' AND name='users'
	`).Scan(&hasUsers)
	if err != nil {
		return fmt.Errorf("check users table: %w", err)
	}

	if hasUsers > 0 {
		// Legacy database exists - mark consolidated migration as applied
		// Note: This assumes the legacy database has all required tables
		// A proper upgrade would require comparing schemas
		_, err = db.conn.Exec(`
			INSERT OR IGNORE INTO schema_migrations (version) VALUES (1)
		`)
		if err != nil {
			return fmt.Errorf("mark consolidated migration applied: %w", err)
		}
	}

	return nil
}
