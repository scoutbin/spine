package release

import (
	"strconv"
	"strings"
)

// Version is a parsed vMAJOR.MINOR.PATCH[-prerelease][+build] version.
// Build metadata is dropped because it has no effect on precedence.
type Version struct {
	Major, Minor, Patch uint64
	Pre                 []string
}

// ParseVersion parses a semantic version with an optional leading "v".
func ParseVersion(s string) (Version, bool) {
	s = strings.TrimPrefix(s, "v")
	core, build, hasBuild := strings.Cut(s, "+")
	if hasBuild && !validIdentifiers(build, false) {
		return Version{}, false
	}
	core, pre, hasPre := strings.Cut(core, "-")
	if hasPre && !validIdentifiers(pre, true) {
		return Version{}, false
	}

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	var nums [3]uint64
	for i, part := range parts {
		if !isNumeric(part) || hasLeadingZero(part) {
			return Version{}, false
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return Version{}, false
		}
		nums[i] = n
	}

	v := Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}
	if hasPre {
		v.Pre = strings.Split(pre, ".")
	}
	return v, true
}

// Compare orders a and b by semver precedence and returns -1, 0 or 1.
// An unparseable version (such as "dev") is older than any release, and two
// unparseable versions are equal.
func Compare(a, b string) int {
	va, okA := ParseVersion(a)
	vb, okB := ParseVersion(b)
	switch {
	case !okA && !okB:
		return 0
	case !okA:
		return -1
	case !okB:
		return 1
	}
	return va.Compare(vb)
}

// Compare orders v and other by semver precedence and returns -1, 0 or 1.
func (v Version) Compare(other Version) int {
	for _, c := range []int{
		compareUint(v.Major, other.Major),
		compareUint(v.Minor, other.Minor),
		compareUint(v.Patch, other.Patch),
	} {
		if c != 0 {
			return c
		}
	}
	return comparePre(v.Pre, other.Pre)
}

func comparePre(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := compareIdentifier(a[i], b[i]); c != 0 {
			return c
		}
	}
	return compareUint(uint64(len(a)), uint64(len(b)))
}

func compareIdentifier(a, b string) int {
	numA, numB := isNumeric(a), isNumeric(b)
	switch {
	case numA && numB:
		// Identifiers have no leading zeros, so length then bytes orders them
		// numerically without overflowing.
		if c := compareUint(uint64(len(a)), uint64(len(b))); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	case numA:
		return -1
	case numB:
		return 1
	}
	return strings.Compare(a, b)
}

func compareUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func validIdentifiers(s string, rejectNumericLeadingZero bool) bool {
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return false
		}
		for _, r := range id {
			if !isIdentifierRune(r) {
				return false
			}
		}
		if rejectNumericLeadingZero && isNumeric(id) && hasLeadingZero(id) {
			return false
		}
	}
	return true
}

func isIdentifierRune(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-'
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func hasLeadingZero(s string) bool {
	return len(s) > 1 && s[0] == '0'
}
