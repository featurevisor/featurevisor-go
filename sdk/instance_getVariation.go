package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

// GetVariation returns the variation value for a given feature and context
func (f *FeaturevisorInstance) GetVariation(featureKey string, context types.Context) *types.VariationValue {
	evaluation := f.EvaluateVariation(featureKey, context)

	if evaluation.VariationValue != nil {
		return evaluation.VariationValue
	}

	if evaluation.Variation != nil {
		return &evaluation.Variation.Value
	}

	return nil
}
