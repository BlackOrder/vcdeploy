// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// AppContext holds shared dependencies for CLI commands.
// This enables dependency injection for testability.
type AppContext struct {
	// Logger is the application logger
	Logger *zap.Logger

	// Config is the master configuration
	Config *config.MasterConfig

	// Storage is the database connection (implements storage.Store interface)
	Storage storage.Store

	// Stdout is the output writer (defaults to os.Stdout)
	Stdout io.Writer

	// Stderr is the error writer (defaults to os.Stderr)
	Stderr io.Writer

	// Stdin is the input reader (defaults to os.Stdin)
	Stdin io.Reader

	// Context is the base context for operations
	Context context.Context

	// ConfigPath is the path to the config file
	ConfigPath string

	// DryRun indicates if operations should be simulated
	DryRun bool
}

// NewAppContext creates a new AppContext with default values.
func NewAppContext() *AppContext {
	return &AppContext{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Stdin:   os.Stdin,
		Context: context.Background(),
	}
}

// WithLogger sets the logger and returns the context for chaining.
func (c *AppContext) WithLogger(logger *zap.Logger) *AppContext {
	c.Logger = logger
	return c
}

// WithConfig sets the config and returns the context for chaining.
func (c *AppContext) WithConfig(cfg *config.MasterConfig) *AppContext {
	c.Config = cfg
	return c
}

// WithStorage sets the storage and returns the context for chaining.
func (c *AppContext) WithStorage(store storage.Store) *AppContext {
	c.Storage = store
	return c
}

// WithStdout sets the stdout writer and returns the context for chaining.
func (c *AppContext) WithStdout(w io.Writer) *AppContext {
	c.Stdout = w
	return c
}

// WithStderr sets the stderr writer and returns the context for chaining.
func (c *AppContext) WithStderr(w io.Writer) *AppContext {
	c.Stderr = w
	return c
}

// WithStdin sets the stdin reader and returns the context for chaining.
func (c *AppContext) WithStdin(r io.Reader) *AppContext {
	c.Stdin = r
	return c
}

// WithContext sets the base context and returns the AppContext for chaining.
func (c *AppContext) WithContext(ctx context.Context) *AppContext {
	c.Context = ctx
	return c
}

// WithConfigPath sets the config path and returns the context for chaining.
func (c *AppContext) WithConfigPath(path string) *AppContext {
	c.ConfigPath = path
	return c
}

// WithDryRun sets the dry-run flag and returns the context for chaining.
func (c *AppContext) WithDryRun(dryRun bool) *AppContext {
	c.DryRun = dryRun
	return c
}

// Printf writes formatted output to Stdout.
func (c *AppContext) Printf(format string, args ...interface{}) {
	fmt.Fprintf(c.Stdout, format, args...)
}

// Println writes a line to Stdout.
func (c *AppContext) Println(args ...interface{}) {
	fmt.Fprintln(c.Stdout, args...)
}

// Errorf writes formatted output to Stderr.
func (c *AppContext) Errorf(format string, args ...interface{}) {
	fmt.Fprintf(c.Stderr, format, args...)
}

// Errorln writes a line to Stderr.
func (c *AppContext) Errorln(args ...interface{}) {
	fmt.Fprintln(c.Stderr, args...)
}

// LoadConfig loads the configuration from ConfigPath.
func (c *AppContext) LoadConfig() error {
	if c.ConfigPath == "" {
		sysCfg, err := config.GetSystemConfig()
		if err != nil {
			return fmt.Errorf("load system config: %w", err)
		}
		c.ConfigPath = sysCfg.MasterConfigPath()
	}

	cfg, err := config.LoadMasterConfig(c.ConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	c.Config = cfg
	return nil
}

// InitLogger initializes the logger based on config.
func (c *AppContext) InitLogger() error {
	if c.Config == nil {
		return fmt.Errorf("config not loaded")
	}

	// Default to development config
	zapCfg := zap.NewDevelopmentConfig()

	// Set log level from config
	if c.Config.Logs.Application.Level != "" {
		switch c.Config.Logs.Application.Level {
		case "debug":
			zapCfg.Level.SetLevel(zap.DebugLevel)
		case "info":
			zapCfg.Level.SetLevel(zap.InfoLevel)
		case "warn":
			zapCfg.Level.SetLevel(zap.WarnLevel)
		case "error":
			zapCfg.Level.SetLevel(zap.ErrorLevel)
		}
	}

	logger, err := zapCfg.Build()
	if err != nil {
		return fmt.Errorf("build logger: %w", err)
	}
	c.Logger = logger
	return nil
}

// OpenStorage opens the database connection.
// If Storage.UseMemoryCache is enabled (default), uses an in-memory cache with
// batched SQLite persistence to eliminate SQLITE_BUSY errors from concurrent access.
func (c *AppContext) OpenStorage() error {
	if c.Config == nil {
		return fmt.Errorf("config not loaded")
	}

	// Use default database path if not configured
	sysCfg, err := config.GetSystemConfig()
	if err != nil {
		return fmt.Errorf("load system config: %w", err)
	}
	dbPath := sysCfg.DatabasePath()
	if c.Config.Backup.Database.Path != "" {
		dbPath = c.Config.Backup.Database.Path
	}

	// Use memory-cached store if enabled (default)
	if c.Config.Storage.UseMemoryCache {
		cachedStore, err := storage.NewCachedStore(dbPath, c.Logger)
		if err != nil {
			return fmt.Errorf("open cached storage: %w", err)
		}
		c.Storage = cachedStore
		return nil
	}

	// Fall back to direct SQLite access
	db, err := storage.New(dbPath, c.Logger)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	c.Storage = db
	return nil
}

// Close cleans up resources.
func (c *AppContext) Close() {
	if c.Storage != nil {
		_ = c.Storage.Close() // #nosec G104 - best effort cleanup
	}
	if c.Logger != nil {
		_ = c.Logger.Sync()
	}
}

// CommandFactory creates command runners with injected dependencies.
// This pattern allows testing CLI commands by providing mock dependencies.
type CommandFactory struct {
	ctx *AppContext
}

// NewCommandFactory creates a new CommandFactory with the given context.
func NewCommandFactory(ctx *AppContext) *CommandFactory {
	return &CommandFactory{ctx: ctx}
}

// Context returns the underlying AppContext.
func (f *CommandFactory) Context() *AppContext {
	return f.ctx
}

// ProjectListRunner creates a project list command runner.
type ProjectListRunner struct {
	ctx *AppContext
}

// NewProjectListRunner creates a new project list runner.
func NewProjectListRunner(ctx *AppContext) *ProjectListRunner {
	return &ProjectListRunner{ctx: ctx}
}

// Run executes the project list command.
func (r *ProjectListRunner) Run() error {
	projects, err := r.ctx.Storage.ListProjects(context.Background())
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	if len(projects) == 0 {
		r.ctx.Println("No projects configured.")
		return nil
	}

	r.ctx.Printf("%-20s %-15s %-40s\n", "NAME", "TYPE", "REPOSITORY")
	r.ctx.Println("------------------------------------------------------------")

	for _, p := range projects {
		r.ctx.Printf("%-20s %-15s %-40s\n", p.Name, derefTypeID(p.TypeID), p.Repository)
	}

	return nil
}

// VersionRunner executes the version command.
type VersionRunner struct {
	ctx     *AppContext
	version string
	commit  string
	build   string
}

// NewVersionRunner creates a new version runner.
func NewVersionRunner(ctx *AppContext, v, c, b string) *VersionRunner {
	return &VersionRunner{
		ctx:     ctx,
		version: v,
		commit:  c,
		build:   b,
	}
}

// Run executes the version command.
func (r *VersionRunner) Run() error {
	r.ctx.Printf("vcdeploy %s\n", r.version)
	r.ctx.Printf("  commit:     %s\n", r.commit)
	r.ctx.Printf("  build time: %s\n", r.build)
	return nil
}

// MasterStatusRunner executes the master status command.
type MasterStatusRunner struct {
	ctx        *AppContext
	masterAddr string
}

// NewMasterStatusRunner creates a new master status runner.
func NewMasterStatusRunner(ctx *AppContext) *MasterStatusRunner {
	return &MasterStatusRunner{
		ctx:        ctx,
		masterAddr: "localhost:9000",
	}
}

// SetMasterAddr sets the master address to check.
func (r *MasterStatusRunner) SetMasterAddr(addr string) {
	r.masterAddr = addr
}

// Run executes the master status command.
func (r *MasterStatusRunner) Run() error {
	r.ctx.Printf("Checking master at %s...\n\n", r.masterAddr)

	// Try to call the health endpoint
	healthURL := fmt.Sprintf("http://%s/api/v1/health", r.masterAddr)
	statsURL := fmt.Sprintf("http://%s/api/v1/stats", r.masterAddr)

	client := &http.Client{Timeout: 5 * time.Second}

	// Check health endpoint
	healthCtx, healthCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer healthCancel()
	healthReq, err := http.NewRequestWithContext(healthCtx, "GET", healthURL, http.NoBody)
	if err != nil {
		r.ctx.Println("Master server status: OFFLINE")
		r.ctx.Printf("  Could not connect to %s\n", r.masterAddr)
		return nil
	}
	healthResp, err := client.Do(healthReq)
	if err != nil {
		r.ctx.Println("Master server status: OFFLINE")
		r.ctx.Printf("  Could not connect to %s\n", r.masterAddr)
		r.ctx.Println("\nTip: If using systemd, check with: systemctl status vcdeploy-master")
		return nil
	}
	defer healthResp.Body.Close()

	if healthResp.StatusCode != http.StatusOK {
		r.ctx.Println("Master server status: UNHEALTHY")
		r.ctx.Printf("  Health check returned: %d\n", healthResp.StatusCode)
		return nil
	}

	r.ctx.Println("Master server status: ONLINE")

	// Get stats for more details
	statsCtx, statsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer statsCancel()
	statsReq, _ := http.NewRequestWithContext(statsCtx, "GET", statsURL, http.NoBody)
	statsResp, err := client.Do(statsReq)
	if err == nil {
		defer statsResp.Body.Close()
		if statsResp.StatusCode == http.StatusOK {
			var stats map[string]interface{}
			body, _ := io.ReadAll(statsResp.Body)
			if json.Unmarshal(body, &stats) == nil {
				r.ctx.Println()
				if projects, ok := stats["projects"].(float64); ok {
					r.ctx.Printf("  Projects: %.0f\n", projects)
				}
				if agents, ok := stats["connected_agents"].(float64); ok {
					r.ctx.Printf("  Connected Agents: %.0f\n", agents)
				}
			}
		}
	}

	r.ctx.Printf("\n  Address: %s\n", r.masterAddr)
	return nil
}

func derefTypeID(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
