// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// --- Agent operations ---

// UpsertAgent creates or updates an agent.
func (db *DB) UpsertAgent(ctx context.Context, agent *Agent) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO agents (id, hostname, labels, capabilities, status, last_seen_at, certificate,
			version, os, arch, update_policy, update_window_start, update_window_end, last_update_at, last_update_error, hmac_secret)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			hostname = excluded.hostname,
			labels = excluded.labels,
			capabilities = excluded.capabilities,
			status = excluded.status,
			last_seen_at = excluded.last_seen_at,
			certificate = COALESCE(excluded.certificate, agents.certificate),
			version = COALESCE(NULLIF(excluded.version, ''), agents.version),
			os = COALESCE(NULLIF(excluded.os, ''), agents.os),
			arch = COALESCE(NULLIF(excluded.arch, ''), agents.arch),
			update_policy = COALESCE(NULLIF(excluded.update_policy, ''), agents.update_policy),
			update_window_start = COALESCE(excluded.update_window_start, agents.update_window_start),
			update_window_end = COALESCE(excluded.update_window_end, agents.update_window_end),
			last_update_at = COALESCE(excluded.last_update_at, agents.last_update_at),
			last_update_error = COALESCE(excluded.last_update_error, agents.last_update_error),
			hmac_secret = COALESCE(excluded.hmac_secret, agents.hmac_secret)
	`, agent.ID, agent.Hostname, mapToJSON(agent.Labels), agent.Capabilities,
		agent.Status, agent.LastSeenAt, agent.Certificate,
		agent.Version, agent.OS, agent.Arch, agent.UpdatePolicy,
		agent.UpdateWindowStart, agent.UpdateWindowEnd, agent.LastUpdateAt, agent.LastUpdateError,
		agent.HMACSecret)
	if err != nil {
		return fmt.Errorf("upserting agent: %w", err)
	}
	return nil
}

// GetAgent retrieves an agent by ID.
func (db *DB) GetAgent(ctx context.Context, id string) (*Agent, error) {
	var agent Agent
	var labels, capabilities sql.NullString
	var lastSeen, lastUpdateAt sql.NullTime
	var version, os, arch, updatePolicy, windowStart, windowEnd, lastUpdateError sql.NullString
	var hmacSecret []byte

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, hostname, labels, capabilities, status, last_seen_at, registered_at, certificate,
			COALESCE(version, ''), COALESCE(os, ''), COALESCE(arch, ''),
			COALESCE(update_policy, 'immediate'), COALESCE(update_window_start, ''), COALESCE(update_window_end, ''),
			last_update_at, COALESCE(last_update_error, ''), hmac_secret
		FROM agents WHERE id = ?
	`, id).Scan(
		&agent.ID, &agent.Hostname, &labels, &capabilities,
		&agent.Status, &lastSeen, &agent.RegisteredAt, &agent.Certificate,
		&version, &os, &arch, &updatePolicy, &windowStart, &windowEnd, &lastUpdateAt, &lastUpdateError,
		&hmacSecret,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying agent: %w", err)
	}

	agent.Labels = jsonToMap(labels.String)
	agent.LastSeenAt = lastSeen.Time
	agent.Version = version.String
	agent.OS = os.String
	agent.Arch = arch.String
	agent.UpdatePolicy = updatePolicy.String
	agent.UpdateWindowStart = windowStart.String
	agent.UpdateWindowEnd = windowEnd.String
	if lastUpdateAt.Valid {
		agent.LastUpdateAt = &lastUpdateAt.Time
	}
	agent.LastUpdateError = lastUpdateError.String
	agent.HMACSecret = hmacSecret
	return &agent, nil
}

// ListAgents returns all agents.
func (db *DB) ListAgents(ctx context.Context) ([]*Agent, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, hostname, labels, capabilities, status, last_seen_at, registered_at,
			COALESCE(version, ''), COALESCE(os, ''), COALESCE(arch, ''),
			COALESCE(update_policy, 'immediate'), COALESCE(update_window_start, ''), COALESCE(update_window_end, ''),
			last_update_at, COALESCE(last_update_error, '')
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
		var lastSeen, lastUpdateAt sql.NullTime
		var version, os, arch, updatePolicy, windowStart, windowEnd, lastUpdateError sql.NullString

		if err := rows.Scan(&agent.ID, &agent.Hostname, &labels, &capabilities,
			&agent.Status, &lastSeen, &agent.RegisteredAt,
			&version, &os, &arch, &updatePolicy, &windowStart, &windowEnd, &lastUpdateAt, &lastUpdateError); err != nil {
			return nil, fmt.Errorf("scanning agent: %w", err)
		}

		agent.Labels = jsonToMap(labels.String)
		agent.LastSeenAt = lastSeen.Time
		agent.Version = version.String
		agent.OS = os.String
		agent.Arch = arch.String
		agent.UpdatePolicy = updatePolicy.String
		agent.UpdateWindowStart = windowStart.String
		agent.UpdateWindowEnd = windowEnd.String
		if lastUpdateAt.Valid {
			agent.LastUpdateAt = &lastUpdateAt.Time
		}
		agent.LastUpdateError = lastUpdateError.String
		agents = append(agents, &agent)
	}

	return agents, rows.Err()
}

// ListAgentsPaginated returns agents with pagination support.
func (db *DB) ListAgentsPaginated(ctx context.Context, limit, offset int) ([]*Agent, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, hostname, labels, capabilities, status, last_seen_at, registered_at,
			COALESCE(version, ''), COALESCE(os, ''), COALESCE(arch, ''),
			COALESCE(update_policy, 'immediate'), COALESCE(update_window_start, ''), COALESCE(update_window_end, ''),
			last_update_at, COALESCE(last_update_error, '')
		FROM agents ORDER BY hostname LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying agents: %w", err)
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		var agent Agent
		var labels, capabilities sql.NullString
		var lastSeen, lastUpdateAt sql.NullTime
		var version, os, arch, updatePolicy, windowStart, windowEnd, lastUpdateError sql.NullString

		if err := rows.Scan(&agent.ID, &agent.Hostname, &labels, &capabilities,
			&agent.Status, &lastSeen, &agent.RegisteredAt,
			&version, &os, &arch, &updatePolicy, &windowStart, &windowEnd, &lastUpdateAt, &lastUpdateError); err != nil {
			return nil, fmt.Errorf("scanning agent: %w", err)
		}

		agent.Labels = jsonToMap(labels.String)
		agent.LastSeenAt = lastSeen.Time
		agent.Version = version.String
		agent.OS = os.String
		agent.Arch = arch.String
		agent.UpdatePolicy = updatePolicy.String
		agent.UpdateWindowStart = windowStart.String
		agent.UpdateWindowEnd = windowEnd.String
		if lastUpdateAt.Valid {
			agent.LastUpdateAt = &lastUpdateAt.Time
		}
		agent.LastUpdateError = lastUpdateError.String
		agents = append(agents, &agent)
	}

	return agents, rows.Err()
}

// CountAgents returns the total number of agents.
func (db *DB) CountAgents(ctx context.Context) (int64, error) {
	var count int64
	err := db.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agents`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting agents: %w", err)
	}
	return count, nil
}

// CountAgentsByStatus returns agent counts grouped by status.
func (db *DB) CountAgentsByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := db.conn.QueryContext(ctx, `SELECT COALESCE(status, 'unknown'), COUNT(*) FROM agents GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("counting agents by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scanning agent count: %w", err)
		}
		counts[status] = count
	}
	return counts, rows.Err()
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


// --- Agent Binary operations ---

// CreateAgentBinary creates a new agent binary record.
func (db *DB) CreateAgentBinary(ctx context.Context, binary *AgentBinary) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO agent_binaries (version, os, arch, path, checksum_sha256, size_bytes, uploaded_at, is_current)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, binary.Version, binary.OS, binary.Arch, binary.Path, binary.ChecksumSHA256, binary.SizeBytes, binary.UploadedAt, binary.IsCurrent)
	if err != nil {
		return fmt.Errorf("creating agent binary: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting last insert id: %w", err)
	}
	binary.ID = id
	return nil
}

// GetAgentBinary retrieves an agent binary by ID.
func (db *DB) GetAgentBinary(ctx context.Context, id int64) (*AgentBinary, error) {
	var binary AgentBinary
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, version, os, arch, path, checksum_sha256, size_bytes, uploaded_at, is_current
		FROM agent_binaries WHERE id = ?
	`, id).Scan(&binary.ID, &binary.Version, &binary.OS, &binary.Arch, &binary.Path,
		&binary.ChecksumSHA256, &binary.SizeBytes, &binary.UploadedAt, &binary.IsCurrent)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting agent binary: %w", err)
	}
	return &binary, nil
}

// GetAgentBinaryByVersion retrieves an agent binary by version, OS, and arch.
func (db *DB) GetAgentBinaryByVersion(ctx context.Context, version, os, arch string) (*AgentBinary, error) {
	var binary AgentBinary
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, version, os, arch, path, checksum_sha256, size_bytes, uploaded_at, is_current
		FROM agent_binaries WHERE version = ? AND os = ? AND arch = ?
	`, version, os, arch).Scan(&binary.ID, &binary.Version, &binary.OS, &binary.Arch, &binary.Path,
		&binary.ChecksumSHA256, &binary.SizeBytes, &binary.UploadedAt, &binary.IsCurrent)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting agent binary by version: %w", err)
	}
	return &binary, nil
}

// GetCurrentAgentBinary retrieves the current agent binary for a given OS and arch.
func (db *DB) GetCurrentAgentBinary(ctx context.Context, os, arch string) (*AgentBinary, error) {
	var binary AgentBinary
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, version, os, arch, path, checksum_sha256, size_bytes, uploaded_at, is_current
		FROM agent_binaries WHERE os = ? AND arch = ? AND is_current = 1
	`, os, arch).Scan(&binary.ID, &binary.Version, &binary.OS, &binary.Arch, &binary.Path,
		&binary.ChecksumSHA256, &binary.SizeBytes, &binary.UploadedAt, &binary.IsCurrent)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting current agent binary: %w", err)
	}
	return &binary, nil
}

// ListAgentBinaries returns all agent binaries.
func (db *DB) ListAgentBinaries(ctx context.Context) ([]*AgentBinary, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, version, os, arch, path, checksum_sha256, size_bytes, uploaded_at, is_current
		FROM agent_binaries ORDER BY uploaded_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing agent binaries: %w", err)
	}
	defer rows.Close()

	var binaries []*AgentBinary
	for rows.Next() {
		var binary AgentBinary
		if err := rows.Scan(&binary.ID, &binary.Version, &binary.OS, &binary.Arch, &binary.Path,
			&binary.ChecksumSHA256, &binary.SizeBytes, &binary.UploadedAt, &binary.IsCurrent); err != nil {
			return nil, fmt.Errorf("scanning agent binary: %w", err)
		}
		binaries = append(binaries, &binary)
	}
	return binaries, rows.Err()
}

// SetCurrentAgentBinary sets a binary as current and unsets all others for the same OS/arch.
func (db *DB) SetCurrentAgentBinary(ctx context.Context, id int64) error {
	// Get the binary to find its OS and arch
	binary, err := db.GetAgentBinary(ctx, id)
	if err != nil {
		return fmt.Errorf("getting agent binary: %w", err)
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Unset current for all binaries with same OS/arch
	_, err = tx.ExecContext(ctx, `
		UPDATE agent_binaries SET is_current = 0 WHERE os = ? AND arch = ?
	`, binary.OS, binary.Arch)
	if err != nil {
		return fmt.Errorf("unsetting current binaries: %w", err)
	}

	// Set this one as current
	_, err = tx.ExecContext(ctx, `
		UPDATE agent_binaries SET is_current = 1 WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("setting current binary: %w", err)
	}

	return tx.Commit()
}

// DeleteAgentBinary deletes an agent binary by ID.
func (db *DB) DeleteAgentBinary(ctx context.Context, id int64) error {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM agent_binaries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting agent binary: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}


// --- Agent Update History operations ---

// CreateAgentUpdateHistory creates a new agent update history record.
func (db *DB) CreateAgentUpdateHistory(ctx context.Context, history *AgentUpdateHistory) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO agent_update_history (agent_id, from_version, to_version, status, error_message, started_at, completed_at, rolled_back)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, history.AgentID, history.FromVersion, history.ToVersion, history.Status, history.ErrorMessage, history.StartedAt, history.CompletedAt, history.RolledBack)
	if err != nil {
		return fmt.Errorf("creating agent update history: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting last insert id: %w", err)
	}
	history.ID = id
	return nil
}

// GetAgentUpdateHistory retrieves an agent update history record by ID.
func (db *DB) GetAgentUpdateHistory(ctx context.Context, id int64) (*AgentUpdateHistory, error) {
	var history AgentUpdateHistory
	var completedAt sql.NullTime
	var errorMsg sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, agent_id, from_version, to_version, status, error_message, started_at, completed_at, rolled_back
		FROM agent_update_history WHERE id = ?
	`, id).Scan(&history.ID, &history.AgentID, &history.FromVersion, &history.ToVersion,
		&history.Status, &errorMsg, &history.StartedAt, &completedAt, &history.RolledBack)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting agent update history: %w", err)
	}
	if errorMsg.Valid {
		history.ErrorMessage = errorMsg.String
	}
	if completedAt.Valid {
		history.CompletedAt = &completedAt.Time
	}
	return &history, nil
}

// UpdateAgentUpdateHistory updates an agent update history record.
func (db *DB) UpdateAgentUpdateHistory(ctx context.Context, history *AgentUpdateHistory) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE agent_update_history 
		SET status = ?, error_message = ?, completed_at = ?, rolled_back = ?
		WHERE id = ?
	`, history.Status, history.ErrorMessage, history.CompletedAt, history.RolledBack, history.ID)
	if err != nil {
		return fmt.Errorf("updating agent update history: %w", err)
	}
	return nil
}

// ListAgentUpdateHistory returns update history for an agent.
func (db *DB) ListAgentUpdateHistory(ctx context.Context, agentID string, limit, offset int) ([]*AgentUpdateHistory, int64, error) {
	// Get total count
	var total int64
	err := db.conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_update_history WHERE agent_id = ?
	`, agentID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting update history: %w", err)
	}

	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, agent_id, from_version, to_version, status, error_message, started_at, completed_at, rolled_back
		FROM agent_update_history 
		WHERE agent_id = ?
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?
	`, agentID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing agent update history: %w", err)
	}
	defer rows.Close()

	var histories []*AgentUpdateHistory
	for rows.Next() {
		var history AgentUpdateHistory
		var completedAt sql.NullTime
		var errorMsg sql.NullString

		if err := rows.Scan(&history.ID, &history.AgentID, &history.FromVersion, &history.ToVersion,
			&history.Status, &errorMsg, &history.StartedAt, &completedAt, &history.RolledBack); err != nil {
			return nil, 0, fmt.Errorf("scanning agent update history: %w", err)
		}
		if errorMsg.Valid {
			history.ErrorMessage = errorMsg.String
		}
		if completedAt.Valid {
			history.CompletedAt = &completedAt.Time
		}
		histories = append(histories, &history)
	}
	return histories, total, rows.Err()
}

// ListAllAgentUpdateHistory returns all update history across all agents with pagination.
func (db *DB) ListAllAgentUpdateHistory(ctx context.Context, limit, offset int) ([]*AgentUpdateHistory, int64, error) {
	// Get total count
	var total int64
	err := db.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_update_history`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting all update history: %w", err)
	}

	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, agent_id, from_version, to_version, status, error_message, started_at, completed_at, rolled_back
		FROM agent_update_history 
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing all agent update history: %w", err)
	}
	defer rows.Close()

	var histories []*AgentUpdateHistory
	for rows.Next() {
		var history AgentUpdateHistory
		var completedAt sql.NullTime
		var errorMsg sql.NullString

		if err := rows.Scan(&history.ID, &history.AgentID, &history.FromVersion, &history.ToVersion,
			&history.Status, &errorMsg, &history.StartedAt, &completedAt, &history.RolledBack); err != nil {
			return nil, 0, fmt.Errorf("scanning agent update history: %w", err)
		}
		if errorMsg.Valid {
			history.ErrorMessage = errorMsg.String
		}
		if completedAt.Valid {
			history.CompletedAt = &completedAt.Time
		}
		histories = append(histories, &history)
	}
	return histories, total, rows.Err()
}

// GetLatestAgentUpdateHistory returns the most recent update history for an agent.
func (db *DB) GetLatestAgentUpdateHistory(ctx context.Context, agentID string) (*AgentUpdateHistory, error) {
	var history AgentUpdateHistory
	var completedAt sql.NullTime
	var errorMsg sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, agent_id, from_version, to_version, status, error_message, started_at, completed_at, rolled_back
		FROM agent_update_history 
		WHERE agent_id = ?
		ORDER BY started_at DESC
		LIMIT 1
	`, agentID).Scan(&history.ID, &history.AgentID, &history.FromVersion, &history.ToVersion,
		&history.Status, &errorMsg, &history.StartedAt, &completedAt, &history.RolledBack)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting latest agent update history: %w", err)
	}
	if errorMsg.Valid {
		history.ErrorMessage = errorMsg.String
	}
	if completedAt.Valid {
		history.CompletedAt = &completedAt.Time
	}
	return &history, nil
}

// UpdateAgentVersion updates the agent's version and last update time.
func (db *DB) UpdateAgentVersion(ctx context.Context, agentID, version string) error {
	now := time.Now()
	_, err := db.conn.ExecContext(ctx, `
		UPDATE agents SET version = ?, last_update_at = ?, last_update_error = '' WHERE id = ?
	`, version, now, agentID)
	if err != nil {
		return fmt.Errorf("updating agent version: %w", err)
	}
	return nil
}

// UpdateAgentUpdateError sets the last update error for an agent.
func (db *DB) UpdateAgentUpdateError(ctx context.Context, agentID, errorMsg string) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE agents SET last_update_error = ? WHERE id = ?
	`, errorMsg, agentID)
	if err != nil {
		return fmt.Errorf("updating agent update error: %w", err)
	}
	return nil
}

// UpdateAgentUpdatePolicy updates the update configuration for an agent.
func (db *DB) UpdateAgentUpdatePolicy(ctx context.Context, agentID, policy, windowStart, windowEnd string) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE agents SET update_policy = ?, update_window_start = ?, update_window_end = ? WHERE id = ?
	`, policy, windowStart, windowEnd, agentID)
	if err != nil {
		return fmt.Errorf("updating agent update policy: %w", err)
	}
	return nil
}

// ListAgentsNeedingUpdate returns agents with a version older than the current binary.
func (db *DB) ListAgentsNeedingUpdate(ctx context.Context) ([]*Agent, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT a.id, a.hostname, a.labels, a.capabilities, a.status, a.last_seen_at, a.registered_at,
			COALESCE(a.version, ''), COALESCE(a.os, ''), COALESCE(a.arch, ''),
			COALESCE(a.update_policy, 'immediate'), COALESCE(a.update_window_start, ''), COALESCE(a.update_window_end, ''),
			a.last_update_at, COALESCE(a.last_update_error, '')
		FROM agents a
		JOIN agent_binaries b ON b.os = a.os AND b.arch = a.arch AND b.is_current = 1
		WHERE a.version != b.version AND a.version != '' AND a.os != '' AND a.arch != ''
		ORDER BY a.hostname
	`)
	if err != nil {
		return nil, fmt.Errorf("listing agents needing update: %w", err)
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		var agent Agent
		var labels, capabilities sql.NullString
		var lastSeen, lastUpdateAt sql.NullTime
		var version, os, arch, updatePolicy, windowStart, windowEnd, lastUpdateError sql.NullString

		if err := rows.Scan(&agent.ID, &agent.Hostname, &labels, &capabilities,
			&agent.Status, &lastSeen, &agent.RegisteredAt,
			&version, &os, &arch, &updatePolicy, &windowStart, &windowEnd, &lastUpdateAt, &lastUpdateError); err != nil {
			return nil, fmt.Errorf("scanning agent: %w", err)
		}

		agent.Labels = jsonToMap(labels.String)
		agent.LastSeenAt = lastSeen.Time
		agent.Version = version.String
		agent.OS = os.String
		agent.Arch = arch.String
		agent.UpdatePolicy = updatePolicy.String
		agent.UpdateWindowStart = windowStart.String
		agent.UpdateWindowEnd = windowEnd.String
		if lastUpdateAt.Valid {
			agent.LastUpdateAt = &lastUpdateAt.Time
		}
		agent.LastUpdateError = lastUpdateError.String
		agents = append(agents, &agent)
	}
	return agents, rows.Err()
}


