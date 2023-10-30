package featurevisor

import "time"

type AttributeKey string

type AttributeValue struct {
	String *string
	Number *float64
	Bool   *bool
	Date   *time.Time
	IsNil  bool
}

type Context map[AttributeKey]AttributeValue

type Attribute struct {
	Archived *bool  `json:"archived,omitempty"`
	Key      string `json:"key"`
	Type     string `json:"type"`
	Capture  *bool  `json:"capture,omitempty"`
}

type Operator string

const (
	Equals                    Operator = "equals"
	NotEquals                 Operator = "notEquals"
	GreaterThan               Operator = "greaterThan"
	GreaterThanOrEquals       Operator = "greaterThanOrEquals"
	LessThan                  Operator = "lessThan"
	LessThanOrEquals          Operator = "lessThanOrEquals"
	Contains                  Operator = "contains"
	NotContains               Operator = "notContains"
	StartsWith                Operator = "startsWith"
	EndsWith                  Operator = "endsWith"
	SemverEquals              Operator = "semverEquals"
	SemverNotEquals           Operator = "semverNotEquals"
	SemverGreaterThan         Operator = "semverGreaterThan"
	SemverGreaterThanOrEquals Operator = "semverGreaterThanOrEquals"
	SemverLessThan            Operator = "semverLessThan"
	SemverLessThanOrEquals    Operator = "semverLessThanOrEquals"
	Before                    Operator = "before"
	After                     Operator = "after"
	In                        Operator = "in"
	NotIn                     Operator = "notIn"
)

type ConditionValue struct {
	String  *string
	Number  *float64
	Bool    *bool
	Date    *time.Time
	IsNil   bool
	Strings *[]string
}

type PlainCondition struct {
	Attribute AttributeKey   `json:"attribute"`
	Operator  Operator       `json:"operator"`
	Value     ConditionValue `json:"value"`
}

type AndCondition struct {
	And []Condition `json:"and"`
}

type OrCondition struct {
	Or []Condition `json:"or"`
}

type NotCondition struct {
	Not []Condition `json:"not"`
}

type Condition struct {
	Plain *PlainCondition
	And   *AndCondition
	Or    *OrCondition
	Not   *NotCondition
}

type SegmentKey string

type Segment struct {
	Archived   *bool       `json:"archived,omitempty"`
	Key        SegmentKey  `json:"key"`
	Conditions interface{} `json:"conditions"` // @TODO: This can be Condition, []Condition, or string
}

type PlainGroupSegment SegmentKey

type AndGroupSegment struct {
	And []GroupSegment `json:"and"`
}

type OrGroupSegment struct {
	Or []GroupSegment `json:"or"`
}

type NotGroupSegment struct {
	Not []GroupSegment `json:"not"`
}

type GroupSegment struct {
	Plain *PlainGroupSegment
	And   *AndGroupSegment
	Or    *OrGroupSegment
	Not   *NotGroupSegment
}

type VariationValue string

type VariableKey string

type VariableType string

const (
	Boolean VariableType = "boolean"
	String  VariableType = "string"
	Integer VariableType = "integer"
	Double  VariableType = "double"
	Array   VariableType = "array"
	Object  VariableType = "object"
	Json    VariableType = "json"
)

type VariableObjectValue map[string]VariableValue

type VariableValue struct {
	Bool   *bool
	String *string
	Number *float64
	// ... and so on for other types
}

type VariableOverrideSegments struct {
	Segments GroupSegment `json:"segments"`
}

type VariableOverrideConditions struct {
	Conditions Condition `json:"conditions"`
}

type VariableOverrideBase struct {
	Value VariableValue `json:"value"`
}

type VariableOverride struct {
	Value      VariableValue `json:"value"`
	Conditions *Condition    `json:"conditions,omitempty"`
	Segments   *GroupSegment `json:"segments,omitempty"`
}

type Variable struct {
	Key         VariableKey        `json:"key"`
	Value       VariableValue      `json:"value"`
	Description *string            `json:"description,omitempty"`
	Overrides   []VariableOverride `json:"overrides,omitempty"`
}

type Variation struct {
	Description *string            `json:"description,omitempty"`
	Value       VariationValue     `json:"value"`
	Weight      *Weight            `json:"weight,omitempty"`
	Variables   []VariableOverride `json:"variables,omitempty"`
}

type VariableSchema struct {
	Key          VariableKey   `json:"key"`
	Type         VariableType  `json:"type"`
	DefaultValue VariableValue `json:"defaultValue"`
}

type FeatureKey string

type Force struct {
	Conditions *[]Condition              `json:"conditions,omitempty"`
	Segments   *[]GroupSegment           `json:"segments,omitempty"`
	Enabled    *bool                     `json:"enabled,omitempty"`
	Variation  *VariationValue           `json:"variation,omitempty"`
	Variables  *map[string]VariableValue `json:"variables,omitempty"`
}

type Slot struct {
	Feature    interface{} `json:"feature"` // @TODO: FeatureKey or false
	Percentage Weight      `json:"percentage"`
}

type Group struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Slots       []Slot `json:"slots"`
}

type BucketKey string
type BucketValue int

/**
 * Datafile-only types
 */
type Percentage int
type Range [2]Percentage

type Allocation struct {
	Variation VariationValue `json:"variation"`
	Range     Range          `json:"range"`
}

type Traffic struct {
	Key        string                    `json:"key"`
	Segments   interface{}               `json:"segments"` // @TODO: GroupSegment, []GroupSegment, or "*"
	Percentage Percentage                `json:"percentage"`
	Enabled    *bool                     `json:"enabled,omitempty"`
	Variation  *VariationValue           `json:"variation,omitempty"`
	Variables  *map[string]VariableValue `json:"variables,omitempty"`
	Allocation []Allocation              `json:"allocation"`
}

type PlainBucketBy AttributeKey
type AndBucketBy []AttributeKey
type OrBucketBy struct {
	Or []AttributeKey `json:"or"`
}

type BucketBy struct {
	Plain *PlainBucketBy
	And   *AndBucketBy
	Or    *OrBucketBy
}

type RequiredWithVariation struct {
	Key       FeatureKey     `json:"key"`
	Variation VariationValue `json:"variation"`
}

type Required struct {
	Key       *FeatureKey
	Variation *RequiredWithVariation
}

type Feature struct {
	Key             FeatureKey        `json:"key"`
	Deprecated      *bool             `json:"deprecated,omitempty"`
	Required        *[]Required       `json:"required,omitempty"`
	VariablesSchema *[]VariableSchema `json:"variablesSchema,omitempty"`
	Variations      *[]Variation      `json:"variations,omitempty"`
	BucketBy        BucketBy          `json:"bucketBy"`
	Traffic         []Traffic         `json:"traffic"`
	Force           *[]Force          `json:"force,omitempty"`
	Ranges          *[]Range          `json:"ranges,omitempty"`
}

type DatafileContent struct {
	SchemaVersion string      `json:"schemaVersion"`
	Revision      string      `json:"revision"`
	Attributes    []Attribute `json:"attributes"`
	Segments      []Segment   `json:"segments"`
	Features      []Feature   `json:"features"`
}

type OverrideFeature struct {
	Enabled   bool                      `json:"enabled"`
	Variation *VariationValue           `json:"variation,omitempty"`
	Variables *map[string]VariableValue `json:"variables,omitempty"`
}

type StickyFeatures map[FeatureKey]OverrideFeature

type InitialFeatures StickyFeatures

/**
 * YAML-only types
 */
type Weight int

type EnvironmentKey string
type RuleKey string

type Rule struct {
	Key        RuleKey                   `json:"key"`
	Segments   []GroupSegment            `json:"segments"`
	Percentage Weight                    `json:"percentage"`
	Enabled    *bool                     `json:"enabled,omitempty"`
	Variation  *VariationValue           `json:"variation,omitempty"`
	Variables  *map[string]VariableValue `json:"variables,omitempty"`
}

type Environment struct {
	Expose *bool    `json:"expose,omitempty"`
	Rules  []Rule   `json:"rules"`
	Force  *[]Force `json:"force,omitempty"`
}

type Tag string

type ParsedFeature struct {
	Key             FeatureKey                     `json:"key"`
	Archived        *bool                          `json:"archived,omitempty"`
	Deprecated      *bool                          `json:"deprecated,omitempty"`
	Description     string                         `json:"description"`
	Tags            []Tag                          `json:"tags"`
	Required        *[]Required                    `json:"required,omitempty"`
	BucketBy        BucketBy                       `json:"bucketBy"`
	VariablesSchema *[]VariableSchema              `json:"variablesSchema,omitempty"`
	Variations      *[]Variation                   `json:"variations,omitempty"`
	Environments    map[EnvironmentKey]Environment `json:"environments"`
}

/**
 * For maintaining old allocations info,
 * allowing for gradual rollout of new allocations
 * with consistent bucketing
 */
type ExistingFeature struct {
	Variations *[]struct {
		Value  VariationValue `json:"value"`
		Weight Weight         `json:"weight"`
	} `json:"variations,omitempty"`
	Traffic *[]struct {
		Key        RuleKey      `json:"key"`
		Percentage Percentage   `json:"percentage"`
		Allocation []Allocation `json:"allocation"`
	} `json:"traffic,omitempty"`
	Ranges *[]Range `json:"ranges,omitempty"`
}

type ExistingFeatures map[FeatureKey]ExistingFeature

type ExistingState struct {
	Features ExistingFeatures `json:"features"`
}

/**
 * Tests
 */
type FeatureAssertion struct {
	Description         *string                   `json:"description,omitempty"`
	Environment         EnvironmentKey            `json:"environment"`
	At                  Weight                    `json:"at"`
	Context             Context                   `json:"context"`
	ExpectedToBeEnabled bool                      `json:"expectedToBeEnabled"`
	ExpectedVariation   *VariationValue           `json:"expectedVariation,omitempty"`
	ExpectedVariables   *map[string]VariableValue `json:"expectedVariables,omitempty"`
}

type TestFeature struct {
	Feature    FeatureKey         `json:"feature"`
	Assertions []FeatureAssertion `json:"assertions"`
}

type SegmentAssertion struct {
	Description     *string `json:"description,omitempty"`
	Context         Context `json:"context"`
	ExpectedToMatch bool    `json:"expectedToMatch"`
}

type TestSegment struct {
	Segment    SegmentKey         `json:"segment"`
	Assertions []SegmentAssertion `json:"assertions"`
}

type Test struct {
	Segment *TestSegment
	Feature *TestFeature
}
