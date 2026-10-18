package commands

import (
	"strings"
	"testing"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

func TestEmptyCheckOperatorsValidateCompareAndShortCircuit(t *testing.T) {
	datafile := featurevisor.DatafileContent{
		Variables: map[string]featurevisor.GlobalVariable{"subject": {}},
		Features:  map[string]featurevisor.Feature{},
	}
	evaluator := checkFixtureEvaluator{
		globals: map[string]interface{}{
			"subject": map[string]interface{}{
				"emptyString":    "",
				"whitespace":     " ",
				"emptyArray":     []interface{}{},
				"arrayWithNull":  []interface{}{nil},
				"emptyObject":    map[string]interface{}{},
				"objectWithNull": map[string]interface{}{"x": nil},
				"null":           nil,
				"number":         0,
				"boolean":        false,
			},
		},
	}

	passing := map[string]interface{}{"checks": []interface{}{
		map[string]interface{}{"path": "emptyString", "operator": "empty"},
		map[string]interface{}{"path": "whitespace", "operator": "notEmpty"},
		map[string]interface{}{"path": "emptyArray", "operator": "empty"},
		map[string]interface{}{"path": "arrayWithNull", "operator": "notEmpty"},
		map[string]interface{}{"path": "emptyObject", "operator": "empty"},
		map[string]interface{}{"path": "objectWithNull", "operator": "notEmpty"},
	}}
	if outcome := evaluateAssertionChecksDetailed(passing, "subject", evaluator, datafile, ""); len(outcome.Failures) != 0 || outcome.HadError {
		t.Fatalf("valid empty checks failed: %#v", outcome)
	}

	for _, check := range []map[string]interface{}{
		{"path": "null", "operator": "empty"},
		{"path": "null", "operator": "notEmpty"},
		{"path": "missing", "operator": "empty"},
		{"path": "missing", "operator": "notEmpty"},
	} {
		outcome := evaluateAssertionChecksDetailed(map[string]interface{}{"checks": []interface{}{check}}, "subject", evaluator, datafile, "")
		if outcome.HadError || len(outcome.Failures) != 1 {
			t.Fatalf("null or missing check should be a non-error false result for %#v: %#v", check, outcome)
		}
	}

	skippedOr := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"or": []interface{}{
		map[string]interface{}{"path": "emptyString", "operator": "empty"},
		map[string]interface{}{"path": "number", "operator": "empty"},
	}}}}
	if outcome := evaluateAssertionChecksDetailed(skippedOr, "subject", evaluator, datafile, ""); len(outcome.Failures) != 0 || outcome.HadError {
		t.Fatalf("OR evaluated an unreachable invalid operand: %#v", outcome)
	}

	skippedAnd := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"and": []interface{}{
		map[string]interface{}{"path": "emptyString", "operator": "notEmpty"},
		map[string]interface{}{"path": "boolean", "operator": "empty"},
	}}}}
	if outcome := evaluateAssertionChecksDetailed(skippedAnd, "subject", evaluator, datafile, ""); outcome.HadError || len(outcome.Failures) != 1 {
		t.Fatalf("AND evaluated an unreachable invalid operand: %#v", outcome)
	}

	for name, assertion := range map[string]map[string]interface{}{
		"or": {"checks": []interface{}{map[string]interface{}{"or": []interface{}{
			map[string]interface{}{"path": "emptyString", "operator": "notEmpty"},
			map[string]interface{}{"path": "number", "operator": "empty"},
			map[string]interface{}{"path": "emptyString", "operator": "empty"},
		}}}},
		"not": {"checks": []interface{}{map[string]interface{}{"not": []interface{}{
			map[string]interface{}{"path": "boolean", "operator": "notEmpty"},
		}}}},
	} {
		outcome := evaluateAssertionChecksDetailed(assertion, "subject", evaluator, datafile, "")
		if !outcome.HadError || len(outcome.Failures) != 1 || !strings.Contains(outcome.Failures[0], "requires a string, array, or object operand") {
			t.Fatalf("%s swallowed an incompatible scalar error: %#v", name, outcome)
		}
	}

	for name, leaf := range map[string]map[string]interface{}{
		"value":      {"operator": "empty", "value": ""},
		"compareTo":  {"operator": "notEmpty", "compareTo": map[string]interface{}{"variable": "subject"}},
		"regexFlags": {"operator": "empty", "regexFlags": "i"},
		"wrong name": {"operator": "isEmpty"},
	} {
		outcome := evaluateAssertionChecksDetailed(map[string]interface{}{"checks": []interface{}{leaf}}, "subject", evaluator, datafile, "")
		if !outcome.HadError {
			t.Fatalf("%s should fail unary operator validation: %#v", name, outcome)
		}
	}
}
