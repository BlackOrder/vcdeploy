// Package commands implements the CLI commands for vcdeploy-agent.
package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
	rootCmd.PersistentFlags().String("config", "/etc/vcdeploy/agent.yaml", "config file")

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
	registerCmd.MarkFlagRequired("master")
	registerCmd.MarkFlagRequired("token")
}

func runStart(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")
	master, _ := cmd.Flags().GetString("master")
	token, _ := cmd.Flags().GetString("token")

	fmt.Printf("Starting agent...\n")
	fmt.Printf("  config: %s\n", configPath)
	if master != "" {
		fmt.Printf("  master: %s\n", master)
	}
	if token != "" {
		fmt.Printf("  token: (provided)\n")
	}

	// TODO: Implement agent start
	// 1. Load config
	// 2. Connect to master
	// 3. Register if needed
	// 4. Start heartbeat
	// 5. Listen for commands

	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	fmt.Println("Agent status: not running")
	return nil
}

func runRegister(cmd *cobra.Command, args []string) error {
	master, _ := cmd.Flags().GetString("master")
	_, _ = cmd.Flags().GetString("token") // token used in actual implementation

	fmt.Printf("Registering agent with master: %s\n", master)
	fmt.Printf("  token: (provided)\n")

	// TODO: Implement registration
	// 1. Connect to master
	// 2. Send registration request with token
	// 3. Receive and store certificate
	// 4. Save config

	return nil
}

func exitWithError(msg string) {
	fmt.Fprintln(os.Stderr, "Error:", msg)
	os.Exit(1)
}
