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
	db *storage.DB
}

// New creates a new agents Service.
func New(db *storage.DB) *Service {
	return &Service{db: db}
}

// Upsert creates or updates an agent.
func (s *Service) Upsert(ctx context.Context, agent *storage.Agent) error {
	if err := s.db.UpsertAgent(ctx, agent); err != nil {
		return fmt.Errorf("upserting agent: %w", err)
	}
	return nil
}

// GetByID retrieves an agent by ID.
func (s *Service) GetByID(ctx context.Context, id string) (*storage.Agent, error) {
	agent, err := s.db.GetAgent(ctx, id)
	if err != nil {
		return nil, err // Returns ErrNotFound if not found
	}
	return agent, nil
}

// List returns all agents.
func (s *Service) List(ctx context.Context) ([]*storage.Agent, error) {
	agents, err := s.db.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}
	return agents, nil
}

// Delete removes an agent by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.db.DeleteAgent(ctx, id); err != nil {
		return fmt.Errorf("deleting agent: %w", err)
	}
	return nil
}

// MarkStale marks agents that haven't been seen since the cutoff as disconnected.
func (s *Service) MarkStale(ctx context.Context, cutoff time.Time) (int64, error) {
	count, err := s.db.MarkStaleAgents(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("marking stale agents: %w", err)
	}
	return count, nil
}

// UpdateStatus updates an agent's status and last seen time.
func (s *Service) UpdateStatus(ctx context.Context, id, status string) error {
	agent, err := s.db.GetAgent(ctx, id)
	if err != nil {
		return fmt.Errorf("getting agent: %w", err)
	}

	agent.Status = status
	agent.LastSeenAt = time.Now()

	if err := s.db.UpsertAgent(ctx, agent); err != nil {
		return fmt.Errorf("updating agent: %w", err)
	}

	return nil
}
