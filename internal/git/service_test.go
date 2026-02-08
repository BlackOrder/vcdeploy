package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// mockStore implements storage.Store for testing.
type mockStore struct {
	storage.Store
	credentials []*storage.SourceCredential
}

func (m *mockStore) ListSourceCredentials(ctx context.Context) ([]*storage.SourceCredential, error) {
	return m.credentials, nil
}

func TestNewService(t *testing.T) {
	t.Parallel()

	logger, _ := zap.NewDevelopment()
	workDir := t.TempDir()

	svc := NewService(nil, nil, workDir, logger)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}

	if svc.workDir != workDir {
		t.Errorf("expected workDir %s, got %s", workDir, svc.workDir)
	}
}

func TestFindCredential_NoMatch(t *testing.T) {
	t.Parallel()

	logger, _ := zap.NewDevelopment()
	workDir := t.TempDir()
	store := &mockStore{
		credentials: []*storage.SourceCredential{
			{
				ID:         "test-id-1",
				Name:       "github-cred",
				Type:       SourceCredentialTypeHTTPSToken,
				URLPattern: `^https://github\.com/private/.*`,
			},
		},
	}

	svc := NewService(store, nil, workDir, logger)

	// Try to find credential for a non-matching URL
	_, err := svc.findCredential(context.Background(), "https://gitlab.com/some/repo.git")
	if err != ErrNoCredential {
		t.Errorf("expected ErrNoCredential, got %v", err)
	}
}

func TestFindCredential_Match(t *testing.T) {
	t.Parallel()

	logger, _ := zap.NewDevelopment()
	workDir := t.TempDir()
	store := &mockStore{
		credentials: []*storage.SourceCredential{
			{
				ID:         "test-id-1",
				Name:       "github-cred",
				Type:       SourceCredentialTypeHTTPSToken,
				URLPattern: `^https://github\.com/private/.*`,
			},
		},
	}

	svc := NewService(store, nil, workDir, logger)

	// Find credential for matching URL
	cred, err := svc.findCredential(context.Background(), "https://github.com/private/repo.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred == nil {
		t.Fatal("expected credential, got nil")
	}
	if cred.ID != "test-id-1" {
		t.Errorf("expected ID test-id-1, got %s", cred.ID)
	}
}

func TestFindCredential_InvalidPattern(t *testing.T) {
	t.Parallel()

	logger, _ := zap.NewDevelopment()
	workDir := t.TempDir()
	store := &mockStore{
		credentials: []*storage.SourceCredential{
			{
				ID:         "test-id-1",
				Name:       "bad-pattern",
				Type:       SourceCredentialTypeHTTPSToken,
				URLPattern: `[invalid`, // Invalid regex
			},
			{
				ID:         "test-id-2",
				Name:       "good-cred",
				Type:       SourceCredentialTypeHTTPSToken,
				URLPattern: `^https://github\.com/.*`,
			},
		},
	}

	svc := NewService(store, nil, workDir, logger)

	// Should skip invalid pattern and match good one
	cred, err := svc.findCredential(context.Background(), "https://github.com/repo.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.ID != "test-id-2" {
		t.Errorf("expected ID test-id-2 (skipping bad pattern), got %s", cred.ID)
	}
}

func TestCreateArchive(t *testing.T) {
	t.Parallel()

	logger, _ := zap.NewDevelopment()
	workDir := t.TempDir()
	svc := NewService(nil, nil, workDir, logger)

	// Create a test directory with some files
	srcDir := filepath.Join(t.TempDir(), "test-repo")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}

	// Create some test files
	testFile := filepath.Join(srcDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repository\n"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	subDir := filepath.Join(srcDir, "src")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	srcFile := filepath.Join(subDir, "main.go")
	if err := os.WriteFile(srcFile, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Create archive
	archivePath := filepath.Join(workDir, "test.tar.gz")
	checksum, err := svc.createArchive(srcDir, archivePath)
	if err != nil {
		t.Fatalf("createArchive failed: %v", err)
	}

	// Verify archive exists
	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("archive not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("archive is empty")
	}

	// Verify checksum is not empty
	if checksum == "" {
		t.Error("checksum is empty")
	}
	if len(checksum) != 64 { // SHA256 hex length
		t.Errorf("unexpected checksum length: %d", len(checksum))
	}
}

func TestEmbedTokenInURL_GitHub(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		url      string
		token    string
		expected string
	}{
		{
			name:     "github https",
			url:      "https://github.com/owner/repo.git",
			token:    "ghp_testtoken123",
			expected: "https://x-access-token:ghp_testtoken123@github.com/owner/repo.git",
		},
		{
			name:     "gitlab https",
			url:      "https://gitlab.com/owner/repo.git",
			token:    "glpat-testtoken",
			expected: "https://oauth2:glpat-testtoken@gitlab.com/owner/repo.git",
		},
		{
			name:     "generic https",
			url:      "https://git.example.com/repo.git",
			token:    "mytoken",
			expected: "https://mytoken@git.example.com/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := embedTokenInURL(tt.url, tt.token)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestEmbedBasicAuthInURL(t *testing.T) {
	t.Parallel()

	result := embedBasicAuthInURL("https://example.com/repo.git", "user", "pass")
	expected := "https://user:pass@example.com/repo.git"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestSanitizeURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "url with token",
			input:    "https://ghp_token123@github.com/repo.git",
			expected: "https://github.com/repo.git",
		},
		{
			name:     "url with basic auth",
			input:    "https://user:password@github.com/repo.git",
			expected: "https://github.com/repo.git",
		},
		{
			name:     "clean url",
			input:    "https://github.com/repo.git",
			expected: "https://github.com/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := sanitizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestSanitizeOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "github token",
			input:    "fatal: Authentication failed for 'https://ghp_abc123xyz@github.com/repo.git'",
			expected: "fatal: Authentication failed for 'https://[REDACTED]@github.com/repo.git'",
		},
		{
			name:     "gitlab token",
			input:    "error with token glpat-abcdef123",
			expected: "error with token [REDACTED]",
		},
		{
			name:     "no sensitive data",
			input:    "Cloning into '/tmp/repo'...",
			expected: "Cloning into '/tmp/repo'...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := sanitizeOutput(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// SourceCredentialType constants for testing (duplicated from storage for test isolation).
const (
	SourceCredentialTypeSSHKey     = "ssh_key"
	SourceCredentialTypeHTTPSToken = "https_token"
	SourceCredentialTypeHTTPSBasic = "https_basic"
)
