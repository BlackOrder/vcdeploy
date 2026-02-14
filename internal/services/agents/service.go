// Package agents provides agent management functionality.
package agents

import (
	"context"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.AgentServicer = (*Service)(nil)

// Service handles agent management.
type Service struct {
	store storage.Store
}

// New creates a new agents Service.
func New(store storage.Store) *Service {
	return &Service{store: store}
}

// Upsert creates or updates an agent.
func (s *Service) Upsert(ctx context.Context, agent *storage.Agent) error {
	if err := s.store.UpsertAgent(ctx, agent); err != nil {
		return fmt.Errorf("upserting agent: %w", err)
	}
	return nil
}

// GetByID retrieves an agent by ID.
func (s *Service) GetByID(ctx context.Context, id string) (*storage.Agent, error) {
	agent, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return nil, err // Returns ErrNotFound if not found
	}
	return agent, nil
}

// List returns all agents.
func (s *Service) List(ctx context.Context) ([]*storage.Agent, error) {
	agents, err := s.store.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}
	return agents, nil
}

// ListPaginated returns agents with pagination support.
func (s *Service) ListPaginated(ctx context.Context, p services.Pagination) (*services.ListResult[*storage.Agent], error) {
	agents, err := s.store.ListAgentsPaginated(ctx, p.Limit, p.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}
	count, err := s.store.CountAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("counting agents: %w", err)
	}
	return &services.ListResult[*storage.Agent]{
		Items:      agents,
		TotalCount: count,
		Pagination: p,
	}, nil
}

// Delete removes an agent by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.store.DeleteAgent(ctx, id); err != nil {
		return fmt.Errorf("deleting agent: %w", err)
	}
	return nil
}

// MarkStale marks agents that haven't been seen since the cutoff as disconnected.
func (s *Service) MarkStale(ctx context.Context, cutoff time.Time) (int64, error) {
	count, err := s.store.MarkStaleAgents(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("marking stale agents: %w", err)
	}
	return count, nil
}

// Count returns the total number of agents.
func (s *Service) Count(ctx context.Context) (int64, error) {
	count, err := s.store.CountAgents(ctx)
	if err != nil {
		return 0, fmt.Errorf("counting agents: %w", err)
	}
	return count, nil
}

// CountByStatus returns agent counts grouped by status.
func (s *Service) CountByStatus(ctx context.Context) (map[string]int64, error) {
	counts, err := s.store.CountAgentsByStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("counting agents by status: %w", err)
	}
	return counts, nil
}

// UpdateStatus updates an agent's status and last seen time.
func (s *Service) UpdateStatus(ctx context.Context, id, status string) error {
	agent, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return fmt.Errorf("getting agent: %w", err)
	}

	agent.Status = storage.AgentStatus(status)
	agent.LastSeenAt = time.Now()

	if err := s.store.UpsertAgent(ctx, agent); err != nil {
		return fmt.Errorf("updating agent: %w", err)
	}

	return nil
}
