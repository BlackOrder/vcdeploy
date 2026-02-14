// Package server provides statistics API handlers.
package server

import (
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// DeploymentStatsDetailed represents detailed deployment statistics.
type DeploymentStatsDetailed struct {
	Total       int64        `json:"total"`
	Successful  int64        `json:"successful"`
	Failed      int64        `json:"failed"`
	Cancelled   int64        `json:"cancelled"`
	Running     int64        `json:"running"`
	SuccessRate float64      `json:"success_rate"`
	AvgDuration int64        `json:"avg_duration_seconds"`
	ByDay       []DailyStats `json:"by_day"`
}

// DailyStats represents daily deployment statistics.
type DailyStats struct {
	Date       string `json:"date"`
	Total      int64  `json:"total"`
	Successful int64  `json:"successful"`
	Failed     int64  `json:"failed"`
}

// AgentStatsDetailed represents detailed agent statistics.
type AgentStatsDetailed struct {
	Total       int64              `json:"total"`
	Online      int64              `json:"online"`
	Offline     int64              `json:"offline"`
	Maintenance int64              `json:"maintenance"`
	Utilization []AgentUtilization `json:"utilization"`
}

// AgentUtilization represents agent utilization over time.
type AgentUtilization struct {
	Timestamp time.Time `json:"timestamp"`
	Online    int64     `json:"online"`
	Deploying int64     `json:"deploying"`
}

// handleDeploymentStats handles GET /api/v1/stats/deployments.
func (s *MasterServer) handleDeploymentStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	// Check read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Parse query parameters
	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = "7d"
	}

	// Parse range (e.g., "7d", "30d", "1m")
	rangeDays := 7
	if len(rangeStr) > 1 {
		numPart := rangeStr[:len(rangeStr)-1]
		unitPart := rangeStr[len(rangeStr)-1:]
		if num, err := strconv.Atoi(numPart); err == nil {
			switch unitPart {
			case "d":
				rangeDays = num
			case "m":
				rangeDays = num * 30
			case "w":
				rangeDays = num * 7
			}
		}
	}

	project := r.URL.Query().Get("project")
	startTime := time.Now().AddDate(0, 0, -rangeDays)

	// Get deployment statistics - get paginated list
	deployments, err := s.store.ListDeploymentsPaginated(ctx, 10000, 0)
	if err != nil {
		s.logger.Error("Failed to get deployments for stats", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	stats := DeploymentStatsDetailed{
		ByDay: make([]DailyStats, 0),
	}

	// Calculate statistics
	dailyMap := make(map[string]*DailyStats)
	var totalDuration int64
	var completedCount int64

	for _, d := range deployments {
		// Filter by time range
		if d.StartedAt.Before(startTime) {
			continue
		}

		// Filter by project if specified
		if project != "" && d.Project != project {
			continue
		}

		stats.Total++

		switch d.Status {
		case "completed", "success":
			stats.Successful++
		case "failed", "error":
			stats.Failed++
		case "cancelled", "canceled":
			stats.Cancelled++
		case "running", "pending":
			stats.Running++
		}

		// Calculate duration for completed deployments
		if d.CompletedAt != nil && !d.CompletedAt.IsZero() {
			duration := d.CompletedAt.Sub(d.StartedAt).Seconds()
			if duration > 0 {
				totalDuration += int64(duration)
				completedCount++
			}
		}

		// Aggregate by day
		dayKey := d.StartedAt.Format("2006-01-02")
		if dailyMap[dayKey] == nil {
			dailyMap[dayKey] = &DailyStats{Date: dayKey}
		}
		dailyMap[dayKey].Total++
		switch d.Status {
		case "completed", "success":
			dailyMap[dayKey].Successful++
		case "failed", "error":
			dailyMap[dayKey].Failed++
		}
	}

	// Calculate success rate
	if stats.Total > 0 && (stats.Successful+stats.Failed) > 0 {
		stats.SuccessRate = float64(stats.Successful) / float64(stats.Successful+stats.Failed) * 100
	}

	// Calculate average duration
	if completedCount > 0 {
		stats.AvgDuration = totalDuration / completedCount
	}

	// Convert daily map to slice (sorted by date)
	for i := 0; i < rangeDays; i++ {
		date := time.Now().AddDate(0, 0, -rangeDays+i+1).Format("2006-01-02")
		if daily, ok := dailyMap[date]; ok {
			stats.ByDay = append(stats.ByDay, *daily)
		} else {
			stats.ByDay = append(stats.ByDay, DailyStats{Date: date})
		}
	}

	s.jsonResponse(w, stats)
}

// handleAgentStats handles GET /api/v1/stats/agents.
func (s *MasterServer) handleAgentStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	// Check read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Get all agents
	agents, err := s.agentService.List(ctx)
	if err != nil {
		s.logger.Error("Failed to list agents for stats", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	stats := AgentStatsDetailed{
		Utilization: make([]AgentUtilization, 0),
	}

	// Calculate statistics
	for _, agent := range agents {
		stats.Total++
		switch string(agent.Status) {
		case "online", "connected":
			stats.Online++
		case "offline", "disconnected":
			stats.Offline++
		case "maintenance":
			stats.Maintenance++
		}
	}

	// Add current utilization snapshot
	stats.Utilization = append(stats.Utilization, AgentUtilization{
		Timestamp: time.Now().UTC(),
		Online:    stats.Online,
		Deploying: 0, // Could be enhanced to track active deployments
	})

	s.jsonResponse(w, stats)
}
