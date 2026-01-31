package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// executeWriteOp executes a single write operation within a transaction.
// This is the callback passed to FlushBatchFunc.
func (s *MemoryStore) executeWriteOp(tx *sql.Tx, op WriteOp) error {
	switch op.Table {
	case "users":
		return s.executeUserOp(tx, op)
	case "sessions":
		return s.executeSessionOp(tx, op)
	case "api_keys":
		return s.executeAPIKeyOp(tx, op)
	case "settings":
		return s.executeSettingOp(tx, op)
	case "projects":
		return s.executeProjectOp(tx, op)
	case "project_types":
		return s.executeProjectTypeOp(tx, op)
	case "project_webhooks", "webhooks":
		return s.executeWebhookOp(tx, op)
	case "secrets":
		return s.executeSecretOp(tx, op)
	case "agents":
		return s.executeAgentOp(tx, op)
	case "agent_binaries":
		return s.executeAgentBinaryOp(tx, op)
	case "agent_update_history":
		return s.executeAgentUpdateHistoryOp(tx, op)
	case "deployments":
		return s.executeDeploymentOp(tx, op)
	case "deployment_logs":
		return s.executeDeploymentLogOp(tx, op)
	case "deployment_rollbacks":
		return s.executeDeploymentRollbackOp(tx, op)
	case "scheduled_deployments":
		return s.executeScheduledDeploymentOp(tx, op)
	case "audit_logs":
		return s.executeAuditLogOp(tx, op)
	case "blocked_ips":
		return s.executeBlockedIPOp(tx, op)
	case "rate_limits":
		return s.executeRateLimitOp(tx, op)
	case "provision_jobs":
		return s.executeProvisionJobOp(tx, op)
	case "ssh_host_keys":
		return s.executeSSHHostKeyOp(tx, op)
	case "ssh_jump_servers":
		return s.executeJumpServerOp(tx, op)
	case "health_check_configs":
		return s.executeHealthCheckConfigOp(tx, op)
	default:
		s.logger.Warn("unknown table for write op",
			zap.String("table", op.Table),
			zap.String("type", op.Type.String()))
		return nil
	}
}

// --- User operations ---

func (s *MemoryStore) executeUserOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		user, ok := op.Data.(*User)
		if !ok {
			return fmt.Errorf("invalid data type for user insert")
		}
		_, err := tx.Exec(`
			INSERT INTO users (id, username, password_hash, email, role, must_change_password, totp_secret, totp_enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				username = excluded.username,
				password_hash = excluded.password_hash,
				email = excluded.email,
				role = excluded.role,
				must_change_password = excluded.must_change_password,
				totp_secret = excluded.totp_secret,
				totp_enabled = excluded.totp_enabled,
				updated_at = excluded.updated_at
		`, user.ID, user.Username, user.PasswordHash, user.Email, user.Role,
			user.MustChangePassword, user.TOTPSecret, user.TOTPEnabled, user.CreatedAt, user.UpdatedAt)
		return err

	case WriteOpUpdate:
		user, ok := op.Data.(*User)
		if !ok {
			return fmt.Errorf("invalid data type for user update")
		}
		_, err := tx.Exec(`
			UPDATE users SET username = ?, password_hash = ?, email = ?, role = ?, 
				must_change_password = ?, totp_secret = ?, totp_enabled = ?, updated_at = ?
			WHERE id = ?
		`, user.Username, user.PasswordHash, user.Email, user.Role,
			user.MustChangePassword, user.TOTPSecret, user.TOTPEnabled, user.UpdatedAt, user.ID)
		return err

	case WriteOpDelete:
		// Accept both int64 and map[string]int64{"id": ...} formats
		var id int64
		switch v := op.Data.(type) {
		case int64:
			id = v
		case map[string]int64:
			id = v["id"]
		default:
			return fmt.Errorf("invalid data type for user delete")
		}
		_, err := tx.Exec(`DELETE FROM users WHERE id = ?`, id)
		return err
	}
	return nil
}

// --- Session operations ---

func (s *MemoryStore) executeSessionOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		session, ok := op.Data.(*Session)
		if !ok {
			return fmt.Errorf("invalid data type for session insert")
		}
		_, err := tx.Exec(`
			INSERT INTO sessions (id, user_id, ip_address, user_agent, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, session.ID, session.UserID, session.IPAddress, session.UserAgent, session.CreatedAt, session.ExpiresAt)
		return err

	case WriteOpDelete:
		switch v := op.Data.(type) {
		case string:
			_, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, v)
			return err
		case int64:
			_, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, v)
			return err
		default:
			return fmt.Errorf("invalid data type for session delete")
		}
	}
	return nil
}

// --- API Key operations ---

func (s *MemoryStore) executeAPIKeyOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		key, ok := op.Data.(*APIKey)
		if !ok {
			return fmt.Errorf("invalid data type for api_key insert")
		}
		_, err := tx.Exec(`
			INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, key.ID, key.UserID, key.Name, key.KeyHash, key.KeyPrefix, key.Scopes, key.ExpiresAt, key.LastUsedAt, key.CreatedAt)
		return err

	case WriteOpUpdate:
		// Handle partial updates (e.g., last_used_at)
		if m, ok := op.Data.(map[string]any); ok {
			if id, ok := m["id"].(int64); ok {
				if lastUsed, ok := m["last_used_at"]; ok {
					_, err := tx.Exec(`UPDATE api_keys SET last_used_at = ? WHERE id = ?`, lastUsed, id)
					return err
				}
			}
		}
		return nil

	case WriteOpDelete:
		// Support both direct int64 and map[string]int64{"id": ...} formats
		var id int64
		switch v := op.Data.(type) {
		case int64:
			id = v
		case map[string]int64:
			var ok bool
			id, ok = v["id"]
			if !ok {
				return fmt.Errorf("invalid data type for api_key delete: missing id in map")
			}
		default:
			return fmt.Errorf("invalid data type for api_key delete: got %T", op.Data)
		}
		_, err := tx.Exec(`DELETE FROM api_keys WHERE id = ?`, id)
		return err
	}
	return nil
}

// --- Setting operations ---

func (s *MemoryStore) executeSettingOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert, WriteOpUpdate:
		setting, ok := op.Data.(*Setting)
		if !ok {
			return fmt.Errorf("invalid data type for setting")
		}
		_, err := tx.Exec(`
			INSERT INTO settings (id, category, key, value, value_type, encrypted, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(category, key) DO UPDATE SET
				value = excluded.value,
				encrypted = excluded.encrypted,
				updated_at = excluded.updated_at
		`, setting.ID, setting.Category, setting.Key, setting.Value, setting.ValueType, setting.Encrypted, setting.Description, setting.CreatedAt, setting.UpdatedAt)
		return err

	case WriteOpDelete:
		key, ok := op.Data.(string)
		if !ok {
			return fmt.Errorf("invalid data type for setting delete")
		}
		// key is "category:key" format
		_, err := tx.Exec(`DELETE FROM settings WHERE category || ':' || key = ?`, key)
		return err
	}
	return nil
}

// --- Project operations ---

func (s *MemoryStore) executeProjectOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		p, ok := op.Data.(*Project)
		if !ok {
			return fmt.Errorf("invalid data type for project insert")
		}
		_, err := tx.Exec(`
			INSERT INTO projects (id, name, repository, branch, deploy_path, type, created_at, updated_at, 
				last_deploy_at, last_deploy_status, health_check_id, auto_rollback_enabled, rollback_on_health_fail)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, p.ID, p.Name, p.Repository, p.Branch, p.DeployPath, p.Type, p.CreatedAt, p.UpdatedAt,
			p.LastDeployAt, p.LastDeployStatus, p.HealthCheckID, p.AutoRollbackEnabled, p.RollbackOnHealthFail)
		return err

	case WriteOpUpdate:
		p, ok := op.Data.(*Project)
		if !ok {
			return fmt.Errorf("invalid data type for project update")
		}
		_, err := tx.Exec(`
			UPDATE projects SET name = ?, repository = ?, branch = ?, deploy_path = ?, type = ?,
				updated_at = ?, last_deploy_at = ?, last_deploy_status = ?, health_check_id = ?,
				auto_rollback_enabled = ?, rollback_on_health_fail = ?
			WHERE id = ?
		`, p.Name, p.Repository, p.Branch, p.DeployPath, p.Type, p.UpdatedAt,
			p.LastDeployAt, p.LastDeployStatus, p.HealthCheckID, p.AutoRollbackEnabled, p.RollbackOnHealthFail, p.ID)
		return err

	case WriteOpDelete:
		id, ok := op.Data.(int64)
		if !ok {
			return fmt.Errorf("invalid data type for project delete")
		}
		_, err := tx.Exec(`DELETE FROM projects WHERE id = ?`, id)
		return err
	}
	return nil
}

// --- Project Type operations ---

func (s *MemoryStore) executeProjectTypeOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		pt, ok := op.Data.(*ProjectType)
		if !ok {
			return fmt.Errorf("invalid data type for project_type insert")
		}
		_, err := tx.Exec(`
			INSERT INTO project_types (id, name, description, build_cmd, project_count, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, pt.ID, pt.Name, pt.Description, pt.BuildCmd, pt.ProjectCount, pt.CreatedAt)
		return err

	case WriteOpUpdate:
		pt, ok := op.Data.(*ProjectType)
		if !ok {
			return fmt.Errorf("invalid data type for project_type update")
		}
		_, err := tx.Exec(`
			UPDATE project_types SET name = ?, description = ?, build_cmd = ?, project_count = ?
			WHERE id = ?
		`, pt.Name, pt.Description, pt.BuildCmd, pt.ProjectCount, pt.ID)
		return err

	case WriteOpDelete:
		id, ok := op.Data.(int64)
		if !ok {
			return fmt.Errorf("invalid data type for project_type delete")
		}
		_, err := tx.Exec(`DELETE FROM project_types WHERE id = ?`, id)
		return err
	}
	return nil
}

// --- Webhook operations ---

func (s *MemoryStore) executeWebhookOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert, WriteOpUpdate:
		wh, ok := op.Data.(*ProjectWebhook)
		if !ok {
			return fmt.Errorf("invalid data type for webhook")
		}
		_, err := tx.Exec(`
			INSERT INTO project_webhooks (id, project_id, provider, secret_encrypted, enabled, require_secret, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_id, provider) DO UPDATE SET
				secret_encrypted = excluded.secret_encrypted,
				enabled = excluded.enabled,
				require_secret = excluded.require_secret,
				updated_at = excluded.updated_at
		`, wh.ID, wh.ProjectID, wh.Provider, wh.SecretEncrypted, wh.Enabled, wh.RequireSecret, wh.CreatedAt, wh.UpdatedAt)
		return err

	case WriteOpDelete:
		id, ok := op.Data.(int64)
		if !ok {
			return fmt.Errorf("invalid data type for webhook delete")
		}
		_, err := tx.Exec(`DELETE FROM project_webhooks WHERE id = ?`, id)
		return err
	}
	return nil
}

// --- Secret operations ---

func (s *MemoryStore) executeSecretOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert, WriteOpUpdate:
		secret, ok := op.Data.(*Secret)
		if !ok {
			return fmt.Errorf("invalid data type for secret")
		}
		_, err := tx.Exec(`
			INSERT INTO secrets (id, project, project_id, scope, key, value_encrypted, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(project, scope, key) DO UPDATE SET
				value_encrypted = excluded.value_encrypted,
				updated_at = excluded.updated_at
		`, secret.ID, secret.Project, secret.ProjectID, secret.Scope, secret.Key, secret.ValueEncrypted, secret.CreatedAt, secret.UpdatedAt)
		return err

	case WriteOpDelete:
		key, ok := op.Data.(string)
		if !ok {
			return fmt.Errorf("invalid data type for secret delete")
		}
		// key is "project:scope:key" format - delete by parsing
		_, err := tx.Exec(`DELETE FROM secrets WHERE project || ':' || scope || ':' || key = ?`, key)
		return err
	}
	return nil
}

// --- Agent operations ---

func (s *MemoryStore) executeAgentOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert, WriteOpUpdate:
		a, ok := op.Data.(*Agent)
		if !ok {
			return fmt.Errorf("invalid data type for agent")
		}
		labelsJSON, _ := json.Marshal(a.Labels)
		_, err := tx.Exec(`
			INSERT INTO agents (id, hostname, labels, capabilities, status, last_seen_at, registered_at, certificate, version, os, arch, update_policy, update_window_start, update_window_end, last_update_at, last_update_error)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				hostname = excluded.hostname,
				labels = excluded.labels,
				capabilities = excluded.capabilities,
				status = excluded.status,
				last_seen_at = excluded.last_seen_at,
				certificate = excluded.certificate,
				version = excluded.version,
				os = excluded.os,
				arch = excluded.arch,
				update_policy = excluded.update_policy,
				update_window_start = excluded.update_window_start,
				update_window_end = excluded.update_window_end,
				last_update_at = excluded.last_update_at,
				last_update_error = excluded.last_update_error
		`, a.ID, a.Hostname, string(labelsJSON), a.Capabilities, a.Status, a.LastSeenAt, a.RegisteredAt, a.Certificate, a.Version, a.OS, a.Arch, a.UpdatePolicy, a.UpdateWindowStart, a.UpdateWindowEnd, a.LastUpdateAt, a.LastUpdateError)
		return err

	case WriteOpDelete:
		id, ok := op.Data.(string)
		if !ok {
			return fmt.Errorf("invalid data type for agent delete")
		}
		_, err := tx.Exec(`DELETE FROM agents WHERE id = ?`, id)
		return err
	}
	return nil
}

// --- Agent Binary operations ---

func (s *MemoryStore) executeAgentBinaryOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		b, ok := op.Data.(*AgentBinary)
		if !ok {
			return fmt.Errorf("invalid data type for agent_binary insert")
		}
		_, err := tx.Exec(`
			INSERT INTO agent_binaries (id, version, os, arch, path, checksum_sha256, size_bytes, uploaded_at, is_current)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, b.ID, b.Version, b.OS, b.Arch, b.Path, b.ChecksumSHA256, b.SizeBytes, b.UploadedAt, b.IsCurrent)
		return err

	case WriteOpUpdate:
		b, ok := op.Data.(*AgentBinary)
		if !ok {
			return fmt.Errorf("invalid data type for agent_binary update")
		}
		_, err := tx.Exec(`
			UPDATE agent_binaries SET version = ?, os = ?, arch = ?, path = ?, 
				checksum_sha256 = ?, size_bytes = ?, is_current = ?
			WHERE id = ?
		`, b.Version, b.OS, b.Arch, b.Path, b.ChecksumSHA256, b.SizeBytes, b.IsCurrent, b.ID)
		return err

	case WriteOpDelete:
		id, ok := op.Data.(int64)
		if !ok {
			return fmt.Errorf("invalid data type for agent_binary delete")
		}
		_, err := tx.Exec(`DELETE FROM agent_binaries WHERE id = ?`, id)
		return err
	}
	return nil
}

// --- Agent Update History operations ---

func (s *MemoryStore) executeAgentUpdateHistoryOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		h, ok := op.Data.(*AgentUpdateHistory)
		if !ok {
			return fmt.Errorf("invalid data type for agent_update_history insert")
		}
		_, err := tx.Exec(`
			INSERT INTO agent_update_history (id, agent_id, from_version, to_version, status, error_message, started_at, completed_at, rolled_back)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, h.ID, h.AgentID, h.FromVersion, h.ToVersion, h.Status, h.ErrorMessage, h.StartedAt, h.CompletedAt, h.RolledBack)
		return err

	case WriteOpUpdate:
		h, ok := op.Data.(*AgentUpdateHistory)
		if !ok {
			return fmt.Errorf("invalid data type for agent_update_history update")
		}
		_, err := tx.Exec(`
			UPDATE agent_update_history SET status = ?, error_message = ?, completed_at = ?, rolled_back = ?
			WHERE id = ?
		`, h.Status, h.ErrorMessage, h.CompletedAt, h.RolledBack, h.ID)
		return err
	}
	return nil
}

// --- Deployment operations ---

func (s *MemoryStore) executeDeploymentOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		d, ok := op.Data.(*DeploymentRecord)
		if !ok {
			return fmt.Errorf("invalid data type for deployment insert")
		}
		_, err := tx.Exec(`
			INSERT INTO deployments (id, project, project_id, target, branch, commit_hash, status, release_number, started_at, completed_at, triggered_by, trigger_source, error_message)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, d.ID, d.Project, d.ProjectID, d.Target, d.Branch, d.CommitHash, d.Status, d.ReleaseNumber, d.StartedAt, d.CompletedAt, d.TriggeredBy, d.TriggerSource, d.ErrorMessage)
		return err

	case WriteOpUpdate:
		d, ok := op.Data.(*DeploymentRecord)
		if !ok {
			return fmt.Errorf("invalid data type for deployment update")
		}
		_, err := tx.Exec(`
			UPDATE deployments SET status = ?, completed_at = ?, error_message = ?
			WHERE id = ?
		`, d.Status, d.CompletedAt, d.ErrorMessage, d.ID)
		return err

	case WriteOpDelete:
		id, ok := op.Data.(string)
		if !ok {
			return fmt.Errorf("invalid data type for deployment delete")
		}
		_, err := tx.Exec(`DELETE FROM deployments WHERE id = ?`, id)
		return err
	}
	return nil
}

// --- Deployment Log operations ---

func (s *MemoryStore) executeDeploymentLogOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		log, ok := op.Data.(*DeploymentLog)
		if !ok {
			return fmt.Errorf("invalid data type for deployment_log insert")
		}
		_, err := tx.Exec(`
			INSERT INTO deployment_logs (id, deployment_id, level, message, source, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, log.ID, log.DeploymentID, log.Level, log.Message, log.Source, log.CreatedAt)
		return err

	case WriteOpDelete:
		deploymentID, ok := op.Data.(string)
		if !ok {
			return fmt.Errorf("invalid data type for deployment_log delete")
		}
		_, err := tx.Exec(`DELETE FROM deployment_logs WHERE deployment_id = ?`, deploymentID)
		return err
	}
	return nil
}

// --- Deployment Rollback operations ---

func (s *MemoryStore) executeDeploymentRollbackOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		r, ok := op.Data.(*DeploymentRollback)
		if !ok {
			return fmt.Errorf("invalid data type for deployment_rollback insert")
		}
		_, err := tx.Exec(`
			INSERT INTO deployment_rollbacks (id, deployment_id, project_name, from_release, to_release, reason, triggered_by, health_check_failed, health_check_error, status, error_message, started_at, completed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, r.ID, r.DeploymentID, r.ProjectName, r.FromRelease, r.ToRelease, r.Reason, r.TriggeredBy, r.HealthCheckFailed, r.HealthCheckError, r.Status, r.ErrorMessage, r.StartedAt, r.CompletedAt)
		return err

	case WriteOpUpdate:
		r, ok := op.Data.(*DeploymentRollback)
		if !ok {
			return fmt.Errorf("invalid data type for deployment_rollback update")
		}
		_, err := tx.Exec(`
			UPDATE deployment_rollbacks SET status = ?, error_message = ?, completed_at = ?
			WHERE id = ?
		`, r.Status, r.ErrorMessage, r.CompletedAt, r.ID)
		return err
	}
	return nil
}

// --- Scheduled Deployment operations ---

func (s *MemoryStore) executeScheduledDeploymentOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		sd, ok := op.Data.(*ScheduledDeployment)
		if !ok {
			return fmt.Errorf("invalid data type for scheduled_deployment insert")
		}
		_, err := tx.Exec(`
			INSERT INTO scheduled_deployments (id, project, target, branch, scheduled_at, scheduled_by, status)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, sd.ID, sd.Project, sd.Target, sd.Branch, sd.ScheduledAt, sd.ScheduledBy, sd.Status)
		return err

	case WriteOpUpdate:
		sd, ok := op.Data.(*ScheduledDeployment)
		if !ok {
			return fmt.Errorf("invalid data type for scheduled_deployment update")
		}
		_, err := tx.Exec(`
			UPDATE scheduled_deployments SET status = ?
			WHERE id = ?
		`, sd.Status, sd.ID)
		return err
	}
	return nil
}

// --- Audit Log operations ---

func (s *MemoryStore) executeAuditLogOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		entry, ok := op.Data.(*AuditEntry)
		if !ok {
			return fmt.Errorf("invalid data type for audit_log insert")
		}
		_, err := tx.Exec(`
			INSERT INTO audit_logs (id, timestamp, source, user, action, resource, resource_id, resource_data, details, ip_address, result)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, entry.ID, entry.Timestamp, entry.Source, entry.User, entry.Action, entry.Resource, entry.ResourceID, entry.ResourceData, entry.Details, entry.IPAddress, entry.Result)
		return err

	case WriteOpDelete:
		// Delete by timestamp (for cleanup)
		ts, ok := op.Data.(string)
		if !ok {
			return fmt.Errorf("invalid data type for audit_log delete")
		}
		_, err := tx.Exec(`DELETE FROM audit_logs WHERE timestamp < ?`, ts)
		return err
	}
	return nil
}

// --- Blocked IP operations ---

func (s *MemoryStore) executeBlockedIPOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		b, ok := op.Data.(*BlockedIP)
		if !ok {
			return fmt.Errorf("invalid data type for blocked_ip insert")
		}
		_, err := tx.Exec(`
			INSERT INTO blocked_ips (id, ip_address, reason, blocked_at, expires_at, blocked_by)
			VALUES (?, ?, ?, ?, ?, ?)
		`, b.ID, b.IPAddress, b.Reason, b.BlockedAt, b.ExpiresAt, b.BlockedBy)
		return err

	case WriteOpDelete:
		ip, ok := op.Data.(string)
		if !ok {
			return fmt.Errorf("invalid data type for blocked_ip delete")
		}
		_, err := tx.Exec(`DELETE FROM blocked_ips WHERE ip_address = ?`, ip)
		return err
	}
	return nil
}

// --- Rate Limit operations ---

func (s *MemoryStore) executeRateLimitOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert, WriteOpUpdate:
		r, ok := op.Data.(*RateLimitRecord)
		if !ok {
			return fmt.Errorf("invalid data type for rate_limit")
		}
		_, err := tx.Exec(`
			INSERT INTO rate_limits (id, key, bucket, count, window_start, window_end)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(key, bucket) DO UPDATE SET
				count = excluded.count,
				window_start = excluded.window_start,
				window_end = excluded.window_end
		`, r.ID, r.Key, r.Bucket, r.Count, r.WindowStart, r.WindowEnd)
		return err

	case WriteOpDelete:
		key, ok := op.Data.(string)
		if !ok {
			return fmt.Errorf("invalid data type for rate_limit delete")
		}
		_, err := tx.Exec(`DELETE FROM rate_limits WHERE key = ?`, key)
		return err
	}
	return nil
}

// --- Provision Job operations ---

func (s *MemoryStore) executeProvisionJobOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		j, ok := op.Data.(*ProvisionJob)
		if !ok {
			return fmt.Errorf("invalid data type for provision_job insert")
		}
		_, err := tx.Exec(`
			INSERT INTO provision_jobs (id, target_host, target_port, target_user, ssh_key_id, agent_binary_id, status, stage, progress, error_message, rollback_data, started_at, completed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, j.ID, j.TargetHost, j.TargetPort, j.TargetUser, j.SSHKeyID, j.AgentBinaryID, j.Status, j.Stage, j.Progress, j.ErrorMessage, j.RollbackData, j.StartedAt, j.CompletedAt)
		return err

	case WriteOpUpdate:
		j, ok := op.Data.(*ProvisionJob)
		if !ok {
			return fmt.Errorf("invalid data type for provision_job update")
		}
		_, err := tx.Exec(`
			UPDATE provision_jobs SET status = ?, stage = ?, progress = ?, error_message = ?, rollback_data = ?, completed_at = ?
			WHERE id = ?
		`, j.Status, j.Stage, j.Progress, j.ErrorMessage, j.RollbackData, j.CompletedAt, j.ID)
		return err
	}
	return nil
}

// --- SSH Host Key operations ---

func (s *MemoryStore) executeSSHHostKeyOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		k, ok := op.Data.(*SSHHostKey)
		if !ok {
			return fmt.Errorf("invalid data type for ssh_host_key insert")
		}
		_, err := tx.Exec(`
			INSERT INTO ssh_host_keys (id, hostname, port, key_type, public_key, fingerprint, trusted, added_by, verified_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, k.ID, k.Hostname, k.Port, k.KeyType, k.PublicKey, k.Fingerprint, k.Trusted, k.AddedBy, k.VerifiedAt, k.CreatedAt, k.UpdatedAt)
		return err

	case WriteOpUpdate:
		k, ok := op.Data.(*SSHHostKey)
		if !ok {
			return fmt.Errorf("invalid data type for ssh_host_key update")
		}
		_, err := tx.Exec(`
			UPDATE ssh_host_keys SET trusted = ?, verified_at = ?, updated_at = ?
			WHERE id = ?
		`, k.Trusted, k.VerifiedAt, k.UpdatedAt, k.ID)
		return err

	case WriteOpDelete:
		id, ok := op.Data.(int64)
		if !ok {
			return fmt.Errorf("invalid data type for ssh_host_key delete")
		}
		_, err := tx.Exec(`DELETE FROM ssh_host_keys WHERE id = ?`, id)
		return err
	}
	return nil
}

// --- Jump Server operations ---

func (s *MemoryStore) executeJumpServerOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		j, ok := op.Data.(*SSHJumpServer)
		if !ok {
			return fmt.Errorf("invalid data type for jump_server insert")
		}
		_, err := tx.Exec(`
			INSERT INTO ssh_jump_servers (id, name, host, port, username, ssh_key_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, j.ID, j.Name, j.Host, j.Port, j.Username, j.SSHKeyID, j.CreatedAt)
		return err

	case WriteOpUpdate:
		j, ok := op.Data.(*SSHJumpServer)
		if !ok {
			return fmt.Errorf("invalid data type for jump_server update")
		}
		_, err := tx.Exec(`
			UPDATE ssh_jump_servers SET name = ?, host = ?, port = ?, username = ?, ssh_key_id = ?
			WHERE id = ?
		`, j.Name, j.Host, j.Port, j.Username, j.SSHKeyID, j.ID)
		return err

	case WriteOpDelete:
		id, ok := op.Data.(int64)
		if !ok {
			return fmt.Errorf("invalid data type for jump_server delete")
		}
		_, err := tx.Exec(`DELETE FROM ssh_jump_servers WHERE id = ?`, id)
		return err
	}
	return nil
}

// --- Health Check Config operations ---

func (s *MemoryStore) executeHealthCheckConfigOp(tx *sql.Tx, op WriteOp) error {
	switch op.Type {
	case WriteOpInsert:
		c, ok := op.Data.(*HealthCheckConfig)
		if !ok {
			return fmt.Errorf("invalid data type for health_check_config insert")
		}
		_, err := tx.Exec(`
			INSERT INTO health_check_configs (id, project_id, name, url, method, expected_status, timeout_seconds, 
				retries, retry_delay_seconds, headers, body, body_contains, enabled, is_global, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, c.ID, c.ProjectID, c.Name, c.URL, c.Method, c.ExpectedStatus, c.TimeoutSeconds,
			c.Retries, c.RetryDelaySeconds, c.Headers, c.Body, c.BodyContains, c.Enabled, c.IsGlobal, c.CreatedAt, c.UpdatedAt)
		return err

	case WriteOpUpdate:
		c, ok := op.Data.(*HealthCheckConfig)
		if !ok {
			return fmt.Errorf("invalid data type for health_check_config update")
		}
		_, err := tx.Exec(`
			UPDATE health_check_configs SET name = ?, url = ?, method = ?, expected_status = ?, timeout_seconds = ?,
				retries = ?, retry_delay_seconds = ?, headers = ?, body = ?, body_contains = ?, enabled = ?, updated_at = ?
			WHERE id = ?
		`, c.Name, c.URL, c.Method, c.ExpectedStatus, c.TimeoutSeconds,
			c.Retries, c.RetryDelaySeconds, c.Headers, c.Body, c.BodyContains, c.Enabled, c.UpdatedAt, c.ID)
		return err

	case WriteOpDelete:
		id, ok := op.Data.(int64)
		if !ok {
			return fmt.Errorf("invalid data type for health_check_config delete")
		}
		_, err := tx.Exec(`DELETE FROM health_check_configs WHERE id = ?`, id)
		return err
	}
	return nil
}
