package types

type OverrideFeature struct {
	Enabled   *bool                    `json:"enabled"`
	Variation *VariationValue          `json:"variation,omitempty"`
	Variables map[VariableKey]VariableValue `json:"variables,omitempty"`
}

type StickyFeatures map[FeatureKey]OverrideFeature

type InitialFeatures StickyFeatures
