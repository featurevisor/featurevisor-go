package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

// SetStickyFeatures updates the sticky features for the instance
func (f *FeaturevisorInstance) SetStickyFeatures(stickyFeatures types.StickyFeatures) {
	f.stickyFeatures = stickyFeatures
}
