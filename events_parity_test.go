package featurevisor

import "testing"

func TestGetParamsForDatafileSetEventShape(t *testing.T) {
	logger := newLogger(loggerOptions{})

	previousReader := newDatafileReader(datafileReaderOptions{
		featurevisorLogger: logger,
		Datafile: DatafileContent{
			SchemaVersion: "2",
			Revision:      "1",
			Segments:      map[SegmentKey]Segment{},
			Features: map[FeatureKey]Feature{
				"a": {Hash: parityStringPtr("h1"), BucketBy: "userId", Traffic: []Traffic{}},
			},
		},
	})
	newReader := newDatafileReader(datafileReaderOptions{
		featurevisorLogger: logger,
		Datafile: DatafileContent{
			SchemaVersion: "2",
			Revision:      "2",
			Segments:      map[SegmentKey]Segment{},
			Features: map[FeatureKey]Feature{
				"a": {Hash: parityStringPtr("h2"), BucketBy: "userId", Traffic: []Traffic{}},
			},
		},
	})

	params := getParamsForDatafileSetEvent(previousReader, newReader, true)

	if _, exists := params["removedFeatures"]; exists {
		t.Fatalf("did not expect removedFeatures field in event details")
	}
	if _, exists := params["changedFeatures"]; exists {
		t.Fatalf("did not expect changedFeatures field in event details")
	}
	if _, exists := params["addedFeatures"]; exists {
		t.Fatalf("did not expect addedFeatures field in event details")
	}
	if params["replaced"] != true {
		t.Fatalf("expected replaced=true in event details")
	}
}

func parityStringPtr(value string) *string {
	return &value
}
