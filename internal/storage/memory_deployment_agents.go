package storage

import (
	"context"
	"fmt"
	"github.com/rs/xid"
	"time"
)

// AssignAgentToDeployment assigns an agent to participate in a deployment.
func (m *MemoryStore) AssignAgentToDeployment(ctx context.Context, deploymentID, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicates
	for _, da := range m.deploymentAgents[deploymentID] {
		if da.AgentID == agentID {
			return fmt.Errorf("agent %s already assigned to deployment %s", agentID, deploymentID)
		}
	}

	m.deploymentAgents[deploymentID] = append(m.deploymentAgents[deploymentID], DeploymentAgent{
		ID:           xid.New().String(),
		DeploymentID: deploymentID,
		AgentID:      agentID,
		Status:       DeploymentStatusPending,
	})

	return nil
}

// AssignAgentsToDeployment assigns multiple agents to a deployment.
func (m *MemoryStore) AssignAgentsToDeployment(ctx context.Context, deploymentID string, agentIDs []string) error {
	for _, agentID := range agentIDs {
		if err := m.AssignAgentToDeployment(ctx, deploymentID, agentID); err != nil {
			return err
		}
	}
	return nil
}

// GetDeploymentAgents returns all agents assigned to a deployment.
func (m *MemoryStore) GetDeploymentAgents(ctx context.Context, deploymentID string) ([]DeploymentAgent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := m.deploymentAgents[deploymentID]
	if agents == nil {
		return []DeploymentAgent{}, nil
	}

	// Return a copy
	result := make([]DeploymentAgent, len(agents))
	copy(result, agents)
	return result, nil
}

// GetAgentDeployments returns all deployments an agent is assigned to.
func (m *MemoryStore) GetAgentDeployments(ctx context.Context, agentID string) ([]DeploymentAgent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []DeploymentAgent
	for _, agents := range m.deploymentAgents {
		for _, da := range agents {
			if da.AgentID == agentID {
				result = append(result, da)
			}
		}
	}
	return result, nil
}

// UpdateDeploymentAgentStatus updates the status of an agent's deployment assignment.
func (m *MemoryStore) UpdateDeploymentAgentStatus(ctx context.Context, deploymentID, agentID string, status DeploymentStatus, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agents := m.deploymentAgents[deploymentID]
	for i := range agents {
		if agents[i].AgentID == agentID {
			agents[i].Status = status
			agents[i].ErrorMessage = errorMsg
			if status.IsTerminal() {
				now := time.Now()
				agents[i].CompletedAt = &now
			}
			return nil
		}
	}
	return fmt.Errorf("agent %s not assigned to deployment %s", agentID, deploymentID)
}

// StartDeploymentAgent marks an agent as having started deployment execution.
func (m *MemoryStore) StartDeploymentAgent(ctx context.Context, deploymentID, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agents := m.deploymentAgents[deploymentID]
	for i := range agents {
		if agents[i].AgentID != agentID {
			continue
		}
		if agents[i].Status != DeploymentStatusPending {
			return fmt.Errorf("agent %s not in pending state for deployment %s", agentID, deploymentID)
		}
		agents[i].Status = DeploymentStatusRunning
		now := time.Now()
		agents[i].StartedAt = &now
		return nil
	}
	return fmt.Errorf("agent %s not assigned to deployment %s", agentID, deploymentID)
}

// IsAgentAssignedToDeployment checks if an agent is assigned to a deployment.
func (m *MemoryStore) IsAgentAssignedToDeployment(ctx context.Context, deploymentID, agentID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, da := range m.deploymentAgents[deploymentID] {
		if da.AgentID == agentID {
			return true, nil
		}
	}
	return false, nil
}

// GetDeploymentAgentStatus gets the status of a specific agent in a deployment.
func (m *MemoryStore) GetDeploymentAgentStatus(ctx context.Context, deploymentID, agentID string) (*DeploymentAgent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, da := range m.deploymentAgents[deploymentID] {
		if da.AgentID == agentID {
			daCopy := da
			return &daCopy, nil
		}
	}
	return nil, nil
}

// CountDeploymentAgentsByStatus returns the count of agents in each status for a deployment.
func (m *MemoryStore) CountDeploymentAgentsByStatus(ctx context.Context, deploymentID string) (map[DeploymentStatus]int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	counts := make(map[DeploymentStatus]int)
	for _, da := range m.deploymentAgents[deploymentID] {
		counts[da.Status]++
	}
	return counts, nil
}

// RemoveAgentFromDeployment removes an agent's assignment from a deployment.
func (m *MemoryStore) RemoveAgentFromDeployment(ctx context.Context, deploymentID, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agents := m.deploymentAgents[deploymentID]
	for i := range agents {
		if agents[i].AgentID == agentID {
			if agents[i].Status != DeploymentStatusPending {
				return fmt.Errorf("agent %s not in pending state for deployment %s", agentID, deploymentID)
			}
			// Remove from slice
			m.deploymentAgents[deploymentID] = append(agents[:i], agents[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("agent %s not found for deployment %s", agentID, deploymentID)
}
