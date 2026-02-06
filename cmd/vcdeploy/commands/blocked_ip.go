package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// blockedIPCmd handles IP blocking management commands
var blockedIPCmd = &cobra.Command{
	Use:     "blocked-ip",
	Aliases: []string{"blocked"},
	Short:   "IP blocking management",
	Long: `Commands for managing blocked IP addresses.

Blocked IPs are denied access to all API endpoints and UI.
Use this to protect against malicious actors or rate limit abusers.

All commands require API authentication via --master and --token flags.`,
}

func init() {
	rootCmd.AddCommand(blockedIPCmd)

	// List blocked IPs
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all blocked IPs",
		Long: `List all currently blocked IP addresses.

Example:
  vcdeploy blocked-ip list --master localhost:9000 --token <token>`,
		RunE: runBlockedIPList,
	}
	blockedIPCmd.AddCommand(listCmd)

	// Block an IP
	addCmd := &cobra.Command{
		Use:   "create <ip>",
		Short: "Block an IP address",
		Long: `Block an IP address from accessing the API.

Example:
  vcdeploy blocked-ip create 192.0.2.100 --reason "Malicious activity" \
    --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runBlockedIPAdd,
	}
	addCmd.Flags().StringP("reason", "r", "", "Reason for blocking")
	addCmd.Flags().String("duration", "", "Block duration (e.g., 24h, 7d)")
	blockedIPCmd.AddCommand(addCmd)

	// Unblock an IP
	removeCmd := &cobra.Command{
		Use:   "delete <ip>",
		Short: "Unblock an IP address",
		Long: `Remove an IP address from the blocklist.

Example:
  vcdeploy blocked-ip delete 192.0.2.100 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runBlockedIPRemove,
	}
	blockedIPCmd.AddCommand(removeCmd)
}

// --- Blocked IP Types ---

type blockedIPListResponse struct {
	BlockedIPs []blockedIPInfo `json:"blocked_ips"`
}

type blockedIPInfo struct {
	IP        string `json:"ip"`
	Reason    string `json:"reason"`
	BlockedAt string `json:"blocked_at"`
	ExpiresAt string `json:"expires_at,omitempty"`
	BlockedBy string `json:"blocked_by"`
}

func runBlockedIPList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/blocked-ips")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var result blockedIPListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.BlockedIPs) == 0 {
		fmt.Println("No blocked IPs found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "IP\tREASON\tBLOCKED AT\tEXPIRES\tBLOCKED BY")
	for _, ip := range result.BlockedIPs {
		expires := ip.ExpiresAt
		if expires == "" {
			expires = "never"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			ip.IP, ip.Reason, ip.BlockedAt, expires, ip.BlockedBy)
	}
	w.Flush()
	return nil
}

func runBlockedIPAdd(cmd *cobra.Command, args []string) error {
	ip := args[0]
	reason, _ := cmd.Flags().GetString("reason")
	duration, _ := cmd.Flags().GetString("duration")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"ip": ip,
	}
	if reason != "" {
		data["reason"] = reason
	}
	if duration != "" {
		data["duration"] = duration
	}

	body, _ := json.Marshal(data)
	resp, err := client.post("/api/v1/blocked-ips", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("IP %s blocked successfully.\n", ip)
	return nil
}

func runBlockedIPRemove(cmd *cobra.Command, args []string) error {
	ip := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.delete("/api/v1/blocked-ips/" + ip)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("IP %s unblocked successfully.\n", ip)
	return nil
}
