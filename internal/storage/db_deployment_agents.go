// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// AssignAgentToDeployment assigns an agent to participate in a deployment.
// Returns an error if the agent is already assigned to the deployment.
func (db *DB) AssignAgentToDeployment(ctx context.Context, deploymentID, agentID string) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO deployment_agents (deployment_id, agent_id, status)
		VALUES (?, ?, ?)
	`, deploymentID, agentID, DeploymentStatusPending)
	if err != nil {
		return fmt.Errorf("assign agent to deployment: %w", err)
	}
	return nil
}

// AssignAgentsToDeployment assigns multiple agents to a deployment in a single transaction.
func (db *DB) AssignAgentsToDeployment(ctx context.Context, deploymentID string, agentIDs []string) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO deployment_agents (deployment_id, agent_id, status)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, agentID := range agentIDs {
		_, err := stmt.ExecContext(ctx, deploymentID, agentID, DeploymentStatusPending)
		if err != nil {
			return fmt.Errorf("assign agent %s: %w", agentID, err)
		}
	}

	return tx.Commit()
}

// GetDeploymentAgents returns all agents assigned to a deployment.
func (db *DB) GetDeploymentAgents(ctx context.Context, deploymentID string) ([]DeploymentAgent, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, deployment_id, agent_id, status, started_at, completed_at, error_message
		FROM deployment_agents
		WHERE deployment_id = ?
		ORDER BY id ASC
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query deployment agents: %w", err)
	}
	defer rows.Close()

	var agents []DeploymentAgent
	for rows.Next() {
		var da DeploymentAgent
		var status string
		var startedAt, completedAt sql.NullTime
		var errorMsg sql.NullString

		if err := rows.Scan(
			&da.ID,
			&da.DeploymentID,
			&da.AgentID,
			&status,
			&startedAt,
			&completedAt,
			&errorMsg,
		); err != nil {
			return nil, fmt.Errorf("scan deployment agent: %w", err)
		}

		da.Status = DeploymentStatus(status)
		if startedAt.Valid {
			da.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			da.CompletedAt = &completedAt.Time
		}
		if errorMsg.Valid {
			da.ErrorMessage = errorMsg.String
		}

		agents = append(agents, da)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deployment agents: %w", err)
	}

	return agents, nil
}

// GetAgentDeployments returns all deployments an agent is assigned to.
func (db *DB) GetAgentDeployments(ctx context.Context, agentID string) ([]DeploymentAgent, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, deployment_id, agent_id, status, started_at, completed_at, error_message
		FROM deployment_agents
		WHERE agent_id = ?
		ORDER BY id DESC
	`, agentID)
	if err != nil {
		return nil, fmt.Errorf("query agent deployments: %w", err)
	}
	defer rows.Close()

	var deployments []DeploymentAgent
	for rows.Next() {
		var da DeploymentAgent
		var status string
		var startedAt, completedAt sql.NullTime
		var errorMsg sql.NullString

		if err := rows.Scan(
			&da.ID,
			&da.DeploymentID,
			&da.AgentID,
			&status,
			&startedAt,
			&completedAt,
			&errorMsg,
		); err != nil {
			return nil, fmt.Errorf("scan deployment agent: %w", err)
		}

		da.Status = DeploymentStatus(status)
		if startedAt.Valid {
			da.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			da.CompletedAt = &completedAt.Time
		}
		if errorMsg.Valid {
			da.ErrorMessage = errorMsg.String
		}

		deployments = append(deployments, da)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent deployments: %w", err)
	}

	return deployments, nil
}

// UpdateDeploymentAgentStatus updates the status of an agent's deployment assignment.
func (db *DB) UpdateDeploymentAgentStatus(ctx context.Context, deploymentID, agentID string, status DeploymentStatus, errorMsg string) error {
	var completedAt interface{}
	if status.IsTerminal() {
		completedAt = time.Now()
	}

	result, err := db.conn.ExecContext(ctx, `
		UPDATE deployment_agents
		SET status = ?, error_message = ?, completed_at = ?
		WHERE deployment_id = ? AND agent_id = ?
	`, status, errorMsg, completedAt, deploymentID, agentID)
	if err != nil {
		return fmt.Errorf("update deployment agent status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("agent %s not assigned to deployment %s", agentID, deploymentID)
	}

	return nil
}

// StartDeploymentAgent marks an agent as having started deployment execution.
func (db *DB) StartDeploymentAgent(ctx context.Context, deploymentID, agentID string) error {
	result, err := db.conn.ExecContext(ctx, `
		UPDATE deployment_agents
		SET status = ?, started_at = ?
		WHERE deployment_id = ? AND agent_id = ? AND status = ?
	`, DeploymentStatusRunning, time.Now(), deploymentID, agentID, DeploymentStatusPending)
	if err != nil {
		return fmt.Errorf("start deployment agent: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("agent %s not in pending state for deployment %s", agentID, deploymentID)
	}

	return nil
}

// IsAgentAssignedToDeployment checks if an agent is assigned to a deployment.
func (db *DB) IsAgentAssignedToDeployment(ctx context.Context, deploymentID, agentID string) (bool, error) {
	var count int
	err := db.conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM deployment_agents
		WHERE deployment_id = ? AND agent_id = ?
	`, deploymentID, agentID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check agent assignment: %w", err)
	}
	return count > 0, nil
}

// GetDeploymentAgentStatus gets the status of a specific agent in a deployment.
func (db *DB) GetDeploymentAgentStatus(ctx context.Context, deploymentID, agentID string) (*DeploymentAgent, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, deployment_id, agent_id, status, started_at, completed_at, error_message
		FROM deployment_agents
		WHERE deployment_id = ? AND agent_id = ?
	`, deploymentID, agentID)

	var da DeploymentAgent
	var status string
	var startedAt, completedAt sql.NullTime
	var errorMsg sql.NullString

	err := row.Scan(
		&da.ID,
		&da.DeploymentID,
		&da.AgentID,
		&status,
		&startedAt,
		&completedAt,
		&errorMsg,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan deployment agent: %w", err)
	}

	da.Status = DeploymentStatus(status)
	if startedAt.Valid {
		da.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		da.CompletedAt = &completedAt.Time
	}
	if errorMsg.Valid {
		da.ErrorMessage = errorMsg.String
	}

	return &da, nil
}

// CountDeploymentAgentsByStatus returns the count of agents in each status for a deployment.
func (db *DB) CountDeploymentAgentsByStatus(ctx context.Context, deploymentID string) (map[DeploymentStatus]int, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT status, COUNT(*) 
		FROM deployment_agents
		WHERE deployment_id = ?
		GROUP BY status
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query deployment agent counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[DeploymentStatus]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan count: %w", err)
		}
		counts[DeploymentStatus(status)] = count
	}

	return counts, rows.Err()
}

// RemoveAgentFromDeployment removes an agent's assignment from a deployment.
// This should only be used for pending deployments before execution starts.
func (db *DB) RemoveAgentFromDeployment(ctx context.Context, deploymentID, agentID string) error {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM deployment_agents
		WHERE deployment_id = ? AND agent_id = ? AND status = ?
	`, deploymentID, agentID, DeploymentStatusPending)
	if err != nil {
		return fmt.Errorf("remove agent from deployment: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("agent %s not found or not in pending state for deployment %s", agentID, deploymentID)
	}

	return nil
}
