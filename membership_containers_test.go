package featurevisor

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestMembershipNativeContainersAndGetters(t *testing.T) {
	cases := []struct {
		name           string
		actual, needle interface{}
		valid, found   bool
	}{
		{"nil strings", []string(nil), "red", false, false},
		{"nil interfaces", []interface{}(nil), nil, false, false},
		{"nil numbers", []int(nil), 1, false, false},
		{"nil", nil, "red", false, false},
		{"empty strings", []string{}, "red", true, false},
		{"empty interfaces", []interface{}{}, nil, true, false},
		{"empty numbers", []int{}, 1, true, false},
		{"empty fixed array", [0]string{}, "red", true, false},
		{"string slice hit", []string{"blue", "red"}, "red", true, true},
		{"interface slice hit", []interface{}{nil, true, "red"}, "red", true, true},
		{"fixed string hit", [2]string{"blue", "red"}, "red", true, true},
		{"numeric slice hit", []int{2, 1}, float64(1), true, true},
		{"fixed numeric hit", [2]int{2, 1}, float64(1), true, true},
		{"float slice hit", []float32{1}, int64(1), true, true},
		{"number not bool", []int{1}, true, true, false},
		{"bool not number", []bool{true}, 1, true, false},
		{"string not number", []string{"1"}, 1, true, false},
		{"fixed mixed null", [3]interface{}{false, 0, nil}, nil, true, true},
		{"fixed mixed boolean", [3]interface{}{false, 0, nil}, false, true, true},
	}
	for _, op := range []Operator{OperatorIncludes, OperatorNotIncludes, OperatorIncludesAny, OperatorNotIncludesAny} {
		for _, row := range cases {
			t.Run(string(op)+"/"+row.name, func(t *testing.T) {
				expected := row.needle
				if op == OperatorIncludesAny || op == OperatorNotIncludesAny {
					expected = []interface{}{row.needle}
				}
				want := row.valid && (row.found == (op == OperatorIncludes || op == OperatorIncludesAny))
				condition := PlainCondition{Attribute: "nested.items", Operator: op, Value: conditionValue(expected)}
				context := Context{"userId": "user", "nested": map[string]interface{}{"items": row.actual}}
				if got := AllConditionsAreMatched(condition, context); got != want {
					t.Errorf("condition got %v want %v", got, want)
				}
				valueJSON, err := json.Marshal(expected)
				if err != nil {
					t.Fatal(err)
				}
				raw := fmt.Sprintf(`{
					"schemaVersion":"2","revision":"containers",
					"segments":{"membership":{"conditions":{"attribute":"nested.items","operator":%q,"value":%s}}},
					"features":{
						"conditioned":{"bucketBy":"userId","traffic":[{"key":"all","segments":["membership"],"percentage":100000,"enabled":true}]},
						"required":{"bucketBy":"userId","conditions":[{"variable":"source","operator":%q,"value":%s}],"traffic":[{"key":"all","segments":"*","percentage":100000,"enabled":true}]}
					},
					"variables":{
						"source":{"type":"array","defaultValue":[]},
						"required":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","operator":%q,"value":%s}]}
					}
				}`, op, valueJSON, op, valueJSON, op, valueJSON)
				var datafile DatafileContent
				if err := json.Unmarshal([]byte(raw), &datafile); err != nil {
					t.Fatal(err)
				}
				sticky := StickyVariables{"source": row.actual}
				level := LogLevelFatal
				sdk := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, StickyVariables: &sticky, LogLevel: &level})
				defer sdk.Close()
				for _, key := range []string{"conditioned", "required"} {
					if got := sdk.IsEnabled(key, context); got != want {
						t.Errorf("%s getter got %v want %v", key, got, want)
					}
				}
				if got := sdk.GetGlobalVariableBoolean("required"); got == nil || *got != want {
					t.Errorf("required global getter got %v want %v", got, want)
				}
				actualJSON, _ := json.Marshal(row.actual)
				getterJSON, err := json.Marshal(sdk.GetGlobalVariable("source"))
				if err != nil || string(getterJSON) != string(actualJSON) {
					t.Fatalf("native getter changed JSON identity: got %s want %s, error %v", getterJSON, actualJSON, err)
				}
			})
		}
	}
}

func TestMembershipTypedNilCandidatesAreNotArrays(t *testing.T) {
	for _, candidates := range []interface{}{[]string(nil), []interface{}(nil), []int(nil)} {
		encoded, err := json.Marshal(candidates)
		if err != nil || string(encoded) != "null" {
			t.Fatalf("typed nil must serialize as null: %s %v", encoded, err)
		}
		for _, op := range []Operator{OperatorIn, OperatorNotIn, OperatorIncludesAny, OperatorNotIncludesAny} {
			actual := interface{}("red")
			if op == OperatorIncludesAny || op == OperatorNotIncludesAny {
				actual = []string{"red"}
			}
			if AllConditionsAreMatched(PlainCondition{Attribute: "value", Operator: op, Value: conditionValue(candidates)}, Context{"value": actual}) {
				t.Errorf("%s matched null candidate array %T", op, candidates)
			}
		}
	}
}
