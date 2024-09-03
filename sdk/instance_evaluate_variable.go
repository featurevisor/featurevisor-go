package sdk

import (
	"encoding/json"
	"github.com/featurevisor/featurevisor-go/types"
)

func (i *FeaturevisorInstance) EvaluateVariable(featureKey types.FeatureKey, variableKey types.VariableKey, context types.Context) Evaluation {
	i.mu.RLock()
	defer i.mu.RUnlock()

	evaluation := Evaluation{
		FeatureKey:  featureKey,
		VariableKey: variableKey,
	}

	flag := i.EvaluateFlag(featureKey, context)
	if flag.Enabled != nil && !*flag.Enabled {
		evaluation.Reason = EvaluationReasonDisabled
		i.logger.Debug("feature is disabled", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	// Check sticky features
	if i.stickyFeatures != nil {
		if sticky, ok := i.stickyFeatures[featureKey]; ok && sticky.Variables != nil {
			if value, ok := sticky.Variables[variableKey]; ok {
				evaluation.Reason = EvaluationReasonSticky
				evaluation.VariableValue = value
				i.logger.Debug("using sticky variable", LogDetails{"evaluation": evaluation})
				return evaluation
			}
		}
	}

	// Check initial features
	if !i.statuses.Ready && i.initialFeatures != nil {
		if initial, ok := i.initialFeatures[featureKey]; ok && initial.Variables != nil {
			if value, ok := initial.Variables[variableKey]; ok {
				evaluation.Reason = EvaluationReasonInitial
				evaluation.VariableValue = value
				i.logger.Debug("using initial variable", LogDetails{"evaluation": evaluation})
				return evaluation
			}
		}
	}

	feature := i.datafileReader.GetFeature(featureKey)
	if feature == nil {
		evaluation.Reason = EvaluationReasonNotFound
		i.logger.Warn("feature not found in datafile", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	variableSchema := findVariableSchema(feature.VariablesSchema, variableKey)
	if variableSchema == nil {
		evaluation.Reason = EvaluationReasonNotFound
		i.logger.Warn("variable schema not found", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	evaluation.VariableSchema = variableSchema

	finalContext := context
	if i.interceptContext != nil {
		finalContext = i.interceptContext(context)
	}

	// Check forced rules
	force, forceIndex := findForceFromFeature(feature, finalContext, i.datafileReader, i.logger)
	if force != nil && force.Variables != nil {
		if value, ok := force.Variables[variableKey]; ok {
			evaluation.Reason = EvaluationReasonForced
			evaluation.ForceIndex = &forceIndex
			evaluation.Force = force
			evaluation.VariableValue = value
			i.logger.Debug("forced variable", LogDetails{"evaluation": evaluation})
			return evaluation
		}
	}

	// Bucketing
	bucketKey, bucketValue := i.getBucketKeyAndValue(feature, finalContext)
	evaluation.BucketKey = bucketKey
	evaluation.BucketValue = bucketValue

	matchedTraffic, matchedAllocation := getMatchedTrafficAndAllocation(feature.Traffic, finalContext, bucketValue, i.datafileReader, i.logger)

	if matchedTraffic != nil {
		// Override from rule
		if matchedTraffic.Variables != nil {
			if value, ok := matchedTraffic.Variables[variableKey]; ok {
				evaluation.Reason = EvaluationReasonRule
				evaluation.RuleKey = matchedTraffic.Key
				evaluation.Traffic = matchedTraffic
				evaluation.VariableValue = value
				i.logger.Debug("override from rule", LogDetails{"evaluation": evaluation})
				return evaluation
			}
		}

		// Regular allocation
		var variationValue types.VariationValue
		if force != nil && force.Variation != nil {
			variationValue = *force.Variation
		} else if matchedAllocation != nil {
			variationValue = matchedAllocation.Variation
		}

		if variationValue != "" && len(feature.Variations) > 0 {
			for _, variation := range feature.Variations {
				if variation.Value == variationValue {
					for _, variable := range variation.Variables {
						if variable.Key == variableKey {
							if len(variable.Overrides) > 0 {
								for _, override := range variable.Overrides {
									if override.Conditions != nil {
										conditions := parseConditions(override.Conditions)
										if allConditionsAreMatched(conditions, finalContext, i.logger) {
											evaluation.Reason = EvaluationReasonOverride
											evaluation.RuleKey = matchedTraffic.Key
											evaluation.Traffic = matchedTraffic
											evaluation.VariableValue = override.Value
											i.logger.Debug("variable override", LogDetails{"evaluation": evaluation})
											return evaluation
										}
									}
									if override.Segments != nil {
										segments := parseSegments(override.Segments)
										if allGroupSegmentsAreMatched(segments, finalContext, i.datafileReader, i.logger) {
											evaluation.Reason = EvaluationReasonOverride
											evaluation.RuleKey = matchedTraffic.Key
											evaluation.Traffic = matchedTraffic
											evaluation.VariableValue = override.Value
											i.logger.Debug("variable override", LogDetails{"evaluation": evaluation})
											return evaluation
										}
									}
								}
							}
							evaluation.Reason = EvaluationReasonAllocated
							evaluation.RuleKey = matchedTraffic.Key
							evaluation.Traffic = matchedTraffic
							evaluation.VariableValue = variable.Value
							i.logger.Debug("allocated variable", LogDetails{"evaluation": evaluation})
							return evaluation
						}
					}
				}
			}
		}
	}

	// Fall back to default
	evaluation.Reason = EvaluationReasonDefaulted
	evaluation.VariableValue = variableSchema.DefaultValue
	i.logger.Debug("using default value", LogDetails{"evaluation": evaluation})
	return evaluation
}

func (i *FeaturevisorInstance) GetVariable(featureKey types.FeatureKey, variableKey types.VariableKey, context types.Context) types.VariableValue {
	evaluation := i.EvaluateVariable(featureKey, variableKey, context)
	if evaluation.VariableValue != nil {
		if evaluation.VariableSchema != nil && evaluation.VariableSchema.Type == "json" {
			if strValue, ok := evaluation.VariableValue.(string); ok {
				var jsonValue interface{}
				err := json.Unmarshal([]byte(strValue), &jsonValue)
				if err == nil {
					return jsonValue
				}
			}
		}
		return evaluation.VariableValue
	}
	return nil
}
