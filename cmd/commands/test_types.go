package commands

import "github.com/featurevisor/featurevisor-go"

// AssertionMatrix represents a matrix of assertions
type AssertionMatrix map[string][]featurevisor.AttributeValue

// ExpectedEvaluations represents expected evaluations
type ExpectedEvaluations struct {
	Flag      map[string]interface{}                              `json:"flag,omitempty"`
	Variation map[string]interface{}                              `json:"variation,omitempty"`
	Variables map[featurevisor.VariableKey]map[string]interface{} `json:"variables,omitempty"`
}

// FeatureChildAssertion represents a child assertion for a feature
type FeatureChildAssertion struct {
	Sticky                *featurevisor.StickyFeatures                            `json:"sticky,omitempty"`
	Context               *featurevisor.Context                                   `json:"context,omitempty"`
	DefaultVariationValue *string                                                 `json:"defaultVariationValue,omitempty"`
	DefaultVariableValues map[string]featurevisor.VariableValue                   `json:"defaultVariableValues,omitempty"`
	ExpectedToBeEnabled   *bool                                                   `json:"expectedToBeEnabled,omitempty"`
	ExpectedVariation     *string                                                 `json:"expectedVariation,omitempty"`
	ExpectedVariables     map[featurevisor.VariableKey]featurevisor.VariableValue `json:"expectedVariables,omitempty"`
	ExpectedEvaluations   *ExpectedEvaluations                                    `json:"expectedEvaluations,omitempty"`
}

// FeatureAssertion represents an assertion for a feature
type FeatureAssertion struct {
	Matrix                *AssertionMatrix                                        `json:"matrix,omitempty"`
	Description           *string                                                 `json:"description,omitempty"`
	Environment           featurevisor.EnvironmentKey                             `json:"environment"`
	Scope                 *string                                                 `json:"scope,omitempty"`
	Tag                   *string                                                 `json:"tag,omitempty"`
	At                    *featurevisor.Weight                                    `json:"at,omitempty"`
	Sticky                *featurevisor.StickyFeatures                            `json:"sticky,omitempty"`
	Context               *featurevisor.Context                                   `json:"context,omitempty"`
	DefaultVariationValue *string                                                 `json:"defaultVariationValue,omitempty"`
	DefaultVariableValues map[string]featurevisor.VariableValue                   `json:"defaultVariableValues,omitempty"`
	ExpectedToBeEnabled   *bool                                                   `json:"expectedToBeEnabled,omitempty"`
	ExpectedVariation     *string                                                 `json:"expectedVariation,omitempty"`
	ExpectedVariables     map[featurevisor.VariableKey]featurevisor.VariableValue `json:"expectedVariables,omitempty"`
	ExpectedEvaluations   *ExpectedEvaluations                                    `json:"expectedEvaluations,omitempty"`
	Children              []FeatureChildAssertion                                 `json:"children,omitempty"`
}

// TestFeature represents a test feature
type TestFeature struct {
	Key        *string                 `json:"key,omitempty"`
	Feature    featurevisor.FeatureKey `json:"feature"`
	Assertions []FeatureAssertion      `json:"assertions"`
}

// SegmentAssertion represents an assertion for a segment
type SegmentAssertion struct {
	Matrix          *AssertionMatrix     `json:"matrix,omitempty"`
	Description     *string              `json:"description,omitempty"`
	Context         featurevisor.Context `json:"context"`
	ExpectedToMatch bool                 `json:"expectedToMatch"`
}

// TestSegment represents a test segment
type TestSegment struct {
	Key        *string                 `json:"key,omitempty"`
	Segment    featurevisor.SegmentKey `json:"segment"`
	Assertions []SegmentAssertion      `json:"assertions"`
}

// Test represents a test
type Test interface{}

// TestResultAssertionError represents an error in a test assertion
type TestResultAssertionError struct {
	Type     string                                 `json:"type"`
	Expected featurevisor.AttributeValue            `json:"expected"`
	Actual   featurevisor.AttributeValue            `json:"actual"`
	Message  *string                                `json:"message,omitempty"`
	Details  map[string]featurevisor.AttributeValue `json:"details,omitempty"`
}

// TestResultAssertion represents a test assertion result
type TestResultAssertion struct {
	Description string                     `json:"description"`
	Duration    int                        `json:"duration"`
	Passed      bool                       `json:"passed"`
	Errors      []TestResultAssertionError `json:"errors,omitempty"`
}

// TestResult represents a test result
type TestResult struct {
	Type       string                `json:"type"`
	Key        string                `json:"key"`
	NotFound   *bool                 `json:"notFound,omitempty"`
	Passed     bool                  `json:"passed"`
	Duration   int                   `json:"duration"`
	Assertions []TestResultAssertion `json:"assertions"`
}

// AssertionResult represents the result of a single assertion
type AssertionResult struct {
	HasError bool    `json:"hasError"`
	Errors   string  `json:"errors"`
	Duration float64 `json:"duration"`
}
