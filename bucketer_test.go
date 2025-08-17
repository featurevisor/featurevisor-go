package featurevisor

import (
	"testing"
)

func TestGetBucketedNumber(t *testing.T) {

	t.Run("should return a number between 0 and 100000", func(t *testing.T) {
		keys := []string{"foo", "bar", "baz", "123adshlk348-93asdlk"}

		for _, key := range keys {
			n := GetBucketedNumber(key)

			if n < 0 {
				t.Errorf("GetBucketedNumber(%s) = %d; want >= 0", key, n)
			}
			if n > MAX_BUCKETED_NUMBER {
				t.Errorf("GetBucketedNumber(%s) = %d; want <= %d", key, n, MAX_BUCKETED_NUMBER)
			}
		}
	})

	t.Run("should return expected number for known keys", func(t *testing.T) {
		expectedResults := map[string]int{
			"foo":         20602,
			"bar":         89144,
			"123.foo":     3151,
			"123.bar":     9710,
			"123.456.foo": 14432,
			"123.456.bar": 1982,
		}

		for key, expected := range expectedResults {
			n := GetBucketedNumber(key)

			if n != expected {
				t.Errorf("GetBucketedNumber(%s) = %d; want %d", key, n, expected)
			}
		}
	})
}

func TestGetBucketKey(t *testing.T) {
	logger := NewLogger(CreateLoggerOptions{})

	t.Run("plain: should return a bucket key for a plain bucketBy", func(t *testing.T) {
		featureKey := FeatureKey("test-feature")
		bucketBy := "userId"
		context := Context{
			"userId":  "123",
			"browser": "chrome",
		}

		bucketKey := GetBucketKey(GetBucketKeyOptions{
			FeatureKey: featureKey,
			BucketBy:   bucketBy,
			Context:    context,
			Logger:     logger,
		})

		expected := "123.test-feature"
		if bucketKey != expected {
			t.Errorf("GetBucketKey() = %s; want %s", bucketKey, expected)
		}
	})

	t.Run("plain: should return a bucket key with feature key only if value is missing in context", func(t *testing.T) {
		featureKey := FeatureKey("test-feature")
		bucketBy := "userId"
		context := Context{
			"browser": "chrome",
		}

		bucketKey := GetBucketKey(GetBucketKeyOptions{
			FeatureKey: featureKey,
			BucketBy:   bucketBy,
			Context:    context,
			Logger:     logger,
		})

		expected := "test-feature"
		if bucketKey != expected {
			t.Errorf("GetBucketKey() = %s; want %s", bucketKey, expected)
		}
	})

	t.Run("and: should combine multiple field values together if present", func(t *testing.T) {
		featureKey := FeatureKey("test-feature")
		bucketBy := []string{"organizationId", "userId"}
		context := Context{
			"organizationId": "123",
			"userId":         "234",
			"browser":        "chrome",
		}

		bucketKey := GetBucketKey(GetBucketKeyOptions{
			FeatureKey: featureKey,
			BucketBy:   bucketBy,
			Context:    context,
			Logger:     logger,
		})

		expected := "123.234.test-feature"
		if bucketKey != expected {
			t.Errorf("GetBucketKey() = %s; want %s", bucketKey, expected)
		}
	})

	t.Run("and: should combine only available field values together if present", func(t *testing.T) {
		featureKey := FeatureKey("test-feature")
		bucketBy := []string{"organizationId", "userId"}
		context := Context{
			"organizationId": "123",
			"browser":        "chrome",
		}

		bucketKey := GetBucketKey(GetBucketKeyOptions{
			FeatureKey: featureKey,
			BucketBy:   bucketBy,
			Context:    context,
			Logger:     logger,
		})

		expected := "123.test-feature"
		if bucketKey != expected {
			t.Errorf("GetBucketKey() = %s; want %s", bucketKey, expected)
		}
	})

	t.Run("and: should combine all available fields, with dot separated paths", func(t *testing.T) {
		featureKey := FeatureKey("test-feature")
		bucketBy := []string{"organizationId", "user.id"}
		context := Context{
			"organizationId": "123",
			"user": map[string]interface{}{
				"id": "234",
			},
			"browser": "chrome",
		}

		bucketKey := GetBucketKey(GetBucketKeyOptions{
			FeatureKey: featureKey,
			BucketBy:   bucketBy,
			Context:    context,
			Logger:     logger,
		})

		expected := "123.234.test-feature"
		if bucketKey != expected {
			t.Errorf("GetBucketKey() = %s; want %s", bucketKey, expected)
		}
	})

	t.Run("or: should take first available field value", func(t *testing.T) {
		featureKey := FeatureKey("test-feature")
		bucketBy := OrBucketBy{
			Or: []string{"userId", "deviceId"},
		}
		context := Context{
			"deviceId": "deviceIdHere",
			"userId":   "234",
			"browser":  "chrome",
		}

		bucketKey := GetBucketKey(GetBucketKeyOptions{
			FeatureKey: featureKey,
			BucketBy:   bucketBy,
			Context:    context,
			Logger:     logger,
		})

		expected := "234.test-feature"
		if bucketKey != expected {
			t.Errorf("GetBucketKey() = %s; want %s", bucketKey, expected)
		}
	})

	t.Run("or: should take first available field value when first is missing", func(t *testing.T) {
		featureKey := FeatureKey("test-feature")
		bucketBy := OrBucketBy{
			Or: []string{"userId", "deviceId"},
		}
		context := Context{
			"deviceId": "deviceIdHere",
			"browser":  "chrome",
		}

		bucketKey := GetBucketKey(GetBucketKeyOptions{
			FeatureKey: featureKey,
			BucketBy:   bucketBy,
			Context:    context,
			Logger:     logger,
		})

		expected := "deviceIdHere.test-feature"
		if bucketKey != expected {
			t.Errorf("GetBucketKey() = %s; want %s", bucketKey, expected)
		}
	})

	t.Run("should handle invalid bucketBy", func(t *testing.T) {
		featureKey := FeatureKey("test-feature")
		// Pass an invalid type that doesn't match any of the expected types
		bucketBy := 123 // This is an invalid type
		context := Context{}

		defer func() {
			if r := recover(); r == nil {
				t.Error("GetBucketKey should panic with invalid bucketBy")
			}
		}()

		GetBucketKey(GetBucketKeyOptions{
			FeatureKey: featureKey,
			BucketBy:   bucketBy,
			Context:    context,
			Logger:     logger,
		})
	})
}

func TestToString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "int",
			input:    123,
			expected: "123",
		},
		{
			name:     "float64",
			input:    123.456,
			expected: "123",
		},
		{
			name:     "bool true",
			input:    true,
			expected: "true",
		},
		{
			name:     "bool false",
			input:    false,
			expected: "false",
		},
		{
			name:     "nil",
			input:    nil,
			expected: "<nil>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toString(tt.input)
			if result != tt.expected {
				t.Errorf("toString(%v) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}
