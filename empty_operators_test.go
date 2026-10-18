package featurevisor

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEmptyOperatorsMatchOnlySupportedShallowValues(t *testing.T) {
	type unsupported struct{}
	var nilSlice []string
	var nilMap map[string]interface{}
	var nilPointer *string
	context := Context{
		"emptyString":     "",
		"whitespace":      " ",
		"emptyArray":      []interface{}{},
		"nonEmptyArray":   []interface{}{nil},
		"emptyObject":     map[string]interface{}{},
		"nonEmptyObject":  map[string]interface{}{"x": nil},
		"null":            nil,
		"number":          0,
		"boolean":         false,
		"unsupported":     unsupported{},
		"time":            time.Time{},
		"nilSlice":        nilSlice,
		"nilMap":          nilMap,
		"nilPointer":      nilPointer,
		"typedEmptySlice": []string{},
		"typedEmptyMap":   map[string]string{},
		"nested": map[string]interface{}{
			"emptyArray":  []interface{}{},
			"emptyObject": map[string]interface{}{},
		},
	}

	tests := []struct {
		name      string
		attribute AttributeKey
		operator  Operator
		expected  bool
	}{
		{name: "empty string", attribute: "emptyString", operator: OperatorEmpty, expected: true},
		{name: "empty string is not nonempty", attribute: "emptyString", operator: OperatorNotEmpty, expected: false},
		{name: "whitespace is nonempty", attribute: "whitespace", operator: OperatorNotEmpty, expected: true},
		{name: "empty array", attribute: "emptyArray", operator: OperatorEmpty, expected: true},
		{name: "array containing null is nonempty", attribute: "nonEmptyArray", operator: OperatorNotEmpty, expected: true},
		{name: "empty object", attribute: "emptyObject", operator: OperatorEmpty, expected: true},
		{name: "object containing null is nonempty", attribute: "nonEmptyObject", operator: OperatorNotEmpty, expected: true},
		{name: "nested empty array", attribute: "nested.emptyArray", operator: OperatorEmpty, expected: true},
		{name: "nested empty object", attribute: "nested.emptyObject", operator: OperatorEmpty, expected: true},
		{name: "null is not empty", attribute: "null", operator: OperatorEmpty, expected: false},
		{name: "null is not nonempty", attribute: "null", operator: OperatorNotEmpty, expected: false},
		{name: "missing is not empty", attribute: "missing", operator: OperatorEmpty, expected: false},
		{name: "missing is not nonempty", attribute: "missing", operator: OperatorNotEmpty, expected: false},
		{name: "number is unsupported", attribute: "number", operator: OperatorEmpty, expected: false},
		{name: "boolean is unsupported", attribute: "boolean", operator: OperatorNotEmpty, expected: false},
		{name: "host struct is unsupported", attribute: "unsupported", operator: OperatorNotEmpty, expected: false},
		{name: "time is not empty", attribute: "time", operator: OperatorEmpty, expected: false},
		{name: "time is not nonempty", attribute: "time", operator: OperatorNotEmpty, expected: false},
		{name: "typed nil slice is null", attribute: "nilSlice", operator: OperatorEmpty, expected: false},
		{name: "typed nil slice is not nonempty", attribute: "nilSlice", operator: OperatorNotEmpty, expected: false},
		{name: "typed nil map is not empty", attribute: "nilMap", operator: OperatorEmpty, expected: false},
		{name: "typed nil map is null", attribute: "nilMap", operator: OperatorNotEmpty, expected: false},
		{name: "typed nil pointer is null", attribute: "nilPointer", operator: OperatorEmpty, expected: false},
		{name: "typed nil pointer is not nonempty", attribute: "nilPointer", operator: OperatorNotEmpty, expected: false},
		{name: "allocated typed empty slice", attribute: "typedEmptySlice", operator: OperatorEmpty, expected: true},
		{name: "allocated typed empty map", attribute: "typedEmptyMap", operator: OperatorEmpty, expected: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			condition := PlainCondition{Attribute: test.attribute, Operator: test.operator}
			if actual := AllConditionsAreMatched(condition, context); actual != test.expected {
				t.Fatalf("expected %v, got %v", test.expected, actual)
			}
		})
	}

	for _, operator := range []string{"empty", "notEmpty"} {
		condition := map[string]interface{}{
			"attribute": "nested.emptyArray",
			"operator":  operator,
		}
		expected := operator == "empty"
		if actual := AllConditionsAreMatched(condition, context); actual != expected {
			t.Fatalf("decoded %s condition expected %v, got %v", operator, expected, actual)
		}
	}
}

func TestEmptyOperatorsWorkInConditionsAndRequiredVariables(t *testing.T) {
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"empty-operators",
		"segments":{
			"empty":{"conditions":{"attribute":"profile.emptyObject","operator":"empty"}},
			"nonempty":{"conditions":{"attribute":"profile.arrayWithNull","operator":"notEmpty"}}
		},
		"features":{
			"conditioned":{"bucketBy":"userId","traffic":[{"key":"all","segments":{"and":["empty","nonempty"]},"percentage":100000,"enabled":true}]}
		},
		"variables":{
			"source":{"type":"json","defaultValue":"{\"emptyString\":\"\",\"whitespace\":\" \",\"emptyArray\":[],\"arrayWithNull\":[null],\"emptyObject\":{},\"objectWithNull\":{\"x\":null},\"null\":null,\"number\":0}"},
			"emptyArray":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","path":["emptyArray"],"operator":"empty"}]},
			"arrayWithNull":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","path":["arrayWithNull"],"operator":"notEmpty"}]},
			"emptyObject":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","path":["emptyObject"],"operator":"empty"}]},
			"objectWithNull":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","path":["objectWithNull"],"operator":"notEmpty"}]},
			"whitespace":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","path":["whitespace"],"operator":"notEmpty"}]},
			"nullEmpty":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","path":["null"],"operator":"empty"}]},
			"nullNotEmpty":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","path":["null"],"operator":"notEmpty"}]},
			"missingEmpty":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","path":["missing"],"operator":"empty"}]},
			"numberNotEmpty":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","path":["number"],"operator":"notEmpty"}]}
		}
	}`)

	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile})
	defer instance.Close()
	context := Context{
		"userId": "user",
		"profile": map[string]interface{}{
			"emptyObject":   map[string]interface{}{},
			"arrayWithNull": []interface{}{nil},
		},
	}
	if !instance.IsEnabled("conditioned", context) {
		t.Fatal("condition operators did not match decoded empty object and nonempty array")
	}

	for _, key := range []string{"emptyArray", "arrayWithNull", "emptyObject", "objectWithNull", "whitespace"} {
		if value := instance.GetGlobalVariable(key); value != true {
			t.Fatalf("%s requirement did not match: %#v", key, value)
		}
	}
	for _, key := range []string{"nullEmpty", "nullNotEmpty", "missingEmpty", "numberNotEmpty"} {
		if value := instance.GetGlobalVariable(key); value != false {
			t.Fatalf("%s unsupported or absent requirement matched: %#v", key, value)
		}
	}

	for _, operator := range []Operator{OperatorEmpty, OperatorNotEmpty} {
		encoded, err := json.Marshal(PlainCondition{Attribute: "value", Operator: operator})
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded["operator"] != string(operator) {
			t.Fatalf("operator %s did not round trip: %s", operator, encoded)
		}
	}
}
