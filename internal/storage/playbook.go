// Package storage provides database operations for vcdeploy.
package storage

import (
	"encoding/json"
	"time"
)

// Playbook represents a deployment composition of recipe components.
type Playbook struct {
	ID              int64            `json:"id"`
	UID             string           `json:"uid"`
	Namespace       string           `json:"namespace"` // "seed" or "user"
	Slug            string           `json:"slug"`      // unique identifier within namespace
	Version         string           `json:"version"`   // semver with 'v' prefix
	Name            string           `json:"name"`      // human-readable name
	Description     string           `json:"description,omitempty"`
	FrameworkType   string           `json:"framework_type,omitempty"` // laravel, nextjs, rails, etc.
	Steps           []PlaybookStep   `json:"steps"`
	SharedDirs      []string         `json:"shared_dirs,omitempty"`
	SharedFiles     []string         `json:"shared_files,omitempty"`
	WritableDirs    []string         `json:"writable_dirs,omitempty"`
	KeepReleases    int              `json:"keep_releases"`
	ValidationRules *ValidationRules `json:"validation_rules,omitempty"`
	IsSeed          bool             `json:"is_seed"`
	IsDeprecated    bool             `json:"is_deprecated"`
	ParentID        *int64           `json:"parent_id,omitempty"` // for copy-on-write customization
	ParentVersion   string           `json:"parent_version,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
}

// PlaybookStep references a component with variable bindings.
type PlaybookStep struct {
	Order            int               `json:"order"`
	ComponentRef     string            `json:"component_ref"`               // namespace:slug:version
	VariableBindings map[string]string `json:"variable_bindings,omitempty"` // variable -> value or {{var}} reference
	Condition        string            `json:"condition,omitempty"`         // expression to evaluate
	Phase            string            `json:"phase"`                       // pre_deploy, deploy, post_deploy, rollback
}

// ValidationRules for playbook execution.
type ValidationRules struct {
	RequiredFiles   []string `json:"required_files,omitempty"`
	RequiredEnvKeys []string `json:"required_env_keys,omitempty"`
	Patterns        []string `json:"patterns,omitempty"`
}

// StepsJSON returns steps as JSON for database storage.
func (p *Playbook) StepsJSON() (string, error) {
	data, err := json.Marshal(p.Steps)
	return string(data), err
}

// ParseStepsJSON parses steps from JSON.
func (p *Playbook) ParseStepsJSON(data string) error {
	if data == "" || data == "null" {
		p.Steps = nil
		return nil
	}
	return json.Unmarshal([]byte(data), &p.Steps)
}

// SharedDirsJSON returns shared_dirs as JSON for database storage.
func (p *Playbook) SharedDirsJSON() (string, error) {
	if p.SharedDirs == nil {
		return "[]", nil
	}
	data, err := json.Marshal(p.SharedDirs)
	return string(data), err
}

// ParseSharedDirsJSON parses shared_dirs from JSON.
func (p *Playbook) ParseSharedDirsJSON(data string) error {
	if data == "" || data == "null" {
		p.SharedDirs = nil
		return nil
	}
	return json.Unmarshal([]byte(data), &p.SharedDirs)
}

// SharedFilesJSON returns shared_files as JSON for database storage.
func (p *Playbook) SharedFilesJSON() (string, error) {
	if p.SharedFiles == nil {
		return "[]", nil
	}
	data, err := json.Marshal(p.SharedFiles)
	return string(data), err
}

// ParseSharedFilesJSON parses shared_files from JSON.
func (p *Playbook) ParseSharedFilesJSON(data string) error {
	if data == "" || data == "null" {
		p.SharedFiles = nil
		return nil
	}
	return json.Unmarshal([]byte(data), &p.SharedFiles)
}

// WritableDirsJSON returns writable_dirs as JSON for database storage.
func (p *Playbook) WritableDirsJSON() (string, error) {
	if p.WritableDirs == nil {
		return "[]", nil
	}
	data, err := json.Marshal(p.WritableDirs)
	return string(data), err
}

// ParseWritableDirsJSON parses writable_dirs from JSON.
func (p *Playbook) ParseWritableDirsJSON(data string) error {
	if data == "" || data == "null" {
		p.WritableDirs = nil
		return nil
	}
	return json.Unmarshal([]byte(data), &p.WritableDirs)
}

// ValidationRulesJSON returns validation_rules as JSON for database storage.
func (p *Playbook) ValidationRulesJSON() (string, error) {
	if p.ValidationRules == nil {
		return "null", nil
	}
	data, err := json.Marshal(p.ValidationRules)
	return string(data), err
}

// ParseValidationRulesJSON parses validation_rules from JSON.
func (p *Playbook) ParseValidationRulesJSON(data string) error {
	if data == "" || data == "null" {
		p.ValidationRules = nil
		return nil
	}
	return json.Unmarshal([]byte(data), &p.ValidationRules)
}

// PlaybookActivation links a project to a specific playbook version.
type PlaybookActivation struct {
	ID          int64                     `json:"id"`
	UID         string                    `json:"uid"`
	ProjectID   int64                     `json:"project_id"`
	PlaybookID  int64                     `json:"playbook_id"`
	ActivatedAt time.Time                 `json:"activated_at"`
	ActivatedBy *int64                    `json:"activated_by,omitempty"`
	Bindings    []PlaybookVariableBinding `json:"bindings,omitempty"` // loaded separately
}

// PlaybookVariableBinding maps a variable to its value source.
type PlaybookVariableBinding struct {
	ID           int64  `json:"id"`
	UID          string `json:"uid"`
	ActivationID int64  `json:"activation_id"`
	VariableName string `json:"variable_name"`
	SourceType   string `json:"source_type"`             // literal, env, secret
	SourceRef    string `json:"source_ref,omitempty"`    // env key name or secret key
	LiteralValue string `json:"literal_value,omitempty"` // for literal source type
}

// RawCommandApproval records admin approval for RAW components.
type RawCommandApproval struct {
	ID           int64     `json:"id"`
	UID          string    `json:"uid"`
	ComponentID  int64     `json:"component_id"`
	ApprovedBy   int64     `json:"approved_by"`
	ApprovedAt   time.Time `json:"approved_at"`
	ApprovalNote string    `json:"approval_note,omitempty"`
}

// Phase constants for playbook steps.
const (
	PhasePrepare    = "prepare"
	PhasePreDeploy  = "pre_deploy"
	PhaseDeploy     = "deploy"
	PhasePostDeploy = "post_deploy"
	PhaseRollback   = "rollback"
	PhaseFinalize   = "finalize"
)

// ValidPhases returns all valid playbook phases.
func ValidPhases() []string {
	return []string{
		PhasePrepare,
		PhasePreDeploy,
		PhaseDeploy,
		PhasePostDeploy,
		PhaseRollback,
		PhaseFinalize,
	}
}

// SourceType constants for variable bindings.
const (
	SourceTypeLiteral = "literal"
	SourceTypeEnv     = "env"
	SourceTypeSecret  = "secret"
)

// ValidSourceTypes returns all valid binding source types.
func ValidSourceTypes() []string {
	return []string{SourceTypeLiteral, SourceTypeEnv, SourceTypeSecret}
}
