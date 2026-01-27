// Package storage provides database operations for vcdeploy.
package storage

import "time"

// --- User Model ---

// User represents a user in the system.
type User struct {
	ID                 int64
	Username           string
	PasswordHash       string
	Email              string
	Role               string
	TOTPSecret         string
	TOTPEnabled        bool
	MustChangePassword bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// --- Agent Model ---

// Agent represents a connected agent.
type Agent struct {
	ID           string
	Hostname     string
	Labels       map[string]string
	Capabilities string // JSON string
	Status       string
	LastSeenAt   time.Time
	RegisteredAt time.Time
	Certificate  string
}

// --- Deployment Models ---

// DeploymentRecord represents a deployment record in the database.
// Note: This is distinct from deploy.DeploymentRun which represents a runtime deployment.
type DeploymentRecord struct {
	ID            string
	Project       string
	Target        string
	Branch        string
	CommitHash    string
	Status        string
	ReleaseNumber int
	StartedAt     time.Time
	CompletedAt   *time.Time
	TriggeredBy   string
	TriggerSource string
	ErrorMessage  string
}

// DeploymentLog represents a log entry for a deployment.
type DeploymentLog struct {
	ID           int64
	DeploymentID string
	Level        string
	Message      string
	Source       string
	CreatedAt    time.Time
}

// DeploymentCLI is a simplified deployment struct for CLI use.
type DeploymentCLI struct {
	ID          string
	ProjectID   int64
	ProjectName string
	Target      string
	Status      string
	TriggeredBy string
	StartedAt   time.Time
	FinishedAt  *time.Time
}

// ScheduledDeployment represents a deployment scheduled for future execution.
type ScheduledDeployment struct {
	ID          string
	Project     string
	Target      string
	Branch      string
	ScheduledAt time.Time
	ScheduledBy string
	Status      string
}

// --- Audit Model ---

// AuditEntry represents an audit log entry.
type AuditEntry struct {
	ID        int64
	Timestamp time.Time
	Source    string
	User      string
	Action    string
	Resource  string
	Details   string
	IPAddress string
	Result    string
}

// --- Secret Models ---

// Secret represents an encrypted secret.
type Secret struct {
	ID             int64
	Project        string
	Scope          string
	Key            string
	ValueEncrypted []byte
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SecretInfo represents secret metadata.
type SecretInfo struct {
	Key       string
	Scope     string
	UpdatedAt time.Time
}

// --- Project Models ---

// Project represents a deployment project.
type Project struct {
	ID               int64
	Name             string
	Repository       string
	Branch           string
	DeployPath       string
	Type             string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	LastDeployAt     *time.Time
	LastDeployStatus string
}

// ProjectType represents a project type template.
type ProjectType struct {
	ID           int64
	Name         string
	Description  string
	BuildCmd     string
	ProjectCount int
	CreatedAt    time.Time
}

// ProjectWebhook represents a project-specific webhook configuration.
type ProjectWebhook struct {
	ID              int64
	ProjectID       int64
	Provider        string
	SecretEncrypted []byte
	Enabled         bool
	RequireSecret   bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// --- Session Model ---

// Session represents a user session.
type Session struct {
	ID        string // TEXT primary key
	UserID    int64
	Token     string // Same as ID for sessions
	IPAddress string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
	LastUsed  time.Time
}

// --- API Key Model ---

// APIKey represents an API key.
type APIKey struct {
	ID         int64
	UserID     int64
	Name       string
	KeyHash    string // SHA-256 hash of the key
	KeyPrefix  string // First 8 characters of the key for identification
	Scopes     string // JSON array of scopes/permissions
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// IsValid checks if an API key is valid (not expired).
func (key *APIKey) IsValid() bool {
	if key == nil {
		return false
	}
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return false
	}
	return true
}

// --- Setting Model ---

// Setting represents a configuration setting.
type Setting struct {
	ID          int64
	Category    string
	Key         string
	Value       string
	ValueType   string
	Encrypted   bool
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// --- SSH Host Key Model ---

// SSHHostKey represents a stored SSH host key.
type SSHHostKey struct {
	ID          int64
	Hostname    string
	Port        int
	KeyType     string
	PublicKey   string // Base64 encoded public key
	Fingerprint string // SHA256 fingerprint
	Trusted     bool
	AddedBy     string
	VerifiedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// --- SSH Jump Server Model ---

// SSHJumpServer represents a bastion/jump server for SSH connections.
type SSHJumpServer struct {
	ID        int64
	Name      string
	Host      string
	Port      int
	Username  string
	SSHKeyID  *int64 // Optional foreign key to ssh_keys
	CreatedAt time.Time
}

// --- Rate Limit Models ---

// BlockedIP represents a blocked IP address.
type BlockedIP struct {
	ID        int64
	IPAddress string
	Reason    string
	BlockedAt time.Time
	ExpiresAt time.Time
	BlockedBy string
}

// RateLimitRecord represents a rate limit tracking record.
type RateLimitRecord struct {
	ID          int64
	Key         string
	Bucket      string
	Count       int
	WindowStart time.Time
	WindowEnd   time.Time
}

// --- Provision Job Models ---

// ProvisionJob represents an agent provisioning job.
type ProvisionJob struct {
	ID            string
	TargetHost    string
	TargetPort    int
	TargetUser    string
	SSHKeyID      *int64
	AgentBinaryID *int64
	Status        string
	Stage         string
	Progress      int
	ErrorMessage  string
	RollbackData  string
	StartedAt     time.Time
	CompletedAt   *time.Time
}

// AgentBinary represents an agent binary release.
type AgentBinary struct {
	ID             int64
	Version        string
	OS             string
	Arch           string
	Path           string
	ChecksumSHA256 string
	SizeBytes      int64
	UploadedAt     time.Time
	IsCurrent      bool
}
