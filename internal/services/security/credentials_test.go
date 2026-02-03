package security

import (
	"context"
	"errors"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/testutil"
	testutil_mocks "github.com/BlackOrder/vcdeploy/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCredentialService_Create(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()
	req := CreateCredentialRequest{
		Name:       "test-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: "github.com/.*",
		Credential: "ghp_test_token_12345",
		CreatedBy:  "test-user",
	}

	info, err := svc.CreateCredential(ctx, req)
	require.NoError(t, err)
	assert.NotEmpty(t, info.ID)
	assert.Equal(t, "test-cred", info.Name)
	assert.Equal(t, storage.SourceCredentialTypeHTTPSToken, info.Type)
	assert.Equal(t, "github.com/.*", info.URLPattern)
	assert.Equal(t, "test-user", info.CreatedBy)

	// Verify saved in database
	saved, err := svc.GetCredential(ctx, info.ID)
	require.NoError(t, err)
	assert.Equal(t, info.Name, saved.Name)
}

func TestCredentialService_Create_AllTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		credType   string
		credential string
	}{
		{
			name:       "https_token",
			credType:   storage.SourceCredentialTypeHTTPSToken,
			credential: "ghp_test_token_12345",
		},
		{
			name:       "https_basic",
			credType:   storage.SourceCredentialTypeHTTPSBasic,
			credential: "user:password123",
		},
		{
			name:       "ssh_key",
			credType:   storage.SourceCredentialTypeSSHKey,
			credential: testEd25519Key,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tdb := testutil.NewTestDB(t)
			kms := testutil_mocks.NewMockKMS()
			svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

			ctx := context.Background()
			req := CreateCredentialRequest{
				Name:       "cred-" + tt.name,
				Type:       tt.credType,
				URLPattern: ".*",
				Credential: tt.credential,
				CreatedBy:  "test-user",
			}

			info, err := svc.CreateCredential(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, tt.credType, info.Type)
		})
	}
}

func TestCredentialService_Create_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		req       CreateCredentialRequest
		wantField string
	}{
		{
			name:      "empty_name",
			req:       CreateCredentialRequest{Name: "", Type: "https_token", URLPattern: ".*", Credential: "token"},
			wantField: "name",
		},
		{
			name: "name_too_long",
			req: CreateCredentialRequest{
				Name:       string(make([]byte, 300)),
				Type:       "https_token",
				URLPattern: ".*",
				Credential: "token",
			},
			wantField: "name",
		},
		{
			name:      "invalid_type",
			req:       CreateCredentialRequest{Name: "test", Type: "invalid", URLPattern: ".*", Credential: "token"},
			wantField: "type",
		},
		{
			name:      "empty_url_pattern",
			req:       CreateCredentialRequest{Name: "test", Type: "https_token", URLPattern: "", Credential: "token"},
			wantField: "url_pattern",
		},
		{
			name:      "invalid_url_pattern",
			req:       CreateCredentialRequest{Name: "test", Type: "https_token", URLPattern: "[invalid", Credential: "token"},
			wantField: "url_pattern",
		},
		{
			name:      "empty_credential",
			req:       CreateCredentialRequest{Name: "test", Type: "https_token", URLPattern: ".*", Credential: ""},
			wantField: "credential",
		},
		{
			name:      "invalid_ssh_key",
			req:       CreateCredentialRequest{Name: "test", Type: "ssh_key", URLPattern: ".*", Credential: "short"},
			wantField: "credential",
		},
		{
			name:      "invalid_basic_auth",
			req:       CreateCredentialRequest{Name: "test", Type: "https_basic", URLPattern: ".*", Credential: "ab"},
			wantField: "credential",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tdb := testutil.NewTestDB(t)
			kms := testutil_mocks.NewMockKMS()
			svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

			ctx := context.Background()
			_, err := svc.CreateCredential(ctx, tt.req)
			require.Error(t, err)

			var inputErr *services.InputError
			require.True(t, errors.As(err, &inputErr))
			assert.Equal(t, tt.wantField, inputErr.Field)
		})
	}
}

func TestCredentialService_Create_DuplicateName(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()
	req := CreateCredentialRequest{
		Name:       "duplicate-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: ".*",
		Credential: "token12345",
		CreatedBy:  "test-user",
	}

	// Create first
	_, err := svc.CreateCredential(ctx, req)
	require.NoError(t, err)

	// Try to create duplicate
	_, err = svc.CreateCredential(ctx, req)
	require.Error(t, err)

	var inputErr *services.InputError
	require.True(t, errors.As(err, &inputErr))
	assert.Equal(t, "name", inputErr.Field)
	assert.Contains(t, inputErr.Message, "already exists")
}

func TestCredentialService_List(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	// Create some credentials
	for i := 1; i <= 3; i++ {
		_, err := svc.CreateCredential(ctx, CreateCredentialRequest{
			Name:       "cred-" + string(rune('0'+i)),
			Type:       storage.SourceCredentialTypeHTTPSToken,
			URLPattern: ".*",
			Credential: "token" + string(rune('0'+i)),
			CreatedBy:  "test-user",
		})
		require.NoError(t, err)
	}

	// List credentials
	creds, err := svc.ListCredentials(ctx)
	require.NoError(t, err)
	assert.Len(t, creds, 3)

	// Verify all have expected fields (no credential values exposed)
	for _, cred := range creds {
		assert.NotEmpty(t, cred.Name)
		assert.NotEmpty(t, cred.Type)
	}
}

func TestCredentialService_Get(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	// Create a credential
	created, err := svc.CreateCredential(ctx, CreateCredentialRequest{
		Name:       "get-test-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: "github.com/.*",
		Credential: "test_token",
		CreatedBy:  "test-user",
	})
	require.NoError(t, err)

	// Get by ID
	info, err := svc.GetCredential(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.Name, info.Name)
	assert.Equal(t, created.URLPattern, info.URLPattern)
}

func TestCredentialService_Get_NotFound(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	_, err := svc.GetCredential(ctx, 99999)
	require.Error(t, err)
}

func TestCredentialService_GetByName(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	// Create a credential
	created, err := svc.CreateCredential(ctx, CreateCredentialRequest{
		Name:       "named-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: ".*",
		Credential: "test_token",
		CreatedBy:  "test-user",
	})
	require.NoError(t, err)

	// Get by name
	info, err := svc.GetCredentialByName(ctx, "named-cred")
	require.NoError(t, err)
	assert.Equal(t, created.ID, info.ID)
}

func TestCredentialService_GetByName_NotFound(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	_, err := svc.GetCredentialByName(ctx, "nonexistent")
	require.Error(t, err)
}

func TestCredentialService_Update(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	// Create a credential
	created, err := svc.CreateCredential(ctx, CreateCredentialRequest{
		Name:       "update-test-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: "github.com/.*",
		Credential: "original_token",
		CreatedBy:  "test-user",
	})
	require.NoError(t, err)

	// Update name
	newName := "updated-cred"
	updated, err := svc.UpdateCredential(ctx, created.ID, UpdateCredentialRequest{
		Name: &newName,
	})
	require.NoError(t, err)
	assert.Equal(t, newName, updated.Name)

	// Verify persisted
	fetched, err := svc.GetCredential(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, newName, fetched.Name)
}

func TestCredentialService_Update_Type(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	created, err := svc.CreateCredential(ctx, CreateCredentialRequest{
		Name:       "type-update-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: ".*",
		Credential: "token123",
		CreatedBy:  "test-user",
	})
	require.NoError(t, err)

	newType := storage.SourceCredentialTypeHTTPSBasic
	updated, err := svc.UpdateCredential(ctx, created.ID, UpdateCredentialRequest{
		Type: &newType,
	})
	require.NoError(t, err)
	assert.Equal(t, newType, updated.Type)
}

func TestCredentialService_Update_URLPattern(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	created, err := svc.CreateCredential(ctx, CreateCredentialRequest{
		Name:       "pattern-update-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: "github.com/.*",
		Credential: "token123",
		CreatedBy:  "test-user",
	})
	require.NoError(t, err)

	newPattern := "gitlab.com/.*"
	updated, err := svc.UpdateCredential(ctx, created.ID, UpdateCredentialRequest{
		URLPattern: &newPattern,
	})
	require.NoError(t, err)
	assert.Equal(t, newPattern, updated.URLPattern)
}

func TestCredentialService_Update_Credential(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	created, err := svc.CreateCredential(ctx, CreateCredentialRequest{
		Name:       "cred-update-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: ".*",
		Credential: "original_token",
		CreatedBy:  "test-user",
	})
	require.NoError(t, err)

	newCred := "new_token_12345"
	_, err = svc.UpdateCredential(ctx, created.ID, UpdateCredentialRequest{
		Credential: &newCred,
	})
	require.NoError(t, err)

	// Verify credential was updated by fetching decrypted value
	decrypted, err := svc.GetDecryptedCredential(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, newCred, decrypted)
}

func TestCredentialService_Update_ValidationErrors(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	created, err := svc.CreateCredential(ctx, CreateCredentialRequest{
		Name:       "validation-test-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: ".*",
		Credential: "token123",
		CreatedBy:  "test-user",
	})
	require.NoError(t, err)

	tests := []struct {
		name      string
		update    UpdateCredentialRequest
		wantField string
	}{
		{
			name:      "empty_name",
			update:    UpdateCredentialRequest{Name: strPtr("")},
			wantField: "name",
		},
		{
			name:      "invalid_type",
			update:    UpdateCredentialRequest{Type: strPtr("invalid")},
			wantField: "type",
		},
		{
			name:      "empty_url_pattern",
			update:    UpdateCredentialRequest{URLPattern: strPtr("")},
			wantField: "url_pattern",
		},
		{
			name:      "invalid_url_pattern",
			update:    UpdateCredentialRequest{URLPattern: strPtr("[invalid")},
			wantField: "url_pattern",
		},
		{
			name:      "empty_credential",
			update:    UpdateCredentialRequest{Credential: strPtr("")},
			wantField: "credential",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.UpdateCredential(ctx, created.ID, tt.update)
			require.Error(t, err)

			var inputErr *services.InputError
			require.True(t, errors.As(err, &inputErr))
			assert.Equal(t, tt.wantField, inputErr.Field)
		})
	}
}

func TestCredentialService_Update_NotFound(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	newName := "new-name"
	_, err := svc.UpdateCredential(ctx, 99999, UpdateCredentialRequest{
		Name: &newName,
	})
	require.Error(t, err)
}

func TestCredentialService_Delete(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	// Create a credential
	created, err := svc.CreateCredential(ctx, CreateCredentialRequest{
		Name:       "delete-test-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: ".*",
		Credential: "token123",
		CreatedBy:  "test-user",
	})
	require.NoError(t, err)

	// Delete it
	err = svc.DeleteCredential(ctx, created.ID)
	require.NoError(t, err)

	// Verify deleted
	_, err = svc.GetCredential(ctx, created.ID)
	require.Error(t, err)
}

func TestCredentialService_Delete_NotFound(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	err := svc.DeleteCredential(ctx, 99999)
	require.Error(t, err)
}

func TestCredentialService_TestCredential_URLMatches(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	created, err := svc.CreateCredential(ctx, CreateCredentialRequest{
		Name:       "test-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: `github\.com/myorg/.*`,
		Credential: "token123",
		CreatedBy:  "test-user",
	})
	require.NoError(t, err)

	// Test matching URL
	result, err := svc.TestCredential(ctx, created.ID, "https://github.com/myorg/myrepo.git")
	require.NoError(t, err)
	assert.True(t, result.URLMatches)
	assert.True(t, result.Success)

	// Test non-matching URL
	result, err = svc.TestCredential(ctx, created.ID, "https://gitlab.com/other/repo.git")
	require.NoError(t, err)
	assert.False(t, result.URLMatches)
	assert.False(t, result.Success)
}

func TestCredentialService_MatchCredentialForURL(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	// Create GitHub credential
	_, err := svc.CreateCredential(ctx, CreateCredentialRequest{
		Name:       "github-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: `github\.com/.*`,
		Credential: "github_token",
		CreatedBy:  "test-user",
	})
	require.NoError(t, err)

	// Create GitLab credential
	_, err = svc.CreateCredential(ctx, CreateCredentialRequest{
		Name:       "gitlab-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: `gitlab\.com/.*`,
		Credential: "gitlab_token",
		CreatedBy:  "test-user",
	})
	require.NoError(t, err)

	// Match GitHub URL
	cred, err := svc.MatchCredentialForURL(ctx, "https://github.com/org/repo.git")
	require.NoError(t, err)
	assert.Equal(t, "github-cred", cred.Name)

	// Match GitLab URL
	cred, err = svc.MatchCredentialForURL(ctx, "https://gitlab.com/org/repo.git")
	require.NoError(t, err)
	assert.Equal(t, "gitlab-cred", cred.Name)

	// No match
	_, err = svc.MatchCredentialForURL(ctx, "https://bitbucket.org/org/repo.git")
	require.Error(t, err)
}

func TestCredentialService_GetDecryptedCredential(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	originalCred := "my_secret_token_12345"
	created, err := svc.CreateCredential(ctx, CreateCredentialRequest{
		Name:       "decrypt-test-cred",
		Type:       storage.SourceCredentialTypeHTTPSToken,
		URLPattern: ".*",
		Credential: originalCred,
		CreatedBy:  "test-user",
	})
	require.NoError(t, err)

	// Get decrypted credential
	decrypted, err := svc.GetDecryptedCredential(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, originalCred, decrypted)
}

func TestCredentialService_GetDecryptedCredential_NotFound(t *testing.T) {
	t.Parallel()

	tdb := testutil.NewTestDB(t)
	kms := testutil_mocks.NewMockKMS()
	svc := NewCredentialService(tdb.Store(), kms, zap.NewNop())

	ctx := context.Background()

	_, err := svc.GetDecryptedCredential(ctx, 99999)
	require.Error(t, err)
}

func TestCreateCredentialRequest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		req       CreateCredentialRequest
		wantErr   bool
		wantField string
	}{
		{
			name:    "valid_https_token",
			req:     CreateCredentialRequest{Name: "test", Type: "https_token", URLPattern: ".*", Credential: "token123"},
			wantErr: false,
		},
		{
			name:    "valid_https_basic",
			req:     CreateCredentialRequest{Name: "test", Type: "https_basic", URLPattern: ".*", Credential: "user:pass"},
			wantErr: false,
		},
		{
			name:    "valid_ssh_key",
			req:     CreateCredentialRequest{Name: "test", Type: "ssh_key", URLPattern: ".*", Credential: testEd25519Key},
			wantErr: false,
		},
		{
			name:      "empty_name",
			req:       CreateCredentialRequest{Name: "", Type: "https_token", URLPattern: ".*", Credential: "token"},
			wantErr:   true,
			wantField: "name",
		},
		{
			name:      "invalid_type",
			req:       CreateCredentialRequest{Name: "test", Type: "invalid", URLPattern: ".*", Credential: "token"},
			wantErr:   true,
			wantField: "type",
		},
		{
			name:      "empty_url_pattern",
			req:       CreateCredentialRequest{Name: "test", Type: "https_token", URLPattern: "", Credential: "token"},
			wantErr:   true,
			wantField: "url_pattern",
		},
		{
			name:      "invalid_regex",
			req:       CreateCredentialRequest{Name: "test", Type: "https_token", URLPattern: "[invalid", Credential: "token"},
			wantErr:   true,
			wantField: "url_pattern",
		},
		{
			name:      "empty_credential",
			req:       CreateCredentialRequest{Name: "test", Type: "https_token", URLPattern: ".*", Credential: ""},
			wantErr:   true,
			wantField: "credential",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.req.Validate()
			if tt.wantErr {
				require.Error(t, err)
				var inputErr *services.InputError
				require.True(t, errors.As(err, &inputErr))
				assert.Equal(t, tt.wantField, inputErr.Field)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// strPtr is a helper to get a pointer to a string.
func strPtr(s string) *string {
	return &s
}
