package types

import "encoding/json"

type SegmentKey string

type Segment struct {
	Key        SegmentKey          `json:"key"`
	Conditions json.RawMessage     `json:"conditions"`
}

type GroupSegment interface{}

type AndGroupSegment struct {
	And []GroupSegment `json:"and"`
}

type OrGroupSegment struct {
	Or []GroupSegment `json:"or"`
}

type NotGroupSegment struct {
	Not []GroupSegment `json:"not"`
}
