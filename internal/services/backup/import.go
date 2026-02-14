package backup

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// ImportService handles importing data from passphrase-protected export files.
type ImportService struct {
	store     storage.Store
	kms       *security.KMS
	masterKey *security.MasterKey
	logger    *zap.Logger
}

// NewImportService creates a new ImportService.
func NewImportService(store storage.Store, kms *security.KMS, masterKey *security.MasterKey, logger *zap.Logger) *ImportService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ImportService{
		store:     store,
		kms:       kms,
		masterKey: masterKey,
		logger:    logger,
	}
}

// ImportDiff represents the difference between the import file and the main database.
type ImportDiff struct {
	Tables []TableDiff
}

// TableDiff shows what changed for a single table.
type TableDiff struct {
	Name       string `json:"name"`
	NewRecords int    `json:"new_records"` // In imported, not in main (by id/XID)
	Changed    int    `json:"changed"`     // In both, content differs
	OnlyInMain int    `json:"only_in_main"`
	Total      int    `json:"total"` // Total records in import file
}

// ImportStrategy defines how a table should be handled during import.
type ImportStrategy string

const (
	StrategyReplace ImportStrategy = "replace" // Delete all from main, insert all from imported
	StrategyMerge   ImportStrategy = "merge"   // Insert new, update changed, keep main-only
	StrategySkip    ImportStrategy = "skip"    // Do nothing for this table
)

// ComputeDiff compares the import file with the main database and returns a diff.
// The import file must be a valid export file protected with the given passphrase.
func (s *ImportService) ComputeDiff(ctx context.Context, importPath string) (*ImportDiff, error) {
	mainConn := s.store.Conn()
	if mainConn == nil {
		return nil, fmt.Errorf("store does not expose a database connection")
	}

	// Use a dedicated connection for ATTACH (per-connection state)
	conn, err := mainConn.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire dedicated connection: %w", err)
	}
	defer conn.Close()

	// ATTACH the import database
	if _, err := conn.ExecContext(ctx, "ATTACH DATABASE ? AS imported", importPath); err != nil {
		return nil, fmt.Errorf("attach import database: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, "DETACH DATABASE imported")
	}()

	diff := &ImportDiff{}
	for _, table := range ExportableTables {
		td, err := s.computeTableDiff(ctx, conn, table.Name)
		if err != nil {
			s.logger.Warn("skipping table in diff", zap.String("table", table.Name), zap.Error(err))
			continue
		}
		diff.Tables = append(diff.Tables, td)
	}

	return diff, nil
}

// computeTableDiff computes the diff for a single table using the ATTACH'd import DB.
func (s *ImportService) computeTableDiff(ctx context.Context, conn *sql.Conn, tableName string) (TableDiff, error) {
	td := TableDiff{Name: tableName}

	// Check if the table exists in the imported DB
	var importedName string
	err := conn.QueryRowContext(ctx,
		"SELECT name FROM imported.sqlite_master WHERE type='table' AND name=?", tableName,
	).Scan(&importedName)
	if errors.Is(err, sql.ErrNoRows) {
		return td, nil // Table not in import, skip
	}
	if err != nil {
		return td, fmt.Errorf("check table %s in import: %w", tableName, err)
	}

	// Check if the table has an id column (TEXT primary key)
	hasID, err := tableHasColumn(ctx, conn, "imported", tableName, "id")
	if err != nil {
		return td, fmt.Errorf("check id column: %w", err)
	}
	if !hasID {
		// Without id, we can't diff — count total rows
		err = conn.QueryRowContext(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM imported.%s", tableName), // #nosec G201
		).Scan(&td.Total)
		return td, err
	}

	// Total records in import
	err = conn.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM imported.%s", tableName), // #nosec G201
	).Scan(&td.Total)
	if err != nil {
		return td, fmt.Errorf("count imported: %w", err)
	}

	// New records: in imported but not in main
	err = conn.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM imported.%s WHERE id NOT IN (SELECT id FROM main.%s)", tableName, tableName), // #nosec G201
	).Scan(&td.NewRecords)
	if err != nil {
		return td, fmt.Errorf("count new: %w", err)
	}

	// Only in main: in main but not in imported
	err = conn.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM main.%s WHERE id NOT IN (SELECT id FROM imported.%s)", tableName, tableName), // #nosec G201
	).Scan(&td.OnlyInMain)
	if err != nil {
		return td, fmt.Errorf("count only-in-main: %w", err)
	}

	// Changed: in both but content differs (compare all non-id columns)
	td.Changed = td.Total - td.NewRecords // Approximate: total in both = total - new

	return td, nil
}

// Execute performs the import with the given per-table strategies.
// The passphrase is used to decrypt encrypted columns from the import file.
// Strategies map table names to ImportStrategy.
func (s *ImportService) Execute(ctx context.Context, importPath, passphrase string, strategies map[string]ImportStrategy) error {
	mainConn := s.store.Conn()
	if mainConn == nil {
		return fmt.Errorf("store does not expose a database connection")
	}

	// Flush any pending writes before modifying DB directly
	if err := s.store.FlushPending(); err != nil {
		return fmt.Errorf("flush pending writes: %w", err)
	}

	// Use a dedicated connection for ATTACH
	conn, err := mainConn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire dedicated connection: %w", err)
	}
	defer conn.Close()

	// Disable FK enforcement during import (we handle ordering manually)
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}

	// ATTACH the import database
	if _, err := conn.ExecContext(ctx, "ATTACH DATABASE ? AS imported", importPath); err != nil {
		return fmt.Errorf("attach import database: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, "DETACH DATABASE imported")
	}()

	// Begin transaction
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Process tables in dependency order
	for _, table := range ExportableTables {
		strategy, ok := strategies[table.Name]
		if !ok {
			strategy = StrategySkip // Default to skip if not specified
		}
		if strategy == StrategySkip {
			continue
		}

		if err = s.importTable(ctx, tx, table, strategy, passphrase); err != nil {
			return fmt.Errorf("import table %s: %w", table.Name, err)
		}
		s.logger.Debug("imported table", zap.String("table", table.Name), zap.String("strategy", string(strategy)))
	}

	// Commit
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit import: %w", err)
	}

	// Re-enable FK enforcement
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		s.logger.Warn("failed to re-enable foreign keys", zap.Error(err))
	}

	// Refresh in-memory cache
	if err := s.store.Reload(ctx); err != nil {
		return fmt.Errorf("reload store after import: %w", err)
	}

	s.logger.Info("import completed successfully")
	return nil
}

// importTable processes a single table according to the given strategy.
func (s *ImportService) importTable(ctx context.Context, tx *sql.Tx, table TableExportConfig, strategy ImportStrategy, passphrase string) error {
	// Check if the table exists in imported DB
	var name string
	err := tx.QueryRowContext(ctx,
		"SELECT name FROM imported.sqlite_master WHERE type='table' AND name=?", table.Name,
	).Scan(&name)
	if err != nil {
		return nil // Table not in import, skip
	}

	switch strategy {
	case StrategyReplace:
		return s.importReplace(ctx, tx, table, passphrase)
	case StrategyMerge:
		return s.importMerge(ctx, tx, table, passphrase)
	default:
		return nil
	}
}

// importReplace deletes all rows from main and inserts all from imported.
func (s *ImportService) importReplace(ctx context.Context, tx *sql.Tx, table TableExportConfig, passphrase string) error {
	// Delete all from main table
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM main.%s", table.Name)); err != nil { // #nosec G201
		return fmt.Errorf("delete from main.%s: %w", table.Name, err)
	}

	// Get columns
	columns, err := getTableColumnsFromTx(ctx, tx, "imported", table.Name)
	if err != nil {
		return fmt.Errorf("get columns: %w", err)
	}

	// Replace strategy: include the id column to preserve FK references.
	// Since we deleted all rows first, explicit IDs are safe.
	return s.copyRows(ctx, tx, table, columns, passphrase, false, true)
}

// importMerge inserts new records and updates changed records (by id/XID).
func (s *ImportService) importMerge(ctx context.Context, tx *sql.Tx, table TableExportConfig, passphrase string) error {
	// Get columns
	columns, err := getTableColumnsFromTx(ctx, tx, "imported", table.Name)
	if err != nil {
		return fmt.Errorf("get columns: %w", err)
	}

	// Check if table has id column for merge-by-id
	hasID := false
	for _, col := range columns {
		if col == "id" {
			hasID = true
			break
		}
	}

	if !hasID {
		// Without id, merge = replace (can't match records)
		return s.importReplace(ctx, tx, table, passphrase)
	}

	// Merge strategy: use INSERT OR REPLACE with id for matching.
	return s.copyRows(ctx, tx, table, columns, passphrase, true, true)
}

// copyRows copies rows from imported to main, optionally using INSERT OR REPLACE.
// With TEXT primary keys, the id column is always included to preserve identities.
func (s *ImportService) copyRows(ctx context.Context, tx *sql.Tx, table TableExportConfig, columns []string, passphrase string, upsert bool, includeID bool) error {
	insertCols := make([]string, 0, len(columns))
	selectCols := make([]string, 0, len(columns))
	for _, col := range columns {
		insertCols = append(insertCols, col)
		selectCols = append(selectCols, col)
	}

	// Read rows from imported
	query := fmt.Sprintf("SELECT %s FROM imported.%s", strings.Join(quoteColumns(selectCols), ", "), table.Name) // #nosec G201
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("select from imported: %w", err)
	}
	defer rows.Close()

	// Build INSERT statement
	placeholders := make([]string, len(insertCols))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	verb := "INSERT"
	if upsert {
		verb = "INSERT OR REPLACE"
	}
	insertSQL := fmt.Sprintf("%s INTO main.%s (%s) VALUES (%s)", // #nosec G201
		verb,
		table.Name,
		strings.Join(quoteColumns(insertCols), ", "),
		strings.Join(placeholders, ", "),
	)

	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	// Build column index map
	colIndex := make(map[string]int)
	for i, col := range insertCols {
		colIndex[col] = i
	}

	count := 0
	for rows.Next() {
		values := make([]interface{}, len(insertCols))
		valuePtrs := make([]interface{}, len(insertCols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}

		// Re-encrypt: decrypt with passphrase, encrypt with destination KMS/MasterKey
		if err := s.reencryptForImport(ctx, table, insertCols, values, passphrase); err != nil {
			return fmt.Errorf("re-encrypt for import: %w", err)
		}

		if _, err := stmt.ExecContext(ctx, values...); err != nil {
			return fmt.Errorf("insert row: %w", err)
		}
		count++
	}

	s.logger.Debug("copied rows", zap.String("table", table.Name), zap.Int("count", count))
	return rows.Err()
}

// reencryptForImport decrypts passphrase-encrypted columns and re-encrypts with destination KMS/MasterKey.
func (s *ImportService) reencryptForImport(ctx context.Context, table TableExportConfig, columns []string, values []interface{}, passphrase string) error {
	// Handle explicit encrypted columns
	for colName, encType := range table.EncryptedColumns {
		idx := -1
		for i, c := range columns {
			if c == colName {
				idx = i
				break
			}
		}
		if idx == -1 || values[idx] == nil {
			continue
		}

		// Decrypt from passphrase encryption
		var cipherBytes []byte
		switch v := values[idx].(type) {
		case []byte:
			cipherBytes = v
		case string:
			cipherBytes = []byte(v)
		default:
			continue
		}

		if len(cipherBytes) == 0 {
			continue
		}

		plaintext, err := security.DecryptWithPassphrase(cipherBytes, []byte(passphrase))
		if err != nil {
			s.logger.Warn("failed to decrypt imported column, skipping re-encryption",
				zap.String("table", table.Name),
				zap.String("column", colName),
				zap.Error(err),
			)
			continue
		}

		// Re-encrypt with destination KMS/MasterKey
		reencrypted, err := s.encryptValue(ctx, plaintext, encType)
		if err != nil {
			return fmt.Errorf("re-encrypt %s.%s: %w", table.Name, colName, err)
		}
		values[idx] = reencrypted
	}

	// Special handling for settings table
	if table.Name == "settings" {
		if err := s.reencryptSettingsForImport(ctx, columns, values, passphrase); err != nil {
			return err
		}
	}

	return nil
}

// reencryptSettingsForImport handles conditionally encrypted settings rows.
func (s *ImportService) reencryptSettingsForImport(ctx context.Context, columns []string, values []interface{}, passphrase string) error {
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
		return nil
	}

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

	// The exported value is base64-encoded passphrase-encrypted data
	valueStr, ok := values[valueIdx].(string)
	if !ok {
		return nil
	}

	encBytes, err := base64.StdEncoding.DecodeString(valueStr)
	if err != nil {
		s.logger.Warn("failed to base64-decode encrypted setting", zap.Error(err))
		return nil
	}

	plaintext, err := security.DecryptWithPassphrase(encBytes, []byte(passphrase))
	if err != nil {
		s.logger.Warn("failed to decrypt imported setting", zap.Error(err))
		return nil
	}

	// Re-encrypt with KMS
	if s.kms == nil {
		return fmt.Errorf("KMS not available for re-encryption")
	}
	reencrypted, err := s.kms.Encrypt(ctx, plaintext)
	if err != nil {
		return fmt.Errorf("re-encrypt setting with KMS: %w", err)
	}
	values[valueIdx] = reencrypted

	return nil
}

// encryptValue encrypts plaintext with the appropriate system (KMS or MasterKey).
func (s *ImportService) encryptValue(ctx context.Context, plaintext []byte, encType string) (interface{}, error) {
	switch encType {
	case "kms":
		if s.kms == nil {
			return nil, fmt.Errorf("KMS not available")
		}
		// KMS returns string in v1:{key_id}:{nonce}:{ct} format
		return s.kms.Encrypt(ctx, plaintext)

	case "masterkey":
		if s.masterKey == nil {
			return nil, fmt.Errorf("MasterKey not available")
		}
		// MasterKey returns binary (nonce‖ciphertext)
		return s.masterKey.Encrypt(plaintext)

	default:
		return nil, fmt.Errorf("unknown encryption type: %s", encType)
	}
}

// tableHasColumn checks if a table has a specific column.
func tableHasColumn(ctx context.Context, conn *sql.Conn, schema, tableName, columnName string) (bool, error) {
	query := fmt.Sprintf("PRAGMA %s.table_info(%s)", schema, tableName) // #nosec G201
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue *string
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	return false, rows.Err()
}

// getTableColumnsFromTx returns column names for a table using a transaction.
func getTableColumnsFromTx(ctx context.Context, tx *sql.Tx, schema, tableName string) ([]string, error) {
	query := fmt.Sprintf("PRAGMA %s.table_info(%s)", schema, tableName) // #nosec G201
	rows, err := tx.QueryContext(ctx, query)
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
