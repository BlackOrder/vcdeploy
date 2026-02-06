// Package server provides the HTTP server implementation.
package server

import (
	"sync"
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
