// Package validation provides input validation for vcdeploy.
package validation

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Format validation patterns
var (
	// Hostname: RFC 1123 compliant
	hostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)

	// SSH public key formats: ssh-rsa, ssh-ed25519, ecdsa-sha2-*
	sshKeyRegex = regexp.MustCompile(`^(ssh-rsa|ssh-ed25519|ecdsa-sha2-nistp256|ecdsa-sha2-nistp384|ecdsa-sha2-nistp521)\s+[A-Za-z0-9+/=]+`)

	// Webhook secret: at least 16 characters, alphanumeric with special chars
	webhookSecretMinLength = 16
)

// Supported webhook providers
var validWebhookProviders = map[string]bool{
	"github":    true,
	"gitlab":    true,
	"bitbucket": true,
	"generic":   true,
}

// ValidateIPAddress validates an IP address (IPv4 or IPv6).
func ValidateIPAddress(ip string) error {
	if ip == "" {
		return fmt.Errorf("IP address is required")
	}
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}
	return nil
}

// ValidateHostname validates a hostname according to RFC 1123.
func ValidateHostname(h string) error {
	if h == "" {
		return fmt.Errorf("hostname is required")
	}
	if len(h) > 253 {
		return fmt.Errorf("hostname too long (max 253 characters)")
	}
	if !hostnameRegex.MatchString(h) {
		return fmt.Errorf("invalid hostname: %s", h)
	}
	return nil
}

// ValidateHostOrIP validates either a hostname or IP address.
func ValidateHostOrIP(s string) error {
	if s == "" {
		return fmt.Errorf("host or IP address is required")
	}
	// Try as IP first (faster)
	if net.ParseIP(s) != nil {
		return nil
	}
	// Try as hostname
	return ValidateHostname(s)
}

// ValidateSSHPublicKey validates an SSH public key format.
func ValidateSSHPublicKey(key string) error {
	if key == "" {
		return fmt.Errorf("SSH public key is required")
	}
	key = strings.TrimSpace(key)
	if !sshKeyRegex.MatchString(key) {
		return fmt.Errorf("invalid SSH public key format (expected ssh-rsa, ssh-ed25519, or ecdsa-sha2-* format)")
	}
	return nil
}

// ValidateWebhookProvider validates a webhook provider name.
func ValidateWebhookProvider(p string) error {
	if p == "" {
		return fmt.Errorf("webhook provider is required")
	}
	if !validWebhookProviders[strings.ToLower(p)] {
		providers := make([]string, 0, len(validWebhookProviders))
		for k := range validWebhookProviders {
			providers = append(providers, k)
		}
		return fmt.Errorf("unsupported webhook provider: %s (supported: %s)", p, strings.Join(providers, ", "))
	}
	return nil
}

// ValidateWebhookSecret validates a webhook secret meets minimum security requirements.
func ValidateWebhookSecret(secret string) error {
	if len(secret) < webhookSecretMinLength {
		return fmt.Errorf("webhook secret must be at least %d characters", webhookSecretMinLength)
	}
	return nil
}

// ValidatePort validates a port number.
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port number: %d (must be 1-65535)", port)
	}
	return nil
}

// ValidatePortString validates a port number from string.
func ValidatePortString(port string) error {
	if port == "" {
		return fmt.Errorf("port is required")
	}
	var portNum int
	if _, err := fmt.Sscanf(port, "%d", &portNum); err != nil {
		return fmt.Errorf("invalid port: %s", port)
	}
	return ValidatePort(portNum)
}

// ValidateCIDR validates a CIDR notation (IP/prefix).
func ValidateCIDR(cidr string) error {
	if cidr == "" {
		return fmt.Errorf("CIDR is required")
	}
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR notation: %s", cidr)
	}
	return nil
}

// Validator method extensions for format validation

// IPAddress validates an IP address field.
func (v *Validator) IPAddress(field, value string) *Validator {
	if err := ValidateIPAddress(value); err != nil {
		v.errors.Add(field, err.Error())
	}
	return v
}

// Hostname validates a hostname field.
func (v *Validator) Hostname(field, value string) *Validator {
	if err := ValidateHostname(value); err != nil {
		v.errors.Add(field, err.Error())
	}
	return v
}

// HostOrIP validates a host or IP address field.
func (v *Validator) HostOrIP(field, value string) *Validator {
	if err := ValidateHostOrIP(value); err != nil {
		v.errors.Add(field, err.Error())
	}
	return v
}

// SSHPublicKey validates an SSH public key field.
func (v *Validator) SSHPublicKey(field, value string) *Validator {
	if err := ValidateSSHPublicKey(value); err != nil {
		v.errors.Add(field, err.Error())
	}
	return v
}

// WebhookProvider validates a webhook provider field.
func (v *Validator) WebhookProvider(field, value string) *Validator {
	if err := ValidateWebhookProvider(value); err != nil {
		v.errors.Add(field, err.Error())
	}
	return v
}

// WebhookSecret validates a webhook secret field.
func (v *Validator) WebhookSecret(field, value string) *Validator {
	if err := ValidateWebhookSecret(value); err != nil {
		v.errors.Add(field, err.Error())
	}
	return v
}

// Port validates a port number field.
func (v *Validator) Port(field string, port int) *Validator {
	if err := ValidatePort(port); err != nil {
		v.errors.Add(field, err.Error())
	}
	return v
}

// CIDR validates a CIDR notation field.
func (v *Validator) CIDR(field, value string) *Validator {
	if err := ValidateCIDR(value); err != nil {
		v.errors.Add(field, err.Error())
	}
	return v
}
