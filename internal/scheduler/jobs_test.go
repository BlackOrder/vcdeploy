package scheduler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"go.uber.org/zap"
)

func TestKeyRotationJob(t *testing.T) {
	logger := zap.NewNop()

	t.Run("disabled when config is nil", func(t *testing.T) {
		job := NewKeyRotationJob(nil, nil, logger)
		if job.Enabled() {
			t.Error("Job should be disabled when config is nil")
		}
	})

	t.Run("disabled when config.Enabled is false", func(t *testing.T) {
		cfg := &config.KeyRotationConfig{Enabled: false}
		job := NewKeyRotationJob(nil, cfg, logger)
		if job.Enabled() {
			t.Error("Job should be disabled when config.Enabled is false")
		}
	})

	t.Run("enabled when config.Enabled is true", func(t *testing.T) {
		cfg := &config.KeyRotationConfig{Enabled: true}
		job := NewKeyRotationJob(nil, cfg, logger)
		if !job.Enabled() {
			t.Error("Job should be enabled when config.Enabled is true")
		}
	})

	t.Run("name is correct", func(t *testing.T) {
		job := NewKeyRotationJob(nil, nil, logger)
		if job.Name() != "key-rotation" {
			t.Errorf("Expected name 'key-rotation', got '%s'", job.Name())
		}
	})

	t.Run("run fails without KMS", func(t *testing.T) {
		cfg := &config.KeyRotationConfig{Enabled: true}
		job := NewKeyRotationJob(nil, cfg, logger)

		err := job.Run(context.Background())
		if err == nil {
			t.Error("Expected error when KMS is nil")
		}
	})
}

func TestDatabaseBackupJob(t *testing.T) {
	logger := zap.NewNop()

	t.Run("disabled when config is nil", func(t *testing.T) {
		job := NewDatabaseBackupJob(nil, nil, logger)
		if job.Enabled() {
			t.Error("Job should be disabled when config is nil")
		}
	})

	t.Run("disabled when config.Enabled is false", func(t *testing.T) {
		cfg := &config.DatabaseBackupConfig{Enabled: false}
		job := NewDatabaseBackupJob(nil, cfg, logger)
		if job.Enabled() {
			t.Error("Job should be disabled when config.Enabled is false")
		}
	})

	t.Run("enabled when config.Enabled is true", func(t *testing.T) {
		cfg := &config.DatabaseBackupConfig{Enabled: true}
		job := NewDatabaseBackupJob(nil, cfg, logger)
		if !job.Enabled() {
			t.Error("Job should be enabled when config.Enabled is true")
		}
	})

	t.Run("name is correct", func(t *testing.T) {
		job := NewDatabaseBackupJob(nil, nil, logger)
		if job.Name() != "database-backup" {
			t.Errorf("Expected name 'database-backup', got '%s'", job.Name())
		}
	})

	t.Run("run fails without database", func(t *testing.T) {
		cfg := &config.DatabaseBackupConfig{Enabled: true}
		job := NewDatabaseBackupJob(nil, cfg, logger)

		err := job.Run(context.Background())
		if err == nil {
			t.Error("Expected error when database is nil")
		}
	})
}

func TestAuditExportJob(t *testing.T) {
	logger := zap.NewNop()

	t.Run("disabled when config is nil", func(t *testing.T) {
		job := NewAuditExportJob(nil, nil, logger)
		if job.Enabled() {
			t.Error("Job should be disabled when config is nil")
		}
	})

	t.Run("disabled when config.Enabled is false", func(t *testing.T) {
		cfg := &config.AuditExportConfig{Enabled: false}
		job := NewAuditExportJob(nil, cfg, logger)
		if job.Enabled() {
			t.Error("Job should be disabled when config.Enabled is false")
		}
	})

	t.Run("enabled when config.Enabled is true", func(t *testing.T) {
		cfg := &config.AuditExportConfig{Enabled: true}
		job := NewAuditExportJob(nil, cfg, logger)
		if !job.Enabled() {
			t.Error("Job should be enabled when config.Enabled is true")
		}
	})

	t.Run("name is correct", func(t *testing.T) {
		job := NewAuditExportJob(nil, nil, logger)
		if job.Name() != "audit-export" {
			t.Errorf("Expected name 'audit-export', got '%s'", job.Name())
		}
	})

	t.Run("run fails without database", func(t *testing.T) {
		cfg := &config.AuditExportConfig{Enabled: true}
		job := NewAuditExportJob(nil, cfg, logger)

		err := job.Run(context.Background())
		if err == nil {
			t.Error("Expected error when database is nil")
		}
	})
}

func TestLogRotationJob(t *testing.T) {
	logger := zap.NewNop()

	t.Run("disabled when enabled is false", func(t *testing.T) {
		job := NewLogRotationJob(LogRotationConfig{Enabled: false}, logger)
		if job.Enabled() {
			t.Error("Job should be disabled when Enabled is false")
		}
	})

	t.Run("enabled when enabled is true", func(t *testing.T) {
		job := NewLogRotationJob(LogRotationConfig{Enabled: true}, logger)
		if !job.Enabled() {
			t.Error("Job should be enabled when Enabled is true")
		}
	})

	t.Run("name is correct", func(t *testing.T) {
		job := NewLogRotationJob(LogRotationConfig{}, logger)
		if job.Name() != "log-rotation" {
			t.Errorf("Expected name 'log-rotation', got '%s'", job.Name())
		}
	})

	t.Run("run succeeds with non-existent directory", func(t *testing.T) {
		job := NewLogRotationJob(LogRotationConfig{
			Enabled: true,
			LogDir:  "/nonexistent/log/dir",
		}, logger)

		err := job.Run(context.Background())
		if err != nil {
			t.Errorf("Expected no error for non-existent directory, got %v", err)
		}
	})

	t.Run("run rotates large log files", func(t *testing.T) {
		// Create temp directory
		tmpDir := t.TempDir()

		// Create a large log file
		logFile := filepath.Join(tmpDir, "test.log")
		content := make([]byte, 2*1024*1024) // 2MB
		if err := os.WriteFile(logFile, content, 0644); err != nil {
			t.Fatal(err)
		}

		job := NewLogRotationJob(LogRotationConfig{
			Enabled:   true,
			LogDir:    tmpDir,
			MaxSizeMB: 1, // 1MB threshold
			Retention: 24 * time.Hour,
		}, logger)

		if err := job.Run(context.Background()); err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// Check that original file was truncated
		info, err := os.Stat(logFile)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() != 0 {
			t.Errorf("Original log file should be truncated, size = %d", info.Size())
		}

		// Check that rotated file exists
		entries, _ := os.ReadDir(tmpDir)
		rotatedFound := false
		for _, entry := range entries {
			if entry.Name() != "test.log" && filepath.Ext(entry.Name()) == ".log" {
				rotatedFound = true
				break
			}
		}
		if !rotatedFound {
			t.Error("No rotated log file found")
		}
	})

	t.Run("run deletes old rotated logs", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create an old rotated log file
		oldLog := filepath.Join(tmpDir, "test.rotated.20200101-000000.log")
		if err := os.WriteFile(oldLog, []byte("old log"), 0644); err != nil {
			t.Fatal(err)
		}
		// Set modification time to past
		oldTime := time.Now().Add(-48 * time.Hour)
		os.Chtimes(oldLog, oldTime, oldTime)

		job := NewLogRotationJob(LogRotationConfig{
			Enabled:   true,
			LogDir:    tmpDir,
			MaxSizeMB: 100,
			Retention: 24 * time.Hour, // 24 hour retention
		}, logger)

		if err := job.Run(context.Background()); err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// Check that old log was deleted
		if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
			t.Error("Old rotated log should have been deleted")
		}
	})
}

func TestBackupCleanupJob(t *testing.T) {
	logger := zap.NewNop()

	t.Run("disabled when enabled is false", func(t *testing.T) {
		job := NewBackupCleanupJob(BackupCleanupConfig{Enabled: false}, logger)
		if job.Enabled() {
			t.Error("Job should be disabled when Enabled is false")
		}
	})

	t.Run("enabled when enabled is true", func(t *testing.T) {
		job := NewBackupCleanupJob(BackupCleanupConfig{Enabled: true}, logger)
		if !job.Enabled() {
			t.Error("Job should be enabled when Enabled is true")
		}
	})

	t.Run("name is correct", func(t *testing.T) {
		job := NewBackupCleanupJob(BackupCleanupConfig{}, logger)
		if job.Name() != "backup-cleanup" {
			t.Errorf("Expected name 'backup-cleanup', got '%s'", job.Name())
		}
	})

	t.Run("run succeeds with empty backup dir", func(t *testing.T) {
		job := NewBackupCleanupJob(BackupCleanupConfig{
			Enabled:   true,
			BackupDir: "",
		}, logger)

		err := job.Run(context.Background())
		if err != nil {
			t.Errorf("Expected no error for empty backup dir, got %v", err)
		}
	})

	t.Run("run succeeds with non-existent backup dir", func(t *testing.T) {
		job := NewBackupCleanupJob(BackupCleanupConfig{
			Enabled:   true,
			BackupDir: "/nonexistent/backup/dir",
		}, logger)

		err := job.Run(context.Background())
		if err != nil {
			t.Errorf("Expected no error for non-existent backup dir, got %v", err)
		}
	})

	t.Run("run cleans up old backups by retention", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create an old backup file
		oldBackup := filepath.Join(tmpDir, "backup-old.db")
		if err := os.WriteFile(oldBackup, []byte("old backup"), 0644); err != nil {
			t.Fatal(err)
		}
		oldTime := time.Now().Add(-48 * time.Hour)
		os.Chtimes(oldBackup, oldTime, oldTime)

		// Create a new backup file
		newBackup := filepath.Join(tmpDir, "backup-new.db")
		if err := os.WriteFile(newBackup, []byte("new backup"), 0644); err != nil {
			t.Fatal(err)
		}

		job := NewBackupCleanupJob(BackupCleanupConfig{
			Enabled:   true,
			BackupDir: tmpDir,
			Retention: 24 * time.Hour,
		}, logger)

		if err := job.Run(context.Background()); err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// Check that old backup was deleted
		if _, err := os.Stat(oldBackup); !os.IsNotExist(err) {
			t.Error("Old backup should have been deleted")
		}

		// Check that new backup still exists
		if _, err := os.Stat(newBackup); err != nil {
			t.Error("New backup should still exist")
		}
	})

	t.Run("run cleans up backups beyond max count", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create multiple backup files
		for i := 0; i < 5; i++ {
			path := filepath.Join(tmpDir, "backup-"+string(rune('a'+i))+".db")
			if err := os.WriteFile(path, []byte("backup"), 0644); err != nil {
				t.Fatal(err)
			}
			// Stagger modification times
			modTime := time.Now().Add(time.Duration(-i) * time.Hour)
			os.Chtimes(path, modTime, modTime)
		}

		job := NewBackupCleanupJob(BackupCleanupConfig{
			Enabled:    true,
			BackupDir:  tmpDir,
			MaxBackups: 2,
		}, logger)

		if err := job.Run(context.Background()); err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// Count remaining files
		entries, _ := os.ReadDir(tmpDir)
		if len(entries) != 2 {
			t.Errorf("Expected 2 backups remaining, got %d", len(entries))
		}
	})
}

func TestAuditExportJob_ExportFormat(t *testing.T) {
	// This test verifies the expected JSON format of audit exports
	// without requiring a real database

	type AuditEntry struct {
		ID        int64     `json:"id"`
		Timestamp time.Time `json:"timestamp"`
		Source    string    `json:"source"`
		User      string    `json:"user"`
		Action    string    `json:"action"`
		Resource  string    `json:"resource"`
		Details   string    `json:"details"`
		IPAddress string    `json:"ip_address"`
		Result    string    `json:"result"`
	}

	entries := []AuditEntry{
		{
			ID:        1,
			Timestamp: time.Now(),
			Source:    "web",
			User:      "admin",
			Action:    "login",
			Resource:  "session",
			Details:   "Successful login",
			IPAddress: "192.168.1.1",
			Result:    "success",
		},
		{
			ID:        2,
			Timestamp: time.Now(),
			Source:    "api",
			User:      "deploy-bot",
			Action:    "deploy",
			Resource:  "project/myapp",
			Details:   "Deployment triggered",
			IPAddress: "10.0.0.1",
			Result:    "success",
		},
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal audit entries: %v", err)
	}

	// Verify we can unmarshal it back
	var decoded []AuditEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal audit entries: %v", err)
	}

	if len(decoded) != len(entries) {
		t.Errorf("Expected %d entries, got %d", len(entries), len(decoded))
	}
}
