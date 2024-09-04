package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

// IsEnabled checks if a feature is enabled for the given context
func (f *FeaturevisorInstance) IsEnabled(featureKey interface{}, context types.Context) bool {
	evaluation := f.EvaluateFlag(featureKey, context)
	return evaluation.Enabled != nil && *evaluation.Enabled
}
