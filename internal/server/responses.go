// Package server provides API response types for consistent JSON formatting.
package server

import "time"

// --- Standard Response Types ---

// StatusResponse is used for simple status messages (delete, update, etc.)
type StatusResponse struct {
	Status string `json:"status"`
}

// ErrorResponse is used for error responses.
type ErrorResponse struct {
	Error string `json:"error"`
}

// --- User Responses ---

// UserResponse represents a user in API responses.
type UserResponse struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// UserCreateResponse represents a newly created user.
type UserCreateResponse struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// --- Project Responses ---

// ProjectResponse represents a project in API responses.
type ProjectResponse struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	Repository    string    `json:"repository"`
	Branch        string    `json:"branch"`
	DeployPath    string    `json:"deployPath"`
	ProjectTypeID string    `json:"projectTypeId,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// --- API Key Responses ---

// APIKeyResponse represents an API key in list responses (no raw key).
type APIKeyResponse struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// APIKeyCreateResponse represents a newly created API key (includes raw key).
type APIKeyCreateResponse struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"` // Only visible on creation
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// --- Agent Responses ---

// AgentResponse represents an agent in API responses.
type AgentResponse struct {
	ID           string            `json:"id"`
	Hostname     string            `json:"hostname"`
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels,omitempty"`
	Version      string            `json:"version,omitempty"`
	OS           string            `json:"os,omitempty"`
	Arch         string            `json:"arch,omitempty"`
	LastSeenAt   time.Time         `json:"lastSeenAt"`
	RegisteredAt time.Time         `json:"registeredAt"`
}

// --- Deployment Responses ---

// DeploymentResponse represents a deployment in API responses.
type DeploymentResponse struct {
	ID          string     `json:"id"`
	Project     string     `json:"project"`
	Target      string     `json:"target"`
	Branch      string     `json:"branch"`
	Status      string     `json:"status"`
	TriggeredBy string     `json:"triggeredBy"`
	StartedAt   time.Time  `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
}

// ScheduledDeploymentResponse represents a scheduled deployment.
type ScheduledDeploymentResponse struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	ScheduledAt time.Time `json:"scheduledAt"`
}

// --- Settings Responses ---

// SettingsImportResponse represents the result of a settings import.
type SettingsImportResponse struct {
	Status   string `json:"status"`
	Imported int    `json:"imported"`
}

// --- Stats Response ---

// StatsResponse represents dashboard statistics.
type StatsResponse struct {
	Projects    int64 `json:"projects"`
	Deployments int64 `json:"deployments"`
	Agents      int64 `json:"agents"`
	Users       int64 `json:"users"`
}

// --- Agent Token Response ---

// AgentTokenResponse represents a newly generated agent registration token.
type AgentTokenResponse struct {
	AgentID string `json:"agentId"`
	Token   string `json:"token"`
	Expires string `json:"expires"`
}
