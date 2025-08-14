package sdk

import (
	"fmt"
	"strings"
)

const (
	HASH_SEED           = 1
	MAX_HASH_VALUE      = 1 << 32
	MAX_BUCKETED_NUMBER = 100000 // 100% * 1000 to include three decimal places in the same integer value
)

// BucketKey represents a bucket key string
type BucketKey = string

// BucketValue represents a bucket value (0 to 100,000)
type BucketValue = int

// GetBucketKeyOptions contains options for getting a bucket key
type GetBucketKeyOptions struct {
	FeatureKey FeatureKey
	BucketBy   BucketBy
	Context    Context
	Logger     *Logger
}

// DEFAULT_BUCKET_KEY_SEPARATOR is the default separator for bucket keys
const DEFAULT_BUCKET_KEY_SEPARATOR = "."

// GetBucketedNumber returns a bucketed number for a given bucket key
func GetBucketedNumber(bucketKey string) BucketValue {
	hashValue := MurmurHashV3(bucketKey, HASH_SEED)
	ratio := float64(hashValue) / float64(MAX_HASH_VALUE)

	return int(ratio * float64(MAX_BUCKETED_NUMBER))
}

// GetBucketKey returns a bucket key based on the feature key, bucket by configuration, and context
func GetBucketKey(options GetBucketKeyOptions) BucketKey {
	featureKey := options.FeatureKey
	bucketBy := options.BucketBy
	context := options.Context
	logger := options.Logger

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
		logger.Error("invalid bucketBy", LogDetails{
			"featureKey": featureKey,
			"bucketBy":   bucketBy,
		})
		panic("invalid bucketBy")
	}

	bucketKey := make([]interface{}, 0)

	// Process each attribute key
	for _, attributeKey := range attributeKeys {
		attributeValue := GetValueFromContext(context, attributeKey)

		if attributeValue == nil {
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

	result := strings.Join(bucketKeyStrings, DEFAULT_BUCKET_KEY_SEPARATOR)

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
		return fmt.Sprintf("%.0f", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		// For other types, try to convert to string
		return fmt.Sprintf("%v", v)
	}
}
