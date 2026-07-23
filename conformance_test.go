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
	NumericBucketKeys []struct {
		Value    float64 `json:"value"`
		Expected string  `json:"expected"`
	} `json:"numericBucketKeys"`
	RegularExpressions struct {
		PortableCases []struct {
			Pattern  string `json:"pattern"`
			Flags    string `json:"flags"`
			Value    string `json:"value"`
			Expected bool   `json:"expected"`
		} `json:"portableCases"`
	} `json:"regularExpressions"`
	ConditionCases []struct {
		Name      string          `json:"name"`
		Condition json.RawMessage `json:"condition"`
		Context   Context         `json:"context"`
		Expected  bool            `json:"expected"`
	} `json:"conditionCases"`
	Defaults struct {
		AggregateCase struct {
			Datafile              DatafileContent `json:"datafile"`
			DefaultVariationValue string          `json:"defaultVariationValue"`
			Expected              struct {
				Enabled   bool   `json:"enabled"`
				Variation string `json:"variation"`
			} `json:"expected"`
		} `json:"aggregateCase"`
	} `json:"defaults"`
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
	if fixture.Version != 2 {
		t.Fatalf("unexpected fixture version %d", fixture.Version)
	}

	for _, item := range fixture.NumericBucketKeys {
		actual := getBucketKey(getBucketKeyOptions{
			FeatureKey:         "feature",
			BucketBy:           "value",
			Context:            Context{"value": item.Value},
			diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{}),
		})
		if actual != item.Expected+".feature" {
			t.Fatalf("numeric bucket key %#v: expected %s.feature, got %s", item.Value, item.Expected, actual)
		}
	}

	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{Datafile: DatafileContent{
		SchemaVersion: "2", Revision: "conformance", Segments: map[SegmentKey]Segment{}, Features: map[FeatureKey]Feature{},
	}, diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{})})
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
		actual := getValueByType(item.Value, item.Type)
		if (actual != nil) != item.Valid {
			t.Fatalf("type %s value %#v: expected valid=%v, got %#v", item.Type, item.Value, item.Valid, actual)
		}
	}

	for _, item := range fixture.RegularExpressions.PortableCases {
		flags := item.Flags
		condition := PlainCondition{
			Attribute:  "value",
			Operator:   OperatorMatches,
			Value:      conditionValue(item.Pattern),
			RegexFlags: &flags,
		}
		actual := reader.AllConditionsAreMatched(condition, Context{"value": item.Value})
		if actual != item.Expected {
			t.Fatalf("regex %q flags %q value %q: expected %v, got %v", item.Pattern, item.Flags, item.Value, item.Expected, actual)
		}
	}

	for _, item := range fixture.ConditionCases {
		var condition Condition
		if err := json.Unmarshal(item.Condition, &condition); err != nil {
			t.Fatalf("condition %q could not be decoded: %v", item.Name, err)
		}
		actual := reader.AllConditionsAreMatched(condition, item.Context)
		if actual != item.Expected {
			t.Fatalf("condition %q: expected %v, got %v", item.Name, item.Expected, actual)
		}
	}

	defaultVariation := VariationValue(fixture.Defaults.AggregateCase.DefaultVariationValue)
	instance := CreateFeaturevisor(FeaturevisorOptions{
		Datafile: fixture.Defaults.AggregateCase.Datafile,
	})
	actualDefault := instance.GetAllEvaluations(
		Context{},
		nil,
		OverrideOptions{DefaultVariationValue: &defaultVariation},
	)[FeatureKey("experiment")]
	if actualDefault.Enabled != fixture.Defaults.AggregateCase.Expected.Enabled ||
		actualDefault.Variation == nil ||
		string(*actualDefault.Variation) != fixture.Defaults.AggregateCase.Expected.Variation {
		t.Fatalf("aggregate default variation mismatch: %#v", actualDefault)
	}
}
