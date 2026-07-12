package featurevisor

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

type conformanceFixture struct {
	Version   int `json:"version"`
	Bucketing struct {
		Allocations            []Allocation              `json:"allocations"`
		AllocationExpectations map[string]VariationValue `json:"allocationExpectations"`
	} `json:"bucketing"`
	TypedVariables []struct {
		Type  string      `json:"type"`
		Value interface{} `json:"value"`
		Valid bool        `json:"valid"`
	} `json:"typedVariables"`
}

func loadConformanceFixture(t *testing.T) conformanceFixture {
	t.Helper()
	content, err := os.ReadFile("conformance/sdk-v3.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture conformanceFixture
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func TestSDKV3ConformanceFixture(t *testing.T) {
	fixture := loadConformanceFixture(t)
	if fixture.Version != 1 {
		t.Fatalf("unexpected fixture version %d", fixture.Version)
	}

	reader := newDatafileReader(datafileReaderOptions{Datafile: DatafileContent{
		SchemaVersion: "2", Revision: "conformance", Segments: map[SegmentKey]Segment{}, Features: map[FeatureKey]Feature{},
	}, featurevisorLogger: newLogger(loggerOptions{})})
	traffic := &Traffic{Allocation: fixture.Bucketing.Allocations}
	for bucket, expected := range fixture.Bucketing.AllocationExpectations {
		var bucketValue int
		if _, err := fmt.Sscanf(bucket, "%d", &bucketValue); err != nil {
			t.Fatal(err)
		}
		allocation := reader.GetMatchedAllocation(traffic, bucketValue)
		if allocation == nil || allocation.Variation != expected {
			t.Fatalf("bucket %d: expected %s, got %#v", bucketValue, expected, allocation)
		}
	}

	for _, item := range fixture.TypedVariables {
		actual := GetValueByType(item.Value, item.Type)
		if (actual != nil) != item.Valid {
			t.Fatalf("type %s value %#v: expected valid=%v, got %#v", item.Type, item.Value, item.Valid, actual)
		}
	}
}
