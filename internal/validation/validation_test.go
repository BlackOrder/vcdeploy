package validation

import (
	"strings"
	"testing"
)

func TestValidator_Required(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantError bool
	}{
		{"non-empty", "hello", false},
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"with spaces", " hello ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.Required("field", tt.value)
			if v.HasErrors() != tt.wantError {
				t.Errorf("Required(%q) hasErrors = %v, want %v", tt.value, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_MinLength(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		min       int
		wantError bool
	}{
		{"exactly min", "abc", 3, false},
		{"above min", "abcd", 3, false},
		{"below min", "ab", 3, true},
		{"empty", "", 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.MinLength("field", tt.value, tt.min)
			if v.HasErrors() != tt.wantError {
				t.Errorf("MinLength(%q, %d) hasErrors = %v, want %v", tt.value, tt.min, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_MaxLength(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		max       int
		wantError bool
	}{
		{"exactly max", "abc", 3, false},
		{"below max", "ab", 3, false},
		{"above max", "abcd", 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.MaxLength("field", tt.value, tt.max)
			if v.HasErrors() != tt.wantError {
				t.Errorf("MaxLength(%q, %d) hasErrors = %v, want %v", tt.value, tt.max, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_Email(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantError bool
	}{
		{"valid email", "test@example.com", false},
		{"valid with subdomain", "test@sub.example.com", false},
		{"valid with plus", "test+tag@example.com", false},
		{"empty (skipped)", "", false},
		{"no at sign", "testexample.com", true},
		{"no domain", "test@", true},
		{"no user", "@example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.Email("email", tt.value)
			if v.HasErrors() != tt.wantError {
				t.Errorf("Email(%q) hasErrors = %v, want %v", tt.value, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_URL(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantError bool
	}{
		{"valid https", "https://example.com", false},
		{"valid http", "http://example.com", false},
		{"valid with path", "https://example.com/path", false},
		{"empty (skipped)", "", false},
		{"no scheme", "example.com", true},
		{"no host", "https://", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.URL("url", tt.value)
			if v.HasErrors() != tt.wantError {
				t.Errorf("URL(%q) hasErrors = %v, want %v", tt.value, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_GitURL(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantError bool
	}{
		{"https url", "https://github.com/user/repo.git", false},
		{"git url", "git://github.com/user/repo.git", false},
		{"ssh url", "ssh://git@github.com/user/repo.git", false},
		{"git@ format", "git@github.com:user/repo.git", false},
		{"empty (skipped)", "", false},
		{"http url", "http://github.com/user/repo.git", true},
		{"plain text", "just some text", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.GitURL("repo", tt.value)
			if v.HasErrors() != tt.wantError {
				t.Errorf("GitURL(%q) hasErrors = %v, want %v", tt.value, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_NoPathTraversal(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantError bool
	}{
		{"simple path", "/var/www/app", false},
		{"relative path", "app/public", false},
		{"empty (skipped)", "", false},
		{"traversal ..", "../etc/passwd", true},
		{"traversal in middle", "/var/www/../etc", true},
		{"absolute outside allowed", "/etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.NoPathTraversal("path", tt.value)
			if v.HasErrors() != tt.wantError {
				t.Errorf("NoPathTraversal(%q) hasErrors = %v, want %v", tt.value, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_SafePath(t *testing.T) {
	allowedBases := []string{"/var/www", "/srv", "/home/deploy"}

	tests := []struct {
		name      string
		value     string
		wantError bool
	}{
		{"allowed base /var/www", "/var/www/myapp", false},
		{"allowed base /srv", "/srv/apps/myapp", false},
		{"allowed base /home", "/home/deploy/apps", false},
		{"empty (skipped)", "", false},
		{"not allowed base", "/etc/passwd", true},
		{"relative path", "var/www/app", true},
		{"traversal", "/var/www/../etc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.SafePath("path", tt.value, allowedBases)
			if v.HasErrors() != tt.wantError {
				t.Errorf("SafePath(%q) hasErrors = %v, want %v", tt.value, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_OneOf(t *testing.T) {
	allowed := []string{"staging", "production", "development"}

	tests := []struct {
		name      string
		value     string
		wantError bool
	}{
		{"valid staging", "staging", false},
		{"valid production", "production", false},
		{"empty (skipped)", "", false},
		{"invalid", "testing", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.OneOf("env", tt.value, allowed)
			if v.HasErrors() != tt.wantError {
				t.Errorf("OneOf(%q) hasErrors = %v, want %v", tt.value, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_Chaining(t *testing.T) {
	v := NewValidator()
	v.Required("name", "test").
		MinLength("name", "test", 2).
		MaxLength("name", "test", 10).
		Pattern("name", "test", ProjectNamePattern, "invalid format")

	if v.HasErrors() {
		t.Errorf("Chained validation should pass: %v", v.Errors())
	}
}

func TestValidator_MultipleErrors(t *testing.T) {
	v := NewValidator()
	v.Required("name", "")
	v.MinLength("email", "ab", 5)
	v.Email("email", "invalid")

	if !v.HasErrors() {
		t.Error("Expected validation errors")
	}
	if len(v.Errors().Errors) != 3 {
		t.Errorf("Expected 3 errors, got %d", len(v.Errors().Errors))
	}
}

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name      string
		username  string
		wantError bool
	}{
		{"valid lowercase", "johndoe", false},
		{"valid with numbers", "john123", false},
		{"valid with underscore", "john_doe", false},
		{"too short", "ab", true},
		{"too long", strings.Repeat("a", 33), true},
		{"starts with number", "1john", true},
		{"has hyphen", "john-doe", true},
		{"has space", "john doe", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateUsername(tt.username)
			if errs.HasErrors() != tt.wantError {
				t.Errorf("ValidateUsername(%q) hasErrors = %v, want %v: %v", tt.username, errs.HasErrors(), tt.wantError, errs)
			}
		})
	}
}

func TestValidateProjectName(t *testing.T) {
	tests := []struct {
		name      string
		project   string
		wantError bool
	}{
		{"valid lowercase", "myproject", false},
		{"valid with hyphen", "my-project", false},
		{"valid with underscore", "my_project", false},
		{"valid mixed case", "MyProject", false},
		{"too short", "a", true},
		{"too long", strings.Repeat("a", 65), true},
		{"starts with number", "1project", true},
		{"has space", "my project", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateProjectName(tt.project)
			if errs.HasErrors() != tt.wantError {
				t.Errorf("ValidateProjectName(%q) hasErrors = %v, want %v: %v", tt.project, errs.HasErrors(), tt.wantError, errs)
			}
		})
	}
}

func TestValidateSecretKey(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		wantError bool
	}{
		{"valid uppercase", "DATABASE_URL", false},
		{"valid single", "A", false},
		{"valid with numbers", "API_KEY_123", false},
		{"lowercase", "database_url", true},
		{"starts with number", "123_KEY", true},
		{"has hyphen", "MY-KEY", true},
		{"too long", strings.Repeat("A", 65), true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateSecretKey(tt.key)
			if errs.HasErrors() != tt.wantError {
				t.Errorf("ValidateSecretKey(%q) hasErrors = %v, want %v: %v", tt.key, errs.HasErrors(), tt.wantError, errs)
			}
		})
	}
}

func TestValidationErrors_Error(t *testing.T) {
	v := NewValidationErrors()
	if v.Error() != "validation failed" {
		t.Errorf("Empty errors should return generic message")
	}

	v.Add("name", "is required")
	if !strings.Contains(v.Error(), "name") || !strings.Contains(v.Error(), "required") {
		t.Errorf("Error message should contain field and message: %s", v.Error())
	}
}
