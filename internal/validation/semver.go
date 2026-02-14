// Package validation provides input validation functions.
package validation

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

// ValidateSemver checks if a version string is valid semver.
// Accepts both "1.0.0" and "v1.0.0" formats.
func ValidateSemver(v string) error {
	normalized := NormalizeSemver(v)
	if !semver.IsValid(normalized) {
		return fmt.Errorf("invalid semver: %s", v)
	}
	return nil
}

// NormalizeSemver ensures version has 'v' prefix for x/mod/semver compatibility.
func NormalizeSemver(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// CompareSemver compares two versions.
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2.
func CompareSemver(v1, v2 string) int {
	return semver.Compare(NormalizeSemver(v1), NormalizeSemver(v2))
}

// LatestSemver returns the highest version from a list.
func LatestSemver(versions []string) string {
	if len(versions) == 0 {
		return ""
	}

	normalized := make([]string, len(versions))
	for i, v := range versions {
		normalized[i] = NormalizeSemver(v)
	}

	semver.Sort(normalized)
	return normalized[len(normalized)-1]
}

// SortSemverDesc sorts versions in descending order (newest first).
func SortSemverDesc(versions []string) []string {
	normalized := make([]string, len(versions))
	for i, v := range versions {
		normalized[i] = NormalizeSemver(v)
	}

	semver.Sort(normalized)

	// Reverse for descending
	result := make([]string, len(normalized))
	for i, v := range normalized {
		result[len(normalized)-1-i] = v
	}
	return result
}

// SortSemverAsc sorts versions in ascending order (oldest first).
func SortSemverAsc(versions []string) []string {
	normalized := make([]string, len(versions))
	for i, v := range versions {
		normalized[i] = NormalizeSemver(v)
	}

	semver.Sort(normalized)
	return normalized
}
