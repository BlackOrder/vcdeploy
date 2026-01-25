// Package webhooks provides webhook handling for Git providers.
package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// Handler processes incoming webhooks from Git providers.
type Handler struct {
	logger    *zap.Logger
	secrets   SecretStore
	processor EventProcessor
}

// SecretStore retrieves webhook secrets for projects.
type SecretStore interface {
	GetWebhookSecret(projectID string) (string, error)
	IsSecretRequired(projectID string) bool
}

// EventProcessor handles processed webhook events.
type EventProcessor interface {
	ProcessPush(event *PushEvent) error
	ProcessPullRequest(event *PullRequestEvent) error
	ProcessTag(event *TagEvent) error
}

// PushEvent represents a push to a repository.
type PushEvent struct {
	Provider    string
	ProjectID   string
	Repository  string
	Branch      string
	Commit      string
	CommitURL   string
	Author      string
	AuthorEmail string
	Message     string
	Timestamp   string
	ForcePush   bool
	Deleted     bool
}

// PullRequestEvent represents a pull/merge request event.
type PullRequestEvent struct {
	Provider     string
	ProjectID    string
	Repository   string
	Action       string // opened, closed, merged, synchronize
	Number       int
	Title        string
	SourceBranch string
	TargetBranch string
	Author       string
}

// TagEvent represents a tag creation/deletion.
type TagEvent struct {
	Provider   string
	ProjectID  string
	Repository string
	Tag        string
	Commit     string
	Author     string
	Deleted    bool
}

// NewHandler creates a new webhook handler.
func NewHandler(logger *zap.Logger, secrets SecretStore, processor EventProcessor) *Handler {
	return &Handler{
		logger:    logger,
		secrets:   secrets,
		processor: processor,
	}
}

// HandleGitHub handles GitHub webhooks.
func (h *Handler) HandleGitHub(w http.ResponseWriter, r *http.Request) {
	projectID := extractProjectID(r.URL.Path, "/webhook/github/")
	if projectID == "" {
		http.Error(w, "Missing project ID", http.StatusBadRequest)
		return
	}
	if !isValidProjectID(projectID) {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Get secret and validate signature
	secret, err := h.secrets.GetWebhookSecret(projectID)
	if err != nil {
		h.logger.Error("Failed to get webhook secret", zap.String("project", projectID), zap.Error(err))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	requireSecret := h.secrets.IsSecretRequired(projectID)
	if !h.validateGitHubSignature(body, signature, secret, requireSecret) {
		h.logger.Warn("Invalid GitHub signature", zap.String("project", projectID))
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	// Process based on event type
	eventType := r.Header.Get("X-GitHub-Event")
	h.logger.Info("Received GitHub webhook",
		zap.String("project", projectID),
		zap.String("event", eventType),
	)

	switch eventType {
	case "push":
		h.handleGitHubPush(w, body, projectID)
	case "pull_request":
		h.handleGitHubPR(w, body, projectID)
	case "create", "delete":
		h.handleGitHubTag(w, body, projectID, eventType == "delete")
	case "ping":
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	default:
		h.logger.Debug("Ignoring GitHub event", zap.String("event", eventType))
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handler) validateGitHubSignature(payload []byte, signature, secret string, requireSecret bool) bool {
	if secret == "" {
		if requireSecret {
			h.logger.Warn("Webhook secret required but not configured - rejecting request")
			return false
		}
		// Warn about missing secret even if not strictly required (security best practice)
		h.logger.Warn("No webhook secret configured - request allowed but signature verification skipped. " +
			"Consider configuring a webhook secret for improved security.")
		return true
	}

	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	sig := strings.TrimPrefix(signature, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expected))
}

func (h *Handler) handleGitHubPush(w http.ResponseWriter, body []byte, projectID string) {
	var payload struct {
		Ref        string `json:"ref"`
		Before     string `json:"before"`
		After      string `json:"after"`
		Forced     bool   `json:"forced"`
		Deleted    bool   `json:"deleted"`
		Repository struct {
			FullName string `json:"full_name"`
			CloneURL string `json:"clone_url"`
		} `json:"repository"`
		HeadCommit struct {
			ID        string `json:"id"`
			Message   string `json:"message"`
			Timestamp string `json:"timestamp"`
			URL       string `json:"url"`
			Author    struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"author"`
		} `json:"head_commit"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("Failed to parse GitHub push", zap.Error(err))
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Extract branch from ref (refs/heads/main -> main)
	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	if strings.HasPrefix(payload.Ref, "refs/tags/") {
		// This is a tag push, ignore here (handled by create/delete events)
		w.WriteHeader(http.StatusOK)
		return
	}

	event := &PushEvent{
		Provider:    "github",
		ProjectID:   projectID,
		Repository:  payload.Repository.CloneURL,
		Branch:      branch,
		Commit:      payload.After,
		CommitURL:   payload.HeadCommit.URL,
		Author:      payload.HeadCommit.Author.Name,
		AuthorEmail: payload.HeadCommit.Author.Email,
		Message:     payload.HeadCommit.Message,
		Timestamp:   payload.HeadCommit.Timestamp,
		ForcePush:   payload.Forced,
		Deleted:     payload.Deleted,
	}

	if err := h.processor.ProcessPush(event); err != nil {
		h.logger.Error("Failed to process push", zap.Error(err))
		http.Error(w, "Processing failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleGitHubPR(w http.ResponseWriter, body []byte, projectID string) {
	var payload struct {
		Action      string `json:"action"`
		Number      int    `json:"number"`
		PullRequest struct {
			Title string `json:"title"`
			User  struct {
				Login string `json:"login"`
			} `json:"user"`
			Head struct {
				Ref string `json:"ref"`
			} `json:"head"`
			Base struct {
				Ref string `json:"ref"`
			} `json:"base"`
		} `json:"pull_request"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("Failed to parse GitHub PR", zap.Error(err))
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	event := &PullRequestEvent{
		Provider:     "github",
		ProjectID:    projectID,
		Repository:   payload.Repository.FullName,
		Action:       payload.Action,
		Number:       payload.Number,
		Title:        payload.PullRequest.Title,
		SourceBranch: payload.PullRequest.Head.Ref,
		TargetBranch: payload.PullRequest.Base.Ref,
		Author:       payload.PullRequest.User.Login,
	}

	if err := h.processor.ProcessPullRequest(event); err != nil {
		h.logger.Error("Failed to process PR", zap.Error(err))
		http.Error(w, "Processing failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleGitHubTag(w http.ResponseWriter, body []byte, projectID string, deleted bool) {
	var payload struct {
		RefType string `json:"ref_type"`
		Ref     string `json:"ref"`
		Sender  struct {
			Login string `json:"login"`
		} `json:"sender"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("Failed to parse GitHub tag event", zap.Error(err))
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	if payload.RefType != "tag" {
		w.WriteHeader(http.StatusOK)
		return
	}

	event := &TagEvent{
		Provider:   "github",
		ProjectID:  projectID,
		Repository: payload.Repository.FullName,
		Tag:        payload.Ref,
		Author:     payload.Sender.Login,
		Deleted:    deleted,
	}

	if err := h.processor.ProcessTag(event); err != nil {
		h.logger.Error("Failed to process tag", zap.Error(err))
		http.Error(w, "Processing failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleGitLab handles GitLab webhooks.
func (h *Handler) HandleGitLab(w http.ResponseWriter, r *http.Request) {
	projectID := extractProjectID(r.URL.Path, "/webhook/gitlab/")
	if projectID == "" {
		http.Error(w, "Missing project ID", http.StatusBadRequest)
		return
	}
	if !isValidProjectID(projectID) {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Get secret and validate token
	secret, err := h.secrets.GetWebhookSecret(projectID)
	if err != nil {
		h.logger.Error("Failed to get webhook secret", zap.String("project", projectID), zap.Error(err))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	token := r.Header.Get("X-Gitlab-Token")
	requireSecret := h.secrets.IsSecretRequired(projectID)
	if secret == "" && requireSecret {
		h.logger.Warn("Webhook secret required but not configured - rejecting request", zap.String("project", projectID))
		http.Error(w, "Webhook secret required", http.StatusUnauthorized)
		return
	}
	if secret == "" && !requireSecret {
		// Warn about missing secret even if not strictly required (security best practice)
		h.logger.Warn("No webhook secret configured - request allowed but token verification skipped. "+
			"Consider configuring a webhook secret for improved security.", zap.String("project", projectID))
	}
	if secret != "" && token != secret {
		h.logger.Warn("Invalid GitLab token", zap.String("project", projectID))
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Process based on event type
	eventType := r.Header.Get("X-Gitlab-Event")
	h.logger.Info("Received GitLab webhook",
		zap.String("project", projectID),
		zap.String("event", eventType),
	)

	switch eventType {
	case "Push Hook":
		h.handleGitLabPush(w, body, projectID)
	case "Merge Request Hook":
		h.handleGitLabMR(w, body, projectID)
	case "Tag Push Hook":
		h.handleGitLabTag(w, body, projectID)
	default:
		h.logger.Debug("Ignoring GitLab event", zap.String("event", eventType))
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handler) handleGitLabPush(w http.ResponseWriter, body []byte, projectID string) {
	var payload struct {
		Ref     string `json:"ref"`
		Before  string `json:"before"`
		After   string `json:"after"`
		Project struct {
			GitHTTPURL string `json:"git_http_url"`
		} `json:"project"`
		Commits []struct {
			ID        string `json:"id"`
			Message   string `json:"message"`
			Timestamp string `json:"timestamp"`
			URL       string `json:"url"`
			Author    struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"author"`
		} `json:"commits"`
		UserName string `json:"user_name"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("Failed to parse GitLab push", zap.Error(err))
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Extract branch from ref
	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

	// Check if this is a deletion (all zeros)
	deleted := payload.After == "0000000000000000000000000000000000000000"

	var commit, message, timestamp, commitURL, author, email string
	if len(payload.Commits) > 0 {
		lastCommit := payload.Commits[len(payload.Commits)-1]
		commit = lastCommit.ID
		message = lastCommit.Message
		timestamp = lastCommit.Timestamp
		commitURL = lastCommit.URL
		author = lastCommit.Author.Name
		email = lastCommit.Author.Email
	} else {
		commit = payload.After
		author = payload.UserName
	}

	event := &PushEvent{
		Provider:    "gitlab",
		ProjectID:   projectID,
		Repository:  payload.Project.GitHTTPURL,
		Branch:      branch,
		Commit:      commit,
		CommitURL:   commitURL,
		Author:      author,
		AuthorEmail: email,
		Message:     message,
		Timestamp:   timestamp,
		Deleted:     deleted,
	}

	if err := h.processor.ProcessPush(event); err != nil {
		h.logger.Error("Failed to process push", zap.Error(err))
		http.Error(w, "Processing failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleGitLabMR(w http.ResponseWriter, body []byte, projectID string) {
	var payload struct {
		ObjectAttributes struct {
			Action       string `json:"action"`
			IID          int    `json:"iid"`
			Title        string `json:"title"`
			SourceBranch string `json:"source_branch"`
			TargetBranch string `json:"target_branch"`
		} `json:"object_attributes"`
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		Project struct {
			PathWithNamespace string `json:"path_with_namespace"`
		} `json:"project"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("Failed to parse GitLab MR", zap.Error(err))
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	event := &PullRequestEvent{
		Provider:     "gitlab",
		ProjectID:    projectID,
		Repository:   payload.Project.PathWithNamespace,
		Action:       payload.ObjectAttributes.Action,
		Number:       payload.ObjectAttributes.IID,
		Title:        payload.ObjectAttributes.Title,
		SourceBranch: payload.ObjectAttributes.SourceBranch,
		TargetBranch: payload.ObjectAttributes.TargetBranch,
		Author:       payload.User.Username,
	}

	if err := h.processor.ProcessPullRequest(event); err != nil {
		h.logger.Error("Failed to process MR", zap.Error(err))
		http.Error(w, "Processing failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleGitLabTag(w http.ResponseWriter, body []byte, projectID string) {
	var payload struct {
		Ref     string `json:"ref"`
		Before  string `json:"before"`
		After   string `json:"after"`
		Project struct {
			PathWithNamespace string `json:"path_with_namespace"`
		} `json:"project"`
		UserName string `json:"user_name"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("Failed to parse GitLab tag", zap.Error(err))
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	tag := strings.TrimPrefix(payload.Ref, "refs/tags/")
	deleted := payload.After == "0000000000000000000000000000000000000000"

	event := &TagEvent{
		Provider:   "gitlab",
		ProjectID:  projectID,
		Repository: payload.Project.PathWithNamespace,
		Tag:        tag,
		Commit:     payload.After,
		Author:     payload.UserName,
		Deleted:    deleted,
	}

	if err := h.processor.ProcessTag(event); err != nil {
		h.logger.Error("Failed to process tag", zap.Error(err))
		http.Error(w, "Processing failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleBitbucket handles Bitbucket webhooks.
func (h *Handler) HandleBitbucket(w http.ResponseWriter, r *http.Request) {
	projectID := extractProjectID(r.URL.Path, "/webhook/bitbucket/")
	if projectID == "" {
		http.Error(w, "Missing project ID", http.StatusBadRequest)
		return
	}
	if !isValidProjectID(projectID) {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Get secret and validate (Bitbucket uses IP whitelisting or UUID matching typically)
	// For simplicity, we check X-Event-Key header existence
	eventType := r.Header.Get("X-Event-Key")
	if eventType == "" {
		h.logger.Warn("Missing Bitbucket event key", zap.String("project", projectID))
		http.Error(w, "Missing event key", http.StatusBadRequest)
		return
	}

	h.logger.Info("Received Bitbucket webhook",
		zap.String("project", projectID),
		zap.String("event", eventType),
	)

	switch eventType {
	case "repo:push":
		h.handleBitbucketPush(w, body, projectID)
	case "pullrequest:created", "pullrequest:updated", "pullrequest:fulfilled", "pullrequest:rejected":
		h.handleBitbucketPR(w, body, projectID, eventType)
	default:
		h.logger.Debug("Ignoring Bitbucket event", zap.String("event", eventType))
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handler) handleBitbucketPush(w http.ResponseWriter, body []byte, projectID string) {
	var payload struct {
		Push struct {
			Changes []struct {
				New struct {
					Name   string `json:"name"`
					Type   string `json:"type"`
					Target struct {
						Hash    string `json:"hash"`
						Message string `json:"message"`
						Date    string `json:"date"`
						Author  struct {
							User struct {
								DisplayName string `json:"display_name"`
							} `json:"user"`
						} `json:"author"`
					} `json:"target"`
				} `json:"new"`
				Old struct {
					Name string `json:"name"`
				} `json:"old"`
				Forced bool `json:"forced"`
			} `json:"changes"`
		} `json:"push"`
		Repository struct {
			FullName string `json:"full_name"`
			Links    struct {
				HTML struct {
					Href string `json:"href"`
				} `json:"html"`
			} `json:"links"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("Failed to parse Bitbucket push", zap.Error(err))
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	for _, change := range payload.Push.Changes {
		if change.New.Type != "branch" {
			continue
		}

		event := &PushEvent{
			Provider:   "bitbucket",
			ProjectID:  projectID,
			Repository: payload.Repository.Links.HTML.Href,
			Branch:     change.New.Name,
			Commit:     change.New.Target.Hash,
			Author:     change.New.Target.Author.User.DisplayName,
			Message:    change.New.Target.Message,
			Timestamp:  change.New.Target.Date,
			ForcePush:  change.Forced,
		}

		if err := h.processor.ProcessPush(event); err != nil {
			h.logger.Error("Failed to process push", zap.Error(err))
			http.Error(w, "Processing failed", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleBitbucketPR(w http.ResponseWriter, body []byte, projectID, eventType string) {
	var payload struct {
		PullRequest struct {
			ID     int    `json:"id"`
			Title  string `json:"title"`
			Source struct {
				Branch struct {
					Name string `json:"name"`
				} `json:"branch"`
			} `json:"source"`
			Destination struct {
				Branch struct {
					Name string `json:"name"`
				} `json:"branch"`
			} `json:"destination"`
			Author struct {
				DisplayName string `json:"display_name"`
			} `json:"author"`
		} `json:"pullrequest"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("Failed to parse Bitbucket PR", zap.Error(err))
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Map Bitbucket event to action
	action := "unknown"
	switch eventType {
	case "pullrequest:created":
		action = "opened"
	case "pullrequest:updated":
		action = "synchronize"
	case "pullrequest:fulfilled":
		action = "merged"
	case "pullrequest:rejected":
		action = "closed"
	}

	event := &PullRequestEvent{
		Provider:     "bitbucket",
		ProjectID:    projectID,
		Repository:   payload.Repository.FullName,
		Action:       action,
		Number:       payload.PullRequest.ID,
		Title:        payload.PullRequest.Title,
		SourceBranch: payload.PullRequest.Source.Branch.Name,
		TargetBranch: payload.PullRequest.Destination.Branch.Name,
		Author:       payload.PullRequest.Author.DisplayName,
	}

	if err := h.processor.ProcessPullRequest(event); err != nil {
		h.logger.Error("Failed to process PR", zap.Error(err))
		http.Error(w, "Processing failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// isValidProjectID validates that a project ID contains only safe characters.
// Valid characters: alphanumeric, dash, underscore (no path traversal or injection)
func isValidProjectID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func extractProjectID(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	id := strings.TrimPrefix(path, prefix)
	// Remove any trailing path segments
	if idx := strings.Index(id, "/"); idx != -1 {
		id = id[:idx]
	}
	return id
}
