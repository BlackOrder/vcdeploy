package validation

import (
	"io"
	"net/http"
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
		// Test path with literal .. in directory name (pathological case)
		{"traversal in cleaned path", "/var/www/..hidden", true},
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

func TestValidationErrors_WriteJSON(t *testing.T) {
	v := NewValidationErrors()
	v.Add("name", "is required")
	v.Add("email", "is invalid")

	// Create a mock response writer
	rr := &mockResponseWriter{
		headers: make(map[string][]string),
	}

	v.WriteJSON(rr)

	// Check status code
	if rr.statusCode != 400 {
		t.Errorf("WriteJSON() status = %d, want 400", rr.statusCode)
	}

	// Check content type
	contentType := rr.headers.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("WriteJSON() Content-Type = %q, want application/json", contentType)
	}

	// Check body contains expected content
	body := rr.body.String()
	if !strings.Contains(body, "name") || !strings.Contains(body, "is required") {
		t.Errorf("WriteJSON() body should contain errors: %s", body)
	}
}

func TestValidator_Alphanumeric(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		allowedExtra string
		wantError    bool
	}{
		{"alphanumeric only", "abc123", "", false},
		{"letters only", "abcdef", "", false},
		{"numbers only", "123456", "", false},
		{"empty (skipped)", "", "", false},
		{"with allowed hyphen", "abc-123", "-", false},
		{"with allowed underscore", "abc_123", "_", false},
		{"with allowed dot", "abc.123", ".", false},
		{"invalid char no extra", "abc-123", "", true},
		{"invalid special char", "abc@123", "", true},
		{"invalid space", "abc 123", "", true},
		{"unicode letters", "áéíóú", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.Alphanumeric("field", tt.value, tt.allowedExtra)
			if v.HasErrors() != tt.wantError {
				t.Errorf("Alphanumeric(%q, %q) hasErrors = %v, want %v", tt.value, tt.allowedExtra, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_StartsWithLetter(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantError bool
	}{
		{"starts with lowercase", "abc123", false},
		{"starts with uppercase", "Abc123", false},
		{"empty (skipped)", "", false},
		{"starts with number", "123abc", true},
		{"starts with underscore", "_abc", true},
		{"starts with hyphen", "-abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.StartsWithLetter("field", tt.value)
			if v.HasErrors() != tt.wantError {
				t.Errorf("StartsWithLetter(%q) hasErrors = %v, want %v", tt.value, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_NoShellMetachars(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantError bool
	}{
		{"plain text", "hello world", false},
		{"path like", "/var/www/app", false},
		{"empty (skipped)", "", false},
		{"semicolon", "cmd; rm -rf /", true},
		{"pipe", "cmd | cat", true},
		{"ampersand", "cmd && echo", true},
		{"dollar", "$VAR", true},
		{"backtick", "`cmd`", true},
		{"parentheses", "(cmd)", true},
		{"braces", "{cmd}", true},
		{"backslash", "cmd\\", true},
		{"less than", "cmd<file", true},
		{"greater than", "cmd>file", true},
		{"exclamation", "cmd!", true},
		{"asterisk", "cmd*", true},
		{"question mark", "cmd?", true},
		{"brackets", "[cmd]", true},
		{"single quote", "cmd'", true},
		{"double quote", `cmd"`, true},
		{"newline", "cmd\n", true},
		{"carriage return", "cmd\r", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.NoShellMetachars("field", tt.value)
			if v.HasErrors() != tt.wantError {
				t.Errorf("NoShellMetachars(%q) hasErrors = %v, want %v", tt.value, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_Positive(t *testing.T) {
	tests := []struct {
		name      string
		value     int
		wantError bool
	}{
		{"positive", 1, false},
		{"large positive", 1000000, false},
		{"zero", 0, true},
		{"negative", -1, true},
		{"large negative", -1000000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.Positive("field", tt.value)
			if v.HasErrors() != tt.wantError {
				t.Errorf("Positive(%d) hasErrors = %v, want %v", tt.value, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_Range(t *testing.T) {
	tests := []struct {
		name      string
		value     int
		min       int
		max       int
		wantError bool
	}{
		{"within range", 5, 1, 10, false},
		{"at min", 1, 1, 10, false},
		{"at max", 10, 1, 10, false},
		{"below min", 0, 1, 10, true},
		{"above max", 11, 1, 10, true},
		{"negative range valid", -5, -10, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.Range("field", tt.value, tt.min, tt.max)
			if v.HasErrors() != tt.wantError {
				t.Errorf("Range(%d, %d, %d) hasErrors = %v, want %v", tt.value, tt.min, tt.max, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidator_Custom(t *testing.T) {
	tests := []struct {
		name      string
		valid     bool
		wantError bool
	}{
		{"valid condition", true, false},
		{"invalid condition", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.Custom("field", tt.valid, "custom validation failed")
			if v.HasErrors() != tt.wantError {
				t.Errorf("Custom(valid=%v) hasErrors = %v, want %v", tt.valid, v.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestExtraCharsMessage(t *testing.T) {
	tests := []struct {
		name     string
		extra    string
		expected string
	}{
		{"empty", "", ""},
		{"hyphen", "-", ", hyphens"},
		{"underscore", "_", ", underscores"},
		{"dot", ".", ", dots"},
		{"slash", "/", ", slashes"},
		{"other char", "@", ", '@'"},
		{"multiple", "-_", ", hyphens, underscores"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extraCharsMessage(tt.extra)
			if result != tt.expected {
				t.Errorf("extraCharsMessage(%q) = %q, want %q", tt.extra, result, tt.expected)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		wantError bool
	}{
		{"valid email", "test@example.com", false},
		{"valid with plus", "test+tag@example.com", false},
		{"empty", "", true},
		{"invalid format", "not-an-email", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateEmail(tt.email)
			if errs.HasErrors() != tt.wantError {
				t.Errorf("ValidateEmail(%q) hasErrors = %v, want %v", tt.email, errs.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidateDeployPath(t *testing.T) {
	allowedBases := []string{"/var/www", "/srv", "/home/deploy"}

	tests := []struct {
		name      string
		path      string
		wantError bool
	}{
		{"valid path", "/var/www/myapp", false},
		{"valid srv", "/srv/apps", false},
		{"empty", "", true},
		{"path traversal", "/var/www/../etc", true},
		{"shell metachar", "/var/www/app;rm", true},
		{"not allowed base", "/etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateDeployPath(tt.path, allowedBases)
			if errs.HasErrors() != tt.wantError {
				t.Errorf("ValidateDeployPath(%q) hasErrors = %v, want %v", tt.path, errs.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidateGitRepository(t *testing.T) {
	tests := []struct {
		name      string
		repo      string
		wantError bool
	}{
		{"https repo", "https://github.com/user/repo.git", false},
		{"git repo", "git://github.com/user/repo.git", false},
		{"ssh repo", "ssh://git@github.com/user/repo.git", false},
		{"git@ format", "git@github.com:user/repo.git", false},
		{"empty", "", true},
		{"http repo", "http://github.com/user/repo.git", true},
		{"invalid", "not-a-repo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateGitRepository(tt.repo)
			if errs.HasErrors() != tt.wantError {
				t.Errorf("ValidateGitRepository(%q) hasErrors = %v, want %v", tt.repo, errs.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestValidateUnixUsername(t *testing.T) {
	tests := []struct {
		name      string
		username  string
		wantError bool
	}{
		{"valid lowercase", "deploy", false},
		{"valid with underscore", "_deploy", false},
		{"valid with numbers", "deploy123", false},
		{"valid with hyphen", "deploy-user", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 33), true},
		{"uppercase", "Deploy", true},
		{"starts with number", "1deploy", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateUnixUsername(tt.username)
			if errs.HasErrors() != tt.wantError {
				t.Errorf("ValidateUnixUsername(%q) hasErrors = %v, want %v", tt.username, errs.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestIsValidUnixUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     bool
	}{
		{"valid lowercase", "deploy", true},
		{"valid with underscore prefix", "_deploy", true},
		{"valid with numbers", "deploy123", true},
		{"empty", "", false},
		{"too long", strings.Repeat("a", 33), false},
		{"uppercase", "Deploy", false},
		{"starts with number", "1deploy", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidUnixUsername(tt.username); got != tt.want {
				t.Errorf("IsValidUnixUsername(%q) = %v, want %v", tt.username, got, tt.want)
			}
		})
	}
}

func TestValidateServiceName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		wantError   bool
	}{
		{"valid simple", "nginx", false},
		{"valid with hyphen", "my-service", false},
		{"valid with dot", "my.service", false},
		{"valid with at", "service@instance", false},
		{"valid with underscore", "my_service", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 257), true},
		{"starts with hyphen", "-service", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateServiceName(tt.serviceName)
			if errs.HasErrors() != tt.wantError {
				t.Errorf("ValidateServiceName(%q) hasErrors = %v, want %v", tt.serviceName, errs.HasErrors(), tt.wantError)
			}
		})
	}
}

func TestIsValidServiceName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		want        bool
	}{
		{"valid simple", "nginx", true},
		{"valid with hyphen", "my-service", true},
		{"valid with dot", "my.service", true},
		{"empty", "", false},
		{"too long", strings.Repeat("a", 257), false},
		{"starts with hyphen", "-service", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidServiceName(tt.serviceName); got != tt.want {
				t.Errorf("IsValidServiceName(%q) = %v, want %v", tt.serviceName, got, tt.want)
			}
		})
	}
}

func TestParseAndValidateJSON(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		contentLen    int64
		maxSize       int64
		wantError     bool
		errorContains string
	}{
		{
			name:       "valid JSON",
			body:       `{"name":"test"}`,
			contentLen: 15,
			maxSize:    1024,
			wantError:  false,
		},
		{
			name:          "content too large",
			body:          `{"name":"test"}`,
			contentLen:    2000,
			maxSize:       100,
			wantError:     true,
			errorContains: "too large",
		},
		{
			name:          "invalid JSON",
			body:          `{invalid}`,
			contentLen:    9,
			maxSize:       1024,
			wantError:     true,
			errorContains: "invalid JSON",
		},
		{
			name:          "unknown field",
			body:          `{"name":"test","unknown":"field"}`,
			contentLen:    33,
			maxSize:       1024,
			wantError:     true,
			errorContains: "invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &mockRequest{
				body:          tt.body,
				contentLength: tt.contentLen,
			}

			var result struct {
				Name string `json:"name"`
			}

			err := ParseAndValidateJSON(req.toHTTPRequest(), tt.maxSize, &result)

			if tt.wantError {
				if err == nil {
					t.Errorf("ParseAndValidateJSON() expected error containing %q, got nil", tt.errorContains)
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("ParseAndValidateJSON() error = %v, want containing %q", err, tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("ParseAndValidateJSON() unexpected error = %v", err)
				}
			}
		})
	}
}

// mockResponseWriter implements http.ResponseWriter for testing
type mockResponseWriter struct {
	headers    http.Header
	body       strings.Builder
	statusCode int
}

func (m *mockResponseWriter) Header() http.Header {
	return m.headers
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	return m.body.Write(b)
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

// mockRequest helps create test HTTP requests
type mockRequest struct {
	body          string
	contentLength int64
}

func (m *mockRequest) toHTTPRequest() *http.Request {
	return &http.Request{
		Body:          io.NopCloser(strings.NewReader(m.body)),
		ContentLength: m.contentLength,
	}
}

func TestValidateBinaryPathComponent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"valid version", "v1.0.0", false, ""},
		{"valid os", "linux", false, ""},
		{"valid arch", "amd64", false, ""},
		{"valid with hyphen", "v1.0.0-beta1", false, ""},
		{"empty", "", true, "empty value"},
		{"path traversal dotdot", "../../../etc/passwd", true, "path traversal not allowed"},
		{"path traversal forward slash", "foo/bar", true, "path separators not allowed"},
		{"path traversal backslash", "foo\\bar", true, "path separators not allowed"},
		{"null byte", "v1.0\x00.0", true, "null bytes not allowed"},
		{"control character", "v1.0\x1f.0", true, "control characters not allowed"},
		{"double dots only", "..", true, "path traversal not allowed"},
		{"mixed attack", "../linux/amd64", true, "path traversal not allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBinaryPathComponent(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateBinaryPathComponent(%q) = nil, want error containing %q", tt.input, tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateBinaryPathComponent(%q) = %q, want error containing %q", tt.input, err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateBinaryPathComponent(%q) = %v, want nil", tt.input, err)
				}
			}
		})
	}
}
