// Package server provides HTTP health check handlers for the master server.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// HealthStatus represents detailed health information.
type HealthStatus struct {
	Status    string                 `json:"status"` // "healthy", "degraded", "unhealthy"
	Checks    map[string]CheckResult `json:"checks"`
	Version   string                 `json:"version,omitempty"`
	Uptime    string                 `json:"uptime,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// CheckResult represents the result of a single health check.
type CheckResult struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// handleHealth handles the /api/v1/health endpoint.
// Returns detailed health status including database and gRPC connectivity.
func (s *MasterServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	health := s.buildDetailedHealth(ctx)

	statusCode := http.StatusOK
	if health.Status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(health)
}

// handleHealthzLive handles /healthz and /livez (Kubernetes liveness probe).
// Returns 200 if the process is alive.
func (s *MasterServer) handleHealthzLive(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleHealthzReady handles /readyz (Kubernetes readiness probe).
// Returns 200 if the server can serve traffic, 503 otherwise.
func (s *MasterServer) handleHealthzReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Check database connectivity
	if err := s.store.Conn().PingContext(ctx); err != nil {
		s.logger.Warn("Readiness check failed: database not ready", zap.Error(err))
		http.Error(w, "database not ready", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// buildDetailedHealth builds a detailed health status.
func (s *MasterServer) buildDetailedHealth(ctx context.Context) HealthStatus {
	health := HealthStatus{
		Status:    "healthy",
		Checks:    make(map[string]CheckResult),
		Timestamp: time.Now().UTC(),
	}

	// Database check
	dbStart := time.Now()
	if err := s.store.Conn().PingContext(ctx); err != nil {
		health.Status = "unhealthy"
		health.Checks["database"] = CheckResult{
			Status:  "unhealthy",
			Message: err.Error(),
		}
	} else {
		health.Checks["database"] = CheckResult{
			Status:  "healthy",
			Latency: time.Since(dbStart).Round(time.Microsecond).String(),
		}
	}

	// gRPC server check
	if s.grpcServer != nil {
		health.Checks["grpc"] = CheckResult{Status: "healthy"}
	} else {
		health.Checks["grpc"] = CheckResult{
			Status:  "degraded",
			Message: "gRPC server not initialized",
		}
		if health.Status == "healthy" {
			health.Status = "degraded"
		}
	}

	// Agent connectivity summary
	s.agentsMu.RLock()
	connectedCount := 0
	totalCount := len(s.agents)
	for _, agent := range s.agents {
		if agent.Status == "connected" {
			connectedCount++
		}
	}
	s.agentsMu.RUnlock()

	agentStatus := "healthy"
	if totalCount > 0 && connectedCount == 0 {
		agentStatus = "degraded"
		if health.Status == "healthy" {
			health.Status = "degraded"
		}
	}
	health.Checks["agents"] = CheckResult{
		Status:  agentStatus,
		Message: fmt.Sprintf("%d/%d connected", connectedCount, totalCount),
	}

	return health
}
