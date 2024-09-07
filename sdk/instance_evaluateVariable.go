package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

// EvaluateVariable evaluates a variable for a given feature, variable key, and context
func (f *FeaturevisorInstance) EvaluateVariable(featureKey string, variableKey types.VariableKey, context types.Context) Evaluation {
	flagEvaluation := f.EvaluateFlag(featureKey, context)

	evaluation := Evaluation{
		FeatureKey:  flagEvaluation.FeatureKey,
		VariableKey: &variableKey,
	}

	if flagEvaluation.Enabled == nil || !*flagEvaluation.Enabled {
		evaluation.Reason = EvaluationReasonDisabled
		f.logger.Debug("feature is disabled", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	// Check sticky features
	if f.stickyFeatures != nil {
		if stickyFeature, ok := f.stickyFeatures[evaluation.FeatureKey]; ok {
			if stickyFeature.Variables != nil {
				if value, exists := stickyFeature.Variables[variableKey]; exists {
					evaluation.Reason = EvaluationReasonSticky
					evaluation.VariableValue = value
					f.logger.Debug("using sticky variable", LogDetails{"evaluation": evaluation})
					return evaluation
				}
			}
		}
	}

	// Check initial features
	if !f.IsReady() && f.initialFeatures != nil {
		if initialFeature, ok := f.initialFeatures[evaluation.FeatureKey]; ok {
			if initialFeature.Variables != nil {
				if value, exists := initialFeature.Variables[variableKey]; exists {
					evaluation.Reason = EvaluationReasonInitial
					evaluation.VariableValue = value
					f.logger.Debug("using initial variable", LogDetails{"evaluation": evaluation})
					return evaluation
				}
			}
		}
	}

	feature := f.GetFeature(evaluation.FeatureKey)
	if feature == nil {
		evaluation.Reason = EvaluationReasonNotFound
		f.logger.Warn("feature not found", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	variableSchema := f.findVariableSchema(feature, variableKey)
	if variableSchema == nil {
		evaluation.Reason = EvaluationReasonNotFound
		f.logger.Warn("variable schema not found", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	evaluation.VariableSchema = variableSchema

	finalContext := context
	if f.interceptContext != nil {
		finalContext = f.interceptContext(context)
	}

	// Check forced rules
	force, forceIndex := f.findForceFromFeature(feature, finalContext)
	if force != nil && force.Variables != nil {
		if value, exists := force.Variables[string(variableKey)]; exists {
			evaluation.Reason = EvaluationReasonForced
			evaluation.ForceIndex = &forceIndex
			evaluation.Force = force
			evaluation.VariableValue = value
			f.logger.Debug("forced variable", LogDetails{"evaluation": evaluation})
			return evaluation
		}
	}

	// Bucketing
	bucketValue := f.GetBucketValue(*feature, finalContext)
	evaluation.BucketValue = bucketValue

	matchedTraffic, matchedAllocation := f.getMatchedTrafficAndAllocation(feature.Traffic, finalContext, int(bucketValue))

	if matchedTraffic != nil {
		// Override from rule
		if matchedTraffic.Variables != nil {
			if value, exists := matchedTraffic.Variables[string(variableKey)]; exists {
				evaluation.Reason = EvaluationReasonRule
				evaluation.RuleKey = types.RuleKey(matchedTraffic.Key)
				evaluation.Traffic = matchedTraffic
				evaluation.VariableValue = value
				f.logger.Debug("override from rule", LogDetails{"evaluation": evaluation})
				return evaluation
			}
		}
		// Regular allocation
		if matchedAllocation != nil {
			for _, variation := range feature.Variations {
				if variation.Value == matchedAllocation.Variation {
					if variation.Variables != nil {
						for _, v := range variation.Variables {
							if v.Key == variableKey {
								evaluation.Reason = EvaluationReasonAllocated
								evaluation.RuleKey = types.RuleKey(matchedTraffic.Key)
								evaluation.Traffic = matchedTraffic
								evaluation.VariableValue = v.Value
								f.logger.Debug("allocated variable", LogDetails{"evaluation": evaluation})
								return evaluation
							}
						}
					}
					break
				}
			}
		}
	}

	// Fall back to default
	evaluation.Reason = EvaluationReasonDefaulted
	evaluation.VariableValue = variableSchema.DefaultValue
	f.logger.Debug("using default value", LogDetails{"evaluation": evaluation})
	return evaluation
}

func (f *FeaturevisorInstance) findVariableSchema(feature *types.Feature, variableKey types.VariableKey) *types.VariableSchema {
	for _, schema := range feature.VariablesSchema {
		if schema.Key == variableKey {
			return &schema
		}
	}
	return nil
}
