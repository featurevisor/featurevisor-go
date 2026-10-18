package commands

import (
	"encoding/json"
	"reflect"
	"testing"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

func TestOrdinaryAssertionReportsRetainTypesPathsAndValues(t *testing.T) {
	var datafile featurevisor.DatafileContent
	if err := json.Unmarshal([]byte(`{"schemaVersion":"2","revision":"reports","segments":{},"features":{"subject":{"bucketBy":"userId","variablesSchema":{"a.b":{"type":"object","defaultValue":{"actual":0}}},"traffic":[{"key":"all","segments":"*","percentage":100000,"enabled":true}]}},"variables":{"subject":{"type":"object","defaultValue":{"actual":0}}}}`), &datafile); err != nil {
		t.Fatal(err)
	}
	sdk := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{Datafile: datafile})
	defer sdk.Close()
	feature := map[string]interface{}{
		"expectedToBeEnabled": false, "expectedVariation": "café 日本 🚀",
		"expectedVariables": map[string]interface{}{"a.b": map[string]interface{}{"expected": false}},
		"expectedEvaluations": map[string]interface{}{
			"flag":      map[string]interface{}{"reason": "wrong"},
			"variation": map[string]interface{}{"reason": "wrong"},
			"variables": map[string]interface{}{"a.b": map[string]interface{}{"reason": "wrong"}},
		},
	}
	child := cloneAssertion(feature)
	feature["children"] = []interface{}{child}
	result := RunTestFeature(feature, "subject", sdk, "error")
	errors := reportErrorsFromResult("feature", result, 7)
	if !result.HasError || len(errors) != 12 {
		t.Fatalf("Expected twelve distinct failures, got %+v: %s", errors, result.Errors)
	}
	type expectedError struct {
		kind             string
		path             []interface{}
		expected, actual interface{}
	}
	expectations := []expectedError{
		{"flag", []interface{}{"expectedToBeEnabled"}, false, true},
		{"variation", []interface{}{"expectedVariation"}, "café 日本 🚀", nil},
		{"variable", []interface{}{"expectedVariables", "a.b"}, map[string]interface{}{"expected": false}, map[string]interface{}{"actual": float64(0)}},
	}
	for _, base := range [][]interface{}{{"assertions", 7}, {"assertions", 7, "children", 0}} {
		for _, want := range expectations {
			assertReportError(t, errors, want.kind, appendPath(base, want.path...), want.expected, want.actual)
		}
		for _, path := range [][]interface{}{{"expectedEvaluations", "flag", "reason"}, {"expectedEvaluations", "variation", "reason"}, {"expectedEvaluations", "variables", "a.b", "reason"}} {
			found := false
			for _, failure := range errors {
				if reflect.DeepEqual(failure.Path, appendPath(base, path...)) {
					found = true
					if failure.Type != "evaluation" || failure.Expected == nil || *failure.Expected != "wrong" || failure.Actual == nil || *failure.Actual == "wrong" {
						t.Fatalf("Invalid evaluation failure: %+v", failure)
					}
				}
			}
			if !found {
				t.Fatalf("Missing evaluation path: %+v", path)
			}
		}
	}
	global := map[string]interface{}{
		"expectedValue": nil, "expectedEvaluation": map[string]interface{}{"variableValue": false},
		"checks": []interface{}{map[string]interface{}{"path": "actual", "operator": "equals", "value": float64(1)}},
	}
	global["children"] = []interface{}{map[string]interface{}{"expectedValue": nil, "expectedEvaluation": map[string]interface{}{"variableValue": false}}}
	result = RunTestVariable(global, "subject", sdk, &checkDiagnosticCollector{}, datafile, nil, "error")
	errors = reportErrorsFromResult("variable", result, 9)
	if len(errors) != 5 {
		t.Fatalf("Expected ordinary and check failures together: %+v", errors)
	}
	for _, base := range [][]interface{}{{"assertions", 9}, {"assertions", 9, "children", 0}} {
		assertReportError(t, errors, "variable", appendPath(base, "expectedValue"), nil, map[string]interface{}{"actual": float64(0)})
		assertReportError(t, errors, "evaluation", appendPath(base, "expectedEvaluation", "variableValue"), false, map[string]interface{}{"actual": float64(0)})
	}
	segment := RunTestSegment(map[string]interface{}{"expectedToMatch": false}, map[string]interface{}{"conditions": "*"}, "error")
	assertReportError(t, reportErrorsFromResult("segment", segment, 3), "segment", []interface{}{"assertions", 3, "expectedToMatch"}, false, true)
}

func assertReportError(t *testing.T, errors []reportError, kind string, path []interface{}, expected, actual interface{}) {
	t.Helper()
	for _, failure := range errors {
		if !reflect.DeepEqual(failure.Path, path) {
			continue
		}
		if failure.Type != kind || failure.Expected == nil || failure.Actual == nil || !reflect.DeepEqual(*failure.Expected, expected) || !reflect.DeepEqual(*failure.Actual, actual) || failure.Message == "Native assertion failed" {
			t.Fatalf("Incorrect report error: %+v", failure)
		}
		raw, err := json.Marshal(failure)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if _, ok := decoded["expected"]; !ok {
			t.Fatalf("Expected omitted: %s", raw)
		}
		if _, ok := decoded["actual"]; !ok {
			t.Fatalf("Actual omitted: %s", raw)
		}
		return
	}
	t.Fatalf("Missing %s failure at %+v in %+v", kind, path, errors)
}
