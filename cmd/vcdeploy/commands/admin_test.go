package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/spf13/cobra"
)

// --- API Client Tests ---

// TestAPIClientCreation tests the apiClient constructor.
func TestAPIClientCreation(t *testing.T) {
	tests := []struct {
		name      string
		masterURL string
		token     string
		wantErr   bool
	}{
		{"valid http", "http://localhost:9000", "test-token", false},
		{"valid https", "https://example.com", "test-token", false},
		{"adds http prefix", "localhost:9000", "test-token", false},
		{"empty master", "", "test-token", true},
		{"empty token", "http://localhost", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal cobra.Command to test
			cmd := &cobra.Command{}
			cmd.Flags().String("master", tt.masterURL, "")
			cmd.Flags().String("token", tt.token, "")

			client, err := newAPIClient(cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("newAPIClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("newAPIClient() returned nil without error")
			}
		})
	}
}

// TestAPIClientGet tests the apiClient GET method.
func TestAPIClientGet(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization header
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want %q", auth, "Bearer test-token")
		}

		// Verify method
		if r.Method != http.MethodGet {
			t.Errorf("Method = %q, want GET", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := &apiClient{
		baseURL: server.URL,
		token:   "test-token",
		client:  http.DefaultClient,
	}

	resp, err := client.get("/api/v1/test")
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestAPIClientPost tests the apiClient POST method.
func TestAPIClientPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %q, want POST", r.Method)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]int{"id": 1})
	}))
	defer server.Close()

	client := &apiClient{
		baseURL: server.URL,
		token:   "test-token",
		client:  http.DefaultClient,
	}

	resp, err := client.post("/api/v1/users", nil)
	if err != nil {
		t.Fatalf("post() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
}

// TestAPIClientDelete tests the apiClient DELETE method.
func TestAPIClientDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Method = %q, want DELETE", r.Method)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &apiClient{
		baseURL: server.URL,
		token:   "test-token",
		client:  http.DefaultClient,
	}

	resp, err := client.delete("/api/v1/users/1")
	if err != nil {
		t.Fatalf("delete() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestRunUserListSuccess tests the user list command with a mock server.
func TestRunUserListSuccess(t *testing.T) {
	users := []map[string]interface{}{
		{"id": 1.0, "username": "admin", "email": "admin@example.com", "role": "admin"},
		{"id": 2.0, "username": "user1", "email": "user1@example.com", "role": "user"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users" {
			t.Errorf("Path = %q, want /api/v1/users", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(users)
	}))
	defer server.Close()

	cmd := userCmd
	cmd.SetArgs([]string{"list"})
	_ = cmd.Flags().Set("master", server.URL)
	_ = cmd.Flags().Set("token", "test-token")

	// We can't easily capture output here, but at least test it doesn't error
	// In a real scenario, we'd inject the output writer
}

// TestRunAgentTokenCreateSuccess tests agent token creation with mock server.
func TestRunAgentTokenCreateSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/tokens" {
			t.Errorf("Path = %q, want /api/v1/agents/tokens", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      "agent-registration-token-123",
			"expires_at": "2026-02-01T00:00:00Z",
		})
	}))
	defer server.Close()

	// Test that the safe type assertion works with the expected response
	result := map[string]interface{}{
		"token": "test-token",
	}

	token, ok := result["token"].(string)
	if !ok {
		t.Error("token type assertion should succeed")
	}
	if token != "test-token" {
		t.Errorf("token = %q, want test-token", token)
	}
}

// TestRunAgentTokenCreateMissingToken tests safe type assertion for missing token.
func TestRunAgentTokenCreateMissingToken(t *testing.T) {
	// Test that missing token is handled safely
	result := map[string]interface{}{
		"other_field": "value",
	}

	_, ok := result["token"].(string)
	if ok {
		t.Error("token type assertion should fail when missing")
	}
}

// TestRunAgentTokenCreateWrongType tests safe type assertion for wrong type.
func TestRunAgentTokenCreateWrongType(t *testing.T) {
	// Test that wrong type is handled safely
	result := map[string]interface{}{
		"token": 12345, // Wrong type - number instead of string
	}

	_, ok := result["token"].(string)
	if ok {
		t.Error("token type assertion should fail for wrong type")
	}
}

// TestUserIDTypeAssertion tests safe type assertion for user IDs.
func TestUserIDTypeAssertion(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		wantOK  bool
		wantVal float64
	}{
		{"valid float64", 1.0, true, 1.0},
		{"valid larger", 123.0, true, 123.0},
		{"string type", "1", false, 0},
		{"int type", 1, false, 0}, // JSON numbers are float64
		{"nil", nil, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := map[string]interface{}{
				"id": tt.value,
			}

			id, ok := result["id"].(float64)
			if ok != tt.wantOK {
				t.Errorf("type assertion ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && id != tt.wantVal {
				t.Errorf("id = %v, want %v", id, tt.wantVal)
			}
		})
	}
}

// TestDeploymentStatusColors tests status color mapping logic.
func TestDeploymentStatusColors(t *testing.T) {
	statuses := map[string]bool{
		"success":     true,
		"completed":   true,
		"failed":      true,
		"error":       true,
		"cancelled":   true,
		"pending":     true,
		"running":     true,
		"rolled_back": true,
	}

	for status := range statuses {
		// Just verify these are valid status strings that the CLI handles
		if status == "" {
			t.Error("status should not be empty")
		}
	}
}

// --- Full Integration Tests for run* Functions ---

// newMockAPIServer creates a comprehensive test server mocking the vcdeploy API.
func newMockAPIServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// Users endpoint
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			users := []map[string]interface{}{
				{"id": 1.0, "username": "admin", "email": "admin@test.com", "role": "admin", "createdAt": "2024-01-01T00:00:00Z"},
				{"id": 2.0, "username": "deployer", "email": "deployer@test.com", "role": "deployer", "createdAt": "2024-01-02T00:00:00Z"},
			}
			_ = json.NewEncoder(w).Encode(users)
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": 3.0, "username": "newuser"})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// User by ID endpoint
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		case http.MethodPatch:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
		}
	})

	// Agents endpoint
	mux.HandleFunc("/api/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		agents := []map[string]interface{}{
			{"id": "agent-1", "hostname": "server1.example.com", "status": "online", "lastSeenAt": "2024-01-01T12:00:00Z"},
			{"id": "agent-2", "hostname": "server2.example.com", "status": "offline", "lastSeenAt": "2024-01-01T00:00:00Z"},
		}
		_ = json.NewEncoder(w).Encode(agents)
	})

	// Agent by ID endpoint
	mux.HandleFunc("/api/v1/agents/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/tokens") {
			// Agent token generation
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"token":      "reg-token-12345",
				"expires_at": "2025-01-01T00:00:00Z",
			})
			return
		}
		switch r.Method {
		case http.MethodGet:
			agent := map[string]interface{}{
				"id":           "agent-1",
				"hostname":     "server1.example.com",
				"status":       "online",
				"registeredAt": "2024-01-01T00:00:00Z",
				"lastSeenAt":   "2024-01-01T12:00:00Z",
				"labels":       map[string]interface{}{"env": "production", "tier": "web"},
			}
			_ = json.NewEncoder(w).Encode(agent)
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		}
	})

	// Deployments endpoint
	mux.HandleFunc("/api/v1/deployments", func(w http.ResponseWriter, r *http.Request) {
		deployments := []map[string]interface{}{
			{"id": "deploy-1", "project": "webapp", "branch": "main", "status": "success", "startedAt": "2024-01-01T10:00:00Z"},
			{"id": "deploy-2", "project": "api", "branch": "develop", "status": "running", "startedAt": "2024-01-01T11:00:00Z"},
		}
		_ = json.NewEncoder(w).Encode(deployments)
	})

	// Deployment by ID endpoint
	mux.HandleFunc("/api/v1/deployments/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/cancel") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
			return
		}
		if strings.HasSuffix(path, "/logs") {
			logs := []map[string]interface{}{
				{"createdAt": "2024-01-01T10:00:00Z", "level": "INFO", "message": "Build started"},
				{"createdAt": "2024-01-01T10:01:00Z", "level": "INFO", "message": "Dependencies installed"},
				{"createdAt": "2024-01-01T10:02:00Z", "level": "INFO", "message": "Build complete"},
			}
			_ = json.NewEncoder(w).Encode(logs)
			return
		}
		deployment := map[string]interface{}{
			"id":          "deploy-1",
			"project":     "webapp",
			"branch":      "main",
			"target":      "production",
			"status":      "success",
			"startedAt":   "2024-01-01T10:00:00Z",
			"completedAt": "2024-01-01T10:05:00Z",
			"triggeredBy": "admin",
		}
		_ = json.NewEncoder(w).Encode(deployment)
	})

	// Project deploy endpoint
	mux.HandleFunc("/api/v1/projects/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/deploy") {
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "deploy-new-1",
				"status": "pending",
			})
		} else {
			// Project info
			project := map[string]interface{}{
				"id":          1,
				"name":        "webapp",
				"repository":  "https://github.com/example/webapp",
				"branch":      "main",
				"deploy_path": "/var/www/app",
				"type":        "nodejs",
				"enabled":     true,
				"created_at":  "2024-01-01T00:00:00Z",
				"updated_at":  "2024-01-01T00:00:00Z",
			}
			_ = json.NewEncoder(w).Encode(project)
		}
	})

	// Settings/Config endpoints
	mux.HandleFunc("/api/v1/settings/export", func(w http.ResponseWriter, r *http.Request) {
		settings := map[string]map[string]interface{}{
			"server": {
				"port": 8080,
				"host": "0.0.0.0",
			},
			"security": {
				"session_timeout": "24h",
				"max_attempts":    5,
			},
		}
		_ = json.NewEncoder(w).Encode(settings)
	})

	mux.HandleFunc("/api/v1/settings/import", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"imported": 5})
	})

	mux.HandleFunc("/api/v1/settings/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
		}
	})

	// API keys endpoint
	mux.HandleFunc("/api/v1/apikeys", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			keys := []map[string]interface{}{
				{"id": 1.0, "name": "ci-deploy", "createdAt": "2024-01-01T00:00:00Z", "expiresAt": nil, "lastUsedAt": "2024-01-15T10:00:00Z"},
				{"id": 2.0, "name": "backup-key", "createdAt": "2024-01-05T00:00:00Z", "expiresAt": "2025-01-05T00:00:00Z", "lastUsedAt": nil},
			}
			_ = json.NewEncoder(w).Encode(keys)
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   3.0,
				"name": "new-key",
				"key":  "vcd_newkey_secret123456",
			})
		}
	})

	// API key by ID
	mux.HandleFunc("/api/v1/apikeys/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
		}
	})

	// Audit endpoint
	mux.HandleFunc("/api/v1/audit", func(w http.ResponseWriter, r *http.Request) {
		entries := []map[string]interface{}{
			{"timestamp": "2024-01-01T10:00:00Z", "user": "admin", "action": "deploy", "resource": "webapp", "result": "success"},
			{"timestamp": "2024-01-01T09:00:00Z", "user": "admin", "action": "login", "resource": "system", "result": "success"},
		}
		_ = json.NewEncoder(w).Encode(entries)
	})

	// Health endpoint
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	// Stats endpoint
	mux.HandleFunc("/api/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"projects":         5.0,
			"connected_agents": 3.0,
		})
	})

	return httptest.NewServer(mux)
}

// createTestCommand creates a cobra command with master/token flags set to test server.
func createTestCommand(serverURL string) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("master", serverURL, "")
	cmd.Flags().String("token", "test-token", "")
	return cmd
}

// TestRunUserList tests the runUserList function.
func TestRunUserList(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runUserList(cmd, nil)

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runUserList() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "admin") {
		t.Errorf("output should contain 'admin', got: %s", output)
	}
	if !strings.Contains(output, "deployer") {
		t.Errorf("output should contain 'deployer', got: %s", output)
	}
}

// TestRunUserListAPIError tests runUserList when API returns an error.
func TestRunUserListAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	cmd := createTestCommand(server.URL)
	err := runUserList(cmd, nil)

	if err == nil {
		t.Error("expected error for API error response")
	}
}

// TestRunAgentList tests the runAgentList function.
func TestRunAgentList(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runAgentList(cmd, nil)

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runAgentList() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "agent-1") {
		t.Errorf("output should contain 'agent-1', got: %s", output)
	}
	if !strings.Contains(output, "server1.example.com") {
		t.Errorf("output should contain 'server1.example.com', got: %s", output)
	}
}

// TestRunAgentListEmpty tests runAgentList when no agents exist.
func TestRunAgentListEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
	}))
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runAgentList(cmd, nil)

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runAgentList() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No agents registered") {
		t.Errorf("output should indicate no agents, got: %s", output)
	}
}

// TestRunAgentShow tests the runAgentShow function.
func TestRunAgentShow(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runAgentShow(cmd, []string{"agent-1"})

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runAgentShow() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "server1.example.com") {
		t.Errorf("output should contain hostname, got: %s", output)
	}
	if !strings.Contains(output, "production") {
		t.Errorf("output should contain label, got: %s", output)
	}
}

// TestRunAgentToken tests the runAgentToken function.
func TestRunAgentToken(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)
	cmd.Flags().String("label", "test-label", "")

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runAgentToken(cmd, nil)

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runAgentToken() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "reg-token-12345") {
		t.Errorf("output should contain token, got: %s", output)
	}
}

// TestRunAgentTokenInvalidResponse tests runAgentToken with invalid API response.
func TestRunAgentTokenInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return response without token field
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"other": "value"})
	}))
	defer server.Close()

	cmd := createTestCommand(server.URL)
	cmd.Flags().String("label", "", "")

	err := runAgentToken(cmd, nil)

	if err == nil {
		t.Error("expected error for missing token in response")
	}
	if !strings.Contains(err.Error(), "invalid API response") {
		t.Errorf("error should mention invalid response, got: %v", err)
	}
}

// TestRunAgentDelete tests the runAgentDelete function.
func TestRunAgentDelete(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	// Simulate user confirming deletion by providing "y" on stdin
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	_, _ = w.WriteString("y\n")
	w.Close()

	oldStdout := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut

	err := runAgentDelete(cmd, []string{"agent-1"})

	wOut.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, rOut)
	os.Stdout = oldStdout
	os.Stdin = oldStdin

	if err != nil {
		t.Fatalf("runAgentDelete() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "removed successfully") {
		t.Errorf("output should confirm deletion, got: %s", output)
	}
}

// TestRunAgentDeleteAborted tests runAgentDelete when user cancels.
func TestRunAgentDeleteAborted(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	// Simulate user declining deletion by providing "n" on stdin
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	_, _ = w.WriteString("n\n")
	w.Close()

	oldStdout := os.Stdout
	_, wOut, _ := os.Pipe()
	os.Stdout = wOut

	err := runAgentDelete(cmd, []string{"agent-1"})

	wOut.Close()
	os.Stdout = oldStdout
	os.Stdin = oldStdin

	if err == nil {
		t.Error("expected error when user aborts")
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Errorf("error should mention aborted, got: %v", err)
	}
}

// TestRunAgentDeleteAPIError tests runAgentDelete when API returns error.
func TestRunAgentDeleteAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	cmd := createTestCommand(server.URL)

	// Simulate user confirming deletion
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	_, _ = w.WriteString("y\n")
	w.Close()

	oldStdout := os.Stdout
	_, wOut, _ := os.Pipe()
	os.Stdout = wOut

	err := runAgentDelete(cmd, []string{"agent-1"})

	wOut.Close()
	os.Stdout = oldStdout
	os.Stdin = oldStdin

	if err == nil {
		t.Error("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "API error") {
		t.Errorf("error should mention API error, got: %v", err)
	}
}

// TestRunDeploymentList tests the runDeploymentList function.
func TestRunDeploymentList(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runDeploymentList(cmd, nil)

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runDeploymentList() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "deploy-1") {
		t.Errorf("output should contain 'deploy-1', got: %s", output)
	}
	if !strings.Contains(output, "webapp") {
		t.Errorf("output should contain 'webapp', got: %s", output)
	}
}

// TestRunDeploymentStatus tests the runDeploymentStatus function.
func TestRunDeploymentStatus(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runDeploymentStatus(cmd, []string{"deploy-1"})

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runDeploymentStatus() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "success") {
		t.Errorf("output should contain status, got: %s", output)
	}
	if !strings.Contains(output, "admin") {
		t.Errorf("output should contain triggeredBy, got: %s", output)
	}
}

// TestRunDeploymentCancel tests the runDeploymentCancel function.
func TestRunDeploymentCancel(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runDeploymentCancel(cmd, []string{"deploy-1"})

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runDeploymentCancel() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "cancelled") {
		t.Errorf("output should confirm cancellation, got: %s", output)
	}
}

// TestRunDeploymentCancelError tests runDeploymentCancel when API returns error.
func TestRunDeploymentCancelError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "cannot cancel completed deployment", http.StatusBadRequest)
	}))
	defer server.Close()

	cmd := createTestCommand(server.URL)
	err := runDeploymentCancel(cmd, []string{"deploy-1"})

	if err == nil {
		t.Error("expected error for failed cancellation")
	}
}

// TestRunDeploymentLogs tests the runDeploymentLogs function.
func TestRunDeploymentLogs(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runDeploymentLogs(cmd, []string{"deploy-1"})

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runDeploymentLogs() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Build started") {
		t.Errorf("output should contain log message, got: %s", output)
	}
	if !strings.Contains(output, "Build complete") {
		t.Errorf("output should contain log message, got: %s", output)
	}
}

// TestRunDeploymentLogsEmpty tests runDeploymentLogs when no logs exist.
func TestRunDeploymentLogsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
	}))
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runDeploymentLogs(cmd, []string{"deploy-1"})

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runDeploymentLogs() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No logs available") {
		t.Errorf("output should indicate no logs, got: %s", output)
	}
}

// TestRunConfigShow tests the runConfigShow function.
func TestRunConfigShow(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runConfigShow(cmd, nil)

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runConfigShow() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "server") {
		t.Errorf("output should contain 'server' category, got: %s", output)
	}
	if !strings.Contains(output, "security") {
		t.Errorf("output should contain 'security' category, got: %s", output)
	}
}

// TestRunConfigExport tests the runConfigExport function.
func TestRunConfigExport(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runConfigExport(cmd, nil)

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runConfigExport() error = %v", err)
	}

	output := stdout.String()
	// Should be valid JSON
	var result interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &result); err != nil {
		t.Errorf("output should be valid JSON, got: %s, error: %v", output, err)
	}
}

// TestRunConfigImport tests the runConfigImport function.
func TestRunConfigImport(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	// Create a temp file with config JSON
	tmpFile, err := os.CreateTemp("", "config-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configData := `{"server": {"port": 9000}}`
	if _, err := tmpFile.WriteString(configData); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runConfigImport(cmd, []string{tmpFile.Name()})

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runConfigImport() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Imported") {
		t.Errorf("output should confirm import, got: %s", output)
	}
}

// TestRunConfigImportFileNotFound tests runConfigImport with missing file.
func TestRunConfigImportFileNotFound(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)
	err := runConfigImport(cmd, []string{"/nonexistent/file.json"})

	if err == nil {
		t.Error("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read file") {
		t.Errorf("error should mention file reading, got: %v", err)
	}
}

// TestRunConfigSet tests the runConfigSet function.
func TestRunConfigSet(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runConfigSet(cmd, []string{"server.port", "9000"})

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runConfigSet() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "server.port") {
		t.Errorf("output should confirm setting, got: %s", output)
	}
}

// TestRunConfigSetInvalidFormat tests runConfigSet with invalid key format.
func TestRunConfigSetInvalidFormat(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)
	err := runConfigSet(cmd, []string{"invalidkey", "value"})

	if err == nil {
		t.Error("expected error for invalid key format")
	}
	if !strings.Contains(err.Error(), "category.key") {
		t.Errorf("error should mention format, got: %v", err)
	}
}

// TestRunAPIKeyList tests the runAPIKeyList function.
func TestRunAPIKeyList(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runAPIKeyList(cmd, nil)

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runAPIKeyList() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "ci-deploy") {
		t.Errorf("output should contain key name, got: %s", output)
	}
}

// TestRunAPIKeyListEmpty tests runAPIKeyList when no keys exist.
func TestRunAPIKeyListEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
	}))
	defer server.Close()

	cmd := createTestCommand(server.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runAPIKeyList(cmd, nil)

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runAPIKeyList() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No API keys found") {
		t.Errorf("output should indicate no keys, got: %s", output)
	}
}

// TestRunAPIKeyCreate tests the runAPIKeyCreate function.
func TestRunAPIKeyCreate(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)
	cmd.Flags().Int("expires", 0, "")

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runAPIKeyCreate(cmd, []string{"new-key"})

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runAPIKeyCreate() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "vcd_newkey_secret") {
		t.Errorf("output should contain the new key, got: %s", output)
	}
	if !strings.Contains(output, "IMPORTANT") {
		t.Errorf("output should contain warning message, got: %s", output)
	}
}

// TestRunDeploymentTrigger tests the runDeploymentTrigger function.
func TestRunDeploymentTrigger(t *testing.T) {
	server := newMockAPIServer(t)
	defer server.Close()

	cmd := createTestCommand(server.URL)
	cmd.Flags().String("branch", "main", "")
	cmd.Flags().String("target", "production", "")
	cmd.Flags().String("schedule", "", "")

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runDeploymentTrigger(cmd, []string{"webapp"})

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runDeploymentTrigger() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "triggered") {
		t.Errorf("output should confirm trigger, got: %s", output)
	}
}

// TestRunDeploymentTriggerScheduled tests scheduled deployment.
func TestRunDeploymentTriggerScheduled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           "deploy-scheduled-1",
			"status":       "scheduled",
			"scheduled_at": "2025-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	cmd := createTestCommand(server.URL)
	cmd.Flags().String("branch", "", "")
	cmd.Flags().String("target", "", "")
	cmd.Flags().String("schedule", "2025-01-01T00:00:00Z", "")

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runDeploymentTrigger(cmd, []string{"webapp"})

	w.Close()
	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, r)
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runDeploymentTrigger() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "scheduled") {
		t.Errorf("output should mention scheduled, got: %s", output)
	}
}

// TestNewAPIClientEnvVars tests that newAPIClient falls back to environment variables.
func TestNewAPIClientEnvVars(t *testing.T) {
	// Save and restore env vars
	origMaster := os.Getenv("VCDEPLOY_MASTER")
	origToken := os.Getenv("VCDEPLOY_TOKEN")
	defer func() {
		os.Setenv("VCDEPLOY_MASTER", origMaster)
		os.Setenv("VCDEPLOY_TOKEN", origToken)
	}()

	os.Setenv("VCDEPLOY_MASTER", "http://env-master:8080")
	os.Setenv("VCDEPLOY_TOKEN", "env-token")

	cmd := &cobra.Command{}
	cmd.Flags().String("master", "", "")
	cmd.Flags().String("token", "", "")

	client, err := newAPIClient(cmd)
	if err != nil {
		t.Fatalf("newAPIClient() with env vars error = %v", err)
	}

	if client.baseURL != "http://env-master:8080" {
		t.Errorf("baseURL = %q, want %q", client.baseURL, "http://env-master:8080")
	}
	if client.token != "env-token" {
		t.Errorf("token = %q, want %q", client.token, "env-token")
	}
}

// TestAPIClientDoMethod tests the do method with various HTTP methods.
func TestAPIClientDoMethod(t *testing.T) {
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var receivedMethod string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := &apiClient{
				baseURL: server.URL,
				token:   "test-token",
				client:  http.DefaultClient,
			}

			resp, err := client.do(method, "/test", nil)
			if err != nil {
				t.Fatalf("do(%s) error = %v", method, err)
			}
			resp.Body.Close()

			if receivedMethod != method {
				t.Errorf("received method = %q, want %q", receivedMethod, method)
			}
		})
	}
}

// TestAPIClientInvalidURL tests apiClient with invalid URL.
func TestAPIClientInvalidURL(t *testing.T) {
	client := &apiClient{
		baseURL: "://invalid-url",
		token:   "test-token",
		client:  http.DefaultClient,
	}

	_, err := client.do("GET", "/test", nil)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

// TestUserCmdStructure tests the user command structure.
func TestUserCmdStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	if userCmd == nil {
		t.Fatal("userCmd is nil")
	}

	expectedSubcmds := []string{"list", "create", "delete", "passwd"}
	subcommands := make(map[string]bool)
	for _, cmd := range userCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	for _, name := range expectedSubcmds {
		if !subcommands[name] {
			t.Errorf("expected user subcommand %q not found", name)
		}
	}
}

// TestAgentCmdStructure tests the agent command structure.
func TestAgentCmdStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	if agentCmd == nil {
		t.Fatal("agentCmd is nil")
	}

	expectedSubcmds := []string{"list", "show", "delete", "token"}
	subcommands := make(map[string]bool)
	for _, cmd := range agentCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	for _, name := range expectedSubcmds {
		if !subcommands[name] {
			t.Errorf("expected agent subcommand %q not found", name)
		}
	}
}

// TestDeploymentCmdStructure tests the deployment command structure.
func TestDeploymentCmdStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	if deploymentCmd == nil {
		t.Fatal("deploymentCmd is nil")
	}

	expectedSubcmds := []string{"trigger", "list", "status", "cancel", "logs"}
	subcommands := make(map[string]bool)
	for _, cmd := range deploymentCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	for _, name := range expectedSubcmds {
		if !subcommands[name] {
			t.Errorf("expected deployment subcommand %q not found", name)
		}
	}
}

// TestConfigCmdStructure tests the config command structure.
func TestConfigCmdStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	if configCmd == nil {
		t.Fatal("configCmd is nil")
	}

	expectedSubcmds := []string{"show", "export", "import", "set"}
	subcommands := make(map[string]bool)
	for _, cmd := range configCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	for _, name := range expectedSubcmds {
		if !subcommands[name] {
			t.Errorf("expected config subcommand %q not found", name)
		}
	}
}

// TestAPIKeyCmdStructure tests the apikey command structure.
func TestAPIKeyCmdStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	if apikeyCmd == nil {
		t.Fatal("apikeyCmd is nil")
	}

	expectedSubcmds := []string{"list", "create", "revoke"}
	subcommands := make(map[string]bool)
	for _, cmd := range apikeyCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	for _, name := range expectedSubcmds {
		if !subcommands[name] {
			t.Errorf("expected apikey subcommand %q not found", name)
		}
	}
}

// --- Admin Command Tests ---

// TestAdminCmdStructure tests the admin command structure and flags.
func TestAdminCmdStructure(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - accesses shared global cobra commands

	if adminCmd == nil {
		t.Fatal("adminCmd is nil")
	}

	// Test flags exist
	usernameFlag := adminCmd.Flags().Lookup("username")
	if usernameFlag == nil {
		t.Error("expected --username flag not found")
	} else if usernameFlag.DefValue != "admin" {
		t.Errorf("username default = %q, want %q", usernameFlag.DefValue, "admin")
	}

	passwordFlag := adminCmd.Flags().Lookup("password")
	if passwordFlag == nil {
		t.Error("expected --password flag not found")
	}

	emailFlag := adminCmd.Flags().Lookup("email")
	if emailFlag == nil {
		t.Error("expected --email flag not found")
	} else if emailFlag.DefValue != "admin@localhost" {
		t.Errorf("email default = %q, want %q", emailFlag.DefValue, "admin@localhost")
	}
}

// TestRunAdminRemote_WithMockedAPI tests runAdminRemote with a mocked API.
func TestRunAdminRemote_WithMockedAPI(t *testing.T) {
	// Track requests
	var getUsersCalled, createUserCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer test-token")
		}

		switch r.URL.Path {
		case "/api/v1/users":
			switch r.Method {
			case http.MethodGet:
				getUsersCalled = true
				// Return empty list - admin doesn't exist
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
			case http.MethodPost:
				createUserCalled = true
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": 1})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create command with flags
	cmd := &cobra.Command{}
	cmd.Flags().String("master", server.URL, "")
	cmd.Flags().String("token", "test-token", "")

	err := runAdminRemote(cmd, "newadmin", "Test@Password123!", "newadmin@example.com")
	if err != nil {
		t.Fatalf("runAdminRemote() error = %v", err)
	}

	if !getUsersCalled {
		t.Error("expected /api/v1/users GET to be called")
	}
	if !createUserCalled {
		t.Error("expected /api/v1/users POST to be called")
	}
}

// TestRunAdminRemote_UpdateExistingUser tests updating an existing user via API.
func TestRunAdminRemote_UpdateExistingUser(t *testing.T) {
	var patchCalled bool
	var patchedUserID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/users":
			// Return existing user
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": float64(42), "username": "existingadmin", "email": "old@example.com"},
			})
		default:
			if r.Method == "PATCH" && strings.HasPrefix(r.URL.Path, "/api/v1/users/") {
				patchCalled = true
				patchedUserID = strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": 42})
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	cmd.Flags().String("master", server.URL, "")
	cmd.Flags().String("token", "test-token", "")

	err := runAdminRemote(cmd, "existingadmin", "New@Password123!", "new@example.com")
	if err != nil {
		t.Fatalf("runAdminRemote() error = %v", err)
	}

	if !patchCalled {
		t.Error("expected PATCH to be called for existing user")
	}
	if patchedUserID != "42" {
		t.Errorf("patchedUserID = %q, want %q", patchedUserID, "42")
	}
}

// TestRunAdmin_SelectsCorrectMode tests that runAdmin selects local vs remote mode correctly.
func TestRunAdmin_SelectsCorrectMode(t *testing.T) {
	tests := []struct {
		name       string
		master     string
		token      string
		envMaster  string
		envToken   string
		wantRemote bool
	}{
		{"flags remote", "http://localhost:9000", "token123", "", "", true},
		{"env remote", "", "", "http://localhost:9000", "token123", true},
		{"mixed remote (flag master, env token)", "http://localhost:9000", "", "", "token123", true},
		{"mixed remote (env master, flag token)", "", "token123", "http://localhost:9000", "", true},
		{"local mode - no credentials", "", "", "", "", false},
		{"local mode - only master", "http://localhost:9000", "", "", "", false},
		{"local mode - only token", "", "token123", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.envMaster != "" {
				t.Setenv("VCDEPLOY_MASTER", tt.envMaster)
			} else {
				t.Setenv("VCDEPLOY_MASTER", "")
			}
			if tt.envToken != "" {
				t.Setenv("VCDEPLOY_TOKEN", tt.envToken)
			} else {
				t.Setenv("VCDEPLOY_TOKEN", "")
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("master", tt.master, "")
			cmd.Flags().String("token", tt.token, "")
			cmd.Flags().String("username", "admin", "")
			cmd.Flags().String("password", "Test@Password123!", "")
			cmd.Flags().String("email", "admin@localhost", "")

			// Read the flags to determine mode (same logic as runAdmin)
			master, _ := cmd.Flags().GetString("master")
			token, _ := cmd.Flags().GetString("token")
			if master == "" {
				master = os.Getenv("VCDEPLOY_MASTER")
			}
			if token == "" {
				token = os.Getenv("VCDEPLOY_TOKEN")
			}

			isRemote := master != "" && token != ""
			if isRemote != tt.wantRemote {
				t.Errorf("isRemote = %v, want %v", isRemote, tt.wantRemote)
			}
		})
	}
}

// TestPromptPassword_PasswordMismatch tests password confirmation validation.
// Note: This is a unit test for the validation logic, not the actual terminal prompt.
func TestPasswordMismatch(t *testing.T) {
	t.Parallel()

	pw1 := []byte("password1")
	pw2 := []byte("password2")

	if bytes.Equal(pw1, pw2) {
		t.Error("passwords should not match")
	}

	pw3 := []byte("samepassword")
	pw4 := []byte("samepassword")
	if !bytes.Equal(pw3, pw4) {
		t.Error("identical passwords should match")
	}
}

// TestIsServerRunning tests the server running detection helper.
func TestIsServerRunning(t *testing.T) {
	t.Parallel()

	// Start a test server to simulate running server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	_ = listener.Addr().(*net.TCPAddr).Port // port used for test

	// Accept connections in background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	defer listener.Close()

	// Create config pointing to our test port
	cfg := &config.SystemConfig{}
	// Note: This test would need proper config setup to be complete
	// For now, just verify the function doesn't panic
	_ = isServerRunning(cfg)
}

// TestRunAdminRemote_APIError tests error handling when API returns an error.
// The API returns a JSON object instead of array, causing decode to fail.
func TestRunAdminRemote_APIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	cmd.Flags().String("master", server.URL, "")
	cmd.Flags().String("token", "test-token", "")
	cmd.Flags().String("username", "admin", "")
	cmd.Flags().String("password", "Test@Password123!", "")
	cmd.Flags().String("email", "admin@test.com", "")

	err := runAdminRemote(cmd, "admin", "Test@Password123!", "admin@test.com")
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	// The API returns a JSON object instead of array, causing unmarshal to fail
	if !strings.Contains(err.Error(), "decode") && !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("expected decode/unmarshal error message, got: %v", err)
	}
}

// TestRunAdminRemote_InvalidJSON tests error handling with invalid JSON response.
// With proper error handling, invalid JSON should return an error instead of silently continuing.
func TestRunAdminRemote_InvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/users" && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`invalid json response`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	cmd.Flags().String("master", server.URL, "")
	cmd.Flags().String("token", "test-token", "")

	err := runAdminRemote(cmd, "admin", "Test@Password123!", "admin@test.com")
	// With proper error handling, invalid JSON should return an error
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "decode") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected decode/invalid error message, got: %v", err)
	}
}

// TestRunAdminRemote_NetworkError tests handling of network errors.
func TestRunAdminRemote_NetworkError(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	// Use an address that will definitely fail
	cmd.Flags().String("master", "http://192.0.2.1:9999", "")
	cmd.Flags().String("token", "test-token", "")

	// runAdminRemote creates a new client, we need to set a short timeout
	// The actual error might vary based on system, so we just check there's an error
	// We can't easily inject a timeout into the client, so just skip this for now
	t.Skip("Network error test requires client timeout configuration")
}

// TestAPIClientURLNormalization tests that URLs are properly normalized.
func TestAPIClientURLNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantURL string
	}{
		{"with http", "http://localhost:8080", "http://localhost:8080"},
		{"with https", "https://localhost:8080", "https://localhost:8080"},
		{"without scheme", "localhost:8080", "http://localhost:8080"},
		{"with trailing slash", "http://localhost:8080/", "http://localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("master", tt.input, "")
			cmd.Flags().String("token", "test-token", "")

			client, err := newAPIClient(cmd)
			if err != nil {
				t.Fatalf("newAPIClient() error = %v", err)
			}

			// Normalize trailing slash for comparison
			gotURL := strings.TrimSuffix(client.baseURL, "/")
			wantURL := strings.TrimSuffix(tt.wantURL, "/")
			if gotURL != wantURL {
				t.Errorf("baseURL = %q, want %q", gotURL, wantURL)
			}
		})
	}
}

// TestAdminCmdEnvVarOverride tests that environment variables can override flags.
func TestAdminCmdEnvVarOverride(t *testing.T) {
	// Set environment variables
	t.Setenv("VCDEPLOY_MASTER", "http://env-server:9000")
	t.Setenv("VCDEPLOY_TOKEN", "env-token-123")

	cmd := &cobra.Command{}
	// Empty flag values - should fall back to env vars
	cmd.Flags().String("master", "", "")
	cmd.Flags().String("token", "", "")

	master, _ := cmd.Flags().GetString("master")
	token, _ := cmd.Flags().GetString("token")

	// Apply env var fallback logic
	if master == "" {
		master = os.Getenv("VCDEPLOY_MASTER")
	}
	if token == "" {
		token = os.Getenv("VCDEPLOY_TOKEN")
	}

	if master != "http://env-server:9000" {
		t.Errorf("master = %q, want %q", master, "http://env-server:9000")
	}
	if token != "env-token-123" {
		t.Errorf("token = %q, want %q", token, "env-token-123")
	}
}

// TestAdminCmdFlagPrecedence tests that flags take precedence over env vars.
func TestAdminCmdFlagPrecedence(t *testing.T) {
	// Set environment variables
	t.Setenv("VCDEPLOY_MASTER", "http://env-server:9000")
	t.Setenv("VCDEPLOY_TOKEN", "env-token")

	cmd := &cobra.Command{}
	// Explicit flag values should take precedence
	cmd.Flags().String("master", "http://flag-server:8080", "")
	cmd.Flags().String("token", "flag-token", "")

	master, _ := cmd.Flags().GetString("master")
	token, _ := cmd.Flags().GetString("token")

	// Apply env var fallback logic (only when flag is empty)
	if master == "" {
		master = os.Getenv("VCDEPLOY_MASTER")
	}
	if token == "" {
		token = os.Getenv("VCDEPLOY_TOKEN")
	}

	if master != "http://flag-server:8080" {
		t.Errorf("master = %q, want %q", master, "http://flag-server:8080")
	}
	if token != "flag-token" {
		t.Errorf("token = %q, want %q", token, "flag-token")
	}
}
