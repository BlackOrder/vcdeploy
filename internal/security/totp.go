// Package security provides TOTP two-factor authentication.
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"time"
)

// TOTPConfig holds TOTP configuration.
type TOTPConfig struct {
	Issuer string
	Digits int
	Period int
}

// DefaultTOTPConfig returns default TOTP configuration.
func DefaultTOTPConfig() TOTPConfig {
	return TOTPConfig{
		Issuer: "vcdeploy",
		Digits: 6,
		Period: 30,
	}
}

// GenerateTOTPSecret generates a new TOTP secret.
func GenerateTOTPSecret() (string, error) {
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generating secret: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

// ValidateTOTP validates a TOTP code against a secret.
func ValidateTOTP(secret, code string, config TOTPConfig) bool {
	// Check current time and ±1 period for clock skew
	now := time.Now().Unix()
	for _, offset := range []int64{-int64(config.Period), 0, int64(config.Period)} {
		expected := generateTOTP(secret, now+offset, config)
		if expected == code {
			return true
		}
	}
	return false
}

// GenerateTOTPURI generates a URI for QR code generation.
func GenerateTOTPURI(secret, username string, config TOTPConfig) string {
	return fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s&digits=%d&period=%d",
		config.Issuer, username, secret, config.Issuer, config.Digits, config.Period,
	)
}

func generateTOTP(secret string, timestamp int64, config TOTPConfig) string {
	// Decode secret
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return ""
	}

	// Calculate counter
	counter := uint64(timestamp / int64(config.Period))

	// Convert counter to bytes (big-endian)
	counterBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(counterBytes, counter)

	// HMAC-SHA1
	mac := hmac.New(sha1.New, key)
	mac.Write(counterBytes)
	hash := mac.Sum(nil)

	// Dynamic truncation
	offset := hash[len(hash)-1] & 0x0f
	code := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff

	// Modulo to get required digits
	mod := uint32(1)
	for i := 0; i < config.Digits; i++ {
		mod *= 10
	}
	code = code % mod

	// Format with leading zeros
	format := fmt.Sprintf("%%0%dd", config.Digits)
	return fmt.Sprintf(format, code)
}
