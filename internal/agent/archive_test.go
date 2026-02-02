package agent

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"go.uber.org/zap"
)

// createTestArchive creates a test tar.gz archive with the given files
func createTestArchive(t *testing.T, files map[string]string) string {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test-*.tar.gz")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer tmpFile.Close()

	gw := gzip.NewWriter(tmpFile)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write content: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	return tmpFile.Name()
}

// createTestArchiveWithType creates a test archive with specific file types
func createTestArchiveWithType(t *testing.T, name string, typeflag byte, linkname string) string {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test-*.tar.gz")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer tmpFile.Close()

	gw := gzip.NewWriter(tmpFile)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name:     name,
		Mode:     0644,
		Size:     0,
		Typeflag: typeflag,
		Linkname: linkname,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	return tmpFile.Name()
}

func testAgent(t *testing.T, dataDir string) *Agent {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	return &Agent{
		config: &config.AgentConfig{
			Paths: config.AgentPathsConfig{
				Data:     dataDir,
				Releases: filepath.Join(dataDir, "releases"),
			},
		},
		logger: logger,
	}
}

func TestExtractArchive_Success(t *testing.T) {
	// Create a simple valid archive
	archive := createTestArchive(t, map[string]string{
		"file1.txt":     "hello world",
		"dir/file2.txt": "nested file",
	})
	defer os.Remove(archive)

	tmpDir := t.TempDir()
	agent := testAgent(t, tmpDir)
	destDir := filepath.Join(tmpDir, "extracted")

	err := agent.extractArchive(archive, destDir)
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}

	// Verify files exist
	content, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
	if err != nil {
		t.Fatalf("read file1.txt: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("file1.txt content = %q, want %q", content, "hello world")
	}

	content, err = os.ReadFile(filepath.Join(destDir, "dir/file2.txt"))
	if err != nil {
		t.Fatalf("read dir/file2.txt: %v", err)
	}
	if string(content) != "nested file" {
		t.Errorf("dir/file2.txt content = %q, want %q", content, "nested file")
	}
}

func TestExtractArchive_PathTraversal(t *testing.T) {
	// Create archive with path traversal attempt
	archive := createTestArchive(t, map[string]string{
		"../../../etc/passwd": "malicious content",
	})
	defer os.Remove(archive)

	tmpDir := t.TempDir()
	agent := testAgent(t, tmpDir)
	destDir := filepath.Join(tmpDir, "extracted")

	err := agent.extractArchive(archive, destDir)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "path traversal") && !strings.Contains(err.Error(), "..") {
		t.Errorf("expected path traversal error, got: %v", err)
	}
}

func TestExtractArchive_AbsolutePath(t *testing.T) {
	// Create archive with absolute path
	archive := createTestArchive(t, map[string]string{
		"/etc/passwd": "malicious content",
	})
	defer os.Remove(archive)

	tmpDir := t.TempDir()
	agent := testAgent(t, tmpDir)
	destDir := filepath.Join(tmpDir, "extracted")

	err := agent.extractArchive(archive, destDir)
	if err == nil {
		t.Fatal("expected error for absolute path, got nil")
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("expected absolute path error, got: %v", err)
	}
}

func TestExtractArchive_SymlinkRejected(t *testing.T) {
	// Create archive with symlink
	archive := createTestArchiveWithType(t, "link", tar.TypeSymlink, "/etc/passwd")
	defer os.Remove(archive)

	tmpDir := t.TempDir()
	agent := testAgent(t, tmpDir)
	destDir := filepath.Join(tmpDir, "extracted")

	err := agent.extractArchive(archive, destDir)
	if err == nil {
		t.Fatal("expected error for symlink, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") && !strings.Contains(err.Error(), "hard link") {
		t.Errorf("expected symlink rejection error, got: %v", err)
	}
}

func TestExtractArchive_HardlinkRejected(t *testing.T) {
	// Create archive with hard link
	archive := createTestArchiveWithType(t, "link", tar.TypeLink, "somefile")
	defer os.Remove(archive)

	tmpDir := t.TempDir()
	agent := testAgent(t, tmpDir)
	destDir := filepath.Join(tmpDir, "extracted")

	err := agent.extractArchive(archive, destDir)
	if err == nil {
		t.Fatal("expected error for hard link, got nil")
	}
	if !strings.Contains(err.Error(), "hard link") && !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected hard link rejection error, got: %v", err)
	}
}

func TestExtractArchive_ModesSanitized(t *testing.T) {
	// Create archive with setuid file
	tmpFile, err := os.CreateTemp("", "test-*.tar.gz")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	gw := gzip.NewWriter(tmpFile)
	tw := tar.NewWriter(gw)

	// Create file with setuid bit
	hdr := &tar.Header{
		Name: "setuid_file",
		Mode: 04755, // setuid bit set
		Size: 5,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatalf("write content: %v", err)
	}

	tw.Close()
	gw.Close()

	tmpDir := t.TempDir()
	agent := testAgent(t, tmpDir)
	destDir := filepath.Join(tmpDir, "extracted")

	err = agent.extractArchive(tmpFile.Name(), destDir)
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}

	// Verify setuid bit was stripped
	info, err := os.Stat(filepath.Join(destDir, "setuid_file"))
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}

	mode := info.Mode()
	if mode&os.ModeSetuid != 0 {
		t.Error("setuid bit was not stripped")
	}
	if mode&os.ModeSetgid != 0 {
		t.Error("setgid bit was not stripped")
	}
}
