package backup

import (
	"context"
	"database/sql"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/rs/xid"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *storage.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func setupTestKMS(t *testing.T, store storage.Store) (*security.KMS, *security.MasterKey) {
	t.Helper()
	mk, err := security.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey: %v", err)
	}
	ctx := context.Background()
	kms, err := security.NewKMS(ctx, store, nil, mk)
	if err != nil {
		t.Fatalf("NewKMS: %v", err)
	}
	// Initialize creates the first active encryption key
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("KMS Initialize: %v", err)
	}
	return kms, mk
}

func TestExport_IncludesAllImportableTables(t *testing.T) {
	db := setupTestDB(t)
	kms, mk := setupTestKMS(t, db)

	outputPath := filepath.Join(t.TempDir(), "export.db")
	svc := NewExportService(db, kms, mk, nil)

	if err := svc.Export(context.Background(), "testpassphrase123", outputPath); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	exportDB, err := sql.Open("sqlite", outputPath)
	if err != nil {
		t.Fatalf("open export db: %v", err)
	}
	defer exportDB.Close()

	for _, table := range ExportableTables {
		var name string
		err := exportDB.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table.Name,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found in export: %v", table.Name, err)
		}
	}

	var metaVal string
	err = exportDB.QueryRow("SELECT value FROM _export_meta WHERE key='format'").Scan(&metaVal)
	if err != nil {
		t.Fatalf("export metadata not found: %v", err)
	}
	if metaVal != "vcdeploy-export" {
		t.Errorf("export format = %q, want %q", metaVal, "vcdeploy-export")
	}
}

func TestExport_EncryptsSecrets(t *testing.T) {
	db := setupTestDB(t)
	kms, mk := setupTestKMS(t, db)
	ctx := context.Background()

	projectUID := xid.New().String()
	_, err := db.Conn().ExecContext(ctx,
		`INSERT INTO projects (uid, name, branch) VALUES (?, ?, ?)`,
		projectUID, "test-project", "main",
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	originalSecret := "super-secret-value-12345"
	encrypted, err := kms.Encrypt(ctx, []byte(originalSecret))
	if err != nil {
		t.Fatalf("KMS encrypt: %v", err)
	}

	secretUID := xid.New().String()
	_, err = db.Conn().ExecContext(ctx,
		`INSERT INTO secrets (uid, project, scope, key, value_encrypted) VALUES (?, ?, ?, ?, ?)`,
		secretUID, "test-project", "env", "API_KEY", encrypted,
	)
	if err != nil {
		t.Fatalf("insert secret: %v", err)
	}

	passphrase := "export-passphrase-123"
	outputPath := filepath.Join(t.TempDir(), "export.db")
	svc := NewExportService(db, kms, mk, nil)
	if err := svc.Export(ctx, passphrase, outputPath); err != nil {
		t.Fatalf("Export: %v", err)
	}

	exportDB, err := sql.Open("sqlite", outputPath)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer exportDB.Close()

	var exportedValue []byte
	err = exportDB.QueryRow("SELECT value_encrypted FROM secrets WHERE uid=?", secretUID).Scan(&exportedValue)
	if err != nil {
		t.Fatalf("read exported secret: %v", err)
	}

	if string(exportedValue) == encrypted {
		t.Error("exported secret is still KMS-encrypted, should be passphrase-encrypted")
	}
	if string(exportedValue) == originalSecret {
		t.Error("exported secret is plaintext, should be passphrase-encrypted")
	}
}

func TestExport_DecryptableWithPassphrase(t *testing.T) {
	db := setupTestDB(t)
	kms, mk := setupTestKMS(t, db)
	ctx := context.Background()

	projectUID := xid.New().String()
	_, err := db.Conn().ExecContext(ctx,
		`INSERT INTO projects (uid, name, branch) VALUES (?, ?, ?)`,
		projectUID, "test-project", "main",
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	originalSecret := "my-super-secret-value"
	encrypted, err := kms.Encrypt(ctx, []byte(originalSecret))
	if err != nil {
		t.Fatalf("KMS encrypt: %v", err)
	}

	secretUID := xid.New().String()
	_, err = db.Conn().ExecContext(ctx,
		`INSERT INTO secrets (uid, project, scope, key, value_encrypted) VALUES (?, ?, ?, ?, ?)`,
		secretUID, "test-project", "env", "DB_PASS", encrypted,
	)
	if err != nil {
		t.Fatalf("insert secret: %v", err)
	}

	passphrase := "my-export-passphrase"
	outputPath := filepath.Join(t.TempDir(), "export.db")
	svc := NewExportService(db, kms, mk, nil)
	if err := svc.Export(ctx, passphrase, outputPath); err != nil {
		t.Fatalf("Export: %v", err)
	}

	exportDB, err := sql.Open("sqlite", outputPath)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer exportDB.Close()

	var exportedValue []byte
	err = exportDB.QueryRow("SELECT value_encrypted FROM secrets WHERE uid=?", secretUID).Scan(&exportedValue)
	if err != nil {
		t.Fatalf("read exported secret: %v", err)
	}

	decrypted, err := security.DecryptWithPassphrase(exportedValue, []byte(passphrase))
	if err != nil {
		t.Fatalf("DecryptWithPassphrase: %v", err)
	}

	if string(decrypted) != originalSecret {
		t.Errorf("decrypted = %q, want %q", string(decrypted), originalSecret)
	}
}

func TestExport_NoKeyMaterial(t *testing.T) {
	db := setupTestDB(t)
	kms, mk := setupTestKMS(t, db)

	outputPath := filepath.Join(t.TempDir(), "export.db")
	svc := NewExportService(db, kms, mk, nil)
	if err := svc.Export(context.Background(), "testpassphrase123", outputPath); err != nil {
		t.Fatalf("Export: %v", err)
	}

	exportDB, err := sql.Open("sqlite", outputPath)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer exportDB.Close()

	var name string
	err = exportDB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='encryption_keys'",
	).Scan(&name)
	if err == nil {
		t.Error("encryption_keys table should NOT exist in export")
	}
}

func TestExport_NoMasterKeyMeta(t *testing.T) {
	db := setupTestDB(t)
	kms, mk := setupTestKMS(t, db)

	outputPath := filepath.Join(t.TempDir(), "export.db")
	svc := NewExportService(db, kms, mk, nil)
	if err := svc.Export(context.Background(), "testpassphrase123", outputPath); err != nil {
		t.Fatalf("Export: %v", err)
	}

	exportDB, err := sql.Open("sqlite", outputPath)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer exportDB.Close()

	var name string
	err = exportDB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='master_key_meta'",
	).Scan(&name)
	if err == nil {
		t.Error("master_key_meta table should NOT exist in export")
	}
}

func TestExport_PreservesUIDs(t *testing.T) {
	db := setupTestDB(t)
	kms, mk := setupTestKMS(t, db)
	ctx := context.Background()

	projectUID := xid.New().String()
	_, err := db.Conn().ExecContext(ctx,
		`INSERT INTO projects (uid, name, branch) VALUES (?, ?, ?)`,
		projectUID, "uid-test-project", "main",
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	userUID := xid.New().String()
	_, err = db.Conn().ExecContext(ctx,
		`INSERT INTO users (uid, username, password_hash, role) VALUES (?, ?, ?, ?)`,
		userUID, "testuser", "$2a$12$fakehash", "admin",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	outputPath := filepath.Join(t.TempDir(), "export.db")
	svc := NewExportService(db, kms, mk, nil)
	if err := svc.Export(ctx, "testpassphrase123", outputPath); err != nil {
		t.Fatalf("Export: %v", err)
	}

	exportDB, err := sql.Open("sqlite", outputPath)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer exportDB.Close()

	var exportedProjectUID string
	err = exportDB.QueryRow("SELECT uid FROM projects WHERE name='uid-test-project'").Scan(&exportedProjectUID)
	if err != nil {
		t.Fatalf("read exported project uid: %v", err)
	}
	if exportedProjectUID != projectUID {
		t.Errorf("project uid = %q, want %q", exportedProjectUID, projectUID)
	}

	var exportedUserUID string
	err = exportDB.QueryRow("SELECT uid FROM users WHERE username='testuser'").Scan(&exportedUserUID)
	if err != nil {
		t.Fatalf("read exported user uid: %v", err)
	}
	if exportedUserUID != userUID {
		t.Errorf("user uid = %q, want %q", exportedUserUID, userUID)
	}
}

func TestExport_EmptyTables(t *testing.T) {
	db := setupTestDB(t)
	kms, mk := setupTestKMS(t, db)

	outputPath := filepath.Join(t.TempDir(), "export.db")
	svc := NewExportService(db, kms, mk, nil)

	if err := svc.Export(context.Background(), "testpassphrase123", outputPath); err != nil {
		t.Fatalf("Export with empty tables: %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("stat export file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("export file is empty")
	}
}

func TestExport_MasterKeyEncryptedColumns(t *testing.T) {
	db := setupTestDB(t)
	kms, mk := setupTestKMS(t, db)
	ctx := context.Background()

	originalPrivateKey := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nfake-key-data\n-----END OPENSSH PRIVATE KEY-----")
	encryptedKey, err := mk.Encrypt(originalPrivateKey)
	if err != nil {
		t.Fatalf("MasterKey encrypt: %v", err)
	}

	sshUID := xid.New().String()
	_, err = db.Conn().ExecContext(ctx,
		`INSERT INTO ssh_keys (uid, name, public_key, private_key_encrypted, key_type, fingerprint) VALUES (?, ?, ?, ?, ?, ?)`,
		sshUID, "test-key", "ssh-ed25519 AAAA...", encryptedKey, "ed25519", "SHA256:fake",
	)
	if err != nil {
		t.Fatalf("insert ssh key: %v", err)
	}

	passphrase := "masterkey-test-pass"
	outputPath := filepath.Join(t.TempDir(), "export.db")
	svc := NewExportService(db, kms, mk, nil)
	if err := svc.Export(ctx, passphrase, outputPath); err != nil {
		t.Fatalf("Export: %v", err)
	}

	exportDB, err := sql.Open("sqlite", outputPath)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer exportDB.Close()

	var exportedKey []byte
	err = exportDB.QueryRow("SELECT private_key_encrypted FROM ssh_keys WHERE uid=?", sshUID).Scan(&exportedKey)
	if err != nil {
		t.Fatalf("read exported ssh key: %v", err)
	}

	if string(exportedKey) == string(encryptedKey) {
		t.Error("exported private key is still MasterKey-encrypted")
	}

	decrypted, err := security.DecryptWithPassphrase(exportedKey, []byte(passphrase))
	if err != nil {
		t.Fatalf("DecryptWithPassphrase: %v", err)
	}

	if string(decrypted) != string(originalPrivateKey) {
		t.Errorf("decrypted key doesn't match original")
	}
}

func TestExport_SettingsEncryptedFlag(t *testing.T) {
	db := setupTestDB(t)
	kms, mk := setupTestKMS(t, db)
	ctx := context.Background()

	plainUID := xid.New().String()
	_, err := db.Conn().ExecContext(ctx,
		`INSERT INTO settings (uid, category, key, value, value_type, encrypted) VALUES (?, ?, ?, ?, ?, ?)`,
		plainUID, "appearance", "theme", "dark", "string", 0,
	)
	if err != nil {
		t.Fatalf("insert plain setting: %v", err)
	}

	originalValue := "smtp-password-123"
	encryptedValue, err := kms.Encrypt(ctx, []byte(originalValue))
	if err != nil {
		t.Fatalf("KMS encrypt: %v", err)
	}

	encUID := xid.New().String()
	_, err = db.Conn().ExecContext(ctx,
		`INSERT INTO settings (uid, category, key, value, value_type, encrypted) VALUES (?, ?, ?, ?, ?, ?)`,
		encUID, "notifications", "smtp_password", encryptedValue, "string", 1,
	)
	if err != nil {
		t.Fatalf("insert encrypted setting: %v", err)
	}

	passphrase := "settings-test-pass"
	outputPath := filepath.Join(t.TempDir(), "export.db")
	svc := NewExportService(db, kms, mk, nil)
	if err := svc.Export(ctx, passphrase, outputPath); err != nil {
		t.Fatalf("Export: %v", err)
	}

	exportDB, err := sql.Open("sqlite", outputPath)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer exportDB.Close()

	var plainValue string
	err = exportDB.QueryRow("SELECT value FROM settings WHERE uid=?", plainUID).Scan(&plainValue)
	if err != nil {
		t.Fatalf("read plain setting: %v", err)
	}
	if plainValue != "dark" {
		t.Errorf("plain setting value = %q, want %q", plainValue, "dark")
	}

	var encExportedValue string
	err = exportDB.QueryRow("SELECT value FROM settings WHERE uid=?", encUID).Scan(&encExportedValue)
	if err != nil {
		t.Fatalf("read encrypted setting: %v", err)
	}

	if encExportedValue == encryptedValue {
		t.Error("encrypted setting still has KMS ciphertext")
	}

	encBytes, err := base64.StdEncoding.DecodeString(encExportedValue)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}

	decrypted, err := security.DecryptWithPassphrase(encBytes, []byte(passphrase))
	if err != nil {
		t.Fatalf("DecryptWithPassphrase: %v", err)
	}

	if string(decrypted) != originalValue {
		t.Errorf("decrypted setting = %q, want %q", string(decrypted), originalValue)
	}
}
