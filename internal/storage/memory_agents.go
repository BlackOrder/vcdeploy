package storage

import (
	"context"
	"strings"
	"time"
)

// --- Agent methods ---

// UpsertAgent creates or updates an agent.
func (s *MemoryStore) UpsertAgent(ctx context.Context, agent *Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Check if agent exists
	existing, exists := s.agents[agent.ID]
	if exists {
		// Update existing agent
		existing.Hostname = agent.Hostname
		existing.Labels = agent.Labels
		existing.Capabilities = agent.Capabilities
		existing.Status = agent.Status
		existing.LastSeenAt = now
		existing.Certificate = agent.Certificate
		existing.Version = agent.Version
		existing.OS = agent.OS
		existing.Arch = agent.Arch
		// Don't overwrite update policy fields on upsert unless explicitly set
		if agent.UpdatePolicy != "" {
			existing.UpdatePolicy = agent.UpdatePolicy
		}
		if agent.UpdateWindowStart != "" {
			existing.UpdateWindowStart = agent.UpdateWindowStart
		}
		if agent.UpdateWindowEnd != "" {
			existing.UpdateWindowEnd = agent.UpdateWindowEnd
		}

		s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpUpdate, "agents", existing))
		return nil
	}

	// Create new agent
	agent.RegisteredAt = now
	agent.LastSeenAt = now
	if agent.Status == "" {
		agent.Status = "online"
	}
	if agent.UpdatePolicy == "" {
		agent.UpdatePolicy = AgentUpdatePolicyImmediate
	}

	// Copy-on-store
	stored := *agent
	if agent.Labels != nil {
		stored.Labels = make(map[string]string, len(agent.Labels))
		for k, v := range agent.Labels {
			stored.Labels[k] = v
		}
	}
	s.agents[agent.ID] = &stored

	s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpInsert, "agents", &stored))
	return nil
}

// GetAgent retrieves an agent by ID.
func (s *MemoryStore) GetAgent(ctx context.Context, id string) (*Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agent, ok := s.agents[id]
	if !ok {
		return nil, ErrNotFound
	}

	// Copy-on-read
	result := *agent
	if agent.Labels != nil {
		result.Labels = make(map[string]string, len(agent.Labels))
		for k, v := range agent.Labels {
			result.Labels[k] = v
		}
	}
	return &result, nil
}

// ListAgents returns all agents.
func (s *MemoryStore) ListAgents(ctx context.Context) ([]*Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agents := make([]*Agent, 0, len(s.agents))
	for _, a := range s.agents {
		// Copy-on-read
		cp := *a
		if a.Labels != nil {
			cp.Labels = make(map[string]string, len(a.Labels))
			for k, v := range a.Labels {
				cp.Labels[k] = v
			}
		}
		agents = append(agents, &cp)
	}
	return agents, nil
}

// ListAgentsPaginated returns agents with pagination support.
func (s *MemoryStore) ListAgentsPaginated(ctx context.Context, limit, offset int) ([]*Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Convert map to slice
	all := make([]*Agent, 0, len(s.agents))
	for _, a := range s.agents {
		// Copy-on-read
		cp := *a
		if a.Labels != nil {
			cp.Labels = make(map[string]string, len(a.Labels))
			for k, v := range a.Labels {
				cp.Labels[k] = v
			}
		}
		all = append(all, &cp)
	}

	// Apply pagination
	if offset >= len(all) {
		return []*Agent{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	return all[offset:end], nil
}

// CountAgents returns the total number of agents.
func (s *MemoryStore) CountAgents(ctx context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return int64(len(s.agents)), nil
}

// CountAgentsByStatus returns a map of status -> count.
func (s *MemoryStore) CountAgentsByStatus(ctx context.Context) (map[string]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := make(map[string]int64)
	for _, a := range s.agents {
		counts[a.Status]++
	}
	return counts, nil
}

// DeleteAgent removes an agent by ID.
func (s *MemoryStore) DeleteAgent(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.agents[id]; !ok {
		return ErrNotFound
	}

	delete(s.agents, id)

	// Cascade delete update history
	for histID, hist := range s.agentUpdates {
		if hist.AgentID == id {
			delete(s.agentUpdates, histID)
		}
	}

	s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpDelete, "agents", id))
	return nil
}

// UpdateAgentVersion updates the version for an agent.
func (s *MemoryStore) UpdateAgentVersion(ctx context.Context, agentID, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, ok := s.agents[agentID]
	if !ok {
		return ErrNotFound
	}

	agent.Version = version
	now := time.Now()
	agent.LastUpdateAt = &now
	agent.LastUpdateError = ""

	s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpUpdate, "agents", agent))
	return nil
}

// UpdateAgentUpdateError updates the last update error for an agent.
func (s *MemoryStore) UpdateAgentUpdateError(ctx context.Context, agentID, errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, ok := s.agents[agentID]
	if !ok {
		return ErrNotFound
	}

	agent.LastUpdateError = errorMsg

	s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpUpdate, "agents", agent))
	return nil
}

// UpdateAgentUpdatePolicy updates the update policy for an agent.
func (s *MemoryStore) UpdateAgentUpdatePolicy(ctx context.Context, agentID, policy, windowStart, windowEnd string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, ok := s.agents[agentID]
	if !ok {
		return ErrNotFound
	}

	agent.UpdatePolicy = policy
	agent.UpdateWindowStart = windowStart
	agent.UpdateWindowEnd = windowEnd

	s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpUpdate, "agents", agent))
	return nil
}

// ListAgentsNeedingUpdate returns agents that need an update.
// An agent needs update if:
// - UpdatePolicy is "immediate" OR
// - UpdatePolicy is "scheduled" and current time is within update window
func (s *MemoryStore) ListAgentsNeedingUpdate(ctx context.Context) ([]*Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var agents []*Agent
	now := time.Now()

	for _, a := range s.agents {
		if a.UpdatePolicy == AgentUpdatePolicyManual {
			continue
		}

		if a.UpdatePolicy == AgentUpdatePolicyImmediate {
			cp := *a
			agents = append(agents, &cp)
			continue
		}

		// Scheduled - check if in window
		if a.UpdatePolicy == AgentUpdatePolicyScheduled {
			if isInUpdateWindow(now, a.UpdateWindowStart, a.UpdateWindowEnd) {
				cp := *a
				agents = append(agents, &cp)
			}
		}
	}
	return agents, nil
}

// isInUpdateWindow checks if the current time is within the update window.
func isInUpdateWindow(now time.Time, windowStart, windowEnd string) bool {
	if windowStart == "" || windowEnd == "" {
		return false
	}

	startParts := strings.Split(windowStart, ":")
	endParts := strings.Split(windowEnd, ":")
	if len(startParts) != 2 || len(endParts) != 2 {
		return false
	}

	nowMinutes := now.Hour()*60 + now.Minute()

	var startHour, startMin, endHour, endMin int
	if ok, _ := parseTimeComponent(startParts[0], &startHour); !ok {
		return false // Invalid start hour format
	}
	if ok, _ := parseTimeComponent(startParts[1], &startMin); !ok {
		return false // Invalid start minute format
	}
	if ok, _ := parseTimeComponent(endParts[0], &endHour); !ok {
		return false // Invalid end hour format
	}
	if ok, _ := parseTimeComponent(endParts[1], &endMin); !ok {
		return false // Invalid end minute format
	}

	startMinutes := startHour*60 + startMin
	endMinutes := endHour*60 + endMin

	// Handle overnight windows (e.g., 22:00 - 06:00)
	if endMinutes < startMinutes {
		return nowMinutes >= startMinutes || nowMinutes <= endMinutes
	}
	return nowMinutes >= startMinutes && nowMinutes <= endMinutes
}

func parseTimeComponent(s string, val *int) (bool, error) {
	var v int
	for _, c := range s {
		if c < '0' || c > '9' {
			return false, nil
		}
		v = v*10 + int(c-'0')
	}
	*val = v
	return true, nil
}

// --- AgentBinary methods ---

// CreateAgentBinary creates a new agent binary record.
func (s *MemoryStore) CreateAgentBinary(ctx context.Context, binary *AgentBinary) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate version/os/arch
	for _, b := range s.agentBinaries {
		if b.Version == binary.Version && b.OS == binary.OS && b.Arch == binary.Arch {
			return ErrDuplicate
		}
	}

	binary.ID = nextID(&s.nextAgentBinaryID)
	binary.UploadedAt = time.Now()

	// Copy-on-store
	stored := *binary
	s.agentBinaries[binary.ID] = &stored

	s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpInsert, "agent_binaries", &stored))
	return nil
}

// GetAgentBinary retrieves an agent binary by ID.
func (s *MemoryStore) GetAgentBinary(ctx context.Context, id int64) (*AgentBinary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	binary, ok := s.agentBinaries[id]
	if !ok {
		return nil, ErrNotFound
	}

	result := *binary
	return &result, nil
}

// GetAgentBinaryByVersion retrieves an agent binary by version, OS, and arch.
func (s *MemoryStore) GetAgentBinaryByVersion(ctx context.Context, version, os, arch string) (*AgentBinary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, b := range s.agentBinaries {
		if b.Version == version && b.OS == os && b.Arch == arch {
			result := *b
			return &result, nil
		}
	}
	return nil, ErrNotFound
}

// GetCurrentAgentBinary retrieves the current agent binary for OS and arch.
func (s *MemoryStore) GetCurrentAgentBinary(ctx context.Context, os, arch string) (*AgentBinary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, b := range s.agentBinaries {
		if b.IsCurrent && b.OS == os && b.Arch == arch {
			result := *b
			return &result, nil
		}
	}
	return nil, ErrNotFound
}

// ListAgentBinaries returns all agent binaries.
func (s *MemoryStore) ListAgentBinaries(ctx context.Context) ([]*AgentBinary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	binaries := make([]*AgentBinary, 0, len(s.agentBinaries))
	for _, b := range s.agentBinaries {
		cp := *b
		binaries = append(binaries, &cp)
	}
	return binaries, nil
}

// SetCurrentAgentBinary sets an agent binary as the current version.
func (s *MemoryStore) SetCurrentAgentBinary(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	target, ok := s.agentBinaries[id]
	if !ok {
		return ErrNotFound
	}

	// Unset current for same OS/arch
	for _, b := range s.agentBinaries {
		if b.OS == target.OS && b.Arch == target.Arch && b.IsCurrent {
			b.IsCurrent = false
			s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpUpdate, "agent_binaries", b))
		}
	}

	// Set new current
	target.IsCurrent = true
	s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpUpdate, "agent_binaries", target))
	return nil
}

// DeleteAgentBinary removes an agent binary by ID.
func (s *MemoryStore) DeleteAgentBinary(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.agentBinaries[id]; !ok {
		return ErrNotFound
	}

	delete(s.agentBinaries, id)
	s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpDelete, "agent_binaries", id))
	return nil
}

// --- AgentUpdateHistory methods ---

// CreateAgentUpdateHistory creates a new update history record.
func (s *MemoryStore) CreateAgentUpdateHistory(ctx context.Context, history *AgentUpdateHistory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	history.ID = nextID(&s.nextAgentUpdateID)
	history.StartedAt = time.Now()
	if history.Status == "" {
		history.Status = "pending"
	}

	// Copy-on-store
	stored := *history
	s.agentUpdates[history.ID] = &stored

	s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpInsert, "agent_update_history", &stored))
	return nil
}

// GetAgentUpdateHistory retrieves an update history record by ID.
func (s *MemoryStore) GetAgentUpdateHistory(ctx context.Context, id int64) (*AgentUpdateHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history, ok := s.agentUpdates[id]
	if !ok {
		return nil, ErrNotFound
	}

	result := *history
	return &result, nil
}

// UpdateAgentUpdateHistory updates an existing update history record.
func (s *MemoryStore) UpdateAgentUpdateHistory(ctx context.Context, history *AgentUpdateHistory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.agentUpdates[history.ID]
	if !ok {
		return ErrNotFound
	}

	existing.Status = history.Status
	existing.ErrorMessage = history.ErrorMessage
	existing.CompletedAt = history.CompletedAt
	existing.RolledBack = history.RolledBack

	s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpUpdate, "agent_update_history", existing))
	return nil
}

// ListAgentUpdateHistory returns update history for an agent with pagination.
func (s *MemoryStore) ListAgentUpdateHistory(ctx context.Context, agentID string, limit, offset int) ([]*AgentUpdateHistory, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect all history for agent
	var all []*AgentUpdateHistory
	for _, h := range s.agentUpdates {
		if h.AgentID == agentID {
			cp := *h
			all = append(all, &cp)
		}
	}

	// Sort by StartedAt descending (newest first)
	for i := 0; i < len(all)-1; i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].StartedAt.After(all[i].StartedAt) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	total := int64(len(all))

	// Apply pagination
	if offset >= len(all) {
		return []*AgentUpdateHistory{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	return all[offset:end], total, nil
}

// ListAllAgentUpdateHistory returns all update history across all agents with pagination.
func (s *MemoryStore) ListAllAgentUpdateHistory(ctx context.Context, limit, offset int) ([]*AgentUpdateHistory, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect all history
	var all []*AgentUpdateHistory
	for _, h := range s.agentUpdates {
		cp := *h
		all = append(all, &cp)
	}

	// Sort by StartedAt descending (newest first)
	for i := 0; i < len(all)-1; i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].StartedAt.After(all[i].StartedAt) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	total := int64(len(all))

	// Apply pagination
	if offset >= len(all) {
		return []*AgentUpdateHistory{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	return all[offset:end], total, nil
}

// GetLatestAgentUpdateHistory returns the most recent update history for an agent.
func (s *MemoryStore) GetLatestAgentUpdateHistory(ctx context.Context, agentID string) (*AgentUpdateHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var latest *AgentUpdateHistory
	for _, h := range s.agentUpdates {
		if h.AgentID == agentID {
			if latest == nil || h.StartedAt.After(latest.StartedAt) {
				latest = h
			}
		}
	}

	if latest == nil {
		return nil, ErrNotFound
	}

	result := *latest
	return &result, nil
}
