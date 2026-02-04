package seeds_test

import (
	"context"
	"os"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/storage/seeds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestDB(t *testing.T) (*storage.DB, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "seeds_test_*.db")
	require.NoError(t, err)
	tmpFile.Close()

	db, err := storage.New(tmpFile.Name(), zap.NewNop())
	require.NoError(t, err)

	err = db.MigrateUp(context.Background())
	require.NoError(t, err)

	return db, func() {
		db.Close()
		os.Remove(tmpFile.Name())
	}
}

func TestLoader_HasSeeds_WithData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	loader := seeds.NewLoader(db)

	// With populated seed definitions, HasSeeds should return true
	assert.True(t, loader.HasSeeds())
}

func TestLoader_LoadSeeds_Empty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	loader := seeds.NewLoader(db)
	logger := zap.NewNop()

	// Should succeed with empty seeds
	err := loader.LoadSeeds(context.Background(), logger)
	assert.NoError(t, err)
}

func TestLoader_LoadSeeds_Idempotent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	loader := seeds.NewLoader(db)
	logger := zap.NewNop()

	// Run twice - should be idempotent
	err := loader.LoadSeeds(context.Background(), logger)
	require.NoError(t, err)

	err = loader.LoadSeeds(context.Background(), logger)
	require.NoError(t, err)
}

func TestLoader_CountSeeds(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	loader := seeds.NewLoader(db)

	components, playbooks := loader.CountSeeds()
	// With populated seed definitions
	assert.Greater(t, components, 0, "Should have seed components")
	assert.Greater(t, playbooks, 0, "Should have seed playbooks")
}

func TestGetEmptyStateTitle(t *testing.T) {
	title := seeds.GetEmptyStateTitle()
	assert.Equal(t, "No Built-in Recipes Yet", title)
}

func TestGetEmptyStateMessage(t *testing.T) {
	msg := seeds.GetEmptyStateMessage()
	assert.Contains(t, msg, "Built-in recipes are coming soon")
}

func TestGetEmptyComponentsMessage(t *testing.T) {
	msg := seeds.GetEmptyComponentsMessage()
	assert.Contains(t, msg, "No recipe components found")
}

func TestGetEmptyPlaybooksMessage(t *testing.T) {
	msg := seeds.GetEmptyPlaybooksMessage()
	assert.Contains(t, msg, "No playbooks found")
}
