package services

import "testing"

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{"valid", "john_doe", false},
		{"valid with numbers", "user123", false},
		{"valid with hyphen", "user-name", false},
		{"valid minimum length", "abc", false},
		{"valid maximum length", "abcdefghijklmnopqrstuvwxyz123456", false},
		{"empty", "", true},
		{"too short", "ab", true},
		{"starts with number", "1user", true},
		{"starts with underscore", "_user", true},
		{"has spaces", "user name", true},
		{"too long", "abcdefghijklmnopqrstuvwxyz1234567", true},
		{"special chars", "user@name", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUsername(%q) error = %v, wantErr %v", tt.username, err, tt.wantErr)
			}
			if err != nil && !IsInvalidInput(err) {
				t.Errorf("ValidateUsername(%q) should return InvalidInput error", tt.username)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{"valid", "user@example.com", false},
		{"valid with plus", "user+tag@example.com", false},
		{"valid with subdomain", "user@mail.example.com", false},
		{"empty allowed", "", false},
		{"no at", "userexample.com", true},
		{"no domain", "user@", true},
		{"no tld", "user@example", true},
		{"double at", "user@@example.com", true},
		{"spaces", "user @example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmail(%q) error = %v, wantErr %v", tt.email, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid", "password123", false},
		{"minimum length", "12345678", false},
		{"with special chars", "P@ssw0rd!!", false},
		{"unicode", "密码密码密码密码", false}, // 8 unicode chars
		{"too short", "1234567", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

func TestValidateRole(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		wantErr bool
	}{
		{"admin", "admin", false},
		{"user", "user", false},
		{"viewer", "viewer", false},
		{"admin uppercase", "ADMIN", false},
		{"admin mixed", "Admin", false},
		{"viewer uppercase", "VIEWER", false},
		{"invalid", "superuser", true},
		{"invalid readonly", "readonly", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRole(tt.role)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRole(%q) error = %v, wantErr %v", tt.role, err, tt.wantErr)
			}
		})
	}
}

func TestValidateProjectName(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		wantErr     bool
	}{
		{"valid", "myproject", false},
		{"valid with numbers", "project123", false},
		{"valid with hyphen", "my-project", false},
		{"valid with underscore", "my_project", false},
		{"valid minimum", "ab", false},
		{"empty", "", true},
		{"too short", "a", true},
		{"starts with number", "1project", true},
		{"too long", "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklm", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProjectName(tt.projectName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProjectName(%q) error = %v, wantErr %v", tt.projectName, err, tt.wantErr)
			}
		})
	}
}

func TestValidateRequired(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		wantErr bool
	}{
		{"non-empty", "name", "value", false},
		{"empty", "name", "", true},
		{"whitespace only", "name", "   ", true},
		{"tabs only", "name", "\t\t", true},
		{"with whitespace", "name", "  value  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequired(tt.field, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRequired(%q, %q) error = %v, wantErr %v", tt.field, tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestValidateMaxLength(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		max     int
		wantErr bool
	}{
		{"under max", "field", "abc", 5, false},
		{"at max", "field", "abcde", 5, false},
		{"over max", "field", "abcdef", 5, true},
		{"empty", "field", "", 5, false},
		{"unicode", "field", "日本語", 3, false},
		{"unicode over", "field", "日本語文", 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMaxLength(tt.field, tt.value, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMaxLength(%q, %q, %d) error = %v, wantErr %v", tt.field, tt.value, tt.max, err, tt.wantErr)
			}
		})
	}
}

func TestValidateMinLength(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		min     int
		wantErr bool
	}{
		{"over min", "field", "abcde", 3, false},
		{"at min", "field", "abc", 3, false},
		{"under min", "field", "ab", 3, true},
		{"empty", "field", "", 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMinLength(tt.field, tt.value, tt.min)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMinLength(%q, %q, %d) error = %v, wantErr %v", tt.field, tt.value, tt.min, err, tt.wantErr)
			}
		})
	}
}

func TestValidateOneOf(t *testing.T) {
	allowed := []string{"pending", "running", "completed", "failed"}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid first", "pending", false},
		{"valid last", "failed", false},
		{"valid middle", "running", false},
		{"invalid", "unknown", true},
		{"empty", "", true},
		{"case sensitive", "PENDING", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOneOf("status", tt.value, allowed)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOneOf(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestValidateID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid", "abc123", false},
		{"valid-long", "abc123xyz789", false},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateID("user_id", tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestValidateStringID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid uuid-like", "abc-123-def", false},
		{"valid short", "id1", false},
		{"empty", "", true},
		{"whitespace only", "   ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStringID("deployment_id", tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStringID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestValidateAPIKeyScopes(t *testing.T) {
	tests := []struct {
		name    string
		scopes  []string
		wantErr bool
	}{
		{"empty scopes (valid, defaults handled by caller)", []string{}, false},
		{"wildcard", []string{"*"}, false},
		{"admin scope", []string{"admin"}, false},
		{"single read scope", []string{"read:projects"}, false},
		{"single write scope", []string{"write:deployments"}, false},
		{"multiple valid scopes", []string{"read:projects", "write:projects", "read:agents"}, false},
		{"all valid scopes", []string{"read:projects", "write:projects", "read:deployments", "write:deployments", "read:agents", "write:agents", "read:users", "write:users", "read:settings", "write:settings"}, false},
		{"invalid scope", []string{"invalid:scope"}, true},
		{"mix valid and invalid", []string{"read:projects", "invalid:scope"}, true},
		{"typo in scope", []string{"read:project"}, true},
		{"wrong format", []string{"projects"}, true},
		{"empty string scope", []string{""}, true},
		{"read:secrets valid", []string{"read:secrets"}, false},
		{"write:apikeys valid", []string{"write:apikeys"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAPIKeyScopes(tt.scopes)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAPIKeyScopes(%v) error = %v, wantErr %v", tt.scopes, err, tt.wantErr)
			}
			if err != nil && !IsInvalidInput(err) {
				t.Errorf("ValidateAPIKeyScopes(%v) should return InvalidInput error", tt.scopes)
			}
		})
	}
}
