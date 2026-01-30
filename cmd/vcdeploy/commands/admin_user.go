package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// userCmd handles user management commands
var userCmd = &cobra.Command{
	Use:   "user",
	Short: "User management",
	Long:  "Commands for managing users in vcdeploy.",
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

	userCmd.AddCommand(&cobra.Command{
		Use:   "passwd [username]",
		Short: "Change user password",
		Args:  cobra.ExactArgs(1),
		RunE:  runUserPasswd,
	})
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

	var users []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSERNAME\tEMAIL\tROLE\tCREATED")
	for _, u := range users {
		fmt.Fprintf(w, "%.0f\t%s\t%s\t%s\t%s\n",
			u["id"], u["username"], u["email"], u["role"], u["createdAt"])
	}
	w.Flush()
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

	var users []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		resp.Body.Close()
		return fmt.Errorf("failed to decode users list: %w", err)
	}
	resp.Body.Close()

	var userID float64
	for _, u := range users {
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

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// First get user ID by username
	resp, err := client.get("/api/v1/users")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}

	var users []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		resp.Body.Close()
		return fmt.Errorf("decode response: %w", err)
	}
	resp.Body.Close()

	var userID float64
	for _, u := range users {
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
		"password": string(pwBytes),
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
