// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// --- Recipe Component Methods ---

// CreateRecipeComponent creates a new recipe component.
func (db *DB) CreateRecipeComponent(ctx context.Context, component *RecipeComponent) error {
	contentJSON, err := component.ContentJSON()
	if err != nil {
		return fmt.Errorf("marshal content: %w", err)
	}
	variablesJSON, err := component.VariablesJSON()
	if err != nil {
		return fmt.Errorf("marshal variables: %w", err)
	}

	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO recipe_components (
			namespace, slug, version, name, description, component_type,
			content, variables, is_seed, is_raw, is_deprecated
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, component.Namespace, component.Slug, component.Version, component.Name,
		component.Description, component.ComponentType, contentJSON, variablesJSON,
		component.IsSeed, component.IsRaw, component.IsDeprecated)
	if err != nil {
		return fmt.Errorf("insert recipe component: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	component.ID = id
	return nil
}

// GetRecipeComponent returns a component by namespace, slug, and version.
func (db *DB) GetRecipeComponent(ctx context.Context, namespace, slug, version string) (*RecipeComponent, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, namespace, slug, version, name, description, component_type,
			   content, variables, is_seed, is_raw, is_deprecated, created_at
		FROM recipe_components
		WHERE namespace = ? AND slug = ? AND version = ?
	`, namespace, slug, version)

	return scanRecipeComponent(row)
}

// GetRecipeComponentByID returns a component by ID.
func (db *DB) GetRecipeComponentByID(ctx context.Context, id int64) (*RecipeComponent, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, namespace, slug, version, name, description, component_type,
			   content, variables, is_seed, is_raw, is_deprecated, created_at
		FROM recipe_components
		WHERE id = ?
	`, id)

	return scanRecipeComponent(row)
}

// ListRecipeComponents returns all components in a namespace.
func (db *DB) ListRecipeComponents(ctx context.Context, namespace string, includeDeprecated bool) ([]*RecipeComponent, error) {
	query := `
		SELECT id, namespace, slug, version, name, description, component_type,
			   content, variables, is_seed, is_raw, is_deprecated, created_at
		FROM recipe_components
		WHERE namespace = ?
	`
	args := []any{namespace}

	if !includeDeprecated {
		query += " AND is_deprecated = 0"
	}
	query += " ORDER BY slug, version"

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query recipe components: %w", err)
	}
	defer rows.Close()

	return scanRecipeComponents(rows)
}

// ListRecipeComponentVersions returns all versions of a component.
func (db *DB) ListRecipeComponentVersions(ctx context.Context, namespace, slug string) ([]*RecipeComponent, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, namespace, slug, version, name, description, component_type,
			   content, variables, is_seed, is_raw, is_deprecated, created_at
		FROM recipe_components
		WHERE namespace = ? AND slug = ?
		ORDER BY created_at DESC
	`, namespace, slug)
	if err != nil {
		return nil, fmt.Errorf("query component versions: %w", err)
	}
	defer rows.Close()

	return scanRecipeComponents(rows)
}

// UpdateRecipeComponent updates an existing component.
func (db *DB) UpdateRecipeComponent(ctx context.Context, component *RecipeComponent) error {
	contentJSON, err := component.ContentJSON()
	if err != nil {
		return fmt.Errorf("marshal content: %w", err)
	}
	variablesJSON, err := component.VariablesJSON()
	if err != nil {
		return fmt.Errorf("marshal variables: %w", err)
	}

	_, err = db.conn.ExecContext(ctx, `
		UPDATE recipe_components SET
			name = ?, description = ?, content = ?, variables = ?,
			is_raw = ?, is_deprecated = ?
		WHERE id = ?
	`, component.Name, component.Description, contentJSON, variablesJSON,
		component.IsRaw, component.IsDeprecated, component.ID)
	if err != nil {
		return fmt.Errorf("update recipe component: %w", err)
	}
	return nil
}

// DeleteRecipeComponent deletes a component by ID.
func (db *DB) DeleteRecipeComponent(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM recipe_components WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete recipe component: %w", err)
	}
	return nil
}

func scanRecipeComponent(row *sql.Row) (*RecipeComponent, error) {
	var c RecipeComponent
	var contentJSON, variablesJSON string
	var description sql.NullString

	err := row.Scan(
		&c.ID, &c.Namespace, &c.Slug, &c.Version, &c.Name, &description,
		&c.ComponentType, &contentJSON, &variablesJSON,
		&c.IsSeed, &c.IsRaw, &c.IsDeprecated, &c.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan recipe component: %w", err)
	}

	if description.Valid {
		c.Description = description.String
	}

	if err := c.ParseContentJSON(contentJSON); err != nil {
		return nil, fmt.Errorf("parse content: %w", err)
	}
	if err := c.ParseVariablesJSON(variablesJSON); err != nil {
		return nil, fmt.Errorf("parse variables: %w", err)
	}

	return &c, nil
}

func scanRecipeComponents(rows *sql.Rows) ([]*RecipeComponent, error) {
	var components []*RecipeComponent
	for rows.Next() {
		var c RecipeComponent
		var contentJSON, variablesJSON string
		var description sql.NullString

		err := rows.Scan(
			&c.ID, &c.Namespace, &c.Slug, &c.Version, &c.Name, &description,
			&c.ComponentType, &contentJSON, &variablesJSON,
			&c.IsSeed, &c.IsRaw, &c.IsDeprecated, &c.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan recipe component: %w", err)
		}

		if description.Valid {
			c.Description = description.String
		}

		if err := c.ParseContentJSON(contentJSON); err != nil {
			return nil, fmt.Errorf("parse content: %w", err)
		}
		if err := c.ParseVariablesJSON(variablesJSON); err != nil {
			return nil, fmt.Errorf("parse variables: %w", err)
		}

		components = append(components, &c)
	}
	return components, rows.Err()
}

// --- Playbook Methods ---

// CreatePlaybook creates a new playbook.
func (db *DB) CreatePlaybook(ctx context.Context, playbook *Playbook) error {
	stepsJSON, err := playbook.StepsJSON()
	if err != nil {
		return fmt.Errorf("marshal steps: %w", err)
	}
	sharedDirsJSON, err := playbook.SharedDirsJSON()
	if err != nil {
		return fmt.Errorf("marshal shared_dirs: %w", err)
	}
	sharedFilesJSON, err := playbook.SharedFilesJSON()
	if err != nil {
		return fmt.Errorf("marshal shared_files: %w", err)
	}
	writableDirsJSON, err := playbook.WritableDirsJSON()
	if err != nil {
		return fmt.Errorf("marshal writable_dirs: %w", err)
	}
	validationRulesJSON, err := playbook.ValidationRulesJSON()
	if err != nil {
		return fmt.Errorf("marshal validation_rules: %w", err)
	}

	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO playbooks (
			namespace, slug, version, name, description, framework_type,
			steps, shared_dirs, shared_files, writable_dirs, keep_releases,
			validation_rules, is_seed, is_deprecated, parent_id, parent_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, playbook.Namespace, playbook.Slug, playbook.Version, playbook.Name,
		playbook.Description, playbook.FrameworkType, stepsJSON, sharedDirsJSON,
		sharedFilesJSON, writableDirsJSON, playbook.KeepReleases, validationRulesJSON,
		playbook.IsSeed, playbook.IsDeprecated, playbook.ParentID, playbook.ParentVersion)
	if err != nil {
		return fmt.Errorf("insert playbook: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	playbook.ID = id
	return nil
}

// GetPlaybook returns a playbook by namespace, slug, and version.
func (db *DB) GetPlaybook(ctx context.Context, namespace, slug, version string) (*Playbook, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, namespace, slug, version, name, description, framework_type,
			   steps, shared_dirs, shared_files, writable_dirs, keep_releases,
			   validation_rules, is_seed, is_deprecated, parent_id, parent_version, created_at
		FROM playbooks
		WHERE namespace = ? AND slug = ? AND version = ?
	`, namespace, slug, version)

	return scanPlaybook(row)
}

// GetPlaybookByID returns a playbook by ID.
func (db *DB) GetPlaybookByID(ctx context.Context, id int64) (*Playbook, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, namespace, slug, version, name, description, framework_type,
			   steps, shared_dirs, shared_files, writable_dirs, keep_releases,
			   validation_rules, is_seed, is_deprecated, parent_id, parent_version, created_at
		FROM playbooks
		WHERE id = ?
	`, id)

	return scanPlaybook(row)
}

// ListPlaybooks returns playbooks filtered by namespace and/or framework type.
func (db *DB) ListPlaybooks(ctx context.Context, namespace, frameworkType string, includeDeprecated bool) ([]*Playbook, error) {
	query := `
		SELECT id, namespace, slug, version, name, description, framework_type,
			   steps, shared_dirs, shared_files, writable_dirs, keep_releases,
			   validation_rules, is_seed, is_deprecated, parent_id, parent_version, created_at
		FROM playbooks
		WHERE 1=1
	`
	var args []any

	if namespace != "" {
		query += " AND namespace = ?"
		args = append(args, namespace)
	}
	if frameworkType != "" {
		query += " AND framework_type = ?"
		args = append(args, frameworkType)
	}
	if !includeDeprecated {
		query += " AND is_deprecated = 0"
	}
	query += " ORDER BY slug, version"

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query playbooks: %w", err)
	}
	defer rows.Close()

	return scanPlaybooks(rows)
}

// ListPlaybookVersions returns all versions of a playbook.
func (db *DB) ListPlaybookVersions(ctx context.Context, namespace, slug string) ([]*Playbook, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, namespace, slug, version, name, description, framework_type,
			   steps, shared_dirs, shared_files, writable_dirs, keep_releases,
			   validation_rules, is_seed, is_deprecated, parent_id, parent_version, created_at
		FROM playbooks
		WHERE namespace = ? AND slug = ?
		ORDER BY created_at DESC
	`, namespace, slug)
	if err != nil {
		return nil, fmt.Errorf("query playbook versions: %w", err)
	}
	defer rows.Close()

	return scanPlaybooks(rows)
}

// UpdatePlaybook updates an existing playbook.
func (db *DB) UpdatePlaybook(ctx context.Context, playbook *Playbook) error {
	stepsJSON, err := playbook.StepsJSON()
	if err != nil {
		return fmt.Errorf("marshal steps: %w", err)
	}
	sharedDirsJSON, err := playbook.SharedDirsJSON()
	if err != nil {
		return fmt.Errorf("marshal shared_dirs: %w", err)
	}
	sharedFilesJSON, err := playbook.SharedFilesJSON()
	if err != nil {
		return fmt.Errorf("marshal shared_files: %w", err)
	}
	writableDirsJSON, err := playbook.WritableDirsJSON()
	if err != nil {
		return fmt.Errorf("marshal writable_dirs: %w", err)
	}
	validationRulesJSON, err := playbook.ValidationRulesJSON()
	if err != nil {
		return fmt.Errorf("marshal validation_rules: %w", err)
	}

	_, err = db.conn.ExecContext(ctx, `
		UPDATE playbooks SET
			name = ?, description = ?, steps = ?, shared_dirs = ?,
			shared_files = ?, writable_dirs = ?, keep_releases = ?,
			validation_rules = ?, is_deprecated = ?
		WHERE id = ?
	`, playbook.Name, playbook.Description, stepsJSON, sharedDirsJSON,
		sharedFilesJSON, writableDirsJSON, playbook.KeepReleases,
		validationRulesJSON, playbook.IsDeprecated, playbook.ID)
	if err != nil {
		return fmt.Errorf("update playbook: %w", err)
	}
	return nil
}

// DeletePlaybook deletes a playbook by ID.
func (db *DB) DeletePlaybook(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM playbooks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete playbook: %w", err)
	}
	return nil
}

func scanPlaybook(row *sql.Row) (*Playbook, error) {
	var p Playbook
	var description, frameworkType, parentVersion sql.NullString
	var parentID sql.NullInt64
	var stepsJSON, sharedDirsJSON, sharedFilesJSON, writableDirsJSON, validationRulesJSON string

	err := row.Scan(
		&p.ID, &p.Namespace, &p.Slug, &p.Version, &p.Name, &description,
		&frameworkType, &stepsJSON, &sharedDirsJSON, &sharedFilesJSON,
		&writableDirsJSON, &p.KeepReleases, &validationRulesJSON,
		&p.IsSeed, &p.IsDeprecated, &parentID, &parentVersion, &p.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan playbook: %w", err)
	}

	if description.Valid {
		p.Description = description.String
	}
	if frameworkType.Valid {
		p.FrameworkType = frameworkType.String
	}
	if parentID.Valid {
		p.ParentID = &parentID.Int64
	}
	if parentVersion.Valid {
		p.ParentVersion = parentVersion.String
	}

	if err := p.ParseStepsJSON(stepsJSON); err != nil {
		return nil, fmt.Errorf("parse steps: %w", err)
	}
	if err := p.ParseSharedDirsJSON(sharedDirsJSON); err != nil {
		return nil, fmt.Errorf("parse shared_dirs: %w", err)
	}
	if err := p.ParseSharedFilesJSON(sharedFilesJSON); err != nil {
		return nil, fmt.Errorf("parse shared_files: %w", err)
	}
	if err := p.ParseWritableDirsJSON(writableDirsJSON); err != nil {
		return nil, fmt.Errorf("parse writable_dirs: %w", err)
	}
	if err := p.ParseValidationRulesJSON(validationRulesJSON); err != nil {
		return nil, fmt.Errorf("parse validation_rules: %w", err)
	}

	return &p, nil
}

func scanPlaybooks(rows *sql.Rows) ([]*Playbook, error) {
	var playbooks []*Playbook
	for rows.Next() {
		var p Playbook
		var description, frameworkType, parentVersion sql.NullString
		var parentID sql.NullInt64
		var stepsJSON, sharedDirsJSON, sharedFilesJSON, writableDirsJSON, validationRulesJSON string

		err := rows.Scan(
			&p.ID, &p.Namespace, &p.Slug, &p.Version, &p.Name, &description,
			&frameworkType, &stepsJSON, &sharedDirsJSON, &sharedFilesJSON,
			&writableDirsJSON, &p.KeepReleases, &validationRulesJSON,
			&p.IsSeed, &p.IsDeprecated, &parentID, &parentVersion, &p.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan playbook: %w", err)
		}

		if description.Valid {
			p.Description = description.String
		}
		if frameworkType.Valid {
			p.FrameworkType = frameworkType.String
		}
		if parentID.Valid {
			p.ParentID = &parentID.Int64
		}
		if parentVersion.Valid {
			p.ParentVersion = parentVersion.String
		}

		if err := p.ParseStepsJSON(stepsJSON); err != nil {
			return nil, fmt.Errorf("parse steps: %w", err)
		}
		if err := p.ParseSharedDirsJSON(sharedDirsJSON); err != nil {
			return nil, fmt.Errorf("parse shared_dirs: %w", err)
		}
		if err := p.ParseSharedFilesJSON(sharedFilesJSON); err != nil {
			return nil, fmt.Errorf("parse shared_files: %w", err)
		}
		if err := p.ParseWritableDirsJSON(writableDirsJSON); err != nil {
			return nil, fmt.Errorf("parse writable_dirs: %w", err)
		}
		if err := p.ParseValidationRulesJSON(validationRulesJSON); err != nil {
			return nil, fmt.Errorf("parse validation_rules: %w", err)
		}

		playbooks = append(playbooks, &p)
	}
	return playbooks, rows.Err()
}

// --- Playbook Activation Methods ---

// CreatePlaybookActivation creates a new activation linking a project to a playbook.
func (db *DB) CreatePlaybookActivation(ctx context.Context, activation *PlaybookActivation) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO playbook_activations (project_id, playbook_id, activated_by)
		VALUES (?, ?, ?)
	`, activation.ProjectID, activation.PlaybookID, activation.ActivatedBy)
	if err != nil {
		return fmt.Errorf("insert playbook activation: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	activation.ID = id
	return nil
}

// GetPlaybookActivation returns the activation for a project.
func (db *DB) GetPlaybookActivation(ctx context.Context, projectID int64) (*PlaybookActivation, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, project_id, playbook_id, activated_at, activated_by
		FROM playbook_activations
		WHERE project_id = ?
	`, projectID)

	return scanPlaybookActivation(row)
}

// GetPlaybookActivationByID returns an activation by ID.
func (db *DB) GetPlaybookActivationByID(ctx context.Context, id int64) (*PlaybookActivation, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, project_id, playbook_id, activated_at, activated_by
		FROM playbook_activations
		WHERE id = ?
	`, id)

	return scanPlaybookActivation(row)
}

// ListActivationsByPlaybook returns all activations using a specific playbook.
func (db *DB) ListActivationsByPlaybook(ctx context.Context, playbookID int64) ([]*PlaybookActivation, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project_id, playbook_id, activated_at, activated_by
		FROM playbook_activations
		WHERE playbook_id = ?
	`, playbookID)
	if err != nil {
		return nil, fmt.Errorf("query activations: %w", err)
	}
	defer rows.Close()

	var activations []*PlaybookActivation
	for rows.Next() {
		var a PlaybookActivation
		var activatedBy sql.NullInt64

		err := rows.Scan(&a.ID, &a.ProjectID, &a.PlaybookID, &a.ActivatedAt, &activatedBy)
		if err != nil {
			return nil, fmt.Errorf("scan activation: %w", err)
		}
		if activatedBy.Valid {
			a.ActivatedBy = &activatedBy.Int64
		}
		activations = append(activations, &a)
	}
	return activations, rows.Err()
}

// DeletePlaybookActivation deletes an activation by ID.
func (db *DB) DeletePlaybookActivation(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM playbook_activations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete playbook activation: %w", err)
	}
	return nil
}

func scanPlaybookActivation(row *sql.Row) (*PlaybookActivation, error) {
	var a PlaybookActivation
	var activatedBy sql.NullInt64

	err := row.Scan(&a.ID, &a.ProjectID, &a.PlaybookID, &a.ActivatedAt, &activatedBy)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan playbook activation: %w", err)
	}
	if activatedBy.Valid {
		a.ActivatedBy = &activatedBy.Int64
	}
	return &a, nil
}

// --- Playbook Variable Binding Methods ---

// CreateVariableBinding creates a new variable binding.
func (db *DB) CreateVariableBinding(ctx context.Context, binding *PlaybookVariableBinding) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO playbook_variable_bindings (
			activation_id, variable_name, source_type, source_ref, literal_value
		) VALUES (?, ?, ?, ?, ?)
	`, binding.ActivationID, binding.VariableName, binding.SourceType,
		binding.SourceRef, binding.LiteralValue)
	if err != nil {
		return fmt.Errorf("insert variable binding: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	binding.ID = id
	return nil
}

// GetVariableBindings returns all bindings for an activation.
func (db *DB) GetVariableBindings(ctx context.Context, activationID int64) ([]*PlaybookVariableBinding, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, activation_id, variable_name, source_type, source_ref, literal_value
		FROM playbook_variable_bindings
		WHERE activation_id = ?
	`, activationID)
	if err != nil {
		return nil, fmt.Errorf("query variable bindings: %w", err)
	}
	defer rows.Close()

	return scanVariableBindings(rows)
}

// UpdateVariableBinding updates an existing binding.
func (db *DB) UpdateVariableBinding(ctx context.Context, binding *PlaybookVariableBinding) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE playbook_variable_bindings SET
			source_type = ?, source_ref = ?, literal_value = ?
		WHERE id = ?
	`, binding.SourceType, binding.SourceRef, binding.LiteralValue, binding.ID)
	if err != nil {
		return fmt.Errorf("update variable binding: %w", err)
	}
	return nil
}

// DeleteVariableBinding deletes a binding by ID.
func (db *DB) DeleteVariableBinding(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM playbook_variable_bindings WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete variable binding: %w", err)
	}
	return nil
}

// FindBindingsBySourceRef finds bindings that reference a specific source.
func (db *DB) FindBindingsBySourceRef(ctx context.Context, sourceType, sourceRef string) ([]*PlaybookVariableBinding, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, activation_id, variable_name, source_type, source_ref, literal_value
		FROM playbook_variable_bindings
		WHERE source_type = ? AND source_ref = ?
	`, sourceType, sourceRef)
	if err != nil {
		return nil, fmt.Errorf("query bindings by source: %w", err)
	}
	defer rows.Close()

	return scanVariableBindings(rows)
}

func scanVariableBindings(rows *sql.Rows) ([]*PlaybookVariableBinding, error) {
	var bindings []*PlaybookVariableBinding
	for rows.Next() {
		var b PlaybookVariableBinding
		var sourceRef, literalValue sql.NullString

		err := rows.Scan(&b.ID, &b.ActivationID, &b.VariableName, &b.SourceType, &sourceRef, &literalValue)
		if err != nil {
			return nil, fmt.Errorf("scan variable binding: %w", err)
		}
		if sourceRef.Valid {
			b.SourceRef = sourceRef.String
		}
		if literalValue.Valid {
			b.LiteralValue = literalValue.String
		}
		bindings = append(bindings, &b)
	}
	return bindings, rows.Err()
}

// --- RAW Command Approval Methods ---

// CreateRawApproval creates an approval record for a RAW component.
func (db *DB) CreateRawApproval(ctx context.Context, approval *RawCommandApproval) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO raw_command_approvals (component_id, approved_by, approval_note)
		VALUES (?, ?, ?)
	`, approval.ComponentID, approval.ApprovedBy, approval.ApprovalNote)
	if err != nil {
		return fmt.Errorf("insert raw approval: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	approval.ID = id
	return nil
}

// GetRawApproval returns the approval for a component.
func (db *DB) GetRawApproval(ctx context.Context, componentID int64) (*RawCommandApproval, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, component_id, approved_by, approved_at, approval_note
		FROM raw_command_approvals
		WHERE component_id = ?
	`, componentID)

	var a RawCommandApproval
	var approvalNote sql.NullString

	err := row.Scan(&a.ID, &a.ComponentID, &a.ApprovedBy, &a.ApprovedAt, &approvalNote)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan raw approval: %w", err)
	}
	if approvalNote.Valid {
		a.ApprovalNote = approvalNote.String
	}
	return &a, nil
}

// DeleteRawApproval deletes an approval by component ID.
func (db *DB) DeleteRawApproval(ctx context.Context, componentID int64) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM raw_command_approvals WHERE component_id = ?`, componentID)
	if err != nil {
		return fmt.Errorf("delete raw approval: %w", err)
	}
	return nil
}

// ListRawApprovals returns all RAW command approvals.
func (db *DB) ListRawApprovals(ctx context.Context) ([]*RawCommandApproval, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, component_id, approved_by, approved_at, approval_note
		FROM raw_command_approvals
		ORDER BY approved_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query raw approvals: %w", err)
	}
	defer rows.Close()

	var approvals []*RawCommandApproval
	for rows.Next() {
		var a RawCommandApproval
		var approvalNote sql.NullString

		err := rows.Scan(&a.ID, &a.ComponentID, &a.ApprovedBy, &a.ApprovedAt, &approvalNote)
		if err != nil {
			return nil, fmt.Errorf("scan raw approval: %w", err)
		}
		if approvalNote.Valid {
			a.ApprovalNote = approvalNote.String
		}
		approvals = append(approvals, &a)
	}
	return approvals, rows.Err()
}

// Unused import guard for json
var _ = json.Marshal
