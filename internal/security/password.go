// Package security provides encryption and authentication utilities.
package security

import (
	"errors"
	"unicode"
)

// Password validation errors.
var (
	ErrPasswordTooShort      = errors.New("password must be at least 12 characters long")
	ErrPasswordNoUppercase   = errors.New("password must contain at least one uppercase letter")
	ErrPasswordNoLowercase   = errors.New("password must contain at least one lowercase letter")
	ErrPasswordNoDigit       = errors.New("password must contain at least one digit")
	ErrPasswordNoSpecialChar = errors.New("password must contain at least one special character")
)

// MinPasswordLength is the minimum required password length.
const MinPasswordLength = 12

// ValidatePassword validates password complexity requirements.
// Returns nil if the password meets all requirements, otherwise returns an error
// describing the first requirement that was not met.
//
// Requirements:
//   - Minimum 12 characters
//   - At least one uppercase letter
//   - At least one lowercase letter
//   - At least one digit
//   - At least one special character (!@#$%^&*()_+-=[]{}|;':\",./<>?)
func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}

	var hasUppercase, hasLowercase, hasDigit, hasSpecial bool

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUppercase = true
		case unicode.IsLower(char):
			hasLowercase = true
		case unicode.IsDigit(char):
			hasDigit = true
		case isSpecialChar(char):
			hasSpecial = true
		}
	}

	if !hasUppercase {
		return ErrPasswordNoUppercase
	}
	if !hasLowercase {
		return ErrPasswordNoLowercase
	}
	if !hasDigit {
		return ErrPasswordNoDigit
	}
	if !hasSpecial {
		return ErrPasswordNoSpecialChar
	}

	return nil
}

// isSpecialChar checks if a character is a special character.
func isSpecialChar(char rune) bool {
	specialChars := "!@#$%^&*()_+-=[]{}|;':\",./<>?`~\\"
	for _, sc := range specialChars {
		if char == sc {
			return true
		}
	}
	return false
}

// ValidatePasswordWithErrors validates password and returns all violations.
// This is useful for providing complete feedback to users.
func ValidatePasswordWithErrors(password string) []error {
	var errs []error

	if len(password) < MinPasswordLength {
		errs = append(errs, ErrPasswordTooShort)
	}

	var hasUppercase, hasLowercase, hasDigit, hasSpecial bool

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUppercase = true
		case unicode.IsLower(char):
			hasLowercase = true
		case unicode.IsDigit(char):
			hasDigit = true
		case isSpecialChar(char):
			hasSpecial = true
		}
	}

	if !hasUppercase {
		errs = append(errs, ErrPasswordNoUppercase)
	}
	if !hasLowercase {
		errs = append(errs, ErrPasswordNoLowercase)
	}
	if !hasDigit {
		errs = append(errs, ErrPasswordNoDigit)
	}
	if !hasSpecial {
		errs = append(errs, ErrPasswordNoSpecialChar)
	}

	return errs
}
