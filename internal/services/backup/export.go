// Package backup provides export and import services for vcdeploy backups.
package backup

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
	_ "modernc.org/sqlite" // sqlite driver
)

// ExportService handles creating passphrase-protected export bundles.
type ExportService struct {
	store     storage.Store
	kms       *security.KMS
	masterKey *security.MasterKey
	logger    *zap.Logger
}

// NewExportService creates a new ExportService.
func NewExportService(store storage.Store, kms *security.KMS, masterKey *security.MasterKey, logger *zap.Logger) *ExportService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ExportService{
		store:     store,
		kms:       kms,
		masterKey: masterKey,
		logger:    logger,
	}
}

// Export creates a passphrase-protected export file at outputPath.
// Sensitive fields are decrypted from KMS/MasterKey and re-encrypted with the passphrase.
// The export file is a standalone SQLite database containing only importable tables.
func (s *ExportService) Export(ctx context.Context, passphrase string, outputPath string) error {
	if passphrase == "" {
		return fmt.Errorf("passphrase is required")
	}

	// Remove existing export file if present
	_ = os.Remove(outputPath) // #nosec G104 - best effort cleanup

	// Open source DB connection
	srcConn := s.store.Conn()
	if srcConn == nil {
		return fmt.Errorf("store does not expose a database connection")
	}

	// Create export database
	exportDB, err := sql.Open("sqlite", outputPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("create export database: %w", err)
	}
	defer exportDB.Close()

	// Enable foreign keys on export DB
	if _, err := exportDB.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}

	// Create schema for each exportable table
	for _, table := range ExportableTables {
		ddl, err := getTableDDL(ctx, srcConn, table.Name)
		if err != nil {
			return fmt.Errorf("get DDL for table %s: %w", table.Name, err)
		}
		if _, err := exportDB.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("create table %s in export: %w", table.Name, err)
		}
	}

	// Export each table
	for _, table := range ExportableTables {
		if err := s.exportTable(ctx, srcConn, exportDB, table, passphrase); err != nil {
			return fmt.Errorf("export table %s: %w", table.Name, err)
		}
	}

	// Add export metadata
	if _, err := exportDB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS _export_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create export metadata table: %w", err)
	}
	if _, err := exportDB.ExecContext(ctx,
		`INSERT INTO _export_meta (key, value) VALUES ('version', '1'), ('format', 'vcdeploy-export')`,
	); err != nil {
		return fmt.Errorf("insert export metadata: %w", err)
	}

	s.logger.Info("export completed", zap.String("output", outputPath))
	return nil
}

// getTableDDL retrieves the CREATE TABLE DDL from the source database.
func getTableDDL(ctx context.Context, db *sql.DB, tableName string) (string, error) {
	var ddl string
	err := db.QueryRowContext(ctx,
		"SELECT sql FROM sqlite_master WHERE type='table' AND name=?", tableName,
	).Scan(&ddl)
	if err != nil {
		return "", fmt.Errorf("table %s not found in source: %w", tableName, err)
	}
	// The DDL from sqlite_master is a CREATE TABLE statement but without IF NOT EXISTS.
	// Add it for safety.
	ddl = strings.Replace(ddl, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", 1)
	return ddl, nil
}

// exportTable copies data from source to export DB, re-encrypting sensitive columns.
func (s *ExportService) exportTable(ctx context.Context, srcDB *sql.DB, exportDB *sql.DB, table TableExportConfig, passphrase string) error {
	// Get column names
	columns, err := getTableColumns(ctx, srcDB, table.Name)
	if err != nil {
		return fmt.Errorf("get columns for %s: %w", table.Name, err)
	}

	// SELECT all rows from source
	query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(quoteColumns(columns), ", "), table.Name) // #nosec G201 - table names from trusted config
	rows, err := srcDB.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query %s: %w", table.Name, err)
	}
	defer rows.Close()

	// Prepare INSERT statement
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", // #nosec G201 - table/column names from trusted config
		table.Name,
		strings.Join(quoteColumns(columns), ", "),
		strings.Join(placeholders, ", "),
	)
	stmt, err := exportDB.PrepareContext(ctx, insertSQL)
	if err != nil {
		return fmt.Errorf("prepare insert for %s: %w", table.Name, err)
	}
	defer stmt.Close()

	// Build column index map for encrypted column lookup
	colIndex := make(map[string]int)
	for i, col := range columns {
		colIndex[col] = i
	}

	// Process rows
	rowCount := 0
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return fmt.Errorf("scan row from %s: %w", table.Name, err)
		}

		// Re-encrypt sensitive columns with passphrase
		if err := s.reencryptRow(ctx, table, columns, values, passphrase); err != nil {
			return fmt.Errorf("re-encrypt row in %s: %w", table.Name, err)
		}

		if _, err := stmt.ExecContext(ctx, values...); err != nil {
			return fmt.Errorf("insert row into export %s: %w", table.Name, err)
		}
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate %s rows: %w", table.Name, err)
	}

	s.logger.Debug("exported table", zap.String("table", table.Name), zap.Int("rows", rowCount))
	return nil
}

// reencryptRow decrypts encrypted columns and re-encrypts them with the passphrase.
func (s *ExportService) reencryptRow(ctx context.Context, table TableExportConfig, columns []string, values []interface{}, passphrase string) error {
	// Handle explicit encrypted columns
	for colName, encType := range table.EncryptedColumns {
		idx := -1
		for i, c := range columns {
			if c == colName {
				idx = i
				break
			}
		}
		if idx == -1 {
			continue
		}

		// Skip NULL values
		if values[idx] == nil {
			continue
		}

		plaintext, err := s.decryptValue(ctx, values[idx], encType)
		if err != nil {
			// Skip values that can't be decrypted (might be empty or invalid)
			s.logger.Warn("failed to decrypt column, preserving as-is",
				zap.String("table", table.Name),
				zap.String("column", colName),
				zap.Error(err),
			)
			continue
		}

		// Re-encrypt with passphrase
		encrypted, err := security.EncryptWithPassphrase(plaintext, []byte(passphrase))
		if err != nil {
			return fmt.Errorf("encrypt %s.%s with passphrase: %w", table.Name, colName, err)
		}
		values[idx] = encrypted
	}

	// Special handling for settings table: check encrypted flag per row
	if table.Name == "settings" {
		if err := s.reencryptSettingsRow(ctx, columns, values, passphrase); err != nil {
			return err
		}
	}

	return nil
}

// reencryptSettingsRow handles the special case where settings.value is conditionally encrypted.
func (s *ExportService) reencryptSettingsRow(ctx context.Context, columns []string, values []interface{}, passphrase string) error {
	// Find column indices
	var valueIdx, encryptedIdx int = -1, -1
	for i, col := range columns {
		switch col {
		case "value":
			valueIdx = i
		case "encrypted":
			encryptedIdx = i
		}
	}

	if valueIdx == -1 || encryptedIdx == -1 {
		return nil // columns not found, skip
	}

	// Check if this setting is encrypted
	isEncrypted := false
	switch v := values[encryptedIdx].(type) {
	case int64:
		isEncrypted = v == 1
	case int:
		isEncrypted = v == 1
	case bool:
		isEncrypted = v
	}

	if !isEncrypted || values[valueIdx] == nil {
		return nil
	}

	// Decrypt the KMS-encrypted value
	valueStr, ok := values[valueIdx].(string)
	if !ok {
		return nil
	}

	plaintext, err := s.kms.Decrypt(ctx, valueStr)
	if err != nil {
		s.logger.Warn("failed to decrypt settings value, preserving as-is", zap.Error(err))
		return nil
	}

	// Re-encrypt with passphrase and base64-encode for TEXT column
	encrypted, err := security.EncryptWithPassphrase(plaintext, []byte(passphrase))
	if err != nil {
		return fmt.Errorf("encrypt settings value with passphrase: %w", err)
	}
	values[valueIdx] = base64.StdEncoding.EncodeToString(encrypted)

	return nil
}

// decryptValue decrypts a value based on its encryption type.
func (s *ExportService) decryptValue(ctx context.Context, value interface{}, encType string) ([]byte, error) {
	switch encType {
	case "kms":
		// KMS values are stored as strings in v1:{key_id}:{nonce}:{ct} format
		strVal, ok := value.(string)
		if !ok {
			// Might be []byte for BLOB columns
			if blobVal, ok := value.([]byte); ok {
				strVal = string(blobVal)
			} else {
				return nil, fmt.Errorf("unexpected value type for KMS column: %T", value)
			}
		}
		if s.kms == nil {
			return nil, fmt.Errorf("KMS not available for decryption")
		}
		return s.kms.Decrypt(ctx, strVal)

	case "masterkey":
		// MasterKey values are stored as BLOB (binary nonce‖ciphertext)
		var blobVal []byte
		switch v := value.(type) {
		case []byte:
			blobVal = v
		case string:
			blobVal = []byte(v)
		default:
			return nil, fmt.Errorf("unexpected value type for masterkey column: %T", value)
		}
		if s.masterKey == nil {
			return nil, fmt.Errorf("MasterKey not available for decryption")
		}
		return s.masterKey.Decrypt(blobVal)

	default:
		return nil, fmt.Errorf("unknown encryption type: %s", encType)
	}
}

// getTableColumns returns the column names for a table.
func getTableColumns(ctx context.Context, db *sql.DB, tableName string) ([]string, error) {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName) // #nosec G201 - trusted table name
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue *string
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

// quoteColumns wraps column names in double-quotes for SQL safety.
func quoteColumns(columns []string) []string {
	quoted := make([]string, len(columns))
	for i, col := range columns {
		quoted[i] = `"` + col + `"`
	}
	return quoted
}
