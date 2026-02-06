// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// auditCmd handles audit log commands
var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit log management",
	Long:  "Commands for viewing and exporting audit logs.",
}

func init() {
	rootCmd.AddCommand(auditCmd)

	// Audit subcommands
	auditListCmd := &cobra.Command{
		Use:   "list",
		Short: "List audit log entries",
		RunE:  runAuditList,
	}
	auditListCmd.Flags().IntP("limit", "n", 50, "Maximum entries to show")
	auditListCmd.Flags().String("action", "", "Filter by action")
	auditListCmd.Flags().String("resource", "", "Filter by resource type")
	auditCmd.AddCommand(auditListCmd)

	auditExportCmd := &cobra.Command{
		Use:   "export [file]",
		Short: "Export audit logs to JSON file",
		Args:  cobra.ExactArgs(1),
		RunE:  runAuditExport,
	}
	auditExportCmd.Flags().IntP("limit", "n", 1000, "Maximum entries to export")
	auditCmd.AddCommand(auditExportCmd)
}

// runAuditList lists audit log entries.
func runAuditList(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get(fmt.Sprintf("/api/v1/audit?limit=%d", limit))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, body)
	}

	var result paginatedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIMESTAMP\tUSER\tACTION\tRESOURCE\tRESULT")
	for _, e := range result.Items {
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\n",
			e["timestamp"], e["user"], e["action"], e["resource"], e["result"])
	}
	return w.Flush()
}

// runAuditExport exports audit logs to a JSON file.
func runAuditExport(cmd *cobra.Command, args []string) error {
	filename := args[0]
	limit, _ := cmd.Flags().GetInt("limit")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get(fmt.Sprintf("/api/v1/audit?limit=%d", limit))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, body)
	}

	var entries interface{}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	f, err := os.Create(filename) // #nosec G304 - filename is CLI flag, user-intended export destination
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(entries); err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}

	fmt.Printf("Exported audit logs to %s\n", filename)
	return nil
}
