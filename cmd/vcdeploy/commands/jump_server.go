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

// jumpServerCmd handles jump server management commands
var jumpServerCmd = &cobra.Command{
	Use:     "jump-server",
	Aliases: []string{"jump-servers", "jumpserver"},
	Short:   "Jump server management",
	Long: `Commands for managing SSH jump servers (bastion hosts).

Jump servers are used as intermediaries for SSH connections to
target hosts that are not directly accessible.

All commands require API authentication via --master and --token flags.`,
}

func init() {
	rootCmd.AddCommand(jumpServerCmd)

	// List jump servers
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all jump servers",
		Long: `List all configured jump servers.

Example:
  vcdeploy jump-server list --master localhost:9000 --token <token>`,
		RunE: runJumpServerList,
	}
	jumpServerCmd.AddCommand(listCmd)

	// Create jump server
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a jump server configuration",
		Long: `Create a new jump server configuration.

Example:
  vcdeploy jump-server create --name bastion --host bastion.example.com \
    --user admin --ssh-key-id 123 --master localhost:9000 --token <token>`,
		RunE: runJumpServerCreate,
	}
	createCmd.Flags().StringP("name", "n", "", "Jump server name (required)")
	createCmd.Flags().String("host", "", "Jump server hostname (required)")
	createCmd.Flags().Int("port", 22, "SSH port")
	createCmd.Flags().StringP("user", "u", "", "SSH username (required)")
	createCmd.Flags().String("ssh-key-id", "", "SSH key ID to use")
	_ = createCmd.MarkFlagRequired("name")
	_ = createCmd.MarkFlagRequired("host")
	_ = createCmd.MarkFlagRequired("user")
	jumpServerCmd.AddCommand(createCmd)

	// Delete jump server
	deleteCmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a jump server configuration",
		Long: `Delete a jump server configuration by name.

Example:
  vcdeploy jump-server delete bastion --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runJumpServerDelete,
	}
	deleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	jumpServerCmd.AddCommand(deleteCmd)

	// Test jump server
	testCmd := &cobra.Command{
		Use:   "test <name>",
		Short: "Test jump server connectivity",
		Long: `Test SSH connectivity through a jump server.

Example:
  vcdeploy jump-server test bastion --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runJumpServerTest,
	}
	jumpServerCmd.AddCommand(testCmd)
}

// --- Jump Server Types ---

type jumpServerListResponse struct {
	JumpServers []jumpServerInfo `json:"jump_servers"`
}

type jumpServerInfo struct {
	Name      string `json:"name"`
	Hostname  string `json:"hostname"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	SSHKeyID  int64  `json:"ssh_key_id,omitempty"`
	CreatedAt string `json:"created_at"`
}

func runJumpServerList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/jump-servers")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var result jumpServerListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.JumpServers) == 0 {
		fmt.Println("No jump servers configured.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tHOST\tPORT\tUSER\tSSH KEY ID\tCREATED")
	for _, js := range result.JumpServers {
		keyID := "-"
		if js.SSHKeyID > 0 {
			keyID = fmt.Sprintf("%d", js.SSHKeyID)
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\n",
			js.Name, js.Hostname, js.Port, js.Username, keyID, js.CreatedAt)
	}
	w.Flush()
	return nil
}

func runJumpServerCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")
	user, _ := cmd.Flags().GetString("user")
	sshKeyID, _ := cmd.Flags().GetString("ssh-key-id")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"name":     name,
		"hostname": host,
		"port":     port,
		"username": user,
	}
	if sshKeyID != "" {
		data["ssh_key_id"] = sshKeyID
	}

	body, _ := json.Marshal(data)
	resp, err := client.post("/api/v1/jump-servers", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("Jump server '%s' created successfully.\n", name)
	return nil
}

func runJumpServerDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	force, _ := cmd.Flags().GetBool("force")

	if !force {
		fmt.Printf("Are you sure you want to delete jump server '%s'? [y/N]: ", name)
		var confirm string
		if _, err := fmt.Scanln(&confirm); err != nil || (confirm != "y" && confirm != "Y") {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.delete("/api/v1/jump-servers/" + name)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("Jump server '%s' deleted successfully.\n", name)
	return nil
}

func runJumpServerTest(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.post("/api/v1/jump-servers/"+name+"/test", nil)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if result.Success {
		fmt.Printf("✅ Jump server '%s' connection test successful.\n", name)
	} else {
		fmt.Printf("❌ Jump server '%s' connection test failed: %s\n", name, result.Message)
	}
	return nil
}
