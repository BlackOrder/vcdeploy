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

// hostKeyCmd handles SSH host key management commands
var hostKeyCmd = &cobra.Command{
	Use:     "host-key",
	Aliases: []string{"host-keys"},
	Short:   "SSH host key management",
	Long: `Commands for managing SSH host keys.

Host keys are used to verify server identity during SSH connections.
This prevents man-in-the-middle attacks during agent provisioning.

All commands require API authentication via --master and --token flags.`,
}

func init() {
	rootCmd.AddCommand(hostKeyCmd)

	// List host keys
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all known host keys",
		Long: `List all stored SSH host keys.

Example:
  vcdeploy host-key list --master localhost:9000 --token <token>`,
		RunE: runHostKeyList,
	}
	hostKeyCmd.AddCommand(listCmd)

	// Create host key
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a host key entry",
		Long: `Create a host key entry for a server.

Example:
  vcdeploy host-key create --host server.example.com --key "ssh-ed25519 AAAA..." \
    --master localhost:9000 --token <token>`,
		RunE: runHostKeyAdd,
	}
	createCmd.Flags().String("host", "", "Hostname (required)")
	createCmd.Flags().StringP("key", "k", "", "Public key in OpenSSH format")
	createCmd.Flags().Int("port", 22, "SSH port")
	_ = createCmd.MarkFlagRequired("host")
	hostKeyCmd.AddCommand(createCmd)

	// Delete host key
	deleteCmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a host key",
		Long: `Delete a stored host key by ID.

Example:
  vcdeploy host-key delete 123 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runHostKeyRemove,
	}
	hostKeyCmd.AddCommand(deleteCmd)

	// Scan host
	scanCmd := &cobra.Command{
		Use:   "scan <host>",
		Short: "Scan and add host key",
		Long: `Scan a host for SSH keys and add them.

Example:
  vcdeploy host-key scan server.example.com --master localhost:9000 --token <token>
  vcdeploy host-key scan server.example.com:2222 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runHostKeyScan,
	}
	scanCmd.Flags().Bool("accept", false, "Automatically accept scanned keys")
	hostKeyCmd.AddCommand(scanCmd)
}

// --- Host Key Types ---

type hostKeyListResponse struct {
	HostKeys []hostKeyInfo `json:"host_keys"`
}

type hostKeyInfo struct {
	ID          int64  `json:"id"`
	Hostname    string `json:"hostname"`
	Port        int    `json:"port"`
	KeyType     string `json:"key_type"`
	Fingerprint string `json:"fingerprint"`
	AddedAt     string `json:"added_at"`
}

func runHostKeyList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/host-keys")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var result hostKeyListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.HostKeys) == 0 {
		fmt.Println("No host keys found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tHOST\tPORT\tTYPE\tFINGERPRINT\tADDED")
	for _, key := range result.HostKeys {
		fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s\n",
			key.ID, key.Hostname, key.Port, key.KeyType, key.Fingerprint, key.AddedAt)
	}
	_ = w.Flush() // #nosec G104 - best effort output flush
	return nil
}

func runHostKeyAdd(cmd *cobra.Command, args []string) error {
	host, _ := cmd.Flags().GetString("host")
	key, _ := cmd.Flags().GetString("key")
	port, _ := cmd.Flags().GetInt("port")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"hostname": host,
		"port":     port,
	}
	if key != "" {
		data["public_key"] = key
	}

	body, _ := json.Marshal(data)
	resp, err := client.post("/api/v1/host-keys", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("Host key for %s:%d added successfully.\n", host, port)
	return nil
}

func runHostKeyRemove(cmd *cobra.Command, args []string) error {
	id := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.delete("/api/v1/host-keys/" + id)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("Host key %s removed successfully.\n", id)
	return nil
}

func runHostKeyScan(cmd *cobra.Command, args []string) error {
	host := args[0]
	accept, _ := cmd.Flags().GetBool("accept")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"hostname":    host,
		"auto_accept": accept,
	}

	body, _ := json.Marshal(data)
	resp, err := client.post("/api/v1/host-keys/scan", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var result struct {
		Keys []hostKeyInfo `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.Keys) == 0 {
		fmt.Printf("No keys found for host %s\n", host)
		return nil
	}

	fmt.Printf("Found %d key(s) for %s:\n", len(result.Keys), host)
	for _, key := range result.Keys {
		fmt.Printf("  - %s: %s\n", key.KeyType, key.Fingerprint)
	}

	if accept {
		fmt.Println("\nKeys have been automatically added.")
	} else {
		fmt.Println("\nUse --accept flag to automatically add scanned keys.")
	}
	return nil
}
