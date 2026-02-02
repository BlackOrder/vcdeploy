package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services/users"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// adminCmd handles administrator account management
var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Administrator account management",
	Long: `Manage the administrator account.

This command can create or update admin credentials either locally (direct database access)
or remotely (via API when --master and --token are provided).

Local mode (lockout recovery):
  vcdeploy admin --username admin --email admin@example.com

Remote mode (requires authentication):
  vcdeploy admin --master localhost:9000 --token <api-token> --username admin

When --password is not provided, you will be prompted to enter it interactively.`,
	RunE: runAdmin,
}

func init() {
	// Admin command with flags
	adminCmd.Flags().StringP("username", "u", "admin", "Admin username")
	adminCmd.Flags().StringP("password", "p", "", "Admin password (if not set, will prompt)")
	adminCmd.Flags().StringP("email", "e", "admin@localhost", "Admin email address")
	rootCmd.AddCommand(adminCmd)
}

// --- API Client Helper ---

type apiClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func newAPIClient(cmd *cobra.Command) (*apiClient, error) {
	master, _ := cmd.Flags().GetString("master")
	token, _ := cmd.Flags().GetString("token")

	// Check environment variables as fallback
	if master == "" {
		master = os.Getenv("VCDEPLOY_MASTER")
	}
	if token == "" {
		token = os.Getenv("VCDEPLOY_TOKEN")
	}

	if master == "" {
		return nil, fmt.Errorf("master address required (--master or VCDEPLOY_MASTER)")
	}
	if token == "" {
		return nil, fmt.Errorf("API token required (--token or VCDEPLOY_TOKEN)")
	}

	// Ensure URL has protocol
	if !strings.HasPrefix(master, "http://") && !strings.HasPrefix(master, "https://") {
		master = "http://" + master
	}

	return &apiClient{
		baseURL: master,
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *apiClient) do(method, path string, body io.Reader) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	return c.client.Do(req)
}

func (c *apiClient) get(path string) (*http.Response, error) {
	return c.do("GET", path, nil)
}

func (c *apiClient) post(path string, body io.Reader) (*http.Response, error) {
	return c.do("POST", path, body)
}

func (c *apiClient) delete(path string) (*http.Response, error) {
	return c.do("DELETE", path, nil)
}

// --- Admin Command ---

func runAdmin(cmd *cobra.Command, args []string) error {
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	email, _ := cmd.Flags().GetString("email")

	// Check if this is remote mode
	master, _ := cmd.Flags().GetString("master")
	token, _ := cmd.Flags().GetString("token")

	// Also check environment variables
	if master == "" {
		master = os.Getenv("VCDEPLOY_MASTER")
	}
	if token == "" {
		token = os.Getenv("VCDEPLOY_TOKEN")
	}

	// Prompt for password if not provided
	if password == "" {
		var err error
		password, err = promptPassword()
		if err != nil {
			return err
		}
	}

	// Validate password complexity
	if err := security.ValidatePassword(password); err != nil {
		return fmt.Errorf("password validation failed: %w", err)
	}

	// Remote mode - use API
	if master != "" && token != "" {
		return runAdminRemote(cmd, username, password, email)
	}

	// Local mode - direct database access
	return runAdminLocal(username, password, email)
}

// promptPassword prompts for password with confirmation.
func promptPassword() (string, error) {
	fmt.Print("Enter password: ")
	pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	fmt.Println()

	fmt.Print("Confirm password: ")
	pwBytes2, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	fmt.Println()

	if !bytes.Equal(pwBytes, pwBytes2) {
		return "", fmt.Errorf("passwords do not match")
	}

	return string(pwBytes), nil
}

// runAdminLocal handles admin setup via direct database access.
func runAdminLocal(username, password, email string) error {
	// Initialize config to get database path
	sysCfg, err := config.GetSystemConfig()
	if err != nil {
		return fmt.Errorf("load system config: %w", err)
	}
	dbPath := sysCfg.DatabasePath()

	// Check if server might be running by trying to connect to the configured port
	if isServerRunning(sysCfg) {
		fmt.Println("Warning: Server appears to be running.")
		fmt.Println("If VCDEPLOY_ADMIN_PASSWORD is set, changes may be overwritten on restart.")
		fmt.Print("Continue? [y/N]: ")
		var confirm string
		_, _ = fmt.Scanln(&confirm) //nolint:errcheck // user confirmation prompt
		if !strings.EqualFold(confirm, "y") {
			return fmt.Errorf("aborted")
		}
	}

	// Open database
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Create user service
	userSvc := users.New(db)
	ctx := context.Background()

	// Try to find existing admin user
	existingUser, err := userSvc.GetByUsername(ctx, username)
	if err == nil && existingUser != nil {
		// Update existing user
		existingUser.Email = email
		if err := userSvc.Update(ctx, existingUser); err != nil {
			return fmt.Errorf("update admin email: %w", err)
		}
		if err := userSvc.UpdatePassword(ctx, existingUser.ID, password); err != nil {
			return fmt.Errorf("update admin password: %w", err)
		}
		fmt.Printf("Admin user '%s' updated successfully.\n", username)
		return nil
	}

	// Create new admin user
	user, err := userSvc.Create(ctx, username, password, email, "admin")
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	fmt.Printf("Admin user '%s' created successfully.\n", user.Username)
	return nil
}

// runAdminRemote handles admin setup via API.
func runAdminRemote(cmd *cobra.Command, username, password, email string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// First, try to list users to find if admin exists
	resp, err := client.get("/api/v1/users")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}

	var result paginatedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		resp.Body.Close()
		return fmt.Errorf("failed to decode users list: %w", err)
	}
	resp.Body.Close()

	// Look for user by username
	var userID float64
	for _, u := range result.Items {
		if u["username"] == username {
			id, ok := u["id"].(float64)
			if ok {
				userID = id
				break
			}
		}
	}

	if userID > 0 {
		// Update existing user
		data, _ := json.Marshal(map[string]string{
			"email":    email,
			"password": password,
		})

		resp, err := client.do("PATCH", fmt.Sprintf("/api/v1/users/%.0f", userID), strings.NewReader(string(data)))
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
		}
		fmt.Printf("Admin user '%s' updated successfully.\n", username)
	} else {
		// Create new user
		data, _ := json.Marshal(map[string]string{
			"username": username,
			"email":    email,
			"password": password,
			"role":     "admin",
		})

		resp, err := client.post("/api/v1/users", strings.NewReader(string(data)))
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
		}
		fmt.Printf("Admin user '%s' created successfully.\n", username)
	}

	return nil
}

// isServerRunning checks if the server appears to be running.
// Uses default ports since MasterConfig may not be loaded yet.
func isServerRunning(_ *config.SystemConfig) bool {
	// Try to connect to default ports (8080 for HTTP, 9090 for gRPC)
	ports := []string{":8080", ":9090"}

	for _, port := range ports {
		conn, err := net.DialTimeout("tcp", "localhost"+port, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
	}
	return false
}
