// Package validation provides input validation for vcdeploy.
package validation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// ValidationError represents a field-level validation error.
//
//nolint:revive // Keeping explicit naming for clarity
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors is a collection of validation errors.
//
//nolint:revive // Keeping explicit naming for clarity
type ValidationErrors struct {
	Errors []ValidationError `json:"errors"`
}

// Error implements the error interface.
func (v *ValidationErrors) Error() string {
	if len(v.Errors) == 0 {
		return "validation failed"
	}
	return fmt.Sprintf("validation failed: %s - %s", v.Errors[0].Field, v.Errors[0].Message)
}

// HasErrors returns true if there are validation errors.
func (v *ValidationErrors) HasErrors() bool {
	return len(v.Errors) > 0
}

// Add adds a validation error.
func (v *ValidationErrors) Add(field, message string) {
	v.Errors = append(v.Errors, ValidationError{Field: field, Message: message})
}

// WriteJSON writes the validation errors as JSON to the response.
func (v *ValidationErrors) WriteJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(v)
}

// NewValidationErrors creates a new ValidationErrors.
func NewValidationErrors() *ValidationErrors {
	return &ValidationErrors{Errors: make([]ValidationError, 0)}
}

// Validator provides validation methods.
type Validator struct {
	errors *ValidationErrors
}

// NewValidator creates a new Validator.
func NewValidator() *Validator {
	return &Validator{errors: NewValidationErrors()}
}

// Errors returns the validation errors.
func (v *Validator) Errors() *ValidationErrors {
	return v.errors
}

// HasErrors returns true if there are validation errors.
func (v *Validator) HasErrors() bool {
	return v.errors.HasErrors()
}

// Required validates that a field is not empty.
func (v *Validator) Required(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.errors.Add(field, "is required")
	}
	return v
}

// MinLength validates minimum string length.
func (v *Validator) MinLength(field, value string, minLen int) *Validator {
	if len(value) < minLen {
		v.errors.Add(field, fmt.Sprintf("must be at least %d characters", minLen))
	}
	return v
}

// MaxLength validates maximum string length.
func (v *Validator) MaxLength(field, value string, maxLen int) *Validator {
	if len(value) > maxLen {
		v.errors.Add(field, fmt.Sprintf("must be at most %d characters", maxLen))
	}
	return v
}

// Pattern validates against a regex pattern.
func (v *Validator) Pattern(field, value string, pattern *regexp.Regexp, message string) *Validator {
	if value != "" && !pattern.MatchString(value) {
		v.errors.Add(field, message)
	}
	return v
}

// Email validates email format.
func (v *Validator) Email(field, value string) *Validator {
	if value == "" {
		return v
	}
	if _, err := mail.ParseAddress(value); err != nil {
		v.errors.Add(field, "must be a valid email address")
	}
	return v
}

// URL validates URL format.
func (v *Validator) URL(field, value string) *Validator {
	if value == "" {
		return v
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		v.errors.Add(field, "must be a valid URL")
	}
	return v
}

// GitURL validates a git repository URL.
func (v *Validator) GitURL(field, value string) *Validator {
	if value == "" {
		return v
	}
	// Accept: https://, git://, ssh://, git@host:path
	if strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, "git://") ||
		strings.HasPrefix(value, "ssh://") ||
		strings.Contains(value, "@") && strings.Contains(value, ":") {
		return v
	}
	v.errors.Add(field, "must be a valid git URL (https://, git://, ssh://, or git@host:path)")
	return v
}

// NoPathTraversal validates that a path doesn't contain traversal attacks.
func (v *Validator) NoPathTraversal(field, value string) *Validator {
	if value == "" {
		return v
	}
	// Check for traversal patterns in the original value
	if strings.Contains(value, "..") {
		v.errors.Add(field, "must not contain path traversal sequences")
		return v
	}
	// Clean the path and ensure it's still sane
	cleaned := filepath.Clean(value)
	// Check for absolute paths that might escape
	if filepath.IsAbs(value) && !strings.HasPrefix(cleaned, "/var/") && !strings.HasPrefix(cleaned, "/home/") && !strings.HasPrefix(cleaned, "/srv/") {
		v.errors.Add(field, "absolute path must be within allowed directories")
	}
	return v
}

// SafePath validates a deployment path is safe.
func (v *Validator) SafePath(field, value string, allowedBases []string) *Validator {
	if value == "" {
		return v
	}
	cleaned := filepath.Clean(value)
	if strings.Contains(cleaned, "..") {
		v.errors.Add(field, "must not contain path traversal sequences")
		return v
	}
	if !filepath.IsAbs(cleaned) {
		v.errors.Add(field, "must be an absolute path")
		return v
	}
	// Check if path is under one of the allowed bases
	allowed := false
	for _, base := range allowedBases {
		if strings.HasPrefix(cleaned, base) {
			allowed = true
			break
		}
	}
	if !allowed {
		v.errors.Add(field, fmt.Sprintf("must be under one of: %v", allowedBases))
	}
	return v
}

// Alphanumeric validates alphanumeric with optional extra chars.
func (v *Validator) Alphanumeric(field, value string, allowedExtra string) *Validator {
	if value == "" {
		return v
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(allowedExtra, r) {
			v.errors.Add(field, fmt.Sprintf("must contain only letters, numbers%s", extraCharsMessage(allowedExtra)))
			return v
		}
	}
	return v
}

// StartsWithLetter validates that value starts with a letter.
func (v *Validator) StartsWithLetter(field, value string) *Validator {
	if value == "" {
		return v
	}
	if !unicode.IsLetter(rune(value[0])) {
		v.errors.Add(field, "must start with a letter")
	}
	return v
}

// OneOf validates that value is one of the allowed values.
func (v *Validator) OneOf(field, value string, allowed []string) *Validator {
	if value == "" {
		return v
	}
	for _, a := range allowed {
		if value == a {
			return v
		}
	}
	v.errors.Add(field, fmt.Sprintf("must be one of: %v", allowed))
	return v
}

// shellMetachars contains characters that have special meaning in shell commands.
// These can be used for command injection if not properly escaped.
const shellMetachars = ";|&$`(){}\\<>!*?[]'\"\n\r"

// NoShellMetachars validates that a value doesn't contain shell metacharacters.
// This prevents command injection when values are interpolated into shell commands.
func (v *Validator) NoShellMetachars(field, value string) *Validator {
	if value == "" {
		return v
	}
	if strings.ContainsAny(value, shellMetachars) {
		v.errors.Add(field, "must not contain shell metacharacters")
	}
	return v
}

// Positive validates that an integer is positive.
func (v *Validator) Positive(field string, value int) *Validator {
	if value <= 0 {
		v.errors.Add(field, "must be a positive number")
	}
	return v
}

// Range validates that an integer is within a range.
func (v *Validator) Range(field string, value, minVal, maxVal int) *Validator {
	if value < minVal || value > maxVal {
		v.errors.Add(field, fmt.Sprintf("must be between %d and %d", minVal, maxVal))
	}
	return v
}

// Custom allows custom validation logic.
func (v *Validator) Custom(field string, valid bool, message string) *Validator {
	if !valid {
		v.errors.Add(field, message)
	}
	return v
}

func extraCharsMessage(extra string) string {
	if extra == "" {
		return ""
	}
	chars := make([]string, 0, len(extra))
	for _, r := range extra {
		switch r {
		case '-':
			chars = append(chars, "hyphens")
		case '_':
			chars = append(chars, "underscores")
		case '.':
			chars = append(chars, "dots")
		case '/':
			chars = append(chars, "slashes")
		default:
			chars = append(chars, fmt.Sprintf("'%c'", r))
		}
	}
	return ", " + strings.Join(chars, ", ")
}

// Common validation patterns
var (
	UsernamePattern     = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{2,31}$`)
	ProjectNamePattern  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{1,63}$`)
	SecretKeyPattern    = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	BranchPattern       = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9/_.-]*$`)
	LabelKeyPattern     = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
	UnixUsernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)
	ServiceNamePattern  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._@-]*$`)
)

// ValidateUsername validates a username.
func ValidateUsername(username string) *ValidationErrors {
	v := NewValidator()
	v.Required("username", username).
		MinLength("username", username, 3).
		MaxLength("username", username, 32).
		Pattern("username", username, UsernamePattern, "must start with a letter and contain only letters, numbers, underscores")
	return v.Errors()
}

// ValidateProjectName validates a project name.
func ValidateProjectName(name string) *ValidationErrors {
	v := NewValidator()
	v.Required("name", name).
		MinLength("name", name, 2).
		MaxLength("name", name, 64).
		Pattern("name", name, ProjectNamePattern, "must start with a letter and contain only letters, numbers, hyphens, underscores")
	return v.Errors()
}

// ValidateSecretKey validates a secret key.
func ValidateSecretKey(key string) *ValidationErrors {
	v := NewValidator()
	v.Required("key", key).
		MaxLength("key", key, 64).
		Pattern("key", key, SecretKeyPattern, "must start with uppercase letter and contain only A-Z, 0-9, underscore")
	return v.Errors()
}

// ValidateEmail validates an email address.
func ValidateEmail(email string) *ValidationErrors {
	v := NewValidator()
	v.Required("email", email).Email("email", email)
	return v.Errors()
}

// ValidateDeployPath validates a deployment path.
// Ensures path is safe (no traversal) and contains no shell metacharacters.
func ValidateDeployPath(path string, allowedBases []string) *ValidationErrors {
	v := NewValidator()
	v.Required("path", path).
		SafePath("path", path, allowedBases).
		NoShellMetachars("path", path)
	return v.Errors()
}

// ValidateGitRepository validates a git repository URL.
func ValidateGitRepository(repo string) *ValidationErrors {
	v := NewValidator()
	v.Required("repository", repo).GitURL("repository", repo)
	return v.Errors()
}

// ValidateUnixUsername validates a Unix/Linux username for safe use in sudo/shell commands.
func ValidateUnixUsername(username string) *ValidationErrors {
	v := NewValidator()
	v.Required("username", username).
		MaxLength("username", username, 32).
		Pattern("username", username, UnixUsernamePattern, "must start with a letter or underscore and contain only lowercase letters, numbers, underscores, hyphens")
	return v.Errors()
}

// IsValidUnixUsername returns true if the username is a valid Unix username.
// This is a convenience function for quick validation without error details.
func IsValidUnixUsername(username string) bool {
	if username == "" || len(username) > 32 {
		return false
	}
	return UnixUsernamePattern.MatchString(username)
}

// ValidateServiceName validates a systemd service name for safe use in systemctl commands.
func ValidateServiceName(name string) *ValidationErrors {
	v := NewValidator()
	v.Required("service", name).
		MaxLength("service", name, 256).
		Pattern("service", name, ServiceNamePattern, "must start with letter or number and contain only letters, numbers, dots, underscores, @, hyphens")
	return v.Errors()
}

// IsValidServiceName returns true if the name is a valid systemd service name.
// This is a convenience function for quick validation without error details.
func IsValidServiceName(name string) bool {
	if name == "" || len(name) > 256 {
		return false
	}
	return ServiceNamePattern.MatchString(name)
}

// ParseAndValidateJSON parses JSON from request body with size limit.
func ParseAndValidateJSON(r *http.Request, maxSize int64, v interface{}) error {
	if r.ContentLength > maxSize {
		return fmt.Errorf("request body too large (max %d bytes)", maxSize)
	}
	r.Body = http.MaxBytesReader(nil, r.Body, maxSize)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

// DefaultMaxBodySize is the default maximum request body size (1MB).
const DefaultMaxBodySize = 1 << 20

// ValidateBinaryPathComponent validates a path component to prevent path traversal attacks.
// It ensures the value doesn't contain directory traversal sequences or path separators.
func ValidateBinaryPathComponent(s string) error {
	if s == "" {
		return fmt.Errorf("empty value")
	}
	if strings.Contains(s, "..") {
		return fmt.Errorf("path traversal not allowed")
	}
	if strings.Contains(s, "/") || strings.Contains(s, "\\") {
		return fmt.Errorf("path separators not allowed")
	}
	if strings.Contains(s, "\x00") {
		return fmt.Errorf("null bytes not allowed")
	}
	// Also validate no control characters
	for _, r := range s {
		if r < 32 || r == 127 {
			return fmt.Errorf("control characters not allowed")
		}
	}
	return nil
}
