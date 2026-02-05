package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// sshKeysCmd handles SSH key management commands
var sshKeysCmd = &cobra.Command{
	Use:   "ssh-keys",
	Short: "SSH key management",
	Long: `Commands for managing SSH keys.

SSH keys are used for:
  - Agent provisioning over SSH
  - Git repository authentication
  - Remote server access

Keys are stored encrypted and the private key is never exposed via the API.

All commands require API authentication via --master and --token flags.`,
}

func init() {
	rootCmd.AddCommand(sshKeysCmd)

	// List SSH keys
	listSSHCmd := &cobra.Command{
		Use:   "list",
		Short: "List all SSH keys",
		Long: `List all SSH keys with their types and fingerprints.

Example:
  vcdeploy ssh-keys list --master localhost:9000 --token <token>`,
		RunE: runSSHKeysList,
	}
	sshKeysCmd.AddCommand(listSSHCmd)

	// Generate SSH key
	genSSHCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a new SSH key",
		Long: `Generate a new Ed25519 SSH key pair.

The private key is stored encrypted and never exposed.
The public key can be retrieved using the 'public' command.

Example:
  vcdeploy ssh-keys generate --name deploy-key --comment "Deployment key" \
    --master localhost:9000 --token <token>`,
		RunE: runSSHKeysGenerate,
	}
	genSSHCmd.Flags().StringP("name", "n", "", "Key name (required)")
	genSSHCmd.Flags().StringP("comment", "c", "", "Key comment (appears in public key)")
	_ = genSSHCmd.MarkFlagRequired("name")
	sshKeysCmd.AddCommand(genSSHCmd)

	// Import SSH key
	importSSHCmd := &cobra.Command{
		Use:   "import",
		Short: "Import an existing SSH key",
		Long: `Import an existing SSH private key.

Supports OpenSSH format private keys.

Examples:
  # Import from file
  vcdeploy ssh-keys import --name my-key --file ~/.ssh/id_ed25519 \
    --master localhost:9000 --token <token>

  # Import from stdin
  cat ~/.ssh/id_ed25519 | vcdeploy ssh-keys import --name my-key --stdin \
    --master localhost:9000 --token <token>`,
		RunE: runSSHKeysImport,
	}
	importSSHCmd.Flags().StringP("name", "n", "", "Key name (required)")
	importSSHCmd.Flags().StringP("file", "f", "", "Path to private key file")
	importSSHCmd.Flags().Bool("stdin", false, "Read private key from stdin")
	importSSHCmd.Flags().StringP("passphrase", "p", "", "Passphrase for encrypted key")
	_ = importSSHCmd.MarkFlagRequired("name")
	sshKeysCmd.AddCommand(importSSHCmd)

	// Get public key
	pubSSHCmd := &cobra.Command{
		Use:   "public [id]",
		Short: "Output public key",
		Long: `Output the public key in OpenSSH format.

This can be used to add the key to authorized_keys or Git provider settings.

Example:
  vcdeploy ssh-keys public 123 --master localhost:9000 --token <token> > key.pub`,
		Args: cobra.ExactArgs(1),
		RunE: runSSHKeysPublic,
	}
	sshKeysCmd.AddCommand(pubSSHCmd)

	// Delete SSH key
	delSSHCmd := &cobra.Command{
		Use:   "delete [id]",
		Short: "Delete an SSH key",
		Long: `Delete an SSH key by ID.

Warning: Deleting a key may break deployments or provisioning that depend on it.

Example:
  vcdeploy ssh-keys delete 123 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runSSHKeysDelete,
	}
	delSSHCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	sshKeysCmd.AddCommand(delSSHCmd)
}

// --- SSH Key Types ---

type sshKeyListResponse struct {
	Keys []sshKeyInfo `json:"keys"`
}

type sshKeyInfo struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Fingerprint string    `json:"fingerprint"`
	Comment     string    `json:"comment,omitempty"`
	PublicKey   string    `json:"public_key,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	LastUsedAt  time.Time `json:"last_used_at,omitempty"`
}

// --- SSH Keys List ---

func runSSHKeysList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/ssh-keys")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result sshKeyListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.Keys) == 0 {
		fmt.Println("No SSH keys found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tTYPE\tFINGERPRINT\tCREATED")

	for i := range result.Keys {
		key := &result.Keys[i]
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
			key.ID,
			key.Name,
			key.Type,
			truncate(key.Fingerprint, 20),
			key.CreatedAt.Format("2006-01-02"),
		)
	}
	w.Flush()

	return nil
}

// --- SSH Keys Generate ---

func runSSHKeysGenerate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	comment, _ := cmd.Flags().GetString("comment")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]interface{}{
		"name": name,
	}
	if comment != "" {
		reqBody["comment"] = comment
	}

	body, _ := json.Marshal(reqBody)
	fmt.Println("Generating SSH key...")

	resp, err := client.post("/api/v1/ssh-keys", jsonReader(body))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return handleAPIError(resp)
	}

	var key sshKeyInfo
	if err := json.NewDecoder(resp.Body).Decode(&key); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("SSH key generated successfully.\n")
	fmt.Printf("ID:          %d\n", key.ID)
	fmt.Printf("Name:        %s\n", key.Name)
	fmt.Printf("Type:        %s\n", key.Type)
	fmt.Printf("Fingerprint: %s\n", key.Fingerprint)
	fmt.Println()
	fmt.Println("Public key (copy this to add to servers):")
	fmt.Println(key.PublicKey)

	return nil
}

// --- SSH Keys Import ---

func runSSHKeysImport(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	filePath, _ := cmd.Flags().GetString("file")
	useStdin, _ := cmd.Flags().GetBool("stdin")
	passphrase, _ := cmd.Flags().GetString("passphrase")

	var privateKey string

	if useStdin {
		// Read from stdin
		reader := bufio.NewReader(os.Stdin)
		var sb strings.Builder
		for {
			line, err := reader.ReadString('\n')
			sb.WriteString(line)
			if err != nil {
				break
			}
		}
		privateKey = sb.String()
	} else if filePath != "" {
		// Read from file
		data, err := os.ReadFile(filePath) // #nosec G304 - filePath is CLI flag, user-intended file
		if err != nil {
			return fmt.Errorf("read key file: %w", err)
		}
		privateKey = string(data)
	} else {
		return fmt.Errorf("either --file or --stdin must be specified")
	}

	if strings.TrimSpace(privateKey) == "" {
		return fmt.Errorf("private key is empty")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]interface{}{
		"name":        name,
		"private_key": privateKey,
	}
	if passphrase != "" {
		reqBody["passphrase"] = passphrase
	}

	body, _ := json.Marshal(reqBody)
	fmt.Println("Importing SSH key...")

	resp, err := client.post("/api/v1/ssh-keys/import", jsonReader(body))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return handleAPIError(resp)
	}

	var key sshKeyInfo
	if err := json.NewDecoder(resp.Body).Decode(&key); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("SSH key imported successfully.\n")
	fmt.Printf("ID:          %d\n", key.ID)
	fmt.Printf("Name:        %s\n", key.Name)
	fmt.Printf("Type:        %s\n", key.Type)
	fmt.Printf("Fingerprint: %s\n", key.Fingerprint)

	return nil
}

// --- SSH Keys Public ---

func runSSHKeysPublic(cmd *cobra.Command, args []string) error {
	keyID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/ssh-keys/" + keyID + "/public")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	// Output just the public key for easy piping
	fmt.Print(result.PublicKey)
	if !strings.HasSuffix(result.PublicKey, "\n") {
		fmt.Println()
	}

	return nil
}

// --- SSH Keys Delete ---

func runSSHKeysDelete(cmd *cobra.Command, args []string) error {
	keyID := args[0]
	force, _ := cmd.Flags().GetBool("force")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// Confirm deletion unless forced
	if !force {
		fmt.Printf("Warning: Deleting this key may break deployments that depend on it.\n")
		fmt.Printf("Are you sure you want to delete SSH key %s? [y/N]: ", keyID)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	resp, err := client.delete("/api/v1/ssh-keys/" + keyID)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return handleAPIError(resp)
	}

	fmt.Printf("SSH key %s deleted.\n", keyID)
	return nil
}
