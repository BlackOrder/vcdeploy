// Package commands implements the CLI commands for vcdeploy-agent.
package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/agent"
	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

// SetVersionInfo sets the version information from main.
func SetVersionInfo(v, c, b string) {
	version = v
	commit = c
	buildTime = b
}

var rootCmd = &cobra.Command{
	Use:   "vcdeploy-agent",
	Short: "vcdeploy Agent - Deployment executor",
	Long: `vcdeploy-agent is the deployment executor that runs on target servers.

It connects to the vcdeploy master via gRPC and executes deployment commands locally.`,
	SilenceUsage: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	sysCfg := config.MustGetSystemConfig()
	rootCmd.PersistentFlags().String("config", sysCfg.AgentConfigPath(), "config file")

	// Add subcommands
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(registerCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("vcdeploy-agent %s\n", version)
		fmt.Printf("  commit:     %s\n", commit)
		fmt.Printf("  build time: %s\n", buildTime)
	},
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the agent",
	Long:  "Start the agent and connect to the master server.",
	RunE:  runStart,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show agent status",
	RunE:  runStatus,
}

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Register agent with master",
	Long:  "Register this agent with the master server using a token.",
	RunE:  runRegister,
}

func init() {
	startCmd.Flags().String("master", "", "Master address (overrides config)")
	startCmd.Flags().String("token", "", "Registration token (for first connection)")

	registerCmd.Flags().String("master", "", "Master address")
	registerCmd.Flags().String("token", "", "Registration token")
	_ = registerCmd.MarkFlagRequired("master")
	_ = registerCmd.MarkFlagRequired("token")
}

func runStart(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")
	master, _ := cmd.Flags().GetString("master")
	token, _ := cmd.Flags().GetString("token")

	fmt.Printf("Starting agent...\n")
	fmt.Printf("  config: %s\n", configPath)

	// Load configuration
	cfg, err := config.LoadAgentConfig(configPath)
	if err != nil {
		// If config doesn't exist and master flag is provided, create a default config
		if os.IsNotExist(err) && master != "" {
			cfg = config.DefaultAgentConfig()
		} else {
			return fmt.Errorf("loading config: %w", err)
		}
	}

	// Override config with command line flags
	if master != "" {
		cfg.Master.Address = master
	}
	if token != "" {
		cfg.Master.Token = token
	}

	// Generate agent ID if not set
	if cfg.Agent.ID == "" {
		hostname, _ := os.Hostname()
		cfg.Agent.ID = hostname
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Create logger
	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	// Create agent
	ag, err := agent.NewAgent(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating agent: %w", err)
	}

	// Write PID file for status checks and process management
	sysCfg := config.MustGetSystemConfig()
	pidFile := sysCfg.AgentPIDPath()

	// Ensure PID directory exists
	pidDir := filepath.Dir(pidFile)
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		logger.Warn("Failed to create PID directory", zap.String("dir", pidDir), zap.Error(err))
	}

	// Write current PID
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
		logger.Warn("Failed to write PID file", zap.String("path", pidFile), zap.Error(err))
	}

	// Defer cleanup of PID file
	defer func() {
		if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
			logger.Warn("Failed to remove PID file", zap.String("path", pidFile), zap.Error(err))
		}
	}()

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh) // Cleanup signal handling on exit

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic in signal handler", zap.Any("panic", r))
			}
		}()
		sig := <-sigCh
		logger.Info("Received signal, shutting down", zap.String("signal", sig.String()))
		cancel()
	}()

	// Start the agent
	logger.Info("Agent starting",
		zap.String("id", cfg.Agent.ID),
		zap.String("master", cfg.Master.Address),
	)

	if err := ag.Start(ctx); err != nil && ctx.Err() == nil {
		return fmt.Errorf("agent error: %w", err)
	}

	logger.Info("Agent stopped")
	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	// Try to load config to get agent info
	cfg, err := config.LoadAgentConfig(configPath)
	if err != nil {
		fmt.Println("Agent status: NOT CONFIGURED")
		fmt.Printf("  config file: %s (not found or invalid)\n", configPath)
		return nil
	}

	// Check if agent process is running via PID file
	sysCfg := config.MustGetSystemConfig()
	pidFile := sysCfg.AgentPIDPath()
	isRunning, pid := checkAgentProcess(pidFile)

	fmt.Printf("Agent ID: %s\n", cfg.Agent.ID)
	fmt.Printf("Master: %s\n", cfg.Master.Address)

	if isRunning {
		fmt.Printf("Status: RUNNING (PID %d)\n", pid)
	} else {
		// Check if we can connect to master
		fmt.Printf("Status: STOPPED\n")
	}

	fmt.Printf("\nConfiguration:\n")
	fmt.Printf("  repos path: %s\n", cfg.Paths.Repos)
	fmt.Printf("  releases path: %s\n", cfg.Paths.Releases)
	fmt.Printf("  config file: %s\n", configPath)

	if !isRunning {
		fmt.Println("\nTo start the agent:")
		fmt.Println("  vcdeploy-agent start")
		fmt.Println("  or: systemctl start vcdeploy-agent")
	}

	return nil
}

// checkAgentProcess checks if the agent process is running by reading its PID file.
func checkAgentProcess(pidFile string) (bool, int) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false, 0
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return false, 0
	}

	// Check if process is running by sending signal 0
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, 0
	}

	// On Unix, FindProcess always succeeds; we need to send signal 0 to check
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false, 0
	}

	return true, pid
}

func runRegister(cmd *cobra.Command, args []string) error {
	master, _ := cmd.Flags().GetString("master")
	token, _ := cmd.Flags().GetString("token")
	configPath, _ := cmd.Flags().GetString("config")

	fmt.Printf("Registering agent with master: %s\n", master)

	// Create logger
	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	// Create or load config
	var cfg *config.AgentConfig
	cfg, err = config.LoadAgentConfig(configPath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			cfg = config.DefaultAgentConfig()
			// Clear the default cert path for initial registration
			// The agent will use insecure credentials until it receives its certificate
			cfg.Master.Cert = ""
		} else {
			return fmt.Errorf("loading config: %w", err)
		}
	}

	cfg.Master.Address = master
	cfg.Master.Token = token

	// Generate agent ID from hostname if not set
	if cfg.Agent.ID == "" {
		hostname, _ := os.Hostname()
		cfg.Agent.ID = hostname
	}

	// Create agent and register
	ag, err := agent.NewAgent(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating agent: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cert, caCert, err := ag.Register(ctx, token)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	// Save certificates
	sysCfg := config.MustGetSystemConfig()
	agentCertsDir := sysCfg.CertsDir() + "/agent"
	certPath := agentCertsDir + "/cert.pem"
	caPath := agentCertsDir + "/ca.pem"

	if err := os.MkdirAll(agentCertsDir, 0o755); err != nil {
		return fmt.Errorf("creating agent directory: %w", err)
	}

	if len(cert) > 0 {
		if err := os.WriteFile(certPath, cert, 0o600); err != nil {
			return fmt.Errorf("saving certificate: %w", err)
		}
		cfg.Master.Cert = certPath
	}

	if len(caCert) > 0 {
		if err := os.WriteFile(caPath, caCert, 0o600); err != nil {
			return fmt.Errorf("saving CA certificate: %w", err)
		}
	}

	// Clear the token from config (used only for initial registration)
	cfg.Master.Token = ""

	// Save updated config
	if err := config.SaveAgentConfig(cfg, configPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Registration successful!\n")
	fmt.Printf("  Agent ID: %s\n", cfg.Agent.ID)
	fmt.Printf("  Config saved to: %s\n", configPath)
	fmt.Printf("  Certificate saved to: %s\n", certPath)

	return nil
}
