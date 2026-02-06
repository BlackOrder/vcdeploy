// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/spf13/cobra"
)

// loginCmd handles user authentication
var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to vcdeploy master",
	Long: `Log in to a vcdeploy master server and store credentials locally.

The API token is stored in ~/.config/vcdeploy/config.yaml and used for
subsequent commands.`,
	Example: `  # Interactive login (prompts for password)
  vcdeploy login --master vcdeploy.example.com:9000 --username admin

  # Non-interactive login with password
  vcdeploy login -m vcdeploy.example.com:9000 -u admin -p "$PASSWORD"

  # Using an existing API token
  vcdeploy login -m vcdeploy.example.com:9000 --token <api-token>`,
	RunE: runLogin,
}

// logoutCmd clears stored credentials
var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and clear stored credentials",
	Long: `Clear stored API token and master server URL.

This removes authentication credentials from the local configuration file.`,
	Example: `  vcdeploy logout`,
	RunE:    runLogout,
}

// whoamiCmd displays current user information
var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Display current user information",
	Long: `Display information about the currently logged in user.

Shows username, role, and authentication status.`,
	Example: `  vcdeploy whoami`,
	RunE:    runWhoami,
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(whoamiCmd)

	// Login flags
	loginCmd.Flags().StringP("username", "u", "", "Username for authentication")
	loginCmd.Flags().StringP("password", "p", "", "Password (will prompt if not provided)")
}

func runLogin(cmd *cobra.Command, args []string) error {
	master, _ := cmd.Flags().GetString("master")
	token, _ := cmd.Flags().GetString("token")
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")

	// If token is provided, just save it
	if token != "" {
		if master == "" {
			return fmt.Errorf("--master is required")
		}
		return saveCredentials(master, token)
	}

	// Otherwise authenticate with username/password
	if master == "" {
		return fmt.Errorf("--master is required")
	}
	if username == "" {
		return fmt.Errorf("--username is required")
	}

	// Prompt for password if not provided
	if password == "" {
		fmt.Print("Password: ")
		passwordBytes, err := term.ReadPassword(syscall.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println() // newline after password
		password = string(passwordBytes)
	}

	// Authenticate
	resp, err := authenticateUser(master, username, password)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Save credentials
	if err := saveCredentials(master, resp.Token); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	fmt.Printf("✓ Logged in as %s\n", username)
	return nil
}

func runLogout(cmd *cobra.Command, args []string) error {
	if err := clearCredentials(); err != nil {
		return fmt.Errorf("failed to clear credentials: %w", err)
	}
	fmt.Println("✓ Logged out successfully")
	return nil
}

func runWhoami(cmd *cobra.Command, args []string) error {
	// Try to use the standard API client method
	client, err := newAPIClient(cmd)
	if err != nil {
		// Fall back to saved credentials
		master, token, loadErr := loadCredentials()
		if loadErr != nil || token == "" {
			fmt.Println("Not logged in. Use 'vcdeploy login' to authenticate.")
			return nil
		}
		// Create client manually
		client = &apiClient{
			baseURL: "http://" + master,
			token:   token,
			client:  &http.Client{Timeout: 30 * time.Second},
		}
	}

	// Get current user info
	resp, err := client.get("/api/v1/users/me")
	if err != nil {
		return fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var user struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
		Email    string `json:"email,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("Role:     %s\n", user.Role)
	if user.Email != "" {
		fmt.Printf("Email:    %s\n", user.Email)
	}
	if client.baseURL != "" {
		fmt.Printf("Master:   %s\n", client.baseURL)
	}

	return nil
}

// authResponse represents the login response
type authResponse struct {
	Token string `json:"token"`
}

func authenticateUser(master, username, password string) (*authResponse, error) {
	url := fmt.Sprintf("http://%s/api/v1/auth/login", master)
	body := fmt.Sprintf(`{"username": %q, "password": %q}`, username, password)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	//nolint:gosec // G107: URL is user-specified master server
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	var result authResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func saveCredentials(master, token string) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	// Read existing config
	config := make(map[string]string)
	if data, err := os.ReadFile(configPath); err == nil {
		// Parse simple key=value format
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
				config[parts[0]] = parts[1]
			}
		}
	}

	// Update credentials
	config["master"] = master
	config["token"] = token

	// Write config
	var lines []string
	for k, v := range config {
		lines = append(lines, fmt.Sprintf("%s=%s", k, v))
	}

	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}

	return os.WriteFile(configPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func clearCredentials() error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	// Just remove the file
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func loadCredentials() (master, token string, err error) {
	configPath, err := getConfigPath()
	if err != nil {
		return "", "", err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", "", err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			switch parts[0] {
			case "master":
				master = parts[1]
			case "token":
				token = parts[1]
			}
		}
	}

	return master, token, nil
}

func getConfigPath() (string, error) {
	return configDir() + "/credentials", nil
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return home + "/.config/vcdeploy"
}
