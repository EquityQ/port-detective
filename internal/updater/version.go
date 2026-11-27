// Package updater checks GitHub Releases for newer port-detective binaries,
// verifies SHA-256 digests, and atomically replaces standalone executables.
package updater

import (
	"fmt"
	"strconv"
	"strings"
)

// SemVer represents a parsed semantic version with optional prerelease metadata.
type SemVer struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
}

// String returns the canonical semantic version string without a leading "v".
func (v SemVer) String() string {
	base := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		return base + "-" + v.Prerelease
	}
	return base
}

// ParseVersion parses a semantic version such as "1.5.0", "v1.5.0", or "1.5.0-dev".
func ParseVersion(raw string) (SemVer, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return SemVer{}, fmt.Errorf("version must not be empty")
	}
	if plus := strings.IndexByte(s, '+'); plus >= 0 {
		if plus == len(s)-1 {
			return SemVer{}, fmt.Errorf("invalid build metadata in version %q", raw)
		}
		s = s[:plus]
	}

	var pre string
	if dash := strings.IndexByte(s, '-'); dash >= 0 {
		pre = s[dash+1:]
		s = s[:dash]
		if pre == "" || strings.ContainsAny(pre, " \t/\\") {
			return SemVer{}, fmt.Errorf("invalid prerelease in version %q", raw)
		}
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return SemVer{}, fmt.Errorf("invalid semantic version %q: expected MAJOR.MINOR.PATCH", raw)
	}
	major, err := parseNonNegInt(parts[0], raw)
	if err != nil {
		return SemVer{}, err
	}
	minor, err := parseNonNegInt(parts[1], raw)
	if err != nil {
		return SemVer{}, err
	}
	patch, err := parseNonNegInt(parts[2], raw)
	if err != nil {
		return SemVer{}, err
	}
	return SemVer{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: pre,
	}, nil
}

// NormalizeTargetVersion validates an explicit release target such as "1.5.0" or "v1.5.0"
// and returns the canonical "X.Y.Z" form without prerelease or build suffixes.
func NormalizeTargetVersion(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("target version must not be empty")
	}
	if strings.ContainsAny(trimmed, "-+/\\") {
		return "", fmt.Errorf("target version %q must be an exact X.Y.Z release", raw)
	}
	parsed, err := ParseVersion(trimmed)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.%d", parsed.Major, parsed.Minor, parsed.Patch), nil
}

// CompareVersions compares two semantic versions per SemVer 2.0.0 ordering, returning
// -1 if a < b, 0 if a == b, and 1 if a > b.
func CompareVersions(a, b SemVer) int {
	if a.Major != b.Major {
		return cmpInt(a.Major, b.Major)
	}
	if a.Minor != b.Minor {
		return cmpInt(a.Minor, b.Minor)
	}
	if a.Patch != b.Patch {
		return cmpInt(a.Patch, b.Patch)
	}
	if a.Prerelease == b.Prerelease {
		return 0
	}
	if a.Prerelease == "" {
		return 1
	}
	if b.Prerelease == "" {
		return -1
	}
	return comparePrerelease(a.Prerelease, b.Prerelease)
}

func comparePrerelease(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	limit := len(pa)
	if len(pb) < limit {
		limit = len(pb)
	}
	for i := 0; i < limit; i++ {
		if pa[i] == pb[i] {
			continue
		}
		aNum, aIsNum := parseDigits(pa[i])
		bNum, bIsNum := parseDigits(pb[i])
		switch {
		case aIsNum && bIsNum:
			if aNum != bNum {
				return cmpInt(aNum, bNum)
			}
		case aIsNum:
			return -1
		case bIsNum:
			return 1
		case pa[i] < pb[i]:
			return -1
		default:
			return 1
		}
	}
	return cmpInt(len(pa), len(pb))
}

func parseNonNegInt(segment, full string) (int, error) {
	if segment == "" || !isASCIIDigits(segment) {
		return 0, fmt.Errorf("invalid semantic version %q", full)
	}
	n, err := strconv.Atoi(segment)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid semantic version %q", full)
	}
	return n, nil
}

func parseDigits(s string) (int, bool) {
	if s == "" || !isASCIIDigits(s) {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func isASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
