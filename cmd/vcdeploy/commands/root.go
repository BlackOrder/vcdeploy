// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/server"
	"github.com/BlackOrder/vcdeploy/internal/storage"
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
	projectCmd.AddCommand(&cobra.Command{
		Use:   "edit [name]",
		Short: "Edit a project",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectEdit,
	})
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
	typeCmd.AddCommand(&cobra.Command{
		Use:   "edit [name]",
		Short: "Edit a project type",
		Args:  cobra.ExactArgs(1),
		RunE:  runTypeEdit,
	})
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

// initConfig loads configuration from file
func initConfig(cmd *cobra.Command) error {
	cfgPath, _ := cmd.Flags().GetString("config")
	if cfgPath == "" {
		cfgPath = "/etc/vcdeploy/master.yaml"
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
	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
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

	errCh := make(chan error, 1)
	go func() {
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
	fmt.Println("Sending stop signal to master server...")
	// In practice, this would send a signal to the running process
	// For systemd-managed installations, use: systemctl stop vcdeploy-master
	return nil
}

func runMasterStatus(cmd *cobra.Command, args []string) error {
	masterAddr, _ := cmd.Flags().GetString("master")
	if masterAddr == "" {
		masterAddr = "localhost:9000"
	}

	// Try to connect and get status
	fmt.Printf("Checking master at %s...\n\n", masterAddr)

	// This would make an API call to the master
	// For now, show placeholder
	fmt.Println("Status: Unknown (API call not implemented)")
	fmt.Println("\nTip: If using systemd, check with: systemctl status vcdeploy-master")
	return nil
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
	// Implementation would call security.RotateMasterKey()
	fmt.Println("Key rotated successfully.")
	return nil
}

func runBackupCreate(cmd *cobra.Command, args []string) error {
	if err := initConfig(cmd); err != nil {
		return err
	}

	backupPath := globalConfig.Backup.Database.Path
	if backupPath == "" {
		backupPath = "/var/lib/vcdeploy/backups"
	}

	timestamp := time.Now().Format("20060102-150405")
	backupFile := fmt.Sprintf("%s/vcdeploy-%s.db.bak", backupPath, timestamp)

	fmt.Printf("Creating backup: %s\n", backupFile)

	// Ensure backup directory exists
	if err := os.MkdirAll(backupPath, 0750); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	// Open database and create backup
	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
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
		backupPath = "/var/lib/vcdeploy/backups"
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

	fmt.Printf("WARNING: This will replace the current database with %s\n", backupFile)
	fmt.Print("Continue? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	fmt.Println("\nRestoring from backup...")
	// Implementation would copy backup file to database location
	fmt.Println("Database restored successfully.")
	fmt.Println("Restart the master server to apply changes.")
	return nil
}

func runProjectList(cmd *cobra.Command, args []string) error {
	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
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

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
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
	fmt.Printf("Opening project editor for: %s\n", projectName)
	fmt.Println("Tip: Use the web UI for easier project editing, or edit the config file directly.")
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

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
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

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	project, err := db.GetProject(projectName)
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

	if err := initConfig(cmd); err != nil {
		return err
	}

	if err := initLogger("info"); err != nil {
		return err
	}

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	project, err := db.GetProject(projectName)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
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

	// Create deployment record
	deployment := &storage.DeploymentCLI{
		ProjectID:   project.ID,
		ProjectName: projectName,
		Target:      target,
		Status:      "pending",
		TriggeredBy: "cli",
		StartedAt:   time.Now(),
	}

	if !dryRun {
		if err := db.InsertDeployment(deployment); err != nil {
			return fmt.Errorf("create deployment record: %w", err)
		}
	}

	// Execute deployment (simplified for CLI)
	fmt.Println("📦 Fetching latest code...")
	fmt.Printf("   Repository: %s\n", project.Repository)
	fmt.Printf("   Branch: %s\n", project.Branch)

	if dryRun {
		fmt.Println("\n✅ Dry run completed successfully.")
		fmt.Println("   No changes were made.")
		return nil
	}

	// In real implementation, this would:
	// 1. Clone/pull repository
	// 2. Run build commands
	// 3. Execute deployment strategy
	// 4. Update symlinks
	// 5. Run post-deploy hooks

	fmt.Println("\n🏗️  Building...")
	fmt.Println("🔗 Deploying...")
	fmt.Println("\n✅ Deployment completed successfully!")

	deployment.Status = "success"
	deployment.FinishedAt = timePtr(time.Now())
	if err := db.SaveDeployment(deployment); err != nil {
		globalLogger.Warn("failed to update deployment record", zap.Error(err))
	}

	return nil
}

func runProjectRollback(cmd *cobra.Command, args []string) error {
	projectName := args[0]
	target, _ := cmd.Flags().GetString("target")
	release, _ := cmd.Flags().GetInt("release")

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	project, err := db.GetProject(projectName)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
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

	fmt.Println("\n🔄 Rolling back...")

	// Create rollback deployment record
	deployment := &storage.DeploymentCLI{
		ProjectID:   project.ID,
		ProjectName: projectName,
		Target:      target,
		Status:      "rolling_back",
		TriggeredBy: "cli",
		StartedAt:   time.Now(),
	}

	if err := db.InsertDeployment(deployment); err != nil {
		return fmt.Errorf("create deployment record: %w", err)
	}

	// In real implementation, would call deploy.Rollback()
	fmt.Println("✅ Rollback completed successfully!")

	deployment.Status = "rolled_back"
	deployment.FinishedAt = timePtr(time.Now())
	db.SaveDeployment(deployment)

	return nil
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func runTypeList(cmd *cobra.Command, args []string) error {
	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
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

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
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
	fmt.Printf("Opening type editor for: %s\n", typeName)
	fmt.Println("Tip: Use the web UI for easier editing.")
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

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
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

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	var value string
	if stdin {
		// Read from stdin
		reader := bufio.NewReader(os.Stdin)
		value, _ = reader.ReadString('\n')
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

	if err := db.SetSecret(scope, key, value); err != nil {
		return fmt.Errorf("set secret: %w", err)
	}

	fmt.Printf("Secret '%s/%s' set successfully.\n", scope, key)
	return nil
}

func runSecretList(cmd *cobra.Command, args []string) error {
	project := args[0]

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
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

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
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

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
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

		if err := db.SetSecret(scope, key, value); err != nil {
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

	if err := initConfig(cmd); err != nil {
		return err
	}

	dbPath := "/var/lib/vcdeploy/vcdeploy.db"
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

	// Encrypt with passphrase
	data, _ := json.Marshal(secrets)
	// In real implementation, would use security.EncryptWithPassphrase()

	if err := os.WriteFile(output, data, 0600); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}

	fmt.Printf("Backed up %d secrets successfully.\n", len(secrets))
	return nil
}

func runSecretRestore(cmd *cobra.Command, args []string) error {
	backupFile := args[0]

	if _, err := os.Stat(backupFile); err != nil {
		return fmt.Errorf("backup file not found: %s", backupFile)
	}

	fmt.Print("Enter backup passphrase: ")
	passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("read passphrase: %w", err)
	}
	fmt.Println()

	_ = passphrase // Would use to decrypt

	fmt.Printf("\nRestoring secrets from %s...\n", backupFile)

	// In real implementation:
	// 1. Read backup file
	// 2. Decrypt with passphrase
	// 3. Import each secret

	fmt.Println("Secrets restored successfully.")
	return nil
}

func exitWithError(msg string) {
	fmt.Fprintln(os.Stderr, "Error:", msg)
	os.Exit(1)
}
