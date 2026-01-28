// Package scheduler provides job implementations for scheduled tasks.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// --- Key Rotation Job ---

// KeyRotationJob rotates encryption keys on a schedule.
type KeyRotationJob struct {
	kms    *security.KMS
	config *config.KeyRotationConfig
	logger *zap.Logger
}

// NewKeyRotationJob creates a new key rotation job.
func NewKeyRotationJob(kms *security.KMS, cfg *config.KeyRotationConfig, logger *zap.Logger) *KeyRotationJob {
	return &KeyRotationJob{
		kms:    kms,
		config: cfg,
		logger: logger,
	}
}

// Name returns the job name.
func (j *KeyRotationJob) Name() string {
	return "key-rotation"
}

// Enabled returns whether the job should run.
func (j *KeyRotationJob) Enabled() bool {
	return j.config != nil && j.config.Enabled
}

// Run executes the key rotation.
func (j *KeyRotationJob) Run(ctx context.Context) error {
	if j.kms == nil {
		return fmt.Errorf("KMS not initialized")
	}

	j.logger.Info("Starting key rotation")

	newKey, err := j.kms.RotateKey(ctx)
	if err != nil {
		return fmt.Errorf("rotating key: %w", err)
	}

	j.logger.Info("Key rotation completed",
		zap.String("keyID", newKey.ID),
		zap.Int("version", newKey.Version),
	)

	return nil
}

// --- Database Backup Job ---

// DatabaseBackupJob backs up the database on a schedule.
type DatabaseBackupJob struct {
	db     *storage.DB
	config *config.DatabaseBackupConfig
	logger *zap.Logger
}

// NewDatabaseBackupJob creates a new database backup job.
func NewDatabaseBackupJob(db *storage.DB, cfg *config.DatabaseBackupConfig, logger *zap.Logger) *DatabaseBackupJob {
	return &DatabaseBackupJob{
		db:     db,
		config: cfg,
		logger: logger,
	}
}

// Name returns the job name.
func (j *DatabaseBackupJob) Name() string {
	return "database-backup"
}

// Enabled returns whether the job should run.
func (j *DatabaseBackupJob) Enabled() bool {
	return j.config != nil && j.config.Enabled
}

// Run executes the database backup.
func (j *DatabaseBackupJob) Run(ctx context.Context) error {
	if j.db == nil {
		return fmt.Errorf("database not initialized")
	}

	backupDir := j.config.Path
	if backupDir == "" {
		backupDir = "/var/lib/vcdeploy/backups"
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return fmt.Errorf("creating backup directory: %w", err)
	}

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("vcdeploy-backup-%s.db", timestamp))

	j.logger.Info("Starting database backup", zap.String("path", backupPath))

	if err := j.db.Backup(backupPath); err != nil {
		return fmt.Errorf("backing up database: %w", err)
	}

	j.logger.Info("Database backup completed", zap.String("path", backupPath))

	// Clean up old backups if retention is configured
	if j.config.Retention > 0 {
		if err := j.cleanOldBackups(backupDir); err != nil {
			j.logger.Warn("Failed to clean old backups", zap.Error(err))
		}
	}

	return nil
}

func (j *DatabaseBackupJob) cleanOldBackups(dir string) error {
	cutoff := time.Now().Add(-j.config.Retention)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading backup directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "vcdeploy-backup-") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			j.logger.Warn("Failed to get file info", zap.String("file", entry.Name()), zap.Error(err))
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, entry.Name())
			if err := os.Remove(path); err != nil {
				j.logger.Warn("Failed to remove old backup", zap.String("path", path), zap.Error(err))
			} else {
				j.logger.Info("Removed old backup", zap.String("path", path))
			}
		}
	}

	return nil
}

// --- Audit Export Job ---

// AuditExportJob exports audit logs on a schedule.
type AuditExportJob struct {
	db     *storage.DB
	config *config.AuditExportConfig
	logger *zap.Logger
}

// NewAuditExportJob creates a new audit export job.
func NewAuditExportJob(db *storage.DB, cfg *config.AuditExportConfig, logger *zap.Logger) *AuditExportJob {
	return &AuditExportJob{
		db:     db,
		config: cfg,
		logger: logger,
	}
}

// Name returns the job name.
func (j *AuditExportJob) Name() string {
	return "audit-export"
}

// Enabled returns whether the job should run.
func (j *AuditExportJob) Enabled() bool {
	return j.config != nil && j.config.Enabled
}

// Run executes the audit log export.
func (j *AuditExportJob) Run(ctx context.Context) error {
	if j.db == nil {
		return fmt.Errorf("database not initialized")
	}

	dest := j.config.Destination
	if dest == "" {
		dest = "/var/lib/vcdeploy/exports/audit"
	}

	// Ensure export directory exists
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return fmt.Errorf("creating export directory: %w", err)
	}

	// Generate export filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	exportPath := filepath.Join(dest, fmt.Sprintf("audit-export-%s.json", timestamp))

	j.logger.Info("Starting audit log export", zap.String("path", exportPath))

	// Export audit logs from the last 24 hours
	since := time.Now().Add(-24 * time.Hour)
	entries, err := j.db.ListAuditLogsSince(ctx, since)
	if err != nil {
		return fmt.Errorf("listing audit logs: %w", err)
	}

	// Write to file
	file, err := os.Create(exportPath)
	if err != nil {
		return fmt.Errorf("creating export file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(entries); err != nil {
		return fmt.Errorf("encoding audit logs: %w", err)
	}

	j.logger.Info("Audit log export completed",
		zap.String("path", exportPath),
		zap.Int("entries", len(entries)),
	)

	return nil
}

// --- Log Rotation Job ---

// LogRotationJob rotates application log files on a schedule.
type LogRotationJob struct {
	logDir    string
	maxSizeMB int
	retention time.Duration
	logger    *zap.Logger
	enabled   bool
}

// LogRotationConfig holds configuration for the log rotation job.
type LogRotationConfig struct {
	Enabled   bool
	LogDir    string
	MaxSizeMB int
	Retention time.Duration
}

// NewLogRotationJob creates a new log rotation job.
func NewLogRotationJob(cfg LogRotationConfig, logger *zap.Logger) *LogRotationJob {
	logDir := cfg.LogDir
	if logDir == "" {
		logDir = "/var/log/vcdeploy"
	}

	maxSize := cfg.MaxSizeMB
	if maxSize <= 0 {
		maxSize = 100 // 100MB default
	}

	retention := cfg.Retention
	if retention <= 0 {
		retention = 7 * 24 * time.Hour // 7 days default
	}

	return &LogRotationJob{
		logDir:    logDir,
		maxSizeMB: maxSize,
		retention: retention,
		logger:    logger,
		enabled:   cfg.Enabled,
	}
}

// Name returns the job name.
func (j *LogRotationJob) Name() string {
	return "log-rotation"
}

// Enabled returns whether the job should run.
func (j *LogRotationJob) Enabled() bool {
	return j.enabled
}

// Run executes the log rotation.
func (j *LogRotationJob) Run(ctx context.Context) error {
	j.logger.Info("Starting log rotation", zap.String("logDir", j.logDir))

	// Ensure log directory exists
	if _, err := os.Stat(j.logDir); os.IsNotExist(err) {
		j.logger.Debug("Log directory does not exist, skipping rotation", zap.String("logDir", j.logDir))
		return nil
	}

	entries, err := os.ReadDir(j.logDir)
	if err != nil {
		return fmt.Errorf("reading log directory: %w", err)
	}

	var rotated, deleted int
	cutoff := time.Now().Add(-j.retention)
	maxBytes := int64(j.maxSizeMB) * 1024 * 1024

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}

		path := filepath.Join(j.logDir, name)
		info, err := entry.Info()
		if err != nil {
			j.logger.Warn("Failed to get file info", zap.String("file", name), zap.Error(err))
			continue
		}

		// Delete old rotated logs
		if strings.Contains(name, ".rotated.") && info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				j.logger.Warn("Failed to delete old log", zap.String("path", path), zap.Error(err))
			} else {
				j.logger.Debug("Deleted old log", zap.String("path", path))
				deleted++
			}
			continue
		}

		// Rotate large logs
		if info.Size() > maxBytes && !strings.Contains(name, ".rotated.") {
			timestamp := time.Now().Format("20060102-150405")
			newName := strings.TrimSuffix(name, ".log") + ".rotated." + timestamp + ".log"
			newPath := filepath.Join(j.logDir, newName)

			if err := j.rotateFile(path, newPath); err != nil {
				j.logger.Warn("Failed to rotate log", zap.String("path", path), zap.Error(err))
			} else {
				j.logger.Info("Rotated log file",
					zap.String("from", path),
					zap.String("to", newPath),
					zap.Int64("sizeMB", info.Size()/(1024*1024)),
				)
				rotated++
			}
		}
	}

	j.logger.Info("Log rotation completed",
		zap.Int("rotated", rotated),
		zap.Int("deleted", deleted),
	)

	return nil
}

func (j *LogRotationJob) rotateFile(src, dst string) error {
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer srcFile.Close()

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating destination: %w", err)
	}
	defer dstFile.Close()

	// Copy content
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copying content: %w", err)
	}

	// Close files before truncating
	srcFile.Close()
	dstFile.Close()

	// Truncate original file
	if err := os.Truncate(src, 0); err != nil {
		return fmt.Errorf("truncating source: %w", err)
	}

	return nil
}

// --- Backup Cleanup Job ---

// BackupCleanupJob cleans up old backups beyond retention.
type BackupCleanupJob struct {
	backupDir  string
	retention  time.Duration
	maxBackups int
	logger     *zap.Logger
	enabled    bool
}

// BackupCleanupConfig holds configuration for backup cleanup.
type BackupCleanupConfig struct {
	Enabled    bool
	BackupDir  string
	Retention  time.Duration
	MaxBackups int
}

// NewBackupCleanupJob creates a new backup cleanup job.
func NewBackupCleanupJob(cfg BackupCleanupConfig, logger *zap.Logger) *BackupCleanupJob {
	return &BackupCleanupJob{
		backupDir:  cfg.BackupDir,
		retention:  cfg.Retention,
		maxBackups: cfg.MaxBackups,
		logger:     logger,
		enabled:    cfg.Enabled,
	}
}

// Name returns the job name.
func (j *BackupCleanupJob) Name() string {
	return "backup-cleanup"
}

// Enabled returns whether the job should run.
func (j *BackupCleanupJob) Enabled() bool {
	return j.enabled
}

// Run executes the backup cleanup.
func (j *BackupCleanupJob) Run(ctx context.Context) error {
	if j.backupDir == "" {
		return nil
	}

	j.logger.Info("Starting backup cleanup", zap.String("backupDir", j.backupDir))

	entries, err := os.ReadDir(j.backupDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading backup directory: %w", err)
	}

	type backupFile struct {
		path    string
		modTime time.Time
	}

	var backups []backupFile
	cutoff := time.Now().Add(-j.retention)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		path := filepath.Join(j.backupDir, entry.Name())
		backups = append(backups, backupFile{path: path, modTime: info.ModTime()})
	}

	// Sort by modification time (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modTime.After(backups[j].modTime)
	})

	var deleted int
	for i, backup := range backups {
		// Delete if beyond max count or beyond retention
		shouldDelete := false
		if j.maxBackups > 0 && i >= j.maxBackups {
			shouldDelete = true
		}
		if j.retention > 0 && backup.modTime.Before(cutoff) {
			shouldDelete = true
		}

		if shouldDelete {
			if err := os.Remove(backup.path); err != nil {
				j.logger.Warn("Failed to delete old backup", zap.String("path", backup.path), zap.Error(err))
			} else {
				j.logger.Debug("Deleted old backup", zap.String("path", backup.path))
				deleted++
			}
		}
	}

	j.logger.Info("Backup cleanup completed", zap.Int("deleted", deleted))
	return nil
}
