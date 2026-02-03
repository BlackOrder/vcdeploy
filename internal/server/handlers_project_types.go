// Package server provides project type handlers for the master server.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// handleProjectTypes handles GET/POST for /api/v1/project-types.
func (s *MasterServer) handleProjectTypes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		types, err := s.projectTypeService.List(ctx)
		if err != nil {
			s.logger.Error("Failed to list project types", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to list project types")
			return
		}

		// Apply pagination
		p := parsePagination(r)
		totalCount := len(types)

		// Apply offset
		if p.Offset >= totalCount {
			types = []*storage.ProjectType{}
		} else {
			types = types[p.Offset:]
			// Apply limit
			if p.Limit > 0 && p.Limit < len(types) {
				types = types[:p.Limit]
			}
		}

		s.jsonResponse(w, map[string]interface{}{
			"items":      types,
			"totalCount": totalCount,
			"limit":      p.Limit,
			"offset":     p.Offset,
		})

	case http.MethodPost:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		// Limit body size to 1MB
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			BuildCmd    string `json:"buildCmd"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if req.Name == "" {
			s.jsonError(w, http.StatusBadRequest, "name is required")
			return
		}

		pt, err := s.projectTypeService.Create(ctx, req.Name, req.Description, req.BuildCmd)
		if err != nil {
			s.logger.Error("Failed to create project type", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to create project type")
			return
		}

		s.logAudit(r, "create", "project_type", fmt.Sprintf("name=%s", req.Name), "success")
		// H4 FIX: POST should return 201 Created, not 200
		s.writeJSON(w, http.StatusCreated, pt)

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleProjectType handles GET/PUT/DELETE for /api/v1/project-types/{name}.
func (s *MasterServer) handleProjectType(w http.ResponseWriter, r *http.Request) {
	// Extract name from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/project-types/")
	parts := strings.Split(path, "/")
	name := parts[0]

	if name == "" {
		s.jsonError(w, http.StatusBadRequest, "Project type name required")
		return
	}

	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		pt, err := s.projectTypeService.GetByName(ctx, name)
		if err != nil {
			s.logger.Error("Failed to get project type", zap.Error(err))
			s.jsonError(w, http.StatusNotFound, "Project type not found")
			return
		}
		s.jsonResponse(w, pt)

	case http.MethodPut:
		var req struct {
			Description string `json:"description"`
			BuildCmd    string `json:"buildCmd"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		pt := &storage.ProjectType{
			Name:        name,
			Description: req.Description,
			BuildCmd:    req.BuildCmd,
		}

		if err := s.projectTypeService.Update(ctx, pt); err != nil {
			s.logger.Error("Failed to update project type", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to update project type")
			return
		}

		s.logAudit(r, "update", "project_type", fmt.Sprintf("name=%s", name), "success")
		s.jsonResponse(w, map[string]string{"status": "updated"})

	case http.MethodDelete:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		if err := s.projectTypeService.Delete(ctx, name); err != nil {
			s.logger.Error("Failed to delete project type", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to delete project type")
			return
		}

		s.logAudit(r, "delete", "project_type", fmt.Sprintf("name=%s", name), "success")
		w.WriteHeader(http.StatusNoContent)

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
