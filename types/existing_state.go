package types

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
