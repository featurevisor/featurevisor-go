package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

func segmentIsMatched(segment types.Segment, context types.Context, logger Logger) bool {
	return allConditionsAreMatched(segment.Conditions, context, logger)
}

func allGroupSegmentsAreMatched(groupSegments interface{}, context types.Context, datafileReader *DatafileReader, logger Logger) bool {
	switch gs := groupSegments.(type) {
	case string:
		if gs == "*" {
			return true
		}
		segment := datafileReader.GetSegment(types.SegmentKey(gs))
		if segment != nil {
			return segmentIsMatched(*segment, context, logger)
		}
		return false

	case types.AndGroupSegment:
		for _, groupSegment := range gs.And {
			if !allGroupSegmentsAreMatched(groupSegment, context, datafileReader, logger) {
				return false
			}
		}
		return true

	case types.OrGroupSegment:
		for _, groupSegment := range gs.Or {
			if allGroupSegmentsAreMatched(groupSegment, context, datafileReader, logger) {
				return true
			}
		}
		return false

	case types.NotGroupSegment:
		for _, groupSegment := range gs.Not {
			if allGroupSegmentsAreMatched(groupSegment, context, datafileReader, logger) {
				return false
			}
		}
		return true

	case []types.GroupSegment:
		for _, groupSegment := range gs {
			if !allGroupSegmentsAreMatched(groupSegment, context, datafileReader, logger) {
				return false
			}
		}
		return true
	}

	return false
}
