// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"errors"
	"fmt"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/services/agents"
	"github.com/BlackOrder/vcdeploy/internal/services/apikeys"
	"github.com/BlackOrder/vcdeploy/internal/services/audit"
	"github.com/BlackOrder/vcdeploy/internal/services/deployments"
	"github.com/BlackOrder/vcdeploy/internal/services/hostkeys"
	"github.com/BlackOrder/vcdeploy/internal/services/projects"
	"github.com/BlackOrder/vcdeploy/internal/services/projecttypes"
	"github.com/BlackOrder/vcdeploy/internal/services/secrets"
	"github.com/BlackOrder/vcdeploy/internal/services/sessions"
	"github.com/BlackOrder/vcdeploy/internal/services/settings"
	"github.com/BlackOrder/vcdeploy/internal/services/users"
	"github.com/BlackOrder/vcdeploy/internal/services/webhooks"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// Services contains all service instances for CLI commands.
// This provides a clean abstraction over the storage layer,
// allowing CLI commands to use business logic without direct DB access.
type Services struct {
	Users        services.UserServicer
	Agents       services.AgentServicer
	Projects     services.ProjectServicer
	ProjectTypes services.ProjectTypeServicer
	Deployments  services.DeploymentServicer
	Secrets      services.SecretServicer
	Audit        services.AuditServicer
	Sessions     services.SessionServicer
	APIKeys      services.APIKeyServicer
	Settings     services.SettingsServicer
	Webhooks     services.WebhookServicer
	HostKeys     services.HostKeyServicer
}

// NewServices creates a new service container with all services initialized.
func NewServices(db *storage.DB, _ *zap.Logger) *Services {
	return &Services{
		Users:        users.New(db),
		Agents:       agents.New(db),
		Projects:     projects.New(db),
		ProjectTypes: projecttypes.New(db),
		Deployments:  deployments.New(db),
		Secrets:      secrets.New(db, nil), // KMS can be nil for CLI
		Audit:        audit.New(db),
		Sessions:     sessions.New(db),
		APIKeys:      apikeys.New(db),
		Settings:     settings.New(db, nil), // KMS can be nil for CLI
		Webhooks:     webhooks.New(db, nil), // KMS can be nil for CLI
		HostKeys:     hostkeys.New(db),
	}
}

// Services field for AppContext - lazy initialization
var appServices *Services

// InitServices initializes services on the AppContext.
// Must be called after OpenStorage.
func (c *AppContext) InitServices() error {
	if c.Storage == nil {
		return errStorageNotOpen
	}

	appServices = NewServices(c.Storage, c.Logger)
	return nil
}

// Services returns the service container.
// Panics if services haven't been initialized via InitServices.
func (c *AppContext) Services() *Services {
	if appServices == nil {
		panic("services not initialized - call InitServices first")
	}
	return appServices
}

// HasServices returns true if services have been initialized.
func (c *AppContext) HasServices() bool {
	return appServices != nil
}

// errStorageNotOpen is returned when services are initialized without storage.
var errStorageNotOpen = errors.New("storage not open - call OpenStorage first")

// CLIServices wraps services with an open database connection for CLI use.
// It provides a convenient way for CLI commands to access services.
type CLIServices struct {
	*Services
	db     *storage.DB
	kms    *security.KMS
	logger *zap.Logger
}

// InitCLIServices initializes services from a database path for CLI command use.
// Returns services and a cleanup function that must be called when done.
func InitCLIServices(dbPath string) (*CLIServices, func(), error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}

	logger := zap.NewNop()

	// Initialize KMS for encrypted operations (secrets, settings)
	kms, err := security.NewKMS(db.Conn(), logger)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("initialize KMS: %w", err)
	}

	svc := &CLIServices{
		Services: &Services{
			Users:        users.New(db),
			Agents:       agents.New(db),
			Projects:     projects.New(db),
			ProjectTypes: projecttypes.New(db),
			Deployments:  deployments.New(db),
			Secrets:      secrets.New(db, kms),
			Audit:        audit.New(db),
			Sessions:     sessions.New(db),
			APIKeys:      apikeys.New(db),
			Settings:     settings.New(db, kms),
			Webhooks:     webhooks.New(db, kms),
			HostKeys:     hostkeys.New(db),
		},
		db:     db,
		kms:    kms,
		logger: logger,
	}

	cleanup := func() {
		db.Close()
	}

	return svc, cleanup, nil
}

// DB returns the underlying database connection.
// Use sparingly - prefer service methods when possible.
func (s *CLIServices) DB() *storage.DB {
	return s.db
}

// KMS returns the key management service.
func (s *CLIServices) KMS() *security.KMS {
	return s.kms
}
