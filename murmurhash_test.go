package featurevisor

import (
	"testing"
)

func TestMurmurHashV3(t *testing.T) {
	tests := []struct {
		name     string
		key      interface{}
		seed     uint32
		expected uint32
	}{
		{
			name:     "empty string",
			key:      "",
			seed:     0,
			expected: 0x00000000,
		},
		{
			name:     "simple string",
			key:      "hello",
			seed:     0,
			expected: 613153351,
		},
		{
			name:     "string with seed",
			key:      "hello",
			seed:     123,
			expected: 1573043710,
		},
		{
			name:     "longer string",
			key:      "featurevisor",
			seed:     0,
			expected: 2801817157,
		},
		{
			name:     "byte slice",
			key:      []byte("hello"),
			seed:     0,
			expected: 613153351,
		},
		{
			name:     "number as string",
			key:      "12345",
			seed:     0,
			expected: 329585043,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MurmurHashV3(tt.key, tt.seed)
			if result != tt.expected {
				t.Errorf("MurmurHashV3(%v, %d) = %d, want %d", tt.key, tt.seed, result, tt.expected)
			}
		})
	}
}

func TestMurmurHashV3Consistency(t *testing.T) {
	// Test that the same input always produces the same output
	key := "test_key"
	seed := uint32(42)

	result1 := MurmurHashV3(key, seed)
	result2 := MurmurHashV3(key, seed)

	if result1 != result2 {
		t.Errorf("MurmurHashV3 is not consistent: %d != %d", result1, result2)
	}
}

func TestMurmurHashV3DifferentSeeds(t *testing.T) {
	key := "test_key"

	// Test that different seeds produce different results
	result1 := MurmurHashV3(key, 0)
	result2 := MurmurHashV3(key, 1)

	if result1 == result2 {
		t.Errorf("MurmurHashV3 with different seeds should produce different results: %d == %d", result1, result2)
	}
}

func BenchmarkMurmurHashV3(b *testing.B) {
	key := "benchmark_test_key"
	seed := uint32(123)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MurmurHashV3(key, seed)
	}
}
