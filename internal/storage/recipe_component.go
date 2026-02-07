// Package storage provides database operations for vcdeploy.
package storage

import (
	"encoding/json"
	"time"
)

// RecipeComponent represents a reusable deployment building block.
type RecipeComponent struct {
	ID            int64                `json:"id"`
	UID           string               `json:"uid"`
	Namespace     string               `json:"namespace"` // "seed" or "user"
	Slug          string               `json:"slug"`      // unique identifier within namespace
	Version       string               `json:"version"`   // semver with 'v' prefix
	Name          string               `json:"name"`      // human-readable name
	Description   string               `json:"description,omitempty"`
	ComponentType string               `json:"component_type"` // hook, command, service_reload, file_op
	Content       ComponentContent     `json:"content"`
	Variables     []VariableDefinition `json:"variables,omitempty"`
	IsSeed        bool                 `json:"is_seed"`
	IsRaw         bool                 `json:"is_raw"` // requires admin approval
	IsDeprecated  bool                 `json:"is_deprecated"`
	CreatedAt     time.Time            `json:"created_at"`
}

// ComponentContent holds the actual commands/operations.
type ComponentContent struct {
	Commands        []string `json:"commands,omitempty"`
	WorkDir         string   `json:"work_dir,omitempty"`
	User            string   `json:"user,omitempty"`
	Timeout         int      `json:"timeout,omitempty"` // seconds
	ContinueOnError bool     `json:"continue_on_error,omitempty"`
	Condition       string   `json:"condition,omitempty"`      // expression to evaluate
	ServiceName     string   `json:"service_name,omitempty"`   // for service_reload
	ServiceAction   string   `json:"service_action,omitempty"` // reload, restart
	SourcePath      string   `json:"source_path,omitempty"`    // for file_op
	DestPath        string   `json:"dest_path,omitempty"`      // for file_op
	FileOp          string   `json:"file_op,omitempty"`        // copy, move, delete, chmod, chown
	FileMode        string   `json:"file_mode,omitempty"`      // for chmod
	FileOwner       string   `json:"file_owner,omitempty"`     // for chown
}

// VariableDefinition describes a variable required by a component.
type VariableDefinition struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string, int, bool, path
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"` // should not be logged
}

// ContentJSON returns content as JSON for database storage.
func (c *RecipeComponent) ContentJSON() (string, error) {
	data, err := json.Marshal(c.Content)
	return string(data), err
}

// VariablesJSON returns variables as JSON for database storage.
func (c *RecipeComponent) VariablesJSON() (string, error) {
	if c.Variables == nil {
		return "[]", nil
	}
	data, err := json.Marshal(c.Variables)
	return string(data), err
}

// ParseContentJSON parses content from JSON.
func (c *RecipeComponent) ParseContentJSON(data string) error {
	return json.Unmarshal([]byte(data), &c.Content)
}

// ParseVariablesJSON parses variables from JSON.
func (c *RecipeComponent) ParseVariablesJSON(data string) error {
	if data == "" || data == "null" {
		c.Variables = nil
		return nil
	}
	return json.Unmarshal([]byte(data), &c.Variables)
}

// ComponentType constants.
const (
	ComponentTypeHook          = "hook"
	ComponentTypeCommand       = "command"
	ComponentTypeServiceReload = "service_reload"
	ComponentTypeFileOp        = "file_op"
)

// ValidComponentTypes returns all valid component types.
func ValidComponentTypes() []string {
	return []string{
		ComponentTypeHook,
		ComponentTypeCommand,
		ComponentTypeServiceReload,
		ComponentTypeFileOp,
	}
}

// Namespace constants.
const (
	NamespaceSeed = "seed"
	NamespaceUser = "user"
)

// ValidNamespaces returns all valid namespaces.
func ValidNamespaces() []string {
	return []string{NamespaceSeed, NamespaceUser}
}
