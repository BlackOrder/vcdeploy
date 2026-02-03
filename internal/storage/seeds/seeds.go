// Package seeds provides seed data infrastructure for built-in recipes and playbooks.
package seeds

import (
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// SeedComponent defines a built-in recipe component.
type SeedComponent struct {
	Slug        string
	Version     string // semver with 'v' prefix
	Name        string
	Description string
	Type        string // hook, command, service_reload, file_op
	Content     storage.ComponentContent
	Variables   []storage.VariableDefinition
}

// SeedPlaybook defines a built-in playbook.
type SeedPlaybook struct {
	Slug          string
	Version       string // semver with 'v' prefix
	Name          string
	Description   string
	FrameworkType string
	Steps         []storage.PlaybookStep
	SharedDirs    []string
	SharedFiles   []string
	WritableDirs  []string
	KeepReleases  int
}

// Empty state constants for UI
const (
	EmptySeedTitle   = "No Built-in Recipes Yet"
	EmptySeedMessage = "Built-in recipes are coming soon. You can create your own components and playbooks now, or check back after the next update for pre-built deployment patterns for Laravel, Node.js, and more."

	EmptyComponentsMessage = "No recipe components found. Create your first component to get started with reusable deployment steps."
	EmptyPlaybooksMessage  = "No playbooks found. Create a playbook to compose your deployment workflow."
)

// Loader handles seed data operations.
type Loader struct {
	store storage.Store
}

// NewLoader creates a new seed loader.
func NewLoader(store storage.Store) *Loader {
	return &Loader{store: store}
}

// HasSeeds returns true if seed data is defined.
func (l *Loader) HasSeeds() bool {
	return len(SeedComponents) > 0 || len(SeedPlaybooks) > 0
}

// GetEmptyStateTitle returns the title for empty seed state.
func GetEmptyStateTitle() string {
	return EmptySeedTitle
}

// GetEmptyStateMessage returns the message for empty seed state.
func GetEmptyStateMessage() string {
	return EmptySeedMessage
}

// GetEmptyComponentsMessage returns the message for empty components state.
func GetEmptyComponentsMessage() string {
	return EmptyComponentsMessage
}

// GetEmptyPlaybooksMessage returns the message for empty playbooks state.
func GetEmptyPlaybooksMessage() string {
	return EmptyPlaybooksMessage
}
