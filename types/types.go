package types

import (
	"encoding/json"
	"time"
)

type BucketKey string
type BucketValue int // 0 to 100,000 (100% * 1000 to include three decimal places in same integer)

type AttributeKey string
type AttributeValue interface{}

type Context map[AttributeKey]AttributeValue

type AttributeType string

const (
	AttributeTypeBoolean AttributeType = "boolean"
	AttributeTypeString  AttributeType = "string"
	AttributeTypeInteger AttributeType = "integer"
	AttributeTypeDouble  AttributeType = "double"
	AttributeTypeDate    AttributeType = "date"
	AttributeTypeSemver  AttributeType = "semver"
)

type Attribute struct {
	Key     AttributeKey  `json:"key"`
	Type    AttributeType `json:"type"`
	Capture *bool         `json:"capture,omitempty"`
}

type Operator string

const (
	OperatorEquals                    Operator = "equals"
	OperatorNotEquals                 Operator = "notEquals"
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
	OperatorIn                        Operator = "in"
	OperatorNotIn                     Operator = "notIn"
)

type ConditionValue interface{}

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
	PlainCondition
	AndCondition
	OrCondition
	NotCondition
}

func (c *Condition) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &c.PlainCondition); err == nil {
		return nil
	}
	if err := json.Unmarshal(data, &c.AndCondition); err == nil {
		return nil
	}
	if err := json.Unmarshal(data, &c.OrCondition); err == nil {
		return nil
	}
	if err := json.Unmarshal(data, &c.NotCondition); err == nil {
		return nil
	}
	return json.Unmarshal(data, &c.PlainCondition)
}

type DatafileContent struct {
	SchemaVersion string      `json:"schemaVersion"`
	Revision      string      `json:"revision"`
	Attributes    []Attribute `json:"attributes"`
	Segments      []Segment   `json:"segments"`
	Features      []Feature   `json:"features"`
}

type Weight float64

type EnvironmentKey string

type RuleKey string

type Rule struct {
	Key        string                   `json:"key"`
	Segments   json.RawMessage          `json:"segments"`
	Percentage Weight                   `json:"percentage"`
	Enabled    *bool                    `json:"enabled,omitempty"`
	Variation  *VariationValue          `json:"variation,omitempty"`
	Variables  map[string]VariableValue `json:"variables,omitempty"`
}

type Tag string

type Environment struct {
	Expose *json.RawMessage `json:"expose,omitempty"`
	Rules  []Rule           `json:"rules"`
	Force  []Force          `json:"force,omitempty"`
}

type ExistingFeature struct {
	Variations []struct {
		Value  VariationValue `json:"value"`
		Weight Weight         `json:"weight"`
	} `json:"variations,omitempty"`
	Traffic []struct {
		Key        RuleKey      `json:"key"`
		Percentage Percentage   `json:"percentage"`
		Allocation []Allocation `json:"allocation"`
	} `json:"traffic"`
	Ranges []Range `json:"ranges,omitempty"`
}

type ExistingFeatures map[FeatureKey]ExistingFeature

type ExistingState struct {
	Features ExistingFeatures `json:"features"`
}

type FeatureKey string

type Force struct {
	Conditions json.RawMessage          `json:"conditions,omitempty"`
	Segments   json.RawMessage          `json:"segments,omitempty"`
	Enabled    *bool                    `json:"enabled,omitempty"`
	Variation  *VariationValue          `json:"variation,omitempty"`
	Variables  map[string]VariableValue `json:"variables,omitempty"`
}

type Percentage float64

type Range [2]Percentage

type Allocation struct {
	Variation VariationValue `json:"variation"`
	Range     Range          `json:"range"`
}

type Traffic struct {
	Key        string                   `json:"key"`
	Segments   json.RawMessage          `json:"segments"`
	Percentage Percentage               `json:"percentage"`
	Enabled    *bool                    `json:"enabled,omitempty"`
	Variation  *VariationValue          `json:"variation,omitempty"`
	Variables  map[string]VariableValue `json:"variables,omitempty"`
	Allocation []Allocation             `json:"allocation"`
}

type BucketBy interface{}

type RequiredWithVariation struct {
	Key       FeatureKey     `json:"key"`
	Variation VariationValue `json:"variation"`
}

type Required interface{}

type Feature struct {
	Key             FeatureKey       `json:"key"`
	Deprecated      *bool            `json:"deprecated,omitempty"`
	Required        []Required       `json:"required,omitempty"`
	VariablesSchema []VariableSchema `json:"variablesSchema,omitempty"`
	Variations      []Variation      `json:"variations,omitempty"`
	BucketBy        BucketBy         `json:"bucketBy"`
	Traffic         []Traffic        `json:"traffic"`
	Force           []Force          `json:"force,omitempty"`
	Ranges          []Range          `json:"ranges,omitempty"`
}

type EntityType string

const (
	EntityTypeAttribute EntityType = "attribute"
	EntityTypeSegment   EntityType = "segment"
	EntityTypeFeature   EntityType = "feature"
	EntityTypeGroup     EntityType = "group"
	EntityTypeTest      EntityType = "test"
)

type CommitHash string

type HistoryEntity struct {
	Type EntityType `json:"type"`
	Key  string     `json:"key"`
}

type HistoryEntry struct {
	Commit    CommitHash      `json:"commit"`
	Author    string          `json:"author"`
	Timestamp string          `json:"timestamp"`
	Entities  []HistoryEntity `json:"entities"`
}

type LastModified struct {
	Commit    CommitHash `json:"commit"`
	Timestamp string     `json:"timestamp"`
	Author    string     `json:"author"`
}

type SearchIndex struct {
	Links *struct {
		Feature   string     `json:"feature"`
		Segment   string     `json:"segment"`
		Attribute string     `json:"attribute"`
		Commit    CommitHash `json:"commit"`
	} `json:"links,omitempty"`
	Entities struct {
		Attributes []struct {
			Attribute
			LastModified   *LastModified `json:"lastModified,omitempty"`
			UsedInSegments []SegmentKey  `json:"usedInSegments"`
			UsedInFeatures []FeatureKey  `json:"usedInFeatures"`
		} `json:"attributes"`
		Segments []struct {
			Segment
			LastModified   *LastModified `json:"lastModified,omitempty"`
			UsedInFeatures []FeatureKey  `json:"usedInFeatures"`
		} `json:"segments"`
		Features []struct {
			ParsedFeature
			LastModified *LastModified `json:"lastModified,omitempty"`
		} `json:"features"`
	} `json:"entities"`
}

type EntityDiff struct {
	Type    EntityType `json:"type"`
	Key     string     `json:"key"`
	Created *bool      `json:"created,omitempty"`
	Deleted *bool      `json:"deleted,omitempty"`
	Updated *bool      `json:"updated,omitempty"`
	Content string     `json:"content,omitempty"`
}

type Commit struct {
	Hash      CommitHash   `json:"hash"`
	Author    string       `json:"author"`
	Timestamp string       `json:"timestamp"`
	Entities  []EntityDiff `json:"entities"`
}

type OverrideFeature struct {
	Enabled   *bool                         `json:"enabled"`
	Variation *VariationValue               `json:"variation,omitempty"`
	Variables map[VariableKey]VariableValue `json:"variables,omitempty"`
}

type StickyFeatures map[FeatureKey]OverrideFeature

type InitialFeatures StickyFeatures

type ParsedFeature struct {
	Key             FeatureKey                     `json:"key"`
	Deprecated      *bool                          `json:"deprecated,omitempty"`
	Description     string                         `json:"description"`
	Tags            []Tag                          `json:"tags"`
	Required        []Required                     `json:"required,omitempty"`
	BucketBy        BucketBy                       `json:"bucketBy"`
	VariablesSchema []VariableSchema               `json:"variablesSchema,omitempty"`
	Variations      []Variation                    `json:"variations,omitempty"`
	Environments    map[EnvironmentKey]Environment `json:"environments"`
}

type SegmentKey string

type Segment struct {
	Key        SegmentKey      `json:"key"`
	Conditions json.RawMessage `json:"conditions"`
}

type GroupSegment interface{}

type AndGroupSegment struct {
	And []GroupSegment `json:"and"`
}

type OrGroupSegment struct {
	Or []GroupSegment `json:"or"`
}

type NotGroupSegment struct {
	Not []GroupSegment `json:"not"`
}

type AssertionMatrix map[string][]AttributeValue

type FeatureAssertion struct {
	Matrix              AssertionMatrix               `json:"matrix,omitempty"`
	Description         string                        `json:"description,omitempty"`
	Environment         EnvironmentKey                `json:"environment"`
	At                  Weight                        `json:"at"`
	Context             Context                       `json:"context"`
	ExpectedToBeEnabled bool                          `json:"expectedToBeEnabled"`
	ExpectedVariation   *VariationValue               `json:"expectedVariation,omitempty"`
	ExpectedVariables   map[VariableKey]VariableValue `json:"expectedVariables,omitempty"`
}

type TestFeature struct {
	Feature    FeatureKey         `json:"feature"`
	Assertions []FeatureAssertion `json:"assertions"`
}

type SegmentAssertion struct {
	Matrix          AssertionMatrix `json:"matrix,omitempty"`
	Description     string          `json:"description,omitempty"`
	Context         Context         `json:"context"`
	ExpectedToMatch bool            `json:"expectedToMatch"`
}

type TestSegment struct {
	Segment    SegmentKey         `json:"segment"`
	Assertions []SegmentAssertion `json:"assertions"`
}

type Test interface{}

type TestResultAssertionError struct {
	Type     string      `json:"type"`
	Expected interface{} `json:"expected"`
	Actual   interface{} `json:"actual"`
	Message  string      `json:"message,omitempty"`
	Details  interface{} `json:"details,omitempty"`
}

type TestResultAssertion struct {
	Description string                     `json:"description"`
	Duration    time.Duration              `json:"duration"`
	Passed      bool                       `json:"passed"`
	Errors      []TestResultAssertionError `json:"errors,omitempty"`
}

type TestResult struct {
	Type       string                `json:"type"`
	Key        string                `json:"key"`
	NotFound   *bool                 `json:"notFound,omitempty"`
	Passed     bool                  `json:"passed"`
	Duration   time.Duration         `json:"duration"`
	Assertions []TestResultAssertion `json:"assertions"`
}

type VariationValue string

type VariableKey string
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

type VariableValue interface{}

type VariableOverrideSegments struct {
	Segments json.RawMessage `json:"segments"`
}

type VariableOverrideConditions struct {
	Conditions json.RawMessage `json:"conditions"`
}

type VariableOverride struct {
	Value      VariableValue   `json:"value"`
	Segments   json.RawMessage `json:"segments,omitempty"`
	Conditions json.RawMessage `json:"conditions,omitempty"`
}

type Variable struct {
	Key       VariableKey        `json:"key"`
	Value     VariableValue      `json:"value"`
	Overrides []VariableOverride `json:"overrides,omitempty"`
}

type Variation struct {
	Value     VariationValue `json:"value"`
	Variables []Variable     `json:"variables,omitempty"`
}

type VariableSchema struct {
	Key          VariableKey   `json:"key"`
	Type         VariableType  `json:"type"`
	DefaultValue VariableValue `json:"defaultValue"`
}
