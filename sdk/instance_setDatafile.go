package sdk

import (
	"encoding/json"
	"errors"

	"github.com/featurevisor/featurevisor-go/types"
)

func (f *FeaturevisorInstance) setDatafile(datafile interface{}) error {
	var datafileContent types.DatafileContent

	switch d := datafile.(type) {
	case string:
		if err := json.Unmarshal([]byte(d), &datafileContent); err != nil {
			return err
		}
	case types.DatafileContent:
		datafileContent = d
	default:
		return errors.New("invalid datafile format")
	}

	f.datafileReader = NewDatafileReader(datafileContent)
	return nil
}
