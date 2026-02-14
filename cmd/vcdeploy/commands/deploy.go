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

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/spf13/cobra"
)

// deploymentCmd handles deployment commands
var deploymentCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deployment commands",
	Long: `Commands for viewing and managing deployments.

To create a new deployment:
  vcdeploy deploy create --project myproject --branch main`,
}

func init() {
	rootCmd.AddCommand(deploymentCmd)

	// Deployment subcommands (view-only operations)
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List recent deployments",
		RunE:  runDeploymentList,
	}
	listCmd.Flags().IntP("limit", "n", 20, "Maximum number of deployments to show")
	deploymentCmd.AddCommand(listCmd)

	deploymentCmd.AddCommand(&cobra.Command{
		Use:   "show [deployment-id]",
		Short: "Show deployment details",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeploymentStatus,
	})

	deploymentCmd.AddCommand(&cobra.Command{
		Use:   "cancel [deployment-id]",
		Short: "Cancel a running deployment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeploymentCancel,
	})

	logsCmd := &cobra.Command{
		Use:   "logs [deployment-id]",
		Short: "View deployment logs",
		Long: `View deployment logs for a specific deployment.

Use --follow to stream logs in real-time as they are generated.`,
		Args: cobra.ExactArgs(1),
		RunE: runDeploymentLogs,
	}
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output in real-time")
	deploymentCmd.AddCommand(logsCmd)

	// Create deployment
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new deployment",
		Long: `Create a new deployment for a project.

Examples:
  vcdeploy deploy create --project myproject
  vcdeploy deploy create --project myproject --branch feature-x
  vcdeploy deploy create --project myproject --commit abc123
  vcdeploy deploy create --project myproject --force --follow
  vcdeploy deploy create --project myproject --dry-run`,
		RunE: runDeployCreate,
	}
	createCmd.Flags().StringP("project", "p", "", "Project to deploy (required)")
	createCmd.Flags().String("branch", "", "Branch to deploy (default: project default)")
	createCmd.Flags().String("commit", "", "Specific commit SHA to deploy")
	createCmd.Flags().String("target", "", "Target environment")
	createCmd.Flags().Bool("dry-run", false, "Validate without deploying")
	createCmd.Flags().Bool("force", false, "Force deployment (bypass locks)")
	createCmd.Flags().BoolP("follow", "f", false, "Follow deployment logs in real-time")
	_ = createCmd.MarkFlagRequired("project")
	deploymentCmd.AddCommand(createCmd)

	// Rollback deployment
	rollbackCmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback a project to a previous release",
		Long: `Rollback a project deployment to a previous successful release.

By default, rolls back to the most recent successful release.
Use --to to target a specific deployment ID.

Examples:
  vcdeploy deploy rollback --project myproject
  vcdeploy deploy rollback --project myproject --to dep_abc123
  vcdeploy deploy rollback --project myproject --target staging`,
		RunE: runDeployRollback,
	}
	rollbackCmd.Flags().StringP("project", "p", "", "Project to rollback (required)")
	rollbackCmd.Flags().String("target", "", "Target environment")
	rollbackCmd.Flags().String("to", "", "Deployment ID to rollback to")
	_ = rollbackCmd.MarkFlagRequired("project")
	deploymentCmd.AddCommand(rollbackCmd)
}

func runDeploymentList(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get(fmt.Sprintf("/api/v1/deployments?limit=%d", limit))
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPROJECT\tBRANCH\tSTATUS\tSTARTED")
	for _, d := range result.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			d["id"], d["project"], d["branch"], d["status"], d["startedAt"])
	}
	_ = w.Flush() // #nosec G104 - best effort output flush
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
	follow, _ := cmd.Flags().GetBool("follow")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	if follow {
		return followDeploymentLogs(cmd.Context(), client, deploymentID)
	}

	return showDeploymentLogs(client, deploymentID)
}

func runDeployCreate(cmd *cobra.Command, _ []string) error {
	projectName, _ := cmd.Flags().GetString("project")
	branch, _ := cmd.Flags().GetString("branch")
	commit, _ := cmd.Flags().GetString("commit")
	target, _ := cmd.Flags().GetString("target")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")
	follow, _ := cmd.Flags().GetBool("follow")

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
		if err := initConfig(cmd); err != nil {
			return err
		}
		svc, cleanup, err := initServices()
		if err != nil {
			return err
		}
		defer cleanup()

		_, err = svc.Projects.GetByName(cmd.Context(), projectName)
		if err != nil {
			return fmt.Errorf("get project: %w", err)
		}
		fmt.Println("\n✅ Dry run completed successfully.")
		fmt.Println("   Configuration is valid, no changes were made.")
		return nil
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// Create deployment request
	reqBody := map[string]interface{}{
		"project": projectName,
		"force":   force,
	}
	if branch != "" {
		reqBody["branch"] = branch
	}
	if commit != "" {
		reqBody["commit"] = commit
	}
	if target != "" {
		reqBody["target"] = target
	}

	reqJSON, _ := json.Marshal(reqBody)
	fmt.Println("📡 Triggering deployment via master...")
	resp, err := client.post("/api/v1/deployments", strings.NewReader(string(reqJSON)))
	if err != nil {
		return fmt.Errorf("master not reachable: %w\nStart the master with: vcdeploy master start", err)
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

	// If --follow is specified, stream logs via SSE
	if follow {
		fmt.Println("📜 Streaming deployment logs...")
		return streamDeploymentLogs(cmd.Context(), client, deployResp.ID, false)
	}

	// Poll for deployment status (interruptible with Ctrl+C)
	fmt.Println("⏳ Waiting for deployment to complete...")
	for i := 0; i < 120; i++ { // Max 10 minutes
		select {
		case <-cmd.Context().Done():
			fmt.Println("\n⚠️  Interrupted. Deployment continues in background.")
			fmt.Printf("   Check status with: vcdeploy deploy show %s\n", deployResp.ID)
			return nil
		case <-time.After(5 * time.Second):
		}

		statusResp, err := client.get("/api/v1/deployments/" + deployResp.ID)
		if err != nil {
			continue
		}

		var status struct {
			Status string `json:"status"`
		}
		_ = json.NewDecoder(statusResp.Body).Decode(&status) //nolint:errcheck // best effort decode
		_ = statusResp.Body.Close()                          // #nosec G104 - best effort cleanup

		switch status.Status {
		case "success", "completed":
			fmt.Println("\n✅ Deployment completed successfully!")
			return nil
		case "failed", "error":
			return fmt.Errorf("deployment failed. Check logs with: vcdeploy deploy logs %s", deployResp.ID)
		case "cancelled":
			return fmt.Errorf("deployment was cancelled")
		}
		fmt.Print(".")
	}

	fmt.Println("\n⚠️  Deployment still in progress. Check status with:")
	fmt.Printf("   vcdeploy deploy show %s\n", deployResp.ID)
	return nil
}

func runDeployRollback(cmd *cobra.Command, _ []string) error {
	projectName, _ := cmd.Flags().GetString("project")
	target, _ := cmd.Flags().GetString("target")
	toDeployment, _ := cmd.Flags().GetString("to")

	fmt.Printf("🔙 Rolling back project: %s\n", projectName)
	if target != "" {
		fmt.Printf("   Target: %s\n", target)
	}
	if toDeployment != "" {
		fmt.Printf("   To deployment: %s\n", toDeployment)
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

	exec, cleanup, err := NewExecutor(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	if exec.IsRemote() {
		// Use API mode
		reqBody := map[string]interface{}{
			"project": projectName,
		}
		if toDeployment != "" {
			reqBody["deployment_id"] = toDeployment
		}
		if target != "" {
			reqBody["target"] = target
		}

		fmt.Println("\n🔄 Triggering rollback via master...")
		resp, err := exec.API().Post("/api/v1/projects/"+projectName+"/rollback", reqBody)
		if err != nil {
			return fmt.Errorf("rollback failed: %w", err)
		}

		fmt.Println("✅ Rollback triggered successfully!")
		if resp != nil {
			if rollbackID, ok := resp["rollback_id"].(string); ok && rollbackID != "" {
				fmt.Printf("   Rollback ID: %s\n", rollbackID)
			}
			if deploymentID, ok := resp["deployment_id"].(string); ok && deploymentID != "" {
				fmt.Printf("   Deployment ID: %s\n", deploymentID)
			}
		}
		return nil
	}

	// Direct mode - find previous successful deployment and create rollback
	ctx := context.Background()
	project, err := exec.Services().Projects.GetByName(ctx, projectName)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	// Get recent deployments
	deployments, err := exec.Services().Deployments.ListRecent(ctx, 10)
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	// Find deployments for this project
	var projectDeployments []*storage.DeploymentRecord
	for _, d := range deployments {
		if d.ProjectID != nil && *d.ProjectID == project.ID {
			projectDeployments = append(projectDeployments, d)
		}
	}

	if len(projectDeployments) == 0 {
		return fmt.Errorf("no deployments found for project %s", projectName)
	}

	var targetDeploy *storage.DeploymentRecord
	if toDeployment != "" {
		// Find specific deployment by ID
		for _, d := range projectDeployments {
			if d.ID == toDeployment {
				targetDeploy = d
				break
			}
		}
		if targetDeploy == nil {
			return fmt.Errorf("deployment %s not found for project %s", toDeployment, projectName)
		}
	} else {
		// Find previous successful deployment (skip current)
		for i, d := range projectDeployments {
			if i > 0 && d.Status == storage.DeploymentStatusSuccess {
				targetDeploy = d
				break
			}
		}
		if targetDeploy == nil {
			return fmt.Errorf("no previous successful deployment found for rollback")
		}
	}

	commitDisplay := targetDeploy.CommitHash
	if len(commitDisplay) > 8 {
		commitDisplay = commitDisplay[:8]
	}
	fmt.Printf("\n🔄 Creating rollback deployment to release %d (commit: %s)...\n", targetDeploy.ReleaseNumber, commitDisplay)

	// Create rollback deployment
	rollback := &storage.DeploymentRecord{
		Project:       projectName,
		ProjectID:     &project.ID,
		Branch:        targetDeploy.Branch,
		CommitHash:    targetDeploy.CommitHash,
		Status:        storage.DeploymentStatusPending,
		ReleaseNumber: targetDeploy.ReleaseNumber,
		TriggeredBy:   "cli",
		TriggerSource: "rollback",
	}
	if target != "" {
		rollback.Target = target
	}

	if err := exec.Services().Deployments.Create(ctx, rollback); err != nil {
		return fmt.Errorf("create rollback deployment: %w", err)
	}

	fmt.Println("✅ Rollback deployment created!")
	fmt.Printf("   Deployment ID: %s\n", rollback.ID)
	fmt.Printf("   Rolling back to release: %d\n", targetDeploy.ReleaseNumber)

	return nil
}

// showDeploymentLogs fetches and displays logs once
func showDeploymentLogs(client *apiClient, deploymentID string) error {
	resp, err := client.get("/api/v1/deployments/" + deploymentID + "/logs")
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

	for _, log := range result.Items {
		timestamp := log["createdAt"]
		level := log["level"]
		message := log["message"]
		fmt.Printf("[%s] %s: %s\n", timestamp, level, message)
	}

	if len(result.Items) == 0 {
		fmt.Println("No logs available for this deployment.")
	}
	return nil
}

// sseEvent represents a Server-Sent Events message
type sseEvent struct {
	Event string
	Data  string
}

// followDeploymentLogs streams logs in real-time via SSE
func followDeploymentLogs(ctx context.Context, client *apiClient, deploymentID string) error {
	return streamDeploymentLogs(ctx, client, deploymentID, true)
}

// streamDeploymentLogs connects to the SSE stream for deployment logs
func streamDeploymentLogs(ctx context.Context, client *apiClient, deploymentID string, showHeader bool) error {
	// Set up signal handler for graceful exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// Create context that can be cancelled
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle signals in a goroutine
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-streamCtx.Done():
		}
	}()

	// Use a buffered writer for efficient output
	writer := bufio.NewWriter(os.Stdout)
	defer func() { _ = writer.Flush() }() // #nosec G104 - best effort flush

	if showHeader {
		fmt.Fprintln(writer, "Following deployment logs... (Ctrl+C to exit)")
		_ = writer.Flush() // #nosec G104 - best effort flush
	}

	// Build SSE streaming URL
	url := client.baseURL + "/api/v1/deployments/" + deploymentID + "/logs?stream=true"

	// Create request with streaming context (no timeout for SSE)
	req, err := http.NewRequestWithContext(streamCtx, "GET", url, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+client.token)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// Use a client without timeout for SSE streaming
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		if streamCtx.Err() != nil {
			fmt.Fprintln(writer, "\nInterrupted.")
			_ = writer.Flush() // #nosec G104 - best effort flush
			return nil
		}
		return fmt.Errorf("connect to log stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stream error: %s - %s", resp.Status, string(body))
	}

	// Parse SSE stream
	reader := bufio.NewReader(resp.Body)
	var currentEvent sseEvent

	for {
		select {
		case <-streamCtx.Done():
			fmt.Fprintln(writer, "\nInterrupted.")
			_ = writer.Flush() // #nosec G104 - best effort flush
			return nil
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF || streamCtx.Err() != nil {
				break
			}
			return fmt.Errorf("read stream: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")

		// Empty line marks end of event
		if line == "" {
			if currentEvent.Data != "" {
				if err := handleSSEEvent(writer, currentEvent); err != nil {
					return err
				}
				_ = writer.Flush() // #nosec G104 - best effort flush
			}
			currentEvent = sseEvent{}
			continue
		}

		// Parse SSE fields
		if strings.HasPrefix(line, "event:") {
			currentEvent.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			currentEvent.Data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}

	return nil
}

// handleSSEEvent processes a single SSE event and outputs to writer
func handleSSEEvent(writer *bufio.Writer, event sseEvent) error {
	switch event.Event {
	case "log":
		var logEntry struct {
			Timestamp string `json:"timestamp"`
			Level     string `json:"level"`
			Message   string `json:"message"`
		}
		if err := json.Unmarshal([]byte(event.Data), &logEntry); err != nil {
			// Try simpler format
			fmt.Fprintln(writer, event.Data)
			return nil
		}
		ts := logEntry.Timestamp
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			ts = t.Format("15:04:05")
		}
		fmt.Fprintf(writer, "[%s] %s: %s\n", ts, logEntry.Level, logEntry.Message)

	case "status":
		var status struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(event.Data), &status); err == nil {
			switch status.Status {
			case "completed", "success":
				fmt.Fprintln(writer, "\n✅ Deployment completed successfully!")
			case "failed", "error":
				fmt.Fprintln(writer, "\n❌ Deployment failed!")
			case "cancelled":
				fmt.Fprintln(writer, "\n⚠️  Deployment cancelled.")
			}
		}

	case "done":
		// Stream complete
		return nil

	case "error":
		fmt.Fprintf(writer, "\nError: %s\n", event.Data)

	default:
		// Unknown event, print data if present
		if event.Data != "" {
			fmt.Fprintln(writer, event.Data)
		}
	}
	return nil
}
