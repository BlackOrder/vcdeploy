// Package security provides recovery code generation for TOTP 2FA.
package security

import (
	"crypto/rand"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// RecoveryCodeCount is the number of recovery codes generated.
const RecoveryCodeCount = 8

// RecoveryCodeLength is the length of each recovery code.
const RecoveryCodeLength = 8

// Characters used for recovery codes (no ambiguous chars: 0/O, 1/I/L)
const recoveryCodeChars = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// GenerateRecoveryCodes creates a set of recovery codes.
// Returns both the plaintext codes (to show user once) and hashed codes (to store).
func GenerateRecoveryCodes() (plaintext []string, hashes []string, err error) {
	plaintext = make([]string, RecoveryCodeCount)
	hashes = make([]string, RecoveryCodeCount)

	for i := 0; i < RecoveryCodeCount; i++ {
		code, err := generateCode(RecoveryCodeLength)
		if err != nil {
			return nil, nil, err
		}
		plaintext[i] = code

		hash, err := bcrypt.GenerateFromPassword([]byte(code), BcryptCost)
		if err != nil {
			return nil, nil, err
		}
		hashes[i] = string(hash)
	}

	return plaintext, hashes, nil
}

// generateCode creates a random alphanumeric code of the specified length.
func generateCode(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.Grow(length)
	for _, v := range b {
		sb.WriteByte(recoveryCodeChars[int(v)%len(recoveryCodeChars)])
	}
	return sb.String(), nil
}

// VerifyRecoveryCode checks if a code matches any of the provided hashes.
// Returns the index of the matching code (0-based) or -1 if no match.
func VerifyRecoveryCode(code string, hashes []string) int {
	code = strings.ToUpper(strings.TrimSpace(code))

	for i, hash := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(code)) == nil {
			return i
		}
	}
	return -1
}

// FormatRecoveryCodes formats codes for display with dashes for readability.
// e.g., "ABCD1234" -> "ABCD-1234"
func FormatRecoveryCodes(codes []string) []string {
	formatted := make([]string, len(codes))
	for i, code := range codes {
		if len(code) == RecoveryCodeLength {
			formatted[i] = code[:4] + "-" + code[4:]
		} else {
			formatted[i] = code
		}
	}
	return formatted
}

// NormalizeRecoveryCode removes dashes and normalizes case for verification.
func NormalizeRecoveryCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, " ", "")
	return code
}
