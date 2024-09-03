package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

type EvaluationReason string

const (
	EvaluationReasonNotFound    EvaluationReason = "not_found"
	EvaluationReasonNoVariations EvaluationReason = "no_variations"
	EvaluationReasonNoMatch     EvaluationReason = "no_match"
	EvaluationReasonDisabled    EvaluationReason = "disabled"
	EvaluationReasonRequired    EvaluationReason = "required"
	EvaluationReasonOutOfRange  EvaluationReason = "out_of_range"
	EvaluationReasonForced      EvaluationReason = "forced"
	EvaluationReasonInitial     EvaluationReason = "initial"
	EvaluationReasonSticky      EvaluationReason = "sticky"
	EvaluationReasonRule        EvaluationReason = "rule"
	EvaluationReasonAllocated   EvaluationReason = "allocated"
	EvaluationReasonDefaulted   EvaluationReason = "defaulted"
	EvaluationReasonOverride    EvaluationReason = "override"
	EvaluationReasonError       EvaluationReason = "error"
)

type Evaluation struct {
	FeatureKey    types.FeatureKey
	Reason        EvaluationReason
	BucketKey     string
	BucketValue   int
	RuleKey       types.RuleKey
	Error         error
	Enabled       *bool
	Traffic       *types.Traffic
	ForceIndex    *int
	Force         *types.Force
	Required      []types.Required
	Sticky        *types.OverrideFeature
	Initial       *types.OverrideFeature
	Variation     *types.Variation
	VariationValue types.VariationValue
	VariableKey   types.VariableKey
	VariableValue types.VariableValue
	VariableSchema *types.VariableSchema
}

func (i *FeaturevisorInstance) EvaluateFlag(featureKey types.FeatureKey, context types.Context) Evaluation {
	i.mu.RLock()
	defer i.mu.RUnlock()

	evaluation := Evaluation{
		FeatureKey: featureKey,
	}

	// Check sticky features
	if i.stickyFeatures != nil {
		if sticky, ok := i.stickyFeatures[featureKey]; ok && sticky.Enabled != nil {
			evaluation.Reason = EvaluationReasonSticky
			evaluation.Sticky = &sticky
			evaluation.Enabled = sticky.Enabled
			i.logger.Debug("using sticky enabled", LogDetails{"evaluation": evaluation})
			return evaluation
		}
	}

	// Check initial features
	if !i.statuses.Ready && i.initialFeatures != nil {
		if initial, ok := i.initialFeatures[featureKey]; ok && initial.Enabled != nil {
			evaluation.Reason = EvaluationReasonInitial
			evaluation.Initial = &initial
			evaluation.Enabled = initial.Enabled
			i.logger.Debug("using initial enabled", LogDetails{"evaluation": evaluation})
			return evaluation
		}
	}

	feature := i.datafileReader.GetFeature(featureKey)
	if feature == nil {
		evaluation.Reason = EvaluationReasonNotFound
		i.logger.Warn("feature not found", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	if feature.Deprecated != nil && *feature.Deprecated {
		i.logger.Warn("feature is deprecated", LogDetails{"featureKey": featureKey})
	}

	finalContext := context
	if i.interceptContext != nil {
		finalContext = i.interceptContext(context)
	}

	// Check forced rules
	force, forceIndex := findForceFromFeature(feature, finalContext, i.datafileReader, i.logger)
	if force != nil && force.Enabled != nil {
		evaluation.Reason = EvaluationReasonForced
		evaluation.ForceIndex = &forceIndex
		evaluation.Force = force
		evaluation.Enabled = force.Enabled
		i.logger.Debug("forced enabled found", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	// Check required features
	if len(feature.Required) > 0 {
		requiredFeaturesAreEnabled := true
		for _, required := range feature.Required {
			var requiredKey types.FeatureKey
			var requiredVariation types.VariationValue

			switch r := required.(type) {
			case string:
				requiredKey = types.FeatureKey(r)
			case types.RequiredWithVariation:
				requiredKey = r.Key
				requiredVariation = r.Variation
			}

			requiredIsEnabled := i.IsEnabled(requiredKey, finalContext)
			if !requiredIsEnabled {
				requiredFeaturesAreEnabled = false
				break
			}

			if requiredVariation != "" {
				requiredVariationValue := i.GetVariation(requiredKey, finalContext)
				if requiredVariationValue != requiredVariation {
					requiredFeaturesAreEnabled = false
					break
				}
			}
		}

		if !requiredFeaturesAreEnabled {
			evaluation.Reason = EvaluationReasonRequired
			evaluation.Required = feature.Required
			evaluation.Enabled = &requiredFeaturesAreEnabled
			i.logger.Debug("required features not enabled", LogDetails{"evaluation": evaluation})
			return evaluation
		}
	}

	// Bucketing
	bucketKey, bucketValue := i.getBucketKeyAndValue(feature, finalContext)
	evaluation.BucketKey = bucketKey
	evaluation.BucketValue = bucketValue

	matchedTraffic := getMatchedTraffic(feature.Traffic, finalContext, i.datafileReader, i.logger)

	if matchedTraffic != nil {
		// Check if mutually exclusive
		if len(feature.Ranges) > 0 {
			for _, r := range feature.Ranges {
				if bucketValue >= int(r[0]) && bucketValue < int(r[1]) {
					enabled := true
					if matchedTraffic.Enabled != nil {
						enabled = *matchedTraffic.Enabled
					}
					evaluation.Reason = EvaluationReasonAllocated
					evaluation.RuleKey = types.RuleKey(matchedTraffic.Key)
					evaluation.Traffic = matchedTraffic
					evaluation.Enabled = &enabled
					i.logger.Debug("matched", LogDetails{"evaluation": evaluation})
					return evaluation
				}
			}

			evaluation.Reason = EvaluationReasonOutOfRange
			evaluation.Enabled = new(bool) // false
			i.logger.Debug("not matched", LogDetails{"evaluation": evaluation})
			return evaluation
		}

		// Override from rule
		if matchedTraffic.Enabled != nil {
			evaluation.Reason = EvaluationReasonOverride
			evaluation.RuleKey = types.RuleKey(matchedTraffic.Key)
			evaluation.Traffic = matchedTraffic
			evaluation.Enabled = matchedTraffic.Enabled
			i.logger.Debug("override from rule", LogDetails{"evaluation": evaluation})
			return evaluation
		}

		// Treated as enabled because of matched traffic
		if bucketValue <= int(matchedTraffic.Percentage) {
			evaluation.Reason = EvaluationReasonRule
			evaluation.RuleKey = matchedTraffic.Key
			evaluation.Traffic = matchedTraffic
			evaluation.Enabled = new(bool) // true
			i.logger.Debug("matched traffic", LogDetails{"evaluation": evaluation})
			return evaluation
		}
	}

	// Nothing matched
	evaluation.Reason = EvaluationReasonNoMatch
	evaluation.Enabled = new(bool) // false
	i.logger.Debug("nothing matched", LogDetails{"evaluation": evaluation})
	return evaluation
}

func (i *FeaturevisorInstance) IsEnabled(featureKey types.FeatureKey, context types.Context) bool {
	evaluation := i.EvaluateFlag(featureKey, context)
	return evaluation.Enabled != nil && *evaluation.Enabled
}
