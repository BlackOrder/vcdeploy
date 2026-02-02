package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/services/users"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/spf13/cobra"
)

// totpCmd handles TOTP management commands
var totpCmd = &cobra.Command{
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
	rootCmd.AddCommand(totpCmd)

	// totp list
	totpCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List users with TOTP enabled",
		Long:  "Display all users who have TOTP two-factor authentication enabled.",
		RunE:  runTOTPList,
	})

	// totp disable
	disableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable TOTP for a user (requires --confirm)",
		Long: `Disable TOTP two-factor authentication for a user who has lost access.

This is an administrative action that should only be used when:
  - User has lost their TOTP device AND all recovery codes
  - User identity has been verified through out-of-band means

This action is logged for audit purposes.`,
		Example: `  # Disable TOTP for user "john"
  vcdeploy totp disable --user john --reason "Lost phone, verified via video call" --confirm

  # Remote mode (via API)
  vcdeploy totp disable --master localhost:9000 --token <api-token> \
    --user john --reason "Lost phone, verified via video call" --confirm`,
		RunE: runTOTPDisable,
	}
	disableCmd.Flags().String("user", "", "Username or user ID (required)")
	disableCmd.Flags().String("reason", "", "Reason for disabling TOTP (required, for audit)")
	disableCmd.Flags().Bool("confirm", false, "Confirm this destructive action")
	_ = disableCmd.MarkFlagRequired("user")
	_ = disableCmd.MarkFlagRequired("reason")
	totpCmd.AddCommand(disableCmd)

	// totp status
	statusCmd := &cobra.Command{
		Use:   "status [username]",
		Short: "Show TOTP status for a user",
		Long:  "Display TOTP status and remaining recovery codes for a specific user.",
		Args:  cobra.ExactArgs(1),
		RunE:  runTOTPStatus,
	}
	totpCmd.AddCommand(statusCmd)
}

func runTOTPList(cmd *cobra.Command, args []string) error {
	// Check if remote mode
	master, _ := cmd.Flags().GetString("master")
	if master != "" || os.Getenv("VCDEPLOY_MASTER") != "" {
		return runTOTPListRemote(cmd)
	}
	return runTOTPListLocal()
}

func runTOTPListLocal() error {
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
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", u.ID, u.Username, u.Email, u.Role)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d users with TOTP enabled\n", len(totpUsers))
	return nil
}

func runTOTPListRemote(cmd *cobra.Command) error {
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
			ID          int64  `json:"id"`
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
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", u.ID, u.Username, u.Email, u.Role)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d users with TOTP enabled\n", len(result.Users))
	return nil
}

func runTOTPDisable(cmd *cobra.Command, args []string) error {
	user, _ := cmd.Flags().GetString("user")
	reason, _ := cmd.Flags().GetString("reason")
	confirm, _ := cmd.Flags().GetBool("confirm")

	if !confirm {
		return fmt.Errorf("this action requires --confirm flag\n\nThis will disable TOTP for the user, removing 2FA protection.\nThe user will need to re-enable TOTP after logging in.")
	}

	if len(reason) < 10 {
		return fmt.Errorf("reason must be at least 10 characters (for audit purposes)")
	}

	// Check if remote mode
	master, _ := cmd.Flags().GetString("master")
	if master != "" || os.Getenv("VCDEPLOY_MASTER") != "" {
		return runTOTPDisableRemote(cmd, user, reason)
	}
	return runTOTPDisableLocal(user, reason)
}

func runTOTPDisableLocal(username, reason string) error {
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

func runTOTPDisableRemote(cmd *cobra.Command, username, reason string) error {
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

func runTOTPStatus(cmd *cobra.Command, args []string) error {
	username := args[0]

	// Check if remote mode
	master, _ := cmd.Flags().GetString("master")
	if master != "" || os.Getenv("VCDEPLOY_MASTER") != "" {
		return runTOTPStatusRemote(cmd, username)
	}
	return runTOTPStatusLocal(username)
}

func runTOTPStatusLocal(username string) error {
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

	fmt.Printf("User: %s (ID: %d)\n", user.Username, user.ID)
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

func runTOTPStatusRemote(cmd *cobra.Command, username string) error {
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
