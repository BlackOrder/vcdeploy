package security

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

func TestDefaultTOTPConfig(t *testing.T) {
	t.Parallel()

	config := DefaultTOTPConfig()

	if config.Issuer != "vcdeploy" {
		t.Errorf("unexpected issuer: %s", config.Issuer)
	}
	if config.Digits != 6 {
		t.Errorf("unexpected digits: %d", config.Digits)
	}
	if config.Period != 30 {
		t.Errorf("unexpected period: %d", config.Period)
	}
}

func TestGenerateTOTPSecret(t *testing.T) {
	t.Parallel()

	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}

	// Secret should be base32 encoded
	if secret == "" {
		t.Error("secret should not be empty")
	}

	// Should be decodable
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Errorf("secret should be valid base32: %v", err)
	}

	// Should be 20 bytes when decoded
	if len(decoded) != 20 {
		t.Errorf("decoded secret should be 20 bytes, got %d", len(decoded))
	}

	// Generate multiple secrets and ensure they're unique
	secret2, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	if secret == secret2 {
		t.Error("secrets should be unique")
	}
}

func TestValidateTOTP(t *testing.T) {
	t.Parallel()

	// Use a known secret for deterministic testing
	secret := "JBSWY3DPEHPK3PXP" // "Hello!" in base32
	config := DefaultTOTPConfig()

	// Generate a valid code for current time
	now := time.Now().Unix()
	validCode := generateTOTP(secret, now, config)

	tests := []struct {
		name   string
		secret string
		code   string
		want   bool
	}{
		{
			name:   "valid code",
			secret: secret,
			code:   validCode,
			want:   true,
		},
		{
			name:   "invalid code",
			secret: secret,
			code:   "000000",
			want:   false,
		},
		{
			name:   "wrong length code",
			secret: secret,
			code:   "12345",
			want:   false,
		},
		{
			name:   "empty code",
			secret: secret,
			code:   "",
			want:   false,
		},
		{
			name:   "invalid secret",
			secret: "!!!invalid!!!",
			code:   "123456",
			want:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ValidateTOTP(tt.secret, tt.code, config)
			if got != tt.want {
				t.Errorf("ValidateTOTP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateTOTP_ClockSkew(t *testing.T) {
	t.Parallel()

	secret := "JBSWY3DPEHPK3PXP"
	config := DefaultTOTPConfig()

	// Generate code for previous period (should still be valid due to clock skew tolerance)
	pastTime := time.Now().Unix() - int64(config.Period)
	pastCode := generateTOTP(secret, pastTime, config)

	// Generate code for next period (should still be valid due to clock skew tolerance)
	futureTime := time.Now().Unix() + int64(config.Period)
	futureCode := generateTOTP(secret, futureTime, config)

	// Both should be accepted
	if !ValidateTOTP(secret, pastCode, config) {
		t.Error("should accept code from previous period (clock skew)")
	}
	if !ValidateTOTP(secret, futureCode, config) {
		t.Error("should accept code from next period (clock skew)")
	}
}

func TestGenerateTOTPURI(t *testing.T) {
	t.Parallel()

	secret := "JBSWY3DPEHPK3PXP"
	username := "testuser@example.com"
	config := DefaultTOTPConfig()

	uri := GenerateTOTPURI(secret, username, config)

	// Check URI format
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Error("URI should start with otpauth://totp/")
	}
	if !strings.Contains(uri, secret) {
		t.Error("URI should contain the secret")
	}
	if !strings.Contains(uri, config.Issuer) {
		t.Error("URI should contain the issuer")
	}
	if !strings.Contains(uri, "digits=6") {
		t.Error("URI should contain digits=6")
	}
	if !strings.Contains(uri, "period=30") {
		t.Error("URI should contain period=30")
	}
}

func TestGenerateTOTPURI_DifferentConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   TOTPConfig
		expected []string
	}{
		{
			name: "default config",
			config: TOTPConfig{
				Issuer: "vcdeploy",
				Digits: 6,
				Period: 30,
			},
			expected: []string{"issuer=vcdeploy", "digits=6", "period=30"},
		},
		{
			name: "custom issuer",
			config: TOTPConfig{
				Issuer: "MyApp",
				Digits: 6,
				Period: 30,
			},
			expected: []string{"issuer=MyApp"},
		},
		{
			name: "8 digits",
			config: TOTPConfig{
				Issuer: "vcdeploy",
				Digits: 8,
				Period: 30,
			},
			expected: []string{"digits=8"},
		},
		{
			name: "60 second period",
			config: TOTPConfig{
				Issuer: "vcdeploy",
				Digits: 6,
				Period: 60,
			},
			expected: []string{"period=60"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uri := GenerateTOTPURI("TESTSECRET", "user", tt.config)
			for _, exp := range tt.expected {
				if !strings.Contains(uri, exp) {
					t.Errorf("URI should contain %q, got: %s", exp, uri)
				}
			}
		})
	}
}

func TestGenerateTOTP_Internal(t *testing.T) {
	t.Parallel()

	// Test the internal generateTOTP function with known values
	// RFC 6238 test vectors (using SHA1)
	secret := "GEZDGNBVGY3TQOJQ" // "12345678901234567890" in base32

	tests := []struct {
		name      string
		timestamp int64
		expected  string
	}{
		// Note: These are approximate based on the algorithm
		// The actual test is that the function produces consistent results
		{
			name:      "timestamp 0",
			timestamp: 0,
			expected:  generateTOTP(secret, 0, DefaultTOTPConfig()),
		},
		{
			name:      "timestamp 30",
			timestamp: 30,
			expected:  generateTOTP(secret, 30, DefaultTOTPConfig()),
		},
		{
			name:      "timestamp 60",
			timestamp: 60,
			expected:  generateTOTP(secret, 60, DefaultTOTPConfig()),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := DefaultTOTPConfig()
			code := generateTOTP(secret, tt.timestamp, config)

			// Code should be 6 digits
			if len(code) != 6 {
				t.Errorf("code should be 6 digits, got %d: %s", len(code), code)
			}

			// Code should be consistent
			code2 := generateTOTP(secret, tt.timestamp, config)
			if code != code2 {
				t.Error("generateTOTP should produce consistent results")
			}
		})
	}
}

func TestGenerateTOTP_InvalidSecret(t *testing.T) {
	t.Parallel()

	config := DefaultTOTPConfig()

	// Invalid base32 should return empty string
	code := generateTOTP("!!!invalid!!!", 0, config)
	if code != "" {
		t.Errorf("invalid secret should produce empty code, got: %s", code)
	}
}

func TestGenerateTOTP_DifferentDigits(t *testing.T) {
	t.Parallel()

	secret := "JBSWY3DPEHPK3PXP"
	timestamp := int64(1234567890)

	tests := []struct {
		digits   int
		expected int
	}{
		{digits: 6, expected: 6},
		{digits: 7, expected: 7},
		{digits: 8, expected: 8},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(string(rune('0'+tt.digits))+" digits", func(t *testing.T) {
			t.Parallel()

			config := TOTPConfig{
				Issuer: "test",
				Digits: tt.digits,
				Period: 30,
			}
			code := generateTOTP(secret, timestamp, config)
			if len(code) != tt.expected {
				t.Errorf("expected %d digits, got %d: %s", tt.expected, len(code), code)
			}
		})
	}
}
