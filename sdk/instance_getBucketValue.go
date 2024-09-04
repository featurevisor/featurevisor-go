package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

// GetBucketValue generates a bucket value for the given feature and context
func (f *FeaturevisorInstance) GetBucketValue(feature types.Feature, context types.Context) types.BucketValue {
	bucketKey := f.GetBucketKey(feature, context)
	value := getBucketedNumber(string(bucketKey))

	if f.configureBucketValue != nil {
		return f.configureBucketValue(feature, context, types.BucketValue(value))
	}

	return types.BucketValue(value)
}
