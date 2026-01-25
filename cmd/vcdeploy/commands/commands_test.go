package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/spf13/cobra"
)

// setupTestDB creates a test database in a temporary directory.
func setupTestDB(t *testing.T) (*storage.DB, func()) {
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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
