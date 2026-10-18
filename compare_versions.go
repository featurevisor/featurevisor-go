package featurevisor

import (
	"fmt"
	"regexp"
	"strings"
)

var semverRegex = regexp.MustCompile(`(?i)^[v^~<>=]*?(\d+)(?:\.([x*]|\d+)(?:\.([x*]|\d+)(?:\.([x*]|\d+))?(?:-([\da-z\-]+(?:\.[\da-z\-]+)*))?(?:\+[\da-z\-]+(?:\.[\da-z\-]+)*)?)?)?$`)

// validateAndParse validates and parses a semantic version string
// Returns the parsed segments or an error if invalid
func validateAndParse(version string) ([]string, error) {
	if version == "" {
		return nil, fmt.Errorf("invalid argument expected string")
	}

	for _, c := range version {
		if c > 127 {
			return nil, fmt.Errorf("invalid argument not valid semver ('%s' received)", version)
		}
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

// compareStrings compares two strings, handling wildcards and mixed types
func compareStrings(a, b string, prerelease bool) int {
	if !prerelease && (isWildcard(a) || isWildcard(b)) {
		return 0
	}
	numeric := func(s string) bool {
		if s == "" {
			return false
		}
		for _, c := range s {
			if c < '0' || c > '9' {
				return false
			}
		}
		return true
	}
	an, bn := numeric(a), numeric(b)
	if an && bn {
		a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
		if len(a) > len(b) {
			return 1
		}
		if len(a) < len(b) {
			return -1
		}
	} else if prerelease && an != bn {
		if an {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

// compareSegments compares two arrays of version segments
func compareSegments(a, b []string, prerelease bool) int {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	for i := 0; i < maxLen; i++ {
		if prerelease && (i == len(a) || i == len(b)) {
			if len(a) < len(b) {
				return -1
			}
			return 1
		}
		aVal := "0"
		bVal := "0"

		if i < len(a) && a[i] != "" {
			aVal = a[i]
		}
		if i < len(b) && b[i] != "" {
			bVal = b[i]
		}

		result := compareStrings(aVal, bVal, prerelease)
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

	// Separate the prerelease capture from the core components
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
	result := compareSegments(n1, n2, false)
	if result != 0 {
		return result, nil
	}

	// Compare prerelease versions
	if p1 != "" && p2 != "" {
		p1Parts := strings.Split(p1, ".")
		p2Parts := strings.Split(p2, ".")
		return compareSegments(p1Parts, p2Parts, true), nil
	} else if p1 != "" || p2 != "" {
		if p1 != "" {
			return -1, nil
		}
		return 1, nil
	}

	return 0, nil
}
