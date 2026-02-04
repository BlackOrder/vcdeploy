package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateIPAddress(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"valid IPv4", "192.168.1.1", false},
		{"valid IPv4 localhost", "127.0.0.1", false},
		{"valid IPv4 any", "0.0.0.0", false},
		{"valid IPv6 localhost", "::1", false},
		{"valid IPv6 full", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", false},
		{"valid IPv6 compressed", "2001:db8::1", false},
		{"empty", "", true},
		{"invalid format", "invalid", true},
		{"invalid octet", "999.999.999.999", true},
		{"hostname not IP", "example.com", true},
		{"partial IP", "192.168.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPAddress(tt.ip)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateHostname(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		wantErr  bool
	}{
		{"simple hostname", "server1", false},
		{"with domain", "server1.example.com", false},
		{"subdomain", "api.v1.example.com", false},
		{"hyphen", "my-server", false},
		{"numbers", "server123", false},
		{"empty", "", true},
		{"starts with hyphen", "-server", true},
		{"ends with hyphen", "server-", true},
		{"starts with dot", ".server", true},
		{"consecutive dots", "server..com", true},
		{"underscore", "my_server", true},
		{"space", "my server", true},
		{"too long label", string(make([]byte, 64)), true}, // 64 chars > 63 max
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostname(tt.hostname)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateHostOrIP(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid IP", "192.168.1.1", false},
		{"valid hostname", "example.com", false},
		{"localhost", "localhost", false},
		{"empty", "", true},
		{"invalid", "---", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostOrIP(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSSHPublicKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			"valid ssh-rsa",
			"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7...",
			false,
		},
		{
			"valid ssh-ed25519",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG...",
			false,
		},
		{
			"valid ecdsa-sha2-nistp256",
			"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTIt...",
			false,
		},
		{
			"valid ecdsa-sha2-nistp384",
			"ecdsa-sha2-nistp384 AAAAE2VjZHNhLXNoYTIt...",
			false,
		},
		{
			"valid ecdsa-sha2-nistp521",
			"ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTIt...",
			false,
		},
		{"empty", "", true},
		{"invalid format", "not-a-key", true},
		{"missing key data", "ssh-rsa ", true},
		{"invalid type", "dsa-foo AAAA...", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSSHPublicKey(tt.key)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateWebhookProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		wantErr  bool
	}{
		{"github", "github", false},
		{"GitHub uppercase", "GitHub", false},
		{"gitlab", "gitlab", false},
		{"bitbucket", "bitbucket", false},
		{"generic", "generic", false},
		{"empty", "", true},
		{"unsupported", "jenkins", true},
		{"invalid", "random", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebhookProvider(tt.provider)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateWebhookSecret(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{"valid 16 chars", "abcdefghijklmnop", false},
		{"valid 32 chars", "abcdefghijklmnopqrstuvwxyz012345", false},
		{"too short", "short", true},
		{"15 chars", "123456789012345", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebhookSecret(tt.secret)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"valid 22", 22, false},
		{"valid 80", 80, false},
		{"valid 443", 443, false},
		{"valid 8080", 8080, false},
		{"min valid", 1, false},
		{"max valid", 65535, false},
		{"zero", 0, true},
		{"negative", -1, true},
		{"too high", 65536, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePortString(t *testing.T) {
	tests := []struct {
		name    string
		port    string
		wantErr bool
	}{
		{"valid", "8080", false},
		{"empty", "", true},
		{"not a number", "abc", true},
		{"out of range", "70000", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePortString(tt.port)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCIDR(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		wantErr bool
	}{
		{"valid IPv4 /24", "192.168.1.0/24", false},
		{"valid IPv4 /32", "192.168.1.1/32", false},
		{"valid IPv4 /8", "10.0.0.0/8", false},
		{"valid IPv6", "2001:db8::/32", false},
		{"empty", "", true},
		{"missing prefix", "192.168.1.0", true},
		{"invalid IP", "999.999.999.0/24", true},
		{"invalid prefix", "192.168.1.0/99", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCIDR(tt.cidr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_FormatMethods(t *testing.T) {
	t.Run("IPAddress method", func(t *testing.T) {
		v := NewValidator()
		v.IPAddress("ip", "invalid")
		assert.True(t, v.HasErrors())
	})

	t.Run("Hostname method", func(t *testing.T) {
		v := NewValidator()
		v.Hostname("host", "valid-host.com")
		assert.False(t, v.HasErrors())
	})

	t.Run("HostOrIP method", func(t *testing.T) {
		v := NewValidator()
		v.HostOrIP("target", "192.168.1.1")
		assert.False(t, v.HasErrors())
	})

	t.Run("SSHPublicKey method", func(t *testing.T) {
		v := NewValidator()
		v.SSHPublicKey("key", "ssh-ed25519 AAAAC3...")
		assert.False(t, v.HasErrors())
	})

	t.Run("WebhookProvider method", func(t *testing.T) {
		v := NewValidator()
		v.WebhookProvider("provider", "github")
		assert.False(t, v.HasErrors())
	})

	t.Run("WebhookSecret method", func(t *testing.T) {
		v := NewValidator()
		v.WebhookSecret("secret", "short")
		assert.True(t, v.HasErrors())
	})

	t.Run("Port method", func(t *testing.T) {
		v := NewValidator()
		v.Port("port", 8080)
		assert.False(t, v.HasErrors())
	})

	t.Run("CIDR method", func(t *testing.T) {
		v := NewValidator()
		v.CIDR("network", "10.0.0.0/8")
		assert.False(t, v.HasErrors())
	})

	t.Run("chained validation", func(t *testing.T) {
		v := NewValidator()
		v.Required("name", "test").
			Port("port", 8080).
			Hostname("host", "example.com")
		assert.False(t, v.HasErrors())
	})
}
