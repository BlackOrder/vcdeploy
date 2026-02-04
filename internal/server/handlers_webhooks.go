// Package server provides webhook handlers for the master server.
package server

import (
	"net/http"
)

// handleGitHubWebhook handles incoming GitHub webhook requests.
func (s *MasterServer) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Delegate to the webhooks handler if configured
	if s.webhookHandler != nil && s.webhookHandler.handler != nil {
		s.webhookHandler.handler.HandleGitHub(w, r)
		return
	}

	// Fallback: basic validation only
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		s.jsonError(w, http.StatusUnauthorized, "missing signature")
		return
	}
	s.logger.Warn("Received GitHub webhook but no processor configured - webhook will not trigger deployment")
	w.WriteHeader(http.StatusOK)
}

// handleGitLabWebhook handles incoming GitLab webhook requests.
func (s *MasterServer) handleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Delegate to the webhooks handler if configured
	if s.webhookHandler != nil && s.webhookHandler.handler != nil {
		s.webhookHandler.handler.HandleGitLab(w, r)
		return
	}

	// Fallback: basic validation only
	token := r.Header.Get("X-Gitlab-Token")
	if token == "" {
		s.jsonError(w, http.StatusUnauthorized, "missing token")
		return
	}
	s.logger.Warn("Received GitLab webhook but no processor configured - webhook will not trigger deployment")
	w.WriteHeader(http.StatusOK)
}

// handleBitbucketWebhook handles incoming Bitbucket webhook requests.
func (s *MasterServer) handleBitbucketWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Delegate to the webhooks handler if configured
	if s.webhookHandler != nil && s.webhookHandler.handler != nil {
		s.webhookHandler.handler.HandleBitbucket(w, r)
		return
	}

	s.logger.Warn("Received Bitbucket webhook but no processor configured - webhook will not trigger deployment")
	w.WriteHeader(http.StatusOK)
}
