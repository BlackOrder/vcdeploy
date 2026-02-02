// Package webhooks provides project webhook configuration management.
package webhooks

import (
	"context"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services/testutil"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func setupTest(t *testing.T) (*Service, func()) {
	t.Helper()
	db, cleanup := testutil.NewTestStore(t)
	svc := New(db, nil) // No KMS for tests
	return svc, cleanup
}

func setupTestWithKMS(t *testing.T) (*Service, storage.Store, func()) {
	t.Helper()

	db, cleanup := testutil.NewTestStore(t)

	// db implements storage.Store interface
	kms, err := security.NewKMS(context.Background(), db, nil)
	if err != nil {
		t.Fatalf("Failed to create KMS: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize KMS: %v", err)
	}

	return New(db, kms), db, cleanup
}

// --- New() Tests ---

func TestNew(t *testing.T) {
	db, cleanup := testutil.NewTestStore(t)
	defer cleanup()

	svc := New(db, nil)
	if svc == nil {
		t.Fatal("New() returned nil")
	}
	if svc.store != db {
		t.Error("New() did not set db correctly")
	}
	if svc.kms != nil {
		t.Error("New() kms should be nil when passed nil")
	}
}

func TestNew_WithKMS(t *testing.T) {
	svc, _, cleanup := setupTestWithKMS(t)
	defer cleanup()

	if svc == nil {
		t.Fatal("New() returned nil")
	}
	if svc.kms == nil {
		t.Error("New() kms should be set when passed a valid KMS")
	}
}

// --- Get() Tests ---

func TestService_Get(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "github"

	// Set up a webhook first
	err := svc.Set(ctx, projectID, provider, []byte("secret"), true, true)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	webhook, err := svc.Get(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if webhook == nil {
		t.Fatal("Get() returned nil webhook")
	}
	if webhook.ProjectID != projectID {
		t.Errorf("Get() ProjectID = %v, want %v", webhook.ProjectID, projectID)
	}
	if webhook.Provider != provider {
		t.Errorf("Get() Provider = %v, want %v", webhook.Provider, provider)
	}
}

func TestService_Get_NotFound(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	_, err := svc.Get(ctx, 9999, "nonexistent")
	if err == nil {
		t.Error("Get() expected error for nonexistent webhook")
	}
	if err != storage.ErrNotFound {
		t.Errorf("Get() error = %v, want %v", err, storage.ErrNotFound)
	}
}

func TestService_Get_DifferentProvider(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)

	// Set up a webhook for github
	err := svc.Set(ctx, projectID, "github", []byte("secret"), true, true)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Try to get a webhook for gitlab
	_, err = svc.Get(ctx, projectID, "gitlab")
	if err != storage.ErrNotFound {
		t.Errorf("Get() error = %v, want %v", err, storage.ErrNotFound)
	}
}

func TestService_Get_DifferentProject(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Set up a webhook for project 1
	err := svc.Set(ctx, 1, "github", []byte("secret"), true, true)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Try to get a webhook for project 2
	_, err = svc.Get(ctx, 2, "github")
	if err != storage.ErrNotFound {
		t.Errorf("Get() error = %v, want %v", err, storage.ErrNotFound)
	}
}

// --- Set() Tests ---

func TestService_Set(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "github"

	err := svc.Set(ctx, projectID, provider, []byte("secret123"), true, true)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	webhook, err := svc.Get(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if webhook.Enabled != true {
		t.Error("Expected webhook to be enabled")
	}
	if webhook.RequireSecret != true {
		t.Error("Expected webhook to require secret")
	}
}

func TestService_Set_EmptySecret(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "github"

	// Set with empty secret
	err := svc.Set(ctx, projectID, provider, []byte{}, true, false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	webhook, err := svc.Get(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(webhook.SecretEncrypted) != 0 {
		t.Errorf("Expected empty secret, got %d bytes", len(webhook.SecretEncrypted))
	}
}

func TestService_Set_NilSecret(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "github"

	// Set with nil secret
	err := svc.Set(ctx, projectID, provider, nil, true, false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	webhook, err := svc.Get(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(webhook.SecretEncrypted) != 0 {
		t.Errorf("Expected empty secret, got %d bytes", len(webhook.SecretEncrypted))
	}
}

func TestService_Set_WithKMS(t *testing.T) {
	svc, _, cleanup := setupTestWithKMS(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "github"
	secret := []byte("my-secret-token-123")

	err := svc.Set(ctx, projectID, provider, secret, true, true)
	if err != nil {
		t.Fatalf("Set() with KMS error = %v", err)
	}

	// Verify webhook was set
	webhook, err := svc.Get(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if webhook == nil {
		t.Fatal("Get() returned nil")
	}
	// The secret should be encrypted (different from plaintext)
	if string(webhook.SecretEncrypted) == string(secret) {
		t.Error("Set() with KMS should encrypt the secret")
	}
}

func TestService_Set_WithKMS_EmptySecret(t *testing.T) {
	svc, _, cleanup := setupTestWithKMS(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "github"

	// Set with empty secret and KMS
	err := svc.Set(ctx, projectID, provider, []byte{}, true, false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	webhook, err := svc.Get(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	// Empty secret should remain empty even with KMS
	if len(webhook.SecretEncrypted) != 0 {
		t.Errorf("Expected empty secret, got %d bytes", len(webhook.SecretEncrypted))
	}
}

func TestService_Set_Update(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "bitbucket"

	// Initial set
	err := svc.Set(ctx, projectID, provider, []byte("secret1"), true, false)
	if err != nil {
		t.Fatalf("Set() initial error = %v", err)
	}

	// Update
	err = svc.Set(ctx, projectID, provider, []byte("secret2"), false, true)
	if err != nil {
		t.Fatalf("Set() update error = %v", err)
	}

	// Verify update
	webhook, err := svc.Get(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if webhook.Enabled != false {
		t.Error("Expected webhook to be disabled after update")
	}
	if webhook.RequireSecret != true {
		t.Error("Expected webhook to require secret after update")
	}
}

func TestService_Set_MultipleProviders(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)

	providers := []string{"github", "gitlab", "bitbucket"}
	for _, provider := range providers {
		err := svc.Set(ctx, projectID, provider, []byte("secret-"+provider), true, true)
		if err != nil {
			t.Fatalf("Set() for %s error = %v", provider, err)
		}
	}

	// Verify all providers are set
	for _, provider := range providers {
		webhook, err := svc.Get(ctx, projectID, provider)
		if err != nil {
			t.Fatalf("Get() for %s error = %v", provider, err)
		}
		if webhook.Provider != provider {
			t.Errorf("Get() Provider = %v, want %v", webhook.Provider, provider)
		}
	}
}

func TestService_Set_MultipleProjects(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	provider := "github"

	projectIDs := []int64{1, 2, 3}
	for _, projectID := range projectIDs {
		err := svc.Set(ctx, projectID, provider, []byte("secret"), true, true)
		if err != nil {
			t.Fatalf("Set() for project %d error = %v", projectID, err)
		}
	}

	// Verify all projects have their own webhook
	for _, projectID := range projectIDs {
		webhook, err := svc.Get(ctx, projectID, provider)
		if err != nil {
			t.Fatalf("Get() for project %d error = %v", projectID, err)
		}
		if webhook.ProjectID != projectID {
			t.Errorf("Get() ProjectID = %v, want %v", webhook.ProjectID, projectID)
		}
	}
}

// --- List() Tests ---

func TestService_List(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)

	// Set up multiple webhooks
	providers := []string{"github", "gitlab", "bitbucket"}
	for _, provider := range providers {
		err := svc.Set(ctx, projectID, provider, []byte("secret"), true, true)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	webhooks, err := svc.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(webhooks) != 3 {
		t.Errorf("List() returned %d webhooks, want 3", len(webhooks))
	}
}

func TestService_List_Empty(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	webhooks, err := svc.List(ctx, 9999)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(webhooks) != 0 {
		t.Errorf("List() returned %d webhooks, want 0", len(webhooks))
	}
}

func TestService_List_OnlyOwnProject(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Set up webhooks for different projects
	err := svc.Set(ctx, 1, "github", []byte("secret"), true, true)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	err = svc.Set(ctx, 2, "gitlab", []byte("secret"), true, true)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// List should only return webhooks for project 1
	webhooks, err := svc.List(ctx, 1)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(webhooks) != 1 {
		t.Errorf("List() returned %d webhooks, want 1", len(webhooks))
	}
	if webhooks[0].ProjectID != 1 {
		t.Errorf("List() returned webhook for wrong project: %v", webhooks[0].ProjectID)
	}
}

// --- Delete() Tests ---

func TestService_Delete(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "github"

	// Set up a webhook
	err := svc.Set(ctx, projectID, provider, []byte("secret"), true, true)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Delete the webhook
	err = svc.Delete(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	_, err = svc.Get(ctx, projectID, provider)
	if err != storage.ErrNotFound {
		t.Errorf("Get() after delete error = %v, want %v", err, storage.ErrNotFound)
	}
}

func TestService_Delete_NonExistent(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Delete a non-existent webhook - should not error
	err := svc.Delete(ctx, 9999, "nonexistent")
	if err != nil {
		t.Fatalf("Delete() of non-existent webhook error = %v", err)
	}
}

func TestService_Delete_OnlyTargetedWebhook(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)

	// Set up multiple webhooks
	err := svc.Set(ctx, projectID, "github", []byte("secret"), true, true)
	if err != nil {
		t.Fatalf("Set() github error = %v", err)
	}
	err = svc.Set(ctx, projectID, "gitlab", []byte("secret"), true, true)
	if err != nil {
		t.Fatalf("Set() gitlab error = %v", err)
	}

	// Delete only github
	err = svc.Delete(ctx, projectID, "github")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify github is deleted
	_, err = svc.Get(ctx, projectID, "github")
	if err != storage.ErrNotFound {
		t.Error("Delete() should have deleted github webhook")
	}

	// Verify gitlab still exists
	webhook, err := svc.Get(ctx, projectID, "gitlab")
	if err != nil {
		t.Fatalf("Get() gitlab after delete error = %v", err)
	}
	if webhook == nil {
		t.Error("Delete() should not have deleted gitlab webhook")
	}
}

// --- GetDecryptedSecret() Tests ---

func TestService_GetDecryptedSecret(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "gitlab"
	secret := []byte("my-secret-token")

	// Set a webhook with secret
	err := svc.Set(ctx, projectID, provider, secret, true, true)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get decrypted secret (without KMS, returns raw value)
	decrypted, err := svc.GetDecryptedSecret(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("GetDecryptedSecret() error = %v", err)
	}
	if string(decrypted) != string(secret) {
		t.Errorf("Expected secret %q, got %q", secret, decrypted)
	}
}

func TestService_GetDecryptedSecret_NotFound(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	_, err := svc.GetDecryptedSecret(ctx, 9999, "nonexistent")
	if err == nil {
		t.Error("GetDecryptedSecret() expected error for nonexistent webhook")
	}
	if err != storage.ErrNotFound {
		t.Errorf("GetDecryptedSecret() error = %v, want %v", err, storage.ErrNotFound)
	}
}

func TestService_GetDecryptedSecret_EmptySecret(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "github"

	// Set a webhook without a secret
	err := svc.Set(ctx, projectID, provider, nil, true, false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get decrypted secret should return nil
	decrypted, err := svc.GetDecryptedSecret(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("GetDecryptedSecret() error = %v", err)
	}
	if decrypted != nil {
		t.Errorf("Expected nil secret, got %q", decrypted)
	}
}

func TestService_GetDecryptedSecret_WithKMS(t *testing.T) {
	svc, _, cleanup := setupTestWithKMS(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "github"
	secret := []byte("my-super-secret-token")

	// Set a webhook with secret (will be encrypted)
	err := svc.Set(ctx, projectID, provider, secret, true, true)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get decrypted secret (should decrypt properly)
	decrypted, err := svc.GetDecryptedSecret(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("GetDecryptedSecret() error = %v", err)
	}
	if string(decrypted) != string(secret) {
		t.Errorf("Expected secret %q, got %q", secret, decrypted)
	}
}

func TestService_GetDecryptedSecret_WithKMS_EmptySecret(t *testing.T) {
	svc, _, cleanup := setupTestWithKMS(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "github"

	// Set a webhook without a secret (empty)
	err := svc.Set(ctx, projectID, provider, []byte{}, true, false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get decrypted secret should return nil for empty secret
	decrypted, err := svc.GetDecryptedSecret(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("GetDecryptedSecret() error = %v", err)
	}
	if decrypted != nil {
		t.Errorf("Expected nil secret for empty, got %q", decrypted)
	}
}

// --- CleanupOrphanedWebhooks() Tests ---

func TestService_CleanupOrphanedWebhooks(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Run cleanup - should not error even with no orphaned webhooks
	count, err := svc.CleanupOrphanedWebhooks(ctx)
	if err != nil {
		t.Fatalf("CleanupOrphanedWebhooks() error = %v", err)
	}

	// Initial cleanup should return 0 (no orphans)
	if count != 0 {
		t.Logf("Initial cleanup removed %d orphaned webhooks", count)
	}
}

func TestService_CleanupOrphanedWebhooks_MultipleCleanups(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Run cleanup multiple times - should be idempotent
	for i := 0; i < 3; i++ {
		count, err := svc.CleanupOrphanedWebhooks(ctx)
		if err != nil {
			t.Fatalf("CleanupOrphanedWebhooks() iteration %d error = %v", i, err)
		}
		if count != 0 {
			t.Errorf("CleanupOrphanedWebhooks() iteration %d count = %d, want 0", i, count)
		}
	}
}

// --- Integration Tests ---

func TestService_GetSetDelete_Integration(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)
	provider := "github"

	// Set a webhook
	err := svc.Set(ctx, projectID, provider, []byte("secret123"), true, true)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get the webhook
	webhook, err := svc.Get(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if webhook.Enabled != true {
		t.Error("Expected webhook to be enabled")
	}
	if webhook.RequireSecret != true {
		t.Error("Expected webhook to require secret")
	}

	// List webhooks
	webhooks, err := svc.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(webhooks) != 1 {
		t.Errorf("Expected 1 webhook, got %d", len(webhooks))
	}

	// Delete webhook
	err = svc.Delete(ctx, projectID, provider)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	webhooks, err = svc.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List() after delete error = %v", err)
	}
	if len(webhooks) != 0 {
		t.Errorf("Expected 0 webhooks after delete, got %d", len(webhooks))
	}
}

func TestService_FullLifecycle_WithKMS(t *testing.T) {
	svc, _, cleanup := setupTestWithKMS(t)
	defer cleanup()

	ctx := context.Background()
	projectID := int64(1)

	providers := []string{"github", "gitlab", "bitbucket"}
	secrets := map[string]string{
		"github":    "github-secret-token",
		"gitlab":    "gitlab-secret-token",
		"bitbucket": "bitbucket-secret-token",
	}

	// Set up all webhooks
	for _, provider := range providers {
		err := svc.Set(ctx, projectID, provider, []byte(secrets[provider]), true, true)
		if err != nil {
			t.Fatalf("Set() for %s error = %v", provider, err)
		}
	}

	// Verify all webhooks exist
	webhooks, err := svc.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(webhooks) != len(providers) {
		t.Errorf("List() returned %d webhooks, want %d", len(webhooks), len(providers))
	}

	// Verify all secrets can be decrypted
	for _, provider := range providers {
		decrypted, err := svc.GetDecryptedSecret(ctx, projectID, provider)
		if err != nil {
			t.Fatalf("GetDecryptedSecret() for %s error = %v", provider, err)
		}
		if string(decrypted) != secrets[provider] {
			t.Errorf("GetDecryptedSecret() for %s = %q, want %q", provider, decrypted, secrets[provider])
		}
	}

	// Update a webhook
	err = svc.Set(ctx, projectID, "github", []byte("new-github-secret"), false, false)
	if err != nil {
		t.Fatalf("Set() update error = %v", err)
	}

	// Verify update
	webhook, err := svc.Get(ctx, projectID, "github")
	if err != nil {
		t.Fatalf("Get() after update error = %v", err)
	}
	if webhook.Enabled != false {
		t.Error("Expected webhook to be disabled after update")
	}

	decrypted, err := svc.GetDecryptedSecret(ctx, projectID, "github")
	if err != nil {
		t.Fatalf("GetDecryptedSecret() after update error = %v", err)
	}
	if string(decrypted) != "new-github-secret" {
		t.Errorf("GetDecryptedSecret() after update = %q, want %q", decrypted, "new-github-secret")
	}

	// Delete all webhooks
	for _, provider := range providers {
		err := svc.Delete(ctx, projectID, provider)
		if err != nil {
			t.Fatalf("Delete() for %s error = %v", provider, err)
		}
	}

	// Verify all deleted
	webhooks, err = svc.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List() after delete error = %v", err)
	}
	if len(webhooks) != 0 {
		t.Errorf("List() after delete returned %d webhooks, want 0", len(webhooks))
	}
}

// --- Table-Driven Tests ---

func TestService_Set_Variations(t *testing.T) {
	tests := []struct {
		name          string
		projectID     int64
		provider      string
		secret        []byte
		enabled       bool
		requireSecret bool
	}{
		{
			name:          "enabled_with_secret",
			projectID:     1,
			provider:      "github",
			secret:        []byte("secret123"),
			enabled:       true,
			requireSecret: true,
		},
		{
			name:          "disabled_without_secret",
			projectID:     2,
			provider:      "gitlab",
			secret:        nil,
			enabled:       false,
			requireSecret: false,
		},
		{
			name:          "enabled_no_secret_required",
			projectID:     3,
			provider:      "bitbucket",
			secret:        []byte("secret"),
			enabled:       true,
			requireSecret: false,
		},
		{
			name:          "disabled_with_secret_required",
			projectID:     4,
			provider:      "custom",
			secret:        []byte("custom-secret"),
			enabled:       false,
			requireSecret: true,
		},
		{
			name:          "empty_secret_string",
			projectID:     5,
			provider:      "webhook",
			secret:        []byte(""),
			enabled:       true,
			requireSecret: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, cleanup := setupTest(t)
			defer cleanup()

			ctx := context.Background()

			err := svc.Set(ctx, tt.projectID, tt.provider, tt.secret, tt.enabled, tt.requireSecret)
			if err != nil {
				t.Fatalf("Set() error = %v", err)
			}

			webhook, err := svc.Get(ctx, tt.projectID, tt.provider)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}

			if webhook.Enabled != tt.enabled {
				t.Errorf("Enabled = %v, want %v", webhook.Enabled, tt.enabled)
			}
			if webhook.RequireSecret != tt.requireSecret {
				t.Errorf("RequireSecret = %v, want %v", webhook.RequireSecret, tt.requireSecret)
			}
		})
	}
}

func TestService_Get_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		projectID int64
		provider  string
		setup     func(*Service, context.Context) error
		wantErr   bool
	}{
		{
			name:      "existing_webhook",
			projectID: 1,
			provider:  "github",
			setup: func(svc *Service, ctx context.Context) error {
				return svc.Set(ctx, 1, "github", []byte("secret"), true, true)
			},
			wantErr: false,
		},
		{
			name:      "nonexistent_webhook",
			projectID: 999,
			provider:  "github",
			setup:     nil,
			wantErr:   true,
		},
		{
			name:      "wrong_provider",
			projectID: 1,
			provider:  "bitbucket",
			setup: func(svc *Service, ctx context.Context) error {
				return svc.Set(ctx, 1, "github", []byte("secret"), true, true)
			},
			wantErr: true,
		},
		{
			name:      "zero_project_id",
			projectID: 0,
			provider:  "github",
			setup:     nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, cleanup := setupTest(t)
			defer cleanup()

			ctx := context.Background()

			if tt.setup != nil {
				if err := tt.setup(svc, ctx); err != nil {
					t.Fatalf("Setup error = %v", err)
				}
			}

			_, err := svc.Get(ctx, tt.projectID, tt.provider)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- DB Error Tests ---

func setupTestWithDB(t *testing.T) (*Service, storage.Store, func()) {
	t.Helper()

	db, cleanup := testutil.NewTestStore(t)

	return New(db, nil), db, cleanup
}

func TestService_List_DBError(t *testing.T) {
	svc, db, cleanup := setupTestWithDB(t)
	defer cleanup()

	ctx := context.Background()

	// Close the DB to trigger an error
	db.Close()

	_, err := svc.List(ctx, 1)
	if err == nil {
		t.Error("List() expected error when DB is closed")
	}
}

func TestService_Delete_DBError(t *testing.T) {
	svc, db, cleanup := setupTestWithDB(t)
	defer cleanup()

	ctx := context.Background()

	// Close the DB to trigger an error
	db.Close()

	err := svc.Delete(ctx, 1, "github")
	if err == nil {
		t.Error("Delete() expected error when DB is closed")
	}
}

func TestService_CleanupOrphanedWebhooks_DBError(t *testing.T) {
	svc, db, cleanup := setupTestWithDB(t)
	defer cleanup()

	ctx := context.Background()

	// Close the DB to trigger an error
	db.Close()

	_, err := svc.CleanupOrphanedWebhooks(ctx)
	if err == nil {
		t.Error("CleanupOrphanedWebhooks() expected error when DB is closed")
	}
}

func TestService_Set_DBError(t *testing.T) {
	svc, db, cleanup := setupTestWithDB(t)
	defer cleanup()

	ctx := context.Background()

	// Close the DB to trigger an error
	db.Close()

	err := svc.Set(ctx, 1, "github", []byte("secret"), true, true)
	if err == nil {
		t.Error("Set() expected error when DB is closed")
	}
}
