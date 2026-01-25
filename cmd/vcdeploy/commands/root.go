// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/server"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/term"
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
	Use:   "vcdeploy",
	Short: "vcdeploy - Deployment Platform",
	Long: `vcdeploy is a deployment platform with master-agent architecture.

It supports:
  - Webhook-driven deployments from GitHub, GitLab, Bitbucket
  - Agent-based and SSH-based deployment targets
  - Symlink-based zero-downtime deployments
  - Centralized configuration and secrets management
  - Full web UI for management`,
	SilenceUsage: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().String("config", "", "config file (default: /etc/vcdeploy/master.yaml)")
	rootCmd.PersistentFlags().String("master", "", "master address for remote CLI (e.g., localhost:9000)")
	rootCmd.PersistentFlags().String("token", "", "API token for remote CLI")

	// Add subcommands
	rootCmd.AddCommand(masterCmd)
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(typeCmd)
	rootCmd.AddCommand(secretCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("vcdeploy %s\n", version)
		fmt.Printf("  commit:     %s\n", commit)
		fmt.Printf("  build time: %s\n", buildTime)
	},
}

// masterCmd handles master-related commands
var masterCmd = &cobra.Command{
	Use:   "master",
	Short: "Master server management",
	Long:  "Commands for managing the vcdeploy master server.",
}

// projectCmd handles project-related commands
var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Project management",
	Long:  "Commands for managing deployment projects.",
}

// typeCmd handles project type commands
var typeCmd = &cobra.Command{
	Use:   "type",
	Short: "Project type management",
	Long:  "Commands for managing project types (templates).",
}

// secretCmd handles secret management commands
var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Secrets management",
	Long:  "Commands for managing deployment secrets.",
}

func init() {
	// Master subcommands
	masterCmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start the master server",
		RunE:  runMasterStart,
	})
	masterCmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the master server",
		RunE:  runMasterStop,
	})
	masterCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show master server status",
		RunE:  runMasterStatus,
	})
	masterCmd.AddCommand(&cobra.Command{
		Use:   "rotate-key",
		Short: "Rotate the master encryption key",
		RunE:  runMasterRotateKey,
	})
	masterCmd.AddCommand(masterBackupCmd)

	// Master backup subcommands
	masterBackupCmd.AddCommand(&cobra.Command{
		Use:   "create",
		Short: "Create a database backup",
		RunE:  runBackupCreate,
	})
	masterBackupCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List available backups",
		RunE:  runBackupList,
	})
	masterBackupCmd.AddCommand(&cobra.Command{
		Use:   "restore [backup-file]",
		Short: "Restore from a backup",
		Args:  cobra.ExactArgs(1),
		RunE:  runBackupRestore,
	})

	// Project subcommands
	projectCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all projects",
		RunE:  runProjectList,
	})
	projectCmd.AddCommand(&cobra.Command{
		Use:   "add [name]",
		Short: "Add a new project",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectAdd,
	})
	editProjectCmd := &cobra.Command{
		Use:   "edit [name]",
		Short: "Edit a project",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectEdit,
	}
	editProjectCmd.Flags().String("repo", "", "Set repository URL")
	editProjectCmd.Flags().String("branch", "", "Set default branch")
	editProjectCmd.Flags().String("path", "", "Set deploy path")
	editProjectCmd.Flags().String("type", "", "Set project type")
	projectCmd.AddCommand(editProjectCmd)
	projectCmd.AddCommand(&cobra.Command{
		Use:   "delete [name]",
		Short: "Delete a project",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectDelete,
	})
	projectCmd.AddCommand(&cobra.Command{
		Use:   "validate [name]",
		Short: "Validate a project configuration",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectValidate,
	})

	deployCmd := &cobra.Command{
		Use:   "deploy [name]",
		Short: "Deploy a project",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectDeploy,
	}
	deployCmd.Flags().String("target", "", "Target to deploy to")
	deployCmd.Flags().Bool("dry-run", false, "Validate without deploying")
	deployCmd.Flags().Bool("force", false, "Force deploy (bypass lock)")
	projectCmd.AddCommand(deployCmd)

	rollbackCmd := &cobra.Command{
		Use:   "rollback [name]",
		Short: "Rollback a project to previous release",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectRollback,
	}
	rollbackCmd.Flags().String("target", "", "Target to rollback")
	rollbackCmd.Flags().Int("release", 0, "Specific release number to rollback to")
	projectCmd.AddCommand(rollbackCmd)

	// Type subcommands
	typeCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all project types",
		RunE:  runTypeList,
	})
	typeCmd.AddCommand(&cobra.Command{
		Use:   "create [name]",
		Short: "Create a new project type",
		Args:  cobra.ExactArgs(1),
		RunE:  runTypeCreate,
	})
	editTypeCmd := &cobra.Command{
		Use:   "edit [name]",
		Short: "Edit a project type",
		Args:  cobra.ExactArgs(1),
		RunE:  runTypeEdit,
	}
	editTypeCmd.Flags().String("description", "", "Set type description")
	editTypeCmd.Flags().String("build-cmd", "", "Set build command")
	typeCmd.AddCommand(editTypeCmd)
	typeCmd.AddCommand(&cobra.Command{
		Use:   "delete [name]",
		Short: "Delete a project type",
		Args:  cobra.ExactArgs(1),
		RunE:  runTypeDelete,
	})

	// Secret subcommands
	setCmd := &cobra.Command{
		Use:   "set [project/scope] [key]",
		Short: "Set a secret value",
		Args:  cobra.ExactArgs(2),
		RunE:  runSecretSet,
	}
	setCmd.Flags().Bool("stdin", false, "Read value from stdin")
	secretCmd.AddCommand(setCmd)

	secretCmd.AddCommand(&cobra.Command{
		Use:   "list [project]",
		Short: "List secrets for a project",
		Args:  cobra.ExactArgs(1),
		RunE:  runSecretList,
	})
	secretCmd.AddCommand(&cobra.Command{
		Use:   "delete [project/scope] [key]",
		Short: "Delete a secret",
		Args:  cobra.ExactArgs(2),
		RunE:  runSecretDelete,
	})

	importCmd := &cobra.Command{
		Use:   "import [project/scope]",
		Short: "Import secrets from stdin (.env format)",
		Args:  cobra.ExactArgs(1),
		RunE:  runSecretImport,
	}
	secretCmd.AddCommand(importCmd)

	backupSecretCmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup all secrets (passphrase protected)",
		RunE:  runSecretBackup,
	}
	backupSecretCmd.Flags().StringP("output", "o", "", "Output file path")
	secretCmd.AddCommand(backupSecretCmd)

	secretCmd.AddCommand(&cobra.Command{
		Use:   "restore [backup-file]",
		Short: "Restore secrets from backup",
		Args:  cobra.ExactArgs(1),
		RunE:  runSecretRestore,
	})
}

var masterBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Database backup management",
}

// Global state for CLI operations
var (
	globalConfig *config.MasterConfig
	globalDB     *storage.DB
	globalLogger *zap.Logger
)

// getDBPath returns the database path from system config.
func getDBPath() string {
	sysCfg := config.MustGetSystemConfig()
	return sysCfg.DatabasePath()
}

// initConfig loads configuration from file
func initConfig(cmd *cobra.Command) error {
	cfgPath, _ := cmd.Flags().GetString("config")
	if cfgPath == "" {
		sysCfg := config.MustGetSystemConfig()
		cfgPath = sysCfg.MasterConfigPath()
	}

	cfg, err := config.LoadMaster(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	globalConfig = cfg
	return nil
}

// initLogger creates a zap logger
func initLogger(level string) error {
	var zapLevel zapcore.Level
	switch strings.ToLower(level) {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapLevel)
	config.EncoderConfig.TimeKey = "time"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := config.Build()
	if err != nil {
		return err
	}
	globalLogger = logger
	return nil
}

// runMasterStart starts the master server
func runMasterStart(cmd *cobra.Command, args []string) error {
	if err := initConfig(cmd); err != nil {
		return err
	}

	logLevel := "info"
	if globalConfig != nil && globalConfig.Logs.Application.Level != "" {
		logLevel = globalConfig.Logs.Application.Level
	}
	if err := initLogger(logLevel); err != nil {
		return err
	}
	defer globalLogger.Sync()

	globalLogger.Info("starting vcdeploy master",
		zap.String("version", version),
		zap.String("commit", commit),
	)

	// Initialize database
	sysCfg := config.MustGetSystemConfig()
	dbPath := sysCfg.DatabasePath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	globalDB = db

	// Create and start master server
	srv, err := server.NewMasterServer(globalConfig, db, globalLogger)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh) // Cleanup signal handling on exit

	errCh := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				globalLogger.Error("panic in server start", zap.Any("panic", r))
				errCh <- fmt.Errorf("server panicked: %v", r)
			}
		}()
		if err := srv.Start(ctx); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-sigCh:
		globalLogger.Info("shutting down gracefully...")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := srv.Stop(shutdownCtx); err != nil {
			globalLogger.Error("shutdown error", zap.Error(err))
		}
	case err := <-errCh:
		return err
	}

	return nil
}

func runMasterStop(cmd *cobra.Command, args []string) error {
	masterAddr, _ := cmd.Flags().GetString("master")
	if masterAddr == "" {
		masterAddr = "localhost:9000"
	}

	fmt.Println("Sending stop signal to master server...")

	// First, try to send a graceful shutdown request via API
	shutdownURL := fmt.Sprintf("http://%s/api/v1/shutdown", masterAddr)
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest(http.MethodPost, shutdownURL, nil)
	if err != nil {
		// If API fails, try PID file approach
		return tryPidFileStop()
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Could not reach master at %s, trying PID file...\n", masterAddr)
		return tryPidFileStop()
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		fmt.Println("Master server is shutting down gracefully.")
		return nil
	}

	// Fallback to PID file approach
	return tryPidFileStop()
}

// tryPidFileStop attempts to stop the master using its PID file.
func tryPidFileStop() error {
	sysCfg := config.MustGetSystemConfig()
	pidFile := sysCfg.MasterPIDPath()

	data, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Println("Could not find running master process.")
		fmt.Println("If using systemd, use: systemctl stop vcdeploy-master")
		return nil
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return fmt.Errorf("invalid PID file: %w", err)
	}

	// Send SIGTERM to the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		if err == os.ErrProcessDone {
			fmt.Println("Process already stopped.")
			return nil
		}
		return fmt.Errorf("failed to signal process: %w", err)
	}

	fmt.Printf("Sent SIGTERM to process %d\n", pid)
	return nil
}

func runMasterStatus(cmd *cobra.Command, args []string) error {
	masterAddr, _ := cmd.Flags().GetString("master")
	if masterAddr == "" {
		masterAddr = "localhost:9000"
	}

	fmt.Printf("Checking master at %s...\n\n", masterAddr)

	// Try to call the health/stats endpoint
	healthURL := fmt.Sprintf("http://%s/api/v1/health", masterAddr)
	statsURL := fmt.Sprintf("http://%s/api/v1/stats", masterAddr)

	client := &http.Client{Timeout: 5 * time.Second}

	// Check health endpoint
	healthResp, err := client.Get(healthURL)
	if err != nil {
		fmt.Println("Status: OFFLINE")
		fmt.Printf("  Could not connect to %s\n", masterAddr)

		// Check if process is running via PID file
		if pid := checkMasterPid(); pid > 0 {
			fmt.Printf("  PID file shows process %d may be starting up\n", pid)
		}

		fmt.Println("\nTip: If using systemd, check with: systemctl status vcdeploy-master")
		return nil
	}
	defer healthResp.Body.Close()

	if healthResp.StatusCode != http.StatusOK {
		fmt.Println("Status: UNHEALTHY")
		fmt.Printf("  Health check returned: %d\n", healthResp.StatusCode)
		return nil
	}

	fmt.Println("Status: ONLINE")

	// Get stats for more details
	statsResp, err := client.Get(statsURL)
	if err == nil {
		defer statsResp.Body.Close()
		if statsResp.StatusCode == http.StatusOK {
			var stats map[string]interface{}
			body, _ := io.ReadAll(statsResp.Body)
			if json.Unmarshal(body, &stats) == nil {
				fmt.Println()
				if projects, ok := stats["projects"].(float64); ok {
					fmt.Printf("  Projects: %.0f\n", projects)
				}
				if agents, ok := stats["connected_agents"].(float64); ok {
					fmt.Printf("  Connected Agents: %.0f\n", agents)
				}
				if pending, ok := stats["pending_deployments"].(float64); ok {
					fmt.Printf("  Pending Deployments: %.0f\n", pending)
				}
				if running, ok := stats["running_deployments"].(float64); ok {
					fmt.Printf("  Running Deployments: %.0f\n", running)
				}
				if uptime, ok := stats["uptime_seconds"].(float64); ok {
					fmt.Printf("  Uptime: %s\n", formatDuration(time.Duration(uptime)*time.Second))
				}
			}
		}
	}

	fmt.Printf("\n  Address: %s\n", masterAddr)
	return nil
}

// checkMasterPid reads the PID file and checks if process exists.
func checkMasterPid() int {
	sysCfg := config.MustGetSystemConfig()
	pidFile := sysCfg.MasterPIDPath()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return 0
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return 0
	}

	// On Unix, FindProcess always succeeds; we need to send signal 0 to check
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return 0
	}

	return pid
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0f seconds", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0f minutes", d.Minutes())
	}
	if d < 24*time.Hour {
		hours := d.Hours()
		mins := (d % time.Hour).Minutes()
		return fmt.Sprintf("%.0fh %.0fm", hours, mins)
	}
	days := d / (24 * time.Hour)
	hours := (d % (24 * time.Hour)).Hours()
	return fmt.Sprintf("%dd %.0fh", days, hours)
}

func runMasterRotateKey(cmd *cobra.Command, args []string) error {
	if err := initConfig(cmd); err != nil {
		return err
	}

	fmt.Print("WARNING: This will rotate the master encryption key.\n")
	fmt.Print("All existing secrets will be re-encrypted.\n")
	fmt.Print("Continue? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	fmt.Println("\nRotating master key...")

	// Open database
	sysCfg := config.MustGetSystemConfig()
	dbPath := sysCfg.DatabasePath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Initialize KMS
	kms, err := security.NewKMS(db.Conn(), globalLogger)
	if err != nil {
		return fmt.Errorf("initialize KMS: %w", err)
	}

	// Rotate the encryption key
	ctx := context.Background()
	newKey, err := kms.RotateKey(ctx)
	if err != nil {
		return fmt.Errorf("rotate key: %w", err)
	}
	fmt.Printf("New key generated: %s (version %d)\n", newKey.ID[:8]+"...", newKey.Version)

	// Re-encrypt all secrets with the new key
	fmt.Println("Re-encrypting secrets...")
	secrets := security.NewSecretService(db, kms)
	if err := secrets.ReEncryptAll(ctx); err != nil {
		return fmt.Errorf("re-encrypt secrets: %w", err)
	}

	fmt.Println("Key rotated successfully.")
	fmt.Println("\nNote: Make sure to restart the master server for changes to take effect.")
	return nil
}

func runBackupCreate(cmd *cobra.Command, args []string) error {
	if err := initConfig(cmd); err != nil {
		return err
	}

	backupPath := globalConfig.Backup.Database.Path
	if backupPath == "" {
		sysCfg := config.MustGetSystemConfig()
		backupPath = sysCfg.BackupsDir()
	}

	timestamp := time.Now().Format("20060102-150405")
	backupFile := fmt.Sprintf("%s/vcdeploy-%s.db.bak", backupPath, timestamp)

	fmt.Printf("Creating backup: %s\n", backupFile)

	// Ensure backup directory exists
	if err := os.MkdirAll(backupPath, 0750); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	// Open database and create backup
	sysCfg := config.MustGetSystemConfig()
	dbPath := sysCfg.DatabasePath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.Backup(backupFile); err != nil {
		return fmt.Errorf("create backup: %w", err)
	}

	fmt.Println("Backup created successfully.")
	return nil
}

func runBackupList(cmd *cobra.Command, args []string) error {
	if err := initConfig(cmd); err != nil {
		return err
	}

	backupPath := globalConfig.Backup.Database.Path
	if backupPath == "" {
		sysCfg := config.MustGetSystemConfig()
		backupPath = sysCfg.BackupsDir()
	}

	entries, err := os.ReadDir(backupPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No backups found.")
			return nil
		}
		return fmt.Errorf("read backup directory: %w", err)
	}

	fmt.Println("Available backups:")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSIZE\tCREATED")

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, _ := entry.Info()
		fmt.Fprintf(w, "%s\t%d MB\t%s\n",
			entry.Name(),
			info.Size()/(1024*1024),
			info.ModTime().Format("2006-01-02 15:04:05"),
		)
	}
	w.Flush()

	return nil
}

func runBackupRestore(cmd *cobra.Command, args []string) error {
	backupFile := args[0]

	// Check backup file exists
	if _, err := os.Stat(backupFile); err != nil {
		return fmt.Errorf("backup file not found: %s", backupFile)
	}

	// Get the database path
	dbPath := getDBPath()

	fmt.Printf("WARNING: This will replace the current database with %s\n", backupFile)
	fmt.Printf("Database location: %s\n", dbPath)
	fmt.Print("Continue? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	fmt.Println("\nRestoring from backup...")

	// Read the backup file
	backupData, err := os.ReadFile(backupFile)
	if err != nil {
		return fmt.Errorf("read backup file: %w", err)
	}

	// Create a backup of the current database before replacing
	if _, err := os.Stat(dbPath); err == nil {
		backupCurrent := dbPath + ".pre-restore." + time.Now().Format("20060102-150405")
		if currentData, err := os.ReadFile(dbPath); err == nil {
			if err := os.WriteFile(backupCurrent, currentData, 0600); err != nil {
				fmt.Printf("Warning: could not backup current database: %v\n", err)
			} else {
				fmt.Printf("Current database backed up to: %s\n", backupCurrent)
			}
		}
	}

	// Write the backup to the database location
	if err := os.WriteFile(dbPath, backupData, 0600); err != nil {
		return fmt.Errorf("write database file: %w", err)
	}

	fmt.Println("Database restored successfully.")
	fmt.Println("Restart the master server to apply changes.")
	return nil
}

func runProjectList(cmd *cobra.Command, args []string) error {
	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	projects, err := db.ListProjects()
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	if len(projects) == 0 {
		fmt.Println("No projects configured.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tREPO\tLAST DEPLOY\tSTATUS")

	for _, p := range projects {
		lastDeploy := "-"
		status := "idle"
		if p.LastDeployAt != nil {
			lastDeploy = p.LastDeployAt.Format("2006-01-02 15:04")
		}
		if p.LastDeployStatus != "" {
			status = p.LastDeployStatus
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			p.Name,
			p.Type,
			p.Repository,
			lastDeploy,
			status,
		)
	}
	w.Flush()

	return nil
}

func runProjectAdd(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Interactive project creation
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Creating project: %s\n\n", projectName)

	fmt.Print("Repository URL: ")
	repoURL, _ := reader.ReadString('\n')
	repoURL = strings.TrimSpace(repoURL)

	fmt.Print("Branch (default: main): ")
	branch, _ := reader.ReadString('\n')
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}

	fmt.Print("Deploy path: ")
	deployPath, _ := reader.ReadString('\n')
	deployPath = strings.TrimSpace(deployPath)

	fmt.Print("Project type (leave empty for auto-detect): ")
	projectType, _ := reader.ReadString('\n')
	projectType = strings.TrimSpace(projectType)

	project := &storage.Project{
		Name:       projectName,
		Repository: repoURL,
		Branch:     branch,
		DeployPath: deployPath,
		Type:       projectType,
		CreatedAt:  time.Now(),
	}

	if err := db.CreateProject(project); err != nil {
		return fmt.Errorf("create project: %w", err)
	}

	fmt.Printf("\nProject '%s' created successfully.\n", projectName)
	fmt.Println("Configure additional settings in the web UI or edit the config file.")
	return nil
}

func runProjectEdit(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Get existing project
	project, err := db.GetProjectByName(cmd.Context(), projectName)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	// Check for flag values
	repoFlag, _ := cmd.Flags().GetString("repo")
	branchFlag, _ := cmd.Flags().GetString("branch")
	pathFlag, _ := cmd.Flags().GetString("path")
	typeFlag, _ := cmd.Flags().GetString("type")

	// If any flags provided, use them directly
	hasFlags := repoFlag != "" || branchFlag != "" || pathFlag != "" || typeFlag != ""

	if hasFlags {
		// Apply flag values
		if repoFlag != "" {
			project.Repository = repoFlag
		}
		if branchFlag != "" {
			project.Branch = branchFlag
		}
		if pathFlag != "" {
			project.DeployPath = pathFlag
		}
		if typeFlag != "" {
			project.Type = typeFlag
		}
	} else {
		// Interactive mode
		fmt.Printf("Editing project: %s\n", projectName)
		fmt.Println("Press Enter to keep current value, or enter a new value.")
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)

		// Repository
		fmt.Printf("Repository [%s]: ", project.Repository)
		input, err := reader.ReadString('\n')
		if err != nil {
			// EOF or broken pipe - user terminated input
			fmt.Println()
			return fmt.Errorf("input terminated: %w", err)
		}
		input = strings.TrimSpace(input)
		if input != "" {
			project.Repository = input
		}

		// Branch
		fmt.Printf("Branch [%s]: ", project.Branch)
		input, err = reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			return fmt.Errorf("input terminated: %w", err)
		}
		input = strings.TrimSpace(input)
		if input != "" {
			project.Branch = input
		}

		// Deploy Path
		fmt.Printf("Deploy Path [%s]: ", project.DeployPath)
		input, err = reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			return fmt.Errorf("input terminated: %w", err)
		}
		input = strings.TrimSpace(input)
		if input != "" {
			project.DeployPath = input
		}

		// Type
		fmt.Printf("Type [%s]: ", project.Type)
		input, err = reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			return fmt.Errorf("input terminated: %w", err)
		}
		input = strings.TrimSpace(input)
		if input != "" {
			project.Type = input
		}
	}

	// Update project
	ctx := context.Background()
	if err := db.UpdateProjectByName(ctx, project); err != nil {
		return fmt.Errorf("update project: %w", err)
	}

	fmt.Printf("\nProject '%s' updated successfully.\n", projectName)
	return nil
}
func runProjectDelete(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	fmt.Printf("WARNING: This will delete project '%s' and all its deployment history.\n", projectName)
	fmt.Print("Type the project name to confirm: ")

	reader := bufio.NewReader(os.Stdin)
	confirmation, _ := reader.ReadString('\n')
	confirmation = strings.TrimSpace(confirmation)

	if confirmation != projectName {
		fmt.Println("Confirmation failed. Aborted.")
		return nil
	}

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.DeleteProject(projectName); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}

	fmt.Printf("Project '%s' deleted.\n", projectName)
	return nil
}

func runProjectValidate(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	project, err := db.GetProjectByName(cmd.Context(), projectName)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	fmt.Printf("Validating project: %s\n\n", projectName)

	errors := []string{}
	warnings := []string{}

	// Basic validations
	if project.Repository == "" {
		errors = append(errors, "Repository URL is not set")
	}
	if project.DeployPath == "" {
		errors = append(errors, "Deploy path is not set")
	}
	if project.Branch == "" {
		warnings = append(warnings, "Branch not set, will default to 'main'")
	}

	// Print results
	if len(errors) > 0 {
		fmt.Println("❌ Errors:")
		for _, e := range errors {
			fmt.Printf("   - %s\n", e)
		}
	}
	if len(warnings) > 0 {
		fmt.Println("⚠️  Warnings:")
		for _, w := range warnings {
			fmt.Printf("   - %s\n", w)
		}
	}
	if len(errors) == 0 && len(warnings) == 0 {
		fmt.Println("✅ Project configuration is valid.")
	}

	return nil
}

func runProjectDeploy(cmd *cobra.Command, args []string) error {
	projectName := args[0]
	target, _ := cmd.Flags().GetString("target")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")

	// Get master address
	masterAddr, _ := cmd.Flags().GetString("master")
	if masterAddr == "" {
		masterAddr = os.Getenv("VCDEPLOY_MASTER")
	}
	if masterAddr == "" {
		masterAddr = "localhost:9000"
	}

	// Get API token
	apiToken, _ := cmd.Flags().GetString("token")
	if apiToken == "" {
		apiToken = os.Getenv("VCDEPLOY_TOKEN")
	}

	fmt.Printf("🚀 Deploying project: %s\n", projectName)
	if target != "" {
		fmt.Printf("   Target: %s\n", target)
	}
	if dryRun {
		fmt.Printf("   Mode: dry-run (no changes will be made)\n")
	}
	if force {
		fmt.Printf("   Mode: force (bypassing locks)\n")
	}
	fmt.Println()

	if dryRun {
		fmt.Println("📋 Dry run - checking deployment configuration...")
		// Just validate locally for dry run
		if err := initConfig(cmd); err != nil {
			return err
		}
		dbPath := getDBPath()
		db, err := storage.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		_, err = db.GetProjectByName(cmd.Context(), projectName)
		if err != nil {
			return fmt.Errorf("get project: %w", err)
		}
		fmt.Println("\n✅ Dry run completed successfully.")
		fmt.Println("   Configuration is valid, no changes were made.")
		return nil
	}

	// Call master API to trigger deployment
	client := &http.Client{Timeout: 30 * time.Second}
	baseURL := "http://" + masterAddr

	// Create deployment request
	reqBody := map[string]interface{}{
		"project": projectName,
		"force":   force,
	}
	if target != "" {
		reqBody["target"] = target
	}

	reqJSON, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", baseURL+"/api/v1/deployments", strings.NewReader(string(reqJSON)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}

	fmt.Println("📡 Triggering deployment via master...")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("master not reachable at %s: %w\nStart the master with: vcdeploy master start", masterAddr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication required. Provide --token or set VCDEPLOY_TOKEN")
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("deployment failed: %s", string(body))
	}

	// Parse response to get deployment ID
	var deployResp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&deployResp); err != nil {
		fmt.Println("✅ Deployment triggered successfully!")
		return nil
	}

	fmt.Printf("   Deployment ID: %s\n", deployResp.ID)
	fmt.Println()

	// Poll for deployment status (interruptible with Ctrl+C)
	fmt.Println("⏳ Waiting for deployment to complete...")
	for i := 0; i < 120; i++ { // Max 10 minutes
		select {
		case <-cmd.Context().Done():
			fmt.Println("\n⚠️  Interrupted. Deployment continues in background.")
			fmt.Printf("   Check status with: vcdeploy deployment status %s\n", deployResp.ID)
			return nil
		case <-time.After(5 * time.Second):
		}

		statusReq, _ := http.NewRequest("GET", baseURL+"/api/v1/deployments/"+deployResp.ID, nil)
		if apiToken != "" {
			statusReq.Header.Set("Authorization", "Bearer "+apiToken)
		}

		statusResp, err := client.Do(statusReq)
		if err != nil {
			continue
		}

		var status struct {
			Status string `json:"status"`
		}
		json.NewDecoder(statusResp.Body).Decode(&status)
		statusResp.Body.Close()

		switch status.Status {
		case "success", "completed":
			fmt.Println("\n✅ Deployment completed successfully!")
			return nil
		case "failed", "error":
			return fmt.Errorf("deployment failed. Check logs with: vcdeploy deployment logs %s", deployResp.ID)
		case "cancelled":
			return fmt.Errorf("deployment was cancelled")
		}
		fmt.Print(".")
	}

	fmt.Println("\n⚠️  Deployment still in progress. Check status with:")
	fmt.Printf("   vcdeploy deployment status %s\n", deployResp.ID)
	return nil
}

func runProjectRollback(cmd *cobra.Command, args []string) error {
	projectName := args[0]
	target, _ := cmd.Flags().GetString("target")
	release, _ := cmd.Flags().GetInt("release")

	// Get master address
	masterAddr, _ := cmd.Flags().GetString("master")
	if masterAddr == "" {
		masterAddr = os.Getenv("VCDEPLOY_MASTER")
	}
	if masterAddr == "" {
		masterAddr = "localhost:9000"
	}

	// Get API token
	apiToken, _ := cmd.Flags().GetString("token")
	if apiToken == "" {
		apiToken = os.Getenv("VCDEPLOY_TOKEN")
	}

	fmt.Printf("🔙 Rolling back project: %s\n", projectName)
	if target != "" {
		fmt.Printf("   Target: %s\n", target)
	}
	if release > 0 {
		fmt.Printf("   To release: %d\n", release)
	} else {
		fmt.Printf("   To: previous release\n")
	}
	fmt.Println()

	fmt.Print("Continue? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	// First, get the latest deployment for this project to rollback
	client := &http.Client{Timeout: 30 * time.Second}
	baseURL := "http://" + masterAddr

	// Get latest deployment for this project
	listReq, _ := http.NewRequest("GET", baseURL+"/api/v1/deployments?project="+projectName+"&limit=1", nil)
	if apiToken != "" {
		listReq.Header.Set("Authorization", "Bearer "+apiToken)
	}

	listResp, err := client.Do(listReq)
	if err != nil {
		return fmt.Errorf("master not reachable at %s: %w\nStart the master with: vcdeploy master start", masterAddr, err)
	}
	defer listResp.Body.Close()

	if listResp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication required. Provide --token or set VCDEPLOY_TOKEN")
	}

	var deployments []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&deployments); err != nil || len(deployments) == 0 {
		return fmt.Errorf("no deployments found for project %s", projectName)
	}

	deploymentID := deployments[0].ID

	// Call rollback API
	reqBody := map[string]interface{}{}
	if release > 0 {
		reqBody["release"] = release
	}
	if target != "" {
		reqBody["target"] = target
	}

	reqJSON, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", baseURL+"/api/v1/deployments/"+deploymentID+"/rollback", strings.NewReader(string(reqJSON)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}

	fmt.Println("\n🔄 Triggering rollback via master...")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("rollback request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("rollback failed: %s", string(body))
	}

	fmt.Println("✅ Rollback triggered successfully!")

	// Parse response
	var rollbackResp struct {
		Message string `json:"message"`
		ID      string `json:"rollback_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rollbackResp); err == nil && rollbackResp.ID != "" {
		fmt.Printf("   Rollback ID: %s\n", rollbackResp.ID)
	}

	return nil
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func runTypeList(cmd *cobra.Command, args []string) error {
	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	types, err := db.ListProjectTypes()
	if err != nil {
		return fmt.Errorf("list types: %w", err)
	}

	if len(types) == 0 {
		fmt.Println("No project types configured.")
		fmt.Println("Create one with: vcdeploy type create <name>")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tPROJECTS")

	for _, t := range types {
		fmt.Fprintf(w, "%s\t%s\t%d\n", t.Name, t.Description, t.ProjectCount)
	}
	w.Flush()

	return nil
}

func runTypeCreate(cmd *cobra.Command, args []string) error {
	typeName := args[0]

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Creating project type: %s\n\n", typeName)

	fmt.Print("Description: ")
	description, _ := reader.ReadString('\n')
	description = strings.TrimSpace(description)

	fmt.Print("Build command (e.g., 'npm run build'): ")
	buildCmd, _ := reader.ReadString('\n')
	buildCmd = strings.TrimSpace(buildCmd)

	projectType := &storage.ProjectType{
		Name:        typeName,
		Description: description,
		BuildCmd:    buildCmd,
		CreatedAt:   time.Now(),
	}

	if err := db.CreateProjectType(projectType); err != nil {
		return fmt.Errorf("create type: %w", err)
	}

	fmt.Printf("\nProject type '%s' created successfully.\n", typeName)
	return nil
}

func runTypeEdit(cmd *cobra.Command, args []string) error {
	typeName := args[0]

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Get existing project type
	projectType, err := db.GetProjectTypeByName(typeName)
	if err != nil {
		return fmt.Errorf("get project type: %w", err)
	}

	// Check for flag values
	descFlag, _ := cmd.Flags().GetString("description")
	buildCmdFlag, _ := cmd.Flags().GetString("build-cmd")

	// If any flags provided, use them directly
	hasFlags := descFlag != "" || buildCmdFlag != ""

	if hasFlags {
		// Apply flag values
		if descFlag != "" {
			projectType.Description = descFlag
		}
		if buildCmdFlag != "" {
			projectType.BuildCmd = buildCmdFlag
		}
	} else {
		// Interactive mode
		fmt.Printf("Editing project type: %s\n", typeName)
		fmt.Println("Press Enter to keep current value, or enter a new value.")
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)

		// Description
		fmt.Printf("Description [%s]: ", projectType.Description)
		input, err := reader.ReadString('\n')
		if err != nil {
			// EOF or broken pipe - user terminated input
			fmt.Println()
			return fmt.Errorf("input terminated: %w", err)
		}
		input = strings.TrimSpace(input)
		if input != "" {
			projectType.Description = input
		}

		// Build Command
		fmt.Printf("Build Command [%s]: ", projectType.BuildCmd)
		input, err = reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			return fmt.Errorf("input terminated: %w", err)
		}
		input = strings.TrimSpace(input)
		if input != "" {
			projectType.BuildCmd = input
		}
	}

	// Update project type
	if err := db.UpdateProjectTypeByName(projectType); err != nil {
		return fmt.Errorf("update project type: %w", err)
	}

	fmt.Printf("\nProject type '%s' updated successfully.\n", typeName)
	return nil
}

func runTypeDelete(cmd *cobra.Command, args []string) error {
	typeName := args[0]

	fmt.Printf("WARNING: This will delete project type '%s'.\n", typeName)
	fmt.Print("Projects using this type will need to be updated.\n")
	fmt.Print("Continue? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.DeleteProjectType(typeName); err != nil {
		return fmt.Errorf("delete type: %w", err)
	}

	fmt.Printf("Project type '%s' deleted.\n", typeName)
	return nil
}

func runSecretSet(cmd *cobra.Command, args []string) error {
	stdin, _ := cmd.Flags().GetBool("stdin")
	scope := args[0]
	key := args[1]

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Initialize KMS for encryption
	kms, err := security.NewKMS(db.Conn(), globalLogger)
	if err != nil {
		return fmt.Errorf("initialize KMS: %w", err)
	}
	secrets := security.NewSecretService(db, kms)

	var value string
	if stdin {
		// Read from stdin
		reader := bufio.NewReader(os.Stdin)
		value, err = reader.ReadString('\n')
		if err != nil && value == "" {
			return fmt.Errorf("read stdin: %w", err)
		}
		value = strings.TrimSpace(value)
	} else {
		// Read interactively without echo
		fmt.Print("Enter value: ")
		byteValue, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		value = string(byteValue)
		fmt.Println() // Add newline after hidden input
	}

	if value == "" {
		return fmt.Errorf("value cannot be empty")
	}

	// Parse scope into project/scope - format: project/scope or just scope (project=scope)
	project := scope
	scopeName := "_default"
	if parts := strings.SplitN(scope, "/", 2); len(parts) == 2 {
		project = parts[0]
		scopeName = parts[1]
	}

	ctx := context.Background()
	if err := secrets.Set(ctx, project, scopeName, key, value); err != nil {
		return fmt.Errorf("set secret: %w", err)
	}

	fmt.Printf("Secret '%s/%s' set successfully (encrypted).\n", scope, key)
	return nil
}

func runSecretList(cmd *cobra.Command, args []string) error {
	project := args[0]

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	secrets, err := db.ListSecrets(project)
	if err != nil {
		return fmt.Errorf("list secrets: %w", err)
	}

	if len(secrets) == 0 {
		fmt.Printf("No secrets configured for '%s'.\n", project)
		return nil
	}

	fmt.Printf("Secrets for '%s':\n\n", project)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tUPDATED\tSCOPE")

	for _, s := range secrets {
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.Key, s.UpdatedAt.Format("2006-01-02 15:04"), s.Scope)
	}
	w.Flush()

	return nil
}

func runSecretDelete(cmd *cobra.Command, args []string) error {
	scope := args[0]
	key := args[1]

	fmt.Printf("Delete secret '%s/%s'? [y/N]: ", scope, key)
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.DeleteSecret(scope, key); err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}

	fmt.Printf("Secret '%s/%s' deleted.\n", scope, key)
	return nil
}

func runSecretImport(cmd *cobra.Command, args []string) error {
	scope := args[0]

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	fmt.Printf("Importing secrets to '%s' from stdin (.env format)...\n", scope)
	fmt.Println("Paste your .env content, then press Ctrl+D when done:")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	count := 0

	// Initialize KMS for encryption
	kms, err := security.NewKMS(db.Conn(), globalLogger)
	if err != nil {
		return fmt.Errorf("initialize KMS: %w", err)
	}
	secrets := security.NewSecretService(db, kms)
	ctx := context.Background()

	// Parse scope into project/scope
	project := scope
	scopeName := "_default"
	if parts := strings.SplitN(scope, "/", 2); len(parts) == 2 {
		project = parts[0]
		scopeName = parts[1]
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			fmt.Printf("  Warning: skipping invalid line: %s\n", line)
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = strings.Trim(value, `"'`)

		if err := secrets.Set(ctx, project, scopeName, key, value); err != nil {
			fmt.Printf("  Error setting %s: %v\n", key, err)
			continue
		}

		count++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	fmt.Printf("\nImported %d secrets.\n", count)
	return nil
}

func runSecretBackup(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	if output == "" {
		output = fmt.Sprintf("secrets-%s.vcbackup", time.Now().Format("20060102-150405"))
	}

	fmt.Print("Enter backup passphrase: ")
	passphrase1, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("read passphrase: %w", err)
	}
	fmt.Println()

	fmt.Print("Confirm passphrase: ")
	passphrase2, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("read passphrase: %w", err)
	}
	fmt.Println()

	if string(passphrase1) != string(passphrase2) {
		return fmt.Errorf("passphrases do not match")
	}

	if len(passphrase1) < 8 {
		return fmt.Errorf("passphrase must be at least 8 characters")
	}

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	fmt.Printf("\nBacking up secrets to %s...\n", output)

	// Export all secrets
	secrets, err := db.ExportAllSecrets()
	if err != nil {
		return fmt.Errorf("export secrets: %w", err)
	}

	// Serialize to JSON
	plaintext, err := json.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("serialize secrets: %w", err)
	}

	// Encrypt with passphrase
	encrypted, err := security.EncryptWithPassphrase(plaintext, passphrase1)
	if err != nil {
		return fmt.Errorf("encrypt secrets: %w", err)
	}

	// Encode as base64 for safe storage
	encoded := base64.StdEncoding.EncodeToString(encrypted)

	// Write to file with header
	backupData := fmt.Sprintf("VCDEPLOY-SECRETS-V1\n%s", encoded)
	if err := os.WriteFile(output, []byte(backupData), 0600); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}

	fmt.Printf("Backed up %d secrets successfully.\n", len(secrets))
	fmt.Printf("Keep this file and passphrase secure!\n")
	return nil
}

func runSecretRestore(cmd *cobra.Command, args []string) error {
	backupFile := args[0]

	// Read backup file
	data, err := os.ReadFile(backupFile)
	if err != nil {
		return fmt.Errorf("read backup file: %w", err)
	}

	// Verify header
	content := string(data)
	if !strings.HasPrefix(content, "VCDEPLOY-SECRETS-V1\n") {
		return fmt.Errorf("invalid backup file format")
	}

	// Extract encoded data
	encoded := strings.TrimPrefix(content, "VCDEPLOY-SECRETS-V1\n")
	encoded = strings.TrimSpace(encoded)

	fmt.Print("Enter backup passphrase: ")
	passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("read passphrase: %w", err)
	}
	fmt.Println()

	// Decode base64
	encrypted, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decode backup: %w", err)
	}

	// Decrypt with passphrase
	decrypted, err := security.DecryptWithPassphrase(encrypted, passphrase)
	if err != nil {
		return fmt.Errorf("decrypt backup: %w (wrong passphrase?)", err)
	}

	// Parse JSON - secrets are stored as map[scope]map[key]value
	var secrets map[string]map[string]string
	if err := json.Unmarshal(decrypted, &secrets); err != nil {
		return fmt.Errorf("parse backup: %w", err)
	}

	// Count total secrets
	totalSecrets := 0
	for _, scopeSecrets := range secrets {
		totalSecrets += len(scopeSecrets)
	}

	fmt.Printf("\nFound %d secrets in backup.\n", totalSecrets)
	fmt.Print("Restore all secrets? This will overwrite existing ones. [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := getDBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	fmt.Printf("\nRestoring secrets from %s...\n", backupFile)

	// Initialize KMS for encryption
	kms, err := security.NewKMS(db.Conn(), globalLogger)
	if err != nil {
		return fmt.Errorf("initialize KMS: %w", err)
	}
	secretsService := security.NewSecretService(db, kms)
	ctx := context.Background()

	// Import each secret
	restored := 0
	for scope, scopeSecrets := range secrets {
		// Parse scope into project/scope
		project := scope
		scopeName := "_default"
		if parts := strings.SplitN(scope, "/", 2); len(parts) == 2 {
			project = parts[0]
			scopeName = parts[1]
		}

		for key, value := range scopeSecrets {
			if err := secretsService.Set(ctx, project, scopeName, key, value); err != nil {
				fmt.Printf("  Warning: failed to restore %s/%s: %v\n", scope, key, err)
				continue
			}
			restored++
		}
	}

	fmt.Printf("\nRestored %d/%d secrets successfully.\n", restored, totalSecrets)
	return nil
}
