package security

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeOps(t *testing.T) {
	// Create a temporary base directory for testing
	baseDir := t.TempDir()

	// Create SafeOps instance
	safeOps, err := NewSafeOps(baseDir)
	if err != nil {
		t.Fatalf("NewSafeOps() error = %v", err)
	}

	t.Run("SafeMkdirAll creates nested directories", func(t *testing.T) {
		err := safeOps.SafeMkdirAll("a/b/c", 0o755)
		if err != nil {
			t.Errorf("SafeMkdirAll() error = %v", err)
		}

		// Verify directory exists
		//nolint:gocritic // Using path separator intentionally for testing path traversal
		info, err := os.Stat(filepath.Join(baseDir, "a/b/c"))
		if err != nil {
			t.Errorf("Directory was not created: %v", err)
		}
		if !info.IsDir() {
			t.Error("Created path is not a directory")
		}
	})

	t.Run("SafeWriteFile creates file", func(t *testing.T) {
		content := []byte("test content")
		err := safeOps.SafeWriteFile("testfile.txt", content, 0o644)
		if err != nil {
			t.Errorf("SafeWriteFile() error = %v", err)
		}

		// Verify file content
		read, err := os.ReadFile(filepath.Join(baseDir, "testfile.txt"))
		if err != nil {
			t.Errorf("File was not created: %v", err)
		}
		if !bytes.Equal(read, content) {
			t.Errorf("Content mismatch: got %s, want %s", read, content)
		}
	})

	t.Run("SafeReadFile reads file", func(t *testing.T) {
		content, err := safeOps.SafeReadFile("testfile.txt")
		if err != nil {
			t.Errorf("SafeReadFile() error = %v", err)
		}
		if string(content) != "test content" {
			t.Errorf("Content mismatch: got %s", content)
		}
	})

	t.Run("SafeRemove removes file", func(t *testing.T) {
		// Create a file to remove
		testPath := filepath.Join(baseDir, "to_remove.txt")
		if err := os.WriteFile(testPath, []byte("delete me"), 0o644); err != nil {
			t.Fatalf("Setup error: %v", err)
		}

		err := safeOps.SafeRemove("to_remove.txt")
		if err != nil {
			t.Errorf("SafeRemove() error = %v", err)
		}

		// Verify file is removed
		if _, err := os.Stat(testPath); !os.IsNotExist(err) {
			t.Error("File was not removed")
		}
	})

	t.Run("SafeRemoveAll removes directory tree", func(t *testing.T) {
		// Create a directory tree to remove
		//nolint:gocritic // Using path separator intentionally for testing path traversal
		if err := os.MkdirAll(filepath.Join(baseDir, "tree/sub"), 0o755); err != nil {
			t.Fatalf("Setup error: %v", err)
		}
		//nolint:gocritic // Using path separator intentionally for testing path traversal
		if err := os.WriteFile(filepath.Join(baseDir, "tree/sub/file.txt"), []byte("content"), 0o644); err != nil {
			t.Fatalf("Setup error: %v", err)
		}

		err := safeOps.SafeRemoveAll("tree")
		if err != nil {
			t.Errorf("SafeRemoveAll() error = %v", err)
		}

		// Verify tree is removed
		if _, err := os.Stat(filepath.Join(baseDir, "tree")); !os.IsNotExist(err) {
			t.Error("Directory tree was not removed")
		}
	})

	t.Run("SafeRename moves file", func(t *testing.T) {
		// Create a file to rename
		oldPath := filepath.Join(baseDir, "old_name.txt")
		if err := os.WriteFile(oldPath, []byte("content"), 0o644); err != nil {
			t.Fatalf("Setup error: %v", err)
		}

		err := safeOps.SafeRename("old_name.txt", "new_name.txt")
		if err != nil {
			t.Errorf("SafeRename() error = %v", err)
		}

		// Verify old path doesn't exist
		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Error("Old file still exists")
		}

		// Verify new path exists
		if _, err := os.Stat(filepath.Join(baseDir, "new_name.txt")); err != nil {
			t.Error("New file does not exist")
		}
	})
}

func TestSafeOps_PathTraversal(t *testing.T) {
	baseDir := t.TempDir()

	safeOps, err := NewSafeOps(baseDir)
	if err != nil {
		t.Fatalf("NewSafeOps() error = %v", err)
	}

	t.Run("SafeRemove blocks traversal", func(t *testing.T) {
		err := safeOps.SafeRemove("../../../etc/passwd")
		if err == nil {
			t.Error("SafeRemove() should have returned an error for path traversal")
		}
	})

	t.Run("SafeRemoveAll blocks traversal", func(t *testing.T) {
		err := safeOps.SafeRemoveAll("../../../tmp")
		if err == nil {
			t.Error("SafeRemoveAll() should have returned an error for path traversal")
		}
	})

	t.Run("SafeMkdir blocks traversal", func(t *testing.T) {
		err := safeOps.SafeMkdir("../../../tmp/evil", 0o755)
		if err == nil {
			t.Error("SafeMkdir() should have returned an error for path traversal")
		}
	})

	t.Run("SafeMkdirAll blocks traversal", func(t *testing.T) {
		err := safeOps.SafeMkdirAll("../../../tmp/evil/nested", 0o755)
		if err == nil {
			t.Error("SafeMkdirAll() should have returned an error for path traversal")
		}
	})

	t.Run("SafeWriteFile blocks traversal", func(t *testing.T) {
		err := safeOps.SafeWriteFile("../../../tmp/evil.txt", []byte("evil"), 0o644)
		if err == nil {
			t.Error("SafeWriteFile() should have returned an error for path traversal")
		}
	})

	t.Run("SafeReadFile blocks traversal", func(t *testing.T) {
		_, err := safeOps.SafeReadFile("/etc/passwd")
		if err == nil {
			t.Error("SafeReadFile() should have returned an error for path traversal")
		}
	})

	t.Run("SafeRename blocks source traversal", func(t *testing.T) {
		err := safeOps.SafeRename("../../../etc/passwd", "stolen.txt")
		if err == nil {
			t.Error("SafeRename() should have returned an error for source path traversal")
		}
	})

	t.Run("SafeRename blocks destination traversal", func(t *testing.T) {
		// Create a file to rename
		testFile := filepath.Join(baseDir, "source.txt")
		if err := os.WriteFile(testFile, []byte("content"), 0o644); err != nil {
			t.Fatalf("Setup error: %v", err)
		}

		err := safeOps.SafeRename("source.txt", "../../../tmp/evil.txt")
		if err == nil {
			t.Error("SafeRename() should have returned an error for destination path traversal")
		}
	})
}

func TestSafeOps_ProtectedPaths(t *testing.T) {
	baseDir := t.TempDir()

	safeOps, err := NewSafeOps(baseDir)
	if err != nil {
		t.Fatalf("NewSafeOps() error = %v", err)
	}

	t.Run("SafeRemove blocks removing base directory", func(t *testing.T) {
		err := safeOps.SafeRemove(baseDir)
		if err != ErrRemoveProtected {
			t.Errorf("SafeRemove() error = %v, want ErrRemoveProtected", err)
		}
	})

	t.Run("SafeRemoveAll blocks removing base directory", func(t *testing.T) {
		err := safeOps.SafeRemoveAll(baseDir)
		if err != ErrRemoveProtected {
			t.Errorf("SafeRemoveAll() error = %v, want ErrRemoveProtected", err)
		}
	})
}

func TestSafeOps_Symlink(t *testing.T) {
	baseDir := t.TempDir()

	safeOps, err := NewSafeOps(baseDir)
	if err != nil {
		t.Fatalf("NewSafeOps() error = %v", err)
	}

	t.Run("SafeSymlink disabled by default", func(t *testing.T) {
		err := safeOps.SafeSymlink("target.txt", "link.txt")
		if err == nil {
			t.Error("SafeSymlink() should be disabled by default")
		}
	})

	// Enable symlinks for further tests
	safeOps.AllowSymlinks = true

	// Create a target file
	targetPath := filepath.Join(baseDir, "target.txt")
	if err := os.WriteFile(targetPath, []byte("content"), 0o644); err != nil {
		t.Fatalf("Setup error: %v", err)
	}

	t.Run("SafeSymlink creates valid symlink", func(t *testing.T) {
		err := safeOps.SafeSymlink("target.txt", "link.txt")
		if err != nil {
			t.Errorf("SafeSymlink() error = %v", err)
		}

		// Verify symlink exists and points to target
		linkPath := filepath.Join(baseDir, "link.txt")
		target, err := os.Readlink(linkPath)
		if err != nil {
			t.Errorf("Failed to read symlink: %v", err)
		}
		if target != "target.txt" {
			t.Errorf("Symlink target = %s, want target.txt", target)
		}
	})

	t.Run("SafeSymlink blocks target outside base", func(t *testing.T) {
		err := safeOps.SafeSymlink("/etc/passwd", "evil_link.txt")
		if err == nil {
			t.Error("SafeSymlink() should block target outside base directory")
		}
	})

	t.Run("SafeSymlink blocks link path outside base", func(t *testing.T) {
		err := safeOps.SafeSymlink("target.txt", "../../../tmp/evil_link")
		if err == nil {
			t.Error("SafeSymlink() should block link path outside base directory")
		}
	})
}

func TestNewSafeOps_Errors(t *testing.T) {
	t.Run("non-existent directory", func(t *testing.T) {
		_, err := NewSafeOps("/nonexistent/path/that/should/not/exist")
		if err == nil {
			t.Error("NewSafeOps() should return error for non-existent directory")
		}
	})

	t.Run("file instead of directory", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "safeops_test")
		if err != nil {
			t.Fatalf("Setup error: %v", err)
		}
		defer os.Remove(tmpFile.Name())
		tmpFile.Close()

		_, err = NewSafeOps(tmpFile.Name())
		if err != ErrNotDirectory {
			t.Errorf("NewSafeOps() error = %v, want ErrNotDirectory", err)
		}
	})
}

func TestSafeOps_SafeOpen(t *testing.T) {
	tmpDir := t.TempDir()
	safeOps, err := NewSafeOps(tmpDir)
	if err != nil {
		t.Fatalf("NewSafeOps() error = %v", err)
	}

	// Create a test file
	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Test successful open
	f, err := safeOps.SafeOpen("testfile.txt")
	if err != nil {
		t.Errorf("SafeOpen() error = %v", err)
	}
	if f != nil {
		f.Close()
	}

	// Test path traversal
	_, err = safeOps.SafeOpen("../etc/passwd")
	if err == nil {
		t.Error("SafeOpen() should fail for path traversal")
	}
}

func TestSafeOps_SafeCreate(t *testing.T) {
	tmpDir := t.TempDir()
	safeOps, err := NewSafeOps(tmpDir)
	if err != nil {
		t.Fatalf("NewSafeOps() error = %v", err)
	}

	// Test successful create with relative path
	f, err := safeOps.SafeCreate("newfile.txt")
	if err != nil {
		t.Errorf("SafeCreate() error = %v", err)
	}
	if f != nil {
		f.Close()
	}

	// Test successful create with absolute path inside base
	absPath := filepath.Join(tmpDir, "newfile2.txt")
	f, err = safeOps.SafeCreate(absPath)
	if err != nil {
		t.Errorf("SafeCreate() with absolute path error = %v", err)
	}
	if f != nil {
		f.Close()
	}

	// Test path traversal with relative path
	_, err = safeOps.SafeCreate("../etc/passwd")
	if err == nil {
		t.Error("SafeCreate() should fail for path traversal")
	}

	// Test path traversal with absolute path outside base
	_, err = safeOps.SafeCreate("/tmp/outside.txt")
	if err != ErrOperationDenied {
		t.Errorf("SafeCreate() with absolute path outside base error = %v, want ErrOperationDenied", err)
	}
}

func TestSafeOps_SafeStat(t *testing.T) {
	tmpDir := t.TempDir()
	safeOps, err := NewSafeOps(tmpDir)
	if err != nil {
		t.Fatalf("NewSafeOps() error = %v", err)
	}

	// Create a test file
	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Test successful stat
	info, err := safeOps.SafeStat("testfile.txt")
	if err != nil {
		t.Errorf("SafeStat() error = %v", err)
	}
	if info == nil {
		t.Error("SafeStat() returned nil info")
	}

	// Test path traversal
	_, err = safeOps.SafeStat("../etc/passwd")
	if err == nil {
		t.Error("SafeStat() should fail for path traversal")
	}
}

func TestSafeOps_SafeLstat(t *testing.T) {
	tmpDir := t.TempDir()
	safeOps, err := NewSafeOps(tmpDir)
	if err != nil {
		t.Fatalf("NewSafeOps() error = %v", err)
	}

	// Create a test file
	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Test successful lstat
	info, err := safeOps.SafeLstat("testfile.txt")
	if err != nil {
		t.Errorf("SafeLstat() error = %v", err)
	}
	if info == nil {
		t.Error("SafeLstat() returned nil info")
	}

	// Test path traversal
	_, err = safeOps.SafeLstat("../etc/passwd")
	if err == nil {
		t.Error("SafeLstat() should fail for path traversal")
	}
}

func TestSafeOps_SafeChmod(t *testing.T) {
	tmpDir := t.TempDir()
	safeOps, err := NewSafeOps(tmpDir)
	if err != nil {
		t.Fatalf("NewSafeOps() error = %v", err)
	}

	// Create a test file
	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Test successful chmod
	err = safeOps.SafeChmod("testfile.txt", 0o600)
	if err != nil {
		t.Errorf("SafeChmod() error = %v", err)
	}

	// Verify the mode changed
	info, _ := os.Stat(testFile)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("SafeChmod() mode = %v, want 0o600", info.Mode().Perm())
	}

	// Test path traversal
	err = safeOps.SafeChmod("../etc/passwd", 0o600)
	if err == nil {
		t.Error("SafeChmod() should fail for path traversal")
	}
}

func TestSafeOps_SafeChown(t *testing.T) {
	tmpDir := t.TempDir()
	safeOps, err := NewSafeOps(tmpDir)
	if err != nil {
		t.Fatalf("NewSafeOps() error = %v", err)
	}

	// Create a test file
	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Test chown with -1 (no change) - this should succeed
	err = safeOps.SafeChown("testfile.txt", -1, -1)
	if err != nil {
		t.Errorf("SafeChown() error = %v", err)
	}

	// Test path traversal
	err = safeOps.SafeChown("../etc/passwd", -1, -1)
	if err == nil {
		t.Error("SafeChown() should fail for path traversal")
	}
}
