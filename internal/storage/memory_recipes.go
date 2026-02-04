package storage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Recipe key helper functions

// recipeComponentKey returns the map key for a recipe component.
func recipeComponentKey(namespace, slug, version string) string {
	return namespace + ":" + slug + ":" + version
}

// playbookKey returns the map key for a playbook.
func playbookKey(namespace, slug, version string) string {
	return namespace + ":" + slug + ":" + version
}

// bindingSourceKey returns the map key for a variable binding by source.
func bindingSourceKey(sourceType, sourceRef string) string {
	return sourceType + ":" + sourceRef
}

// --- RecipeComponent operations ---

// CreateRecipeComponent creates a new recipe component.
func (m *MemoryStore) CreateRecipeComponent(ctx context.Context, c *RecipeComponent) error {
	if c == nil {
		return fmt.Errorf("recipe component cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate key
	key := recipeComponentKey(c.Namespace, c.Slug, c.Version)
	if _, exists := m.recipeComponentsByKey[key]; exists {
		return fmt.Errorf("recipe component with key %s already exists", key)
	}

	// Assign ID if not set
	if c.ID == 0 {
		c.ID = nextID(&m.nextRecipeComponentID)
	}

	// Set created time if not set
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}

	// Store a copy
	stored := *c
	m.recipeComponents[c.ID] = &stored
	m.recipeComponentsByKey[key] = &stored

	// Queue for persistence
	m.queueWrite(m.coreWrites, NewWriteOp(WriteOpInsert, "recipe_components", &stored))

	return nil
}

// GetRecipeComponent returns a component by namespace, slug, and version.
func (m *MemoryStore) GetRecipeComponent(ctx context.Context, namespace, slug, version string) (*RecipeComponent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := recipeComponentKey(namespace, slug, version)
	c, exists := m.recipeComponentsByKey[key]
	if !exists {
		return nil, ErrNotFound
	}

	// Return a copy
	result := *c
	return &result, nil
}

// GetRecipeComponentByID returns a component by ID.
func (m *MemoryStore) GetRecipeComponentByID(ctx context.Context, id int64) (*RecipeComponent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, exists := m.recipeComponents[id]
	if !exists {
		return nil, ErrNotFound
	}

	// Return a copy
	result := *c
	return &result, nil
}

// ListRecipeComponents returns all components in a namespace.
func (m *MemoryStore) ListRecipeComponents(ctx context.Context, namespace string, includeDeprecated bool) ([]*RecipeComponent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*RecipeComponent
	for _, c := range m.recipeComponents {
		// Filter by namespace if specified
		if namespace != "" && c.Namespace != namespace {
			continue
		}
		// Filter deprecated unless explicitly requested
		if !includeDeprecated && c.IsDeprecated {
			continue
		}
		// Return a copy
		copy := *c
		result = append(result, &copy)
	}

	// Sort by namespace, slug, version for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		if result[i].Slug != result[j].Slug {
			return result[i].Slug < result[j].Slug
		}
		return result[i].Version < result[j].Version
	})

	return result, nil
}

// ListRecipeComponentVersions returns all versions of a component.
func (m *MemoryStore) ListRecipeComponentVersions(ctx context.Context, namespace, slug string) ([]*RecipeComponent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*RecipeComponent
	for _, c := range m.recipeComponents {
		if c.Namespace == namespace && c.Slug == slug {
			// Return a copy
			copy := *c
			result = append(result, &copy)
		}
	}

	// Sort by version (descending for latest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Version > result[j].Version
	})

	return result, nil
}

// UpdateRecipeComponent updates an existing component.
func (m *MemoryStore) UpdateRecipeComponent(ctx context.Context, c *RecipeComponent) error {
	if c == nil {
		return fmt.Errorf("recipe component cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.recipeComponents[c.ID]
	if !exists {
		return ErrNotFound
	}

	// Remove old key if changed
	oldKey := recipeComponentKey(existing.Namespace, existing.Slug, existing.Version)
	newKey := recipeComponentKey(c.Namespace, c.Slug, c.Version)
	if oldKey != newKey {
		delete(m.recipeComponentsByKey, oldKey)
		// Check new key doesn't exist
		if _, exists := m.recipeComponentsByKey[newKey]; exists {
			// Restore old key on conflict
			m.recipeComponentsByKey[oldKey] = existing
			return fmt.Errorf("recipe component with key %s already exists", newKey)
		}
	}

	// Store updated copy
	stored := *c
	m.recipeComponents[c.ID] = &stored
	m.recipeComponentsByKey[newKey] = &stored

	// Queue for persistence
	m.queueWrite(m.coreWrites, NewWriteOp(WriteOpUpdate, "recipe_components", &stored))

	return nil
}

// DeleteRecipeComponent deletes a component by ID.
func (m *MemoryStore) DeleteRecipeComponent(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, exists := m.recipeComponents[id]
	if !exists {
		return ErrNotFound
	}

	// Remove from both maps
	delete(m.recipeComponents, id)
	key := recipeComponentKey(c.Namespace, c.Slug, c.Version)
	delete(m.recipeComponentsByKey, key)

	// Queue for persistence
	m.queueWrite(m.coreWrites, NewWriteOp(WriteOpDelete, "recipe_components", id))

	return nil
}

// --- Playbook operations ---

// CreatePlaybook creates a new playbook.
func (m *MemoryStore) CreatePlaybook(ctx context.Context, p *Playbook) error {
	if p == nil {
		return fmt.Errorf("playbook cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate key
	key := playbookKey(p.Namespace, p.Slug, p.Version)
	if _, exists := m.playbooksByKey[key]; exists {
		return fmt.Errorf("playbook with key %s already exists", key)
	}

	// Assign ID if not set
	if p.ID == 0 {
		p.ID = nextID(&m.nextPlaybookID)
	}

	// Set created time if not set
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}

	// Store a copy
	stored := *p
	// Deep copy slices
	if p.Steps != nil {
		stored.Steps = make([]PlaybookStep, len(p.Steps))
		copy(stored.Steps, p.Steps)
	}
	if p.SharedDirs != nil {
		stored.SharedDirs = make([]string, len(p.SharedDirs))
		copy(stored.SharedDirs, p.SharedDirs)
	}
	if p.SharedFiles != nil {
		stored.SharedFiles = make([]string, len(p.SharedFiles))
		copy(stored.SharedFiles, p.SharedFiles)
	}
	if p.WritableDirs != nil {
		stored.WritableDirs = make([]string, len(p.WritableDirs))
		copy(stored.WritableDirs, p.WritableDirs)
	}
	if p.ValidationRules != nil {
		rules := *p.ValidationRules
		stored.ValidationRules = &rules
	}

	m.playbooks[p.ID] = &stored
	m.playbooksByKey[key] = &stored

	// Queue for persistence
	m.queueWrite(m.coreWrites, NewWriteOp(WriteOpInsert, "playbooks", &stored))

	return nil
}

// GetPlaybook returns a playbook by namespace, slug, and version.
func (m *MemoryStore) GetPlaybook(ctx context.Context, namespace, slug, version string) (*Playbook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := playbookKey(namespace, slug, version)
	p, exists := m.playbooksByKey[key]
	if !exists {
		return nil, ErrNotFound
	}

	// Return a copy
	return copyPlaybook(p), nil
}

// GetPlaybookByID returns a playbook by ID.
func (m *MemoryStore) GetPlaybookByID(ctx context.Context, id int64) (*Playbook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, exists := m.playbooks[id]
	if !exists {
		return nil, ErrNotFound
	}

	// Return a copy
	return copyPlaybook(p), nil
}

// ListPlaybooks returns playbooks filtered by namespace and/or framework type.
func (m *MemoryStore) ListPlaybooks(ctx context.Context, namespace, frameworkType string, includeDeprecated bool) ([]*Playbook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Playbook
	for _, p := range m.playbooks {
		// Filter by namespace if specified
		if namespace != "" && p.Namespace != namespace {
			continue
		}
		// Filter by framework type if specified
		if frameworkType != "" && p.FrameworkType != frameworkType {
			continue
		}
		// Filter deprecated unless explicitly requested
		if !includeDeprecated && p.IsDeprecated {
			continue
		}
		result = append(result, copyPlaybook(p))
	}

	// Sort by namespace, slug, version for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		if result[i].Slug != result[j].Slug {
			return result[i].Slug < result[j].Slug
		}
		return result[i].Version < result[j].Version
	})

	return result, nil
}

// ListPlaybookVersions returns all versions of a playbook.
func (m *MemoryStore) ListPlaybookVersions(ctx context.Context, namespace, slug string) ([]*Playbook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Playbook
	for _, p := range m.playbooks {
		if p.Namespace == namespace && p.Slug == slug {
			result = append(result, copyPlaybook(p))
		}
	}

	// Sort by version (descending for latest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Version > result[j].Version
	})

	return result, nil
}

// UpdatePlaybook updates an existing playbook.
func (m *MemoryStore) UpdatePlaybook(ctx context.Context, p *Playbook) error {
	if p == nil {
		return fmt.Errorf("playbook cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.playbooks[p.ID]
	if !exists {
		return ErrNotFound
	}

	// Remove old key if changed
	oldKey := playbookKey(existing.Namespace, existing.Slug, existing.Version)
	newKey := playbookKey(p.Namespace, p.Slug, p.Version)
	if oldKey != newKey {
		delete(m.playbooksByKey, oldKey)
		// Check new key doesn't exist
		if _, exists := m.playbooksByKey[newKey]; exists {
			// Restore old key on conflict
			m.playbooksByKey[oldKey] = existing
			return fmt.Errorf("playbook with key %s already exists", newKey)
		}
	}

	// Store updated copy
	stored := *p
	// Deep copy slices
	if p.Steps != nil {
		stored.Steps = make([]PlaybookStep, len(p.Steps))
		copy(stored.Steps, p.Steps)
	}
	if p.SharedDirs != nil {
		stored.SharedDirs = make([]string, len(p.SharedDirs))
		copy(stored.SharedDirs, p.SharedDirs)
	}
	if p.SharedFiles != nil {
		stored.SharedFiles = make([]string, len(p.SharedFiles))
		copy(stored.SharedFiles, p.SharedFiles)
	}
	if p.WritableDirs != nil {
		stored.WritableDirs = make([]string, len(p.WritableDirs))
		copy(stored.WritableDirs, p.WritableDirs)
	}
	if p.ValidationRules != nil {
		rules := *p.ValidationRules
		stored.ValidationRules = &rules
	}

	m.playbooks[p.ID] = &stored
	m.playbooksByKey[newKey] = &stored

	// Queue for persistence
	m.queueWrite(m.coreWrites, NewWriteOp(WriteOpUpdate, "playbooks", &stored))

	return nil
}

// DeletePlaybook deletes a playbook by ID.
func (m *MemoryStore) DeletePlaybook(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.playbooks[id]
	if !exists {
		return ErrNotFound
	}

	// Remove from both maps
	delete(m.playbooks, id)
	key := playbookKey(p.Namespace, p.Slug, p.Version)
	delete(m.playbooksByKey, key)

	// Queue for persistence
	m.queueWrite(m.coreWrites, NewWriteOp(WriteOpDelete, "playbooks", id))

	return nil
}

// copyPlaybook creates a deep copy of a playbook.
func copyPlaybook(p *Playbook) *Playbook {
	result := *p
	if p.Steps != nil {
		result.Steps = make([]PlaybookStep, len(p.Steps))
		copy(result.Steps, p.Steps)
	}
	if p.SharedDirs != nil {
		result.SharedDirs = make([]string, len(p.SharedDirs))
		copy(result.SharedDirs, p.SharedDirs)
	}
	if p.SharedFiles != nil {
		result.SharedFiles = make([]string, len(p.SharedFiles))
		copy(result.SharedFiles, p.SharedFiles)
	}
	if p.WritableDirs != nil {
		result.WritableDirs = make([]string, len(p.WritableDirs))
		copy(result.WritableDirs, p.WritableDirs)
	}
	if p.ValidationRules != nil {
		rules := *p.ValidationRules
		result.ValidationRules = &rules
	}
	return &result
}

// --- PlaybookActivation operations ---

// CreatePlaybookActivation creates a new activation linking a project to a playbook.
func (m *MemoryStore) CreatePlaybookActivation(ctx context.Context, a *PlaybookActivation) error {
	if a == nil {
		return fmt.Errorf("playbook activation cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if project already has an activation
	for _, existing := range m.activationsByProject[a.ProjectID] {
		if existing.ProjectID == a.ProjectID {
			return fmt.Errorf("project %d already has an active playbook", a.ProjectID)
		}
	}

	// Assign ID if not set
	if a.ID == 0 {
		a.ID = nextID(&m.nextPlaybookActivationID)
	}

	// Set activated time if not set
	if a.ActivatedAt.IsZero() {
		a.ActivatedAt = time.Now()
	}

	// Store a copy
	stored := *a
	m.playbookActivations[a.ID] = &stored

	// Update secondary indexes
	m.activationsByProject[a.ProjectID] = append(m.activationsByProject[a.ProjectID], &stored)
	m.activationsByPlaybook[a.PlaybookID] = append(m.activationsByPlaybook[a.PlaybookID], &stored)

	// Queue for persistence
	m.queueWrite(m.coreWrites, NewWriteOp(WriteOpInsert, "playbook_activations", &stored))

	return nil
}

// GetPlaybookActivation returns the activation for a project.
func (m *MemoryStore) GetPlaybookActivation(ctx context.Context, projectID int64) (*PlaybookActivation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activations := m.activationsByProject[projectID]
	if len(activations) == 0 {
		return nil, ErrNotFound
	}

	// Return the most recent activation (should typically be only one)
	result := *activations[len(activations)-1]
	return &result, nil
}

// GetPlaybookActivationByID returns an activation by ID.
func (m *MemoryStore) GetPlaybookActivationByID(ctx context.Context, id int64) (*PlaybookActivation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	a, exists := m.playbookActivations[id]
	if !exists {
		return nil, ErrNotFound
	}

	// Return a copy
	result := *a
	return &result, nil
}

// ListActivationsByPlaybook returns all activations using a specific playbook.
func (m *MemoryStore) ListActivationsByPlaybook(ctx context.Context, playbookID int64) ([]*PlaybookActivation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activations := m.activationsByPlaybook[playbookID]
	result := make([]*PlaybookActivation, len(activations))
	for i, a := range activations {
		copy := *a
		result[i] = &copy
	}

	return result, nil
}

// DeletePlaybookActivation deletes an activation by ID.
func (m *MemoryStore) DeletePlaybookActivation(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, exists := m.playbookActivations[id]
	if !exists {
		return ErrNotFound
	}

	// Remove from primary map
	delete(m.playbookActivations, id)

	// Remove from secondary indexes
	m.activationsByProject[a.ProjectID] = removeActivationFromSlice(m.activationsByProject[a.ProjectID], id)
	m.activationsByPlaybook[a.PlaybookID] = removeActivationFromSlice(m.activationsByPlaybook[a.PlaybookID], id)

	// Queue for persistence
	m.queueWrite(m.coreWrites, NewWriteOp(WriteOpDelete, "playbook_activations", id))

	return nil
}

// removeActivationFromSlice removes an activation by ID from a slice.
func removeActivationFromSlice(slice []*PlaybookActivation, id int64) []*PlaybookActivation {
	for i, a := range slice {
		if a.ID == id {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// --- PlaybookVariableBinding operations ---

// CreateVariableBinding creates a new variable binding.
func (m *MemoryStore) CreateVariableBinding(ctx context.Context, b *PlaybookVariableBinding) error {
	if b == nil {
		return fmt.Errorf("variable binding cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Assign ID if not set
	if b.ID == 0 {
		b.ID = nextID(&m.nextVariableBindingID)
	}

	// Store a copy
	stored := *b
	m.variableBindings[b.ID] = &stored

	// Update secondary indexes
	m.bindingsByActivation[b.ActivationID] = append(m.bindingsByActivation[b.ActivationID], &stored)

	// Add to source ref index if applicable
	if b.SourceType != SourceTypeLiteral && b.SourceRef != "" {
		key := bindingSourceKey(b.SourceType, b.SourceRef)
		m.bindingsBySourceRef[key] = append(m.bindingsBySourceRef[key], &stored)
	}

	// Queue for persistence
	m.queueWrite(m.coreWrites, NewWriteOp(WriteOpInsert, "playbook_variable_bindings", &stored))

	return nil
}

// GetVariableBindings returns all bindings for an activation.
func (m *MemoryStore) GetVariableBindings(ctx context.Context, activationID int64) ([]*PlaybookVariableBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bindings := m.bindingsByActivation[activationID]
	result := make([]*PlaybookVariableBinding, len(bindings))
	for i, b := range bindings {
		copy := *b
		result[i] = &copy
	}

	return result, nil
}

// UpdateVariableBinding updates an existing binding.
func (m *MemoryStore) UpdateVariableBinding(ctx context.Context, b *PlaybookVariableBinding) error {
	if b == nil {
		return fmt.Errorf("variable binding cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.variableBindings[b.ID]
	if !exists {
		return ErrNotFound
	}

	// Remove from old source ref index if applicable
	if existing.SourceType != SourceTypeLiteral && existing.SourceRef != "" {
		oldKey := bindingSourceKey(existing.SourceType, existing.SourceRef)
		m.bindingsBySourceRef[oldKey] = removeBindingFromSlice(m.bindingsBySourceRef[oldKey], b.ID)
	}

	// Store updated copy
	stored := *b
	m.variableBindings[b.ID] = &stored

	// Update in activation index
	bindings := m.bindingsByActivation[b.ActivationID]
	for i, binding := range bindings {
		if binding.ID == b.ID {
			bindings[i] = &stored
			break
		}
	}

	// Add to new source ref index if applicable
	if b.SourceType != SourceTypeLiteral && b.SourceRef != "" {
		key := bindingSourceKey(b.SourceType, b.SourceRef)
		m.bindingsBySourceRef[key] = append(m.bindingsBySourceRef[key], &stored)
	}

	// Queue for persistence
	m.queueWrite(m.coreWrites, NewWriteOp(WriteOpUpdate, "playbook_variable_bindings", &stored))

	return nil
}

// DeleteVariableBinding deletes a binding by ID.
func (m *MemoryStore) DeleteVariableBinding(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, exists := m.variableBindings[id]
	if !exists {
		return ErrNotFound
	}

	// Remove from primary map
	delete(m.variableBindings, id)

	// Remove from activation index
	m.bindingsByActivation[b.ActivationID] = removeBindingFromSlice(m.bindingsByActivation[b.ActivationID], id)

	// Remove from source ref index if applicable
	if b.SourceType != SourceTypeLiteral && b.SourceRef != "" {
		key := bindingSourceKey(b.SourceType, b.SourceRef)
		m.bindingsBySourceRef[key] = removeBindingFromSlice(m.bindingsBySourceRef[key], id)
	}

	// Queue for persistence
	m.queueWrite(m.coreWrites, NewWriteOp(WriteOpDelete, "playbook_variable_bindings", id))

	return nil
}

// FindBindingsBySourceRef finds bindings that reference a specific source (env key or secret).
func (m *MemoryStore) FindBindingsBySourceRef(ctx context.Context, sourceType, sourceRef string) ([]*PlaybookVariableBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := bindingSourceKey(sourceType, sourceRef)
	bindings := m.bindingsBySourceRef[key]
	result := make([]*PlaybookVariableBinding, len(bindings))
	for i, b := range bindings {
		copy := *b
		result[i] = &copy
	}

	return result, nil
}

// removeBindingFromSlice removes a binding by ID from a slice.
func removeBindingFromSlice(slice []*PlaybookVariableBinding, id int64) []*PlaybookVariableBinding {
	for i, b := range slice {
		if b.ID == id {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// --- RawCommandApproval operations ---

// CreateRawApproval creates an approval record for a RAW component.
func (m *MemoryStore) CreateRawApproval(ctx context.Context, a *RawCommandApproval) error {
	if a == nil {
		return fmt.Errorf("raw approval cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if component already has an approval
	for _, existing := range m.rawApprovalsByComponent[a.ComponentID] {
		if existing.ComponentID == a.ComponentID {
			return fmt.Errorf("component %d already has an approval", a.ComponentID)
		}
	}

	// Assign ID if not set
	if a.ID == 0 {
		a.ID = nextID(&m.nextRawApprovalID)
	}

	// Set approved time if not set
	if a.ApprovedAt.IsZero() {
		a.ApprovedAt = time.Now()
	}

	// Store a copy
	stored := *a
	m.rawApprovals[a.ID] = &stored

	// Update secondary index
	m.rawApprovalsByComponent[a.ComponentID] = append(m.rawApprovalsByComponent[a.ComponentID], &stored)

	// Queue for persistence
	m.queueWrite(m.coreWrites, NewWriteOp(WriteOpInsert, "raw_command_approvals", &stored))

	return nil
}

// GetRawApproval returns the approval for a component.
func (m *MemoryStore) GetRawApproval(ctx context.Context, componentID int64) (*RawCommandApproval, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	approvals := m.rawApprovalsByComponent[componentID]
	if len(approvals) == 0 {
		return nil, ErrNotFound
	}

	// Return the most recent approval (should typically be only one)
	result := *approvals[len(approvals)-1]
	return &result, nil
}

// DeleteRawApproval deletes an approval by component ID.
func (m *MemoryStore) DeleteRawApproval(ctx context.Context, componentID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	approvals := m.rawApprovalsByComponent[componentID]
	if len(approvals) == 0 {
		return ErrNotFound
	}

	// Remove all approvals for this component
	for _, a := range approvals {
		delete(m.rawApprovals, a.ID)
		// Queue for persistence
		m.queueWrite(m.coreWrites, NewWriteOp(WriteOpDelete, "raw_command_approvals", a.ID))
	}

	// Clear the secondary index
	delete(m.rawApprovalsByComponent, componentID)

	return nil
}

// ListRawApprovals returns all RAW command approvals.
func (m *MemoryStore) ListRawApprovals(ctx context.Context) ([]*RawCommandApproval, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*RawCommandApproval, 0, len(m.rawApprovals))
	for _, a := range m.rawApprovals {
		copy := *a
		result = append(result, &copy)
	}

	// Sort by approved time for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].ApprovedAt.Before(result[j].ApprovedAt)
	})

	return result, nil
}

// --- Loader functions for startup ---

// loadRecipeComponentsFromDB loads recipe components from the database into memory.
func (m *MemoryStore) loadRecipeComponentsFromDB(_ context.Context, db *DB) error {
	rows, err := db.conn.Query(`
		SELECT id, namespace, slug, version, name, description, component_type, 
		       content, variables, is_seed, is_raw, is_deprecated, created_at
		FROM recipe_components
	`)
	if err != nil {
		return fmt.Errorf("query recipe_components: %w", err)
	}
	defer rows.Close()

	m.mu.Lock()
	defer m.mu.Unlock()

	var maxID int64
	for rows.Next() {
		c := &RecipeComponent{}
		var contentJSON, variablesJSON string
		err := rows.Scan(
			&c.ID, &c.Namespace, &c.Slug, &c.Version, &c.Name, &c.Description,
			&c.ComponentType, &contentJSON, &variablesJSON, &c.IsSeed, &c.IsRaw,
			&c.IsDeprecated, &c.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("scan recipe_component: %w", err)
		}

		// Parse JSON fields
		if err := c.ParseContentJSON(contentJSON); err != nil {
			return fmt.Errorf("parse content for component %d: %w", c.ID, err)
		}
		if err := c.ParseVariablesJSON(variablesJSON); err != nil {
			return fmt.Errorf("parse variables for component %d: %w", c.ID, err)
		}

		m.recipeComponents[c.ID] = c
		key := recipeComponentKey(c.Namespace, c.Slug, c.Version)
		m.recipeComponentsByKey[key] = c

		if c.ID > maxID {
			maxID = c.ID
		}
	}

	m.nextRecipeComponentID.Store(maxID)
	return rows.Err()
}

// loadPlaybooksFromDB loads playbooks from the database into memory.
func (m *MemoryStore) loadPlaybooksFromDB(_ context.Context, db *DB) error {
	rows, err := db.conn.Query(`
		SELECT id, namespace, slug, version, name, description, framework_type,
		       steps, shared_dirs, shared_files, writable_dirs, keep_releases,
		       validation_rules, is_seed, is_deprecated, parent_id, parent_version, created_at
		FROM playbooks
	`)
	if err != nil {
		return fmt.Errorf("query playbooks: %w", err)
	}
	defer rows.Close()

	m.mu.Lock()
	defer m.mu.Unlock()

	var maxID int64
	for rows.Next() {
		p := &Playbook{}
		var stepsJSON, sharedDirsJSON, sharedFilesJSON, writableDirsJSON, validationRulesJSON string
		var parentID *int64
		var parentVersion *string
		err := rows.Scan(
			&p.ID, &p.Namespace, &p.Slug, &p.Version, &p.Name, &p.Description, &p.FrameworkType,
			&stepsJSON, &sharedDirsJSON, &sharedFilesJSON, &writableDirsJSON, &p.KeepReleases,
			&validationRulesJSON, &p.IsSeed, &p.IsDeprecated, &parentID, &parentVersion, &p.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("scan playbook: %w", err)
		}

		// Parse JSON fields
		if err := p.ParseStepsJSON(stepsJSON); err != nil {
			return fmt.Errorf("parse steps for playbook %d: %w", p.ID, err)
		}
		if err := p.ParseSharedDirsJSON(sharedDirsJSON); err != nil {
			return fmt.Errorf("parse shared_dirs for playbook %d: %w", p.ID, err)
		}
		if err := p.ParseSharedFilesJSON(sharedFilesJSON); err != nil {
			return fmt.Errorf("parse shared_files for playbook %d: %w", p.ID, err)
		}
		if err := p.ParseWritableDirsJSON(writableDirsJSON); err != nil {
			return fmt.Errorf("parse writable_dirs for playbook %d: %w", p.ID, err)
		}
		if err := p.ParseValidationRulesJSON(validationRulesJSON); err != nil {
			return fmt.Errorf("parse validation_rules for playbook %d: %w", p.ID, err)
		}

		p.ParentID = parentID
		if parentVersion != nil {
			p.ParentVersion = *parentVersion
		}

		m.playbooks[p.ID] = p
		key := playbookKey(p.Namespace, p.Slug, p.Version)
		m.playbooksByKey[key] = p

		if p.ID > maxID {
			maxID = p.ID
		}
	}

	m.nextPlaybookID.Store(maxID)
	return rows.Err()
}

// loadPlaybookActivationsFromDB loads playbook activations from the database into memory.
func (m *MemoryStore) loadPlaybookActivationsFromDB(_ context.Context, db *DB) error {
	rows, err := db.conn.Query(`
		SELECT id, project_id, playbook_id, activated_at, activated_by
		FROM playbook_activations
	`)
	if err != nil {
		return fmt.Errorf("query playbook_activations: %w", err)
	}
	defer rows.Close()

	m.mu.Lock()
	defer m.mu.Unlock()

	var maxID int64
	for rows.Next() {
		a := &PlaybookActivation{}
		err := rows.Scan(&a.ID, &a.ProjectID, &a.PlaybookID, &a.ActivatedAt, &a.ActivatedBy)
		if err != nil {
			return fmt.Errorf("scan playbook_activation: %w", err)
		}

		m.playbookActivations[a.ID] = a
		m.activationsByProject[a.ProjectID] = append(m.activationsByProject[a.ProjectID], a)
		m.activationsByPlaybook[a.PlaybookID] = append(m.activationsByPlaybook[a.PlaybookID], a)

		if a.ID > maxID {
			maxID = a.ID
		}
	}

	m.nextPlaybookActivationID.Store(maxID)
	return rows.Err()
}

// loadVariableBindingsFromDB loads variable bindings from the database into memory.
func (m *MemoryStore) loadVariableBindingsFromDB(_ context.Context, db *DB) error {
	rows, err := db.conn.Query(`
		SELECT id, activation_id, variable_name, source_type, source_ref, literal_value
		FROM playbook_variable_bindings
	`)
	if err != nil {
		return fmt.Errorf("query playbook_variable_bindings: %w", err)
	}
	defer rows.Close()

	m.mu.Lock()
	defer m.mu.Unlock()

	var maxID int64
	for rows.Next() {
		b := &PlaybookVariableBinding{}
		var sourceRef, literalValue *string
		err := rows.Scan(&b.ID, &b.ActivationID, &b.VariableName, &b.SourceType, &sourceRef, &literalValue)
		if err != nil {
			return fmt.Errorf("scan playbook_variable_binding: %w", err)
		}

		if sourceRef != nil {
			b.SourceRef = *sourceRef
		}
		if literalValue != nil {
			b.LiteralValue = *literalValue
		}

		m.variableBindings[b.ID] = b
		m.bindingsByActivation[b.ActivationID] = append(m.bindingsByActivation[b.ActivationID], b)

		// Add to source ref index if applicable
		if b.SourceType != SourceTypeLiteral && b.SourceRef != "" {
			key := bindingSourceKey(b.SourceType, b.SourceRef)
			m.bindingsBySourceRef[key] = append(m.bindingsBySourceRef[key], b)
		}

		if b.ID > maxID {
			maxID = b.ID
		}
	}

	m.nextVariableBindingID.Store(maxID)
	return rows.Err()
}

// loadRawApprovalsFromDB loads raw command approvals from the database into memory.
func (m *MemoryStore) loadRawApprovalsFromDB(_ context.Context, db *DB) error {
	rows, err := db.conn.Query(`
		SELECT id, component_id, approved_by, approved_at, approval_note
		FROM raw_command_approvals
	`)
	if err != nil {
		return fmt.Errorf("query raw_command_approvals: %w", err)
	}
	defer rows.Close()

	m.mu.Lock()
	defer m.mu.Unlock()

	var maxID int64
	for rows.Next() {
		a := &RawCommandApproval{}
		var note *string
		err := rows.Scan(&a.ID, &a.ComponentID, &a.ApprovedBy, &a.ApprovedAt, &note)
		if err != nil {
			return fmt.Errorf("scan raw_command_approval: %w", err)
		}

		if note != nil {
			a.ApprovalNote = *note
		}

		m.rawApprovals[a.ID] = a
		m.rawApprovalsByComponent[a.ComponentID] = append(m.rawApprovalsByComponent[a.ComponentID], a)

		if a.ID > maxID {
			maxID = a.ID
		}
	}

	m.nextRawApprovalID.Store(maxID)
	return rows.Err()
}

// Suppress unused import warning for strings package
var _ = strings.TrimSpace
