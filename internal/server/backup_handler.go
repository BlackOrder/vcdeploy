package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services/backup"
	"go.uber.org/zap"
)

// exportDownload tracks a pending export download.
type exportDownload struct {
	filePath  string
	expiresAt time.Time
}

// exportDownloads tracks download tokens → file paths with expiry.
var (
	exportDownloads   = make(map[string]*exportDownload)
	exportDownloadsMu sync.Mutex
)

// handleBackupExport handles POST /api/v1/admin/backup/export.
// Creates a passphrase-protected export and returns a download URL.
func (s *MasterServer) handleBackupExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Require admin role
	user, _ := GetUserFromContext(r.Context())
	if user == nil || user.Role != "admin" {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}

	var req struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, `{"error":"passphrase is required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Passphrase) < 8 {
		http.Error(w, `{"error":"passphrase must be at least 8 characters"}`, http.StatusBadRequest)
		return
	}

	// Create export in temp directory
	tmpDir := os.TempDir()
	timestamp := time.Now().Format("20060102-150405")
	outputPath := filepath.Join(tmpDir, fmt.Sprintf("vcdeploy-export-%s.db", timestamp))

	exportSvc := backup.NewExportService(s.store, s.kms, s.masterKey, s.logger)
	if err := exportSvc.Export(r.Context(), req.Passphrase, outputPath); err != nil {
		s.logger.Error("backup export failed", zap.Error(err))
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "export failed: " + err.Error(),
		})
		return
	}

	// Generate download token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		_ = os.Remove(outputPath)
		http.Error(w, "failed to generate download token", http.StatusInternalServerError)
		return
	}
	token := hex.EncodeToString(tokenBytes)

	exportDownloadsMu.Lock()
	exportDownloads[token] = &exportDownload{
		filePath:  outputPath,
		expiresAt: time.Now().Add(10 * time.Minute),
	}
	exportDownloadsMu.Unlock()

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"download_url": fmt.Sprintf("/api/v1/admin/backup/download/%s", token),
		"expires":      time.Now().Add(10 * time.Minute).Format(time.RFC3339),
		"message":      "Export created successfully. Use the download URL to retrieve the file.",
	})
}

// handleBackupDownload handles GET /api/v1/admin/backup/download/{token}.
// Serves the export file and cleans up after download.
func (s *MasterServer) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Require admin role
	user, _ := GetUserFromContext(r.Context())
	if user == nil || user.Role != "admin" {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}

	// Extract token from URL
	token := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/backup/download/")
	if token == "" {
		http.Error(w, "download token required", http.StatusBadRequest)
		return
	}

	exportDownloadsMu.Lock()
	dl, ok := exportDownloads[token]
	if ok {
		delete(exportDownloads, token)
	}
	exportDownloadsMu.Unlock()

	if !ok {
		http.Error(w, "invalid or expired download token", http.StatusNotFound)
		return
	}

	if time.Now().After(dl.expiresAt) {
		_ = os.Remove(dl.filePath)
		http.Error(w, "download token has expired", http.StatusGone)
		return
	}

	// Serve the file
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="vcdeploy-export.db"`))
	http.ServeFile(w, r, dl.filePath)

	// Clean up the temp file after serving
	go func() {
		time.Sleep(5 * time.Second)
		_ = os.Remove(dl.filePath)
	}()
}

// cleanupExpiredExports removes expired download tokens and their files.
// Called periodically by the scheduler.
func cleanupExpiredExports() {
	exportDownloadsMu.Lock()
	defer exportDownloadsMu.Unlock()

	now := time.Now()
	for token, dl := range exportDownloads {
		if now.After(dl.expiresAt) {
			_ = os.Remove(dl.filePath)
			delete(exportDownloads, token)
		}
	}
}

// importSession tracks an uploaded import file pending execution.
type importSession struct {
	filePath  string
	diff      *backup.ImportDiff
	expiresAt time.Time
}

var (
	importSessions   = make(map[string]*importSession)
	importSessionsMu sync.Mutex
)

// handleBackupImportUpload handles POST /api/v1/admin/backup/import/upload.
// Accepts a multipart upload of an export file + passphrase, returns session ID and diff.
func (s *MasterServer) handleBackupImportUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Require admin role
	user, _ := GetUserFromContext(r.Context())
	if user == nil || user.Role != "admin" {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}

	// Require maintenance mode
	if !s.maintenanceMode.Load() {
		s.writeJSON(w, http.StatusConflict, map[string]string{
			"error": "maintenance mode must be enabled before importing",
		})
		return
	}

	// Parse multipart form (limit to 256MB)
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}

	passphrase := r.FormValue("passphrase")
	if passphrase == "" {
		http.Error(w, `{"error":"passphrase is required"}`, http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "import file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save to temp file
	tmpFile, err := os.CreateTemp("", "vcdeploy-import-*.db")
	if err != nil {
		http.Error(w, "failed to create temp file", http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		http.Error(w, "failed to save uploaded file", http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	// Compute diff
	importSvc := backup.NewImportService(s.store, s.kms, s.masterKey, s.logger)
	diff, err := importSvc.ComputeDiff(r.Context(), tmpFile.Name())
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		s.logger.Error("import diff computation failed", zap.Error(err))
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to compute import diff: " + err.Error(),
		})
		return
	}

	// Generate session ID
	sessionBytes := make([]byte, 32)
	if _, err := rand.Read(sessionBytes); err != nil {
		_ = os.Remove(tmpFile.Name())
		http.Error(w, "failed to generate session ID", http.StatusInternalServerError)
		return
	}
	sessionID := hex.EncodeToString(sessionBytes)

	importSessionsMu.Lock()
	importSessions[sessionID] = &importSession{
		filePath:  tmpFile.Name(),
		diff:      diff,
		expiresAt: time.Now().Add(30 * time.Minute),
	}
	importSessionsMu.Unlock()

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_id": sessionID,
		"diff":       diff,
		"expires":    time.Now().Add(30 * time.Minute).Format(time.RFC3339),
		"message":    "Review the diff and call /api/v1/admin/backup/import/execute to apply.",
	})
}

// handleBackupImportExecute handles POST /api/v1/admin/backup/import/execute.
// Applies the import with per-table strategies.
func (s *MasterServer) handleBackupImportExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Require admin role
	user, _ := GetUserFromContext(r.Context())
	if user == nil || user.Role != "admin" {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}

	// Require maintenance mode
	if !s.maintenanceMode.Load() {
		s.writeJSON(w, http.StatusConflict, map[string]string{
			"error": "maintenance mode must be enabled before importing",
		})
		return
	}

	var req struct {
		SessionID  string            `json:"session_id"`
		Passphrase string            `json:"passphrase"`
		Strategies map[string]string `json:"strategies"` // table_name → "replace"|"merge"|"skip"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" || req.Passphrase == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "session_id and passphrase are required",
		})
		return
	}

	// Look up session
	importSessionsMu.Lock()
	session, ok := importSessions[req.SessionID]
	if ok {
		delete(importSessions, req.SessionID)
	}
	importSessionsMu.Unlock()

	if !ok {
		s.writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "invalid or expired import session",
		})
		return
	}

	if time.Now().After(session.expiresAt) {
		_ = os.Remove(session.filePath)
		s.writeJSON(w, http.StatusGone, map[string]string{
			"error": "import session has expired",
		})
		return
	}

	// Convert string strategies to typed
	strategies := make(map[string]backup.ImportStrategy)
	for table, strat := range req.Strategies {
		switch strat {
		case "replace":
			strategies[table] = backup.StrategyReplace
		case "merge":
			strategies[table] = backup.StrategyMerge
		case "skip", "":
			strategies[table] = backup.StrategySkip
		default:
			_ = os.Remove(session.filePath)
			http.Error(w, fmt.Sprintf(`{"error":"invalid strategy %q for table %s"}`, strat, table), http.StatusBadRequest)
			return
		}
	}

	// Execute import
	importSvc := backup.NewImportService(s.store, s.kms, s.masterKey, s.logger)
	if err := importSvc.Execute(r.Context(), session.filePath, req.Passphrase, strategies); err != nil {
		_ = os.Remove(session.filePath)
		s.logger.Error("import execution failed", zap.Error(err))
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "import failed: " + err.Error(),
		})
		return
	}

	// Clean up
	_ = os.Remove(session.filePath)

	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "Import completed successfully. Disable maintenance mode when ready.",
	})
}
