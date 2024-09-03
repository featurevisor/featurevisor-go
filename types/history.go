package types

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
