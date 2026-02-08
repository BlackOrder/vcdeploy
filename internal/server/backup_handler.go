package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
