package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

func (f *FeaturevisorInstance) getMatchedTraffic(traffic []types.Traffic, context types.Context) *types.Traffic {
	for _, t := range traffic {
		parsedSegments := f.parseFromStringifiedSegments(t.Segments)
		if allGroupSegmentsAreMatched(parsedSegments, context, f.datafileReader, f.logger) {
			return &t
		}
	}
	return nil
}
