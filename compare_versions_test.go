package featurevisor

import (
	"testing"
)

func TestValidateAndParse(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		expectError bool
	}{
		{
			name:        "valid simple version",
			version:     "1.2.3",
			expectError: false,
		},
		{
			name:        "valid version with v prefix",
			version:     "v1.2.3",
			expectError: false,
		},
		{
			name:        "valid version with pre-release",
			version:     "1.2.3-alpha.1",
			expectError: false,
		},
		{
			name:        "valid version with build metadata",
			version:     "1.2.3+build.1",
			expectError: false,
		},
		{
			name:        "valid version with wildcard",
			version:     "1.2.*",
			expectError: false,
		},
		{
			name:        "invalid version - empty string",
			version:     "",
			expectError: true,
		},
		{
			name:        "invalid version - not semver",
			version:     "not-a-version",
			expectError: true,
		},
		{
			name:        "valid version - missing patch",
			version:     "1.2",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateAndParse(tt.version)
			if tt.expectError {
				if err == nil {
					t.Errorf("ValidateAndParse(%s) expected error but got none", tt.version)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateAndParse(%s) unexpected error: %v", tt.version, err)
				}
				if len(result) == 0 {
					t.Errorf("ValidateAndParse(%s) returned empty result", tt.version)
				}
			}
		})
	}
}

func TestIsWildcard(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"*", true},
		{"x", true},
		{"X", true},
		{"1", false},
		{"alpha", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := IsWildcard(tt.input)
			if result != tt.expected {
				t.Errorf("IsWildcard(%s) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTryParse(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
	}{
		{"123", 123},
		{"0", 0},
		{"alpha", "alpha"},
		{"1.2", "1.2"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := TryParse(tt.input)
			if result != tt.expected {
				t.Errorf("TryParse(%s) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCompareStrings(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{"numbers", "1", "2", -1},
		{"numbers equal", "1", "1", 0},
		{"numbers reverse", "2", "1", 1},
		{"strings", "alpha", "beta", -1},
		{"strings equal", "alpha", "alpha", 0},
		{"strings reverse", "beta", "alpha", 1},
		{"mixed types", "1", "alpha", -1},
		{"wildcard a", "*", "1", 0},
		{"wildcard b", "1", "*", 0},
		{"both wildcards", "*", "x", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareStrings(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("CompareStrings(%s, %s) = %d, expected %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestCompareSegments(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected int
	}{
		{"equal segments", []string{"1", "2", "3"}, []string{"1", "2", "3"}, 0},
		{"a less than b", []string{"1", "2", "3"}, []string{"1", "2", "4"}, -1},
		{"a greater than b", []string{"1", "2", "4"}, []string{"1", "2", "3"}, 1},
		{"different lengths", []string{"1", "2"}, []string{"1", "2", "3"}, -1},
		{"different lengths reverse", []string{"1", "2", "3"}, []string{"1", "2"}, 1},
		{"empty segments", []string{}, []string{}, 0},
		{"with wildcards", []string{"1", "*", "3"}, []string{"1", "2", "3"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareSegments(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("CompareSegments(%v, %v) = %d, expected %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name        string
		v1          string
		v2          string
		expected    int
		expectError bool
	}{
		{
			name:        "equal versions",
			v1:          "1.2.3",
			v2:          "1.2.3",
			expected:    0,
			expectError: false,
		},
		{
			name:        "v1 less than v2",
			v1:          "1.2.3",
			v2:          "1.2.4",
			expected:    -1,
			expectError: false,
		},
		{
			name:        "v1 greater than v2",
			v1:          "1.2.4",
			v2:          "1.2.3",
			expected:    1,
			expectError: false,
		},
		{
			name:        "major version difference",
			v1:          "1.2.3",
			v2:          "2.0.0",
			expected:    -1,
			expectError: false,
		},
		{
			name:        "minor version difference",
			v1:          "1.2.3",
			v2:          "1.3.0",
			expected:    -1,
			expectError: false,
		},
		{
			name:        "with pre-release",
			v1:          "1.2.3-alpha",
			v2:          "1.2.3",
			expected:    -1,
			expectError: false,
		},
		{
			name:        "both with pre-release",
			v1:          "1.2.3-alpha.1",
			v2:          "1.2.3-alpha.2",
			expected:    -1,
			expectError: false,
		},
		{
			name:        "with build metadata",
			v1:          "1.2.3+build.1",
			v2:          "1.2.3+build.2",
			expected:    0, // Build metadata should be ignored
			expectError: false,
		},
		{
			name:        "with v prefix",
			v1:          "v1.2.3",
			v2:          "1.2.3",
			expected:    0,
			expectError: false,
		},
		{
			name:        "invalid first version",
			v1:          "not-a-version",
			v2:          "1.2.3",
			expectError: true,
		},
		{
			name:        "invalid second version",
			v1:          "1.2.3",
			v2:          "not-a-version",
			expectError: true,
		},
		{
			name:        "wildcard comparison",
			v1:          "1.2.*",
			v2:          "1.2.3",
			expected:    0, // Wildcards should be treated as equal
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CompareVersions(tt.v1, tt.v2)
			if tt.expectError {
				if err == nil {
					t.Errorf("CompareVersions(%s, %s) expected error but got none", tt.v1, tt.v2)
				}
			} else {
				if err != nil {
					t.Errorf("CompareVersions(%s, %s) unexpected error: %v", tt.v1, tt.v2, err)
				}
				if result != tt.expected {
					t.Errorf("CompareVersions(%s, %s) = %d, expected %d", tt.v1, tt.v2, result, tt.expected)
				}
			}
		})
	}
}

func BenchmarkCompareVersions(b *testing.B) {
	v1 := "1.2.3-alpha.1"
	v2 := "1.2.3-beta.2"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompareVersions(v1, v2)
	}
}
