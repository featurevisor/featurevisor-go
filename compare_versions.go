package featurevisor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var semverRegex = regexp.MustCompile(`(?i)^[v^~<>=]*?(\d+)(?:\.([x*]|\d+)(?:\.([x*]|\d+)(?:\.([x*]|\d+))?(?:-([\da-z\-]+(?:\.[\da-z\-]+)*))?(?:\+[\da-z\-]+(?:\.[\da-z\-]+)*)?)?)?$`)

// validateAndParse validates and parses a semantic version string
// Returns the parsed segments or an error if invalid
func validateAndParse(version string) ([]string, error) {
	if version == "" {
		return nil, fmt.Errorf("invalid argument expected string")
	}

	matches := semverRegex.FindStringSubmatch(version)
	if matches == nil {
		return nil, fmt.Errorf("invalid argument not valid semver ('%s' received)", version)
	}

	// Remove the full match (first element) and return the groups
	return matches[1:], nil
}

// isWildcard checks if a string is a wildcard character
func isWildcard(s string) bool {
	return s == "*" || s == "x" || s == "X"
}

// forceType ensures both values are of the same type for comparison
func forceType(a, b interface{}) (interface{}, interface{}) {
	if fmt.Sprintf("%T", a) != fmt.Sprintf("%T", b) {
		return fmt.Sprintf("%v", a), fmt.Sprintf("%v", b)
	}
	return a, b
}

// tryParse attempts to parse a string as an integer, returns the original string if it fails
func tryParse(v string) interface{} {
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return v
}

// compareStrings compares two strings, handling wildcards and mixed types
func compareStrings(a, b string) int {
	if isWildcard(a) || isWildcard(b) {
		return 0
	}

	ap, bp := forceType(tryParse(a), tryParse(b))

	switch apVal := ap.(type) {
	case int:
		if bpVal, ok := bp.(int); ok {
			if apVal > bpVal {
				return 1
			} else if apVal < bpVal {
				return -1
			}
			return 0
		}
		// Mixed types, convert to string comparison
		apStr := fmt.Sprintf("%v", ap)
		bpStr := fmt.Sprintf("%v", bp)
		return strings.Compare(apStr, bpStr)
	default:
		// String comparison
		apStr := fmt.Sprintf("%v", ap)
		bpStr := fmt.Sprintf("%v", bp)
		return strings.Compare(apStr, bpStr)
	}
}

// compareSegments compares two arrays of version segments
func compareSegments(a, b []string) int {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	for i := 0; i < maxLen; i++ {
		aVal := "0"
		bVal := "0"

		if i < len(a) {
			aVal = a[i]
		}
		if i < len(b) {
			bVal = b[i]
		}

		result := compareStrings(aVal, bVal)
		if result != 0 {
			return result
		}
	}

	return 0
}

// compareVersions compares two semantic version strings
// Returns:
//
//	-1 if v1 < v2
//	 0 if v1 == v2
//	 1 if v1 > v2
func compareVersions(v1, v2 string) (int, error) {
	// Validate input and split into segments
	n1, err := validateAndParse(v1)
	if err != nil {
		return 0, fmt.Errorf("invalid first version: %w", err)
	}

	n2, err := validateAndParse(v2)
	if err != nil {
		return 0, fmt.Errorf("invalid second version: %w", err)
	}

	// Pop off the patch (last element)
	var p1, p2 string
	if len(n1) > 0 {
		p1 = n1[len(n1)-1]
		n1 = n1[:len(n1)-1]
	}
	if len(n2) > 0 {
		p2 = n2[len(n2)-1]
		n2 = n2[:len(n2)-1]
	}

	// Compare main version segments
	result := compareSegments(n1, n2)
	if result != 0 {
		return result, nil
	}

	// Compare pre-release versions
	if p1 != "" && p2 != "" {
		p1Parts := strings.Split(p1, ".")
		p2Parts := strings.Split(p2, ".")
		return compareSegments(p1Parts, p2Parts), nil
	} else if p1 != "" || p2 != "" {
		if p1 != "" {
			return -1, nil
		}
		return 1, nil
	}

	return 0, nil
}
