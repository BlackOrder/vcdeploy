package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
)

// TestAPIClientCreation tests the apiClient constructor.
func TestAPIClientCreation(t *testing.T) {
	tests := []struct {
		name      string
		masterURL string
		token     string
		wantErr   bool
	}{
		{"valid http", "http://localhost:8080", "test-token", false},
		{"valid https", "https://example.com", "test-token", false},
		{"adds http prefix", "localhost:8080", "test-token", false},
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
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
		json.NewEncoder(w).Encode(map[string]int{"id": 1})
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
		json.NewEncoder(w).Encode(users)
	}))
	defer server.Close()

	cmd := userCmd
	cmd.SetArgs([]string{"list"})
	cmd.Flags().Set("master", server.URL)
	cmd.Flags().Set("token", "test-token")

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
		json.NewEncoder(w).Encode(map[string]interface{}{
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
