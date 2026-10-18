package featurevisor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
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
		Datafile        DatafileContent `json:"datafile"`
		EvaluationCases []struct {
			Name     string                 `json:"name"`
			Type     EvaluationType         `json:"type"`
			Feature  *string                `json:"feature"`
			Variable string                 `json:"variable"`
			Context  Context                `json:"context"`
			Expected map[string]interface{} `json:"expected"`
		} `json:"evaluationCases"`
		Cases []struct {
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
	RequiredFeatureGroupInputs []struct {
		A bool `json:"a"`
		B bool `json:"b"`
	} `json:"requiredFeatureGroupInputs"`
	RequiredFeatureGroups []struct {
		Name         string            `json:"name"`
		Requirements []RequiredFeature `json:"requirements"`
		Results      []bool            `json:"results"`
	} `json:"requiredFeatureGroups"`
	RequiredVariables struct {
		Datafile    DatafileContent `json:"datafile"`
		StickyCases []struct {
			Name            string          `json:"name"`
			StickyVariables StickyVariables `json:"stickyVariables"`
			StickyFeatures  StickyFeatures  `json:"stickyFeatures"`
			Matched         bool            `json:"matched"`
			Accumulated     interface{}     `json:"accumulated"`
		} `json:"stickyCases"`
		Expected struct {
			MatchedValue    interface{}      `json:"matchedValue"`
			DefaultValue    interface{}      `json:"defaultValue"`
			UnmetReason     EvaluationReason `json:"unmetReason"`
			NullGetterValue interface{}      `json:"nullGetterValue"`
		} `json:"expected"`
	} `json:"requiredVariables"`
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
	if fixture.Version != 15 {
		t.Fatalf("unexpected fixture version %d", fixture.Version)
	}
	if len(fixture.ConditionCases) != 189 {
		t.Fatalf("unexpected condition fixture case count %d", len(fixture.ConditionCases))
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

	executedAnyCases := 0
	for _, item := range fixture.ConditionCases {
		var condition Condition
		if err := json.Unmarshal(item.Condition, &condition); err != nil {
			t.Fatalf("condition %q could not be decoded: %v", item.Name, err)
		}
		actual := reader.AllConditionsAreMatched(condition, item.Context)
		if strings.Contains(strings.ToLower(item.Name), "includesany") {
			executedAnyCases++
		}
		if actual != item.Expected {
			t.Fatalf("condition %q: expected %v, got %v", item.Name, item.Expected, actual)
		}
	}

	if executedAnyCases != 73 {
		t.Fatalf("executed %d includesAny condition cases, expected 73", executedAnyCases)
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

type sdkV4Fixture struct {
	Version int `json:"version"`
	Cases   []struct {
		Name     string                     `json:"name"`
		Datafile json.RawMessage            `json:"datafile"`
		Context  Context                    `json:"context"`
		Expected map[string]json.RawMessage `json:"expected"`
	} `json:"cases"`
}

func TestSDKV4PortableCases(t *testing.T) {
	content, err := os.ReadFile("conformance/sdk-v4-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture sdkV4Fixture
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 1 {
		t.Fatalf("unexpected v4 fixture version %d", fixture.Version)
	}
	if len(fixture.Cases) != 255 {
		t.Fatalf("unexpected v4 fixture case count %d", len(fixture.Cases))
	}
	for _, testCase := range fixture.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			originalDatafile := append(json.RawMessage(nil), testCase.Datafile...)
			originalContext, err := json.Marshal(testCase.Context)
			if err != nil {
				t.Fatal(err)
			}
			instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: string(testCase.Datafile)})
			defer instance.Close()
			for pass := 0; pass < 2; pass++ {
				value := instance.GetGlobalVariable("subject", testCase.Context)
				evaluation := instance.EvaluateGlobalVariable("subject", testCase.Context)
				var expectedValue interface{}
				if err := json.Unmarshal(testCase.Expected["value"], &expectedValue); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(value, expectedValue) {
					t.Fatalf("pass %d getter: expected %#v, got %#v", pass+1, expectedValue, value)
				}
				if !reflect.DeepEqual(evaluation.VariableValue, expectedValue) {
					t.Fatalf("pass %d detail value: expected %#v, got %#v", pass+1, expectedValue, evaluation.VariableValue)
				}
				var reason EvaluationReason
				if err := json.Unmarshal(testCase.Expected["reason"], &reason); err != nil {
					t.Fatal(err)
				}
				if evaluation.Reason != reason {
					t.Fatalf("pass %d reason: expected %s, got %s", pass+1, reason, evaluation.Reason)
				}
				checks := map[string]interface{}{
					"variableOverrideIndex": evaluation.VariableOverrideIndex,
					"variableOverrideKey":   evaluation.VariableOverrideKey,
					"variableOverridePath":  evaluation.VariableOverridePath,
					"variableOverridePaths": evaluation.VariableOverridePaths,
				}
				for key, actual := range checks {
					raw, exists := testCase.Expected[key]
					if !exists {
						continue
					}
					var expected interface{}
					if err := json.Unmarshal(raw, &expected); err != nil {
						t.Fatal(err)
					}
					encoded, err := json.Marshal(actual)
					if err != nil {
						t.Fatal(err)
					}
					actual = nil
					if err := json.Unmarshal(encoded, &actual); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(actual, expected) {
						t.Fatalf("pass %d %s: expected %#v, got %#v", pass+1, key, expected, actual)
					}
				}
			}
			if !reflect.DeepEqual(testCase.Datafile, originalDatafile) {
				t.Fatal("datafile input changed during repeated evaluation")
			}
			currentContext, err := json.Marshal(testCase.Context)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(currentContext, originalContext) {
				t.Fatal("context input changed during repeated evaluation")
			}
		})
	}
}

func TestSDKV4PrecomputedAndRuntimeProgramsStayEquivalentAcrossChildStateAndDatafileUpdates(t *testing.T) {
	content, err := os.ReadFile("conformance/sdk-v4-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture sdkV4Fixture
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	datafileFor := func(name string) json.RawMessage {
		t.Helper()
		for _, testCase := range fixture.Cases {
			if testCase.Name == name {
				return testCase.Datafile
			}
		}
		t.Fatalf("missing portable case %q", name)
		return nil
	}
	expectedChild := map[string]interface{}{
		"stage": "parent",
		"rows": []interface{}{
			map[string]interface{}{"id": float64(0), "tags": []interface{}{}},
			map[string]interface{}{"id": float64(1), "tags": []interface{}{"parent", "child"}},
		},
		"flags": []interface{}{"a"},
	}
	expectedParent := map[string]interface{}{
		"stage": "parent",
		"rows":  []interface{}{map[string]interface{}{"id": float64(1), "tags": []interface{}{"parent"}}},
		"flags": []interface{}{"a"},
	}
	expectedReset := map[string]interface{}{"stage": "reset", "rows": []interface{}{}, "flags": []interface{}{}}
	expectedDefault := map[string]interface{}{
		"stage": "default",
		"rows":  []interface{}{map[string]interface{}{"id": float64(1), "tags": []interface{}{}}},
		"flags": []interface{}{nil, "a"},
	}

	for _, initial := range []struct {
		name string
		next string
	}{
		{name: "first match precomputed stringified: child", next: "first match runtime stringified: child"},
		{name: "first match runtime stringified: child", next: "first match precomputed stringified: child"},
	} {
		t.Run(initial.name, func(t *testing.T) {
			root := CreateFeaturevisor(FeaturevisorOptions{
				Datafile: string(datafileFor(initial.name)),
				Context:  Context{"region": "eu", "eligible": true},
			})
			defer root.Close()
			child := root.Spawn(Context{"country": "nl"})
			defer child.Close()
			sticky := StickyVariables{"permission": false}
			stickyChild := root.Spawn(Context{"country": "nl"}, SpawnOptions{StickyVariables: &sticky})
			defer stickyChild.Close()

			if value := child.GetGlobalVariable("subject"); !reflect.DeepEqual(value, expectedChild) {
				t.Fatalf("child context did not select nested override: %#v", value)
			}
			if value := stickyChild.GetGlobalVariable("subject"); !reflect.DeepEqual(value, expectedParent) {
				t.Fatalf("child sticky variable did not select parent fallback: %#v", value)
			}
			child.SetContext(Context{"reset": true})
			if value := child.GetGlobalVariable("subject"); !reflect.DeepEqual(value, expectedReset) {
				t.Fatalf("child context update did not select literal reset: %#v", value)
			}

			root.SetDatafile(`{"schemaVersion":"2","revision":"cache-update","segments":{"europe":{"conditions":"[{\"attribute\":\"region\",\"operator\":\"equals\",\"value\":\"asia\"}]"}},"features":{},"variables":{}}`)
			if value := child.GetGlobalVariable("subject"); !reflect.DeepEqual(value, expectedDefault) {
				t.Fatalf("merged segment update did not invalidate selector state: %#v", value)
			}

			child.SetContext(Context{"reset": false})
			root.SetDatafile(string(datafileFor(initial.next)), true)
			if value := child.GetGlobalVariable("subject"); !reflect.DeepEqual(value, expectedChild) {
				t.Fatalf("existing child did not observe replacement representation: %#v", value)
			}
		})
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

func TestRequiredFeaturesEvaluationConformance(t *testing.T) {
	fixture := loadConformanceFixture(t).RequiredFeatures
	if len(fixture.EvaluationCases) == 0 {
		t.Fatal("missing required feature evaluation cases")
	}
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: fixture.Datafile, LogLevel: &level})
	defer instance.Close()
	for _, item := range fixture.EvaluationCases {
		t.Run(item.Name, func(t *testing.T) {
			var evaluation Evaluation
			switch item.Type {
			case EvaluationTypeFlag, EvaluationTypeVariation:
				if item.Feature == nil {
					t.Fatal("missing feature selector")
				}
				if item.Type == EvaluationTypeFlag {
					evaluation = instance.EvaluateFlag(*item.Feature, item.Context)
				} else {
					evaluation = instance.EvaluateVariation(*item.Feature, item.Context)
				}
			case EvaluationTypeVariable:
				if item.Feature == nil {
					evaluation = instance.EvaluateGlobalVariable(item.Variable, item.Context)
				} else {
					evaluation = instance.EvaluateVariable(*item.Feature, item.Variable, item.Context)
				}
			default:
				t.Fatalf("unknown evaluation type %q", item.Type)
			}
			encoded, err := json.Marshal(evaluation)
			if err != nil {
				t.Fatal(err)
			}
			var actual map[string]interface{}
			if err := json.Unmarshal(encoded, &actual); err != nil {
				t.Fatal(err)
			}
			if len(item.Expected) == 0 {
				t.Fatal("missing evaluation expectations")
			}
			for key, expected := range item.Expected {
				if value, exists := actual[key]; !exists || !reflect.DeepEqual(value, expected) {
					t.Errorf("%s: expected %#v, got %#v (present: %v)", key, expected, value, exists)
				}
			}
		})
	}
}

func TestRequiredFeatureGroupsConformance(t *testing.T) {
	fixture := loadConformanceFixture(t)
	for _, group := range fixture.RequiredFeatureGroups {
		t.Run(group.Name, func(t *testing.T) {
			for index, input := range fixture.RequiredFeatureGroupInputs {
				datafile := DatafileContent{SchemaVersion: "2", Revision: "groups", Segments: map[string]Segment{}, Features: map[string]Feature{
					"a":       {BucketBy: "userId", Traffic: []Traffic{{Key: "all", Segments: "*", Percentage: 100000}}},
					"b":       {BucketBy: "userId", Traffic: []Traffic{{Key: "all", Segments: "*", Percentage: 100000}}},
					"subject": {BucketBy: "userId", RequiredFeatures: group.Requirements, Traffic: []Traffic{{Key: "all", Segments: "*", Percentage: 100000}}},
				}}
				sticky := StickyFeatures{"a": {Enabled: input.A}, "b": {Enabled: input.B}}
				instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, StickyFeatures: &sticky})
				if actual := instance.IsEnabled("subject", Context{"userId": "u"}); actual != group.Results[index] {
					t.Fatalf("input %d: expected %v, got %v", index, group.Results[index], actual)
				}
			}
		})
	}
}

func TestRequiredVariablesConformance(t *testing.T) {
	fixture := loadConformanceFixture(t).RequiredVariables
	for _, testCase := range fixture.StickyCases {
		t.Run(testCase.Name, func(t *testing.T) {
			stickyFeatures, stickyVariables := testCase.StickyFeatures, testCase.StickyVariables
			instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: fixture.Datafile, StickyFeatures: &stickyFeatures, StickyVariables: &stickyVariables})
			if actual := instance.IsEnabled("gated", Context{"userId": "u"}); actual != testCase.Matched {
				t.Fatalf("gated: expected %v, got %v", testCase.Matched, actual)
			}
			expected := fixture.Expected.DefaultValue
			if testCase.Matched {
				expected = fixture.Expected.MatchedValue
			}
			if actual := instance.GetGlobalVariable("root"); !reflect.DeepEqual(actual, expected) {
				t.Fatalf("root: expected %#v, got %#v", expected, actual)
			}
			if actual := instance.GetGlobalVariable("override"); !reflect.DeepEqual(actual, expected) {
				t.Fatalf("override: expected %#v, got %#v", expected, actual)
			}
			if actual := instance.GetGlobalVariable("accumulated"); !reflect.DeepEqual(actual, testCase.Accumulated) {
				t.Fatalf("accumulated: expected %#v, got %#v", testCase.Accumulated, actual)
			}
			if actual := instance.GetVariable("featureOverride", "message", Context{"userId": "u"}); !reflect.DeepEqual(actual, expected) {
				t.Fatalf("feature override: expected %#v, got %#v", expected, actual)
			}
		})
	}
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: fixture.Datafile})
	if evaluation := instance.EvaluateGlobalVariable("root"); evaluation.Reason != fixture.Expected.UnmetReason || evaluation.Conditions == nil {
		t.Fatalf("unexpected unmet evaluation: %#v", evaluation)
	}
	if value := instance.GetGlobalVariable("nullGate"); !reflect.DeepEqual(value, fixture.Expected.MatchedValue) {
		t.Fatalf("malformed JSON null requirement did not match: %#v", value)
	}
}
