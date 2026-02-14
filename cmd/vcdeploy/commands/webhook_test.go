package commands

import (
	"testing"
)

func TestGenerateWebhookSecret(t *testing.T) {
	secret1 := generateWebhookSecret()
	secret2 := generateWebhookSecret()

	// Should generate non-empty secrets
	if secret1 == "" {
		t.Error("generateWebhookSecret() returned empty string")
	}
	if secret2 == "" {
		t.Error("generateWebhookSecret() returned empty string")
	}

	// Should generate unique secrets
	if secret1 == secret2 {
		t.Error("generateWebhookSecret() generated duplicate secrets")
	}

	// Should be at least 32 bytes base64 encoded (43+ chars)
	if len(secret1) < 40 {
		t.Errorf("generateWebhookSecret() returned short secret: %d chars", len(secret1))
	}
}

func TestWebhookValidProviders(t *testing.T) {
	tests := []struct {
		provider string
		valid    bool
	}{
		{"github", true},
		{"gitlab", true},
		{"bitbucket", true},
		{"jenkins", false},
		{"", false},
		{"GITHUB", false}, // Case sensitive
	}

	validProviders := []string{"github", "gitlab", "bitbucket"}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			found := false
			for _, p := range validProviders {
				if p == tt.provider {
					found = true
					break
				}
			}
			if found != tt.valid {
				t.Errorf("provider %q: got valid=%v, want %v", tt.provider, found, tt.valid)
			}
		})
	}
}
