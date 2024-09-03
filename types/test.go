package types

import "time"

type AssertionMatrix map[string][]AttributeValue

type FeatureAssertion struct {
	Matrix              AssertionMatrix `json:"matrix,omitempty"`
	Description         string          `json:"description,omitempty"`
	Environment         EnvironmentKey  `json:"environment"`
	At                  Weight          `json:"at"`
	Context             Context         `json:"context"`
	ExpectedToBeEnabled bool            `json:"expectedToBeEnabled"`
	ExpectedVariation   *VariationValue `json:"expectedVariation,omitempty"`
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
	Description string                    `json:"description"`
	Duration    time.Duration             `json:"duration"`
	Passed      bool                      `json:"passed"`
	Errors      []TestResultAssertionError `json:"errors,omitempty"`
}

type TestResult struct {
	Type       string               `json:"type"`
	Key        string               `json:"key"`
	NotFound   *bool                `json:"notFound,omitempty"`
	Passed     bool                 `json:"passed"`
	Duration   time.Duration        `json:"duration"`
	Assertions []TestResultAssertion `json:"assertions"`
}
