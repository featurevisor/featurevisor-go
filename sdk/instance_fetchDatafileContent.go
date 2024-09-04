package sdk

import (
	"errors"
	"net/http"
	"io/ioutil"

	"github.com/featurevisor/featurevisor-go/types"
)

func (f *FeaturevisorInstance) fetchDatafileContent(datafileURL string, handleDatafileFetch func(string) (types.DatafileContent, error)) (types.DatafileContent, error) {
	if handleDatafileFetch != nil {
		return handleDatafileFetch(datafileURL)
	}

	// Default fetch logic
	resp, err := http.Get(datafileURL)
	if err != nil {
		return types.DatafileContent{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.DatafileContent{}, errors.New("failed to fetch datafile")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return types.DatafileContent{}, err
	}

	var datafileContent types.DatafileContent
	if err := json.Unmarshal(body, &datafileContent); err != nil {
		return types.DatafileContent{}, err
	}

	return datafileContent, nil
}
