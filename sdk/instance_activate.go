package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

// Activate activates a feature for the given context and returns the variation value
func (f *FeaturevisorInstance) Activate(featureKey string, context types.Context) *types.VariationValue {
	evaluation := f.EvaluateVariation(featureKey, context)
	variationValue := f.GetVariation(featureKey, context)

	if variationValue != nil {
		finalContext := context
		if f.interceptContext != nil {
			finalContext = f.interceptContext(context)
		}

		captureContext := make(types.Context)
		attributes := f.datafileReader.GetAllAttributes()
		for _, attr := range attributes {
			if attr.Capture != nil && *attr.Capture {
				if value, ok := finalContext[attr.Key]; ok {
					captureContext[attr.Key] = value
				}
			}
		}

		f.emitter.Emit(
			EventActivation,
			string(evaluation.FeatureKey),
			*variationValue,
			finalContext,
			captureContext,
			evaluation,
		)

		return variationValue
	}

	return nil
}
