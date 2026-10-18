package featurevisor

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestIncludesAnyPrimitiveArrays(t *testing.T) {
	tests := []struct {
		name               string
		actual, candidates interface{}
		valid, found       bool
	}{
		{"mixed", []interface{}{"red", 1, true, nil}, []interface{}{"absent", 1.0}, true, true},
		{"null", []interface{}{nil}, []interface{}{nil}, true, true},
		{"boolean", []interface{}{false}, []interface{}{false}, true, true},
		{"number not boolean", []interface{}{1}, []interface{}{true}, true, false},
		{"boolean not number", []interface{}{true}, []interface{}{1}, true, false},
		{"string not number", []interface{}{"1"}, []interface{}{1}, true, false},
		{"string not boolean", []interface{}{"true"}, []interface{}{true}, true, false},
		{"duplicates", []string{"red", "red"}, []string{"blue", "red", "red"}, true, true},
		{"typed numbers", [2]int{2, 1}, []float64{1}, true, true},
		{"empty actual", []interface{}{}, []interface{}{nil}, true, false},
		{"empty candidates", []interface{}{1}, []interface{}{}, true, false},
		{"both empty", []interface{}{}, []interface{}{}, true, false},
		{"structured actual", []interface{}{map[string]interface{}{}, []interface{}{1}, "red"}, []interface{}{"red"}, true, true},
		{"structured actual ignored", []interface{}{map[string]interface{}{}, []interface{}{1}}, []interface{}{nil, 1}, true, false},
		{"object candidate after match", []interface{}{1}, []interface{}{1, map[string]interface{}{}}, false, false},
		{"array candidate after match", []interface{}{1}, []interface{}{1, []interface{}{}}, false, false},
		{"object candidate first", []interface{}{1}, []interface{}{map[string]interface{}{}, 1}, false, false},
		{"null actual", nil, []interface{}{}, false, false},
		{"typed nil actual", []string(nil), []interface{}{}, false, false},
		{"map actual", map[string]interface{}{}, []interface{}{}, false, false},
		{"scalar actual", "red", []interface{}{"red"}, false, false},
		{"null candidates", []interface{}{}, nil, false, false},
		{"typed nil candidates", []interface{}{}, []string(nil), false, false},
		{"scalar candidates", []interface{}{"red"}, "red", false, false},
		{"map candidates", []interface{}{}, map[string]interface{}{}, false, false},
	}
	regex := func(string, string) *regexp.Regexp { return nil }
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, op := range []Operator{OperatorIncludesAny, OperatorNotIncludesAny} {
				condition := PlainCondition{Attribute: "nested.items", Operator: op, Value: conditionValue(test.candidates)}
				want := test.valid && (test.found == (op == OperatorIncludesAny))
				if got := conditionIsMatched(condition, Context{"nested": map[string]interface{}{"items": test.actual}}, regex); got != want {
					t.Fatalf("%s got %v want %v", op, got, want)
				}
				if conditionIsMatched(condition, Context{}, regex) {
					t.Fatal("missing path matched")
				}
				if !test.valid {
					context := Context{"nested": map[string]interface{}{"items": test.actual}}
					if !AllConditionsAreMatched(NotCondition{Not: []Condition{condition}}, context) {
						t.Fatal("logical NOT did not negate an invalid comparison")
					}
					if AllConditionsAreMatched(OrCondition{Or: []Condition{condition}}, context) {
						t.Fatal("logical OR matched an invalid comparison")
					}
				}
			}
		})
	}
}

func TestIncludesAnyRequirementsAndLogicalGroups(t *testing.T) {
	for _, test := range []struct {
		op, candidates string
		want           bool
	}{
		{"includesAny", "[1.0]", true},
		{"notIncludesAny", "[false]", true},
		{"includesAny", "[true]", false},
		{"notIncludesAny", "[1]", false},
		{"includesAny", "[]", false},
		{"notIncludesAny", "[]", true},
		{"includesAny", "[1,{}]", false},
		{"notIncludesAny", "[1,[]]", false},
	} {
		t.Run(test.op+test.candidates, func(t *testing.T) {
			raw := strings.ReplaceAll(strings.ReplaceAll(`{
				"schemaVersion":"2","revision":"any","segments":{},
				"features":{"gate":{"bucketBy":"userId","conditions":[{"variable":"source","path":["items"],"operator":"OP","value":CANDIDATES}],"traffic":[{"key":"all","segments":"*","percentage":100000,"enabled":true}]}},
				"variables":{"source":{"type":"object","defaultValue":{"items":[1,null,{"x":1}]}},
				"gate":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","path":["items"],"operator":"OP","value":CANDIDATES}]}}
			}`, "OP", test.op), "CANDIDATES", test.candidates)
			var datafile DatafileContent
			if err := json.Unmarshal([]byte(raw), &datafile); err != nil {
				t.Fatal(err)
			}
			level := LogLevelFatal
			sdk := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, LogLevel: &level})
			defer sdk.Close()
			if got := sdk.IsEnabled("gate", Context{"userId": "user"}); got != test.want {
				t.Fatalf("feature got %v", got)
			}
			if got := sdk.GetGlobalVariableBoolean("gate", Context{}); got == nil || *got != test.want {
				t.Fatalf("global got %v", got)
			}
		})
	}
}
