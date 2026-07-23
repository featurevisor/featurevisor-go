package featurevisor

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

const (
	hashSeed            = 1
	maxHashValue        = 1 << 32
	MAX_BUCKETED_NUMBER = 100000 // 100% * 1000 to include three decimal places in the same integer value
)

// BucketKey represents a bucket key string
type BucketKey = string

// BucketValue represents a bucket value (0 to 100,000)
type BucketValue = int

// getBucketKeyOptions contains options for getting a bucket key
type getBucketKeyOptions struct {
	FeatureKey         FeatureKey
	BucketBy           BucketBy
	Context            Context
	diagnosticReporter *diagnosticReporter
}

// defaultBucketKeySeparator is the default separator for bucket keys
const defaultBucketKeySeparator = "."

// getBucketedNumber returns a bucketed number for a given bucket key
func getBucketedNumber(bucketKey string) BucketValue {
	hashValue := murmurHashV3(bucketKey, hashSeed)
	ratio := float64(hashValue) / float64(maxHashValue)

	return int(ratio * float64(MAX_BUCKETED_NUMBER))
}

// getBucketKey returns a bucket key based on the feature key, bucket by configuration, and context
func getBucketKey(options getBucketKeyOptions) BucketKey {
	featureKey := options.FeatureKey
	bucketBy := options.BucketBy
	context := options.Context
	diagnostics := options.diagnosticReporter

	var bucketType string
	var attributeKeys []string

	// Determine bucket type and extract attribute keys
	switch b := bucketBy.(type) {
	case string:
		bucketType = "plain"
		attributeKeys = []string{b}
	case []string:
		bucketType = "and"
		attributeKeys = b
	case OrBucketBy:
		bucketType = "or"
		attributeKeys = b.Or
	case map[string]interface{}:
		// Handle JSON unmarshaled bucketBy
		if orValue, exists := b["or"]; exists {
			bucketType = "or"
			if orArray, ok := orValue.([]string); ok {
				attributeKeys = orArray
			} else if orArray, ok := orValue.([]interface{}); ok {
				attributeKeys = make([]string, len(orArray))
				for i, v := range orArray {
					if str, ok := v.(string); ok {
						attributeKeys[i] = str
					}
				}
			}
		} else {
			// This is a plain string case that was unmarshaled as map
			bucketType = "plain"
			// Try to extract the single key
			for key := range b {
				attributeKeys = []string{key}
				break
			}
		}
	case []interface{}:
		// Handle JSON unmarshaled array
		bucketType = "and"
		attributeKeys = make([]string, len(b))
		for i, v := range b {
			if str, ok := v.(string); ok {
				attributeKeys[i] = str
			}
		}
	default:
		diagnostics.Error("invalid bucketBy", logDetails{
			"featureKey": featureKey,
			"bucketBy":   bucketBy,
		})
		panic("invalid bucketBy")
	}

	bucketKey := make([]interface{}, 0)

	// Process each attribute key
	for _, attributeKey := range attributeKeys {
		attributeValue := getValueFromContext(context, attributeKey)

		if attributeValue == nil && !pathExists(context, attributeKey) {
			continue
		}

		if bucketType == "plain" || bucketType == "and" {
			bucketKey = append(bucketKey, attributeValue)
		} else {
			// or - take the first available value
			if len(bucketKey) == 0 {
				bucketKey = append(bucketKey, attributeValue)
			}
		}
	}

	// Always append the feature key
	bucketKey = append(bucketKey, featureKey)

	// Convert bucket key elements to strings and join
	bucketKeyStrings := make([]string, len(bucketKey))
	for i, value := range bucketKey {
		bucketKeyStrings[i] = toString(value)
	}

	result := strings.Join(bucketKeyStrings, defaultBucketKeySeparator)

	return result
}

// toString converts a value to string representation
func toString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case int:
		return fmt.Sprintf("%d", v)
	case float64:
		return javascriptFloat(v, 64)
	case float32:
		return javascriptFloat(float64(v), 32)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		valueOf := reflect.ValueOf(value)
		if valueOf.IsValid() && (valueOf.Kind() == reflect.Slice || valueOf.Kind() == reflect.Array) {
			parts := make([]string, valueOf.Len())
			for index := 0; index < valueOf.Len(); index++ {
				parts[index] = toString(valueOf.Index(index).Interface())
			}
			return strings.Join(parts, ",")
		}
		if valueOf.IsValid() && valueOf.Kind() == reflect.Map {
			return "[object Object]"
		}
		return fmt.Sprintf("%v", v)
	}
}

func javascriptFloat(value float64, bitSize int) string {
	if math.IsNaN(value) {
		return "NaN"
	}
	if math.IsInf(value, 1) {
		return "Infinity"
	}
	if math.IsInf(value, -1) {
		return "-Infinity"
	}
	if value == 0 {
		return "0"
	}

	absolute := math.Abs(value)
	format := byte('f')
	if absolute >= 1e21 || absolute < 1e-6 {
		format = 'g'
	}
	result := strconv.FormatFloat(value, format, -1, bitSize)
	if exponentIndex := strings.IndexByte(result, 'e'); exponentIndex != -1 {
		exponent, _ := strconv.Atoi(result[exponentIndex+1:])
		return fmt.Sprintf("%se%+d", result[:exponentIndex], exponent)
	}
	return result
}
