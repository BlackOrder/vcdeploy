// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"context"
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
func NewServices(store storage.Store, _ *zap.Logger) *Services {
	return &Services{
		Users:        users.New(store),
		Agents:       agents.New(store),
		Projects:     projects.New(store),
		ProjectTypes: projecttypes.New(store),
		Deployments:  deployments.New(store),
		Secrets:      secrets.New(store, nil), // KMS can be nil for CLI
		Audit:        audit.New(store),
		Sessions:     sessions.New(store),
		APIKeys:      apikeys.New(store),
		Settings:     settings.New(store, nil), // KMS can be nil for CLI
		Webhooks:     webhooks.New(store, nil), // KMS can be nil for CLI
		HostKeys:     hostkeys.New(store),
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
// Returns nil if services haven't been initialized via InitServices.
// Callers should check HasServices() first or handle nil return.
func (c *AppContext) Services() *Services {
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
	store  storage.Store
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

	// Use the db as a Store interface (DB implements storage.Store)
	var store storage.Store = db

	// Initialize KMS for encrypted operations (secrets, settings)
	kms, err := security.NewKMS(context.Background(), store, logger)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("initialize KMS: %w", err)
	}

	svc := &CLIServices{
		Services: &Services{
			Users:        users.New(store),
			Agents:       agents.New(store),
			Projects:     projects.New(store),
			ProjectTypes: projecttypes.New(store),
			Deployments:  deployments.New(store),
			Secrets:      secrets.New(store, kms),
			Audit:        audit.New(store),
			Sessions:     sessions.New(store),
			APIKeys:      apikeys.New(store),
			Settings:     settings.New(store, kms),
			Webhooks:     webhooks.New(store, kms),
			HostKeys:     hostkeys.New(store),
		},
		store:  store,
		kms:    kms,
		logger: logger,
	}

	cleanup := func() {
		store.Close()
	}

	return svc, cleanup, nil
}

// Store returns the underlying storage interface.
// Use sparingly - prefer service methods when possible.
func (s *CLIServices) Store() storage.Store {
	return s.store
}

// KMS returns the key management service.
func (s *CLIServices) KMS() *security.KMS {
	return s.kms
}
