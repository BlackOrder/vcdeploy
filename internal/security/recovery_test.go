package security

import (
	"strings"
	"testing"
)

func TestGenerateRecoveryCodes(t *testing.T) {
	t.Parallel()

	plaintext, hashes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes() error = %v", err)
	}

	if len(plaintext) != RecoveryCodeCount {
		t.Errorf("got %d plaintext codes, want %d", len(plaintext), RecoveryCodeCount)
	}
	if len(hashes) != RecoveryCodeCount {
		t.Errorf("got %d hashes, want %d", len(hashes), RecoveryCodeCount)
	}

	// Verify each code has correct length
	for i, code := range plaintext {
		if len(code) != RecoveryCodeLength {
			t.Errorf("code[%d] length = %d, want %d", i, len(code), RecoveryCodeLength)
		}
	}

	// Verify codes contain only valid characters
	for i, code := range plaintext {
		for _, c := range code {
			if !strings.ContainsRune(recoveryCodeChars, c) {
				t.Errorf("code[%d] contains invalid character: %c", i, c)
			}
		}
	}
}

func TestGenerateRecoveryCodes_UniquePerCall(t *testing.T) {
	t.Parallel()

	codes1, _, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("first GenerateRecoveryCodes() error = %v", err)
	}

	codes2, _, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("second GenerateRecoveryCodes() error = %v", err)
	}

	// All codes should be unique between calls
	codeSet := make(map[string]bool)
	for _, code := range codes1 {
		codeSet[code] = true
	}
	for _, code := range codes2 {
		if codeSet[code] {
			t.Errorf("code %s appeared in both code sets (collision)", code)
		}
	}
}

func TestVerifyRecoveryCode_Valid(t *testing.T) {
	t.Parallel()

	plaintext, hashes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes() error = %v", err)
	}

	// Each plaintext code should verify against its hash
	for i, code := range plaintext {
		idx := VerifyRecoveryCode(code, hashes)
		if idx != i {
			t.Errorf("VerifyRecoveryCode(%q) = %d, want %d", code, idx, i)
		}
	}
}

func TestVerifyRecoveryCode_CaseInsensitive(t *testing.T) {
	t.Parallel()

	plaintext, hashes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes() error = %v", err)
	}

	// Lowercase should also work
	lowerCode := strings.ToLower(plaintext[0])
	idx := VerifyRecoveryCode(lowerCode, hashes)
	if idx != 0 {
		t.Errorf("VerifyRecoveryCode(lowercase) = %d, want 0", idx)
	}
}

func TestVerifyRecoveryCode_Invalid(t *testing.T) {
	t.Parallel()

	_, hashes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes() error = %v", err)
	}

	// Invalid code should return -1
	idx := VerifyRecoveryCode("INVALID1", hashes)
	if idx != -1 {
		t.Errorf("VerifyRecoveryCode(invalid) = %d, want -1", idx)
	}
}

func TestVerifyRecoveryCode_WithDashes(t *testing.T) {
	t.Parallel()

	plaintext, hashes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes() error = %v", err)
	}

	// Code with dashes should also work after normalization
	formatted := FormatRecoveryCodes(plaintext)
	normalized := NormalizeRecoveryCode(formatted[0])
	idx := VerifyRecoveryCode(normalized, hashes)
	if idx != 0 {
		t.Errorf("VerifyRecoveryCode(with dashes) = %d, want 0", idx)
	}
}

func TestFormatRecoveryCodes(t *testing.T) {
	t.Parallel()

	codes := []string{"ABCD1234", "EFGH5678"}
	formatted := FormatRecoveryCodes(codes)

	expected := []string{"ABCD-1234", "EFGH-5678"}
	for i, got := range formatted {
		if got != expected[i] {
			t.Errorf("formatted[%d] = %q, want %q", i, got, expected[i])
		}
	}
}

func TestNormalizeRecoveryCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"ABCD-1234", "ABCD1234"},
		{"abcd-1234", "ABCD1234"},
		{"  ABCD 1234  ", "ABCD1234"},
		{"abcd1234", "ABCD1234"},
	}

	for _, tt := range tests {
		got := NormalizeRecoveryCode(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeRecoveryCode(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestGenerateCode_Length(t *testing.T) {
	t.Parallel()

	for length := 4; length <= 16; length++ {
		code, err := generateCode(length)
		if err != nil {
			t.Errorf("generateCode(%d) error = %v", length, err)
			continue
		}
		if len(code) != length {
			t.Errorf("generateCode(%d) length = %d, want %d", length, len(code), length)
		}
	}
}
