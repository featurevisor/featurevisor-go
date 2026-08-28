package featurevisor

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
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
	GlobalVariables struct {
		Datafile DatafileContent `json:"datafile"`
		Cases    []struct {
			Name                  string           `json:"name"`
			Key                   string           `json:"key"`
			Context               Context          `json:"context"`
			StickyVariables       StickyVariables  `json:"stickyVariables"`
			DefaultVariableValue  json.RawMessage  `json:"defaultVariableValue"`
			ExpectedValue         json.RawMessage  `json:"expectedValue"`
			ExpectedReason        EvaluationReason `json:"expectedReason"`
			ExpectedOverrideIndex *int             `json:"expectedOverrideIndex"`
			ExpectedOverrideKey   *string          `json:"expectedOverrideKey"`
			ExpectedOverridePath  []string         `json:"expectedOverridePath"`
		} `json:"cases"`
		OverloadCase struct {
			SharedKey            string      `json:"sharedKey"`
			FeatureVariableKey   string      `json:"featureVariableKey"`
			ExpectedGlobalValue  interface{} `json:"expectedGlobalValue"`
			ExpectedFeatureValue interface{} `json:"expectedFeatureValue"`
		} `json:"overloadCase"`
		DatafileUpdateCase struct {
			Initial            DatafileContent `json:"initial"`
			Merge              DatafileContent `json:"merge"`
			Replacement        DatafileContent `json:"replacement"`
			ExpectedAfterMerge struct {
				Features         []string `json:"features"`
				Variables        []string `json:"variables"`
				ChangedFeatures  []string `json:"changedFeatures"`
				ChangedVariables []string `json:"changedVariables"`
			} `json:"expectedAfterMerge"`
			ExpectedAfterReplacement struct {
				Features         []string `json:"features"`
				Variables        []string `json:"variables"`
				ChangedFeatures  []string `json:"changedFeatures"`
				ChangedVariables []string `json:"changedVariables"`
			} `json:"expectedAfterReplacement"`
		} `json:"datafileUpdateCase"`
		DependencyUpdateCase struct {
			Modes []struct {
				Name    string `json:"name"`
				Replace bool   `json:"replace"`
			} `json:"modes"`
			Initial                         DatafileContent `json:"initial"`
			Updated                         DatafileContent `json:"updated"`
			WithoutSegment                  DatafileContent `json:"withoutSegment"`
			ExpectedChangedFeatures         []string        `json:"expectedChangedFeatures"`
			ExpectedChangedVariables        []string        `json:"expectedChangedVariables"`
			ExpectedRemovedSegmentFeatures  []string        `json:"expectedRemovedSegmentFeatures"`
			ExpectedRemovedSegmentVariables []string        `json:"expectedRemovedSegmentVariables"`
		} `json:"dependencyUpdateCase"`
	} `json:"globalVariables"`
	RequiredFeatures struct {
		Datafile DatafileContent `json:"datafile"`
		Cases    []struct {
			Name            string `json:"name"`
			Feature         string `json:"feature"`
			ExpectedEnabled bool   `json:"expectedEnabled"`
		} `json:"cases"`
		FeatureVariableCase struct {
			Feature             string      `json:"feature"`
			Variable            string      `json:"variable"`
			ExpectedValue       interface{} `json:"expectedValue"`
			ExpectedOverrideKey string      `json:"expectedOverrideKey"`
		} `json:"featureVariableCase"`
	} `json:"requiredFeatures"`
}

func sameStringSet(actual interface{}, expected []string) bool {
	actualValues, ok := actual.([]string)
	if !ok {
		return false
	}
	if len(actualValues) != len(expected) {
		return false
	}
	seen := map[string]int{}
	for _, value := range actualValues {
		seen[value]++
	}
	for _, value := range expected {
		seen[value]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}

func TestGlobalVariableDatafileUpdates(t *testing.T) {
	fixture := loadConformanceFixture(t).GlobalVariables.DatafileUpdateCase
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: fixture.Initial})
	var details EventDetails
	unsubscribe := instance.On(EventNameDatafileSet, func(value EventDetails) { details = value })
	defer unsubscribe()
	instance.SetDatafile(fixture.Merge)
	if !sameStringSet(instance.GetFeatureKeys(), fixture.ExpectedAfterMerge.Features) || !sameStringSet(instance.GetGlobalVariableKeys(), fixture.ExpectedAfterMerge.Variables) {
		t.Fatal("merge did not preserve and add expected entities")
	}
	if !sameStringSet(details["features"], fixture.ExpectedAfterMerge.ChangedFeatures) || !sameStringSet(details["variables"], fixture.ExpectedAfterMerge.ChangedVariables) {
		t.Fatalf("unexpected merge details: %#v", details)
	}
	instance.SetDatafile(fixture.Replacement, true)
	if !sameStringSet(instance.GetFeatureKeys(), fixture.ExpectedAfterReplacement.Features) || !sameStringSet(instance.GetGlobalVariableKeys(), fixture.ExpectedAfterReplacement.Variables) {
		t.Fatal("replacement did not retain expected entities")
	}
	if !sameStringSet(details["features"], fixture.ExpectedAfterReplacement.ChangedFeatures) || !sameStringSet(details["variables"], fixture.ExpectedAfterReplacement.ChangedVariables) {
		t.Fatalf("unexpected replacement details: %#v", details)
	}
}

func TestGlobalVariableDependencyUpdates(t *testing.T) {
	fixture := loadConformanceFixture(t).GlobalVariables.DependencyUpdateCase
	for _, mode := range fixture.Modes {
		t.Run(mode.Name, func(t *testing.T) {
			instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: fixture.Initial})
			var details EventDetails
			instance.On(EventNameDatafileSet, func(value EventDetails) { details = value })
			instance.SetDatafile(fixture.Updated, mode.Replace)
			if !sameStringSet(details["features"], fixture.ExpectedChangedFeatures) || !sameStringSet(details["variables"], fixture.ExpectedChangedVariables) {
				t.Fatalf("unexpected dependency details: %#v", details)
			}
		})
	}
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: fixture.Initial})
	var details EventDetails
	instance.On(EventNameDatafileSet, func(value EventDetails) { details = value })
	instance.SetDatafile(fixture.WithoutSegment, true)
	if !sameStringSet(details["features"], fixture.ExpectedRemovedSegmentFeatures) || !sameStringSet(details["variables"], fixture.ExpectedRemovedSegmentVariables) {
		t.Fatalf("unexpected removed segment details: %#v", details)
	}
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
	if fixture.Version != 6 {
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
	actualDefault := instance.GetFeatureEvaluations(
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

func TestGlobalVariableConformance(t *testing.T) {
	fixture := loadConformanceFixture(t)
	for _, item := range fixture.GlobalVariables.Cases {
		item := item
		t.Run(item.Name, func(t *testing.T) {
			options := FeaturevisorOptions{Datafile: fixture.GlobalVariables.Datafile}
			if item.StickyVariables != nil {
				options.StickyVariables = &item.StickyVariables
			}
			instance := CreateFeaturevisor(options)
			override := OverrideOptions{}
			if len(item.DefaultVariableValue) > 0 {
				override.DefaultVariableValueSet = true
				if err := json.Unmarshal(item.DefaultVariableValue, &override.DefaultVariableValue); err != nil {
					t.Fatal(err)
				}
			}
			evaluation := instance.EvaluateGlobalVariable(item.Key, item.Context, override)
			if evaluation.Reason != item.ExpectedReason {
				t.Fatalf("expected reason %s, got %s", item.ExpectedReason, evaluation.Reason)
			}
			if len(item.ExpectedValue) > 0 {
				var expected interface{}
				if err := json.Unmarshal(item.ExpectedValue, &expected); err != nil {
					t.Fatal(err)
				}
				actual := evaluation.VariableValue
				if !reflect.DeepEqual(actual, expected) {
					t.Fatalf("expected value %#v, got %#v", expected, actual)
				}
			}
			if !reflect.DeepEqual(evaluation.VariableOverrideIndex, item.ExpectedOverrideIndex) {
				t.Fatalf("expected override index %#v, got %#v", item.ExpectedOverrideIndex, evaluation.VariableOverrideIndex)
			}
			if !reflect.DeepEqual(evaluation.VariableOverrideKey, item.ExpectedOverrideKey) {
				t.Fatalf("expected override key %#v, got %#v", item.ExpectedOverrideKey, evaluation.VariableOverrideKey)
			}
			if !reflect.DeepEqual(evaluation.VariableOverridePath, item.ExpectedOverridePath) {
				t.Fatalf("expected override path %#v, got %#v", item.ExpectedOverridePath, evaluation.VariableOverridePath)
			}
		})
	}
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: fixture.GlobalVariables.Datafile})
	overload := fixture.GlobalVariables.OverloadCase
	if actual := instance.GetGlobalVariable(overload.SharedKey); !reflect.DeepEqual(actual, overload.ExpectedGlobalValue) {
		t.Fatalf("global collision value: expected %#v, got %#v", overload.ExpectedGlobalValue, actual)
	}
	if actual := instance.GetVariable(overload.SharedKey, overload.FeatureVariableKey); !reflect.DeepEqual(actual, overload.ExpectedFeatureValue) {
		t.Fatalf("feature collision value: expected %#v, got %#v", overload.ExpectedFeatureValue, actual)
	}
}

func TestRequiredFeaturesConformance(t *testing.T) {
	fixture := loadConformanceFixture(t)
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: fixture.RequiredFeatures.Datafile})
	for _, item := range fixture.RequiredFeatures.Cases {
		if actual := instance.IsEnabled(item.Feature); actual != item.ExpectedEnabled {
			t.Errorf("%s: expected %v, got %v", item.Name, item.ExpectedEnabled, actual)
		}
	}
	item := fixture.RequiredFeatures.FeatureVariableCase
	evaluation := instance.EvaluateVariable(item.Feature, item.Variable)
	if !reflect.DeepEqual(evaluation.VariableValue, item.ExpectedValue) {
		t.Fatalf("expected feature variable %#v, got %#v", item.ExpectedValue, evaluation.VariableValue)
	}
	if evaluation.VariableOverrideKey == nil || *evaluation.VariableOverrideKey != item.ExpectedOverrideKey {
		t.Fatalf("expected override key %q, got %#v", item.ExpectedOverrideKey, evaluation.VariableOverrideKey)
	}
}
