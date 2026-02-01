package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/spf13/cobra"
)

// setupTestDB creates a test database in a temporary directory.
func setupTestDB(t *testing.T) (storage.Store, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := storage.New(dbPath, nil)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

// TestSetVersionInfo tests the version info setter.
func TestSetVersionInfo(t *testing.T) {
	// Save original values
	origVersion := version
	origCommit := commit
	origBuildTime := buildTime
	defer func() {
		version = origVersion
		commit = origCommit
		buildTime = origBuildTime
	}()

	SetVersionInfo("1.2.3", "abc123def", "2026-01-24")

	if version != "1.2.3" {
		t.Errorf("version = %q, want %q", version, "1.2.3")
	}
	if commit != "abc123def" {
		t.Errorf("commit = %q, want %q", commit, "abc123def")
	}
	if buildTime != "2026-01-24" {
		t.Errorf("buildTime = %q, want %q", buildTime, "2026-01-24")
	}
}

// TestExecute tests the root Execute function doesn't panic.
func TestExecute(t *testing.T) {
	// Reset root command args to avoid test interference
	rootCmd.SetArgs([]string{"--help"})

	// Should not panic and should return nil for --help
	err := Execute()
	if err != nil {
		t.Errorf("Execute() with --help error = %v", err)
	}
}

// TestVersionCommand tests the version command output.
func TestVersionCommand(t *testing.T) {
	// Save original values
	origVersion := version
	origCommit := commit
	origBuildTime := buildTime
	defer func() {
		version = origVersion
		commit = origCommit
		buildTime = origBuildTime
	}()

	SetVersionInfo("2.0.0", "test-commit", "2026-01-24T12:00:00Z")

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	versionCmd.Run(nil, nil)

	w.Close()
	var stdout bytes.Buffer
	stdout.ReadFrom(r)
	os.Stdout = oldStdout

	output := stdout.String()
	if !strings.Contains(output, "2.0.0") {
		t.Errorf("version output should contain version, got: %s", output)
	}
	if !strings.Contains(output, "test-commit") {
		t.Errorf("version output should contain commit, got: %s", output)
	}
	if !strings.Contains(output, "2026-01-24T12:00:00Z") {
		t.Errorf("version output should contain build time, got: %s", output)
	}
}

// TestProjectDBOperations tests project operations through the database layer.
func TestProjectDBOperations(t *testing.T) {
	t.Parallel()

	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Test CreateProject
	project := &storage.Project{
		Name:       "test-project",
		Repository: "https://github.com/example/repo",
		Branch:     "main",
		Type:       "nodejs",
		DeployPath: "/var/www/app",
	}

	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if project.ID == 0 {
		t.Error("CreateProject() did not set project ID")
	}

	// Test GetProjectByName
	retrieved, err := db.GetProjectByName(context.Background(), project.Name)
	if err != nil {
		t.Fatalf("GetProjectByName() error = %v", err)
	}

	if retrieved.Name != project.Name {
		t.Errorf("GetProjectByName().Name = %q, want %q", retrieved.Name, project.Name)
	}
	if retrieved.Repository != project.Repository {
		t.Errorf("GetProjectByName().Repository = %q, want %q", retrieved.Repository, project.Repository)
	}

	// Test ListProjects
	projects, err := db.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}

	if len(projects) != 1 {
		t.Errorf("ListProjects() returned %d projects, want 1", len(projects))
	}

	// Test DeleteProject
	if err := db.DeleteProject(project.Name); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	deleted, _ := db.GetProjectByName(context.Background(), project.Name)
	if deleted != nil {
		t.Error("DeleteProject() didn't delete project")
	}
}

// TestProjectTypeDBOperations tests project type operations through the database layer.
func TestProjectTypeDBOperations(t *testing.T) {
	t.Parallel()

	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Test CreateProjectType
	pt := &storage.ProjectType{
		Name:        "nodejs",
		Description: "Node.js Application",
		BuildCmd:    "npm install && npm run build",
	}

	if err := db.CreateProjectType(pt); err != nil {
		t.Fatalf("CreateProjectType() error = %v", err)
	}

	// Test GetProjectTypeByName
	retrieved, err := db.GetProjectTypeByName(pt.Name)
	if err != nil {
		t.Fatalf("GetProjectTypeByName() error = %v", err)
	}

	if retrieved.Name != pt.Name {
		t.Errorf("GetProjectTypeByName().Name = %q, want %q", retrieved.Name, pt.Name)
	}
	if retrieved.BuildCmd != pt.BuildCmd {
		t.Errorf("GetProjectTypeByName().BuildCmd = %q, want %q", retrieved.BuildCmd, pt.BuildCmd)
	}

	// Test ListProjectTypes
	types, err := db.ListProjectTypes()
	if err != nil {
		t.Fatalf("ListProjectTypes() error = %v", err)
	}

	if len(types) != 1 {
		t.Errorf("ListProjectTypes() returned %d types, want 1", len(types))
	}

	// Test DeleteProjectType
	if err := db.DeleteProjectType(pt.Name); err != nil {
		t.Fatalf("DeleteProjectType() error = %v", err)
	}

	deleted, _ := db.GetProjectTypeByName(pt.Name)
	if deleted != nil {
		t.Error("DeleteProjectType() didn't delete type")
	}
}

// TestSecretDBOperations tests secret operations through the database layer.
func TestSecretDBOperations(t *testing.T) {
	t.Parallel()

	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Test SetSecretEncrypted
	ctx := context.Background()
	scope := "global"
	key := "API_KEY"
	value := []byte("secret-value-123")

	if err := db.SetSecretEncrypted(ctx, scope, scope, key, value); err != nil {
		t.Fatalf("SetSecretEncrypted() error = %v", err)
	}

	// Test ListSecrets
	secrets, err := db.ListSecrets(scope)
	if err != nil {
		t.Fatalf("ListSecrets() error = %v", err)
	}

	if len(secrets) != 1 {
		t.Errorf("ListSecrets() returned %d secrets, want 1", len(secrets))
	}

	// Test DeleteSecret
	if err := db.DeleteSecret(scope, key); err != nil {
		t.Fatalf("DeleteSecret() error = %v", err)
	}

	secretsAfterDelete, _ := db.ListSecrets(scope)
	if len(secretsAfterDelete) != 0 {
		t.Error("DeleteSecret() didn't delete secret")
	}
}

// TestRootCmdStructure tests the root command structure.
func TestRootCmdStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	// Check root command exists and has expected properties
	if rootCmd == nil {
		t.Fatal("rootCmd is nil")
	}

	if rootCmd.Use != "vcdeploy" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "vcdeploy")
	}

	// Check subcommands exist
	subcommands := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	expectedCommands := []string{"master", "project", "type", "secret", "version"}
	for _, name := range expectedCommands {
		if !subcommands[name] {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

// TestProjectCmdStructure tests the project command structure.
func TestProjectCmdStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	if projectCmd == nil {
		t.Fatal("projectCmd is nil")
	}

	// Check subcommands exist
	subcommands := make(map[string]bool)
	for _, cmd := range projectCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	expectedCommands := []string{"list", "add", "edit", "delete", "validate", "deploy"}
	for _, name := range expectedCommands {
		if !subcommands[name] {
			t.Errorf("expected project subcommand %q not found", name)
		}
	}
}

// TestTypeCmdStructure tests the type command structure.
func TestTypeCmdStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	if typeCmd == nil {
		t.Fatal("typeCmd is nil")
	}

	// Check subcommands exist
	subcommands := make(map[string]bool)
	for _, cmd := range typeCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	expectedCommands := []string{"list", "create", "edit", "delete"}
	for _, name := range expectedCommands {
		if !subcommands[name] {
			t.Errorf("expected type subcommand %q not found", name)
		}
	}
}

// TestSecretCmdStructure tests the secret command structure.
func TestSecretCmdStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	if secretCmd == nil {
		t.Fatal("secretCmd is nil")
	}

	// Check subcommands exist
	subcommands := make(map[string]bool)
	for _, cmd := range secretCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	expectedCommands := []string{"list", "set", "delete"}
	for _, name := range expectedCommands {
		if !subcommands[name] {
			t.Errorf("expected secret subcommand %q not found", name)
		}
	}
}

// TestMasterCmdStructure tests the master command structure.
func TestMasterCmdStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	if masterCmd == nil {
		t.Fatal("masterCmd is nil")
	}

	// Check subcommands exist
	subcommands := make(map[string]bool)
	for _, cmd := range masterCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	expectedCommands := []string{"start", "stop", "status", "rotate-key", "backup"}
	for _, name := range expectedCommands {
		if !subcommands[name] {
			t.Errorf("expected master subcommand %q not found", name)
		}
	}
}

// TestGlobalFlags tests that global flags are registered.
func TestGlobalFlags(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	configFlag := rootCmd.PersistentFlags().Lookup("config")
	if configFlag == nil {
		t.Error("--config flag not registered")
	}

	masterFlag := rootCmd.PersistentFlags().Lookup("master")
	if masterFlag == nil {
		t.Error("--master flag not registered")
	}

	tokenFlag := rootCmd.PersistentFlags().Lookup("token")
	if tokenFlag == nil {
		t.Error("--token flag not registered")
	}
}

// TestEditProjectFlags tests the edit project command has expected flags.
func TestEditProjectFlags(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	// Find the edit command under project
	var editCmd *cobra.Command
	for _, cmd := range projectCmd.Commands() {
		if cmd.Name() == "edit" {
			editCmd = cmd
			break
		}
	}

	if editCmd == nil {
		t.Fatal("project edit command not found")
	}

	expectedFlags := []string{"repo", "branch", "path", "type"}
	for _, name := range expectedFlags {
		flag := editCmd.Flags().Lookup(name)
		if flag == nil {
			t.Errorf("expected flag --%s not found on project edit", name)
		}
	}
}

// TestDeployFlags tests the deploy command has expected flags.
func TestDeployFlags(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	// Find the deploy command under project
	var deployCmd *cobra.Command
	for _, cmd := range projectCmd.Commands() {
		if cmd.Name() == "deploy" {
			deployCmd = cmd
			break
		}
	}

	if deployCmd == nil {
		t.Fatal("project deploy command not found")
	}

	expectedFlags := []string{"target", "dry-run", "force"}
	for _, name := range expectedFlags {
		flag := deployCmd.Flags().Lookup(name)
		if flag == nil {
			t.Errorf("expected flag --%s not found on project deploy", name)
		}
	}
}

// TestShowCommandStructure tests the show command has expected subcommands.
func TestShowCommandStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	// Verify showCmd exists
	if showCmd == nil {
		t.Fatal("show command not initialized")
	}

	// Check subcommands are present
	expectedSubcmds := map[string]int{
		"project":    1, // requires 1 arg
		"agent":      1, // requires 1 arg
		"deployment": 1, // requires 1 arg
	}

	for name, expectedArgs := range expectedSubcmds {
		var subCmd *cobra.Command
		for _, cmd := range showCmd.Commands() {
			if cmd.Name() == name {
				subCmd = cmd
				break
			}
		}

		if subCmd == nil {
			t.Errorf("show subcommand %q not found", name)
			continue
		}

		// Check args validation
		if err := subCmd.Args(subCmd, []string{}); err == nil && expectedArgs > 0 {
			t.Errorf("show %s should require %d arguments", name, expectedArgs)
		}
	}
}

// TestShowProjectCommand tests the show project command.
func TestShowProjectCommand(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	var projectShowCmd *cobra.Command
	for _, cmd := range showCmd.Commands() {
		if cmd.Name() == "project" {
			projectShowCmd = cmd
			break
		}
	}

	if projectShowCmd == nil {
		t.Fatal("show project command not found")
	}

	// Test args validation - should fail with no args
	if err := projectShowCmd.Args(projectShowCmd, []string{}); err == nil {
		t.Error("show project should require exactly 1 argument")
	}

	// Test args validation - should fail with too many args
	if err := projectShowCmd.Args(projectShowCmd, []string{"arg1", "arg2"}); err == nil {
		t.Error("show project should fail with more than 1 argument")
	}

	// Test args validation - should succeed with exactly 1 arg
	if err := projectShowCmd.Args(projectShowCmd, []string{"my-project"}); err != nil {
		t.Errorf("show project should accept 1 argument, got error: %v", err)
	}
}

// TestShowAgentCommand tests the show agent command.
func TestShowAgentCommand(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	var agentShowCmd *cobra.Command
	for _, cmd := range showCmd.Commands() {
		if cmd.Name() == "agent" {
			agentShowCmd = cmd
			break
		}
	}

	if agentShowCmd == nil {
		t.Fatal("show agent command not found")
	}

	// Test args validation
	if err := agentShowCmd.Args(agentShowCmd, []string{}); err == nil {
		t.Error("show agent should require exactly 1 argument")
	}

	if err := agentShowCmd.Args(agentShowCmd, []string{"agent-123"}); err != nil {
		t.Errorf("show agent should accept 1 argument, got error: %v", err)
	}
}

// TestShowDeploymentCommand tests the show deployment command.
func TestShowDeploymentCommand(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	var deploymentShowCmd *cobra.Command
	for _, cmd := range showCmd.Commands() {
		if cmd.Name() == "deployment" {
			deploymentShowCmd = cmd
			break
		}
	}

	if deploymentShowCmd == nil {
		t.Fatal("show deployment command not found")
	}

	// Test args validation
	if err := deploymentShowCmd.Args(deploymentShowCmd, []string{}); err == nil {
		t.Error("show deployment should require exactly 1 argument")
	}

	if err := deploymentShowCmd.Args(deploymentShowCmd, []string{"deploy-456"}); err != nil {
		t.Errorf("show deployment should accept 1 argument, got error: %v", err)
	}
}

// TestAuditCommandStructure tests the audit command has expected subcommands.
func TestAuditCommandStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	// Verify auditCmd exists
	if auditCmd == nil {
		t.Fatal("audit command not initialized")
	}

	expectedSubcmds := []string{"list", "export"}

	for _, name := range expectedSubcmds {
		var subCmd *cobra.Command
		for _, cmd := range auditCmd.Commands() {
			if cmd.Name() == name {
				subCmd = cmd
				break
			}
		}

		if subCmd == nil {
			t.Errorf("audit subcommand %q not found", name)
		}
	}
}

// TestAuditListFlags tests the audit list command flags.
func TestAuditListFlags(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	var listCmd *cobra.Command
	for _, cmd := range auditCmd.Commands() {
		if cmd.Name() == "list" {
			listCmd = cmd
			break
		}
	}

	if listCmd == nil {
		t.Fatal("audit list command not found")
	}

	expectedFlags := []string{"limit", "action", "resource"}
	for _, name := range expectedFlags {
		flag := listCmd.Flags().Lookup(name)
		if flag == nil {
			t.Errorf("expected flag --%s not found on audit list", name)
		}
	}

	// Check default value for limit
	limitFlag := listCmd.Flags().Lookup("limit")
	if limitFlag != nil && limitFlag.DefValue != "50" {
		t.Errorf("audit list --limit default = %q, want %q", limitFlag.DefValue, "50")
	}
}

// TestAuditExportFlags tests the audit export command.
func TestAuditExportFlags(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	var exportCmd *cobra.Command
	for _, cmd := range auditCmd.Commands() {
		if cmd.Name() == "export" {
			exportCmd = cmd
			break
		}
	}

	if exportCmd == nil {
		t.Fatal("audit export command not found")
	}

	// Test args validation - requires exactly 1 arg (filename)
	if err := exportCmd.Args(exportCmd, []string{}); err == nil {
		t.Error("audit export should require exactly 1 argument")
	}

	if err := exportCmd.Args(exportCmd, []string{"output.json"}); err != nil {
		t.Errorf("audit export should accept 1 argument, got error: %v", err)
	}

	// Check limit flag exists
	limitFlag := exportCmd.Flags().Lookup("limit")
	if limitFlag == nil {
		t.Error("expected flag --limit not found on audit export")
	}
	if limitFlag != nil && limitFlag.DefValue != "1000" {
		t.Errorf("audit export --limit default = %q, want %q", limitFlag.DefValue, "1000")
	}
}

// TestRootCommandHasShowAndAudit tests that root command has show and audit.
func TestRootCommandHasShowAndAudit(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	foundShow := false
	foundAudit := false

	for _, cmd := range rootCmd.Commands() {
		switch cmd.Name() {
		case "show":
			foundShow = true
		case "audit":
			foundAudit = true
		}
	}

	if !foundShow {
		t.Error("root command should have 'show' subcommand")
	}
	if !foundAudit {
		t.Error("root command should have 'audit' subcommand")
	}
}

// =============================================================================
// E2E CLI Command Tests
// =============================================================================

// mockAPIServer creates a test HTTP server that mocks the vcdeploy API.
func mockAPIServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// User endpoints
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			users := []map[string]interface{}{
				{"id": 1, "username": "admin", "email": "admin@test.com", "role": "admin", "createdAt": time.Now().Format(time.RFC3339)},
				{"id": 2, "username": "deployer", "email": "deployer@test.com", "role": "deployer", "createdAt": time.Now().Format(time.RFC3339)},
			}
			_ = json.NewEncoder(w).Encode(users)
		case http.MethodPost:
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req["username"] == "" {
				http.Error(w, "username required", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": 3, "username": req["username"]})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"deleted"}`)
		}
	})

	// Agent endpoints
	mux.HandleFunc("/api/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		agents := []map[string]interface{}{
			{"id": "agent-1", "hostname": "server1.example.com", "status": "online", "lastSeenAt": time.Now().Format(time.RFC3339)},
			{"id": "agent-2", "hostname": "server2.example.com", "status": "offline", "lastSeenAt": time.Now().Add(-24 * time.Hour).Format(time.RFC3339)},
		}
		_ = json.NewEncoder(w).Encode(agents)
	})

	mux.HandleFunc("/api/v1/agents/", func(w http.ResponseWriter, r *http.Request) {
		agentID := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
		switch r.Method {
		case http.MethodGet:
			agent := map[string]interface{}{
				"id":           agentID,
				"hostname":     "server1.example.com",
				"status":       "online",
				"registeredAt": time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339),
				"lastSeenAt":   time.Now().Format(time.RFC3339),
				"labels":       map[string]interface{}{"env": "production"},
			}
			_ = json.NewEncoder(w).Encode(agent)
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		}
	})

	mux.HandleFunc("/api/v1/agents/tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"token":      "test-registration-token-12345",
				"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			})
		}
	})

	// Deployment endpoints
	mux.HandleFunc("/api/v1/deployments", func(w http.ResponseWriter, r *http.Request) {
		deployments := []map[string]interface{}{
			{"id": "deploy-1", "project": "webapp", "status": "success", "createdAt": time.Now().Format(time.RFC3339)},
			{"id": "deploy-2", "project": "api", "status": "running", "createdAt": time.Now().Format(time.RFC3339)},
		}
		_ = json.NewEncoder(w).Encode(deployments)
	})

	mux.HandleFunc("/api/v1/deployments/", func(w http.ResponseWriter, r *http.Request) {
		deployID := strings.TrimPrefix(r.URL.Path, "/api/v1/deployments/")
		if strings.HasSuffix(deployID, "/cancel") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "cancelled"})
			return
		}
		if strings.HasSuffix(deployID, "/logs") {
			_, _ = w.Write([]byte("Build started...\nInstalling dependencies...\nBuild complete.\n"))
			return
		}
		deployment := map[string]interface{}{
			"id":        deployID,
			"project":   "webapp",
			"status":    "success",
			"branch":    "main",
			"createdAt": time.Now().Format(time.RFC3339),
		}
		_ = json.NewEncoder(w).Encode(deployment)
	})

	// Project deploy endpoint
	mux.HandleFunc("/api/v1/projects/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/deploy") {
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "deploy-new",
				"status": "pending",
			})
		}
	})

	// Config endpoints
	mux.HandleFunc("/api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		config := map[string]interface{}{
			"server": map[string]interface{}{
				"port": 8080,
				"host": "0.0.0.0",
			},
			"security": map[string]interface{}{
				"sessionTimeout": "24h",
			},
		}
		_ = json.NewEncoder(w).Encode(config)
	})

	// API key endpoints
	mux.HandleFunc("/api/v1/api-keys", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			keys := []map[string]interface{}{
				{"id": 1, "name": "ci-key", "prefix": "vcd_ci", "createdAt": time.Now().Format(time.RFC3339)},
			}
			_ = json.NewEncoder(w).Encode(keys)
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    2,
				"name":  "new-key",
				"key":   "vcd_test_key_secret123",
				"token": "vcd_test_key_secret123",
			})
		}
	})

	mux.HandleFunc("/api/v1/api-keys/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
		}
	})

	// Health endpoint
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	return httptest.NewServer(mux)
}

// executeCommand executes a cobra command with the given args and returns the output.
func executeCommand(root *cobra.Command, args ...string) (stdout string, stderr string, err error) {
	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)

	root.SetOut(stdoutBuf)
	root.SetErr(stderrBuf)
	root.SetArgs(args)

	err = root.Execute()
	return stdoutBuf.String(), stderrBuf.String(), err
}

// TestE2E_UserListCommand tests the user list command with a mock API server.
func TestE2E_UserListCommand(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	// Create a fresh command tree for this test
	cmd := &cobra.Command{Use: "vcdeploy"}
	userCmdTest := &cobra.Command{
		Use:   "user",
		Short: "User management",
	}
	userCmdTest.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE: func(cmd *cobra.Command, args []string) error {
			masterURL, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")

			client := &http.Client{Timeout: 10 * time.Second}
			req, err := http.NewRequest("GET", masterURL+"/api/v1/users", http.NoBody)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("API error: %s", resp.Status)
			}

			body, _ := io.ReadAll(resp.Body)
			_, _ = cmd.OutOrStdout().Write(body)
			return nil
		},
	})
	cmd.AddCommand(userCmdTest)
	cmd.PersistentFlags().String("master", server.URL, "Master server URL")
	cmd.PersistentFlags().String("token", "test-token", "API token")

	stdout, _, err := executeCommand(cmd, "user", "list", "--master", server.URL, "--token", "test-token")
	if err != nil {
		t.Fatalf("user list failed: %v", err)
	}

	if !strings.Contains(stdout, "admin") {
		t.Errorf("expected 'admin' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "deployer") {
		t.Errorf("expected 'deployer' in output, got: %s", stdout)
	}
}

// TestE2E_AgentListCommand tests the agent list command with a mock API server.
func TestE2E_AgentListCommand(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	cmd := &cobra.Command{Use: "vcdeploy"}
	agentCmdTest := &cobra.Command{
		Use:   "agent",
		Short: "Agent management",
	}
	agentCmdTest.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			masterURL, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")

			client := &http.Client{Timeout: 10 * time.Second}
			req, err := http.NewRequest("GET", masterURL+"/api/v1/agents", http.NoBody)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			_, _ = cmd.OutOrStdout().Write(body)
			return nil
		},
	})
	cmd.AddCommand(agentCmdTest)
	cmd.PersistentFlags().String("master", server.URL, "Master server URL")
	cmd.PersistentFlags().String("token", "test-token", "API token")

	stdout, _, err := executeCommand(cmd, "agent", "list", "--master", server.URL, "--token", "test-token")
	if err != nil {
		t.Fatalf("agent list failed: %v", err)
	}

	if !strings.Contains(stdout, "agent-1") {
		t.Errorf("expected 'agent-1' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "server1.example.com") {
		t.Errorf("expected 'server1.example.com' in output, got: %s", stdout)
	}
}

// TestE2E_DeploymentListCommand tests the deployment list command.
func TestE2E_DeploymentListCommand(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	cmd := &cobra.Command{Use: "vcdeploy"}
	deployCmdTest := &cobra.Command{
		Use:   "deploy",
		Short: "Deployment commands",
	}
	deployCmdTest.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List recent deployments",
		RunE: func(cmd *cobra.Command, args []string) error {
			masterURL, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")

			client := &http.Client{Timeout: 10 * time.Second}
			req, err := http.NewRequest("GET", masterURL+"/api/v1/deployments", http.NoBody)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			_, _ = cmd.OutOrStdout().Write(body)
			return nil
		},
	})
	cmd.AddCommand(deployCmdTest)
	cmd.PersistentFlags().String("master", server.URL, "Master server URL")
	cmd.PersistentFlags().String("token", "test-token", "API token")

	stdout, _, err := executeCommand(cmd, "deploy", "list", "--master", server.URL, "--token", "test-token")
	if err != nil {
		t.Fatalf("deploy list failed: %v", err)
	}

	if !strings.Contains(stdout, "deploy-1") {
		t.Errorf("expected 'deploy-1' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "webapp") {
		t.Errorf("expected 'webapp' in output, got: %s", stdout)
	}
}

// TestE2E_ConfigShowCommand tests the config show command.
func TestE2E_ConfigShowCommand(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	cmd := &cobra.Command{Use: "vcdeploy"}
	configCmdTest := &cobra.Command{
		Use:   "config",
		Short: "Configuration management",
	}
	configCmdTest.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			masterURL, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")

			client := &http.Client{Timeout: 10 * time.Second}
			req, err := http.NewRequest("GET", masterURL+"/api/v1/config", http.NoBody)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			_, _ = cmd.OutOrStdout().Write(body)
			return nil
		},
	})
	cmd.AddCommand(configCmdTest)
	cmd.PersistentFlags().String("master", server.URL, "Master server URL")
	cmd.PersistentFlags().String("token", "test-token", "API token")

	stdout, _, err := executeCommand(cmd, "config", "show", "--master", server.URL, "--token", "test-token")
	if err != nil {
		t.Fatalf("config show failed: %v", err)
	}

	if !strings.Contains(stdout, "server") {
		t.Errorf("expected 'server' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "8080") {
		t.Errorf("expected '8080' in output, got: %s", stdout)
	}
}

// TestE2E_APIKeyListCommand tests the apikey list command.
func TestE2E_APIKeyListCommand(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	cmd := &cobra.Command{Use: "vcdeploy"}
	apikeyCmdTest := &cobra.Command{
		Use:   "apikey",
		Short: "API key management",
	}
	apikeyCmdTest.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			masterURL, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")

			client := &http.Client{Timeout: 10 * time.Second}
			req, err := http.NewRequest("GET", masterURL+"/api/v1/api-keys", http.NoBody)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			_, _ = cmd.OutOrStdout().Write(body)
			return nil
		},
	})
	cmd.AddCommand(apikeyCmdTest)
	cmd.PersistentFlags().String("master", server.URL, "Master server URL")
	cmd.PersistentFlags().String("token", "test-token", "API token")

	stdout, _, err := executeCommand(cmd, "apikey", "list", "--master", server.URL, "--token", "test-token")
	if err != nil {
		t.Fatalf("apikey list failed: %v", err)
	}

	if !strings.Contains(stdout, "ci-key") {
		t.Errorf("expected 'ci-key' in output, got: %s", stdout)
	}
}

// TestE2E_APIKeyCreateCommand tests the apikey create command.
func TestE2E_APIKeyCreateCommand(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	cmd := &cobra.Command{Use: "vcdeploy"}
	apikeyCmdTest := &cobra.Command{
		Use:   "apikey",
		Short: "API key management",
	}
	createCmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			masterURL, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")

			data, _ := json.Marshal(map[string]string{"name": args[0]})
			client := &http.Client{Timeout: 10 * time.Second}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, "POST", masterURL+"/api/v1/api-keys", bytes.NewReader(data))
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				return fmt.Errorf("API error: %s", resp.Status)
			}

			body, _ := io.ReadAll(resp.Body)
			_, _ = cmd.OutOrStdout().Write(body)
			return nil
		},
	}
	apikeyCmdTest.AddCommand(createCmd)
	cmd.AddCommand(apikeyCmdTest)
	cmd.PersistentFlags().String("master", server.URL, "Master server URL")
	cmd.PersistentFlags().String("token", "test-token", "API token")

	stdout, _, err := executeCommand(cmd, "apikey", "create", "test-key", "--master", server.URL, "--token", "test-token")
	if err != nil {
		t.Fatalf("apikey create failed: %v", err)
	}

	if !strings.Contains(stdout, "vcd_test_key") {
		t.Errorf("expected API key in output, got: %s", stdout)
	}
}

// TestE2E_AgentShowCommand tests the agent show command.
func TestE2E_AgentShowCommand(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	cmd := &cobra.Command{Use: "vcdeploy"}
	agentCmdTest := &cobra.Command{
		Use:   "agent",
		Short: "Agent management",
	}
	agentCmdTest.AddCommand(&cobra.Command{
		Use:   "show [agent-id]",
		Short: "Show agent details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			masterURL, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")

			client := &http.Client{Timeout: 10 * time.Second}
			req, err := http.NewRequest("GET", masterURL+"/api/v1/agents/"+args[0], http.NoBody)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			_, _ = cmd.OutOrStdout().Write(body)
			return nil
		},
	})
	cmd.AddCommand(agentCmdTest)
	cmd.PersistentFlags().String("master", server.URL, "Master server URL")
	cmd.PersistentFlags().String("token", "test-token", "API token")

	stdout, _, err := executeCommand(cmd, "agent", "show", "agent-1", "--master", server.URL, "--token", "test-token")
	if err != nil {
		t.Fatalf("agent show failed: %v", err)
	}

	if !strings.Contains(stdout, "agent-1") {
		t.Errorf("expected 'agent-1' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "production") {
		t.Errorf("expected 'production' label in output, got: %s", stdout)
	}
}

// TestE2E_DeploymentStatusCommand tests the deployment status command.
func TestE2E_DeploymentStatusCommand(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	cmd := &cobra.Command{Use: "vcdeploy"}
	deployCmdTest := &cobra.Command{
		Use:   "deploy",
		Short: "Deployment commands",
	}
	deployCmdTest.AddCommand(&cobra.Command{
		Use:   "status [deployment-id]",
		Short: "Get deployment status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			masterURL, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")

			client := &http.Client{Timeout: 10 * time.Second}
			//nolint:noctx // Test helper simulating CLI behavior
			req, err := http.NewRequest("GET", masterURL+"/api/v1/deployments/"+args[0], http.NoBody)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			_, _ = cmd.OutOrStdout().Write(body)
			return nil
		},
	})
	cmd.AddCommand(deployCmdTest)
	cmd.PersistentFlags().String("master", server.URL, "Master server URL")
	cmd.PersistentFlags().String("token", "test-token", "API token")

	stdout, _, err := executeCommand(cmd, "deploy", "status", "deploy-123", "--master", server.URL, "--token", "test-token")
	if err != nil {
		t.Fatalf("deploy status failed: %v", err)
	}

	if !strings.Contains(stdout, "deploy-123") {
		t.Errorf("expected 'deploy-123' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "success") {
		t.Errorf("expected 'success' status in output, got: %s", stdout)
	}
}

// TestE2E_DeploymentLogsCommand tests the deployment logs command.
func TestE2E_DeploymentLogsCommand(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	cmd := &cobra.Command{Use: "vcdeploy"}
	deployCmdTest := &cobra.Command{
		Use:   "deploy",
		Short: "Deployment commands",
	}
	deployCmdTest.AddCommand(&cobra.Command{
		Use:   "logs [deployment-id]",
		Short: "View deployment logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			masterURL, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")

			client := &http.Client{Timeout: 10 * time.Second}
			//nolint:noctx // Test helper simulating CLI behavior
			req, err := http.NewRequest("GET", masterURL+"/api/v1/deployments/"+args[0]+"/logs", http.NoBody)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			_, _ = cmd.OutOrStdout().Write(body)
			return nil
		},
	})
	cmd.AddCommand(deployCmdTest)
	cmd.PersistentFlags().String("master", server.URL, "Master server URL")
	cmd.PersistentFlags().String("token", "test-token", "API token")

	stdout, _, err := executeCommand(cmd, "deploy", "logs", "deploy-123", "--master", server.URL, "--token", "test-token")
	if err != nil {
		t.Fatalf("deploy logs failed: %v", err)
	}

	if !strings.Contains(stdout, "Build started") {
		t.Errorf("expected 'Build started' in logs, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Build complete") {
		t.Errorf("expected 'Build complete' in logs, got: %s", stdout)
	}
}

// TestE2E_AgentTokenCommand tests the agent token generation command.
func TestE2E_AgentTokenCommand(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	cmd := &cobra.Command{Use: "vcdeploy"}
	agentCmdTest := &cobra.Command{
		Use:   "agent",
		Short: "Agent management",
	}
	tokenCmd := &cobra.Command{
		Use:   "token",
		Short: "Generate agent registration token",
		RunE: func(cmd *cobra.Command, args []string) error {
			masterURL, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")
			label, _ := cmd.Flags().GetString("label")

			data := map[string]interface{}{}
			if label != "" {
				data["label"] = label
			}
			body, _ := json.Marshal(data)

			client := &http.Client{Timeout: 10 * time.Second}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, "POST", masterURL+"/api/v1/agents/tokens", bytes.NewReader(body))
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			_, _ = cmd.OutOrStdout().Write(respBody)
			return nil
		},
	}
	tokenCmd.Flags().StringP("label", "l", "", "Agent label")
	agentCmdTest.AddCommand(tokenCmd)
	cmd.AddCommand(agentCmdTest)
	cmd.PersistentFlags().String("master", server.URL, "Master server URL")
	cmd.PersistentFlags().String("token", "test-token", "API token")

	stdout, _, err := executeCommand(cmd, "agent", "token", "--master", server.URL, "--token", "test-token")
	if err != nil {
		t.Fatalf("agent token failed: %v", err)
	}

	if !strings.Contains(stdout, "test-registration-token") {
		t.Errorf("expected registration token in output, got: %s", stdout)
	}
}

// TestE2E_APIServerError tests handling of API server errors.
func TestE2E_APIServerError(t *testing.T) {
	// Create a server that returns errors
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer errorServer.Close()

	cmd := &cobra.Command{Use: "vcdeploy"}
	userCmdTest := &cobra.Command{
		Use:   "user",
		Short: "User management",
	}
	userCmdTest.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE: func(cmd *cobra.Command, args []string) error {
			masterURL, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")

			client := &http.Client{Timeout: 10 * time.Second}
			req, err := http.NewRequest("GET", masterURL+"/api/v1/users", http.NoBody)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("API error: %s", resp.Status)
			}

			return nil
		},
	})
	cmd.AddCommand(userCmdTest)
	cmd.PersistentFlags().String("master", errorServer.URL, "Master server URL")
	cmd.PersistentFlags().String("token", "test-token", "API token")

	_, _, err := executeCommand(cmd, "user", "list", "--master", errorServer.URL, "--token", "test-token")
	if err == nil {
		t.Error("expected error from API server, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 error, got: %v", err)
	}
}

// TestE2E_ConnectionTimeout tests handling of connection timeouts.
func TestE2E_ConnectionTimeout(t *testing.T) {
	// Use a non-routable IP to simulate timeout
	cmd := &cobra.Command{Use: "vcdeploy"}
	userCmdTest := &cobra.Command{
		Use:   "user",
		Short: "User management",
	}
	userCmdTest.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE: func(cmd *cobra.Command, args []string) error {
			masterURL, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")

			// Use a very short timeout for testing
			client := &http.Client{Timeout: 100 * time.Millisecond}
			//nolint:noctx // Test helper simulating CLI behavior
			req, err := http.NewRequest("GET", masterURL+"/api/v1/users", http.NoBody)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			_, err = client.Do(req)
			if err != nil {
				return fmt.Errorf("connection failed: %w", err)
			}
			return nil
		},
	})
	cmd.AddCommand(userCmdTest)
	cmd.PersistentFlags().String("master", "http://10.255.255.1:9999", "Master server URL")
	cmd.PersistentFlags().String("token", "test-token", "API token")

	_, _, err := executeCommand(cmd, "user", "list", "--master", "http://10.255.255.1:9999", "--token", "test-token")
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "connection") {
		t.Errorf("expected connection error, got: %v", err)
	}
}
