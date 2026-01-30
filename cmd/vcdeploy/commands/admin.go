package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services/users"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// adminCmd handles administrator account management
var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Administrator account management",
	Long: `Manage the administrator account.

This command can create or update admin credentials either locally (direct database access)
or remotely (via API when --master and --token are provided).

Local mode (lockout recovery):
  vcdeploy admin --username admin --email admin@example.com

Remote mode (requires authentication):
  vcdeploy admin --master localhost:8080 --token <api-token> --username admin

When --password is not provided, you will be prompted to enter it interactively.`,
	RunE: runAdmin,
}

// userCmd handles user management commands
var userCmd = &cobra.Command{
	Use:   "user",
	Short: "User management",
	Long:  "Commands for managing users in vcdeploy.",
}

// agentCmd handles agent management commands
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent management",
	Long:  "Commands for managing deployment agents.",
}

// deployCmd (global) handles deployment commands
var deploymentCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deployment commands",
	Long:  "Commands for managing and triggering deployments.",
}

// configCmd handles configuration commands
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
	Long:  "Commands for viewing and managing server configuration.",
}

// apikeyCmd handles API key commands
var apikeyCmd = &cobra.Command{
	Use:   "apikey",
	Short: "API key management",
	Long:  "Commands for managing API keys.",
}

func init() {
	// Register new command groups
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(deploymentCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(apikeyCmd)

	// Admin command with flags
	adminCmd.Flags().StringP("username", "u", "admin", "Admin username")
	adminCmd.Flags().StringP("password", "p", "", "Admin password (if not set, will prompt)")
	adminCmd.Flags().StringP("email", "e", "admin@localhost", "Admin email address")
	rootCmd.AddCommand(adminCmd)

	// User subcommands
	userCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE:  runUserList,
	})

	createUserCmd := &cobra.Command{
		Use:   "create [username]",
		Short: "Create a new user",
		Args:  cobra.ExactArgs(1),
		RunE:  runUserCreate,
	}
	createUserCmd.Flags().StringP("email", "e", "", "User email address")
	createUserCmd.Flags().StringP("role", "r", "user", "User role (admin, deployer, viewer)")
	createUserCmd.Flags().StringP("password", "p", "", "User password (if not set, will prompt)")
	userCmd.AddCommand(createUserCmd)

	userCmd.AddCommand(&cobra.Command{
		Use:   "delete [username]",
		Short: "Delete a user",
		Args:  cobra.ExactArgs(1),
		RunE:  runUserDelete,
	})

	userCmd.AddCommand(&cobra.Command{
		Use:   "passwd [username]",
		Short: "Change user password",
		Args:  cobra.ExactArgs(1),
		RunE:  runUserPasswd,
	})

	// Agent subcommands
	agentCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all agents",
		RunE:  runAgentList,
	})

	agentCmd.AddCommand(&cobra.Command{
		Use:   "show [agent-id]",
		Short: "Show agent details",
		Args:  cobra.ExactArgs(1),
		RunE:  runAgentShow,
	})

	agentCmd.AddCommand(&cobra.Command{
		Use:   "delete [agent-id]",
		Short: "Remove an agent",
		Args:  cobra.ExactArgs(1),
		RunE:  runAgentDelete,
	})

	agentTokenCmd := &cobra.Command{
		Use:   "token",
		Short: "Generate agent registration token",
		RunE:  runAgentToken,
	}
	agentTokenCmd.Flags().StringP("label", "l", "", "Agent label for the token")
	agentCmd.AddCommand(agentTokenCmd)

	agentUpdateCmd := &cobra.Command{
		Use:   "update [agent-id]",
		Short: "Update an agent to the latest version",
		Long:  "Trigger a self-update of the specified agent to the latest available version.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runAgentUpdate,
	}
	agentUpdateCmd.Flags().Bool("all", false, "Update all agents")
	agentUpdateCmd.Flags().String("version", "", "Specific version to update to")
	agentCmd.AddCommand(agentUpdateCmd)

	// Deployment subcommands
	triggerCmd := &cobra.Command{
		Use:   "trigger [project]",
		Short: "Trigger a deployment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeploymentTrigger,
	}
	triggerCmd.Flags().StringP("branch", "b", "", "Branch to deploy")
	triggerCmd.Flags().StringP("target", "t", "", "Target environment")
	triggerCmd.Flags().String("schedule", "", "Schedule deployment (RFC3339 format)")
	deploymentCmd.AddCommand(triggerCmd)

	deploymentCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List recent deployments",
		RunE:  runDeploymentList,
	})

	deploymentCmd.AddCommand(&cobra.Command{
		Use:   "status [deployment-id]",
		Short: "Get deployment status",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeploymentStatus,
	})

	deploymentCmd.AddCommand(&cobra.Command{
		Use:   "cancel [deployment-id]",
		Short: "Cancel a running deployment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeploymentCancel,
	})

	deploymentCmd.AddCommand(&cobra.Command{
		Use:   "logs [deployment-id]",
		Short: "View deployment logs",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeploymentLogs,
	})

	// Config subcommands
	configCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE:  runConfigShow,
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "export",
		Short: "Export configuration to JSON",
		RunE:  runConfigExport,
	})

	importConfigCmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Import configuration from JSON",
		Args:  cobra.ExactArgs(1),
		RunE:  runConfigImport,
	}
	configCmd.AddCommand(importConfigCmd)

	setConfigCmd := &cobra.Command{
		Use:   "set [category.key] [value]",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE:  runConfigSet,
	}
	configCmd.AddCommand(setConfigCmd)

	// API Key subcommands
	apikeyCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all API keys",
		RunE:  runAPIKeyList,
	})

	createKeyCmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new API key",
		Args:  cobra.ExactArgs(1),
		RunE:  runAPIKeyCreate,
	}
	createKeyCmd.Flags().Int("expires", 0, "Days until expiry (0 = never)")
	apikeyCmd.AddCommand(createKeyCmd)

	apikeyCmd.AddCommand(&cobra.Command{
		Use:   "revoke [key-id]",
		Short: "Revoke an API key",
		Args:  cobra.ExactArgs(1),
		RunE:  runAPIKeyRevoke,
	})
}

// --- API Client Helper ---

type apiClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func newAPIClient(cmd *cobra.Command) (*apiClient, error) {
	master, _ := cmd.Flags().GetString("master")
	token, _ := cmd.Flags().GetString("token")

	// Check environment variables as fallback
	if master == "" {
		master = os.Getenv("VCDEPLOY_MASTER")
	}
	if token == "" {
		token = os.Getenv("VCDEPLOY_TOKEN")
	}

	if master == "" {
		return nil, fmt.Errorf("master address required (--master or VCDEPLOY_MASTER)")
	}
	if token == "" {
		return nil, fmt.Errorf("API token required (--token or VCDEPLOY_TOKEN)")
	}

	// Ensure URL has protocol
	if !strings.HasPrefix(master, "http://") && !strings.HasPrefix(master, "https://") {
		master = "http://" + master
	}

	return &apiClient{
		baseURL: master,
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *apiClient) do(method, path string, body io.Reader) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	return c.client.Do(req)
}

func (c *apiClient) get(path string) (*http.Response, error) {
	return c.do("GET", path, nil)
}

func (c *apiClient) post(path string, body io.Reader) (*http.Response, error) {
	return c.do("POST", path, body)
}

func (c *apiClient) delete(path string) (*http.Response, error) {
	return c.do("DELETE", path, nil)
}

// --- User Commands ---

func runUserList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/users")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var users []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSERNAME\tEMAIL\tROLE\tCREATED")
	for _, u := range users {
		fmt.Fprintf(w, "%.0f\t%s\t%s\t%s\t%s\n",
			u["id"], u["username"], u["email"], u["role"], u["createdAt"])
	}
	w.Flush()
	return nil
}

func runUserCreate(cmd *cobra.Command, args []string) error {
	username := args[0]
	email, _ := cmd.Flags().GetString("email")
	role, _ := cmd.Flags().GetString("role")
	password, _ := cmd.Flags().GetString("password")

	// Prompt for password if not provided
	if password == "" {
		fmt.Print("Enter password: ")
		pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		fmt.Println()

		fmt.Print("Confirm password: ")
		pwBytes2, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		fmt.Println()

		if !bytes.Equal(pwBytes, pwBytes2) {
			return fmt.Errorf("passwords do not match")
		}
		password = string(pwBytes)
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data, _ := json.Marshal(map[string]string{
		"username": username,
		"email":    email,
		"password": password,
		"role":     role,
	})

	resp, err := client.post("/api/v1/users", strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	fmt.Printf("User '%s' created successfully.\n", username)
	return nil
}

func runUserDelete(cmd *cobra.Command, args []string) error {
	username := args[0]

	fmt.Printf("Are you sure you want to delete user '%s'? (y/N): ", username)
	var confirm string
	_, _ = fmt.Scanln(&confirm)
	if !strings.EqualFold(confirm, "y") {
		return fmt.Errorf("aborted")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// First get user ID by username
	resp, err := client.get("/api/v1/users")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}

	var users []map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&users)
	resp.Body.Close()

	var userID float64
	for _, u := range users {
		if u["username"] == username {
			id, ok := u["id"].(float64)
			if !ok {
				return fmt.Errorf("invalid API response: user ID is not a number")
			}
			userID = id
			break
		}
	}
	if userID == 0 {
		return fmt.Errorf("user not found: %s", username)
	}

	resp, err = client.delete(fmt.Sprintf("/api/v1/users/%.0f", userID))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("User '%s' deleted successfully.\n", username)
	return nil
}

func runUserPasswd(cmd *cobra.Command, args []string) error {
	username := args[0]

	fmt.Print("Enter new password: ")
	pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	fmt.Println()

	fmt.Print("Confirm new password: ")
	pwBytes2, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	fmt.Println()

	if !bytes.Equal(pwBytes, pwBytes2) {
		return fmt.Errorf("passwords do not match")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// First get user ID by username
	resp, err := client.get("/api/v1/users")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}

	var users []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		resp.Body.Close()
		return fmt.Errorf("decode response: %w", err)
	}
	resp.Body.Close()

	var userID float64
	for _, u := range users {
		if u["username"] == username {
			id, ok := u["id"].(float64)
			if !ok {
				return fmt.Errorf("invalid API response: user ID is not a number")
			}
			userID = id
			break
		}
	}
	if userID == 0 {
		return fmt.Errorf("user not found: %s", username)
	}

	// Update user password via PATCH
	data, _ := json.Marshal(map[string]string{
		"password": string(pwBytes),
	})

	resp, err = client.do("PATCH", fmt.Sprintf("/api/v1/users/%.0f", userID), strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	fmt.Printf("Password for '%s' updated successfully.\n", username)
	return nil
}

// --- Agent Commands ---

func runAgentList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/agents")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var agents []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tHOSTNAME\tSTATUS\tLAST SEEN")
	for _, a := range agents {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			a["id"], a["hostname"], a["status"], a["lastSeenAt"])
	}
	w.Flush()

	if len(agents) == 0 {
		fmt.Println("No agents registered.")
	}
	return nil
}

func runAgentShow(cmd *cobra.Command, args []string) error {
	agentID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/agents/" + agentID)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var agent map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&agent); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Agent ID:     %s\n", agent["id"])
	fmt.Printf("Hostname:     %s\n", agent["hostname"])
	fmt.Printf("Status:       %s\n", agent["status"])
	fmt.Printf("Registered:   %s\n", agent["registeredAt"])
	fmt.Printf("Last Seen:    %s\n", agent["lastSeenAt"])
	if labels, ok := agent["labels"].(map[string]interface{}); ok && len(labels) > 0 {
		fmt.Printf("Labels:\n")
		for k, v := range labels {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
	return nil
}

func runAgentDelete(cmd *cobra.Command, args []string) error {
	agentID := args[0]

	fmt.Printf("Are you sure you want to remove agent '%s'? (y/N): ", agentID)
	var confirm string
	_, _ = fmt.Scanln(&confirm)
	if !strings.EqualFold(confirm, "y") {
		return fmt.Errorf("aborted")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.delete("/api/v1/agents/" + agentID)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("Agent '%s' removed successfully.\n", agentID)
	return nil
}

func runAgentToken(cmd *cobra.Command, args []string) error {
	label, _ := cmd.Flags().GetString("label")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data := map[string]interface{}{}
	if label != "" {
		data["label"] = label
	}

	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	resp, err := client.post("/api/v1/agents/tokens", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	token, ok := result["token"].(string)
	if !ok {
		return fmt.Errorf("invalid API response: missing or invalid token")
	}

	fmt.Println("Agent Registration Token:")
	fmt.Printf("  Token: %s\n", token)
	if label != "" {
		fmt.Printf("  Label: %s\n", label)
	}
	if expiresAt, ok := result["expires_at"]; ok {
		fmt.Printf("  Expires: %s\n", expiresAt)
	}
	fmt.Println("\nUse this command on the target server:")
	fmt.Printf("  vcdeploy-agent register --master <master-url> --token %s\n", token)
	return nil
}

// --- Deployment Commands ---

func runDeploymentTrigger(cmd *cobra.Command, args []string) error {
	project := args[0]
	branch, _ := cmd.Flags().GetString("branch")
	target, _ := cmd.Flags().GetString("target")
	schedule, _ := cmd.Flags().GetString("schedule")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"project": project,
	}
	if branch != "" {
		data["branch"] = branch
	}
	if target != "" {
		data["target"] = target
	}
	if schedule != "" {
		data["scheduled_at"] = schedule
	}

	body, _ := json.Marshal(data)
	resp, err := client.post("/api/v1/projects/"+project+"/deploy", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(bodyBytes))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if schedule != "" {
		fmt.Printf("Deployment scheduled: %s\n", result["id"])
		fmt.Printf("Scheduled for: %s\n", result["scheduled_at"])
	} else {
		fmt.Printf("Deployment triggered: %s\n", result["id"])
		fmt.Printf("Status: %s\n", result["status"])
	}
	return nil
}

func runDeploymentList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/deployments?limit=20")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var deployments []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&deployments); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPROJECT\tBRANCH\tSTATUS\tSTARTED")
	for _, d := range deployments {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			d["id"], d["project"], d["branch"], d["status"], d["startedAt"])
	}
	w.Flush()
	return nil
}

func runDeploymentStatus(cmd *cobra.Command, args []string) error {
	deploymentID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/deployments/" + deploymentID)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var d map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Deployment:   %s\n", d["id"])
	fmt.Printf("Project:      %s\n", d["project"])
	fmt.Printf("Branch:       %s\n", d["branch"])
	fmt.Printf("Target:       %s\n", d["target"])
	fmt.Printf("Status:       %s\n", d["status"])
	fmt.Printf("Started:      %s\n", d["startedAt"])
	if d["completedAt"] != nil {
		fmt.Printf("Completed:    %s\n", d["completedAt"])
	}
	if d["triggeredBy"] != nil {
		fmt.Printf("Triggered By: %s\n", d["triggeredBy"])
	}
	if d["errorMessage"] != nil && d["errorMessage"] != "" {
		fmt.Printf("Error:        %s\n", d["errorMessage"])
	}
	return nil
}

func runDeploymentCancel(cmd *cobra.Command, args []string) error {
	deploymentID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.post("/api/v1/deployments/"+deploymentID+"/cancel", nil)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	fmt.Printf("Deployment '%s' cancelled.\n", deploymentID)
	return nil
}

func runDeploymentLogs(cmd *cobra.Command, args []string) error {
	deploymentID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/deployments/" + deploymentID + "/logs")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var logs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	for _, log := range logs {
		timestamp := log["createdAt"]
		level := log["level"]
		message := log["message"]
		fmt.Printf("[%s] %s: %s\n", timestamp, level, message)
	}

	if len(logs) == 0 {
		fmt.Println("No logs available for this deployment.")
	}
	return nil
}

// --- Config Commands ---

func runConfigShow(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/settings/export")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var settings map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	for category, values := range settings {
		fmt.Printf("[%s]\n", category)
		for key, val := range values {
			fmt.Printf("  %s = %v\n", key, val)
		}
		fmt.Println()
	}
	return nil
}

func runConfigExport(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/settings/export")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	// Output as JSON
	_, _ = io.Copy(os.Stdout, resp.Body)
	fmt.Println()
	return nil
}

func runConfigImport(cmd *cobra.Command, args []string) error {
	filename := args[0]

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.post("/api/v1/settings/import", strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	fmt.Printf("Imported %v settings successfully.\n", result["imported"])
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	keyPath := args[0]
	value := args[1]

	parts := strings.SplitN(keyPath, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("key must be in format 'category.key'")
	}
	category := parts[0]
	key := parts[1]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data, _ := json.Marshal(map[string]string{key: value})
	resp, err := client.do("PUT", "/api/v1/settings/"+category, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	fmt.Printf("Set %s.%s = %s\n", category, key, value)
	return nil
}

// --- API Key Commands ---

func runAPIKeyList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/apikeys")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var keys []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tCREATED\tEXPIRES\tLAST USED")
	for _, k := range keys {
		expires := "never"
		if k["expiresAt"] != nil {
			expires = fmt.Sprintf("%v", k["expiresAt"])
		}
		lastUsed := "never"
		if k["lastUsedAt"] != nil {
			lastUsed = fmt.Sprintf("%v", k["lastUsedAt"])
		}
		fmt.Fprintf(w, "%.0f\t%s\t%s\t%s\t%s\n",
			k["id"], k["name"], k["createdAt"], expires, lastUsed)
	}
	w.Flush()

	if len(keys) == 0 {
		fmt.Println("No API keys found.")
	}
	return nil
}

func runAPIKeyCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	expiresIn, _ := cmd.Flags().GetInt("expires")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data, _ := json.Marshal(map[string]interface{}{
		"name":            name,
		"expires_in_days": expiresIn,
	})

	resp, err := client.post("/api/v1/apikeys", strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Println("API Key created successfully!")
	fmt.Println()
	fmt.Printf("  Name: %s\n", result["name"])
	fmt.Printf("  Key:  %s\n", result["key"])
	fmt.Println()
	fmt.Println("IMPORTANT: Save this key now. You won't be able to see it again!")
	return nil
}

func runAPIKeyRevoke(cmd *cobra.Command, args []string) error {
	keyID := args[0]

	fmt.Printf("Are you sure you want to revoke API key '%s'? (y/N): ", keyID)
	var confirm string
	_, _ = fmt.Scanln(&confirm)
	if !strings.EqualFold(confirm, "y") {
		return fmt.Errorf("aborted")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.delete("/api/v1/apikeys/" + keyID)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	fmt.Printf("API key '%s' revoked successfully.\n", keyID)
	return nil
}

// --- Admin Command ---

func runAdmin(cmd *cobra.Command, args []string) error {
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	email, _ := cmd.Flags().GetString("email")

	// Check if this is remote mode
	master, _ := cmd.Flags().GetString("master")
	token, _ := cmd.Flags().GetString("token")

	// Also check environment variables
	if master == "" {
		master = os.Getenv("VCDEPLOY_MASTER")
	}
	if token == "" {
		token = os.Getenv("VCDEPLOY_TOKEN")
	}

	// Prompt for password if not provided
	if password == "" {
		var err error
		password, err = promptPassword()
		if err != nil {
			return err
		}
	}

	// Validate password complexity
	if err := security.ValidatePassword(password); err != nil {
		return fmt.Errorf("password validation failed: %w", err)
	}

	// Remote mode - use API
	if master != "" && token != "" {
		return runAdminRemote(cmd, username, password, email)
	}

	// Local mode - direct database access
	return runAdminLocal(username, password, email)
}

// promptPassword prompts for password with confirmation.
func promptPassword() (string, error) {
	fmt.Print("Enter password: ")
	pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	fmt.Println()

	fmt.Print("Confirm password: ")
	pwBytes2, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	fmt.Println()

	if !bytes.Equal(pwBytes, pwBytes2) {
		return "", fmt.Errorf("passwords do not match")
	}

	return string(pwBytes), nil
}

// runAdminLocal handles admin setup via direct database access.
func runAdminLocal(username, password, email string) error {
	// Initialize config to get database path
	sysCfg := config.MustGetSystemConfig()
	dbPath := sysCfg.DatabasePath()

	// Check if server might be running by trying to connect to the configured port
	if isServerRunning(sysCfg) {
		fmt.Println("Warning: Server appears to be running.")
		fmt.Println("If VCDEPLOY_ADMIN_PASSWORD is set, changes may be overwritten on restart.")
		fmt.Print("Continue? [y/N]: ")
		var confirm string
		_, _ = fmt.Scanln(&confirm)
		if !strings.EqualFold(confirm, "y") {
			return fmt.Errorf("aborted")
		}
	}

	// Open database
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Create user service
	userSvc := users.New(db)
	ctx := context.Background()

	// Try to find existing admin user
	existingUser, err := userSvc.GetByUsername(ctx, username)
	if err == nil && existingUser != nil {
		// Update existing user
		existingUser.Email = email
		if err := userSvc.Update(ctx, existingUser); err != nil {
			return fmt.Errorf("update admin email: %w", err)
		}
		if err := userSvc.UpdatePassword(ctx, existingUser.ID, password); err != nil {
			return fmt.Errorf("update admin password: %w", err)
		}
		fmt.Printf("Admin user '%s' updated successfully.\n", username)
		return nil
	}

	// Create new admin user
	user, err := userSvc.Create(ctx, username, password, email, "admin")
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	fmt.Printf("Admin user '%s' created successfully.\n", user.Username)
	return nil
}

// runAdminRemote handles admin setup via API.
func runAdminRemote(cmd *cobra.Command, username, password, email string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// First, try to list users to find if admin exists
	resp, err := client.get("/api/v1/users")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}

	var usersList []map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&usersList)
	resp.Body.Close()

	// Look for user by username
	var userID float64
	for _, u := range usersList {
		if u["username"] == username {
			id, ok := u["id"].(float64)
			if ok {
				userID = id
				break
			}
		}
	}

	if userID > 0 {
		// Update existing user
		data, _ := json.Marshal(map[string]string{
			"email":    email,
			"password": password,
		})

		req, _ := http.NewRequest("PATCH", fmt.Sprintf("/api/v1/users/%.0f", userID), strings.NewReader(string(data)))
		resp, err := client.do("PATCH", fmt.Sprintf("/api/v1/users/%.0f", userID), strings.NewReader(string(data)))
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
		}
		fmt.Printf("Admin user '%s' updated successfully.\n", username)
		_ = req // suppress unused warning
	} else {
		// Create new user
		data, _ := json.Marshal(map[string]string{
			"username": username,
			"email":    email,
			"password": password,
			"role":     "admin",
		})

		resp, err := client.post("/api/v1/users", strings.NewReader(string(data)))
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
		}
		fmt.Printf("Admin user '%s' created successfully.\n", username)
	}

	return nil
}

// isServerRunning checks if the server appears to be running.
func isServerRunning(sysCfg *config.SystemConfig) bool {
	// Try to connect to common ports
	ports := []string{":8080", ":9090"}

	for _, port := range ports {
		conn, err := net.DialTimeout("tcp", "localhost"+port, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
	}
	return false
}
