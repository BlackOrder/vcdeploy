package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore_RecipeComponent_CRUD(t *testing.T) {
	store := NewMemoryStore(nil)
	ctx := context.Background()

	// Create a recipe component
	component := &RecipeComponent{
		Namespace:     "seed",
		Slug:          "test-hook",
		Version:       "v1.0.0",
		Name:          "Test Hook",
		Description:   "A test hook component",
		ComponentType: "hook",
		Content: ComponentContent{
			Commands: []string{"echo 'Hello'", "echo 'World'"},
			WorkDir:  "/var/www",
		},
		Variables: []VariableDefinition{
			{Name: "APP_ENV", Type: "string", Required: true},
		},
		IsSeed: true,
	}

	err := store.CreateRecipeComponent(ctx, component)
	require.NoError(t, err)
	assert.NotZero(t, component.ID)

	// Read by ID
	found, err := store.GetRecipeComponentByID(ctx, component.ID)
	require.NoError(t, err)
	assert.Equal(t, component.Name, found.Name)
	assert.Equal(t, component.Namespace, found.Namespace)
	assert.Equal(t, len(component.Content.Commands), len(found.Content.Commands))

	// Read by key
	found, err = store.GetRecipeComponent(ctx, "seed", "test-hook", "v1.0.0")
	require.NoError(t, err)
	assert.Equal(t, component.ID, found.ID)

	// Update
	found.Description = "Updated description"
	err = store.UpdateRecipeComponent(ctx, found)
	require.NoError(t, err)

	updated, err := store.GetRecipeComponentByID(ctx, component.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated description", updated.Description)

	// List
	components, err := store.ListRecipeComponents(ctx, "seed", false)
	require.NoError(t, err)
	assert.Len(t, components, 1)

	// List versions
	versions, err := store.ListRecipeComponentVersions(ctx, "seed", "test-hook")
	require.NoError(t, err)
	assert.Len(t, versions, 1)

	// Delete
	err = store.DeleteRecipeComponent(ctx, component.ID)
	require.NoError(t, err)

	_, err = store.GetRecipeComponentByID(ctx, component.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_RecipeComponent_DuplicateKey(t *testing.T) {
	store := NewMemoryStore(nil)
	ctx := context.Background()

	component := &RecipeComponent{
		Namespace:     "seed",
		Slug:          "test-hook",
		Version:       "v1.0.0",
		Name:          "Test Hook",
		ComponentType: "hook",
	}

	err := store.CreateRecipeComponent(ctx, component)
	require.NoError(t, err)

	// Try to create duplicate
	duplicate := &RecipeComponent{
		Namespace:     "seed",
		Slug:          "test-hook",
		Version:       "v1.0.0",
		Name:          "Duplicate Hook",
		ComponentType: "hook",
	}

	err = store.CreateRecipeComponent(ctx, duplicate)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestMemoryStore_Playbook_CRUD(t *testing.T) {
	store := NewMemoryStore(nil)
	ctx := context.Background()

	playbook := &Playbook{
		Namespace:     "seed",
		Slug:          "laravel",
		Version:       "v1.0.0",
		Name:          "Laravel Playbook",
		Description:   "Standard Laravel deployment",
		FrameworkType: "laravel",
		Steps: []PlaybookStep{
			{Order: 1, ComponentRef: "seed:artisan-migrate:v1.0.0", Phase: PhaseDeploy},
			{Order: 2, ComponentRef: "seed:artisan-cache:v1.0.0", Phase: PhasePostDeploy},
		},
		SharedDirs:   []string{"storage", "bootstrap/cache"},
		WritableDirs: []string{"storage/logs"},
		KeepReleases: 5,
		IsSeed:       true,
	}

	err := store.CreatePlaybook(ctx, playbook)
	require.NoError(t, err)
	assert.NotZero(t, playbook.ID)

	// Read by ID
	found, err := store.GetPlaybookByID(ctx, playbook.ID)
	require.NoError(t, err)
	assert.Equal(t, playbook.Name, found.Name)
	assert.Len(t, found.Steps, 2)
	assert.Len(t, found.SharedDirs, 2)

	// Read by key
	found, err = store.GetPlaybook(ctx, "seed", "laravel", "v1.0.0")
	require.NoError(t, err)
	assert.Equal(t, playbook.ID, found.ID)

	// Update
	found.Description = "Updated Laravel deployment"
	found.Steps = append(found.Steps, PlaybookStep{
		Order: 3, ComponentRef: "seed:artisan-optimize:v1.0.0", Phase: PhaseFinalize,
	})
	err = store.UpdatePlaybook(ctx, found)
	require.NoError(t, err)

	updated, err := store.GetPlaybookByID(ctx, playbook.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Laravel deployment", updated.Description)
	assert.Len(t, updated.Steps, 3)

	// List by framework
	playbooks, err := store.ListPlaybooks(ctx, "", "laravel", false)
	require.NoError(t, err)
	assert.Len(t, playbooks, 1)

	// Delete
	err = store.DeletePlaybook(ctx, playbook.ID)
	require.NoError(t, err)

	_, err = store.GetPlaybookByID(ctx, playbook.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_PlaybookActivation_CRUD(t *testing.T) {
	store := NewMemoryStore(nil)
	ctx := context.Background()

	// Create a playbook first
	playbook := &Playbook{
		Namespace: "seed",
		Slug:      "laravel",
		Version:   "v1.0.0",
		Name:      "Laravel Playbook",
		IsSeed:    true,
	}
	err := store.CreatePlaybook(ctx, playbook)
	require.NoError(t, err)

	// Create activation
	activation := &PlaybookActivation{
		ProjectID:   1,
		PlaybookID:  playbook.ID,
		ActivatedAt: time.Now(),
	}

	err = store.CreatePlaybookActivation(ctx, activation)
	require.NoError(t, err)
	assert.NotZero(t, activation.ID)

	// Read by project ID
	found, err := store.GetPlaybookActivation(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, activation.ID, found.ID)
	assert.Equal(t, playbook.ID, found.PlaybookID)

	// Read by ID
	found, err = store.GetPlaybookActivationByID(ctx, activation.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), found.ProjectID)

	// List by playbook
	activations, err := store.ListActivationsByPlaybook(ctx, playbook.ID)
	require.NoError(t, err)
	assert.Len(t, activations, 1)

	// Delete
	err = store.DeletePlaybookActivation(ctx, activation.ID)
	require.NoError(t, err)

	_, err = store.GetPlaybookActivation(ctx, 1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_VariableBinding_CRUD(t *testing.T) {
	store := NewMemoryStore(nil)
	ctx := context.Background()

	// Create a binding
	binding := &PlaybookVariableBinding{
		ActivationID: 1,
		VariableName: "DB_PASSWORD",
		SourceType:   SourceTypeSecret,
		SourceRef:    "database/password",
	}

	err := store.CreateVariableBinding(ctx, binding)
	require.NoError(t, err)
	assert.NotZero(t, binding.ID)

	// Get bindings for activation
	bindings, err := store.GetVariableBindings(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, bindings, 1)
	assert.Equal(t, "DB_PASSWORD", bindings[0].VariableName)

	// Find by source ref
	found, err := store.FindBindingsBySourceRef(ctx, SourceTypeSecret, "database/password")
	require.NoError(t, err)
	assert.Len(t, found, 1)

	// Update
	binding.LiteralValue = ""
	binding.SourceRef = "database/new-password"
	err = store.UpdateVariableBinding(ctx, binding)
	require.NoError(t, err)

	// Old ref should return empty
	found, err = store.FindBindingsBySourceRef(ctx, SourceTypeSecret, "database/password")
	require.NoError(t, err)
	assert.Len(t, found, 0)

	// New ref should work
	found, err = store.FindBindingsBySourceRef(ctx, SourceTypeSecret, "database/new-password")
	require.NoError(t, err)
	assert.Len(t, found, 1)

	// Delete
	err = store.DeleteVariableBinding(ctx, binding.ID)
	require.NoError(t, err)

	bindings, err = store.GetVariableBindings(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, bindings, 0)
}

func TestMemoryStore_RawApproval_CRUD(t *testing.T) {
	store := NewMemoryStore(nil)
	ctx := context.Background()

	// Create a raw approval
	approval := &RawCommandApproval{
		ComponentID:  1,
		ApprovedBy:   1,
		ApprovedAt:   time.Now(),
		ApprovalNote: "Approved for production use",
	}

	err := store.CreateRawApproval(ctx, approval)
	require.NoError(t, err)
	assert.NotZero(t, approval.ID)

	// Get by component ID
	found, err := store.GetRawApproval(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, approval.ID, found.ID)
	assert.Equal(t, "Approved for production use", found.ApprovalNote)

	// List all
	approvals, err := store.ListRawApprovals(ctx)
	require.NoError(t, err)
	assert.Len(t, approvals, 1)

	// Delete by component ID
	err = store.DeleteRawApproval(ctx, 1)
	require.NoError(t, err)

	_, err = store.GetRawApproval(ctx, 1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_ListRecipeComponents_NamespaceFilter(t *testing.T) {
	store := NewMemoryStore(nil)
	ctx := context.Background()

	// Create components in different namespaces
	seedComponent := &RecipeComponent{
		Namespace:     "seed",
		Slug:          "seed-hook",
		Version:       "v1.0.0",
		Name:          "Seed Hook",
		ComponentType: "hook",
		IsSeed:        true,
	}
	userComponent := &RecipeComponent{
		Namespace:     "user",
		Slug:          "user-hook",
		Version:       "v1.0.0",
		Name:          "User Hook",
		ComponentType: "hook",
		IsSeed:        false,
	}

	require.NoError(t, store.CreateRecipeComponent(ctx, seedComponent))
	require.NoError(t, store.CreateRecipeComponent(ctx, userComponent))

	// List all
	all, err := store.ListRecipeComponents(ctx, "", false)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// List seed only
	seed, err := store.ListRecipeComponents(ctx, "seed", false)
	require.NoError(t, err)
	assert.Len(t, seed, 1)
	assert.Equal(t, "seed", seed[0].Namespace)

	// List user only
	user, err := store.ListRecipeComponents(ctx, "user", false)
	require.NoError(t, err)
	assert.Len(t, user, 1)
	assert.Equal(t, "user", user[0].Namespace)
}

func TestMemoryStore_ListPlaybooks_Filters(t *testing.T) {
	store := NewMemoryStore(nil)
	ctx := context.Background()

	// Create playbooks
	laravelPlaybook := &Playbook{
		Namespace:     "seed",
		Slug:          "laravel",
		Version:       "v1.0.0",
		Name:          "Laravel",
		FrameworkType: "laravel",
		IsSeed:        true,
	}
	railsPlaybook := &Playbook{
		Namespace:     "seed",
		Slug:          "rails",
		Version:       "v1.0.0",
		Name:          "Rails",
		FrameworkType: "rails",
		IsSeed:        true,
	}
	deprecatedPlaybook := &Playbook{
		Namespace:     "seed",
		Slug:          "old",
		Version:       "v1.0.0",
		Name:          "Old",
		FrameworkType: "laravel",
		IsDeprecated:  true,
		IsSeed:        true,
	}

	require.NoError(t, store.CreatePlaybook(ctx, laravelPlaybook))
	require.NoError(t, store.CreatePlaybook(ctx, railsPlaybook))
	require.NoError(t, store.CreatePlaybook(ctx, deprecatedPlaybook))

	// List all (excluding deprecated)
	all, err := store.ListPlaybooks(ctx, "", "", false)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// List all (including deprecated)
	allWithDeprecated, err := store.ListPlaybooks(ctx, "", "", true)
	require.NoError(t, err)
	assert.Len(t, allWithDeprecated, 3)

	// Filter by framework
	laravel, err := store.ListPlaybooks(ctx, "", "laravel", false)
	require.NoError(t, err)
	assert.Len(t, laravel, 1)
	assert.Equal(t, "laravel", laravel[0].FrameworkType)
}
