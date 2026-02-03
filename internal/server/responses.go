// Package server provides API response types for consistent JSON formatting.
package server

import "time"

// --- Standard Response Types ---

// StatusResponse is used for simple status messages (delete, update, etc.)
type StatusResponse struct {
	Status string `json:"status"`
}

// ErrorResponse is the standard error response format for all API endpoints.
// H14 FIX: Updated to match actual usage pattern and now used by jsonError.
type ErrorResponse struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
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

// --- Pagination Responses ---

// PaginatedResponse is the standard response format for paginated list endpoints.
type PaginatedResponse struct {
	Items      interface{} `json:"items"`
	TotalCount int64       `json:"totalCount"`
	Limit      int         `json:"limit,omitempty"`
	Offset     int         `json:"offset,omitempty"`
}

// --- SSH Key Responses ---

// SSHKeyResponse represents an SSH key in API responses.
type SSHKeyResponse struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Fingerprint string    `json:"fingerprint"`
	PublicKey   string    `json:"publicKey"`
	CreatedAt   time.Time `json:"createdAt"`
}

// --- Credential Responses ---

// CredentialResponse represents a source credential in API responses.
type CredentialResponse struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	URLPattern string    `json:"urlPattern"`
	CreatedAt  time.Time `json:"createdAt"`
}

// --- Provision Job Responses ---

// ProvisionJobResponse represents a provisioning job in API responses.
type ProvisionJobResponse struct {
	ID           string     `json:"id"`
	TargetHost   string     `json:"targetHost"`
	TargetPort   int        `json:"targetPort"`
	TargetUser   string     `json:"targetUser"`
	Status       string     `json:"status"`
	Stage        string     `json:"stage"`
	Progress     int        `json:"progress"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	StartedAt    time.Time  `json:"startedAt"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
}

// ProvisionLogsResponse represents provisioning logs in API responses.
type ProvisionLogsResponse struct {
	JobID  string               `json:"job_id"`
	Status string               `json:"status"`
	Stage  string               `json:"stage"`
	Logs   []*ProvisionLogEntry `json:"logs"`
}

// ProvisionLogEntry represents a single provisioning log entry.
type ProvisionLogEntry struct {
	ID        int64     `json:"id"`
	JobID     string    `json:"jobId"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

// --- Host Key Responses ---

// HostKeyResponse represents an SSH host key in API responses.
type HostKeyResponse struct {
	ID         int64     `json:"id"`
	Hostname   string    `json:"hostname"`
	Port       int       `json:"port"`
	KeyType    string    `json:"keyType"`
	PublicKey  string    `json:"publicKey"`
	Trusted    bool      `json:"trusted"`
	VerifiedBy string    `json:"verifiedBy,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// --- Jump Server Responses ---

// JumpServerResponse represents an SSH jump server in API responses.
type JumpServerResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	SSHKeyID  *int64    `json:"sshKeyId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// --- Health Check Responses ---

// HealthCheckConfigResponse represents a health check config in API responses.
type HealthCheckConfigResponse struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	URL               string `json:"url"`
	Method            string `json:"method"`
	ExpectedStatus    int    `json:"expectedStatus"`
	TimeoutSeconds    int    `json:"timeoutSeconds"`
	Retries           int    `json:"retries"`
	RetryDelaySeconds int    `json:"retryDelaySeconds"`
	Headers           string `json:"headers,omitempty"`
	Body              string `json:"body,omitempty"`
	ProjectID         *int64 `json:"projectId,omitempty"`
	Enabled           bool   `json:"enabled"`
	IsGlobal          bool   `json:"isGlobal"`
}

// --- Agent Binary Responses ---

// AgentBinaryResponse represents an agent binary in API responses.
type AgentBinaryResponse struct {
	ID        int64     `json:"id"`
	Version   string    `json:"version"`
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
	Checksum  string    `json:"checksum"`
	Size      int64     `json:"size"`
	IsCurrent bool      `json:"isCurrent"`
	CreatedAt time.Time `json:"createdAt"`
}

// --- Certificate Responses ---

// CertificateResponse represents a certificate in API responses.
type CertificateResponse struct {
	SerialNumber string    `json:"serialNumber"`
	Subject      string    `json:"subject"`
	Issuer       string    `json:"issuer"`
	NotBefore    time.Time `json:"notBefore"`
	NotAfter     time.Time `json:"notAfter"`
	Status       string    `json:"status"`
}

// --- Project Type Responses ---

// ProjectTypeResponse represents a project type in API responses.
type ProjectTypeResponse struct {
	ID          int64     `json:"id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	BuildCmd    string    `json:"buildCmd,omitempty"`
	DeployCmd   string    `json:"deployCmd,omitempty"`
	RollbackCmd string    `json:"rollbackCmd,omitempty"`
	CleanupCmd  string    `json:"cleanupCmd,omitempty"`
	CreatedAt   time.Time `json:"createdAt,omitempty"`
}
