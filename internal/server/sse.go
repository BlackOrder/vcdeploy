// Package server provides the HTTP server implementation.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SSEBroker manages SSE connections and event distribution.
type SSEBroker struct {
	clients    map[chan SSEEvent]struct{}
	register   chan chan SSEEvent
	unregister chan chan SSEEvent
	broadcast  chan SSEEvent
	mu         sync.RWMutex
	done       chan struct{}
}

// SSEEvent represents an event to send to clients.
type SSEEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// NewSSEBroker creates a new SSE broker.
func NewSSEBroker() *SSEBroker {
	b := &SSEBroker{
		clients:    make(map[chan SSEEvent]struct{}),
		register:   make(chan chan SSEEvent),
		unregister: make(chan chan SSEEvent),
		broadcast:  make(chan SSEEvent, 100),
		done:       make(chan struct{}),
	}
	go b.run()
	return b
}

func (b *SSEBroker) run() {
	for {
		select {
		case <-b.done:
			// Close all client channels
			b.mu.Lock()
			for client := range b.clients {
				close(client)
			}
			b.clients = make(map[chan SSEEvent]struct{})
			b.mu.Unlock()
			return
		case client := <-b.register:
			b.mu.Lock()
			b.clients[client] = struct{}{}
			b.mu.Unlock()
		case client := <-b.unregister:
			b.mu.Lock()
			if _, ok := b.clients[client]; ok {
				delete(b.clients, client)
				close(client)
			}
			b.mu.Unlock()
		case event := <-b.broadcast:
			b.mu.RLock()
			for client := range b.clients {
				select {
				case client <- event:
				default:
					// Client buffer full, skip event
				}
			}
			b.mu.RUnlock()
		}
	}
}

// Publish sends an event to all connected clients.
func (b *SSEBroker) Publish(eventType string, data interface{}) {
	select {
	case b.broadcast <- SSEEvent{Type: eventType, Data: data}:
	default:
		// Broadcast channel full, drop event
	}
}

// ClientCount returns the number of connected clients.
func (b *SSEBroker) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}

// Close shuts down the broker.
func (b *SSEBroker) Close() {
	close(b.done)
}

// streamLogsConfig holds callbacks for the generic SSE log streaming handler.
type streamLogsConfig struct {
	// listLogs returns all existing log entries as JSON-marshalable objects.
	listLogs func() ([]interface{}, string, error)
	// listLogsAfter returns log entries with ID > afterID, and the new last ID.
	listLogsAfter func(afterID string) ([]interface{}, string, error)
	// isComplete returns true and the terminal status if the resource is done.
	isComplete func() (bool, string)
}

// streamLogs streams log entries via Server-Sent Events using the provided callbacks.
// This is the unified SSE streaming handler for deployments and provision jobs.
func (s *MasterServer) streamLogs(w http.ResponseWriter, r *http.Request, cfg streamLogsConfig) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.jsonError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Send initial logs
	logs, lastID, err := cfg.listLogs()
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	for _, log := range logs {
		logJSON, err := json.Marshal(log)
		if err != nil {
			s.logger.Error("Failed to marshal log", zap.Error(err))
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", logJSON)
	}
	flusher.Flush()

	// Poll for new logs until resource completes or client disconnects
	const maxStreamDuration = 30 * time.Minute
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout := time.NewTimer(maxStreamDuration)
	defer timeout.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout.C:
			fmt.Fprintf(w, "event: timeout\ndata: {\"message\":\"Max streaming duration reached\"}\n\n")
			flusher.Flush()
			return
		case <-ticker.C:
			newLogs, newLastID, err := cfg.listLogsAfter(lastID)
			if err != nil {
				s.logger.Error("Failed to poll logs", zap.Error(err))
				continue
			}

			for _, log := range newLogs {
				logJSON, err := json.Marshal(log)
				if err != nil {
					s.logger.Error("Failed to marshal log", zap.Error(err))
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", logJSON)
			}
			if newLastID != lastID {
				lastID = newLastID
			}
			flusher.Flush()

			// Check if resource has reached terminal state
			if done, status := cfg.isComplete(); done {
				fmt.Fprintf(w, "event: complete\ndata: {\"status\":\"%s\"}\n\n", status)
				flusher.Flush()
				return
			}
		}
	}
}
