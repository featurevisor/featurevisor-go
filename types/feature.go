package types

import "encoding/json"

type FeatureKey string

type Force struct {
	Conditions json.RawMessage     `json:"conditions,omitempty"`
	Segments   json.RawMessage     `json:"segments,omitempty"`
	Enabled    *bool               `json:"enabled,omitempty"`
	Variation  *VariationValue     `json:"variation,omitempty"`
	Variables  map[string]VariableValue `json:"variables,omitempty"`
}

type Percentage float64

type Range [2]Percentage

type Allocation struct {
	Variation VariationValue `json:"variation"`
	Range     Range          `json:"range"`
}

type Traffic struct {
	Key        string           `json:"key"`
	Segments   json.RawMessage  `json:"segments"`
	Percentage Percentage       `json:"percentage"`
	Enabled    *bool            `json:"enabled,omitempty"`
	Variation  *VariationValue  `json:"variation,omitempty"`
	Variables  map[string]VariableValue `json:"variables,omitempty"`
	Allocation []Allocation     `json:"allocation"`
}

type BucketBy interface{}

type RequiredWithVariation struct {
	Key       FeatureKey     `json:"key"`
	Variation VariationValue `json:"variation"`
}

type Required interface{}

type Feature struct {
	Key             FeatureKey          `json:"key"`
	Deprecated      *bool               `json:"deprecated,omitempty"`
	Required        []Required          `json:"required,omitempty"`
	VariablesSchema []VariableSchema    `json:"variablesSchema,omitempty"`
	Variations      []Variation         `json:"variations,omitempty"`
	BucketBy        BucketBy            `json:"bucketBy"`
	Traffic         []Traffic           `json:"traffic"`
	Force           []Force             `json:"force,omitempty"`
	Ranges          []Range             `json:"ranges,omitempty"`
}
