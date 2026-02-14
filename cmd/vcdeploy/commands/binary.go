package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// binaryCmd handles binary artifact management commands
var binaryCmd = &cobra.Command{
	Use:   "binary",
	Short: "Binary artifact management",
	Long: `Commands for managing binary artifacts.

Binary artifacts are compiled executables or packages that can be
deployed to target hosts as part of the deployment process.

All commands require API authentication via --master and --token flags.`,
}

func init() {
	rootCmd.AddCommand(binaryCmd)

	// List binaries
	listCmd := &cobra.Command{
		Use:   "list [project]",
		Short: "List binary artifacts",
		Long: `List binary artifacts. Optionally filter by project.

Example:
  vcdeploy binary list --master localhost:9000 --token <token>
  vcdeploy binary list myproject --master localhost:9000 --token <token>`,
		RunE: runBinaryList,
	}
	binaryCmd.AddCommand(listCmd)

	// Upload binary
	uploadCmd := &cobra.Command{
		Use:   "upload <file>",
		Short: "Upload a binary artifact",
		Long: `Upload a binary artifact to the server.

Example:
  vcdeploy binary upload ./myapp --project myproject --version v1.2.3 \
    --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runBinaryUpload,
	}
	uploadCmd.Flags().StringP("project", "p", "", "Project name (required)")
	uploadCmd.Flags().StringP("version", "v", "", "Version tag (required)")
	uploadCmd.Flags().String("platform", "linux-amd64", "Target platform")
	uploadCmd.Flags().String("description", "", "Description of the binary")
	_ = uploadCmd.MarkFlagRequired("project")
	_ = uploadCmd.MarkFlagRequired("version")
	binaryCmd.AddCommand(uploadCmd)

	// Download binary
	downloadCmd := &cobra.Command{
		Use:   "download <project> <version>",
		Short: "Download a binary artifact",
		Long: `Download a binary artifact from the server.

Example:
  vcdeploy binary download myproject v1.2.3 --output ./myapp \
    --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(2),
		RunE: runBinaryDownload,
	}
	downloadCmd.Flags().StringP("output", "o", "", "Output file path")
	downloadCmd.Flags().String("platform", "linux-amd64", "Target platform")
	binaryCmd.AddCommand(downloadCmd)

	// Delete binary
	deleteCmd := &cobra.Command{
		Use:   "delete <project> <version>",
		Short: "Delete a binary artifact",
		Long: `Delete a binary artifact from the server.

Example:
  vcdeploy binary delete myproject v1.2.3 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(2),
		RunE: runBinaryDelete,
	}
	deleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	deleteCmd.Flags().String("platform", "", "Delete only specific platform (deletes all if not specified)")
	binaryCmd.AddCommand(deleteCmd)

	// Show binary info
	showCmd := &cobra.Command{
		Use:   "show <project> <version>",
		Short: "Show binary artifact details",
		Long: `Show detailed information about a binary artifact.

Example:
  vcdeploy binary show myproject v1.2.3 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(2),
		RunE: runBinaryShow,
	}
	binaryCmd.AddCommand(showCmd)
}

// --- Binary Types ---

type binaryListResponse struct {
	Binaries []binaryInfo `json:"binaries"`
}

type binaryInfo struct {
	ID          int64  `json:"id"`
	Project     string `json:"project"`
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	Checksum    string `json:"checksum"`
	Description string `json:"description,omitempty"`
	UploadedBy  string `json:"uploaded_by"`
	UploadedAt  string `json:"uploaded_at"`
}

func runBinaryList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	endpoint := "/api/v1/binaries"
	if len(args) > 0 {
		endpoint = "/api/v1/projects/" + args[0] + "/binaries"
	}

	resp, err := client.get(endpoint)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var result binaryListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.Binaries) == 0 {
		fmt.Println("No binaries found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tVERSION\tPLATFORM\tSIZE\tUPLOADED\tUPLOADED BY")
	for i := range result.Binaries {
		b := &result.Binaries[i]
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			b.Project, b.Version, b.Platform,
			formatSize(b.Size), b.UploadedAt, b.UploadedBy)
	}
	_ = w.Flush() // #nosec G104 - best effort output flush
	return nil
}

func runBinaryUpload(cmd *cobra.Command, args []string) error {
	filePath := args[0]
	project, _ := cmd.Flags().GetString("project")
	version, _ := cmd.Flags().GetString("version")
	platform, _ := cmd.Flags().GetString("platform")
	description, _ := cmd.Flags().GetString("description")

	// Open file
	file, err := os.Open(filePath) // #nosec G304 - user-specified file path from CLI argument
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}

	// Add metadata
	if err := writer.WriteField("project", project); err != nil {
		return fmt.Errorf("write project field: %w", err)
	}
	if err := writer.WriteField("version", version); err != nil {
		return fmt.Errorf("write version field: %w", err)
	}
	if err := writer.WriteField("platform", platform); err != nil {
		return fmt.Errorf("write platform field: %w", err)
	}
	if description != "" {
		if err := writer.WriteField("description", description); err != nil {
			return fmt.Errorf("write description field: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	// Create custom request with multipart content type
	masterURL, _ := cmd.Flags().GetString("master")
	if masterURL == "" {
		return fmt.Errorf("--master flag is required")
	}

	req, err := http.NewRequestWithContext(cmd.Context(), "POST", "http://"+masterURL+"/api/v1/binaries", &buf)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	token, _ := cmd.Flags().GetString("token")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	fmt.Printf("Binary uploaded successfully: %s %s (%s)\n", project, version, platform)

	// Suppress unused variable warning
	_ = client

	return nil
}

func runBinaryDownload(cmd *cobra.Command, args []string) error {
	project := args[0]
	version := args[1]
	output, _ := cmd.Flags().GetString("output")
	platform, _ := cmd.Flags().GetString("platform")

	if output == "" {
		output = fmt.Sprintf("%s-%s-%s", project, version, platform)
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("/api/v1/projects/%s/binaries/%s?platform=%s", project, version, platform)
	resp, err := client.get(endpoint)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	// Write to file
	outFile, err := os.Create(output) // #nosec G304 - user-specified output path from CLI flag
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer outFile.Close()

	n, err := io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Printf("Downloaded %s (%s)\n", output, formatSize(n))
	return nil
}

func runBinaryDelete(cmd *cobra.Command, args []string) error {
	project := args[0]
	version := args[1]
	force, _ := cmd.Flags().GetBool("force")
	platform, _ := cmd.Flags().GetString("platform")

	if !force {
		msg := fmt.Sprintf("Are you sure you want to delete binary '%s %s'", project, version)
		if platform != "" {
			msg += fmt.Sprintf(" (%s)", platform)
		}
		fmt.Printf("%s? [y/N]: ", msg)
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

	endpoint := fmt.Sprintf("/api/v1/projects/%s/binaries/%s", project, version)
	if platform != "" {
		endpoint += "?platform=" + platform
	}

	resp, err := client.delete(endpoint)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("Binary '%s %s' deleted successfully.\n", project, version)
	return nil
}

func runBinaryShow(cmd *cobra.Command, args []string) error {
	project := args[0]
	version := args[1]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("/api/v1/projects/%s/binaries/%s/info", project, version)
	resp, err := client.get(endpoint)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var b binaryInfo
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Project:     %s\n", b.Project)
	fmt.Printf("Version:     %s\n", b.Version)
	fmt.Printf("Platform:    %s\n", b.Platform)
	fmt.Printf("Filename:    %s\n", b.Filename)
	fmt.Printf("Size:        %s\n", formatSize(b.Size))
	fmt.Printf("Checksum:    %s\n", b.Checksum)
	fmt.Printf("Uploaded By: %s\n", b.UploadedBy)
	fmt.Printf("Uploaded At: %s\n", b.UploadedAt)
	if b.Description != "" {
		fmt.Printf("Description: %s\n", b.Description)
	}
	return nil
}

// formatSize formats a byte size into human-readable format
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
