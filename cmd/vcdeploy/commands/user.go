package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/services/users"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// userCmd handles user management commands
var userCmd = &cobra.Command{
	Use:   "user",
	Short: "User management",
	Long:  "Commands for managing users in vcdeploy.",
}

// userTOTPCmd handles TOTP subcommands under user
var userTOTPCmd = &cobra.Command{
	Use:   "totp",
	Short: "Manage TOTP two-factor authentication",
	Long: `Administrative commands for managing user TOTP settings.

This command provides tools to:
  - List users with TOTP enabled
  - Disable TOTP for locked-out users
  - Check TOTP status for a specific user

Use these commands when a user has lost access to their TOTP device
and all recovery codes.`,
}

func init() {
	rootCmd.AddCommand(userCmd)

	// User subcommands
	userCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE:  runUserList,
	})

	createUserCmd := &cobra.Command{
		Use:   "create [username]",
		Short: "Create a new user",
		Args:  cobra.ExactArgs(1),
		RunE:  runUserCreate,
	}
	createUserCmd.Flags().StringP("email", "e", "", "User email address")
	createUserCmd.Flags().StringP("role", "r", "user", "User role (admin, deployer, viewer)")
	createUserCmd.Flags().StringP("password", "p", "", "User password (if not set, will prompt)")
	userCmd.AddCommand(createUserCmd)

	userCmd.AddCommand(&cobra.Command{
		Use:   "delete [username]",
		Short: "Delete a user",
		Args:  cobra.ExactArgs(1),
		RunE:  runUserDelete,
	})

	passwordCmd := &cobra.Command{
		Use:   "password [username]",
		Short: "Change user password",
		Long: `Change the password for a user account.

Example:
  vcdeploy user password admin
  vcdeploy user password admin --password "newpass123"`,
		Args: cobra.ExactArgs(1),
		RunE: runUserPasswd,
	}
	passwordCmd.Flags().StringP("password", "p", "", "New password (if not set, will prompt)")
	userCmd.AddCommand(passwordCmd)

	// Add TOTP subcommand group
	userCmd.AddCommand(userTOTPCmd)

	// user totp list
	userTOTPCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List users with TOTP enabled",
		Long:  "Display all users who have TOTP two-factor authentication enabled.",
		RunE:  runUserTOTPList,
	})

	// user totp show
	userTOTPCmd.AddCommand(&cobra.Command{
		Use:   "show [username]",
		Short: "Show TOTP status for a user",
		Long:  "Display TOTP status and remaining recovery codes for a specific user.",
		Args:  cobra.ExactArgs(1),
		RunE:  runUserTOTPShow,
	})

	// user totp disable
	totpDisableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable TOTP for a user (requires --confirm)",
		Long: `Disable TOTP two-factor authentication for a user who has lost access.

This is an administrative action that should only be used when:
  - User has lost their TOTP device AND all recovery codes
  - User identity has been verified through out-of-band means

This action is logged for audit purposes.`,
		Example: `  # Disable TOTP for user "john"
  vcdeploy user totp disable --user john --reason "Lost phone, verified via video call" --confirm

  # Remote mode (via API)
  vcdeploy user totp disable --master localhost:9000 --token <api-token> \
    --user john --reason "Lost phone, verified via video call" --confirm`,
		RunE: runUserTOTPDisable,
	}
	totpDisableCmd.Flags().String("user", "", "Username or user ID (required)")
	totpDisableCmd.Flags().String("reason", "", "Reason for disabling TOTP (required, for audit)")
	totpDisableCmd.Flags().Bool("confirm", false, "Confirm this destructive action")
	_ = totpDisableCmd.MarkFlagRequired("user")
	_ = totpDisableCmd.MarkFlagRequired("reason")
	userTOTPCmd.AddCommand(totpDisableCmd)

	// Recovery subcommand - emergency lockout recovery
	recoveryCmd := &cobra.Command{
		Use:   "recovery",
		Short: "Emergency lockout recovery",
		Long: `Emergency credential recovery when locked out.

Requires direct server access and master config file.
Used when locked out of the web UI or CLI.

Local mode (lockout recovery):
  vcdeploy user recovery --username admin --email admin@example.com

Remote mode (requires authentication):
  vcdeploy user recovery --master localhost:9000 --token <api-token> --username admin

When --password is not provided, you will be prompted to enter it interactively.`,
		RunE: runAdmin,
	}
	recoveryCmd.Flags().StringP("username", "u", "admin", "Admin username")
	recoveryCmd.Flags().StringP("password", "p", "", "Admin password (if not set, will prompt)")
	recoveryCmd.Flags().StringP("email", "e", "admin@localhost", "Admin email address")
	userCmd.AddCommand(recoveryCmd)
}

// paginatedResponse represents a paginated API response.
type paginatedResponse struct {
	Items      []map[string]interface{} `json:"items"`
	TotalCount int                      `json:"totalCount"`
	Limit      int                      `json:"limit"`
	Offset     int                      `json:"offset"`
}

func runUserList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/users")
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
	fmt.Fprintln(w, "ID\tUSERNAME\tEMAIL\tROLE\tCREATED")
	for _, u := range result.Items {
		fmt.Fprintf(w, "%.0f\t%s\t%s\t%s\t%s\n",
			u["id"], u["username"], u["email"], u["role"], u["createdAt"])
	}
	_ = w.Flush() // #nosec G104 - best effort output flush
	return nil
}

func runUserCreate(cmd *cobra.Command, args []string) error {
	username := args[0]
	email, _ := cmd.Flags().GetString("email")
	role, _ := cmd.Flags().GetString("role")
	password, _ := cmd.Flags().GetString("password")

	// Prompt for password if not provided
	if password == "" {
		fmt.Print("Enter password: ")
		pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		fmt.Println()

		fmt.Print("Confirm password: ")
		pwBytes2, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		fmt.Println()

		if !bytes.Equal(pwBytes, pwBytes2) {
			return fmt.Errorf("passwords do not match")
		}
		password = string(pwBytes)
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data, _ := json.Marshal(map[string]string{
		"username": username,
		"email":    email,
		"password": password,
		"role":     role,
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

	fmt.Printf("User '%s' created successfully.\n", username)
	return nil
}

func runUserDelete(cmd *cobra.Command, args []string) error {
	username := args[0]

	fmt.Printf("Are you sure you want to delete user '%s'? (y/N): ", username)
	var confirm string
	_, _ = fmt.Scanln(&confirm) //nolint:errcheck // user confirmation prompt
	if !strings.EqualFold(confirm, "y") {
		return fmt.Errorf("aborted")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// First get user ID by username
	resp, err := client.get("/api/v1/users")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}

	var result paginatedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		_ = resp.Body.Close() // #nosec G104 - best effort cleanup
		return fmt.Errorf("failed to decode users list: %w", err)
	}
	_ = resp.Body.Close() // #nosec G104 - best effort cleanup

	var userID float64
	for _, u := range result.Items {
		if u["username"] == username {
			id, ok := u["id"].(float64)
			if !ok {
				return fmt.Errorf("invalid API response: user ID is not a number")
			}
			userID = id
			break
		}
	}
	if userID == 0 {
		return fmt.Errorf("user not found: %s", username)
	}

	resp, err = client.delete(fmt.Sprintf("/api/v1/users/%.0f", userID))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("User '%s' deleted successfully.\n", username)
	return nil
}

func runUserPasswd(cmd *cobra.Command, args []string) error {
	username := args[0]

	var password string

	// Check if password was provided via flag
	passwordFlag, _ := cmd.Flags().GetString("password")
	if passwordFlag != "" {
		password = passwordFlag
	} else {
		// Prompt for password interactively
		fmt.Print("Enter new password: ")
		pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		fmt.Println()

		fmt.Print("Confirm new password: ")
		pwBytes2, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		fmt.Println()

		if !bytes.Equal(pwBytes, pwBytes2) {
			return fmt.Errorf("passwords do not match")
		}
		password = string(pwBytes)
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// First get user ID by username
	resp, err := client.get("/api/v1/users")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}

	var result paginatedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		_ = resp.Body.Close() // #nosec G104 - best effort cleanup
		return fmt.Errorf("decode response: %w", err)
	}
	_ = resp.Body.Close() // #nosec G104 - best effort cleanup

	var userID float64
	for _, u := range result.Items {
		if u["username"] == username {
			id, ok := u["id"].(float64)
			if !ok {
				return fmt.Errorf("invalid API response: user ID is not a number")
			}
			userID = id
			break
		}
	}
	if userID == 0 {
		return fmt.Errorf("user not found: %s", username)
	}

	// Update user password via PATCH
	data, _ := json.Marshal(map[string]string{
		"password": password,
	})

	resp, err = client.do("PATCH", fmt.Sprintf("/api/v1/users/%.0f", userID), strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	fmt.Printf("Password for '%s' updated successfully.\n", username)
	return nil
}

// --- TOTP Functions ---

func runUserTOTPList(cmd *cobra.Command, args []string) error {
	// Check if remote mode
	master, _ := cmd.Flags().GetString("master")
	if master != "" || os.Getenv("VCDEPLOY_MASTER") != "" {
		return runUserTOTPListRemote(cmd)
	}
	return runUserTOTPListLocal()
}

func runUserTOTPListLocal() error {
	sysCfg, err := config.GetSystemConfig()
	if err != nil {
		return fmt.Errorf("load system config: %w", err)
	}

	db, err := storage.Open(sysCfg.DatabasePath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	userSvc := users.New(db)
	ctx := context.Background()

	allUsers, err := userSvc.List(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	// Filter to users with TOTP enabled
	var totpUsers []*storage.User
	for _, u := range allUsers {
		if u.TOTPEnabled {
			totpUsers = append(totpUsers, u)
		}
	}

	if len(totpUsers) == 0 {
		fmt.Println("No users have TOTP enabled.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSERNAME\tEMAIL\tROLE")
	for _, u := range totpUsers {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", u.ID, u.Username, u.Email, u.Role)
	}
	_ = w.Flush() // #nosec G104 - best effort output flush

	fmt.Printf("\nTotal: %d users with TOTP enabled\n", len(totpUsers))
	return nil
}

func runUserTOTPListRemote(cmd *cobra.Command) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/admin/totp/users")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result struct {
		Users []struct {
			ID          string `json:"id"`
			Username    string `json:"username"`
			Email       string `json:"email"`
			Role        string `json:"role"`
			TOTPEnabled bool   `json:"totpEnabled"`
		} `json:"users"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.Users) == 0 {
		fmt.Println("No users have TOTP enabled.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSERNAME\tEMAIL\tROLE")
	for _, u := range result.Users {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", u.ID, u.Username, u.Email, u.Role)
	}
	_ = w.Flush() // #nosec G104 - best effort output flush

	fmt.Printf("\nTotal: %d users with TOTP enabled\n", len(result.Users))
	return nil
}

func runUserTOTPShow(cmd *cobra.Command, args []string) error {
	username := args[0]

	// Check if remote mode
	master, _ := cmd.Flags().GetString("master")
	if master != "" || os.Getenv("VCDEPLOY_MASTER") != "" {
		return runUserTOTPShowRemote(cmd, username)
	}
	return runUserTOTPShowLocal(username)
}

func runUserTOTPShowLocal(username string) error {
	sysCfg, err := config.GetSystemConfig()
	if err != nil {
		return fmt.Errorf("load system config: %w", err)
	}

	db, err := storage.Open(sysCfg.DatabasePath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	userSvc := users.New(db)
	ctx := context.Background()

	user, err := userSvc.GetByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	fmt.Printf("User: %s (ID: %s)\n", user.Username, user.ID)
	fmt.Printf("Email: %s\n", user.Email)
	fmt.Printf("Role: %s\n", user.Role)
	fmt.Printf("TOTP Enabled: %v\n", user.TOTPEnabled)

	if user.TOTPEnabled {
		// Count remaining recovery codes
		remaining, err := db.CountUnusedRecoveryCodes(ctx, user.ID)
		if err != nil {
			fmt.Printf("Recovery Codes Remaining: <error: %v>\n", err)
		} else {
			fmt.Printf("Recovery Codes Remaining: %d\n", remaining)
			if remaining <= 2 {
				fmt.Println("\n⚠️  Warning: User is running low on recovery codes!")
			}
		}
	}

	return nil
}

func runUserTOTPShowRemote(cmd *cobra.Command, username string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get(fmt.Sprintf("/api/v1/admin/totp/status/%s", username))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result struct {
		UserID                 int64  `json:"user_id"`
		Username               string `json:"username"`
		Email                  string `json:"email"`
		Role                   string `json:"role"`
		TOTPEnabled            bool   `json:"totp_enabled"`
		RecoveryCodesRemaining int    `json:"recovery_codes_remaining"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("User: %s (ID: %d)\n", result.Username, result.UserID)
	fmt.Printf("Email: %s\n", result.Email)
	fmt.Printf("Role: %s\n", result.Role)
	fmt.Printf("TOTP Enabled: %v\n", result.TOTPEnabled)

	if result.TOTPEnabled {
		fmt.Printf("Recovery Codes Remaining: %d\n", result.RecoveryCodesRemaining)
		if result.RecoveryCodesRemaining <= 2 {
			fmt.Println("\n⚠️  Warning: User is running low on recovery codes!")
		}
	}

	return nil
}

func runUserTOTPDisable(cmd *cobra.Command, args []string) error {
	user, _ := cmd.Flags().GetString("user")
	reason, _ := cmd.Flags().GetString("reason")
	confirm, _ := cmd.Flags().GetBool("confirm")

	if !confirm {
		return fmt.Errorf("this action requires --confirm flag; this will disable TOTP for the user, removing 2FA protection; the user will need to re-enable TOTP after logging in")
	}

	if len(reason) < 10 {
		return fmt.Errorf("reason must be at least 10 characters (for audit purposes)")
	}

	// Check if remote mode
	master, _ := cmd.Flags().GetString("master")
	if master != "" || os.Getenv("VCDEPLOY_MASTER") != "" {
		return runUserTOTPDisableRemote(cmd, user, reason)
	}
	return runUserTOTPDisableLocal(user, reason)
}

func runUserTOTPDisableLocal(username, reason string) error {
	sysCfg, err := config.GetSystemConfig()
	if err != nil {
		return fmt.Errorf("load system config: %w", err)
	}

	db, err := storage.Open(sysCfg.DatabasePath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	userSvc := users.New(db)
	ctx := context.Background()

	// Find user by username
	user, err := userSvc.GetByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	if !user.TOTPEnabled {
		return fmt.Errorf("user %q does not have TOTP enabled", username)
	}

	// Disable TOTP
	if err := userSvc.SetTOTP(ctx, user.ID, "", false); err != nil {
		return fmt.Errorf("disable TOTP: %w", err)
	}

	// Delete recovery codes
	if err := db.DeleteRecoveryCodes(ctx, user.ID); err != nil {
		// Log but don't fail - TOTP is already disabled
		fmt.Fprintf(os.Stderr, "Warning: failed to delete recovery codes: %v\n", err)
	}

	fmt.Printf("TOTP disabled for user %q\n", username)
	fmt.Printf("Reason: %s\n", reason)
	fmt.Println("\nThe user will need to re-enable TOTP on next login.")
	return nil
}

func runUserTOTPDisableRemote(cmd *cobra.Command, username, reason string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data, _ := json.Marshal(map[string]string{
		"username": username,
		"reason":   reason,
	})

	resp, err := client.post("/api/v1/admin/totp/disable", jsonReader(data))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	fmt.Printf("TOTP disabled for user %q\n", username)
	fmt.Printf("Reason: %s\n", reason)
	fmt.Println("\nThe user will need to re-enable TOTP on next login.")
	return nil
}
