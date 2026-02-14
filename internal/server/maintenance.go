package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// maintenanceMiddleware blocks mutating requests when the server is in
// maintenance mode. Reads (GET, HEAD, OPTIONS) pass through. The admin
// maintenance toggle, health endpoint, and import endpoints are always
// allowed so the admin can control the mode and perform imports.
func (s *MasterServer) maintenanceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.maintenanceMode.Load() {
			next.ServeHTTP(w, r)
			return
		}

		// Allow read-only methods
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		// Allow maintenance toggle endpoint
		if r.URL.Path == "/api/v1/admin/maintenance" {
			next.ServeHTTP(w, r)
			return
		}

		// Allow health endpoint
		if r.URL.Path == "/health" || r.URL.Path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Allow import endpoints during maintenance
		if strings.HasPrefix(r.URL.Path, "/api/v1/admin/backup/import") {
			next.ServeHTTP(w, r)
			return
		}

		// Block all other mutations
		w.Header().Set("Retry-After", "1800")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "Server is in maintenance mode. Only read operations are allowed.",
		})
	})
}

// handleMaintenanceToggle enables or disables maintenance mode.
// POST /api/v1/admin/maintenance
// Body: {"enabled": true} or {"enabled": false}
// GET  /api/v1/admin/maintenance — returns current status
func (s *MasterServer) handleMaintenanceToggle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// Read access to check maintenance status
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}
		s.jsonResponse(w, map[string]interface{}{
			"maintenance": s.maintenanceMode.Load(),
		})

	case http.MethodPost:
		// Admin-only: toggling maintenance mode
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if req.Enabled {
			// Enter maintenance: set flag, flush pending writes
			s.maintenanceMode.Store(true)
			s.logger.Info("maintenance mode enabled, flushing pending writes")

			if err := s.store.FlushPending(); err != nil {
				s.logger.Error("failed to flush pending writes during maintenance enter", zap.Error(err))
				// Continue — maintenance mode is set even if flush had issues
			}

			s.logAudit(r, "update", "system", "Entered maintenance mode", "success")
			s.jsonResponse(w, map[string]interface{}{
				"status":      "maintenance_enabled",
				"maintenance": true,
			})
		} else {
			// Exit maintenance: refresh cache from DB (import may have changed it), then clear flag
			if err := s.store.Reload(r.Context()); err != nil {
				s.logger.Error("failed to reload store after maintenance exit", zap.Error(err))
			}
			s.maintenanceMode.Store(false)
			s.logger.Info("maintenance mode disabled, store reloaded")

			s.logAudit(r, "update", "system", "Exited maintenance mode", "success")
			s.jsonResponse(w, map[string]interface{}{
				"status":      "maintenance_disabled",
				"maintenance": false,
			})
		}

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
