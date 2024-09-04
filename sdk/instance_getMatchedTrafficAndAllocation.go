package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

func (f *FeaturevisorInstance) getMatchedTrafficAndAllocation(traffic []types.Traffic, context types.Context, bucketValue int) (*types.Traffic, *types.Allocation) {
	matchedTraffic := f.getMatchedTraffic(traffic, context)
	if matchedTraffic == nil {
		return nil, nil
	}

	matchedAllocation := f.getMatchedAllocation(*matchedTraffic, bucketValue)
	return matchedTraffic, matchedAllocation
}
