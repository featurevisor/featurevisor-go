package commands

import (
	"encoding/json"
	"testing"
)

func TestIncludesAnyChecksLiteralCompareToAndErrors(t *testing.T) {
	var cases []struct {
		Name               string
		Actual, Candidates interface{}
		Valid, Found       bool
	}
	if err := json.Unmarshal([]byte(`[{"name":"mixed","actual":["red",1,true,null],"candidates":["absent",1],"valid":true,"found":true},{"name":"null","actual":[null],"candidates":[null],"valid":true,"found":true},{"name":"boolean","actual":[false],"candidates":[false],"valid":true,"found":true},{"name":"number not bool","actual":[1],"candidates":[true],"valid":true,"found":false},{"name":"bool not number","actual":[true],"candidates":[1],"valid":true,"found":false},{"name":"string not number","actual":["1"],"candidates":[1],"valid":true,"found":false},{"name":"string not bool","actual":["true"],"candidates":[true],"valid":true,"found":false},{"name":"duplicates","actual":["red","red"],"candidates":["blue","red","red"],"valid":true,"found":true},{"name":"empty actual","actual":[],"candidates":[null],"valid":true,"found":false},{"name":"empty candidate","actual":[1],"candidates":[],"valid":true,"found":false},{"name":"both empty","actual":[],"candidates":[],"valid":true,"found":false},{"name":"structured actual","actual":[{},[1],"red"],"candidates":["red"],"valid":true,"found":true},{"name":"structured ignored","actual":[{},[1]],"candidates":[null,1],"valid":true,"found":false},{"name":"object after match","actual":[1],"candidates":[1,{}],"valid":false,"found":false},{"name":"array after match","actual":[1],"candidates":[1,[]],"valid":false,"found":false},{"name":"object first","actual":[1],"candidates":[{},1],"valid":false,"found":false},{"name":"null actual","actual":null,"candidates":[],"valid":false,"found":false},{"name":"object actual","actual":{},"candidates":[],"valid":false,"found":false},{"name":"scalar actual","actual":"red","candidates":["red"],"valid":false,"found":false},{"name":"null candidates","actual":[],"candidates":null,"valid":false,"found":false},{"name":"scalar candidates","actual":["red"],"candidates":"red","valid":false,"found":false},{"name":"object candidates","actual":[],"candidates":{},"valid":false,"found":false}]`), &cases); err != nil {
		t.Fatal(err)
	}
	for _, row := range cases {
		for _, op := range []string{"includesAny", "notIncludesAny"} {
			for _, compareTo := range []bool{false, true} {
				t.Run(row.Name+op+map[bool]string{false: " literal", true: " compareTo"}[compareTo], func(t *testing.T) {
					leaf := map[string]interface{}{"variable": "subject", "path": "items", "operator": op}
					if compareTo {
						leaf["compareTo"] = map[string]interface{}{"variable": "other"}
					} else {
						leaf["value"] = row.Candidates
					}
					resolver := checkFixtureEvaluator{globals: map[string]interface{}{"subject": map[string]interface{}{"items": row.Actual}, "other": row.Candidates}}
					want := row.Valid && (row.Found == (op == "includesAny"))
					outcome := evaluateAssertionChecksDetailed(map[string]interface{}{"checks": []interface{}{leaf}}, "", resolver, checkFixtureDatafile(), "")
					if outcome.HadError == row.Valid || (len(outcome.Failures) == 0) != want {
						t.Fatalf("got %#v want pass=%v error=%v", outcome, want, !row.Valid)
					}
					if !row.Valid && len(outcome.ReportErrors) == 0 {
						t.Fatal("missing structured error")
					}
					if !row.Valid {
						for _, group := range []string{"not", "or"} {
							grouped := map[string]interface{}{group: []interface{}{leaf}}
							result := evaluateAssertionChecksDetailed(map[string]interface{}{"checks": []interface{}{grouped}}, "", resolver, checkFixtureDatafile(), "")
							if !result.HadError || len(result.Failures) == 0 {
								t.Fatalf("%s concealed error: %#v", group, result)
							}
						}
					}
				})
			}
		}
	}
}

func TestIncludesAnyChecksMissingPathsAndSkippedOperands(t *testing.T) {
	resolver := checkFixtureEvaluator{globals: map[string]interface{}{"subject": map[string]interface{}{}, "other": map[string]interface{}{}}}
	for _, op := range []string{"includesAny", "notIncludesAny"} {
		leaf := map[string]interface{}{"variable": "subject", "path": "missing", "operator": op, "value": []interface{}{}}
		result := evaluateAssertionChecksDetailed(map[string]interface{}{"checks": []interface{}{leaf}}, "", resolver, checkFixtureDatafile(), "")
		if result.HadError || len(result.Failures) == 0 {
			t.Fatalf("missing changed semantics: %#v", result)
		}
		skipped := map[string]interface{}{"or": []interface{}{map[string]interface{}{"variable": "subject", "operator": "exists"}, map[string]interface{}{"variable": "subject", "operator": op, "compareTo": map[string]interface{}{"variable": "other"}}}}
		result = evaluateAssertionChecksDetailed(map[string]interface{}{"checks": []interface{}{skipped}}, "", resolver, checkFixtureDatafile(), "")
		if result.HadError || len(result.Failures) != 0 {
			t.Fatalf("skipped operand evaluated: %#v", result)
		}
	}
}
