package commands

import (
	"bufio"
	"context"
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

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// agentCmd handles agent management commands
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent management",
	Long:  "Commands for managing deployment agents.",
}

// agentProvisionCmd handles provision subcommands under agent
var agentProvisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Agent provisioning",
	Long: `Commands for provisioning agents via SSH.

Agent provisioning allows you to install and configure the vcdeploy agent
on remote servers using SSH. The provisioning process:

1. Connects to the target server via SSH
2. Downloads the agent binary
3. Installs and configures the agent
4. Registers the agent with the master
5. Starts the agent service

All commands require API authentication via --master and --token flags.`,
}

func init() {
	rootCmd.AddCommand(agentCmd)

	// Agent subcommands
	agentListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all agents",
		RunE:  runAgentList,
	}
	agentListCmd.Flags().StringP("output", "o", "table", "Output format: table, json, yaml")
	agentCmd.AddCommand(agentListCmd)

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

	// Add provision subcommand group
	agentCmd.AddCommand(agentProvisionCmd)

	// agent provision create (start provisioning)
	provisionCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new provisioning job",
		Long: `Provision an agent on a remote server via SSH.

The provisioning process:
1. Connects to the target server via SSH
2. Downloads the agent binary
3. Installs and configures the agent
4. Registers the agent with the master
5. Starts the agent service`,
		Example: `  # Basic provisioning
  vcdeploy agent provision create --host myserver.com --master localhost:9000 --token <token>

  # With custom options
  vcdeploy agent provision create --host myserver.com --user deploy --port 2222 \
    --agent-id prod-web-01 --groups web,production`,
		RunE: runAgentProvisionCreate,
	}
	provisionCreateCmd.Flags().String("host", "", "Target host to provision (required)")
	provisionCreateCmd.Flags().IntP("port", "p", 22, "SSH port")
	provisionCreateCmd.Flags().StringP("user", "u", "root", "SSH user")
	provisionCreateCmd.Flags().String("ssh-key", "", "SSH key ID to use for authentication")
	provisionCreateCmd.Flags().String("agent-id", "", "Agent ID to assign (auto-generated if not set)")
	provisionCreateCmd.Flags().String("agent-name", "", "Agent display name")
	provisionCreateCmd.Flags().String("groups", "", "Comma-separated list of groups to assign")
	provisionCreateCmd.Flags().Bool("no-start", false, "Don't start the agent after installation")
	_ = provisionCreateCmd.MarkFlagRequired("host")
	agentProvisionCmd.AddCommand(provisionCreateCmd)

	// agent provision list
	provisionListCmd := &cobra.Command{
		Use:   "list",
		Short: "List provisioning jobs",
		Long: `List recent provisioning jobs with their status.

Example:
  vcdeploy agent provision list --master localhost:9000 --token <token>`,
		RunE: runAgentProvisionList,
	}
	provisionListCmd.Flags().IntP("limit", "n", 20, "Maximum number of jobs to show")
	provisionListCmd.Flags().String("status", "", "Filter by status (pending, running, completed, failed)")
	agentProvisionCmd.AddCommand(provisionListCmd)

	// agent provision show
	provisionShowCmd := &cobra.Command{
		Use:   "show [provision-id]",
		Short: "Show provisioning job status",
		Long: `Check the status of a provisioning job.

Example:
  vcdeploy agent provision show abc123 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runAgentProvisionShow,
	}
	agentProvisionCmd.AddCommand(provisionShowCmd)

	// agent provision logs
	provisionLogsCmd := &cobra.Command{
		Use:   "logs [provision-id]",
		Short: "Show provisioning job logs",
		Long: `View the output logs from a provisioning job.

Example:
  vcdeploy agent provision logs abc123 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runAgentProvisionLogs,
	}
	provisionLogsCmd.Flags().BoolP("follow", "f", false, "Follow log output in real-time")
	agentProvisionCmd.AddCommand(provisionLogsCmd)
}

func runAgentList(cmd *cobra.Command, args []string) error {
	outputFormat, _ := cmd.Flags().GetString("output")

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

	var result paginatedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	switch outputFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Items)
	case "yaml":
		return yaml.NewEncoder(os.Stdout).Encode(result.Items)
	default:
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tHOSTNAME\tSTATUS\tLAST SEEN")
		for _, a := range result.Items {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				a["id"], a["hostname"], a["status"], a["lastSeenAt"])
		}
		w.Flush()

		if len(result.Items) == 0 {
			fmt.Println("No agents registered.")
		}
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
	_, _ = fmt.Scanln(&confirm) //nolint:errcheck // user confirmation prompt
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

// Note: runAgentUpdate is defined in root.go with a more complete implementation

// --- Provision Types ---

type agentProvisionRequest struct {
	Host      string   `json:"host"`
	Port      int      `json:"port"`
	User      string   `json:"user"`
	SSHKeyID  string   `json:"ssh_key_id,omitempty"`
	AgentID   string   `json:"agent_id,omitempty"`
	AgentName string   `json:"agent_name,omitempty"`
	Groups    []string `json:"groups,omitempty"`
	NoStart   bool     `json:"no_start,omitempty"`
}

type agentProvisionJobInfo struct {
	ID        string    `json:"id"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	User      string    `json:"user"`
	AgentID   string    `json:"agent_id,omitempty"`
	Status    string    `json:"status"`
	Progress  int       `json:"progress,omitempty"`
	Message   string    `json:"message,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Duration  string    `json:"duration,omitempty"`
}

type agentProvisionListResponse struct {
	Jobs []agentProvisionJobInfo `json:"jobs"`
}

// agentProvisionLogEntry represents a single log entry
type agentProvisionLogEntry struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

// agentProvisionLogsResult is the API response structure
type agentProvisionLogsResult struct {
	JobID  string                   `json:"jobId"`
	Status string                   `json:"status"`
	Stage  string                   `json:"stage"`
	Logs   []agentProvisionLogEntry `json:"logs"`
	Output string                   `json:"output,omitempty"`
}

// --- Provision Functions ---

func runAgentProvisionCreate(cmd *cobra.Command, args []string) error {
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")
	user, _ := cmd.Flags().GetString("user")
	sshKeyID, _ := cmd.Flags().GetString("ssh-key")
	agentID, _ := cmd.Flags().GetString("agent-id")
	agentName, _ := cmd.Flags().GetString("agent-name")
	groupsStr, _ := cmd.Flags().GetString("groups")
	noStart, _ := cmd.Flags().GetBool("no-start")

	if host == "" {
		return fmt.Errorf("--host is required")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	req := agentProvisionRequest{
		Host:      host,
		Port:      port,
		User:      user,
		SSHKeyID:  sshKeyID,
		AgentID:   agentID,
		AgentName: agentName,
		NoStart:   noStart,
	}

	if groupsStr != "" {
		req.Groups = strings.Split(groupsStr, ",")
		for i, g := range req.Groups {
			req.Groups[i] = strings.TrimSpace(g)
		}
	}

	body, _ := json.Marshal(req)

	fmt.Printf("Starting provisioning on %s:%d...\n", host, port)

	resp, err := client.post("/api/v1/agents/provision", jsonReader(body))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 202 {
		return handleAPIError(resp)
	}

	var job agentProvisionJobInfo
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Provisioning job started.\n")
	fmt.Printf("Job ID:  %s\n", job.ID)
	fmt.Printf("Status:  %s\n", job.Status)
	fmt.Printf("\nTo check status:\n")
	fmt.Printf("  vcdeploy agent provision show %s\n", job.ID)
	fmt.Printf("\nTo view logs:\n")
	fmt.Printf("  vcdeploy agent provision logs %s\n", job.ID)

	return nil
}

func runAgentProvisionList(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	statusFilter, _ := cmd.Flags().GetString("status")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/v1/agents/provision?limit=%d", limit)
	if statusFilter != "" {
		path += "&status=" + statusFilter
	}

	resp, err := client.get(path)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result agentProvisionListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.Jobs) == 0 {
		fmt.Println("No provisioning jobs found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tHOST\tSTATUS\tAGENT ID\tSTARTED\tDURATION")

	for i := range result.Jobs {
		job := &result.Jobs[i]
		started := ""
		if !job.StartedAt.IsZero() {
			started = job.StartedAt.Format("01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s:%d\t%s\t%s\t%s\t%s\n",
			truncate(job.ID, 12),
			job.Host,
			job.Port,
			job.Status,
			job.AgentID,
			started,
			job.Duration,
		)
	}
	w.Flush()

	return nil
}

func runAgentProvisionShow(cmd *cobra.Command, args []string) error {
	jobID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/agents/provision/" + jobID + "/status")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var job agentProvisionJobInfo
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Job ID:     %s\n", job.ID)
	fmt.Printf("Target:     %s@%s:%d\n", job.User, job.Host, job.Port)
	fmt.Printf("Status:     %s\n", job.Status)

	if job.Progress > 0 && job.Progress < 100 {
		fmt.Printf("Progress:   %d%%\n", job.Progress)
	}

	if job.Message != "" {
		fmt.Printf("Message:    %s\n", job.Message)
	}

	if job.AgentID != "" {
		fmt.Printf("Agent ID:   %s\n", job.AgentID)
	}

	if !job.StartedAt.IsZero() {
		fmt.Printf("Started:    %s\n", job.StartedAt.Format(time.RFC3339))
	}

	if !job.EndedAt.IsZero() {
		fmt.Printf("Ended:      %s\n", job.EndedAt.Format(time.RFC3339))
		if job.Duration != "" {
			fmt.Printf("Duration:   %s\n", job.Duration)
		}
	}

	if job.Error != "" {
		fmt.Printf("\nError:\n%s\n", job.Error)
	}

	// Show status indicator
	fmt.Println()
	switch job.Status {
	case "pending":
		fmt.Println("⏳ Job is queued and waiting to start")
	case "running":
		fmt.Println("⚙️  Job is in progress")
	case "completed":
		fmt.Println("✓ Provisioning completed successfully")
	case "failed":
		fmt.Println("✗ Provisioning failed")
	}

	return nil
}

func runAgentProvisionLogs(cmd *cobra.Command, args []string) error {
	jobID := args[0]
	follow, _ := cmd.Flags().GetBool("follow")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	if follow {
		return followAgentProvisionLogs(cmd.Context(), client, jobID)
	}

	return showAgentProvisionLogs(client, jobID)
}

// showAgentProvisionLogs fetches and displays logs once
func showAgentProvisionLogs(client *apiClient, jobID string) error {
	resp, err := client.get("/api/v1/agents/provision/" + jobID + "/logs")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result agentProvisionLogsResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	for _, entry := range result.Logs {
		ts := entry.Timestamp.Format("15:04:05")
		fmt.Printf("[%s] %s: %s\n", ts, entry.Level, entry.Message)
	}

	// Print raw output if available
	if result.Output != "" {
		if len(result.Logs) > 0 {
			fmt.Println("\n--- Raw Output ---")
		}
		fmt.Println(result.Output)
	}

	return nil
}

// followAgentProvisionLogs polls for logs and streams them in real-time
func followAgentProvisionLogs(ctx context.Context, client *apiClient, jobID string) error {
	// Set up signal handler for graceful exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	var lastID int64
	seenIDs := make(map[int64]bool)
	terminalStates := map[string]bool{
		"completed": true,
		"failed":    true,
		"cancelled": true,
	}

	// Use a buffered writer for efficient output
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()

	fmt.Fprintln(writer, "Following provision job logs... (Ctrl+C to exit)")
	writer.Flush()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-sigCh:
			fmt.Fprintln(writer, "\nInterrupted.")
			writer.Flush()
			return nil
		case <-ticker.C:
			resp, err := client.get("/api/v1/agents/provision/" + jobID + "/logs")
			if err != nil {
				fmt.Fprintf(writer, "\rError fetching logs: %v\n", err)
				writer.Flush()
				continue
			}

			var result agentProvisionLogsResult
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				resp.Body.Close()
				fmt.Fprintf(writer, "\rError decoding response: %v\n", err)
				writer.Flush()
				continue
			}
			resp.Body.Close()

			// Print only new logs (those we haven't seen before)
			for _, log := range result.Logs {
				if !seenIDs[log.ID] && log.ID > lastID {
					ts := log.Timestamp.Format("15:04:05")
					fmt.Fprintf(writer, "[%s] %s: %s\n", ts, log.Level, log.Message)
					seenIDs[log.ID] = true
					if log.ID > lastID {
						lastID = log.ID
					}
				}
			}
			writer.Flush()

			// Check if job has reached a terminal state
			if terminalStates[result.Status] {
				fmt.Fprintf(writer, "\nJob %s (status: %s, stage: %s)\n", result.JobID, result.Status, result.Stage)
				writer.Flush()
				return nil
			}
		}
	}
}
