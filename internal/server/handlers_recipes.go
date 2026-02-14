// Package server provides recipe management handlers for the master server.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services/recipes"
	"github.com/BlackOrder/vcdeploy/internal/services/recipes/export"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/validation"
	"go.uber.org/zap"
)

// --- Component API ---

// handleRecipeComponents handles GET/POST for /api/v1/recipes/components.
func (s *MasterServer) handleRecipeComponents(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		s.handleListComponents(ctx, w, r)
	case http.MethodPost:
		s.handleCreateComponent(ctx, w, r)
	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleRecipeComponent handles GET/PUT/DELETE for /api/v1/recipes/components/{id}.
func (s *MasterServer) handleRecipeComponent(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Extract ID from path: /api/v1/recipes/components/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/recipes/components/")
	id := path
	if id == "" {
		s.jsonError(w, http.StatusBadRequest, "invalid component ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetComponent(ctx, w, id)
	case http.MethodPut:
		s.handleUpdateComponent(ctx, w, r, id)
	case http.MethodDelete:
		s.handleDeleteComponent(ctx, w, id)
	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *MasterServer) handleListComponents(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Check read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = storage.NamespaceUser
	}
	includeDeprecated := r.URL.Query().Get("include_deprecated") == "true"

	components, err := s.store.ListRecipeComponents(ctx, namespace, includeDeprecated)
	if err != nil {
		s.logger.Error("Failed to list components", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to list components")
		return
	}

	// Apply pagination
	p := parsePagination(r)
	totalCount := len(components)

	// Apply offset and limit
	if p.Offset >= totalCount {
		components = nil
	} else {
		components = components[p.Offset:]
		if p.Limit > 0 && p.Limit < len(components) {
			components = components[:p.Limit]
		}
	}

	s.jsonResponse(w, PaginatedResponse{
		Items:      components,
		TotalCount: int64(totalCount),
		Limit:      p.Limit,
		Offset:     p.Offset,
	})
}

func (s *MasterServer) handleGetComponent(ctx context.Context, w http.ResponseWriter, id string) {
	// Check read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	component, err := s.store.GetRecipeComponentByID(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get component", zap.Error(err), zap.String("id", id))
		s.jsonError(w, http.StatusInternalServerError, "failed to get component")
		return
	}
	if component == nil {
		s.jsonError(w, http.StatusNotFound, "component not found")
		return
	}

	s.jsonResponse(w, component)
}

// CreateComponentRequest is the request body for creating a component.
type CreateComponentRequest struct {
	Slug          string                       `json:"slug"`
	Version       string                       `json:"version"`
	Name          string                       `json:"name"`
	Description   string                       `json:"description,omitempty"`
	ComponentType string                       `json:"component_type"`
	Content       storage.ComponentContent     `json:"content"`
	Variables     []storage.VariableDefinition `json:"variables,omitempty"`
	IsRaw         bool                         `json:"is_raw"`
}

func (s *MasterServer) handleCreateComponent(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Limit body size to 1MB to prevent DoS
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req CreateComponentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          req.Slug,
		Version:       req.Version,
		Name:          req.Name,
		Description:   req.Description,
		ComponentType: req.ComponentType,
		Content:       req.Content,
		Variables:     req.Variables,
		IsRaw:         req.IsRaw,
		IsSeed:        false,
		CreatedAt:     time.Now(),
	}

	svc := recipes.NewComponentService(s.store)
	if err := svc.Create(ctx, component); err != nil {
		s.logger.Error("Failed to create component", zap.Error(err))
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.logger.Info("Component created",
		zap.String("slug", component.Slug),
		zap.String("version", component.Version))

	s.writeJSON(w, http.StatusCreated, component)
}

func (s *MasterServer) handleUpdateComponent(ctx context.Context, w http.ResponseWriter, r *http.Request, id string) {
	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Get existing component
	existing, err := s.store.GetRecipeComponentByID(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get component", zap.Error(err), zap.String("id", id))
		s.jsonError(w, http.StatusInternalServerError, "failed to get component")
		return
	}
	if existing == nil {
		s.jsonError(w, http.StatusNotFound, "component not found")
		return
	}

	// Cannot update seed components
	if existing.IsSeed {
		s.jsonError(w, http.StatusForbidden, "cannot modify seed components")
		return
	}

	// Limit body size to 1MB to prevent DoS
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req CreateComponentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Update fields
	existing.Name = req.Name
	existing.Description = req.Description
	existing.Content = req.Content
	existing.Variables = req.Variables
	existing.IsRaw = req.IsRaw

	svc := recipes.NewComponentService(s.store)
	if err := svc.Update(ctx, existing); err != nil {
		s.logger.Error("Failed to update component", zap.Error(err))
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.logger.Info("Component updated",
		zap.String("slug", existing.Slug),
		zap.String("version", existing.Version))

	s.jsonResponse(w, existing)
}

func (s *MasterServer) handleDeleteComponent(ctx context.Context, w http.ResponseWriter, id string) {
	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Get existing component
	existing, err := s.store.GetRecipeComponentByID(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get component", zap.Error(err), zap.String("id", id))
		s.jsonError(w, http.StatusInternalServerError, "failed to get component")
		return
	}
	if existing == nil {
		s.jsonError(w, http.StatusNotFound, "component not found")
		return
	}

	// Cannot delete seed components
	if existing.IsSeed {
		s.jsonError(w, http.StatusForbidden, "cannot delete seed components")
		return
	}

	svc := recipes.NewComponentService(s.store)
	if err := svc.Delete(ctx, id); err != nil {
		s.logger.Error("Failed to delete component", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logger.Info("Component deleted",
		zap.String("slug", existing.Slug),
		zap.String("version", existing.Version))

	w.WriteHeader(http.StatusNoContent)
}

// --- Playbook API ---

// handleRecipePlaybooks handles GET/POST for /api/v1/recipes/playbooks.
func (s *MasterServer) handleRecipePlaybooks(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		s.handleListPlaybooks(ctx, w, r)
	case http.MethodPost:
		s.handleCreatePlaybook(ctx, w, r)
	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleRecipePlaybook handles GET/PUT/DELETE for /api/v1/recipes/playbooks/{id}.
func (s *MasterServer) handleRecipePlaybook(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Extract ID from path: /api/v1/recipes/playbooks/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/recipes/playbooks/")
	id := path
	if id == "" {
		s.jsonError(w, http.StatusBadRequest, "invalid playbook ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetPlaybook(ctx, w, id)
	case http.MethodPut:
		s.handleUpdatePlaybook(ctx, w, r, id)
	case http.MethodDelete:
		s.handleDeletePlaybook(ctx, w, id)
	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *MasterServer) handleListPlaybooks(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Check read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = storage.NamespaceUser
	}
	framework := r.URL.Query().Get("framework")
	includeDeprecated := r.URL.Query().Get("include_deprecated") == "true"

	playbooks, err := s.store.ListPlaybooks(ctx, namespace, framework, includeDeprecated)
	if err != nil {
		s.logger.Error("Failed to list playbooks", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to list playbooks")
		return
	}

	// Apply pagination
	p := parsePagination(r)
	totalCount := len(playbooks)

	// Apply offset and limit
	if p.Offset >= totalCount {
		playbooks = nil
	} else {
		playbooks = playbooks[p.Offset:]
		if p.Limit > 0 && p.Limit < len(playbooks) {
			playbooks = playbooks[:p.Limit]
		}
	}

	s.jsonResponse(w, PaginatedResponse{
		Items:      playbooks,
		TotalCount: int64(totalCount),
		Limit:      p.Limit,
		Offset:     p.Offset,
	})
}

func (s *MasterServer) handleGetPlaybook(ctx context.Context, w http.ResponseWriter, id string) {
	// Check read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	playbook, err := s.store.GetPlaybookByID(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get playbook", zap.Error(err), zap.String("id", id))
		s.jsonError(w, http.StatusInternalServerError, "failed to get playbook")
		return
	}
	if playbook == nil {
		s.jsonError(w, http.StatusNotFound, "playbook not found")
		return
	}

	s.jsonResponse(w, playbook)
}

// CreatePlaybookRequest is the request body for creating a playbook.
type CreatePlaybookRequest struct {
	Slug            string                   `json:"slug"`
	Version         string                   `json:"version"`
	Name            string                   `json:"name"`
	Description     string                   `json:"description,omitempty"`
	FrameworkType   string                   `json:"framework_type,omitempty"`
	Steps           []storage.PlaybookStep   `json:"steps"`
	SharedDirs      []string                 `json:"shared_dirs,omitempty"`
	SharedFiles     []string                 `json:"shared_files,omitempty"`
	WritableDirs    []string                 `json:"writable_dirs,omitempty"`
	KeepReleases    int                      `json:"keep_releases"`
	ValidationRules *storage.ValidationRules `json:"validation_rules,omitempty"`
}

func (s *MasterServer) handleCreatePlaybook(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Limit body size to 1MB to prevent DoS
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req CreatePlaybookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	playbook := &storage.Playbook{
		Namespace:       storage.NamespaceUser,
		Slug:            req.Slug,
		Version:         req.Version,
		Name:            req.Name,
		Description:     req.Description,
		FrameworkType:   req.FrameworkType,
		Steps:           req.Steps,
		SharedDirs:      req.SharedDirs,
		SharedFiles:     req.SharedFiles,
		WritableDirs:    req.WritableDirs,
		KeepReleases:    req.KeepReleases,
		ValidationRules: req.ValidationRules,
		IsSeed:          false,
		CreatedAt:       time.Now(),
	}

	svc := recipes.NewPlaybookService(s.store)
	if err := svc.Create(ctx, playbook); err != nil {
		s.logger.Error("Failed to create playbook", zap.Error(err))
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.logger.Info("Playbook created",
		zap.String("slug", playbook.Slug),
		zap.String("version", playbook.Version))

	s.writeJSON(w, http.StatusCreated, playbook)
}

func (s *MasterServer) handleUpdatePlaybook(ctx context.Context, w http.ResponseWriter, r *http.Request, id string) {
	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Get existing playbook
	existing, err := s.store.GetPlaybookByID(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get playbook", zap.Error(err), zap.String("id", id))
		s.jsonError(w, http.StatusInternalServerError, "failed to get playbook")
		return
	}
	if existing == nil {
		s.jsonError(w, http.StatusNotFound, "playbook not found")
		return
	}

	// Cannot update seed playbooks
	if existing.IsSeed {
		s.jsonError(w, http.StatusForbidden, "cannot modify seed playbooks")
		return
	}

	// Limit body size to 1MB to prevent DoS
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req CreatePlaybookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Update fields
	existing.Name = req.Name
	existing.Description = req.Description
	existing.Steps = req.Steps
	existing.SharedDirs = req.SharedDirs
	existing.SharedFiles = req.SharedFiles
	existing.WritableDirs = req.WritableDirs
	existing.KeepReleases = req.KeepReleases
	existing.ValidationRules = req.ValidationRules

	svc := recipes.NewPlaybookService(s.store)
	if err := svc.Update(ctx, existing); err != nil {
		s.logger.Error("Failed to update playbook", zap.Error(err))
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.logger.Info("Playbook updated",
		zap.String("slug", existing.Slug),
		zap.String("version", existing.Version))

	s.jsonResponse(w, existing)
}

func (s *MasterServer) handleDeletePlaybook(ctx context.Context, w http.ResponseWriter, id string) {
	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Get existing playbook
	existing, err := s.store.GetPlaybookByID(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get playbook", zap.Error(err), zap.String("id", id))
		s.jsonError(w, http.StatusInternalServerError, "failed to get playbook")
		return
	}
	if existing == nil {
		s.jsonError(w, http.StatusNotFound, "playbook not found")
		return
	}

	// Cannot delete seed playbooks
	if existing.IsSeed {
		s.jsonError(w, http.StatusForbidden, "cannot delete seed playbooks")
		return
	}

	svc := recipes.NewPlaybookService(s.store)
	if err := svc.Delete(ctx, id); err != nil {
		s.logger.Error("Failed to delete playbook", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logger.Info("Playbook deleted",
		zap.String("slug", existing.Slug),
		zap.String("version", existing.Version))

	w.WriteHeader(http.StatusNoContent)
}

// --- Activation API ---

// handleProjectPlaybookByID handles GET/POST/DELETE for /api/v1/projects/{id}/playbook.
func (s *MasterServer) handleProjectPlaybookByID(w http.ResponseWriter, r *http.Request, projectID string) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		s.handleGetProjectPlaybookByID(ctx, w, projectID)
	case http.MethodPost:
		s.handleActivatePlaybookByID(ctx, w, r, projectID)
	case http.MethodDelete:
		s.handleDeactivatePlaybookByID(ctx, w, projectID)
	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *MasterServer) handleGetProjectPlaybookByID(ctx context.Context, w http.ResponseWriter, projectID string) {
	// Check read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	svc := recipes.NewActivationService(s.store)
	activation, playbook, err := svc.GetActiveWithPlaybook(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get activation", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to get activation")
		return
	}

	if activation == nil {
		s.jsonResponse(w, map[string]interface{}{
			"active":   false,
			"playbook": nil,
		})
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"active":     true,
		"activation": activation,
		"playbook":   playbook,
	})
}

// ActivatePlaybookRequest is the request body for activating a playbook.
type ActivatePlaybookRequest struct {
	PlaybookID string                             `json:"playbook_id"`
	Bindings   map[string]recipes.VariableBinding `json:"bindings,omitempty"`
}

func (s *MasterServer) handleActivatePlaybookByID(ctx context.Context, w http.ResponseWriter, r *http.Request, projectID string) {
	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Limit body size to 1MB to prevent DoS
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req ActivatePlaybookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get user ID from context
	var userID *string
	if user, ok := GetUserFromContext(ctx); ok && user != nil {
		userID = &user.ID
	}

	svc := recipes.NewActivationService(s.store)
	activation, err := svc.Activate(ctx, projectID, req.PlaybookID, req.Bindings, userID)
	if err != nil {
		s.logger.Error("Failed to activate playbook", zap.Error(err))
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.logger.Info("Playbook activated",
		zap.String("project_id", projectID),
		zap.String("playbook_id", req.PlaybookID))

	s.writeJSON(w, http.StatusCreated, activation)
}

func (s *MasterServer) handleDeactivatePlaybookByID(ctx context.Context, w http.ResponseWriter, projectID string) {
	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	svc := recipes.NewActivationService(s.store)
	if err := svc.Deactivate(ctx, projectID); err != nil {
		s.logger.Error("Failed to deactivate playbook", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logger.Info("Playbook deactivated", zap.String("project_id", projectID))

	w.WriteHeader(http.StatusNoContent)
}

// --- Activation Routes (documented API) ---

// ActivationRequest is the request body for POST /api/v1/recipes/activations.
type ActivationRequest struct {
	ProjectID  string                             `json:"project_id"`
	PlaybookID string                             `json:"playbook_id"`
	Bindings   map[string]recipes.VariableBinding `json:"bindings,omitempty"`
}

// handleActivations handles POST for /api/v1/recipes/activations.
func (s *MasterServer) handleActivations(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Limit body size to 1MB to prevent DoS
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req ActivationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ProjectID == "" {
		s.jsonError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if req.PlaybookID == "" {
		s.jsonError(w, http.StatusBadRequest, "playbook_id is required")
		return
	}

	// Get user ID from context
	var userID *string
	if user, ok := GetUserFromContext(ctx); ok && user != nil {
		userID = &user.ID
	}

	svc := recipes.NewActivationService(s.store)
	activation, err := svc.Activate(ctx, req.ProjectID, req.PlaybookID, req.Bindings, userID)
	if err != nil {
		s.logger.Error("Failed to activate playbook", zap.Error(err))
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.logger.Info("Playbook activated",
		zap.String("project_id", req.ProjectID),
		zap.String("playbook_id", req.PlaybookID))

	s.writeJSON(w, http.StatusCreated, activation)
}

// handleActivation handles GET/DELETE for /api/v1/recipes/activations/{project_id}.
func (s *MasterServer) handleActivation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Extract project ID from path: /api/v1/recipes/activations/{project_id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/recipes/activations/")
	// Handle optional /detail suffix
	path = strings.TrimSuffix(path, "/detail")
	projectID := path
	if projectID == "" {
		s.jsonError(w, http.StatusBadRequest, "invalid project ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetProjectPlaybookByID(ctx, w, projectID)
	case http.MethodDelete:
		s.handleDeactivatePlaybookByID(ctx, w, projectID)
	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// --- RAW Approval API (Admin only) ---

// handleRawApprovals handles GET/POST for /api/v1/recipes/raw-approvals.
func (s *MasterServer) handleRawApprovals(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Admin only
	if !s.requireAdmin(ctx, w) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleListRawApprovals(ctx, w, r)
	case http.MethodPost:
		s.handleApproveRaw(ctx, w, r)
	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleRawApproval handles DELETE for /api/v1/recipes/raw-approvals/{id}.
func (s *MasterServer) handleRawApproval(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Admin only
	if !s.requireAdmin(ctx, w) {
		return
	}

	// Extract ID from path: /api/v1/recipes/raw-approvals/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/recipes/raw-approvals/")
	id := path
	if id == "" {
		s.jsonError(w, http.StatusBadRequest, "invalid approval ID")
		return
	}

	if r.Method != http.MethodDelete {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	s.handleRevokeRawApproval(ctx, w, id)
}

func (s *MasterServer) handleListRawApprovals(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	svc := recipes.NewRawApprovalService(s.store)

	pendingOnly := r.URL.Query().Get("pending_only") == "true"

	if pendingOnly {
		// List pending RAW components that need approval
		components, err := svc.ListPendingApprovals(ctx)
		if err != nil {
			s.logger.Error("Failed to list pending RAW components", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "failed to list pending components")
			return
		}
		s.jsonResponse(w, map[string]interface{}{
			"pending_components": components,
			"count":              len(components),
		})
		return
	}

	// List all existing approvals
	approvals, err := svc.ListAllApprovals(ctx)
	if err != nil {
		s.logger.Error("Failed to list RAW approvals", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to list approvals")
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"approvals": approvals,
		"count":     len(approvals),
	})
}

// ApproveRawRequest is the request body for approving a RAW command.
type ApproveRawRequest struct {
	ComponentID string `json:"component_id"`
	Reason      string `json:"reason,omitempty"`
}

func (s *MasterServer) handleApproveRaw(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var req ApproveRawRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get admin user ID from context
	user, ok := GetUserFromContext(ctx)
	if !ok || user == nil {
		s.jsonError(w, http.StatusUnauthorized, "user not found")
		return
	}

	svc := recipes.NewRawApprovalService(s.store)
	if err := svc.Approve(ctx, req.ComponentID, user.ID, req.Reason); err != nil {
		s.logger.Error("Failed to approve RAW component", zap.Error(err))
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.logger.Info("RAW component approved",
		zap.String("component_id", req.ComponentID),
		zap.String("approved_by", user.ID))

	s.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"component_id": req.ComponentID,
		"approved_by":  user.ID,
		"message":      "RAW component approved",
	})
}

// RevokeRawRequest is the request body for revoking a RAW approval.
// The component_id is extracted from the URL path, but we need admin user ID.
func (s *MasterServer) handleRevokeRawApproval(ctx context.Context, w http.ResponseWriter, componentID string) {
	// Get admin user ID from context
	user, ok := GetUserFromContext(ctx)
	if !ok || user == nil {
		s.jsonError(w, http.StatusUnauthorized, "user not found")
		return
	}

	svc := recipes.NewRawApprovalService(s.store)
	if err := svc.RevokeApproval(ctx, componentID, user.ID); err != nil {
		s.logger.Error("Failed to revoke RAW approval", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logger.Info("RAW approval revoked",
		zap.String("component_id", componentID),
		zap.String("revoked_by", user.ID))

	w.WriteHeader(http.StatusNoContent)
}

// --- Export/Import API ---

// handleRecipeExport handles GET for /api/v1/recipes/export.
func (s *MasterServer) handleRecipeExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutLong)
	defer cancel()

	// Check read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Use a placeholder version - in production this would come from build info
	exporter := export.NewExporter(s.store, "v0.1.0")
	bundle, err := exporter.ExportAll(ctx)
	if err != nil {
		s.logger.Error("Failed to export recipes", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to export recipes")
		return
	}

	s.logger.Info("Recipe data exported",
		zap.Int("components", len(bundle.Components)),
		zap.Int("playbooks", len(bundle.Playbooks)))

	// Set filename header for download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=recipes-%s.json", bundle.ExportedAt.Format("2006-01-02")))
	s.jsonResponse(w, bundle)
}

// RecipeImportRequest is the request body for importing recipes.
type RecipeImportRequest struct {
	Bundle   export.ExportBundle     `json:"bundle"`
	Strategy export.ConflictStrategy `json:"strategy"`
	DryRun   bool                    `json:"dry_run"`
}

// handleRecipeImport handles POST for /api/v1/recipes/import.
func (s *MasterServer) handleRecipeImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutLong)
	defer cancel()

	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	var req RecipeImportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Default strategy
	if req.Strategy == "" {
		req.Strategy = export.ConflictSkip
	}

	importer := export.NewImporter(s.store)

	var result *export.ImportResult
	var err error

	if req.DryRun {
		result, err = importer.DryRun(ctx, &req.Bundle, req.Strategy)
	} else {
		result, err = importer.Import(ctx, &req.Bundle, req.Strategy)
	}

	if err != nil {
		s.logger.Error("Failed to import recipes", zap.Error(err))
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !req.DryRun {
		s.logger.Info("Recipe data imported",
			zap.Int("components_imported", result.ComponentsImported),
			zap.Int("playbooks_imported", result.PlaybooksImported))
	}

	s.jsonResponse(w, map[string]interface{}{
		"dry_run": req.DryRun,
		"result":  result,
	})
}

// requireAdmin checks if the current user is an admin and returns error if not.
func (s *MasterServer) requireAdmin(ctx context.Context, w http.ResponseWriter) bool {
	user, ok := GetUserFromContext(ctx)
	if !ok || user == nil {
		s.jsonError(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	if user.Role != "admin" {
		s.jsonError(w, http.StatusForbidden, "admin access required")
		return false
	}
	return true
}

// --- Migration API ---

// handleMigrationPreview handles GET /api/v1/recipes/migration/preview/{project_id}.
func (s *MasterServer) handleMigrationPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Extract project ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/recipes/migration/preview/")
	projectID := path
	if projectID == "" {
		s.jsonError(w, http.StatusBadRequest, "invalid project ID")
		return
	}

	// Get project
	project, err := s.store.GetProjectByID(ctx, projectID)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "project not found")
		return
	}

	// Create a basic preview
	// Note: For a full preview, the project's YAML config file would need to be loaded
	// This provides a basic preview based on project type
	preview := &recipes.MigrationPreview{
		ProjectName:     project.Name,
		ProjectType:     derefStr(project.TypeID),
		PreDeployHooks:  0,
		PostDeployHooks: 0,
		ReloadActions:   0,
		RollbackHooks:   0,
		TotalComponents: 0,
		Warnings:        []string{},
	}

	// Estimate components based on project type
	switch derefStr(project.TypeID) {
	case "laravel", "php":
		preview.PreDeployHooks = 2  // composer install, cache clear
		preview.PostDeployHooks = 3 // migrations, cache warm, queue restart
		preview.ReloadActions = 1   // php-fpm reload
		preview.Warnings = append(preview.Warnings, "Estimated based on Laravel project type")
	case "rails", "ruby":
		preview.PreDeployHooks = 2
		preview.PostDeployHooks = 2
		preview.ReloadActions = 1
		preview.Warnings = append(preview.Warnings, "Estimated based on Rails project type")
	case "node", "nodejs":
		preview.PreDeployHooks = 1
		preview.PostDeployHooks = 1
		preview.ReloadActions = 1
		preview.Warnings = append(preview.Warnings, "Estimated based on Node.js project type")
	case "static":
		preview.PreDeployHooks = 0
		preview.PostDeployHooks = 0
		preview.ReloadActions = 0
		preview.Warnings = append(preview.Warnings, "Static projects typically have no hooks")
	default:
		preview.Warnings = append(preview.Warnings, "Unknown project type - migration may need manual configuration")
	}

	preview.TotalComponents = preview.PreDeployHooks + preview.PostDeployHooks + preview.ReloadActions + preview.RollbackHooks

	// Check if a playbook already exists for this project
	existingActivation, err := s.activationService.GetActive(ctx, projectID)
	if err == nil && existingActivation != nil {
		preview.Warnings = append(preview.Warnings, "Project already has an active playbook - this will be replaced")
	}

	s.jsonResponse(w, preview)
}

// handleMigration handles POST /api/v1/recipes/migration/{project_id}.
// This creates a new playbook for the project based on its type.
// For actual YAML migration with existing config files, use the CLI: vcdeploy recipes import-yaml
func (s *MasterServer) handleMigration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutLong)
	defer cancel()

	// Extract project ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/recipes/migration/")
	path = strings.TrimSuffix(path, "/")
	projectID := path
	if projectID == "" {
		s.jsonError(w, http.StatusBadRequest, "invalid project ID")
		return
	}

	// Parse request body
	var req struct {
		Name             string `json:"name"`
		Version          string `json:"version"`
		CreateComponents bool   `json:"createComponents"`
		Activate         bool   `json:"activate"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get project
	project, err := s.store.GetProjectByID(ctx, projectID)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "project not found")
		return
	}

	// Create an empty playbook for the project
	playbookName := req.Name
	if playbookName == "" {
		playbookName = project.Name + "-playbook"
	}
	playbookVersion := req.Version
	if playbookVersion == "" {
		playbookVersion = "v1.0.0"
	}

	// Create a slug from the name
	slug := strings.ToLower(strings.ReplaceAll(playbookName, " ", "-"))

	// Create the playbook
	playbook := &storage.Playbook{
		Name:        playbookName,
		Slug:        slug,
		Description: fmt.Sprintf("Deployment playbook for %s (%s)", project.Name, derefStr(project.TypeID)),
		Version:     playbookVersion,
		Namespace:   "user",
		Steps:       []storage.PlaybookStep{},
		CreatedAt:   time.Now(),
	}

	// Create the playbook
	if err := s.store.CreatePlaybook(ctx, playbook); err != nil {
		s.logger.Error("Failed to create playbook", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to create playbook")
		return
	}

	var activation *storage.PlaybookActivation

	// Optionally activate the playbook
	if req.Activate {
		activation = &storage.PlaybookActivation{
			ProjectID:   projectID,
			PlaybookID:  playbook.ID,
			ActivatedAt: time.Now(),
		}

		if err := s.store.CreatePlaybookActivation(ctx, activation); err != nil {
			s.logger.Error("Failed to activate playbook", zap.Error(err))
			// Continue - playbook was created, just not activated
		}
	}

	s.logger.Info("Created playbook for project",
		zap.String("project_id", projectID),
		zap.String("playbook_name", playbook.Name))

	s.jsonResponse(w, map[string]interface{}{
		"success":   true,
		"playbook":  playbook,
		"activated": activation != nil,
		"message":   "Playbook created. Use the Playbook Composer to add deployment steps.",
	})
}
