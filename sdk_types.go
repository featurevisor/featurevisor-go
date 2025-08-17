package featurevisor

import (
	"encoding/json"
	"fmt"
)

/**
 * Attribute
 */

// AttributeKey represents the key of an attribute
type AttributeKey = string

// AttributeValue represents the value of an attribute
type AttributeValue interface{}

// AttributeObjectValue represents an object with attribute key-value pairs
type AttributeObjectValue map[AttributeKey]AttributeValue

// AttributeType represents the type of an attribute
type AttributeType string

const (
	AttributeTypeBoolean AttributeType = "boolean"
	AttributeTypeString  AttributeType = "string"
	AttributeTypeInteger AttributeType = "integer"
	AttributeTypeDouble  AttributeType = "double"
	AttributeTypeDate    AttributeType = "date"
	AttributeTypeSemver  AttributeType = "semver"
	AttributeTypeObject  AttributeType = "object"
	AttributeTypeArray   AttributeType = "array"
)

// AttributeProperty represents a property of an attribute
type AttributeProperty struct {
	Type        AttributeType `json:"type"`
	Description *string       `json:"description,omitempty"`
}

// Attribute represents an attribute definition
type Attribute struct {
	Archived    *bool                              `json:"archived,omitempty"`
	Key         *AttributeKey                      `json:"key,omitempty"`
	Type        AttributeType                      `json:"type"`
	Description *string                            `json:"description,omitempty"`
	Properties  map[AttributeKey]AttributeProperty `json:"properties,omitempty"`
}

/**
 * Bucket
 */
// BucketBy represents how to bucket users
type BucketBy interface{}

// PlainBucketBy represents a simple bucket by attribute key
type PlainBucketBy = AttributeKey

// AndBucketBy represents an AND condition for bucketing
type AndBucketBy = []AttributeKey

// OrBucketBy represents an OR condition for bucketing
type OrBucketBy struct {
	Or []AttributeKey `json:"or"`
}

/**
 * Condition
 */
// Condition represents a condition for segment matching
type Condition interface{}

// Operator represents the comparison operator
type Operator string

const (
	OperatorEquals                    Operator = "equals"
	OperatorNotEquals                 Operator = "notEquals"
	OperatorExists                    Operator = "exists"
	OperatorNotExists                 Operator = "notExists"
	OperatorGreaterThan               Operator = "greaterThan"
	OperatorGreaterThanOrEquals       Operator = "greaterThanOrEquals"
	OperatorLessThan                  Operator = "lessThan"
	OperatorLessThanOrEquals          Operator = "lessThanOrEquals"
	OperatorContains                  Operator = "contains"
	OperatorNotContains               Operator = "notContains"
	OperatorStartsWith                Operator = "startsWith"
	OperatorEndsWith                  Operator = "endsWith"
	OperatorSemverEquals              Operator = "semverEquals"
	OperatorSemverNotEquals           Operator = "semverNotEquals"
	OperatorSemverGreaterThan         Operator = "semverGreaterThan"
	OperatorSemverGreaterThanOrEquals Operator = "semverGreaterThanOrEquals"
	OperatorSemverLessThan            Operator = "semverLessThan"
	OperatorSemverLessThanOrEquals    Operator = "semverLessThanOrEquals"
	OperatorBefore                    Operator = "before"
	OperatorAfter                     Operator = "after"
	OperatorIncludes                  Operator = "includes"
	OperatorNotIncludes               Operator = "notIncludes"
	OperatorMatches                   Operator = "matches"
	OperatorNotMatches                Operator = "notMatches"
	OperatorIn                        Operator = "in"
	OperatorNotIn                     Operator = "notIn"
)

// ConditionValue represents the value in a condition
type ConditionValue interface{}

// PlainCondition represents a simple condition
type PlainCondition struct {
	Attribute  AttributeKey    `json:"attribute"`
	Operator   Operator        `json:"operator"`
	Value      *ConditionValue `json:"value,omitempty"`
	RegexFlags *string         `json:"regexFlags,omitempty"`
}

// AndCondition represents an AND condition
type AndCondition struct {
	And []Condition `json:"and"`
}

// OrCondition represents an OR condition
type OrCondition struct {
	Or []Condition `json:"or"`
}

// NotCondition represents a NOT condition
type NotCondition struct {
	Not Condition `json:"not"`
}

/**
 * Context
 */
// Context represents the user context with attribute key-value pairs
type Context map[string]interface{}

/**
 * DatafileContent
 */
// DatafileContent represents the content of a datafile
type DatafileContent struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Revision      string                 `json:"revision"`
	Segments      map[SegmentKey]Segment `json:"segments"`
	Features      map[FeatureKey]Feature `json:"features"`
}

// FromJSON parses a JSON string and returns a DatafileContent
func (dc *DatafileContent) FromJSON(jsonStr string) error {
	return json.Unmarshal([]byte(jsonStr), dc)
}

// ToJSON converts a DatafileContent to JSON string
func (dc *DatafileContent) ToJSON() (string, error) {
	bytes, err := json.Marshal(dc)
	if err != nil {
		return "", fmt.Errorf("failed to marshal DatafileContent to JSON: %w", err)
	}
	return string(bytes), nil
}

// DatafileContentV1 represents the content of a v1 datafile
type DatafileContentV1 struct {
	SchemaVersion string      `json:"schemaVersion"`
	Revision      string      `json:"revision"`
	Attributes    []Attribute `json:"attributes"`
	Segments      []Segment   `json:"segments"`
	Features      []FeatureV1 `json:"features"`
}

/**
 * Feature
 */
// FeatureKey represents the key of a feature
type FeatureKey = string

// Feature represents a feature in the datafile
type Feature struct {
	Key                    *FeatureKey                    `json:"key,omitempty"`
	Hash                   *string                        `json:"hash,omitempty"`
	Deprecated             *bool                          `json:"deprecated,omitempty"`
	Required               []Required                     `json:"required,omitempty"`
	VariablesSchema        map[VariableKey]VariableSchema `json:"variablesSchema,omitempty"`
	DisabledVariationValue *VariationValue                `json:"disabledVariationValue,omitempty"`
	Variations             []Variation                    `json:"variations,omitempty"`
	BucketBy               BucketBy                       `json:"bucketBy"`
	Traffic                []Traffic                      `json:"traffic"`
	Force                  []Force                        `json:"force,omitempty"`
	Ranges                 []Range                        `json:"ranges,omitempty"`
}

// FeatureV1 represents a feature in v1 format
type FeatureV1 struct {
	Key             *FeatureKey      `json:"key,omitempty"`
	Hash            *string          `json:"hash,omitempty"`
	Deprecated      *bool            `json:"deprecated,omitempty"`
	Required        []Required       `json:"required,omitempty"`
	BucketBy        BucketBy         `json:"bucketBy"`
	Traffic         []Traffic        `json:"traffic"`
	Force           []Force          `json:"force,omitempty"`
	Ranges          []Range          `json:"ranges,omitempty"`
	VariablesSchema []VariableSchema `json:"variablesSchema,omitempty"`
	Variations      []VariationV1    `json:"variations,omitempty"`
}

// ParsedFeature represents a parsed feature
type ParsedFeature struct {
	Key                    FeatureKey                     `json:"key"`
	Archived               *bool                          `json:"archived,omitempty"`
	Deprecated             *bool                          `json:"deprecated,omitempty"`
	Description            string                         `json:"description"`
	Tags                   []Tag                          `json:"tags"`
	Required               []Required                     `json:"required,omitempty"`
	BucketBy               BucketBy                       `json:"bucketBy"`
	DisabledVariationValue *VariationValue                `json:"disabledVariationValue,omitempty"`
	VariablesSchema        map[VariableKey]VariableSchema `json:"variablesSchema,omitempty"`
	Variations             []Variation                    `json:"variations,omitempty"`
	Expose                 interface{}                    `json:"expose,omitempty"` // ExposeByEnvironment | Expose
	Force                  interface{}                    `json:"force,omitempty"`  // ForceByEnvironment | Force[]
	Rules                  interface{}                    `json:"rules,omitempty"`  // RulesByEnvironment | Rule[]
}

// EvaluatedFeature represents an evaluated feature
type EvaluatedFeature struct {
	Enabled   bool                          `json:"enabled"`
	Variation *VariationValue               `json:"variation,omitempty"`
	Variables map[VariableKey]VariableValue `json:"variables,omitempty"`
}

// EvaluatedFeatures represents evaluated features
type EvaluatedFeatures map[FeatureKey]EvaluatedFeature

// StickyFeatures represents sticky features
type StickyFeatures = EvaluatedFeatures

// RequiredWithVariation represents a required feature with variation
type RequiredWithVariation struct {
	Key       FeatureKey     `json:"key"`
	Variation VariationValue `json:"variation"`
}

// Required represents a required feature
type Required interface{}

// Weight represents a weight value (0 to 100)
type Weight = int

// EnvironmentKey represents the key of an environment
type EnvironmentKey = string

// Tag represents a tag
type Tag = string

// RuleKey represents the key of a rule
type RuleKey = string

// Rule represents a rule
type Rule struct {
	Key              RuleKey                  `json:"key"`
	Description      *string                  `json:"description,omitempty"`
	Segments         interface{}              `json:"segments"` // GroupSegment | GroupSegment[]
	Percentage       Weight                   `json:"percentage"`
	Enabled          *bool                    `json:"enabled,omitempty"`
	Variation        *VariationValue          `json:"variation,omitempty"`
	Variables        map[string]VariableValue `json:"variables,omitempty"`
	VariationWeights map[string]Weight        `json:"variationWeights,omitempty"`
}

// RulesByEnvironment represents rules by environment
type RulesByEnvironment map[EnvironmentKey][]Rule

// Force represents a force rule
type Force struct {
	Conditions interface{}              `json:"conditions,omitempty"` // Condition | Condition[]
	Segments   interface{}              `json:"segments,omitempty"`   // GroupSegment | GroupSegment[]
	Enabled    *bool                    `json:"enabled,omitempty"`
	Variation  *VariationValue          `json:"variation,omitempty"`
	Variables  map[string]VariableValue `json:"variables,omitempty"`
}

// ForceByEnvironment represents force rules by environment
type ForceByEnvironment map[EnvironmentKey][]Force

// Expose represents exposure settings
type Expose interface{}

// ExposeByEnvironment represents exposure settings by environment
type ExposeByEnvironment map[EnvironmentKey]Expose

/**
 * Segment
 */
// SegmentKey represents the key of a segment
type SegmentKey = string

// Segment represents a segment definition
type Segment struct {
	Archived    *bool       `json:"archived,omitempty"`
	Key         *SegmentKey `json:"key,omitempty"`
	Conditions  interface{} `json:"conditions"` // Condition | Condition[] | string
	Description *string     `json:"description,omitempty"`
}

// PlainGroupSegment represents a simple group segment
type PlainGroupSegment = SegmentKey

// AndGroupSegment represents an AND group segment
type AndGroupSegment struct {
	And []GroupSegment `json:"and"`
}

// OrGroupSegment represents an OR group segment
type OrGroupSegment struct {
	Or []GroupSegment `json:"or"`
}

// NotGroupSegment represents a NOT group segment
type NotGroupSegment struct {
	Not GroupSegment `json:"not"`
}

// GroupSegment represents a group of segments
type GroupSegment interface{}

/**
 * Traffic
 */
// Percentage represents a percentage value (0 to 100,000)
type Percentage = int

// Range represents a range [start, end]
type Range = [2]Percentage

// Allocation represents an allocation
type Allocation struct {
	Variation VariationValue `json:"variation"`
	Range     Range          `json:"range"`
}

// Traffic represents traffic configuration
type Traffic struct {
	Key              RuleKey                  `json:"key"`
	Segments         interface{}              `json:"segments"` // GroupSegment | GroupSegment[] | "*"
	Percentage       Percentage               `json:"percentage"`
	Enabled          *bool                    `json:"enabled,omitempty"`
	Variation        *VariationValue          `json:"variation,omitempty"`
	Variables        map[string]VariableValue `json:"variables,omitempty"`
	VariationWeights map[string]Weight        `json:"variationWeights,omitempty"`
	Allocation       []Allocation             `json:"allocation,omitempty"`
}

/**
 * Variable
 */
// VariableKey represents the key of a variable
type VariableKey = string

// VariableType represents the type of a variable
type VariableType string

const (
	VariableTypeBoolean VariableType = "boolean"
	VariableTypeString  VariableType = "string"
	VariableTypeInteger VariableType = "integer"
	VariableTypeDouble  VariableType = "double"
	VariableTypeArray   VariableType = "array"
	VariableTypeObject  VariableType = "object"
	VariableTypeJSON    VariableType = "json"
)

// VariableValue represents the value of a variable
type VariableValue interface{}

// VariableObjectValue represents an object with variable key-value pairs
type VariableObjectValue map[string]VariableValue

// VariableOverrideSegments represents variable override with segments
type VariableOverrideSegments struct {
	Segments interface{} `json:"segments"` // GroupSegment | GroupSegment[]
}

// VariableOverrideConditions represents variable override with conditions
type VariableOverrideConditions struct {
	Conditions interface{} `json:"conditions"` // Condition | Condition[]
}

// VariableOverride represents a variable override
type VariableOverride struct {
	Value      VariableValue `json:"value"`
	Conditions interface{}   `json:"conditions,omitempty"` // Condition | Condition[]
	Segments   interface{}   `json:"segments,omitempty"`   // GroupSegment | GroupSegment[]
}

// VariableV1 represents a variable in v1 format
type VariableV1 struct {
	Key         VariableKey        `json:"key"`
	Value       VariableValue      `json:"value"`
	Description *string            `json:"description,omitempty"`
	Overrides   []VariableOverride `json:"overrides,omitempty"`
}

// VariableSchema represents the schema of a variable
type VariableSchema struct {
	Deprecated             *bool          `json:"deprecated,omitempty"`
	Key                    *VariableKey   `json:"key,omitempty"`
	Type                   VariableType   `json:"type"`
	DefaultValue           VariableValue  `json:"defaultValue"`
	Description            *string        `json:"description,omitempty"`
	UseDefaultWhenDisabled *bool          `json:"useDefaultWhenDisabled,omitempty"`
	DisabledValue          *VariableValue `json:"disabledValue,omitempty"`
}

/**
 * Variation
 */
// VariationValue represents the value of a variation
type VariationValue = string

// VariationV1 represents a variation in v1 format
type VariationV1 struct {
	Description *string        `json:"description,omitempty"`
	Value       VariationValue `json:"value"`
	Weight      *Weight        `json:"weight,omitempty"`
	Variables   []VariableV1   `json:"variables,omitempty"`
}

// Variation represents a variation
type Variation struct {
	Description       *string                            `json:"description,omitempty"`
	Value             VariationValue                     `json:"value"`
	Weight            *Weight                            `json:"weight,omitempty"`
	Variables         map[VariableKey]VariableValue      `json:"variables,omitempty"`
	VariableOverrides map[VariableKey][]VariableOverride `json:"variableOverrides,omitempty"`
}
