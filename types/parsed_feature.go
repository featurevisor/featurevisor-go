package types

type ParsedFeature struct {
	Key             FeatureKey                   `json:"key"`
	Deprecated      *bool                        `json:"deprecated,omitempty"`
	Description     string                       `json:"description"`
	Tags            []Tag                        `json:"tags"`
	Required        []Required                   `json:"required,omitempty"`
	BucketBy        BucketBy                     `json:"bucketBy"`
	VariablesSchema []VariableSchema             `json:"variablesSchema,omitempty"`
	Variations      []Variation                  `json:"variations,omitempty"`
	Environments    map[EnvironmentKey]Environment `json:"environments"`
}
