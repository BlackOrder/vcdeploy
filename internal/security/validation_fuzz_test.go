package security

import (
	"testing"
)

func FuzzValidateAgentID(f *testing.F) {
	// Valid seeds
	f.Add("agent-01")
	f.Add("my_agent")
	f.Add("Agent123")
	f.Add("web-server-prod-01")
	f.Add("db_replica_2")

	// Attack seeds
	f.Add("$(whoami)")
	f.Add("`id`")
	f.Add("agent;rm -rf /")
	f.Add("agent\necho pwned")
	f.Add("agent\x00evil")
	f.Add("../../../etc/passwd")
	f.Add("agent|cat /etc/passwd")
	f.Add("agent&&evil")
	f.Add("agent>file")
	f.Add("agent<file")
	f.Add("agent&background")
	f.Add("${IFS}cat")
	f.Add("agent\rcarriage")

	f.Fuzz(func(t *testing.T, id string) {
		err := ValidateAgentID(id)

		// If validation passes, verify it's actually safe
		if err == nil {
			// Must not contain shell metacharacters
			for _, r := range id {
				switch r {
				case '$', '`', ';', '|', '&', '\n', '\r', '\x00', '>', '<', '(', ')', '{', '}', '[', ']', '\'', '"', '\\', '!', '#', '*', '?':
					t.Errorf("ValidateAgentID accepted dangerous char %q (0x%02x) in: %q", r, r, id)
				}
			}
			// Must not be empty
			if len(id) == 0 {
				t.Error("ValidateAgentID accepted empty string")
			}
			// Must not contain path traversal
			if containsPathTraversal(id) {
				t.Errorf("ValidateAgentID accepted path traversal in: %q", id)
			}
		}
	})
}

func FuzzValidateHostname(f *testing.F) {
	// Valid seeds
	f.Add("example.com")
	f.Add("server-01.internal")
	f.Add("192.168.1.1")
	f.Add("localhost")
	f.Add("my-server.example.org")
	f.Add("10.0.0.1")
	f.Add("2001:db8::1")

	// Attack seeds
	f.Add("$(whoami).evil.com")
	f.Add("host`id`.com")
	f.Add("host;rm -rf /.com")
	f.Add("host\necho pwned")
	f.Add("host\x00evil")
	f.Add("host|cat.com")
	f.Add("host&&evil.com")
	f.Add("host>file.com")
	f.Add("${IFS}cat.com")
	f.Add("host\revil")

	f.Fuzz(func(t *testing.T, hostname string) {
		err := ValidateHostname(hostname)

		if err == nil {
			// Must not contain shell metacharacters
			for _, r := range hostname {
				switch r {
				case '$', '`', ';', '|', '&', '\n', '\r', '\x00', '>', '<', '(', ')', '{', '}', '\'', '"', '\\', '!', '#', '*', '?':
					t.Errorf("ValidateHostname accepted dangerous char %q (0x%02x) in: %q", r, r, hostname)
				}
			}
			// Must not be empty
			if len(hostname) == 0 {
				t.Error("ValidateHostname accepted empty string")
			}
		}
	})
}

// containsPathTraversal checks if a string contains path traversal patterns.
func containsPathTraversal(s string) bool {
	// Check for common path traversal patterns
	patterns := []string{"../", "..\\", "/.."}
	for _, p := range patterns {
		if containsString(s, p) {
			return true
		}
	}
	return false
}

// containsString checks if s contains substr.
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
