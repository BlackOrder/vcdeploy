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
	"golang.org/x/term"
)

// credentialCmd handles source credential management commands
var credentialCmd = &cobra.Command{
	Use:     "credential",
	Aliases: []string{"credentials", "creds"},
	Short:   "Source credential management",
	Long: `Commands for managing source credentials.

Source credentials are used for authenticating with Git repositories
when cloning and pulling code during deployments.

Supported credential types:
  - basic:       Username/password (for HTTPS)
  - ssh:         SSH key reference (for SSH URLs)
  - token:       Personal access token (GitHub, GitLab)
  - deploy-key:  Repository deploy key

All commands require API authentication via --master and --token flags.`,
}

func init() {
	rootCmd.AddCommand(credentialCmd)

	// List credentials
	listCredsCmd := &cobra.Command{
		Use:   "list",
		Short: "List all source credentials",
		Long: `List all configured source credentials.

Example:
  vcdeploy credentials list --master localhost:9000 --token <token>`,
		RunE: runCredsList,
	}
	listCredsCmd.Flags().StringP("type", "t", "", "Filter by credential type")
	credentialCmd.AddCommand(listCredsCmd)

	// Create credential
	createCredsCmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"add"},
		Short:   "Create a new source credential",
		Long: `Create a new source credential for Git authentication.

Examples:
  # Create a personal access token for GitHub
  vcdeploy credentials create --name github-token --type token --url-pattern "github.com/*" \
    --master localhost:9000 --token <token>

  # Create basic auth credentials
  vcdeploy credentials create --name gitlab-creds --type basic --url-pattern "gitlab.com/myorg/*" \
    --username myuser --master localhost:9000 --token <token>

  # Create SSH key reference
  vcdeploy credentials create --name ssh-deploy --type ssh --url-pattern "git@github.com:myorg/*" \
    --ssh-key-id 123 --master localhost:9000 --token <token>`,
		RunE: runCredsAdd,
	}
	createCredsCmd.Flags().StringP("name", "n", "", "Credential name (required)")
	createCredsCmd.Flags().StringP("type", "t", "", "Credential type: basic, ssh, token, deploy-key (required)")
	createCredsCmd.Flags().StringP("url-pattern", "u", "", "URL pattern to match (e.g., github.com/*)")
	createCredsCmd.Flags().String("username", "", "Username for basic auth")
	createCredsCmd.Flags().String("password", "", "Password for basic auth (will prompt if not provided)")
	createCredsCmd.Flags().String("ssh-key-id", "", "SSH key ID for ssh type")
	createCredsCmd.Flags().Bool("stdin", false, "Read secret value from stdin")
	_ = createCredsCmd.MarkFlagRequired("name")
	_ = createCredsCmd.MarkFlagRequired("type")
	credentialCmd.AddCommand(createCredsCmd)

	// Delete credential
	deleteCredsCmd := &cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a source credential",
		Long: `Delete a source credential by ID.

Example:
  vcdeploy credentials delete 123 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runCredsDelete,
	}
	deleteCredsCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	credentialCmd.AddCommand(deleteCredsCmd)

	// Test credential
	testCredsCmd := &cobra.Command{
		Use:   "test [id] [repo-url]",
		Short: "Test a credential against a repository URL",
		Long: `Test if a credential can authenticate with a repository.

Example:
  vcdeploy credentials test 123 https://github.com/myorg/myrepo.git \
    --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(2),
		RunE: runCredsTest,
	}
	credentialCmd.AddCommand(testCredsCmd)
}

// --- Credential Types ---

type credentialListResponse struct {
	Credentials []credentialInfo `json:"credentials"`
}

type credentialInfo struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	URLPattern  string    `json:"url_pattern,omitempty"`
	Username    string    `json:"username,omitempty"`
	SSHKeyID    int64     `json:"ssh_key_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastUsedAt  time.Time `json:"last_used_at,omitempty"`
	UsageCount  int       `json:"usage_count"`
	Description string    `json:"description,omitempty"`
}

// --- Credentials List ---

func runCredsList(cmd *cobra.Command, args []string) error {
	typeFilter, _ := cmd.Flags().GetString("type")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	path := "/api/v1/credentials"
	if typeFilter != "" {
		path += "?type=" + typeFilter
	}

	resp, err := client.get(path)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result credentialListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.Credentials) == 0 {
		fmt.Println("No source credentials found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tTYPE\tURL PATTERN\tUSAGE")

	for i := range result.Credentials {
		cred := &result.Credentials[i]
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%d\n",
			cred.ID,
			cred.Name,
			cred.Type,
			truncate(cred.URLPattern, 30),
			cred.UsageCount,
		)
	}
	w.Flush()

	return nil
}

// --- Credentials Add ---

func runCredsAdd(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	credType, _ := cmd.Flags().GetString("type")
	urlPattern, _ := cmd.Flags().GetString("url-pattern")
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	sshKeyID, _ := cmd.Flags().GetString("ssh-key-id")
	useStdin, _ := cmd.Flags().GetBool("stdin")

	// Validate type
	validTypes := map[string]bool{
		"basic":      true,
		"ssh":        true,
		"token":      true,
		"deploy-key": true,
	}
	if !validTypes[credType] {
		return fmt.Errorf("invalid credential type: %s (valid types: basic, ssh, token, deploy-key)", credType)
	}

	// Build request
	reqBody := map[string]interface{}{
		"name": name,
		"type": credType,
	}

	if urlPattern != "" {
		reqBody["url_pattern"] = urlPattern
	}

	// Handle type-specific fields
	switch credType {
	case "basic":
		if username == "" {
			return fmt.Errorf("--username is required for basic auth credentials")
		}
		reqBody["username"] = username

		// Get password
		if password == "" && !useStdin {
			fmt.Print("Password: ")
			pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return fmt.Errorf("read password: %w", err)
			}
			fmt.Println()
			password = string(pwBytes)
		} else if useStdin {
			reader := bufio.NewReader(os.Stdin)
			password, _ = reader.ReadString('\n')
			password = strings.TrimSpace(password)
		}
		reqBody["secret"] = password

	case "token":
		var tokenValue string
		if useStdin {
			reader := bufio.NewReader(os.Stdin)
			tokenValue, _ = reader.ReadString('\n')
			tokenValue = strings.TrimSpace(tokenValue)
		} else if password != "" {
			tokenValue = password
		} else {
			fmt.Print("Token: ")
			tokenBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return fmt.Errorf("read token: %w", err)
			}
			fmt.Println()
			tokenValue = string(tokenBytes)
		}
		reqBody["secret"] = tokenValue

	case "ssh", "deploy-key":
		if sshKeyID == "" {
			return fmt.Errorf("--ssh-key-id is required for %s credentials", credType)
		}
		reqBody["ssh_key_id"] = sshKeyID
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	body, _ := json.Marshal(reqBody)
	resp, err := client.post("/api/v1/credentials", jsonReader(body))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return handleAPIError(resp)
	}

	var created credentialInfo
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Credential '%s' created successfully (ID: %d)\n", created.Name, created.ID)
	return nil
}

// --- Credentials Delete ---

func runCredsDelete(cmd *cobra.Command, args []string) error {
	credID := args[0]
	force, _ := cmd.Flags().GetBool("force")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// Confirm deletion unless forced
	if !force {
		fmt.Printf("Are you sure you want to delete credential %s? [y/N]: ", credID)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	resp, err := client.delete("/api/v1/credentials/" + credID)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return handleAPIError(resp)
	}

	fmt.Printf("Credential %s deleted.\n", credID)
	return nil
}

// --- Credentials Test ---

type testCredentialResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func runCredsTest(cmd *cobra.Command, args []string) error {
	credID := args[0]
	repoURL := args[1]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]string{
		"repo_url": repoURL,
	}
	body, _ := json.Marshal(reqBody)

	fmt.Printf("Testing credential %s against %s...\n", credID, repoURL)

	resp, err := client.post("/api/v1/credentials/"+credID+"/test", jsonReader(body))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	var result testCredentialResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if result.Success {
		fmt.Println("✓ Authentication successful")
		if result.Message != "" {
			fmt.Printf("  %s\n", result.Message)
		}
	} else {
		fmt.Println("✗ Authentication failed")
		if result.Error != "" {
			fmt.Printf("  Error: %s\n", result.Error)
		}
	}

	return nil
}
