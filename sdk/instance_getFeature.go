package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

// GetFeature returns the feature configuration for the given feature key or feature object
func (f *FeaturevisorInstance) GetFeature(featureKey interface{}) *types.Feature {
	if f.datafileReader == nil {
		return nil
	}

	switch key := featureKey.(type) {
	case string:
		return f.datafileReader.GetFeature(key)
	case types.Feature:
		return &key
	default:
		return nil
	}
}
