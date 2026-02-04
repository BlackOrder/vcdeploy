// Package server provides recipe UI handlers for the master server.
package server

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/BlackOrder/vcdeploy/internal/services/recipes"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/storage/seeds"
	"go.uber.org/zap"
)

// handleRecipesUI renders the recipe components page.
func (s *MasterServer) handleRecipesUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user components
	var userComponents []map[string]interface{}
	if s.componentService != nil {
		components, err := s.componentService.List(ctx, "user", false)
		if err != nil {
			s.logger.Error("Failed to list user components", zap.Error(err))
		} else {
			for _, c := range components {
				userComponents = append(userComponents, map[string]interface{}{
					"ID":            c.ID,
					"Slug":          c.Slug,
					"Name":          c.Name,
					"Version":       c.Version,
					"Description":   c.Description,
					"ComponentType": c.ComponentType,
					"IsRaw":         c.IsRaw,
					"IsDeprecated":  c.IsDeprecated,
				})
			}
		}
	}

	// Get seed components
	var seedComponents []map[string]interface{}
	if s.componentService != nil {
		components, err := s.componentService.List(ctx, "seed", false)
		if err != nil {
			s.logger.Error("Failed to list seed components", zap.Error(err))
		} else {
			for _, c := range components {
				seedComponents = append(seedComponents, map[string]interface{}{
					"ID":            c.ID,
					"Slug":          c.Slug,
					"Name":          c.Name,
					"Version":       c.Version,
					"Description":   c.Description,
					"ComponentType": c.ComponentType,
					"IsRaw":         c.IsRaw,
				})
			}
		}
	}

	data := s.withCommonData(r, map[string]interface{}{
		"Title":            "Recipe Components",
		"Active":           "recipes",
		"UserComponents":   userComponents,
		"SeedComponents":   seedComponents,
		"EmptyUserTitle":   "No Custom Components",
		"EmptyUserMessage": seeds.GetEmptyComponentsMessage(),
		"EmptySeedTitle":   seeds.GetEmptyStateTitle(),
		"EmptySeedMessage": seeds.GetEmptyStateMessage(),
	})

	s.renderTemplate(w, "recipes", data)
}

// handlePlaybooksUI renders the playbooks page.
func (s *MasterServer) handlePlaybooksUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user playbooks
	var userPlaybooks []map[string]interface{}
	if s.playbookService != nil {
		playbooks, err := s.playbookService.List(ctx, "user", "", false)
		if err != nil {
			s.logger.Error("Failed to list user playbooks", zap.Error(err))
		} else {
			for _, p := range playbooks {
				userPlaybooks = append(userPlaybooks, map[string]interface{}{
					"ID":            p.ID,
					"Slug":          p.Slug,
					"Name":          p.Name,
					"Version":       p.Version,
					"Description":   p.Description,
					"FrameworkType": p.FrameworkType,
					"Steps":         p.Steps,
					"IsDeprecated":  p.IsDeprecated,
					"ParentID":      p.ParentID,
				})
			}
		}
	}

	// Get seed playbooks
	var seedPlaybooks []map[string]interface{}
	if s.playbookService != nil {
		playbooks, err := s.playbookService.List(ctx, "seed", "", false)
		if err != nil {
			s.logger.Error("Failed to list seed playbooks", zap.Error(err))
		} else {
			for _, p := range playbooks {
				seedPlaybooks = append(seedPlaybooks, map[string]interface{}{
					"ID":            p.ID,
					"Slug":          p.Slug,
					"Name":          p.Name,
					"Version":       p.Version,
					"Description":   p.Description,
					"FrameworkType": p.FrameworkType,
					"Steps":         p.Steps,
				})
			}
		}
	}

	data := s.withCommonData(r, map[string]interface{}{
		"Title":            "Playbooks",
		"Active":           "playbooks",
		"UserPlaybooks":    userPlaybooks,
		"SeedPlaybooks":    seedPlaybooks,
		"EmptyUserTitle":   "No Custom Playbooks",
		"EmptyUserMessage": seeds.EmptyPlaybooksMessage,
		"EmptySeedTitle":   seeds.GetEmptyStateTitle(),
		"EmptySeedMessage": seeds.GetEmptyStateMessage(),
	})

	s.renderTemplate(w, "playbooks", data)
}

// handleRawApprovalsUI renders the RAW command approvals page (admin only).
func (s *MasterServer) handleRawApprovalsUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if user is admin
	user, ok := GetUserFromContext(ctx)
	if !ok || user == nil || user.Role != "admin" {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	// Get pending approvals
	var pendingApprovals []map[string]interface{}
	var approvedComponents []map[string]interface{}
	if s.rawApprovalService != nil {
		pending, err := s.rawApprovalService.ListPendingApprovals(ctx)
		if err != nil {
			s.logger.Error("Failed to list pending approvals", zap.Error(err))
		} else {
			for _, c := range pending {
				pendingApprovals = append(pendingApprovals, map[string]interface{}{
					"ID":            c.ID,
					"Name":          c.Name,
					"Slug":          c.Slug,
					"Version":       c.Version,
					"Description":   c.Description,
					"ComponentType": c.ComponentType,
					"Namespace":     c.Namespace,
				})
			}
		}

		approved, err := s.rawApprovalService.ListAllApprovals(ctx)
		if err != nil {
			s.logger.Error("Failed to list approved components", zap.Error(err))
		} else {
			for _, a := range approved {
				// Get component details
				if s.componentService != nil {
					comp, err := s.componentService.GetByID(ctx, a.ComponentID)
					if err == nil && comp != nil {
						approvedComponents = append(approvedComponents, map[string]interface{}{
							"ApprovalID":   a.ID,
							"ComponentID":  a.ComponentID,
							"Name":         comp.Name,
							"Slug":         comp.Slug,
							"Version":      comp.Version,
							"ApprovedAt":   a.ApprovedAt,
							"ApprovedBy":   a.ApprovedBy,
							"ApprovalNote": a.ApprovalNote,
						})
					}
				}
			}
		}
	}

	data := s.withCommonData(r, map[string]interface{}{
		"Title":              "RAW Command Approvals",
		"Active":             "recipes",
		"PendingApprovals":   pendingApprovals,
		"ApprovedComponents": approvedComponents,
	})

	s.renderTemplate(w, "raw-approvals", data)
}

// handleComponentDetailPartial returns component details as HTML partial.
func (s *MasterServer) handleComponentDetailPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse component ID from path: /partials/recipes/components/{id}
	path := strings.TrimPrefix(r.URL.Path, "/partials/recipes/components/")
	path = strings.TrimSuffix(path, "/")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "invalid component ID", http.StatusBadRequest)
		return
	}

	if s.componentService == nil {
		http.Error(w, "service not available", http.StatusServiceUnavailable)
		return
	}

	component, err := s.componentService.GetByID(ctx, id)
	if err != nil {
		http.Error(w, "component not found", http.StatusNotFound)
		return
	}

	// Get approval status if RAW
	var approvalStatus *recipes.ApprovalStatus
	if component.IsRaw && s.rawApprovalService != nil {
		approvalStatus, _ = s.rawApprovalService.GetApprovalStatus(ctx, id)
	}

	// Get all versions
	var versions []string
	allVersions, err := s.componentService.GetVersions(ctx, component.Namespace, component.Slug)
	if err == nil {
		for _, v := range allVersions {
			versions = append(versions, v.Version)
		}
	}

	// Render partial HTML
	w.Header().Set("Content-Type", "text/html")
	s.renderComponentDetailPartial(w, component, approvalStatus, versions)
}

// renderComponentDetailPartial renders the component detail HTML.
func (s *MasterServer) renderComponentDetailPartial(w http.ResponseWriter, c *storage.RecipeComponent, approvalStatus *recipes.ApprovalStatus, versions []string) {
	var sb strings.Builder

	// Header with name and version
	sb.WriteString(`<div class="space-y-6">`)
	sb.WriteString(`<div class="flex items-start justify-between">`)
	sb.WriteString(`<div>`)
	sb.WriteString(fmt.Sprintf(`<h3 class="text-lg font-medium text-dark-text">%s</h3>`, template.HTMLEscapeString(c.Name)))
	sb.WriteString(fmt.Sprintf(`<p class="text-sm text-dark-muted mt-1">%s:%s</p>`, template.HTMLEscapeString(c.Namespace), template.HTMLEscapeString(c.Slug)))
	sb.WriteString(`</div>`)
	sb.WriteString(`<div class="flex items-center space-x-2">`)
	sb.WriteString(fmt.Sprintf(`<span class="inline-flex items-center px-2.5 py-0.5 rounded text-sm font-medium bg-accent-500/20 text-accent-400">%s</span>`, template.HTMLEscapeString(c.Version)))
	sb.WriteString(fmt.Sprintf(`<span class="inline-flex items-center px-2.5 py-0.5 rounded text-sm font-medium bg-dark-bg text-dark-muted">%s</span>`, template.HTMLEscapeString(c.ComponentType)))
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	// Description
	if c.Description != "" {
		sb.WriteString(fmt.Sprintf(`<p class="text-sm text-dark-muted">%s</p>`, template.HTMLEscapeString(c.Description)))
	}

	// RAW warning
	if c.IsRaw {
		sb.WriteString(`<div class="bg-red-500/10 border border-red-500/50 rounded-lg p-4">`)
		sb.WriteString(`<div class="flex">`)
		sb.WriteString(`<svg class="h-5 w-5 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"></path></svg>`)
		sb.WriteString(`<div class="ml-3">`)
		sb.WriteString(`<h4 class="text-sm font-medium text-red-400">RAW Component</h4>`)
		if approvalStatus != nil && approvalStatus.IsApproved {
			sb.WriteString(fmt.Sprintf(`<p class="mt-1 text-sm text-green-300">Approved on %s</p>`, approvalStatus.ApprovedAt.Format("Jan 2, 2006")))
			if approvalStatus.ApprovalNote != "" {
				sb.WriteString(fmt.Sprintf(`<p class="mt-1 text-sm text-dark-muted">Note: %s</p>`, template.HTMLEscapeString(approvalStatus.ApprovalNote)))
			}
		} else {
			sb.WriteString(`<p class="mt-1 text-sm text-red-300">Requires admin approval before use</p>`)
		}
		sb.WriteString(`</div>`)
		sb.WriteString(`</div>`)
		sb.WriteString(`</div>`)
	}

	// Commands
	sb.WriteString(`<div>`)
	sb.WriteString(`<h4 class="text-sm font-medium text-dark-muted mb-2">Commands</h4>`)
	sb.WriteString(`<div class="bg-dark-bg border border-dark-border rounded-lg p-4 font-mono text-sm">`)
	for _, cmd := range c.Content.Commands {
		sb.WriteString(fmt.Sprintf(`<div class="text-dark-text">%s</div>`, template.HTMLEscapeString(cmd)))
	}
	if len(c.Content.Commands) == 0 {
		sb.WriteString(`<div class="text-dark-muted italic">No commands defined</div>`)
	}
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	// Variables
	if len(c.Variables) > 0 {
		sb.WriteString(`<div>`)
		sb.WriteString(`<h4 class="text-sm font-medium text-dark-muted mb-2">Variables</h4>`)
		sb.WriteString(`<div class="space-y-2">`)
		for _, v := range c.Variables {
			sb.WriteString(`<div class="flex items-center justify-between bg-dark-bg border border-dark-border rounded-lg p-3">`)
			sb.WriteString(`<div>`)
			sb.WriteString(fmt.Sprintf(`<span class="font-mono text-sm text-dark-text">{{%s}}</span>`, template.HTMLEscapeString(v.Name)))
			if v.Description != "" {
				sb.WriteString(fmt.Sprintf(`<p class="text-xs text-dark-muted mt-1">%s</p>`, template.HTMLEscapeString(v.Description)))
			}
			sb.WriteString(`</div>`)
			sb.WriteString(`<div class="flex items-center space-x-2">`)
			sb.WriteString(fmt.Sprintf(`<span class="text-xs text-dark-muted">%s</span>`, template.HTMLEscapeString(v.Type)))
			if v.Required {
				sb.WriteString(`<span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-red-500/20 text-red-400">Required</span>`)
			}
			sb.WriteString(`</div>`)
			sb.WriteString(`</div>`)
		}
		sb.WriteString(`</div>`)
		sb.WriteString(`</div>`)
	}

	// Version history dropdown
	if len(versions) > 1 {
		sb.WriteString(`<div>`)
		sb.WriteString(`<h4 class="text-sm font-medium text-dark-muted mb-2">Version History</h4>`)
		sb.WriteString(`<select class="px-3 py-2 bg-dark-bg border border-dark-border rounded-md text-dark-text text-sm focus:outline-none focus:ring-2 focus:ring-accent-500">`)
		for _, v := range versions {
			selected := ""
			if v == c.Version {
				selected = " selected"
			}
			sb.WriteString(fmt.Sprintf(`<option value="%s"%s>%s</option>`, template.HTMLEscapeString(v), selected, template.HTMLEscapeString(v)))
		}
		sb.WriteString(`</select>`)
		sb.WriteString(`</div>`)
	}

	// Actions (only for user components)
	if c.Namespace == "user" {
		sb.WriteString(`<div class="flex justify-end space-x-3 pt-4 border-t border-dark-border">`)
		sb.WriteString(fmt.Sprintf(`<button onclick="editComponent(%d)" class="px-4 py-2 text-sm font-medium text-dark-muted hover:text-white border border-dark-border rounded-md">Edit</button>`, c.ID))
		sb.WriteString(fmt.Sprintf(`<button onclick="deleteComponent(%d)" class="px-4 py-2 text-sm font-medium text-red-400 hover:text-red-300 border border-red-500/50 rounded-md">Delete</button>`, c.ID))
		sb.WriteString(`</div>`)
	}

	sb.WriteString(`</div>`)

	w.Write([]byte(sb.String()))
}

// handlePlaybookDetailPartial returns playbook details as HTML partial.
func (s *MasterServer) handlePlaybookDetailPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse playbook ID from path: /partials/recipes/playbooks/{id}
	path := strings.TrimPrefix(r.URL.Path, "/partials/recipes/playbooks/")
	path = strings.TrimSuffix(path, "/")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "invalid playbook ID", http.StatusBadRequest)
		return
	}

	if s.playbookService == nil {
		http.Error(w, "service not available", http.StatusServiceUnavailable)
		return
	}

	playbook, err := s.playbookService.GetByID(ctx, id)
	if err != nil {
		http.Error(w, "playbook not found", http.StatusNotFound)
		return
	}

	// Get all versions
	var versions []string
	allVersions, err := s.playbookService.GetVersions(ctx, playbook.Namespace, playbook.Slug)
	if err == nil {
		for _, v := range allVersions {
			versions = append(versions, v.Version)
		}
	}

	// Render partial HTML
	w.Header().Set("Content-Type", "text/html")
	s.renderPlaybookDetailPartial(w, playbook, versions)
}

// renderPlaybookDetailPartial renders the playbook detail HTML.
func (s *MasterServer) renderPlaybookDetailPartial(w http.ResponseWriter, p *storage.Playbook, versions []string) {
	var sb strings.Builder

	// Header with name and version
	sb.WriteString(`<div class="space-y-6">`)
	sb.WriteString(`<div class="flex items-start justify-between">`)
	sb.WriteString(`<div>`)
	sb.WriteString(fmt.Sprintf(`<h3 class="text-lg font-medium text-dark-text">%s</h3>`, template.HTMLEscapeString(p.Name)))
	sb.WriteString(fmt.Sprintf(`<p class="text-sm text-dark-muted mt-1">%s:%s</p>`, template.HTMLEscapeString(p.Namespace), template.HTMLEscapeString(p.Slug)))
	sb.WriteString(`</div>`)
	sb.WriteString(`<div class="flex items-center space-x-2">`)
	sb.WriteString(fmt.Sprintf(`<span class="inline-flex items-center px-2.5 py-0.5 rounded text-sm font-medium bg-accent-500/20 text-accent-400">%s</span>`, template.HTMLEscapeString(p.Version)))
	if p.FrameworkType != "" {
		sb.WriteString(fmt.Sprintf(`<span class="inline-flex items-center px-2.5 py-0.5 rounded text-sm font-medium bg-dark-bg text-dark-muted">%s</span>`, template.HTMLEscapeString(p.FrameworkType)))
	}
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	// Description
	if p.Description != "" {
		sb.WriteString(fmt.Sprintf(`<p class="text-sm text-dark-muted">%s</p>`, template.HTMLEscapeString(p.Description)))
	}

	// Deployment Settings
	sb.WriteString(`<div class="grid grid-cols-2 gap-4">`)
	sb.WriteString(`<div class="bg-dark-bg border border-dark-border rounded-lg p-4">`)
	sb.WriteString(`<h4 class="text-xs font-medium text-dark-muted uppercase tracking-wider mb-2">Keep Releases</h4>`)
	sb.WriteString(fmt.Sprintf(`<p class="text-lg font-medium text-dark-text">%d</p>`, p.KeepReleases))
	sb.WriteString(`</div>`)
	sb.WriteString(`<div class="bg-dark-bg border border-dark-border rounded-lg p-4">`)
	sb.WriteString(`<h4 class="text-xs font-medium text-dark-muted uppercase tracking-wider mb-2">Steps</h4>`)
	sb.WriteString(fmt.Sprintf(`<p class="text-lg font-medium text-dark-text">%d</p>`, len(p.Steps)))
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	// Steps
	sb.WriteString(`<div>`)
	sb.WriteString(`<h4 class="text-sm font-medium text-dark-muted mb-3">Deployment Steps</h4>`)
	if len(p.Steps) > 0 {
		sb.WriteString(`<div class="space-y-2">`)
		for i, step := range p.Steps {
			sb.WriteString(`<div class="flex items-center space-x-3 bg-dark-bg border border-dark-border rounded-lg p-3">`)
			sb.WriteString(fmt.Sprintf(`<span class="flex-shrink-0 w-6 h-6 rounded-full bg-accent-500/20 text-accent-400 flex items-center justify-center text-xs font-medium">%d</span>`, i+1))
			sb.WriteString(`<div class="flex-1 min-w-0">`)
			sb.WriteString(fmt.Sprintf(`<p class="text-sm font-mono text-dark-text truncate">%s</p>`, template.HTMLEscapeString(step.ComponentRef)))
			sb.WriteString(fmt.Sprintf(`<p class="text-xs text-dark-muted">Phase: %s</p>`, template.HTMLEscapeString(step.Phase)))
			sb.WriteString(`</div>`)
			sb.WriteString(`</div>`)
		}
		sb.WriteString(`</div>`)
	} else {
		sb.WriteString(`<div class="text-center py-6 bg-dark-bg border border-dark-border rounded-lg">`)
		sb.WriteString(`<p class="text-sm text-dark-muted">No steps defined. Add steps using the playbook composer.</p>`)
		sb.WriteString(`</div>`)
	}
	sb.WriteString(`</div>`)

	// Shared directories
	if len(p.SharedDirs) > 0 {
		sb.WriteString(`<div>`)
		sb.WriteString(`<h4 class="text-sm font-medium text-dark-muted mb-2">Shared Directories</h4>`)
		sb.WriteString(`<div class="flex flex-wrap gap-2">`)
		for _, dir := range p.SharedDirs {
			sb.WriteString(fmt.Sprintf(`<span class="inline-flex items-center px-2 py-1 rounded text-xs font-mono bg-dark-bg text-dark-muted">%s</span>`, template.HTMLEscapeString(dir)))
		}
		sb.WriteString(`</div>`)
		sb.WriteString(`</div>`)
	}

	// Version history dropdown
	if len(versions) > 1 {
		sb.WriteString(`<div>`)
		sb.WriteString(`<h4 class="text-sm font-medium text-dark-muted mb-2">Version History</h4>`)
		sb.WriteString(`<select class="px-3 py-2 bg-dark-bg border border-dark-border rounded-md text-dark-text text-sm focus:outline-none focus:ring-2 focus:ring-accent-500">`)
		for _, v := range versions {
			selected := ""
			if v == p.Version {
				selected = " selected"
			}
			sb.WriteString(fmt.Sprintf(`<option value="%s"%s>%s</option>`, template.HTMLEscapeString(v), selected, template.HTMLEscapeString(v)))
		}
		sb.WriteString(`</select>`)
		sb.WriteString(`</div>`)
	}

	// Actions (only for user playbooks)
	if p.Namespace == "user" {
		sb.WriteString(`<div class="flex justify-end space-x-3 pt-4 border-t border-dark-border">`)
		sb.WriteString(fmt.Sprintf(`<button onclick="editPlaybook(%d)" class="px-4 py-2 text-sm font-medium text-dark-muted hover:text-white border border-dark-border rounded-md">Edit</button>`, p.ID))
		sb.WriteString(fmt.Sprintf(`<button onclick="deletePlaybook(%d)" class="px-4 py-2 text-sm font-medium text-red-400 hover:text-red-300 border border-red-500/50 rounded-md">Delete</button>`, p.ID))
		sb.WriteString(`</div>`)
	}

	sb.WriteString(`</div>`)

	w.Write([]byte(sb.String()))
}

// handlePlaybookComposerUI renders the playbook composer page for editing playbook steps.
func (s *MasterServer) handlePlaybookComposerUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse playbook ID from query param
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		// New playbook - redirect to playbooks page
		http.Redirect(w, r, "/playbooks", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid playbook ID", http.StatusBadRequest)
		return
	}

	if s.playbookService == nil {
		http.Error(w, "Service not available", http.StatusServiceUnavailable)
		return
	}

	// Get the playbook
	playbook, err := s.playbookService.GetByID(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get playbook", zap.Error(err), zap.Int64("id", id))
		http.Error(w, "Playbook not found", http.StatusNotFound)
		return
	}

	// Only user playbooks can be edited
	if playbook.Namespace != "user" {
		http.Error(w, "Cannot edit seed playbooks directly", http.StatusForbidden)
		return
	}

	// Get all available components (both seed and user)
	var allComponents []map[string]interface{}
	if s.componentService != nil {
		// Get seed components
		seedComponents, err := s.componentService.List(ctx, "seed", false)
		if err == nil {
			for _, c := range seedComponents {
				allComponents = append(allComponents, map[string]interface{}{
					"ID":        c.ID,
					"Namespace": c.Namespace,
					"Slug":      c.Slug,
					"Name":      c.Name,
					"Version":   c.Version,
					"IsRaw":     c.IsRaw,
				})
			}
		}

		// Get user components
		userComponents, err := s.componentService.List(ctx, "user", false)
		if err == nil {
			for _, c := range userComponents {
				allComponents = append(allComponents, map[string]interface{}{
					"ID":        c.ID,
					"Namespace": c.Namespace,
					"Slug":      c.Slug,
					"Name":      c.Name,
					"Version":   c.Version,
					"IsRaw":     c.IsRaw,
				})
			}
		}
	}

	// Convert playbook steps to template format
	var steps []map[string]interface{}
	for _, step := range playbook.Steps {
		steps = append(steps, map[string]interface{}{
			"Order":            step.Order,
			"ComponentRef":     step.ComponentRef,
			"Phase":            step.Phase,
			"Condition":        step.Condition,
			"VariableBindings": step.VariableBindings,
		})
	}

	data := s.withCommonData(r, map[string]interface{}{
		"Title":      "Edit Playbook - " + playbook.Name,
		"Active":     "playbooks",
		"Playbook":   playbook,
		"Steps":      steps,
		"Components": allComponents,
	})

	s.renderTemplate(w, "playbook-composer", data)
}
