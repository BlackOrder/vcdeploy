package security_test

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAgentID(t *testing.T) {
	// Create a valid 64 char string
	valid64 := "a123456789012345678901234567890123456789012345678901234567890123"

	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		// Valid IDs
		{"valid simple", "agent1", nil},
		{"valid with hyphen", "agent-1", nil},
		{"valid with underscore", "agent_1", nil},
		{"valid mixed", "agent-1_test", nil},
		{"valid 3 chars", "abc", nil},
		{"valid 64 chars", valid64, nil},
		{"valid uppercase", "AgentOne", nil},
		{"valid numeric start", "1agent", nil},

		// Invalid IDs
		{"empty", "", security.ErrEmptyValue},
		{"too short 1 char", "a", security.ErrInvalidAgentID},
		{"too short 2 chars", "ab", security.ErrInvalidAgentID},
		{"starts with hyphen", "-agent", security.ErrInvalidAgentID},
		{"starts with underscore", "_agent", security.ErrInvalidAgentID},
		{"contains space", "agent 1", security.ErrInvalidAgentID},
		{"contains dot", "agent.1", security.ErrInvalidAgentID},
		{"contains special", "agent@1", security.ErrInvalidAgentID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := security.ValidateAgentID(tt.id)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAgentID_ReservedNames(t *testing.T) {
	reserved := []string{
		"master", "server", "admin", "root", "system",
		"localhost", "all", "any", "none", "default",
		"internal", "external", "public", "private",
		"Master", "SERVER", "Admin", // case insensitive
	}

	for _, name := range reserved {
		t.Run(name, func(t *testing.T) {
			err := security.ValidateAgentID(name)
			require.Error(t, err)
			assert.ErrorIs(t, err, security.ErrReservedAgentID)
		})
	}
}

func TestValidateHostname(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		wantErr  error
	}{
		// Valid hostnames
		{"simple", "localhost", nil},
		{"with domain", "host.example.com", nil},
		{"with subdomain", "sub.host.example.com", nil},
		{"with hyphen", "my-host.example.com", nil},
		{"with numbers", "host123.example.com", nil},

		// Invalid hostnames
		{"empty", "", security.ErrEmptyValue},
		{"starts with hyphen", "-host.com", security.ErrInvalidHostname},
		{"ends with hyphen", "host-.com", security.ErrInvalidHostname},
		{"contains underscore", "my_host.com", security.ErrInvalidHostname},
		{"starts with dot", ".example.com", security.ErrInvalidHostname},
		{"ends with dot", "example.com.", security.ErrInvalidHostname},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := security.ValidateHostname(tt.hostname)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateHostname_TooLong(t *testing.T) {
	// Create a hostname that's too long (>253 chars)
	longHostname := string(make([]byte, 254))
	for i := range longHostname {
		longHostname = string(append([]byte(longHostname[:i]), 'a'))
	}
	// Simpler approach
	longHostname = ""
	for i := 0; i < 260; i++ {
		longHostname += "a"
	}

	err := security.ValidateHostname(longHostname)
	require.Error(t, err)
	assert.ErrorIs(t, err, security.ErrValueTooLong)
}

func TestValidateRegistrationToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		// Valid tokens
		{"valid 32 chars", "12345678901234567890123456789012", false},
		{"valid longer", "abcdefghij1234567890abcdefghij1234567890", false},

		// Invalid tokens
		{"empty", "", true},
		{"too short", "short", true},
		{"31 chars", "1234567890123456789012345678901", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := security.ValidateRegistrationToken(tt.token)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateLabel(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		// Valid labels
		{"simple", "env", "prod", false},
		{"with dot", "app.version", "1.0", false},
		{"with hyphen", "app-name", "myapp", false},
		{"with underscore", "app_name", "myapp", false},
		{"empty value", "key", "", false},

		// Invalid labels
		{"empty key", "", "value", true},
		{"key starts with hyphen", "-key", "value", true},
		{"key starts with dot", ".key", "value", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := security.ValidateLabel(tt.key, tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCSR(t *testing.T) {
	tests := []struct {
		name    string
		csr     []byte
		wantErr bool
	}{
		// Invalid CSRs
		{"empty", []byte{}, true},
		{"no PEM header", []byte("some random data"), true},

		// Valid CSR header (just checking format, not content)
		{"valid header", []byte("-----BEGIN CERTIFICATE REQUEST-----\ntest\n-----END CERTIFICATE REQUEST-----"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := security.ValidateCSR(tt.csr)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSanitizeAgentID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "agent1", "agent1"},
		{"uppercase", "AgentOne", "agentone"},
		{"with spaces", "my agent", "my-agent"},
		{"with dots", "host.example.com", "host-example-com"},
		{"special chars", "agent@#$123", "agent123"},
		{"leading special", "@#agent", "agent"},
		{"trailing hyphen", "agent-", "agent"},
		{"consecutive special", "agent---test", "agent-test"},
		{"too short", "ab", "ab-agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := security.SanitizeAgentID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsReservedAgentID(t *testing.T) {
	// Test reserved IDs
	assert.True(t, security.IsReservedAgentID("master"))
	assert.True(t, security.IsReservedAgentID("Master"))
	assert.True(t, security.IsReservedAgentID("MASTER"))
	assert.True(t, security.IsReservedAgentID("admin"))
	assert.True(t, security.IsReservedAgentID("root"))

	// Test non-reserved IDs
	assert.False(t, security.IsReservedAgentID("agent1"))
	assert.False(t, security.IsReservedAgentID("my-agent"))
	assert.False(t, security.IsReservedAgentID("production"))
}
