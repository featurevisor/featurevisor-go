package featurevisor

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"
)

func assertEvaluationFields(t *testing.T, evaluation Evaluation, expected map[string]interface{}) {
	t.Helper()
	if len(expected) == 0 {
		t.Fatal("missing expected evaluation fields")
	}
	raw, err := json.Marshal(evaluation)
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]interface{}
	if err := json.Unmarshal(raw, &actual); err != nil {
		t.Fatal(err)
	}
	if evaluation.Error != nil {
		actual["error"] = evaluation.Error.Error()
	}
	for key, want := range expected {
		if key == "value" {
			continue
		}
		got, present := actual[key]
		if !present {
			t.Errorf("missing evaluation field %s", key)
			continue
		}
		if !sameJSONValue(got, want) {
			t.Errorf("%s: got %#v, want %#v (evaluation: %s)", key, got, want, raw)
		}
	}
}

func TestUnifiedConditionsCapability(t *testing.T) {
	raw, err := os.ReadFile("conformance/sdk-conditions.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version    int
		Capability string
		Cases      []struct {
			Name, Type, Feature, Variable string
			Datafile                      DatafileContent
			Context                       Context
			Options                       struct {
				StickyFeatures  StickyFeatures
				StickyVariables StickyVariables
			}
			Dependencies []map[string]interface{}
			Expected     map[string]interface{}
		}
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 1 || fixture.Capability != "unified-conditions" || len(fixture.Cases) == 0 {
		t.Fatal("unsupported conditions capability")
	}
	executed := [2]int{}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Type != "flag" && tc.Type != "variable" {
				t.Fatalf("unsupported case type %q", tc.Type)
			}
			dependencies := []map[string]interface{}{}
			level := LogLevelFatal
			f := CreateFeaturevisor(FeaturevisorOptions{Datafile: tc.Datafile, LogLevel: &level, StickyFeatures: &tc.Options.StickyFeatures, StickyVariables: &tc.Options.StickyVariables,
				Modules: []*FeaturevisorModule{{BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
					if options.DependencyPurpose != "" {
						item := map[string]interface{}{"dependencyPurpose": options.DependencyPurpose}
						if options.FeatureKey != "" {
							item["feature"] = options.FeatureKey
						}
						if options.VariableKey != nil {
							item["variable"] = *options.VariableKey
						}
						dependencies = append(dependencies, item)
					}
					return options
				}}},
			})
			defer f.Close()
			for attempt := 0; attempt < 2; attempt++ {
				dependencies = []map[string]interface{}{}
				var evaluation Evaluation
				if tc.Type == "flag" {
					evaluation = f.EvaluateFlag(tc.Feature, tc.Context)
				} else {
					evaluation = f.EvaluateGlobalVariable(tc.Variable, tc.Context)
				}
				assertEvaluationFields(t, evaluation, tc.Expected)
				if tc.Dependencies != nil && !reflect.DeepEqual(dependencies, tc.Dependencies) {
					t.Errorf("dependencies: got %#v, want %#v", dependencies, tc.Dependencies)
				}
				if value, exists := tc.Expected["value"]; exists {
					if got := f.GetGlobalVariable(tc.Variable, tc.Context); !sameJSONValue(got, value) {
						t.Errorf("getter: got %#v, want %#v", got, value)
					}
				}
				if tc.Type == "flag" {
					if got := f.IsEnabled(tc.Feature, tc.Context); got != (tc.Expected["enabled"] == true) {
						t.Errorf("flag getter: %v", got)
					}
				}
				executed[attempt]++
			}
		})
	}
	for attempt, count := range executed {
		if count != len(fixture.Cases) {
			t.Errorf("repetition %d executed %d of %d cases", attempt+1, count, len(fixture.Cases))
		}
	}
}

func TestCompositionCapability(t *testing.T) {
	raw, err := os.ReadFile("conformance/sdk-composition.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version    int
		Capability string
		Datafile   DatafileContent
		Cases      []struct {
			Name, Variable     string
			Context            Context
			StickyFeatures     StickyFeatures
			StickyVariables    StickyVariables
			ExpectedValue      interface{}
			ExpectedEvaluation map[string]interface{}
		}
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 2 || fixture.Capability != "composition-and-overrides" || len(fixture.Cases) == 0 {
		t.Fatal("unsupported composition capability")
	}
	executed := [2]int{}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			before, _ := json.Marshal(fixture.Datafile)
			level := LogLevelFatal
			f := CreateFeaturevisor(FeaturevisorOptions{Datafile: fixture.Datafile, LogLevel: &level, StickyFeatures: &tc.StickyFeatures, StickyVariables: &tc.StickyVariables})
			defer f.Close()
			for attempt := 0; attempt < 2; attempt++ {
				if got := f.GetGlobalVariable(tc.Variable, tc.Context); !sameJSONValue(got, tc.ExpectedValue) {
					t.Errorf("getter: got %#v, want %#v", got, tc.ExpectedValue)
				}
				assertEvaluationFields(t, f.EvaluateGlobalVariable(tc.Variable, tc.Context), tc.ExpectedEvaluation)
				executed[attempt]++
			}
			wantPolicy := interface{}(map[string]interface{}{"limit": float64(50), "methods": []interface{}{"card"}})
			if v, exists := tc.StickyVariables["policy"]; exists && v == nil {
				wantPolicy = nil
			}
			if got := f.GetGlobalVariable("policy"); !reflect.DeepEqual(got, wantPolicy) {
				t.Errorf("source changed: %#v", got)
			}
			if _, mapCompose := fixture.Datafile.Variables[tc.Variable].Compose.([]interface{}); mapCompose && tc.ExpectedEvaluation["reason"] != "error" && tc.ExpectedEvaluation["reason"] != "sticky" {
				a, b := f.GetGlobalVariable(tc.Variable, tc.Context), f.GetGlobalVariable(tc.Variable, tc.Context)
				if reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer() {
					t.Error("map composition outer object is shared")
				}
			}
			after, _ := json.Marshal(fixture.Datafile)
			if string(before) != string(after) {
				t.Error("caller datafile changed")
			}
		})
	}
	for attempt, count := range executed {
		if count != len(fixture.Cases) {
			t.Errorf("repetition %d executed %d of %d cases", attempt+1, count, len(fixture.Cases))
		}
	}
}

func sameJSONValue(a, b interface{}) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}

func TestDiagnosticsCapability(t *testing.T) {
	raw, err := os.ReadFile("conformance/sdk-diagnostics.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Kind    string
		Version int
		Cases   []struct {
			Name       string
			Operations []struct {
				Action, Feature         string
				Datafile                json.RawMessage
				Replace, ExpectedResult bool
				Diagnostics             []map[string]interface{}
			}
		}
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Kind != "featurevisor.sdk-diagnostics" || fixture.Version != 1 || len(fixture.Cases) == 0 {
		t.Fatal("unsupported diagnostics fixture")
	}
	canonical := func(rows []map[string]interface{}) []string {
		result := make([]string, 0, len(rows))
		for _, row := range rows {
			encoded, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			result = append(result, string(encoded))
		}
		sort.Strings(result)
		return result
	}
	for repetition := 0; repetition < 2*len(fixture.Cases); repetition++ {
		tc := fixture.Cases[repetition%len(fixture.Cases)]
		t.Run(tc.Name, func(t *testing.T) {
			seen := []map[string]interface{}{}
			var f *Featurevisor
			reentering := false
			level := LogLevelError
			f = CreateFeaturevisor(FeaturevisorOptions{LogLevel: &level, OnDiagnostic: func(d FeaturevisorDiagnostic) {
				if d.Code != "conditions_parse_error" {
					return
				}
				seen = append(seen, map[string]interface{}{"code": d.Code, "level": d.Level, "details": d.Details, "revision": f.GetRevision()})
				if d.Details["entityType"] == "feature" && !reentering {
					reentering = true
					before := len(seen)
					f.IsEnabled(d.Details["entityKey"].(string))
					if len(seen) != before {
						t.Error("reentry emitted diagnostics")
					}
					reentering = false
				}
			}})
			defer f.Close()
			if len(tc.Operations) == 0 {
				t.Fatal("empty operations")
			}
			for index, op := range tc.Operations {
				before := len(seen)
				var result bool
				switch op.Action {
				case "setDatafile":
					result = f.SetDatafile(string(op.Datafile), op.Replace)
				case "isEnabled":
					result = f.IsEnabled(op.Feature)
				default:
					t.Fatalf("unsupported action %q", op.Action)
				}
				if result != op.ExpectedResult {
					t.Errorf("operation %d result: %v", index, result)
				}
				if !reflect.DeepEqual(canonical(seen[before:]), canonical(op.Diagnostics)) {
					t.Errorf("operation %d diagnostics: got %#v, want %#v", index, seen[before:], op.Diagnostics)
				}
			}
		})
	}
}
