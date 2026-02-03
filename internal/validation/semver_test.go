package validation

import (
	"testing"
)

func TestValidateSemver(t *testing.T) {
	tests := []struct {
		version string
		valid   bool
	}{
		{"v1.0.0", true},
		{"1.0.0", true},
		{"v1.2.3-alpha", true},
		{"v1.2.3-alpha.1", true},
		{"v1.2.3+build", true},
		{"v1.2.3-alpha+build", true},
		{"v0.0.0", true},
		// Note: x/mod/semver accepts short versions like "v1", "v1.0"
		{"v1", true},
		{"v1.0", true},
		{"1", true},
		{"1.0", true},
		// Invalid cases
		{"invalid", false},
		{"", false},
		{"v", false},
		{"vv1.0.0", false},
		{"v-1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			err := ValidateSemver(tt.version)
			if tt.valid && err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected invalid, got nil error")
			}
		})
	}
}

func TestNormalizeSemver(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.0.0", "v1.0.0"},
		{"v1.0.0", "v1.0.0"},
		{"2.0.0-alpha", "v2.0.0-alpha"},
		{"v2.0.0-alpha", "v2.0.0-alpha"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeSemver(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeSemver(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"1.0.0", "v1.0.0", 0},
		{"v1.0.0", "v2.0.0", -1},
		{"v2.0.0", "v1.0.0", 1},
		{"v1.0.0", "v1.1.0", -1},
		{"v1.1.0", "v1.0.0", 1},
		{"v1.0.0", "v1.0.1", -1},
		{"v1.0.1", "v1.0.0", 1},
		{"v1.0.0-alpha", "v1.0.0", -1},
		{"v1.0.0", "v1.0.0-alpha", 1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			result := CompareSemver(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("CompareSemver(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestLatestSemver(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		expected string
	}{
		{
			name:     "multiple versions",
			versions: []string{"v1.0.0", "v2.0.0", "v1.5.0", "v2.1.0"},
			expected: "v2.1.0",
		},
		{
			name:     "mixed prefix",
			versions: []string{"1.0.0", "v2.0.0", "1.5.0"},
			expected: "v2.0.0",
		},
		{
			name:     "single version",
			versions: []string{"v1.0.0"},
			expected: "v1.0.0",
		},
		{
			name:     "empty list",
			versions: []string{},
			expected: "",
		},
		{
			name:     "prerelease versions",
			versions: []string{"v1.0.0-alpha", "v1.0.0", "v1.0.0-beta"},
			expected: "v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LatestSemver(tt.versions)
			if result != tt.expected {
				t.Errorf("LatestSemver(%v) = %q, want %q", tt.versions, result, tt.expected)
			}
		})
	}
}

func TestSortSemverDesc(t *testing.T) {
	versions := []string{"v1.0.0", "v2.1.0", "v1.5.0", "v2.0.0"}
	expected := []string{"v2.1.0", "v2.0.0", "v1.5.0", "v1.0.0"}

	result := SortSemverDesc(versions)

	if len(result) != len(expected) {
		t.Fatalf("len mismatch: got %d, want %d", len(result), len(expected))
	}

	for i, v := range expected {
		if result[i] != v {
			t.Errorf("SortSemverDesc()[%d] = %q, want %q", i, result[i], v)
		}
	}
}

func TestSortSemverAsc(t *testing.T) {
	versions := []string{"v2.1.0", "v1.0.0", "v1.5.0", "v2.0.0"}
	expected := []string{"v1.0.0", "v1.5.0", "v2.0.0", "v2.1.0"}

	result := SortSemverAsc(versions)

	if len(result) != len(expected) {
		t.Fatalf("len mismatch: got %d, want %d", len(result), len(expected))
	}

	for i, v := range expected {
		if result[i] != v {
			t.Errorf("SortSemverAsc()[%d] = %q, want %q", i, result[i], v)
		}
	}
}
