package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrateUp(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create fresh database - should run all migrations
	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	// Verify migrations were applied
	status, err := db.GetMigrationStatus()
	if err != nil {
		t.Fatalf("GetMigrationStatus() failed: %v", err)
	}

	// Should have at least the base migrations applied
	if len(status) == 0 {
		t.Error("Expected migrations to be registered")
	}

	// All migrations should be applied
	for _, s := range status {
		if !s.Applied {
			t.Errorf("Migration %d (%s) should be applied", s.Version, s.Description)
		}
	}
}

func TestMigrationStatus(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	status, err := db.GetMigrationStatus()
	if err != nil {
		t.Fatalf("GetMigrationStatus() failed: %v", err)
	}

	// Check that we have migrations
	if len(status) < 5 {
		t.Errorf("Expected at least 5 migrations, got %d", len(status))
	}

	// Check that status contains required fields
	for _, s := range status {
		if s.Version <= 0 {
			t.Errorf("Migration version should be positive, got %d", s.Version)
		}
		if s.Description == "" {
			t.Errorf("Migration %d should have a description", s.Version)
		}
	}
}

func TestMigrateTo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Get current version (should be fully migrated)
	status, _ := db.GetMigrationStatus()
	maxVersion := 0
	for _, s := range status {
		if s.Version > maxVersion && s.Applied {
			maxVersion = s.Version
		}
	}

	// Migrate to same version should be no-op
	if err := db.MigrateTo(ctx, maxVersion); err != nil {
		t.Errorf("MigrateTo(%d) failed: %v", maxVersion, err)
	}
}

func TestMigrateDown(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Get current state
	statusBefore, _ := db.GetMigrationStatus()
	appliedBefore := 0
	for _, s := range statusBefore {
		if s.Applied {
			appliedBefore++
		}
	}

	// Roll back one migration
	if err := db.MigrateDown(ctx, 1); err != nil {
		t.Fatalf("MigrateDown(1) failed: %v", err)
	}

	// Check that one migration was rolled back
	statusAfter, _ := db.GetMigrationStatus()
	appliedAfter := 0
	for _, s := range statusAfter {
		if s.Applied {
			appliedAfter++
		}
	}

	if appliedAfter != appliedBefore-1 {
		t.Errorf("Expected %d migrations applied after rollback, got %d", appliedBefore-1, appliedAfter)
	}

	// Migrate back up
	if err := db.MigrateUp(ctx); err != nil {
		t.Fatalf("MigrateUp() failed: %v", err)
	}

	// Should be back to original state
	statusFinal, _ := db.GetMigrationStatus()
	appliedFinal := 0
	for _, s := range statusFinal {
		if s.Applied {
			appliedFinal++
		}
	}

	if appliedFinal != appliedBefore {
		t.Errorf("Expected %d migrations applied after re-migrate, got %d", appliedBefore, appliedFinal)
	}
}

func TestMigrateDownInvalidSteps(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Should error with zero steps
	if err := db.MigrateDown(ctx, 0); err == nil {
		t.Error("MigrateDown(0) should fail")
	}

	// Should error with negative steps
	if err := db.MigrateDown(ctx, -1); err == nil {
		t.Error("MigrateDown(-1) should fail")
	}
}

func TestLegacyMigration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create database with New() - should handle fresh db
	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	db.Close()

	// Re-open - should detect existing migrations
	db2, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() second open failed: %v", err)
	}
	defer db2.Close()

	// Verify it didn't try to re-apply migrations
	status, err := db2.GetMigrationStatus()
	if err != nil {
		t.Fatalf("GetMigrationStatus() failed: %v", err)
	}

	// All should still be applied
	for _, s := range status {
		if !s.Applied {
			t.Errorf("Migration %d should still be applied", s.Version)
		}
	}
}

func TestNewTablesExist(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	// Check that all new tables from migrations exist
	tables := []string{
		"schema_migrations",
		"users",
		"sessions",
		"api_keys",
		"agents",
		"secrets",
		"deployments",
		"deployment_logs",
		"audit_logs",
		"webhook_secrets",
		"master_key_meta",
		"projects",
		"project_types",
		"ssh_keys",
		"known_hosts",
		"encryption_keys",
		"encryption_key_usage",
		"certificate_authorities",
		"agent_certificates",
		"rate_limits",
		"blocked_ips",
		"agent_binaries",
		"agent_provision_jobs",
		"acme_certificates",
		"acme_accounts",
	}

	for _, table := range tables {
		var count int
		err := db.conn.QueryRow(`
			SELECT COUNT(*) FROM sqlite_master 
			WHERE type='table' AND name=?
		`, table).Scan(&count)
		if err != nil {
			t.Errorf("Error checking table %s: %v", table, err)
			continue
		}
		if count != 1 {
			t.Errorf("Table %s should exist", table)
		}
	}
}

func TestMigrationVersionsAreUnique(t *testing.T) {
	versions := make(map[int]bool)
	for _, m := range migrations {
		if versions[m.Version] {
			t.Errorf("Duplicate migration version: %d", m.Version)
		}
		versions[m.Version] = true
	}
}

func TestMigrationVersionsAreSequential(t *testing.T) {
	if len(migrations) == 0 {
		t.Skip("No migrations to test")
	}

	// Collect versions
	var versions []int
	for _, m := range migrations {
		versions = append(versions, m.Version)
	}

	// Check sequential (allow gaps but start from 1)
	if versions[0] != 1 {
		t.Errorf("First migration should be version 1, got %d", versions[0])
	}
}

func TestMigrationsHaveDownFunctions(t *testing.T) {
	for _, m := range migrations {
		if m.Down == nil {
			t.Errorf("Migration %d (%s) has no Down function", m.Version, m.Description)
		}
	}
}

func TestConcurrentDatabaseAccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// First connection
	db1, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db1.Close()

	// Second connection to same database
	db2, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("Second New() failed: %v", err)
	}
	defer db2.Close()

	// Both should work
	status1, err := db1.GetMigrationStatus()
	if err != nil {
		t.Errorf("db1.GetMigrationStatus() failed: %v", err)
	}

	status2, err := db2.GetMigrationStatus()
	if err != nil {
		t.Errorf("db2.GetMigrationStatus() failed: %v", err)
	}

	if len(status1) != len(status2) {
		t.Errorf("Migration status mismatch: %d vs %d", len(status1), len(status2))
	}
}

func TestDatabaseBackupAfterMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupPath := filepath.Join(tmpDir, "backup.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Create backup
	if err := db.Backup(backupPath); err != nil {
		t.Fatalf("Backup failed: %v", err)
	}
	db.Close()

	// Open backup and verify migrations
	dbBackup, err := New(backupPath, nil)
	if err != nil {
		t.Fatalf("Open backup failed: %v", err)
	}
	defer dbBackup.Close()

	status, err := dbBackup.GetMigrationStatus()
	if err != nil {
		t.Fatalf("GetMigrationStatus on backup failed: %v", err)
	}

	// All migrations should be applied
	for _, s := range status {
		if !s.Applied {
			t.Errorf("Migration %d should be applied in backup", s.Version)
		}
	}
}
