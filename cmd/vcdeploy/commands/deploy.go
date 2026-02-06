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
)

// deploymentCmd handles deployment commands
var deploymentCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deployment commands",
	Long: `Commands for viewing and managing deployment records.

To trigger a new deployment, use 'project deploy' instead:
  vcdeploy project deploy myproject --branch main`,
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
