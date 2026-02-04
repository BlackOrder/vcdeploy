// Package export provides recipe export/import functionality.
package export

import (
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// FormatVersion is the current export format version.
const FormatVersion = "v1.0.0"

// ExportBundle contains exported recipe data.
//
//nolint:revive // ExportBundle is more descriptive than Bundle when used outside package
type ExportBundle struct {
	FormatVersion   string            `json:"format_version"`
	ExportedAt      time.Time         `json:"exported_at"`
	VCDeployVersion string            `json:"vcdeploy_version"`
	Components      []ComponentExport `json:"components"`
	Playbooks       []PlaybookExport  `json:"playbooks"`
}

// ComponentExport is a component in export format.
type ComponentExport struct {
	Slug          string                       `json:"slug"`
	Version       string                       `json:"version"`
	Name          string                       `json:"name"`
	Description   string                       `json:"description,omitempty"`
	ComponentType string                       `json:"component_type"`
	Content       storage.ComponentContent     `json:"content"`
	Variables     []storage.VariableDefinition `json:"variables,omitempty"`
	IsRaw         bool                         `json:"is_raw"`
}

// PlaybookExport is a playbook in export format.
type PlaybookExport struct {
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

// ConflictStrategy determines how to handle import conflicts.
type ConflictStrategy string

const (
	// ConflictSkip skips importing items that already exist.
	ConflictSkip ConflictStrategy = "skip"
	// ConflictOverwrite deletes existing items and imports new ones.
	ConflictOverwrite ConflictStrategy = "overwrite"
	// ConflictRename appends "-imported" to the slug for conflicting items.
	ConflictRename ConflictStrategy = "rename"
)

// IsValid returns true if the strategy is a known value.
func (s ConflictStrategy) IsValid() bool {
	switch s {
	case ConflictSkip, ConflictOverwrite, ConflictRename:
		return true
	default:
		return false
	}
}
