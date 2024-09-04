package sdk

import (
	"strings"

	"github.com/featurevisor/featurevisor-go/types"
)

// GetBucketKey generates a bucket key for the given feature and context
func (f *FeaturevisorInstance) GetBucketKey(feature types.Feature, context types.Context) types.BucketKey {
	featureKey := feature.Key
	var attributeKeys []string
	var bucketType string

	switch bucketBy := feature.BucketBy.(type) {
	case string:
		bucketType = "plain"
		attributeKeys = []string{bucketBy}
	case []string:
		bucketType = "and"
		attributeKeys = bucketBy
	case map[string]interface{}:
		if orKeys, ok := bucketBy["or"].([]string); ok {
			bucketType = "or"
			attributeKeys = orKeys
		}
	}

	if bucketType == "" {
		f.logger.Error("invalid bucketBy", LogDetails{"featureKey": featureKey, "bucketBy": feature.BucketBy})
		return ""
	}

	var bucketKey []string

	for _, attributeKey := range attributeKeys {
		attributeValue, ok := context[attributeKey]
		if !ok {
			continue
		}

		switch bucketType {
		case "plain", "and":
			bucketKey = append(bucketKey, attributeValue.(string))
		case "or":
			if len(bucketKey) == 0 {
				bucketKey = append(bucketKey, attributeValue.(string))
			}
		}
	}

	bucketKey = append(bucketKey, featureKey)

	result := strings.Join(bucketKey, f.bucketKeySeparator)

	if f.configureBucketKey != nil {
		return f.configureBucketKey(feature, context, types.BucketKey(result))
	}

	return types.BucketKey(result)
}
