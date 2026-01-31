package services

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	usernameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{2,31}$`)
	emailRegex    = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	projectRegex  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{1,63}$`)
)

// ValidateUsername validates a username.
// Username must be 3-32 characters, start with a letter, and contain only
// alphanumeric characters, underscores, and hyphens.
func ValidateUsername(username string) error {
	if username == "" {
		return InvalidInput("validation", "username is required")
	}
	if !usernameRegex.MatchString(username) {
		return InvalidInput("validation", "username must be 3-32 chars, start with letter, contain only alphanumeric, underscore, hyphen")
	}
	return nil
}

// ValidateEmail validates an email address.
// Empty email is allowed (optional field).
func ValidateEmail(email string) error {
	if email == "" {
		return nil
	}
	if !emailRegex.MatchString(email) {
		return InvalidInput("validation", "invalid email format")
	}
	return nil
}

// ValidatePassword validates a password.
// Password must be 8-128 characters.
func ValidatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < 8 {
		return InvalidInput("validation", "password must be at least 8 characters")
	}
	if length > 128 {
		return InvalidInput("validation", "password must be at most 128 characters")
	}
	return nil
}

// ValidateRole validates a user role.
// Valid roles are: admin, user, viewer.
func ValidateRole(role string) error {
	validRoles := map[string]bool{
		"admin":  true,
		"user":   true,
		"viewer": true,
	}
	if !validRoles[strings.ToLower(role)] {
		return InvalidInput("validation", "role must be admin, user, or viewer")
	}
	return nil
}

// ValidateProjectName validates a project name.
// Project name must be 2-64 characters, start with a letter.
func ValidateProjectName(name string) error {
	if name == "" {
		return InvalidInput("validation", "project name is required")
	}
	if !projectRegex.MatchString(name) {
		return InvalidInput("validation", "project name must be 2-64 chars, start with letter")
	}
	return nil
}

// ValidateRequired checks if a string is non-empty.
func ValidateRequired(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return InvalidInput("validation", field+" is required")
	}
	return nil
}

// ValidateMaxLength checks string length does not exceed maxLen.
func ValidateMaxLength(field, value string, maxLen int) error {
	if utf8.RuneCountInString(value) > maxLen {
		return InvalidInput("validation", field+" exceeds maximum length")
	}
	return nil
}

// ValidateMinLength checks string length meets minimum.
func ValidateMinLength(field, value string, minLen int) error {
	if utf8.RuneCountInString(value) < minLen {
		return InvalidInput("validation", field+" does not meet minimum length")
	}
	return nil
}

// ValidateOneOf checks if value is one of the allowed values.
func ValidateOneOf(field, value string, allowed []string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return InvalidInput("validation", field+" must be one of: "+strings.Join(allowed, ", "))
}

// ValidateID checks if an ID is positive.
func ValidateID(field string, id int64) error {
	if id <= 0 {
		return InvalidInput("validation", field+" must be a positive integer")
	}
	return nil
}

// ValidateStringID checks if a string ID is non-empty and reasonable length.
func ValidateStringID(field, id string) error {
	if strings.TrimSpace(id) == "" {
		return InvalidInput("validation", field+" is required")
	}
	if len(id) > 255 {
		return InvalidInput("validation", field+" exceeds maximum length")
	}
	return nil
}
