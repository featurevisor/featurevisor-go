package types

import "encoding/json"

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
	Value      VariableValue `json:"value"`
	Segments   json.RawMessage `json:"segments,omitempty"`
	Conditions json.RawMessage `json:"conditions,omitempty"`
}

type Variable struct {
	Key       VariableKey       `json:"key"`
	Value     VariableValue     `json:"value"`
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
