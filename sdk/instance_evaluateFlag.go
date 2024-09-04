package sdk

import (
	"fmt"
	"github.com/featurevisor/featurevisor-go/types"
)

// EvaluationReason represents the reason for a feature flag evaluation result
type EvaluationReason string

const (
	EvaluationReasonNotFound      EvaluationReason = "not_found"
	EvaluationReasonNoVariations  EvaluationReason = "no_variations"
	EvaluationReasonNoMatch       EvaluationReason = "no_match"
	EvaluationReasonDisabled      EvaluationReason = "disabled"
	EvaluationReasonRequired      EvaluationReason = "required"
	EvaluationReasonOutOfRange    EvaluationReason = "out_of_range"
	EvaluationReasonForced        EvaluationReason = "forced"
	EvaluationReasonInitial       EvaluationReason = "initial"
	EvaluationReasonSticky        EvaluationReason = "sticky"
	EvaluationReasonRule          EvaluationReason = "rule"
	EvaluationReasonAllocated     EvaluationReason = "allocated"
	EvaluationReasonDefaulted     EvaluationReason = "defaulted"
	EvaluationReasonOverride      EvaluationReason = "override"
	EvaluationReasonError         EvaluationReason = "error"
)

// Evaluation represents the result of a feature flag evaluation
type Evaluation struct {
	FeatureKey   types.FeatureKey
	Reason       EvaluationReason
	BucketKey    types.BucketKey
	BucketValue  types.BucketValue
	RuleKey      types.RuleKey
	Error        error
	Enabled      *bool
	Traffic      *types.Traffic
	ForceIndex   *int
	Force        *types.Force
	Required     []types.Required
	Sticky       *types.OverrideFeature
	Initial      *types.OverrideFeature
	Variation    *types.Variation
	VariationValue *types.VariationValue
	VariableKey  *types.VariableKey
	VariableValue interface{}
	VariableSchema *types.VariableSchema
}

// EvaluateFlag evaluates a feature flag for the given context
func (f *FeaturevisorInstance) EvaluateFlag(featureKey interface{}, context types.Context) Evaluation {
	var evaluation Evaluation
	var feature *types.Feature

	switch key := featureKey.(type) {
	case string:
		feature = f.GetFeature(key)
		evaluation.FeatureKey = types.FeatureKey(key)
	case types.Feature:
		feature = &key
		evaluation.FeatureKey = key.Key
	default:
		evaluation.Reason = EvaluationReasonError
		evaluation.Error = fmt.Errorf("invalid feature key type")
		return evaluation
	}

	if feature == nil {
		evaluation.Reason = EvaluationReasonNotFound
		f.logger.Warn("feature not found", LogDetails{"featureKey": evaluation.FeatureKey})
		return evaluation
	}

	if feature.Deprecated {
		f.logger.Warn("feature is deprecated", LogDetails{"featureKey": feature.Key})
	}

	finalContext := context
	if f.interceptContext != nil {
		finalContext = f.interceptContext(context)
	}

	// Check sticky features
	if f.stickyFeatures != nil {
		if stickyFeature, ok := f.stickyFeatures[string(evaluation.FeatureKey)]; ok && stickyFeature.Enabled != nil {
			evaluation.Reason = EvaluationReasonSticky
			evaluation.Sticky = &stickyFeature
			evaluation.Enabled = stickyFeature.Enabled
			f.logger.Debug("using sticky enabled", LogDetails{"evaluation": evaluation})
			return evaluation
		}
	}

	// Check initial features
	if !f.IsReady() && f.initialFeatures != nil {
		if initialFeature, ok := f.initialFeatures[string(evaluation.FeatureKey)]; ok {
			if initialFeature.Enabled != nil {
				evaluation.Reason = EvaluationReasonInitial
				evaluation.Initial = &initialFeature
				evaluation.Enabled = initialFeature.Enabled
				f.logger.Debug("using initial enabled", LogDetails{"evaluation": evaluation})
				return evaluation
			}
		}
	}

	// Check forced rules
	force, forceIndex := f.findForceFromFeature(feature, finalContext)
	if force != nil && force.Enabled != nil {
		evaluation.Reason = EvaluationReasonForced
		evaluation.ForceIndex = &forceIndex
		evaluation.Force = force
		evaluation.Enabled = force.Enabled
		f.logger.Debug("forced enabled found", LogDetails{"evaluation": evaluation})
		return evaluation
	}

	// Check required features
	if len(feature.Required) > 0 {
		requiredFeaturesAreEnabled := true
		for _, required := range feature.Required {
			var requiredKey string
			var requiredVariation *string

			if requiredStr, ok := required.(string); ok {
				requiredKey = requiredStr
			} else if requiredObj, ok := required.(map[string]interface{}); ok {
				requiredKey = requiredObj["key"].(string)
				if v, ok := requiredObj["variation"]; ok {
					variationStr := v.(string)
					requiredVariation = &variationStr
				}
			}

			requiredIsEnabled := f.IsEnabled(requiredKey, finalContext)
			if !requiredIsEnabled {
				requiredFeaturesAreEnabled = false
				break
			}

			if requiredVariation != nil {
				requiredVariationValue := f.GetVariation(requiredKey, finalContext)
				if requiredVariationValue != *requiredVariation {
					requiredFeaturesAreEnabled = false
					break
				}
			}
		}

		if !requiredFeaturesAreEnabled {
			evaluation.Reason = EvaluationReasonRequired
			evaluation.Required = feature.Required
			evaluation.Enabled = new(bool)
			*evaluation.Enabled = false
			f.logger.Debug("required features not enabled", LogDetails{"evaluation": evaluation})
			return evaluation
		}
	}

	// Bucketing
	bucketKey := f.GetBucketKey(*feature, finalContext)
	bucketValue := f.GetBucketValue(*feature, finalContext)
	evaluation.BucketKey = bucketKey
	evaluation.BucketValue = bucketValue

	matchedTraffic := f.getMatchedTraffic(feature.Traffic, finalContext)

	if matchedTraffic != nil {
		// Check if mutually exclusive
		if len(feature.Ranges) > 0 {
			matchedRange := false
			for _, r := range feature.Ranges {
				if int(bucketValue) >= r[0] && int(bucketValue) < r[1] {
					matchedRange = true
					break
				}
			}

			if matchedRange {
				evaluation.Reason = EvaluationReasonAllocated
				evaluation.RuleKey = matchedTraffic.Key
				evaluation.Traffic = matchedTraffic
				if matchedTraffic.Enabled != nil {
					evaluation.Enabled = matchedTraffic.Enabled
				} else {
					evaluation.Enabled = new(bool)
					*evaluation.Enabled = true
				}
				f.logger.Debug("matched", LogDetails{"evaluation": evaluation})
				return evaluation
			}

			evaluation.Reason = EvaluationReasonOutOfRange
			evaluation.Enabled = new(bool)
			*evaluation.Enabled = false
			f.logger.Debug("not matched", LogDetails{"evaluation": evaluation})
			return evaluation
		}

		// Override from rule
		if matchedTraffic.Enabled != nil {
			evaluation.Reason = EvaluationReasonOverride
			evaluation.RuleKey = matchedTraffic.Key
			evaluation.Traffic = matchedTraffic
			evaluation.Enabled = matchedTraffic.Enabled
			f.logger.Debug("override from rule", LogDetails{"evaluation": evaluation})
			return evaluation
		}

		// Treated as enabled because of matched traffic
		if int(bucketValue) <= matchedTraffic.Percentage {
			evaluation.Reason = EvaluationReasonRule
			evaluation.RuleKey = matchedTraffic.Key
			evaluation.Traffic = matchedTraffic
			evaluation.Enabled = new(bool)
			*evaluation.Enabled = true
			f.logger.Debug("matched traffic", LogDetails{"evaluation": evaluation})
			return evaluation
		}
	}

	// Nothing matched
	evaluation.Reason = EvaluationReasonNoMatch
	evaluation.Enabled = new(bool)
	*evaluation.Enabled = false
	f.logger.Debug("nothing matched", LogDetails{"evaluation": evaluation})
	return evaluation
}
