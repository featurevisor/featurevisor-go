package types

import "encoding/json"

type Weight float64

type EnvironmentKey string

type RuleKey string

type Rule struct {
	Key        RuleKey          `json:"key"`
	Segments   json.RawMessage  `json:"segments"`
	Percentage Weight           `json:"percentage"`
	Enabled    *bool            `json:"enabled,omitempty"`
	Variation  *VariationValue  `json:"variation,omitempty"`
	Variables  map[string]VariableValue `json:"variables,omitempty"`
}

type Tag string

type Environment struct {
	Expose *json.RawMessage `json:"expose,omitempty"`
	Rules  []Rule           `json:"rules"`
	Force  []Force          `json:"force,omitempty"`
}
