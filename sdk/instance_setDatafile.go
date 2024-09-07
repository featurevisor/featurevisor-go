package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

func (f *FeaturevisorInstance) setDatafile(datafileContent types.DatafileContent) error {
	f.datafileReader = NewDatafileReader(datafileContent)
	return nil
}
