package featurevisor

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestPortableSemver(t *testing.T) {
	var fixture struct {
		Kind    string `json:"kind"`
		Version int    `json:"version"`
		Cases   []struct {
			Name, Left, Right string
			Order             *int
		} `json:"cases"`
	}
	content, err := os.ReadFile("conformance/semver.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Kind != "featurevisor.semver" || fixture.Version != 1 || len(fixture.Cases) != 44 {
		t.Fatal("Unsupported semver fixture")
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				a, b := c.Left, c.Right
				if reverse {
					a, b = b, a
				}
				order := 0
				if c.Order != nil {
					order = *c.Order
					if reverse {
						order = -order
					}
				}
				for op, matched := range map[string]bool{
					"semverEquals": order == 0, "semverNotEquals": order != 0,
					"semverGreaterThan": order > 0, "semverGreaterThanOrEquals": order >= 0,
					"semverLessThan": order < 0, "semverLessThanOrEquals": order <= 0,
				} {
					label := fmt.Sprintf("%s reverse=%v", op, reverse)
					expected := c.Order != nil && matched
					condition := map[string]interface{}{"attribute": "version", "operator": op, "value": b}
					if got := AllConditionsAreMatched(condition, Context{"version": a}); got != expected {
						t.Fatalf("%s condition: got %v, want %v", label, got, expected)
					}
					for _, featureSource := range []bool{false, true} {
						for _, negated := range []bool{false, true} {
							leaf := map[string]interface{}{"variable": "source", "operator": op, "value": b}
							if featureSource {
								leaf["feature"] = "sourceFeature"
							}
							var requirement interface{} = leaf
							if negated {
								requirement = map[string]interface{}{"not": []interface{}{requirement}}
							}
							raw, err := json.Marshal(map[string]interface{}{
								"schemaVersion": "2", "revision": "semver", "segments": map[string]interface{}{},
								"features": map[string]interface{}{
									"sourceFeature": map[string]interface{}{
										"bucketBy":        "userId",
										"variablesSchema": map[string]interface{}{"source": map[string]interface{}{"type": "string", "defaultValue": a}},
										"traffic":         []interface{}{map[string]interface{}{"key": "all", "segments": "*", "percentage": 100000, "enabled": true}},
									},
									"gate": map[string]interface{}{
										"bucketBy": "userId", "conditions": []interface{}{requirement},
										"traffic": []interface{}{map[string]interface{}{"key": "all", "segments": "*", "percentage": 100000, "enabled": true}},
									}},
								"variables": map[string]interface{}{
									"source": map[string]interface{}{"type": "string", "defaultValue": a},
									"gate":   map[string]interface{}{"type": "boolean", "defaultValue": true, "disabledValue": false, "conditions": []interface{}{requirement}},
								},
							})
							if err != nil {
								t.Fatal(err)
							}
							var datafile DatafileContent
							if err := json.Unmarshal(raw, &datafile); err != nil {
								t.Fatal(err)
							}
							level := LogLevelFatal
							sdk := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, LogLevel: &level})
							flag, variable := sdk.EvaluateFlag("gate"), sdk.EvaluateGlobalVariable("gate")
							sdk.Close()
							if c.Order == nil {
								if flag.Reason != EvaluationReasonError || variable.Reason != EvaluationReasonError {
									t.Fatalf("%s invalid requirements did not error: %+v %+v", label, flag, variable)
								}
							} else {
								want := matched != negated
								if flag.Reason == EvaluationReasonError || variable.Reason == EvaluationReasonError || flag.Enabled == nil || *flag.Enabled != want || variable.VariableValue != want {
									t.Fatalf("%s negated=%v requirements: %+v %+v, want %v", label, negated, flag, variable, want)
								}
							}
						}
					}
				}
				actual, err := compareVersions(a, b)
				if c.Order == nil {
					if err == nil {
						t.Fatalf("Expected invalid version: %q %q", a, b)
					}
				} else {
					expected := *c.Order
					if reverse {
						expected = -expected
					}
					if err != nil || actual != expected {
						t.Fatalf("%q %q: got %d, %v; expected %d", a, b, actual, err, expected)
					}
				}
			}
		})
	}
}
