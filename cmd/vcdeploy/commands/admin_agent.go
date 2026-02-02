package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// agentCmd handles agent management commands
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent management",
	Long:  "Commands for managing deployment agents.",
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
