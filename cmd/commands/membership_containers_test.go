package commands

import (
	"encoding/json"
	"testing"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

func TestMembershipCheckNilArrayOperands(t *testing.T) {
	for _, op := range []string{"includes", "notIncludes", "includesAny", "notIncludesAny", "in", "notIn"} {
		t.Run(op, func(t *testing.T) {
			left, right := interface{}([]interface{}(nil)), interface{}("red")
			if op == "includesAny" || op == "notIncludesAny" {
				right = []interface{}{"red"}
			}
			if op == "in" || op == "notIn" {
				left, right = "red", []interface{}(nil)
			}
			if matched, err := matchCheckValues(left, op, right, nil); err == nil || matched {
				t.Errorf("typed nil array operand must error, got %v %v", matched, err)
			}
			if op == "in" || op == "notIn" || op == "includesAny" || op == "notIncludesAny" {
				if err := validateCheckLiteral(op, []interface{}(nil), nil); err == nil {
					t.Error("typed nil literal array was accepted")
				}
			}
			if op == "includesAny" || op == "notIncludesAny" {
				if matched, err := matchCheckValues([]interface{}{}, op, []interface{}(nil), nil); err == nil || matched {
					t.Errorf("typed nil candidate array must error, got %v %v", matched, err)
				}
			}
		})
	}
}

func TestMembershipCheckNativeGetterContainers(t *testing.T) {
	var datafile featurevisor.DatafileContent
	if err := json.Unmarshal([]byte(`{"schemaVersion":"2","revision":"containers","segments":{},"features":{},"variables":{"subject":{"type":"array","defaultValue":[]},"other":{"type":"array","defaultValue":[]}}}`), &datafile); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		name         string
		actual       interface{}
		valid, found bool
	}{
		{"nil strings", []string(nil), false, false},
		{"nil interfaces", []interface{}(nil), false, false},
		{"nil numbers", []int(nil), false, false},
		{"empty strings", []string{}, true, false},
		{"empty interfaces", []interface{}{}, true, false},
		{"empty fixed array", [0]string{}, true, false},
		{"string slice", []string{"red"}, true, false},
		{"numeric slice", []int{1}, true, true},
		{"numeric fixed array", [1]int{1}, true, true},
		{"boolean fixed array", [1]bool{true}, true, false},
	}
	for _, row := range rows {
		for _, op := range []string{"includes", "notIncludes", "includesAny", "notIncludesAny"} {
			for _, compareTo := range []bool{false, true} {
				t.Run(row.name+"/"+op+map[bool]string{false: "/literal", true: "/compareTo"}[compareTo], func(t *testing.T) {
					candidate := interface{}(float64(1))
					if op == "includesAny" || op == "notIncludesAny" {
						candidate = []interface{}{float64(1)}
					}
					sticky := featurevisor.StickyVariables{"subject": row.actual, "other": candidate}
					level := featurevisor.LogLevelFatal
					diagnostics := &checkDiagnosticCollector{}
					sdk := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{Datafile: datafile, StickyVariables: &sticky, LogLevel: &level, OnDiagnostic: diagnostics.record})
					defer sdk.Close()
					leaf := map[string]interface{}{"variable": "subject", "operator": op}
					if compareTo {
						leaf["compareTo"] = map[string]interface{}{"variable": "other"}
					} else {
						leaf["value"] = candidate
					}
					resolver := nativeCheckResolver{getter: sdk, diagnostics: diagnostics}
					assertion := map[string]interface{}{"checks": []interface{}{leaf}}
					result := evaluateAssertionChecksDetailed(assertion, "", resolver, datafile, "")
					want := row.valid && (row.found == (op == "includes" || op == "includesAny"))
					if result.HadError != !row.valid || (len(result.Failures) == 0) != want {
						t.Fatalf("got %#v want pass=%v error=%v", result, want, !row.valid)
					}
					if !row.valid {
						if len(result.ReportErrors) == 0 {
							t.Fatal("missing structured error")
						}
						for _, group := range []string{"not", "or"} {
							grouped := map[string]interface{}{"checks": []interface{}{map[string]interface{}{group: []interface{}{leaf}}}}
							outcome := evaluateAssertionChecksDetailed(grouped, "", resolver, datafile, "")
							if !outcome.HadError || len(outcome.Failures) == 0 {
								t.Fatalf("%s concealed error: %#v", group, outcome)
							}
						}
					}
				})
			}
		}
	}
}

func TestMembershipCheckNativeCandidateContainers(t *testing.T) {
	var datafile featurevisor.DatafileContent
	if err := json.Unmarshal([]byte(`{"schemaVersion":"2","revision":"candidates","segments":{},"features":{},"variables":{"subject":{"type":"array","defaultValue":[]},"other":{"type":"array","defaultValue":[]}}}`), &datafile); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		name         string
		candidates   interface{}
		valid, found bool
	}{
		{"nil strings", []string(nil), false, false},
		{"nil interfaces", []interface{}(nil), false, false},
		{"nil numbers", []int(nil), false, false},
		{"empty strings", []string{}, true, false},
		{"empty interfaces", []interface{}{}, true, false},
		{"empty fixed array", [0]int{}, true, false},
		{"numeric slice", []int{1}, true, true},
		{"numeric fixed array", [1]int{1}, true, true},
		{"boolean slice", []bool{true}, true, false},
		{"string slice", []string{"1"}, true, false},
	}
	for _, row := range rows {
		for _, op := range []string{"in", "notIn", "includesAny", "notIncludesAny"} {
			t.Run(row.name+"/"+op, func(t *testing.T) {
				actual := interface{}(float64(1))
				if op == "includesAny" || op == "notIncludesAny" {
					actual = []int{1}
				}
				sticky := featurevisor.StickyVariables{"subject": actual, "other": row.candidates}
				level := featurevisor.LogLevelFatal
				diagnostics := &checkDiagnosticCollector{}
				sdk := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{Datafile: datafile, StickyVariables: &sticky, LogLevel: &level, OnDiagnostic: diagnostics.record})
				defer sdk.Close()
				resolver := nativeCheckResolver{getter: sdk, diagnostics: diagnostics}
				leaf := map[string]interface{}{"variable": "subject", "operator": op, "compareTo": map[string]interface{}{"variable": "other"}}
				result := evaluateAssertionChecksDetailed(map[string]interface{}{"checks": []interface{}{leaf}}, "", resolver, datafile, "")
				want := row.valid && (row.found == (op == "in" || op == "includesAny"))
				if result.HadError != !row.valid || (len(result.Failures) == 0) != want {
					t.Fatalf("got %#v want pass=%v error=%v", result, want, !row.valid)
				}
				if !row.valid {
					if len(result.ReportErrors) == 0 {
						t.Fatal("missing structured comparison error")
					}
					for _, group := range []string{"not", "or"} {
						grouped := map[string]interface{}{"checks": []interface{}{map[string]interface{}{group: []interface{}{leaf}}}}
						result := evaluateAssertionChecksDetailed(grouped, "", resolver, datafile, "")
						if !result.HadError || len(result.Failures) == 0 {
							t.Fatalf("%s concealed comparison error: %#v", group, result)
						}
					}
				}
			})
		}
	}
}
