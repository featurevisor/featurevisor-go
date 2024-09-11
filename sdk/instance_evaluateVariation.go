package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

// EvaluateVariation evaluates the variation for a given feature and context
func (f *FeaturevisorInstance) EvaluateVariation(featureKey string, context types.Context) Evaluation {
	evaluation := f.EvaluateFlag(featureKey, context)

	if evaluation.Enabled == nil || !*evaluation.Enabled {
		evaluation.Reason = EvaluationReasonDisabled
		f.logger.Debug("feature is disabled", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	// Check sticky features
	if f.stickyFeatures != nil {
		if stickyFeature, ok := f.stickyFeatures[evaluation.FeatureKey]; ok {
			if stickyFeature.Variation != nil {
				evaluation.Reason = EvaluationReasonSticky
				evaluation.VariationValue = stickyFeature.Variation
				f.logger.Debug("using sticky variation", LogDetails{"evaluation": evaluation})
				return evaluation
			}
		}
	}

	// Check initial features
	if !f.IsReady() && f.initialFeatures != nil {
		if initialFeature, ok := f.initialFeatures[evaluation.FeatureKey]; ok {
			if initialFeature.Variation != nil {
				evaluation.Reason = EvaluationReasonInitial
				evaluation.VariationValue = initialFeature.Variation
				f.logger.Debug("using initial variation", LogDetails{"evaluation": evaluation})
				return evaluation
			}
		}
	}

	feature := f.GetFeature(evaluation.FeatureKey)
	if feature == nil {
		evaluation.Reason = EvaluationReasonNotFound
		f.logger.Warn("feature not found", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	if len(feature.Variations) == 0 {
		evaluation.Reason = EvaluationReasonNoVariations
		f.logger.Warn("no variations", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	finalContext := context
	if f.interceptContext != nil {
		finalContext = f.interceptContext(context)
	}

	// Check forced rules
	force, forceIndex := f.findForceFromFeature(feature, finalContext)
	if force != nil && force.Variation != nil {
		for _, variation := range feature.Variations {
			if variation.Value == *force.Variation {
				evaluation.Reason = EvaluationReasonForced
				evaluation.ForceIndex = &forceIndex
				evaluation.Force = force
				evaluation.Variation = &variation
				f.logger.Debug("forced variation found", LogDetails{"evaluation": evaluation})
				return evaluation
			}
		}
	}

	// Bucketing
	bucketValue := f.GetBucketValue(*feature, finalContext)
	evaluation.BucketValue = bucketValue

	matchedTraffic, matchedAllocation := f.getMatchedTrafficAndAllocation(feature.Traffic, finalContext, int(bucketValue))

	if matchedTraffic != nil {
		// Override from rule
		if matchedTraffic.Variation != nil {
			for _, variation := range feature.Variations {
				if variation.Value == *matchedTraffic.Variation {
					evaluation.Reason = EvaluationReasonRule
					evaluation.RuleKey = matchedTraffic.Key
					evaluation.Traffic = matchedTraffic
					evaluation.Variation = &variation
					f.logger.Debug("override from rule", LogDetails{"evaluation": evaluation})
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
					f.logger.Debug("allocated variation", LogDetails{"evaluation": evaluation})
					return evaluation
				}
			}
		}
	}

	// Nothing matched
	evaluation.Reason = EvaluationReasonNoMatch
	f.logger.Debug("no matched variation", LogDetails{"evaluation": evaluation})
	return evaluation
}
