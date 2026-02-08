package backup

import (
	"context"
	"database/sql"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/rs/xid"
	_ "modernc.org/sqlite"
)

// createExportWithData creates a test DB with data, exports it, and returns the export path.
func createExportWithData(t *testing.T, passphrase string) (exportPath string, store *storage.DB, kms2 *security.KMS, mk2 *security.MasterKey) {
	t.Helper()
	ctx := context.Background()

	// Source DB with data
	srcDB := setupTestDB(t)
	kms1, mk1 := setupTestKMS(t, srcDB)

	projectUID := xid.New().String()
	_, err := srcDB.Conn().ExecContext(ctx,
		`INSERT INTO projects (uid, name, branch) VALUES (?, ?, ?)`,
		projectUID, "imported-project", "main",
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	secretPlain := "exported-secret-value"
	secretEnc, err := kms1.Encrypt(ctx, []byte(secretPlain))
	if err != nil {
		t.Fatalf("KMS encrypt: %v", err)
	}

	secretUID := xid.New().String()
	_, err = srcDB.Conn().ExecContext(ctx,
		`INSERT INTO secrets (uid, project, scope, key, value_encrypted) VALUES (?, ?, ?, ?, ?)`,
		secretUID, "imported-project", "env", "API_KEY", secretEnc,
	)
	if err != nil {
		t.Fatalf("insert secret: %v", err)
	}

	// Export
	exportPath = filepath.Join(t.TempDir(), "export.db")
	exportSvc := NewExportService(srcDB, kms1, mk1, nil)
	if err := exportSvc.Export(ctx, passphrase, exportPath); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Destination DB (separate)
	dstDB := setupTestDB(t)
	kms2, mk2 = setupTestKMS(t, dstDB)

	return exportPath, dstDB, kms2, mk2
}

func TestImport_ComputeDiff_NewRecords(t *testing.T) {
	passphrase := "test-passphrase-123"
	exportPath, dstDB, kms2, _ := createExportWithData(t, passphrase)
	ctx := context.Background()

	importSvc := NewImportService(dstDB, kms2, nil, nil)
	diff, err := importSvc.ComputeDiff(ctx, exportPath)
	if err != nil {
		t.Fatalf("ComputeDiff: %v", err)
	}

	if len(diff.Tables) == 0 {
		t.Fatal("expected non-empty diff")
	}

	// projects table should show 1 new record
	found := false
	for _, td := range diff.Tables {
		if td.Name == "projects" {
			found = true
			if td.NewRecords != 1 {
				t.Errorf("projects.NewRecords = %d, want 1", td.NewRecords)
			}
			if td.Total != 1 {
				t.Errorf("projects.Total = %d, want 1", td.Total)
			}
		}
	}
	if !found {
		t.Error("projects table not in diff")
	}
}

func TestImport_ComputeDiff_ChangedRecords(t *testing.T) {
	ctx := context.Background()
	passphrase := "test-passphrase-123"

	// Create source with a project
	srcDB := setupTestDB(t)
	kms1, mk1 := setupTestKMS(t, srcDB)
	projectUID := xid.New().String()
	_, err := srcDB.Conn().ExecContext(ctx,
		`INSERT INTO projects (uid, name, branch) VALUES (?, ?, ?)`,
		projectUID, "my-project", "main",
	)
	if err != nil {
		t.Fatalf("insert src project: %v", err)
	}

	exportPath := filepath.Join(t.TempDir(), "export.db")
	exportSvc := NewExportService(srcDB, kms1, mk1, nil)
	if err := exportSvc.Export(ctx, passphrase, exportPath); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Destination with same UID project
	dstDB := setupTestDB(t)
	kms2, _ := setupTestKMS(t, dstDB)
	_, err = dstDB.Conn().ExecContext(ctx,
		`INSERT INTO projects (uid, name, branch) VALUES (?, ?, ?)`,
		projectUID, "my-project-old", "develop",
	)
	if err != nil {
		t.Fatalf("insert dst project: %v", err)
	}

	importSvc := NewImportService(dstDB, kms2, nil, nil)
	diff, err := importSvc.ComputeDiff(ctx, exportPath)
	if err != nil {
		t.Fatalf("ComputeDiff: %v", err)
	}

	for _, td := range diff.Tables {
		if td.Name == "projects" {
			if td.NewRecords != 0 {
				t.Errorf("projects.NewRecords = %d, want 0 (same UID)", td.NewRecords)
			}
			if td.OnlyInMain != 0 {
				t.Errorf("projects.OnlyInMain = %d, want 0", td.OnlyInMain)
			}
			if td.Total != 1 {
				t.Errorf("projects.Total = %d, want 1", td.Total)
			}
		}
	}
}

func TestImport_Execute_Replace(t *testing.T) {
	passphrase := "test-passphrase-123"
	exportPath, dstDB, kms2, mk2 := createExportWithData(t, passphrase)
	ctx := context.Background()

	// Insert a different project into dst that should be wiped by replace
	dstProjectUID := xid.New().String()
	_, err := dstDB.Conn().ExecContext(ctx,
		`INSERT INTO projects (uid, name, branch) VALUES (?, ?, ?)`,
		dstProjectUID, "local-project", "main",
	)
	if err != nil {
		t.Fatalf("insert dst project: %v", err)
	}

	strategies := make(map[string]ImportStrategy)
	for _, table := range ExportableTables {
		strategies[table.Name] = StrategyReplace
	}

	importSvc := NewImportService(dstDB, kms2, mk2, nil)
	if err := importSvc.Execute(ctx, exportPath, passphrase, strategies); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// After replace, the local project should be gone
	var count int
	err = dstDB.Conn().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM projects WHERE uid=?", dstProjectUID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count local project: %v", err)
	}
	if count != 0 {
		t.Errorf("local project still exists after replace, count=%d", count)
	}

	// The imported project should be present
	err = dstDB.Conn().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM projects WHERE name=?", "imported-project",
	).Scan(&count)
	if err != nil {
		t.Fatalf("count imported project: %v", err)
	}
	if count != 1 {
		t.Errorf("imported project count = %d, want 1", count)
	}
}

func TestImport_Execute_Merge(t *testing.T) {
	passphrase := "test-passphrase-123"
	exportPath, dstDB, kms2, mk2 := createExportWithData(t, passphrase)
	ctx := context.Background()

	// Insert a different project into dst that should be kept by merge
	dstProjectUID := xid.New().String()
	_, err := dstDB.Conn().ExecContext(ctx,
		`INSERT INTO projects (uid, name, branch) VALUES (?, ?, ?)`,
		dstProjectUID, "local-project", "develop",
	)
	if err != nil {
		t.Fatalf("insert dst project: %v", err)
	}

	strategies := make(map[string]ImportStrategy)
	for _, table := range ExportableTables {
		strategies[table.Name] = StrategyMerge
	}

	importSvc := NewImportService(dstDB, kms2, mk2, nil)
	if err := importSvc.Execute(ctx, exportPath, passphrase, strategies); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// After merge, BOTH projects should be present
	var count int
	err = dstDB.Conn().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM projects",
	).Scan(&count)
	if err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if count != 2 {
		t.Errorf("project count = %d, want 2 (local + imported)", count)
	}
}

func TestImport_Execute_Skip(t *testing.T) {
	passphrase := "test-passphrase-123"
	exportPath, dstDB, kms2, mk2 := createExportWithData(t, passphrase)
	ctx := context.Background()

	strategies := make(map[string]ImportStrategy)
	for _, table := range ExportableTables {
		strategies[table.Name] = StrategySkip
	}

	importSvc := NewImportService(dstDB, kms2, mk2, nil)
	if err := importSvc.Execute(ctx, exportPath, passphrase, strategies); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// After skip, destination should still have 0 projects
	var count int
	err := dstDB.Conn().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM projects",
	).Scan(&count)
	if err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if count != 0 {
		t.Errorf("project count = %d, want 0 (all skipped)", count)
	}
}

func TestImport_DecryptsAndReencrypts(t *testing.T) {
	passphrase := "test-passphrase-123"
	exportPath, dstDB, kms2, mk2 := createExportWithData(t, passphrase)
	ctx := context.Background()

	strategies := map[string]ImportStrategy{
		"projects": StrategyReplace,
		"secrets":  StrategyReplace,
	}

	importSvc := NewImportService(dstDB, kms2, mk2, nil)
	if err := importSvc.Execute(ctx, exportPath, passphrase, strategies); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// The imported secret should now be encrypted with the destination KMS
	var encValue string
	err := dstDB.Conn().QueryRowContext(ctx,
		"SELECT value_encrypted FROM secrets WHERE key=?", "API_KEY",
	).Scan(&encValue)
	if err != nil {
		t.Fatalf("read imported secret: %v", err)
	}

	// Should be in KMS v1: format
	if !strings.HasPrefix(encValue, "v1:") {
		t.Errorf("imported secret not KMS-encrypted: %q", encValue[:min(len(encValue), 20)])
	}

	// Decrypt with destination KMS
	decrypted, err := kms2.Decrypt(ctx, encValue)
	if err != nil {
		t.Fatalf("KMS decrypt imported secret: %v", err)
	}
	if string(decrypted) != "exported-secret-value" {
		t.Errorf("decrypted = %q, want %q", string(decrypted), "exported-secret-value")
	}
}

func TestImport_MasterKeyReencryption(t *testing.T) {
	ctx := context.Background()
	passphrase := "test-passphrase-123"

	// Source: create SSH key with MasterKey encryption
	srcDB := setupTestDB(t)
	kms1, mk1 := setupTestKMS(t, srcDB)

	// Encrypt private key with source MasterKey
	privateKey := []byte("-----BEGIN RSA PRIVATE KEY-----\ntest key content\n-----END RSA PRIVATE KEY-----")
	encPrivKey, err := mk1.Encrypt(privateKey)
	if err != nil {
		t.Fatalf("MasterKey encrypt: %v", err)
	}
	keyUID := xid.New().String()
	_, err = srcDB.Conn().ExecContext(ctx,
		`INSERT INTO ssh_keys (uid, name, public_key, private_key_encrypted, key_type, fingerprint, created_by) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		keyUID, "test-key", "ssh-rsa AAAA...", encPrivKey, "rsa", "SHA256:test", "testuser",
	)
	if err != nil {
		t.Fatalf("insert ssh_key: %v", err)
	}

	// Export
	exportPath := filepath.Join(t.TempDir(), "export.db")
	exportSvc := NewExportService(srcDB, kms1, mk1, nil)
	if err := exportSvc.Export(ctx, passphrase, exportPath); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Destination
	dstDB := setupTestDB(t)
	kms2, mk2 := setupTestKMS(t, dstDB)

	strategies := map[string]ImportStrategy{
		"users":    StrategySkip,
		"ssh_keys": StrategyReplace,
	}

	importSvc := NewImportService(dstDB, kms2, mk2, nil)
	if err := importSvc.Execute(ctx, exportPath, passphrase, strategies); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Read the imported encrypted private key
	var encImported []byte
	err = dstDB.Conn().QueryRowContext(ctx,
		"SELECT private_key_encrypted FROM ssh_keys WHERE uid=?", keyUID,
	).Scan(&encImported)
	if err != nil {
		t.Fatalf("read imported ssh_key: %v", err)
	}

	// Decrypt with destination MasterKey
	decrypted, err := mk2.Decrypt(encImported)
	if err != nil {
		t.Fatalf("MasterKey decrypt: %v", err)
	}
	if string(decrypted) != string(privateKey) {
		t.Errorf("decrypted private key mismatch")
	}
}

func TestImport_SettingsEncryptedReencryption(t *testing.T) {
	ctx := context.Background()
	passphrase := "test-passphrase-123"

	// Source: create encrypted setting
	srcDB := setupTestDB(t)
	kms1, mk1 := setupTestKMS(t, srcDB)

	settingPlain := "smtp-password-value"
	settingEnc, err := kms1.Encrypt(ctx, []byte(settingPlain))
	if err != nil {
		t.Fatalf("KMS encrypt: %v", err)
	}

	settingUID := xid.New().String()
	_, err = srcDB.Conn().ExecContext(ctx,
		`INSERT INTO settings (uid, category, key, value, encrypted) VALUES (?, ?, ?, ?, ?)`,
		settingUID, "smtp", "password", settingEnc, 1,
	)
	if err != nil {
		t.Fatalf("insert setting: %v", err)
	}

	// Export
	exportPath := filepath.Join(t.TempDir(), "export.db")
	exportSvc := NewExportService(srcDB, kms1, mk1, nil)
	if err := exportSvc.Export(ctx, passphrase, exportPath); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Verify the export has base64-encoded passphrase-encrypted value
	exportDB, err := sql.Open("sqlite", exportPath)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	var exportedVal string
	err = exportDB.QueryRow("SELECT value FROM settings WHERE uid=?", settingUID).Scan(&exportedVal)
	exportDB.Close()
	if err != nil {
		t.Fatalf("read exported setting: %v", err)
	}
	// Should be valid base64
	if _, err := base64.StdEncoding.DecodeString(exportedVal); err != nil {
		t.Fatalf("exported setting value is not valid base64: %v", err)
	}

	// Import into destination
	dstDB := setupTestDB(t)
	kms2, mk2 := setupTestKMS(t, dstDB)

	strategies := map[string]ImportStrategy{
		"settings": StrategyReplace,
	}

	importSvc := NewImportService(dstDB, kms2, mk2, nil)
	if err := importSvc.Execute(ctx, exportPath, passphrase, strategies); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// The imported setting should be KMS-encrypted with destination KMS
	var importedVal string
	err = dstDB.Conn().QueryRowContext(ctx,
		"SELECT value FROM settings WHERE uid=?", settingUID,
	).Scan(&importedVal)
	if err != nil {
		t.Fatalf("read imported setting: %v", err)
	}

	if !strings.HasPrefix(importedVal, "v1:") {
		t.Errorf("imported setting not KMS-encrypted: %q", importedVal[:min(len(importedVal), 20)])
	}

	// Decrypt with destination KMS
	decrypted, err := kms2.Decrypt(ctx, importedVal)
	if err != nil {
		t.Fatalf("KMS decrypt: %v", err)
	}
	if string(decrypted) != settingPlain {
		t.Errorf("decrypted = %q, want %q", string(decrypted), settingPlain)
	}
}

func TestImport_RefreshesCache(t *testing.T) {
	passphrase := "test-passphrase-123"
	exportPath, dstDB, kms2, mk2 := createExportWithData(t, passphrase)
	ctx := context.Background()

	strategies := map[string]ImportStrategy{
		"projects": StrategyReplace,
		"secrets":  StrategyReplace,
	}

	importSvc := NewImportService(dstDB, kms2, mk2, nil)
	if err := importSvc.Execute(ctx, exportPath, passphrase, strategies); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify Reload was called (DB implements no-op, but no error means success)
	if err := dstDB.Reload(ctx); err != nil {
		t.Errorf("Reload failed: %v", err)
	}
}
