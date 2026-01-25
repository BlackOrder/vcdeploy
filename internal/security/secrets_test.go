package security

import (
	"testing"
)

func TestValidateSecretKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid uppercase", "DATABASE_URL", false},
		{"valid single char", "A", false},
		{"valid with numbers", "API_KEY_123", false},
		{"empty", "", true},
		{"starts with number", "123_KEY", true},
		{"lowercase", "database_url", true},
		{"has spaces", "MY KEY", true},
		{"has hyphen", "MY-KEY", true},
		{"too long", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSecretKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestValidateProjectName(t *testing.T) {
	tests := []struct {
		name    string
		project string
		wantErr bool
	}{
		{"valid lowercase", "myproject", false},
		{"valid with hyphen", "my-project", false},
		{"valid with underscore", "my_project", false},
		{"valid with numbers", "project123", false},
		{"valid mixed", "My-Project_123", false},
		{"empty", "", true},
		{"starts with number", "123project", true},
		{"starts with hyphen", "-project", true},
		{"has spaces", "my project", true},
		{"too long", string(make([]byte, 65)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProjectName(tt.project)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProjectName(%q) error = %v, wantErr %v", tt.project, err, tt.wantErr)
			}
		})
	}
}
