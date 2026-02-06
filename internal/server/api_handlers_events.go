// Package server provides the HTTP server implementation.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// handleEventStream handles GET /api/v1/events/stream for SSE event streaming.
func (s *MasterServer) handleEventStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Parse type filter
	typesParam := r.URL.Query().Get("types")
	typeFilter := parseTypeFilter(typesParam)

	// Create client channel with buffer
	client := make(chan SSEEvent, 10)
	s.sseBroker.register <- client
	defer func() {
		s.sseBroker.unregister <- client
	}()

	// Get flusher for streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.jsonError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	// Stream events
	for {
		select {
		case event, ok := <-client:
			if !ok {
				// Channel closed
				return
			}
			if shouldSendEvent(event.Type, typeFilter) {
				data, err := json.Marshal(event.Data)
				if err != nil {
					continue
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

// parseTypeFilter parses comma-separated event types from query parameter.
func parseTypeFilter(types string) map[string]bool {
	if types == "" {
		return nil // nil means accept all types
	}

	filter := make(map[string]bool)
	for _, t := range strings.Split(types, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			filter[t] = true
		}
	}

	if len(filter) == 0 {
		return nil
	}

	return filter
}

// shouldSendEvent checks if an event type should be sent based on the filter.
func shouldSendEvent(eventType string, filter map[string]bool) bool {
	if filter == nil {
		return true // No filter means send all
	}
	return filter[eventType]
}

// DeploymentEventData represents deployment status change event data.
type DeploymentEventData struct {
	ID        string `json:"id"`
	Project   string `json:"project"`
	Status    string `json:"status"`
	Target    string `json:"target,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Message   string `json:"message,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// AgentEventData represents agent status change event data.
type AgentEventData struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"` // "online", "offline", "heartbeat"
	Hostname  string `json:"hostname,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// AuditEventData represents audit log event data.
type AuditEventData struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Action    string `json:"action"`
	Resource  string `json:"resource"`
	IP        string `json:"ip,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// HealthEventData represents health check event data.
type HealthEventData struct {
	Name      string `json:"name"`
	Status    string `json:"status"` // "healthy", "unhealthy", "degraded"
	Message   string `json:"message,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// NotificationEventData represents system notification event data.
type NotificationEventData struct {
	Level     string `json:"level"` // "info", "warning", "error"
	Title     string `json:"title"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// publishDeploymentEvent publishes a deployment status change event.
func (s *MasterServer) publishDeploymentEvent(id, project, status, target, branch, message string) {
	if s.sseBroker == nil {
		return
	}
	s.sseBroker.Publish("deployment", DeploymentEventData{
		ID:        id,
		Project:   project,
		Status:    status,
		Target:    target,
		Branch:    branch,
		Message:   message,
		Timestamp: time.Now().Unix(),
	})
}

// publishAgentEvent publishes an agent status change event.
func (s *MasterServer) publishAgentEvent(id, name, status, hostname string) {
	if s.sseBroker == nil {
		return
	}
	s.sseBroker.Publish("agent", AgentEventData{
		ID:        id,
		Name:      name,
		Status:    status,
		Hostname:  hostname,
		Timestamp: time.Now().Unix(),
	})
}

// publishAuditEvent publishes an audit log event.
func (s *MasterServer) publishAuditEvent(id, username, action, resource, ip string) {
	if s.sseBroker == nil {
		return
	}
	s.sseBroker.Publish("audit", AuditEventData{
		ID:        id,
		Username:  username,
		Action:    action,
		Resource:  resource,
		IP:        ip,
		Timestamp: time.Now().Unix(),
	})
}

// publishHealthEvent publishes a health check event.
// TODO: Integrate with background health check scheduler when implemented.
//
//nolint:unused // Reserved for future background health check integration
func (s *MasterServer) publishHealthEvent(name, status, message string) {
	if s.sseBroker == nil {
		return
	}
	s.sseBroker.Publish("health", HealthEventData{
		Name:      name,
		Status:    status,
		Message:   message,
		Timestamp: time.Now().Unix(),
	})
}

// publishNotificationEvent publishes a system notification event.
// TODO: Integrate with alerting manager when implemented.
//
//nolint:unused // Reserved for future alerting integration
func (s *MasterServer) publishNotificationEvent(level, title, message string) {
	if s.sseBroker == nil {
		return
	}
	s.sseBroker.Publish("notification", NotificationEventData{
		Level:     level,
		Title:     title,
		Message:   message,
		Timestamp: time.Now().Unix(),
	})
}
