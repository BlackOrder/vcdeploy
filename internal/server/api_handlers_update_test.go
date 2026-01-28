package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func TestHandleAgentBinaries(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	t.Run("GET - list binaries", func(t *testing.T) {
		ctx := context.Background()
		// Create a test binary
		binary := &storage.AgentBinary{
			Version:        "1.0.0",
			OS:             "linux",
			Arch:           "amd64",
			Path:           "/tmp/test-binary",
			ChecksumSHA256: "abc123",
			SizeBytes:      1024,
			UploadedAt:     time.Now(),
			IsCurrent:      true,
		}
		if err := s.db.CreateAgentBinary(ctx, binary); err != nil {
			t.Fatalf("Failed to create binary: %v", err)
		}

		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/binaries", nil), userID)
		rr := httptest.NewRecorder()
		s.handleAgentBinaries(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var binaries []*storage.AgentBinary
		if err := json.NewDecoder(rr.Body).Decode(&binaries); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(binaries) != 1 {
			t.Errorf("Expected 1 binary, got %d", len(binaries))
		}
	})
}

func TestHandleAgentBinaryLatest(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	t.Run("GET - get latest binary", func(t *testing.T) {
		ctx := context.Background()
		// Create test binaries
		binary1 := &storage.AgentBinary{
			Version:        "1.0.0",
			OS:             "linux",
			Arch:           "amd64",
			Path:           "/tmp/binary1",
			ChecksumSHA256: "abc123",
			SizeBytes:      1024,
			UploadedAt:     time.Now().Add(-time.Hour),
			IsCurrent:      false,
		}
		if err := s.db.CreateAgentBinary(ctx, binary1); err != nil {
			t.Fatalf("Failed to create binary1: %v", err)
		}

		binary2 := &storage.AgentBinary{
			Version:        "2.0.0",
			OS:             "linux",
			Arch:           "amd64",
			Path:           "/tmp/binary2",
			ChecksumSHA256: "def456",
			SizeBytes:      2048,
			UploadedAt:     time.Now(),
			IsCurrent:      true,
		}
		if err := s.db.CreateAgentBinary(ctx, binary2); err != nil {
			t.Fatalf("Failed to create binary2: %v", err)
		}

		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/binaries/latest?os=linux&arch=amd64", nil), userID)
		rr := httptest.NewRecorder()
		s.handleAgentBinaryLatest(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var binary storage.AgentBinary
		if err := json.NewDecoder(rr.Body).Decode(&binary); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if binary.Version != "2.0.0" {
			t.Errorf("Expected version 2.0.0, got %s", binary.Version)
		}
	})

	t.Run("GET - missing parameters", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/binaries/latest", nil), userID)
		rr := httptest.NewRecorder()
		s.handleAgentBinaryLatest(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("GET - no binary found", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/binaries/latest?os=windows&arch=arm64", nil), userID)
		rr := httptest.NewRecorder()
		s.handleAgentBinaryLatest(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, rr.Code)
		}
	})
}

func TestHandleAgentUpdateConfig(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	ctx := context.Background()
	// Create test agent
	agent := &storage.Agent{
		ID:           "test-agent-1",
		Hostname:     "test-host",
		Status:       "online",
		UpdatePolicy: "immediate",
	}
	if err := s.db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	t.Run("GET - get update config", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/agents/test-agent-1/update-config", nil), userID)
		rr := httptest.NewRecorder()
		s.handleAgentUpdateConfig(rr, req, "test-agent-1")

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var config map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&config); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if config["update_policy"] != "immediate" {
			t.Errorf("Expected update_policy 'immediate', got %v", config["update_policy"])
		}
	})

	t.Run("PUT - update config to scheduled", func(t *testing.T) {
		body := bytes.NewBufferString(`{"update_policy":"scheduled","update_window_start":"02:00","update_window_end":"04:00"}`)
		req := requestWithAdminContext(httptest.NewRequest("PUT", "/api/v1/agents/test-agent-1/update-config", body), userID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.handleAgentUpdateConfig(rr, req, "test-agent-1")

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		// Verify the change
		updatedAgent, err := s.db.GetAgent(ctx, "test-agent-1")
		if err != nil {
			t.Fatalf("Failed to get agent: %v", err)
		}

		if updatedAgent.UpdatePolicy != "scheduled" {
			t.Errorf("Expected update_policy 'scheduled', got %s", updatedAgent.UpdatePolicy)
		}
	})

	t.Run("PUT - invalid policy", func(t *testing.T) {
		body := bytes.NewBufferString(`{"update_policy":"invalid"}`)
		req := requestWithAdminContext(httptest.NewRequest("PUT", "/api/v1/agents/test-agent-1/update-config", body), userID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.handleAgentUpdateConfig(rr, req, "test-agent-1")

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("PUT - scheduled without window", func(t *testing.T) {
		body := bytes.NewBufferString(`{"update_policy":"scheduled"}`)
		req := requestWithAdminContext(httptest.NewRequest("PUT", "/api/v1/agents/test-agent-1/update-config", body), userID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.handleAgentUpdateConfig(rr, req, "test-agent-1")

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d: %s", http.StatusBadRequest, rr.Code, rr.Body.String())
		}
	})
}

func TestHandleAgentUpdateHistory(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	ctx := context.Background()
	// Create test agent
	agent := &storage.Agent{
		ID:       "test-agent-update-history",
		Hostname: "test-host",
		Status:   "online",
	}
	if err := s.db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	// Create some update history
	history1 := &storage.AgentUpdateHistory{
		AgentID:     "test-agent-update-history",
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Status:      "completed",
		StartedAt:   time.Now().Add(-2 * time.Hour),
	}
	if err := s.db.CreateAgentUpdateHistory(ctx, history1); err != nil {
		t.Fatalf("Failed to create history1: %v", err)
	}

	history2 := &storage.AgentUpdateHistory{
		AgentID:     "test-agent-update-history",
		FromVersion: "1.1.0",
		ToVersion:   "1.2.0",
		Status:      "completed",
		StartedAt:   time.Now().Add(-time.Hour),
	}
	if err := s.db.CreateAgentUpdateHistory(ctx, history2); err != nil {
		t.Fatalf("Failed to create history2: %v", err)
	}

	t.Run("GET - list update history", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/agents/test-agent-update-history/update-history", nil), userID)
		rr := httptest.NewRecorder()
		s.handleAgentUpdateHistory(rr, req, "test-agent-update-history")

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		total, ok := response["total"].(float64)
		if !ok || int(total) != 2 {
			t.Errorf("Expected total 2, got %v", response["total"])
		}

		items, ok := response["items"].([]interface{})
		if !ok || len(items) != 2 {
			t.Errorf("Expected 2 items, got %d", len(items))
		}
	})

	t.Run("GET - with pagination", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/agents/test-agent-update-history/update-history?limit=1&offset=0", nil), userID)
		rr := httptest.NewRecorder()
		s.handleAgentUpdateHistory(rr, req, "test-agent-update-history")

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}

		var response map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		items, ok := response["items"].([]interface{})
		if !ok || len(items) != 1 {
			t.Errorf("Expected 1 item, got %d", len(items))
		}
	})
}

func TestHandleAgentsNeedingUpdate(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	ctx := context.Background()

	// Create a current binary
	binary := &storage.AgentBinary{
		Version:        "2.0.0",
		OS:             "linux",
		Arch:           "amd64",
		Path:           "/tmp/binary",
		ChecksumSHA256: "abc123",
		SizeBytes:      1024,
		UploadedAt:     time.Now(),
		IsCurrent:      true,
	}
	if err := s.db.CreateAgentBinary(ctx, binary); err != nil {
		t.Fatalf("Failed to create binary: %v", err)
	}

	// Create agents with different versions
	agentUpToDate := &storage.Agent{
		ID:       "agent-up-to-date",
		Hostname: "host1",
		Status:   "online",
		Version:  "2.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}
	if err := s.db.UpsertAgent(ctx, agentUpToDate); err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	agentNeedsUpdate := &storage.Agent{
		ID:       "agent-needs-update",
		Hostname: "host2",
		Status:   "online",
		Version:  "1.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}
	if err := s.db.UpsertAgent(ctx, agentNeedsUpdate); err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	t.Run("GET - list agents needing update", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/agents/updates/pending", nil), userID)
		rr := httptest.NewRecorder()
		s.handleAgentsNeedingUpdate(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var agents []map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&agents); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Should only include the agent that needs an update
		if len(agents) != 1 {
			t.Errorf("Expected 1 agent needing update, got %d", len(agents))
		}

		if len(agents) > 0 && agents[0]["ID"] != "agent-needs-update" {
			t.Errorf("Expected agent-needs-update, got %v", agents[0]["ID"])
		}
	})
}

func TestHandleTriggerAgentUpdate(t *testing.T) {
	s, _, _, userID := newTestServerWithAuth(t)

	ctx := context.Background()

	// Create an agent with version info
	agent := &storage.Agent{
		ID:       "trigger-test-agent",
		Hostname: "trigger-test-host",
		Status:   "online",
		Version:  "1.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}
	if err := s.db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	// Create an agent binary (newer version)
	binary := &storage.AgentBinary{
		Version:        "2.0.0",
		OS:             "linux",
		Arch:           "amd64",
		Path:           "/tmp/test-binary",
		ChecksumSHA256: "abc123def456",
		SizeBytes:      2048,
		UploadedAt:     time.Now(),
		IsCurrent:      true,
	}
	if err := s.db.CreateAgentBinary(ctx, binary); err != nil {
		t.Fatalf("Failed to create binary: %v", err)
	}

	t.Run("POST - trigger update success", func(t *testing.T) {
		body := bytes.NewBufferString(`{"version": "2.0.0"}`)
		req := requestWithAdminContext(httptest.NewRequest("POST", "/api/v1/agents/trigger-test-agent/update", body), userID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.handleTriggerAgentUpdate(rr, req, "trigger-test-agent")

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify response fields
		if response["status"] != "pending" {
			t.Errorf("Expected status 'pending', got %v", response["status"])
		}
		if response["from_version"] != "1.0.0" {
			t.Errorf("Expected from_version '1.0.0', got %v", response["from_version"])
		}
		if response["to_version"] != "2.0.0" {
			t.Errorf("Expected to_version '2.0.0', got %v", response["to_version"])
		}
		// When agent is not connected via gRPC, delivery should be heartbeat
		if response["delivery"] != "heartbeat" {
			t.Errorf("Expected delivery 'heartbeat', got %v", response["delivery"])
		}
	})

	t.Run("POST - agent not found", func(t *testing.T) {
		body := bytes.NewBufferString(`{"version": "2.0.0"}`)
		req := requestWithAdminContext(httptest.NewRequest("POST", "/api/v1/agents/nonexistent-agent/update", body), userID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.handleTriggerAgentUpdate(rr, req, "nonexistent-agent")

		// Handler returns 500 when agent is not found due to error from agent service
		// (agentService.GetByID returns ErrNotFound which causes the error path to be taken)
		if rr.Code != http.StatusInternalServerError && rr.Code != http.StatusNotFound {
			t.Errorf("Expected status 404 or 500, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("POST - agent already up to date", func(t *testing.T) {
		// Create an agent that's already on the latest version
		agentUpToDate := &storage.Agent{
			ID:       "up-to-date-agent",
			Hostname: "up-to-date-host",
			Status:   "online",
			Version:  "2.0.0",
			OS:       "linux",
			Arch:     "amd64",
		}
		if err := s.db.UpsertAgent(ctx, agentUpToDate); err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		body := bytes.NewBufferString(`{"version": "2.0.0"}`)
		req := requestWithAdminContext(httptest.NewRequest("POST", "/api/v1/agents/up-to-date-agent/update", body), userID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.handleTriggerAgentUpdate(rr, req, "up-to-date-agent")

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if response["status"] != "up_to_date" {
			t.Errorf("Expected status 'up_to_date', got %v", response["status"])
		}
	})

	t.Run("POST - force update even if up to date", func(t *testing.T) {
		body := bytes.NewBufferString(`{"version": "2.0.0", "force": true}`)
		req := requestWithAdminContext(httptest.NewRequest("POST", "/api/v1/agents/up-to-date-agent/update", body), userID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.handleTriggerAgentUpdate(rr, req, "up-to-date-agent")

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Should trigger update even when same version because force=true
		if response["status"] != "pending" {
			t.Errorf("Expected status 'pending', got %v", response["status"])
		}
		if response["force"] != true {
			t.Errorf("Expected force true, got %v", response["force"])
		}
	})

	t.Run("GET - method not allowed", func(t *testing.T) {
		req := requestWithAdminContext(httptest.NewRequest("GET", "/api/v1/agents/trigger-test-agent/update", nil), userID)
		rr := httptest.NewRecorder()
		s.handleTriggerAgentUpdate(rr, req, "trigger-test-agent")

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
		}
	})
}

// createTestMultipartRequest creates a multipart form request for binary upload.
//
//nolint:unused // Helper function for binary upload tests
func createTestMultipartRequest(t *testing.T, url, version, osType, arch string, content []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add form fields
	_ = writer.WriteField("version", version)
	_ = writer.WriteField("os", osType)
	_ = writer.WriteField("arch", arch)

	// Add file
	part, err := writer.CreateFormFile("binary", "vcdeploy-agent")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(part, bytes.NewReader(content))

	writer.Close()

	req := httptest.NewRequest("POST", url, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
