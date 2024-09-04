package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

type ForceResult struct {
	Force      *types.Force
	ForceIndex int
}

func (f *FeaturevisorInstance) findForceFromFeature(feature *types.Feature, context types.Context) (*types.Force, int) {
	if feature.Force == nil {
		return nil, -1
	}

	for i, force := range feature.Force {
		if force.Conditions != nil {
			parsedConditions := f.parseFromStringifiedSegments(force.Conditions)
			if allConditionsAreMatched(parsedConditions, context, f.logger) {
				return &force, i
			}
		}

		if force.Segments != nil {
			parsedSegments := f.parseFromStringifiedSegments(force.Segments)
			if allGroupSegmentsAreMatched(parsedSegments, context, f.datafileReader, f.logger) {
				return &force, i
			}
		}
	}

	return nil, -1
}
