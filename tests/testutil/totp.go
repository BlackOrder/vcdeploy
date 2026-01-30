package testutil

import (
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
)

// TestTOTPSecret is a known TOTP secret for testing purposes.
// DO NOT use this in production.
const TestTOTPSecret = "JBSWY3DPEHPK3PXP"

// GenerateValidTOTP generates a valid TOTP code for the given secret at the current time.
func GenerateValidTOTP(secret string) string {
	return GenerateTOTPAtTime(secret, time.Now())
}

// GenerateTOTPAtTime generates a TOTP code for the given secret at the specified time.
func GenerateTOTPAtTime(secret string, t time.Time) string {
	config := security.DefaultTOTPConfig()
	return security.GenerateTOTPCode(secret, t.Unix(), config)
}

// GenerateExpiredTOTP generates a TOTP code that was valid 2 periods ago (expired).
func GenerateExpiredTOTP(secret string) string {
	config := security.DefaultTOTPConfig()
	// 3 periods ago - outside the ±1 tolerance window
	expiredTime := time.Now().Add(-time.Duration(3*config.Period) * time.Second)
	return security.GenerateTOTPCode(secret, expiredTime.Unix(), config)
}
