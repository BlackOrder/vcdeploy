package security

import (
	"errors"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{
			name:     "valid password",
			password: "MySecureP@ssw0rd!",
			wantErr:  nil,
		},
		{
			name:     "valid password with various special chars",
			password: "TestAdmin123!@#",
			wantErr:  nil,
		},
		{
			name:     "valid password minimum length",
			password: "Abcdefgh12!@",
			wantErr:  nil,
		},
		{
			name:     "too short",
			password: "Abc123!@#",
			wantErr:  ErrPasswordTooShort,
		},
		{
			name:     "too short - 11 chars",
			password: "Abcdefgh1!@",
			wantErr:  ErrPasswordTooShort,
		},
		{
			name:     "missing uppercase",
			password: "abcdefghij12!@",
			wantErr:  ErrPasswordNoUppercase,
		},
		{
			name:     "missing lowercase",
			password: "ABCDEFGHIJ12!@",
			wantErr:  ErrPasswordNoLowercase,
		},
		{
			name:     "missing digit",
			password: "Abcdefghijk!@#",
			wantErr:  ErrPasswordNoDigit,
		},
		{
			name:     "missing special char",
			password: "Abcdefghij123",
			wantErr:  ErrPasswordNoSpecialChar,
		},
		{
			name:     "empty password",
			password: "",
			wantErr:  ErrPasswordTooShort,
		},
		{
			name:     "only lowercase",
			password: "abcdefghijklmnop",
			wantErr:  ErrPasswordNoUppercase,
		},
		{
			name:     "only uppercase",
			password: "ABCDEFGHIJKLMNOP",
			wantErr:  ErrPasswordNoLowercase,
		},
		{
			name:     "only digits",
			password: "1234567890123456",
			wantErr:  ErrPasswordNoUppercase,
		},
		{
			name:     "only special chars",
			password: "!@#$%^&*()_+-=[]",
			wantErr:  ErrPasswordNoUppercase,
		},
		{
			name:     "common weak password",
			password: "password",
			wantErr:  ErrPasswordTooShort,
		},
		{
			name:     "admin123 weak password",
			password: "admin123",
			wantErr:  ErrPasswordTooShort,
		},
		{
			name:     "testpass weak password",
			password: "testpass",
			wantErr:  ErrPasswordTooShort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidatePassword(%q) = %v, want %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePasswordWithErrors(t *testing.T) {
	tests := []struct {
		name       string
		password   string
		wantErrs   int
		wantErrors []error
	}{
		{
			name:     "valid password",
			password: "MySecureP@ssw0rd!",
			wantErrs: 0,
		},
		{
			name:       "missing everything",
			password:   "",
			wantErrs:   5,
			wantErrors: []error{ErrPasswordTooShort, ErrPasswordNoUppercase, ErrPasswordNoLowercase, ErrPasswordNoDigit, ErrPasswordNoSpecialChar},
		},
		{
			name:       "only has length",
			password:   "aaaaaaaaaaaa",
			wantErrs:   3,
			wantErrors: []error{ErrPasswordNoUppercase, ErrPasswordNoDigit, ErrPasswordNoSpecialChar},
		},
		{
			name:       "missing multiple requirements",
			password:   "abcdefghij12",
			wantErrs:   2,
			wantErrors: []error{ErrPasswordNoUppercase, ErrPasswordNoSpecialChar},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidatePasswordWithErrors(tt.password)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidatePasswordWithErrors(%q) returned %d errors, want %d", tt.password, len(errs), tt.wantErrs)
			}

			if tt.wantErrors != nil {
				for _, wantErr := range tt.wantErrors {
					found := false
					for _, gotErr := range errs {
						if errors.Is(gotErr, wantErr) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("ValidatePasswordWithErrors(%q) missing expected error %v", tt.password, wantErr)
					}
				}
			}
		})
	}
}

func TestIsSpecialChar(t *testing.T) {
	specialChars := []rune{'!', '@', '#', '$', '%', '^', '&', '*', '(', ')', '_', '+', '-', '=', '[', ']', '{', '}', '|', ';', ':', '\'', '"', ',', '.', '/', '<', '>', '?', '`', '~', '\\'}
	for _, char := range specialChars {
		if !isSpecialChar(char) {
			t.Errorf("isSpecialChar(%q) = false, want true", char)
		}
	}

	nonSpecialChars := []rune{'a', 'A', '0', ' '}
	for _, char := range nonSpecialChars {
		if isSpecialChar(char) {
			t.Errorf("isSpecialChar(%q) = true, want false", char)
		}
	}
}

func TestWeakPasswordsFail(t *testing.T) {
	// These are common weak passwords that MUST fail validation
	weakPasswords := []string{
		"password",
		"password1",
		"admin123",
		"testpass",
		"123456",
		"qwerty",
		"letmein",
		"welcome",
		"monkey",
		"dragon",
		"master",
		"12345678",
		"abc123",
		"password123",
		"admin",
		"root",
		"test",
		"guest",
		"changeme",
		"pass1234",
	}

	for _, weakPassword := range weakPasswords {
		t.Run(weakPassword, func(t *testing.T) {
			err := ValidatePassword(weakPassword)
			if err == nil {
				t.Errorf("ValidatePassword(%q) = nil, expected validation to fail for weak password", weakPassword)
			}
		})
	}
}

func TestMinPasswordLength(t *testing.T) {
	if MinPasswordLength != 12 {
		t.Errorf("MinPasswordLength = %d, want 12", MinPasswordLength)
	}
}

func BenchmarkValidatePassword(b *testing.B) {
	password := "MySecureP@ssw0rd!"
	for i := 0; i < b.N; i++ {
		_ = ValidatePassword(password)
	}
}

func BenchmarkValidatePasswordWithErrors(b *testing.B) {
	password := "MySecureP@ssw0rd!"
	for i := 0; i < b.N; i++ {
		ValidatePasswordWithErrors(password)
	}
}
