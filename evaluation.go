package featurevisor

// EvaluationReason represents the reason for an evaluation result
type EvaluationReason string

const (
	// Feature specific
	EvaluationReasonFeatureNotFound EvaluationReason = "feature_not_found" // feature is not found in datafile
	EvaluationReasonDisabled        EvaluationReason = "disabled"          // feature is disabled
	EvaluationReasonRequired        EvaluationReason = "required"          // required features are not enabled
	EvaluationReasonOutOfRange      EvaluationReason = "out_of_range"      // out of range when mutually exclusive experiments are involved via Groups

	// Variations specific
	EvaluationReasonNoVariations      EvaluationReason = "no_variations"      // feature has no variations
	EvaluationReasonVariationDisabled EvaluationReason = "variation_disabled" // feature is disabled, and variation's disabledVariationValue is used

	// Variable specific
	EvaluationReasonVariableNotFound EvaluationReason = "variable_not_found" // variable's schema is not defined in the feature
	EvaluationReasonVariableDefault  EvaluationReason = "variable_default"   // default variable value used
	EvaluationReasonVariableDisabled EvaluationReason = "variable_disabled"  // feature is disabled, and variable's disabledValue is used
	EvaluationReasonVariableOverride EvaluationReason = "variable_override"  // variable overridden from inside a variation

	// Common
	EvaluationReasonNoMatch   EvaluationReason = "no_match"  // no rules matched
	EvaluationReasonForced    EvaluationReason = "forced"    // against a forced rule
	EvaluationReasonSticky    EvaluationReason = "sticky"    // against a sticky feature
	EvaluationReasonRule      EvaluationReason = "rule"      // against a regular rule
	EvaluationReasonAllocated EvaluationReason = "allocated" // regular allocation based on bucketing

	EvaluationReasonError EvaluationReason = "error" // error
)

// EvaluationType represents the type of evaluation
type EvaluationType string

const (
	EvaluationTypeFlag      EvaluationType = "flag"
	EvaluationTypeVariation EvaluationType = "variation"
	EvaluationTypeVariable  EvaluationType = "variable"
)

// Evaluation represents the result of an evaluation
type Evaluation struct {
	// Required
	Type       EvaluationType   `json:"type"`
	FeatureKey FeatureKey       `json:"featureKey"`
	Reason     EvaluationReason `json:"reason"`

	// Common
	BucketKey   *BucketKey        `json:"bucketKey,omitempty"`
	BucketValue *BucketValue      `json:"bucketValue,omitempty"`
	RuleKey     *RuleKey          `json:"ruleKey,omitempty"`
	Error       error             `json:"error,omitempty"`
	Enabled     *bool             `json:"enabled,omitempty"`
	Traffic     *Traffic          `json:"traffic,omitempty"`
	ForceIndex  *int              `json:"forceIndex,omitempty"`
	Force       *Force            `json:"force,omitempty"`
	Required    []Required        `json:"required,omitempty"`
	Sticky      *EvaluatedFeature `json:"sticky,omitempty"`

	// Variation
	Variation      *Variation      `json:"variation,omitempty"`
	VariationValue *VariationValue `json:"variationValue,omitempty"`

	// Variable
	VariableKey    *VariableKey    `json:"variableKey,omitempty"`
	VariableValue  VariableValue   `json:"variableValue,omitempty"`
	VariableSchema *VariableSchema `json:"variableSchema,omitempty"`
}
