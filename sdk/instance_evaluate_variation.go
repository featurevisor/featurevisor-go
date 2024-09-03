package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

func (i *FeaturevisorInstance) EvaluateVariation(featureKey types.FeatureKey, context types.Context) Evaluation {
	i.mu.RLock()
	defer i.mu.RUnlock()

	evaluation := Evaluation{
		FeatureKey: featureKey,
	}

	flag := i.EvaluateFlag(featureKey, context)
	if flag.Enabled != nil && !*flag.Enabled {
		evaluation.Reason = EvaluationReasonDisabled
		i.logger.Debug("feature is disabled", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	// Check sticky features
	if i.stickyFeatures != nil {
		if sticky, ok := i.stickyFeatures[featureKey]; ok && sticky.Variation != nil {
			evaluation.Reason = EvaluationReasonSticky
			evaluation.VariationValue = *sticky.Variation
			i.logger.Debug("using sticky variation", LogDetails{"evaluation": evaluation})
			return evaluation
		}
	}

	// Check initial features
	if !i.statuses.Ready && i.initialFeatures != nil {
		if initial, ok := i.initialFeatures[featureKey]; ok && initial.Variation != nil {
			evaluation.Reason = EvaluationReasonInitial
			evaluation.VariationValue = *initial.Variation
			i.logger.Debug("using initial variation", LogDetails{"evaluation": evaluation})
			return evaluation
		}
	}

	feature := i.datafileReader.GetFeature(featureKey)
	if feature == nil {
		evaluation.Reason = EvaluationReasonNotFound
		i.logger.Warn("feature not found", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	if len(feature.Variations) == 0 {
		evaluation.Reason = EvaluationReasonNoVariations
		i.logger.Warn("no variations", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	finalContext := context
	if i.interceptContext != nil {
		finalContext = i.interceptContext(context)
	}

	// Check forced rules
	force, forceIndex := findForceFromFeature(feature, finalContext, i.datafileReader, i.logger)
	if force != nil && force.Variation != nil {
		for _, variation := range feature.Variations {
			if variation.Value == *force.Variation {
				evaluation.Reason = EvaluationReasonForced
				evaluation.ForceIndex = &forceIndex
				evaluation.Force = force
				evaluation.Variation = &variation
				i.logger.Debug("forced variation found", LogDetails{"evaluation": evaluation})
				return evaluation
			}
		}
	}

	// Bucketing
	bucketKey, bucketValue := i.getBucketKeyAndValue(feature, finalContext)
	evaluation.BucketKey = bucketKey
	evaluation.BucketValue = bucketValue

	matchedTraffic, matchedAllocation := getMatchedTrafficAndAllocation(feature.Traffic, finalContext, bucketValue, i.datafileReader, i.logger)

	if matchedTraffic != nil {
		// Override from rule
		if matchedTraffic.Variation != nil {
			for _, variation := range feature.Variations {
				if variation.Value == *matchedTraffic.Variation {
					evaluation.Reason = EvaluationReasonRule
					evaluation.RuleKey = matchedTraffic.Key
					evaluation.Traffic = matchedTraffic
					evaluation.Variation = &variation
					i.logger.Debug("override from rule", LogDetails{"evaluation": evaluation})
					return evaluation
				}
			}
		}

		// Regular allocation
		if matchedAllocation != nil {
			for _, variation := range feature.Variations {
				if variation.Value == matchedAllocation.Variation {
					evaluation.Reason = EvaluationReasonAllocated
					evaluation.RuleKey = matchedTraffic.Key
					evaluation.Traffic = matchedTraffic
					evaluation.Variation = &variation
					i.logger.Debug("allocated variation", LogDetails{"evaluation": evaluation})
					return evaluation
				}
			}
		}
	}

	// Nothing matched
	evaluation.Reason = EvaluationReasonNoMatch
	i.logger.Debug("no matched variation", LogDetails{"evaluation": evaluation})
	return evaluation
}

func (i *FeaturevisorInstance) GetVariation(featureKey types.FeatureKey, context types.Context) types.VariationValue {
	evaluation := i.EvaluateVariation(featureKey, context)
	if evaluation.VariationValue != "" {
		return evaluation.VariationValue
	}
	if evaluation.Variation != nil {
		return evaluation.Variation.Value
	}
	return ""
}
