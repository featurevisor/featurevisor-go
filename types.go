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

// Test types
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

// Site index and history types
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
	Links   *struct {
		Feature  string     `json:"feature"`
		Segment  string     `json:"segment"`
		Attribute string     `json:"attribute"`
		Commit   CommitHash `json:"commit"`
	} `json:"links,omitempty"`
	Entities struct {
		Attributes []struct {
			Attribute
			LastModified    *LastModified `json:"lastModified,omitempty"`
			UsedInSegments  []SegmentKey  `json:"usedInSegments"`
			UsedInFeatures  []FeatureKey  `json:"usedInFeatures"`
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
	Hash      CommitHash  `json:"hash"`
	Author    string      `json:"author"`
	Timestamp string      `json:"timestamp"`
	Entities  []EntityDiff `json:"entities"`
}
