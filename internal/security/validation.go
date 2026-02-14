// Package security provides security services for vcdeploy.
package security

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	// agentIDRegex matches valid agent IDs:
	// - Must start with alphanumeric character
	// - Can contain alphanumeric, hyphens, underscores
	// - 3-64 characters total
	agentIDRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{2,63}$`)

	// hostnameRegex matches valid hostnames
	hostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

	// reservedAgentIDs are names that cannot be used as agent IDs
	reservedAgentIDs = []string{
		"master", "server", "admin", "root", "system",
		"localhost", "all", "any", "none", "default",
		"internal", "external", "public", "private",
	}

	// ErrInvalidAgentID is returned when an agent ID doesn't match the required format.
	ErrInvalidAgentID = errors.New("invalid agent ID: must be 3-64 alphanumeric characters, hyphens, or underscores, starting with alphanumeric")

	// ErrReservedAgentID is returned when an agent ID is a reserved name.
	ErrReservedAgentID = errors.New("agent ID is a reserved name")

	// ErrInvalidHostname is returned when a hostname doesn't match the required format.
	ErrInvalidHostname = errors.New("invalid hostname format")

	// ErrEmptyValue is returned when a required value is empty.
	ErrEmptyValue = errors.New("value cannot be empty")

	// ErrValueTooLong is returned when a value exceeds the maximum length.
	ErrValueTooLong = errors.New("value exceeds maximum length")
)

// ValidateAgentID validates an agent ID.
// Valid agent IDs:
// - Start with an alphanumeric character
// - Contain only alphanumeric characters, hyphens, and underscores
// - Are 3-64 characters long
// - Are not reserved names
func ValidateAgentID(id string) error {
	if id == "" {
		return ErrEmptyValue
	}

	if !agentIDRegex.MatchString(id) {
		return ErrInvalidAgentID
	}

	// Check reserved names (case-insensitive)
	lowerID := strings.ToLower(id)
	for _, reserved := range reservedAgentIDs {
		if lowerID == reserved {
			return fmt.Errorf("%w: '%s'", ErrReservedAgentID, id)
		}
	}

	return nil
}

// ValidateHostname validates a hostname.
func ValidateHostname(hostname string) error {
	if hostname == "" {
		return ErrEmptyValue
	}

	if len(hostname) > 253 {
		return ErrValueTooLong
	}

	if !hostnameRegex.MatchString(hostname) {
		return ErrInvalidHostname
	}

	return nil
}

// ValidateRegistrationToken validates a registration token format.
func ValidateRegistrationToken(token string) error {
	if token == "" {
		return ErrEmptyValue
	}

	// Tokens should be at least 32 characters for security
	if len(token) < 32 {
		return errors.New("token too short: must be at least 32 characters")
	}

	if len(token) > 256 {
		return ErrValueTooLong
	}

	return nil
}

// ValidateLabel validates a label key or value.
func ValidateLabel(key, value string) error {
	if key == "" {
		return fmt.Errorf("label key: %w", ErrEmptyValue)
	}

	if len(key) > 63 {
		return fmt.Errorf("label key: %w", ErrValueTooLong)
	}

	if len(value) > 255 {
		return fmt.Errorf("label value: %w", ErrValueTooLong)
	}

	// Key must start with alphanumeric and contain only alphanumeric, hyphens, underscores, dots
	keyRegex := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	if !keyRegex.MatchString(key) {
		return errors.New("invalid label key: must start with alphanumeric and contain only alphanumeric, hyphens, underscores, or dots")
	}

	return nil
}

// ValidateCSR performs basic validation on a Certificate Signing Request.
func ValidateCSR(csr []byte) error {
	if len(csr) == 0 {
		return errors.New("CSR cannot be empty")
	}

	// Check for PEM header
	if !strings.Contains(string(csr), "-----BEGIN CERTIFICATE REQUEST-----") {
		return errors.New("invalid CSR: must be PEM encoded")
	}

	// Size limit (16KB should be plenty for a CSR)
	if len(csr) > 16384 {
		return errors.New("CSR too large: maximum 16KB")
	}

	return nil
}

// SanitizeAgentID returns a sanitized version of an agent ID.
// It converts to lowercase and replaces invalid characters with hyphens.
// This is useful for generating agent IDs from hostnames.
func SanitizeAgentID(id string) string {
	// Convert to lowercase
	id = strings.ToLower(id)

	// Replace invalid characters with hyphens
	result := make([]byte, 0, len(id))
	lastWasHyphen := false

	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			result = append(result, c)
			lastWasHyphen = false
		} else if c == '-' || c == '_' || c == '.' || c == ' ' {
			// Replace dots, spaces with hyphens, but avoid consecutive hyphens
			if !lastWasHyphen && len(result) > 0 {
				result = append(result, '-')
				lastWasHyphen = true
			}
		}
	}

	// Remove trailing hyphens
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}

	// Ensure minimum length
	if len(result) < 3 {
		// Pad with random-ish suffix
		result = append(result, []byte("-agent")...)
	}

	// Truncate to maximum length
	if len(result) > 64 {
		result = result[:64]
	}

	// Remove trailing hyphens after truncation
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}

	return string(result)
}

// IsReservedAgentID checks if an agent ID is reserved.
func IsReservedAgentID(id string) bool {
	lowerID := strings.ToLower(id)
	for _, reserved := range reservedAgentIDs {
		if lowerID == reserved {
			return true
		}
	}
	return false
}
