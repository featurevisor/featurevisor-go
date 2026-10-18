package featurevisor

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestNullPredicatesDistinguishTypedNilAndEmptyContainers(t *testing.T) {
	type strings []string
	type object map[string]interface{}
	zero := 0
	rows := []struct {
		name  string
		value interface{}
		null  bool
	}{
		{"nil", nil, true},
		{"nil strings", []string(nil), true},
		{"nil interfaces", []interface{}(nil), true},
		{"nil named slice", strings(nil), true},
		{"nil map", map[string]interface{}(nil), true},
		{"nil named map", object(nil), true},
		{"nil pointer", (*string)(nil), true},
		{"allocated empty strings", []string{}, false},
		{"allocated empty interfaces", []interface{}{}, false},
		{"allocated named slice", strings{}, false},
		{"allocated empty map", map[string]interface{}{}, false},
		{"allocated named map", object{}, false},
		{"fixed empty array", [0]string{}, false},
		{"array containing null", []interface{}{nil}, false},
		{"false", false, false},
		{"zero", zero, false},
		{"empty string", "", false},
		{"nonnil pointer", &zero, false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			for _, attribute := range []AttributeKey{"value", "nested.value"} {
				context := Context{"value": row.value, "nested": map[string]interface{}{"value": row.value}}
				for _, op := range []Operator{OperatorIsNull, OperatorNotNull, OperatorExists, OperatorNotExists} {
					want := (op == OperatorIsNull && row.null) || (op == OperatorNotNull && !row.null) || op == OperatorExists
					if got := AllConditionsAreMatched(PlainCondition{Attribute: attribute, Operator: op}, context); got != want {
						t.Errorf("%s %s got %v want %v", attribute, op, got, want)
					}
				}
			}
			for _, op := range []Operator{OperatorIsNull, OperatorNotNull} {
				for _, attribute := range []AttributeKey{"missing", "nested.missing", "parent.child"} {
					context := Context{"nested": map[string]interface{}{"value": row.value}, "parent": row.value}
					if AllConditionsAreMatched(PlainCondition{Attribute: attribute, Operator: op}, context) {
						t.Errorf("%s matched missing path %s", op, attribute)
					}
				}
			}
			encoded, err := json.Marshal(row.value)
			if err != nil || (string(encoded) == "null") != row.null {
				t.Fatalf("null predicate must match JSON identity: %s %v", encoded, err)
			}
		})
	}
}

func TestTypedNilNullPredicatesThroughGettersChildrenAndRequirements(t *testing.T) {
	for _, value := range []interface{}{[]string(nil), map[string]interface{}(nil), (*string)(nil), []string{}, map[string]interface{}{}} {
		for _, op := range []Operator{OperatorIsNull, OperatorNotNull} {
			t.Run(fmt.Sprintf("%T/%v/%s", value, conditionValueIsNull(value), op), func(t *testing.T) {
				datafile := alignmentDatafile(t, fmt.Sprintf(`{
					"schemaVersion":"2","revision":"typed-null",
					"segments":{"null":{"conditions":{"attribute":"nested.value","operator":%q}}},
					"features":{
						"conditioned":{"bucketBy":"userId","traffic":[{"key":"all","segments":"null","percentage":100000}]},
						"required":{"bucketBy":"userId","conditions":[{"variable":"source","operator":%q}],"traffic":[{"key":"all","segments":"*","percentage":100000}]}
					},
					"variables":{
						"source":{"type":"object","defaultValue":{}},
						"required":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","operator":%q}]}
					}
				}`, op, op, op))
				sticky := StickyVariables{"source": value}
				level := LogLevelFatal
				f := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, StickyVariables: &sticky, LogLevel: &level})
				defer f.Close()
				context := Context{"userId": "member", "nested": map[string]interface{}{"value": value}}
				child := f.Spawn(context, SpawnOptions{StickyVariables: &sticky})
				defer child.Close()
				encoded, _ := json.Marshal(value)
				want := (string(encoded) == "null") == (op == OperatorIsNull)
				for _, key := range []string{"conditioned", "required"} {
					if f.IsEnabled(key, context) != want || child.IsEnabled(key) != want {
						t.Errorf("root and child feature %s expected %v", key, want)
					}
				}
				if f.GetGlobalVariable("required") != want || child.GetGlobalVariable("required") != want {
					t.Errorf("root and child global requirement expected %v", want)
				}
			})
		}
	}
}

func TestTypedNilFeatureVariableNullPrerequisites(t *testing.T) {
	for _, value := range []interface{}{[]string(nil), map[string]interface{}(nil), (*string)(nil), []string{}, map[string]interface{}{}} {
		for _, op := range []Operator{OperatorIsNull, OperatorNotNull} {
			t.Run(fmt.Sprintf("%T/%v/%s", value, conditionValueIsNull(value), op), func(t *testing.T) {
				datafile := alignmentDatafile(t, fmt.Sprintf(`{
					"schemaVersion":"2","revision":"feature-null","segments":{},
					"features":{
						"source":{"bucketBy":"userId","traffic":[{"key":"all","segments":"*","percentage":100000}],"variablesSchema":{"value":{"type":"object","defaultValue":{}}}},
						"required":{"bucketBy":"userId","conditions":[{"feature":"source","variable":"value","operator":%q}],"traffic":[{"key":"all","segments":"*","percentage":100000}]}
					},
					"variables":{"required":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"feature":"source","variable":"value","operator":%q}]}}
				}`, op, op))
				sticky := StickyFeatures{"source": EvaluatedFeature{Enabled: true, Variables: map[VariableKey]VariableValue{"value": value}}}
				level := LogLevelFatal
				f := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, StickyFeatures: &sticky, LogLevel: &level})
				defer f.Close()
				context := Context{"userId": "member"}
				child := f.Spawn(context, SpawnOptions{StickyFeatures: &sticky})
				defer child.Close()
				encoded, _ := json.Marshal(value)
				want := (string(encoded) == "null") == (op == OperatorIsNull)
				if f.IsEnabled("required", context) != want || child.IsEnabled("required") != want {
					t.Errorf("root and child feature requirement expected %v", want)
				}
				if f.GetGlobalVariable("required", context) != want || child.GetGlobalVariable("required") != want {
					t.Errorf("root and child global requirement expected %v", want)
				}
			})
		}
	}
}

func TestTypedNilLiteralNullComparisonsAndArrayElements(t *testing.T) {
	for _, value := range []interface{}{nil, []string(nil), []interface{}(nil), map[string]interface{}(nil), (*string)(nil)} {
		for _, op := range []Operator{OperatorEquals, OperatorNotEquals, OperatorIn, OperatorNotIn, OperatorIncludes, OperatorNotIncludes, OperatorIncludesAny, OperatorNotIncludesAny} {
			t.Run(fmt.Sprintf("%T/%s", value, op), func(t *testing.T) {
				actual, expected := value, interface{}(nil)
				want := op == OperatorEquals || op == OperatorIn || op == OperatorIncludes || op == OperatorIncludesAny
				if op == OperatorIncludes || op == OperatorNotIncludes || op == OperatorIncludesAny || op == OperatorNotIncludesAny {
					actual = []interface{}{value}
				}
				if op == OperatorIn || op == OperatorNotIn || op == OperatorIncludesAny || op == OperatorNotIncludesAny {
					expected = []interface{}{nil}
				}
				condition := PlainCondition{Attribute: "value", Operator: op, Value: conditionValue(expected)}
				if got := AllConditionsAreMatched(condition, Context{"value": actual}); got != want {
					t.Errorf("actual JSON null got %v want %v", got, want)
				}
				actual, expected = interface{}(nil), value
				if op == OperatorIncludes || op == OperatorNotIncludes || op == OperatorIncludesAny || op == OperatorNotIncludesAny {
					actual = []interface{}{nil}
				}
				if op == OperatorIn || op == OperatorNotIn || op == OperatorIncludesAny || op == OperatorNotIncludesAny {
					expected = []interface{}{value}
				}
				condition.Value = conditionValue(expected)
				if got := AllConditionsAreMatched(condition, Context{"value": actual}); got != want {
					t.Errorf("candidate JSON null got %v want %v", got, want)
				}
			})
		}
	}
	for _, value := range []interface{}{[]string{}, map[string]interface{}{}, [0]string{}, false, 0, ""} {
		if strictPrimitiveEqual(value, nil) || strictPrimitiveEqual(nil, value) {
			t.Errorf("nonnil value %T matched null", value)
		}
	}
}
