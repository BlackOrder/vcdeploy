package security

import (
	"strings"
	"testing"
)

func FuzzCommandValidator(f *testing.F) {
	// Add programmatic seeds with OWASP command injection patterns
	// Valid commands
	f.Add("echo hello")
	f.Add("git status")
	f.Add("npm install")
	f.Add("go build")
	f.Add("make test")

	// Command substitution attacks
	f.Add("$(whoami)")
	f.Add("`id`")
	f.Add("$(cat /etc/passwd)")
	f.Add("$((1+1))")

	// Command chaining attacks
	f.Add("cmd; rm -rf /")
	f.Add("cmd && evil")
	f.Add("cmd || evil")
	f.Add("cmd | cat /etc/passwd")

	// Environment variable injection
	f.Add("${IFS}cat${IFS}/etc/passwd")
	f.Add("${PATH}")
	f.Add("$HOME/.ssh/id_rsa")

	// Newline and control character injection
	f.Add("cmd\nrm -rf /")
	f.Add("cmd\recho pwned")
	f.Add("cmd\x00evil")

	// Redirection attacks
	f.Add("cmd > /etc/passwd")
	f.Add("cmd < /etc/passwd")
	f.Add("cmd >> /etc/passwd")
	f.Add("cmd << EOF")

	// Background execution
	f.Add("cmd & background")

	// Dangerous commands
	f.Add("eval echo test")
	f.Add("exec /bin/sh")

	validator := NewCommandValidator()

	f.Fuzz(func(t *testing.T, cmd string) {
		err := validator.Validate(cmd)

		// These patterns MUST always be rejected
		mustReject := []string{
			"$(", "`", "${", "&&", "||", ";", "|",
			">", "<", ">>", "<<", "&",
			"eval ", "exec ",
			"\n", "\r", "\x00",
		}

		for _, pattern := range mustReject {
			if strings.Contains(cmd, pattern) && err == nil {
				t.Errorf("validator accepted command with dangerous pattern %q: %s", pattern, cmd)
			}
		}
	})
}
