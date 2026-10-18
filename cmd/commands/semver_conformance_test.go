package commands

import (
	"encoding/json"
	"os"
	"testing"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

func TestPortableSemverChecks(t *testing.T) {
	var fixture struct {
		Kind    string
		Version int
		Cases   []struct {
			Name, Left, Right string
			Order             *int
		}
	}
	raw, err := os.ReadFile("../../conformance/semver.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Kind != "featurevisor.semver" || fixture.Version != 1 || len(fixture.Cases) != 44 {
		t.Fatal("Unsupported semver fixture")
	}
	for _, entry := range fixture.Cases {
		t.Run(entry.Name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				left, right, order := entry.Left, entry.Right, 0
				if entry.Order != nil {
					order = *entry.Order
				}
				if reverse {
					left, right, order = right, left, -order
				}
				for op, matched := range map[string]bool{
					"semverEquals": order == 0, "semverNotEquals": order != 0,
					"semverGreaterThan": order > 0, "semverGreaterThanOrEquals": order >= 0,
					"semverLessThan": order < 0, "semverLessThanOrEquals": order <= 0,
				} {
					for _, compareTo := range []bool{false, true} {
						for _, negated := range []bool{false, true} {
							check := map[string]interface{}{"variable": "left", "operator": op, "value": right}
							if compareTo {
								delete(check, "value")
								check["compareTo"] = map[string]interface{}{"variable": "right"}
							}
							if negated {
								check = map[string]interface{}{"not": []interface{}{check}}
							}
							datafile := featurevisor.DatafileContent{Variables: map[string]featurevisor.GlobalVariable{"left": {}, "right": {}}}
							evaluator := checkFixtureEvaluator{globals: map[string]interface{}{"left": left, "right": right}}
							outcome := evaluateAssertionChecksDetailed(map[string]interface{}{"checks": []interface{}{check}}, "", evaluator, datafile, "")
							if outcome.HadError != (entry.Order == nil) {
								t.Fatalf("%s reverse=%v: %+v", op, reverse, outcome)
							}
							want := entry.Order != nil && matched != negated
							if (len(outcome.Failures) == 0) != want {
								t.Fatalf("%s compareTo=%v negated=%v: %+v", op, compareTo, negated, outcome)
							}
							if entry.Order == nil && (len(outcome.ReportErrors) != 1 || outcome.ReportErrors[0].Type != "check") {
								t.Fatalf("Missing structured comparison error: %+v", outcome)
							}
						}
					}
				}
			}
		})
	}
}
