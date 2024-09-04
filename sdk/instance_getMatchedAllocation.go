package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

func (f *FeaturevisorInstance) getMatchedAllocation(traffic types.Traffic, bucketValue int) *types.Allocation {
	for _, allocation := range traffic.Allocation {
		start, end := allocation.Range[0], allocation.Range[1]
		if int(start) <= bucketValue && int(end) >= bucketValue {
			return &allocation
		}
	}
	return nil
}
