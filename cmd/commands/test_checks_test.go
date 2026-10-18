package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

func TestPortableCheckDateFractionalPrecision(t *testing.T) {
	for _, digits := range []string{"1239", "123999", "1239999", "123999999"} {
		value, err := parsePortableCheckDate("2024-01-01T00:00:00." + digits + "Z")
		if err != nil || value.Nanosecond() != 123000000 {
			t.Errorf("%s must truncate to .123: %v, %v", digits, value, err)
		}
	}
	if _, err := parsePortableCheckDate("2024-01-01T00:00:00.1234567890Z"); err == nil {
		t.Fatal("ten fractional digits must be rejected")
	}
	value, err := parsePortableCheckDate("1969-12-31T23:59:59.999999999Z")
	if err != nil || value != time.Unix(-1, 999000000).UTC() {
		t.Fatalf("pre-epoch fraction must truncate to the preceding millisecond: %v, %v", value, err)
	}
}

func TestPortableCheckFixtureThroughNativeGetters(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "conformance", "test-checks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version  int                          `json:"version"`
		Datafile featurevisor.DatafileContent `json:"datafile"`
		Cases    []struct {
			Name   string        `json:"name"`
			Owner  string        `json:"owner"`
			Checks []interface{} `json:"checks"`
			Passed bool          `json:"passed"`
			Error  bool          `json:"error"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 3 || len(fixture.Cases) != 255 {
		t.Fatalf("unexpected fixture shape: version=%d cases=%d", fixture.Version, len(fixture.Cases))
	}
	executedAnyCases := 0
	for _, testCase := range fixture.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			level := featurevisor.LogLevelFatal
			diagnostics := &checkDiagnosticCollector{}
			instance := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{Datafile: fixture.Datafile, LogLevel: &level, OnDiagnostic: diagnostics.record})
			defer instance.Close()
			assertion := map[string]interface{}{"checks": testCase.Checks}
			outcome := evaluateAssertionChecksDetailed(assertion, testCase.Owner, nativeCheckResolver{getter: instance, diagnostics: diagnostics}, fixture.Datafile, "")
			if strings.Contains(strings.ToLower(testCase.Name), "includesany") {
				executedAnyCases++
			}
			expectedPassed := testCase.Passed
			expectedError := testCase.Error
			for _, name := range []string{"or skips unreached missing feature", "and skips unreached missing feature", "short circuit skips unresolved ref", "AND short circuit"} {
				if testCase.Name == name {
					expectedPassed = false
					expectedError = true
				}
			}
			if passed := len(outcome.Failures) == 0; passed != expectedPassed || outcome.HadError != expectedError {
				t.Fatalf("expected passed=%v error=%v, got passed=%v error=%v failures=%#v", expectedPassed, expectedError, passed, outcome.HadError, outcome.Failures)
			}
		})
	}
	if executedAnyCases != 36 {
		t.Fatalf("executed %d includesAny check cases, expected 36", executedAnyCases)
	}
}

type checkFixtureEvaluator struct {
	globals    map[string]interface{}
	features   map[string]map[string]interface{}
	enabled    map[string]bool
	variations map[string]*string
	errors     map[string]error
}

type noGetterCheckEvaluator struct{ calls int }

func (e *noGetterCheckEvaluator) ResolveGlobalVariable(string) (featurevisor.VariableValue, error) {
	e.calls++
	return nil, nil
}
func (e *noGetterCheckEvaluator) ResolveFeatureVariable(string, string) (featurevisor.VariableValue, error) {
	e.calls++
	return nil, nil
}
func (e *noGetterCheckEvaluator) ResolveFeatureEnabled(string) (bool, error) {
	e.calls++
	return false, nil
}
func (e *noGetterCheckEvaluator) ResolveFeatureVariation(string) (*string, error) {
	e.calls++
	return nil, nil
}
func (e *noGetterCheckEvaluator) ValidateSegment(string) error { return nil }

type countingSegmentEvaluator struct {
	checkFixtureEvaluator
	available   map[string]bool
	matches     map[string]bool
	validations map[string]int
	resolutions map[string]int
}

func (e *countingSegmentEvaluator) ValidateSegment(key string) error {
	e.validations[key]++
	if !e.available[key] {
		return fmt.Errorf("segment %q definition is missing", key)
	}
	return nil
}

func (e *countingSegmentEvaluator) ResolveSegment(key string) (bool, error) {
	e.resolutions[key]++
	return e.matches[key], nil
}

func (e checkFixtureEvaluator) ResolveGlobalVariable(key string) (featurevisor.VariableValue, error) {
	return e.globals[key], nil
}

func (e checkFixtureEvaluator) ResolveFeatureVariable(feature, variable string) (featurevisor.VariableValue, error) {
	return e.features[feature][variable], nil
}

func (e checkFixtureEvaluator) ResolveFeatureEnabled(feature string) (bool, error) {
	return e.enabled[feature], e.errors[feature+":enabled"]
}

func (e checkFixtureEvaluator) ResolveFeatureVariation(feature string) (*string, error) {
	return e.variations[feature], e.errors[feature+":variation"]
}

func checkFixtureDatafile() featurevisor.DatafileContent {
	return featurevisor.DatafileContent{
		Variables: map[string]featurevisor.GlobalVariable{
			"subject": {}, "other": {}, "nil": {},
		},
		Features: map[string]featurevisor.Feature{
			"checkout": {VariablesSchema: map[string]featurevisor.VariableSchema{"plan": {}}},
		},
	}
}

func TestFeatureChecksValidateEvaluateAndPreserveFatalErrors(t *testing.T) {
	beta := "beta"
	evaluator := checkFixtureEvaluator{
		enabled:    map[string]bool{"checkout": false},
		variations: map[string]*string{"checkout": &beta},
		errors:     map[string]error{},
	}
	datafile := checkFixtureDatafile()
	passing := map[string]interface{}{"checks": []interface{}{
		map[string]interface{}{"feature": "checkout", "variation": "beta"},
		map[string]interface{}{"and": []interface{}{
			map[string]interface{}{"feature": "checkout", "enabled": false},
			map[string]interface{}{"feature": "checkout", "enabled": false, "variation": "beta"},
		}},
	}}
	if failures := evaluateAssertionChecks(passing, "", evaluator, datafile, ""); len(failures) != 0 {
		t.Fatalf("expected variation-only and grouped checks to pass, got %#v", failures)
	}

	failing := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"feature": "checkout", "enabled": true, "variation": "wrong"}}}
	failures := evaluateAssertionChecks(failing, "", evaluator, datafile, "")
	if len(failures) != 1 || !strings.Contains(failures[0], `feature "checkout"`) || !strings.Contains(failures[0], "expected enabled=true, actual enabled=false") || !strings.Contains(failures[0], `expected variation="wrong", actual variation="beta"`) {
		t.Fatalf("expected descriptive enabled failure, got %#v", failures)
	}

	invalid := []map[string]interface{}{
		{"feature": "checkout"},
		{"feature": "checkout", "enabled": true, "operator": "equals", "value": true},
		{"feature": "checkout", "variation": 1},
		{"feature": "", "enabled": false},
	}
	for _, leaf := range invalid {
		assertion := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"or": []interface{}{map[string]interface{}{"feature": "checkout", "enabled": false}, leaf}}}}
		if outcome := evaluateAssertionChecksDetailed(assertion, "", evaluator, datafile, ""); !outcome.HadError {
			t.Fatalf("expected upfront validation error for %#v", leaf)
		}
	}

	missing := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"not": []interface{}{map[string]interface{}{"feature": "missing", "enabled": false}}}}}
	if outcome := evaluateAssertionChecksDetailed(missing, "", evaluator, datafile, ""); !outcome.HadError || !strings.Contains(outcome.Failures[0], "absent from the selected datafile") {
		t.Fatalf("expected missing feature error to escape NOT, got %#v", outcome)
	}

	evaluator.errors["checkout:variation"] = fmt.Errorf("native variation failure")
	fatal := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"or": []interface{}{
		map[string]interface{}{"feature": "checkout", "enabled": true, "variation": "beta"},
		map[string]interface{}{"feature": "checkout", "variation": "beta"},
	}}}}
	if outcome := evaluateAssertionChecksDetailed(fatal, "", evaluator, datafile, ""); !outcome.HadError || !strings.Contains(outcome.Failures[0], "native variation failure") {
		t.Fatalf("expected getter error to escape OR, got %#v", outcome)
	}
}

func TestSegmentChecksValidateNestEvaluateAndReportEvidence(t *testing.T) {
	definitions := map[string]interface{}{
		"all":   map[string]interface{}{"conditions": "*"},
		"dutch": map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"attribute": "country", "operator": "equals", "value": "nl"}}},
		"adult": map[string]interface{}{"conditions": map[string]interface{}{"and": []interface{}{
			map[string]interface{}{"attribute": "country", "operator": "equals", "value": "nl"},
			map[string]interface{}{"attribute": "age", "operator": "greaterThanOrEquals", "value": float64(18)},
		}}},
	}
	content := featurevisor.DatafileContent{SchemaVersion: "4", Revision: "checks", Features: map[string]featurevisor.Feature{}, Segments: map[string]featurevisor.Segment{}}
	instance, diagnostics := buildInstanceForAssertion(content, "error", map[string]interface{}{})
	defer instance.Close()
	keys := segmentGateKeys(content, definitions)
	installSegmentGates(instance, content, definitions, keys)
	resolver := nativeCheckResolver{getter: instance, diagnostics: diagnostics, datafile: content, segmentGates: keys, segmentDefinitions: definitions, context: featurevisor.Context{"country": "nl", "age": float64(21)}}
	assertion := map[string]interface{}{"checks": []interface{}{
		map[string]interface{}{"segment": "all", "matches": true},
		map[string]interface{}{"segment": "dutch", "matches": true},
		map[string]interface{}{"and": []interface{}{
			map[string]interface{}{"segment": "adult", "matches": true},
			map[string]interface{}{"not": []interface{}{map[string]interface{}{"segment": "dutch", "matches": false}}},
		}},
		map[string]interface{}{"or": []interface{}{
			map[string]interface{}{"segment": "dutch", "matches": false},
			map[string]interface{}{"segment": "adult", "matches": true},
		}},
	}}
	if outcome := evaluateAssertionChecksDetailed(assertion, "", resolver, featurevisor.DatafileContent{}, ""); len(outcome.Failures) != 0 {
		t.Fatalf("expected nested segment checks to pass: %#v", outcome)
	}

	failing := map[string]interface{}{"checks": []interface{}{map[string]interface{}{
		"key": "audience", "description": "Audience is excluded", "segment": "dutch", "matches": false,
	}}}
	outcome := evaluateAssertionChecksDetailed(failing, "", resolver, featurevisor.DatafileContent{}, "")
	if len(outcome.Failures) != 1 || !strings.Contains(outcome.Failures[0], `segment "dutch"`) {
		t.Fatalf("expected segment-specific failure, got %#v", outcome.Failures)
	}
	evaluated, ok := outcome.ReportErrors[0].Details["evaluated"].([]map[string]interface{})
	if !ok || len(evaluated) != 1 || !valuesEqual(evaluated[0]["reference"], map[string]interface{}{"segment": "dutch"}) || evaluated[0]["property"] != "matches" || evaluated[0]["operator"] != "equals" || evaluated[0]["expected"] != false || evaluated[0]["actual"] != true {
		t.Fatalf("unexpected segment evidence: %#v", outcome.ReportErrors)
	}
	if segment := segmentKeyFromCheckDetails(outcome.ReportErrors[0].Details); segment != "dutch" {
		t.Fatalf("expected human report segment label, got %q", segment)
	}

	missing := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"or": []interface{}{
		map[string]interface{}{"segment": "dutch", "matches": true},
		map[string]interface{}{"segment": "missing", "matches": false},
	}}}}
	if missingOutcome := evaluateAssertionChecksDetailed(missing, "", resolver, featurevisor.DatafileContent{}, ""); !missingOutcome.HadError || !strings.Contains(missingOutcome.Failures[0], `segment "missing"`) {
		t.Fatalf("expected missing skipped segment definition to fail: %#v", missingOutcome)
	}

	invalid := []map[string]interface{}{
		{"segment": "", "matches": true},
		{"segment": "dutch"},
		{"segment": "dutch", "matches": "yes"},
		{"matches": false},
		{"segment": "dutch", "matches": true, "operator": "equals", "value": true},
		{"segment": "dutch", "matches": true, "feature": "checkout"},
		{"segment": "dutch", "matches": true, "target": "other"},
	}
	for _, leaf := range invalid {
		invalidOutcome := evaluateAssertionChecksDetailed(map[string]interface{}{"checks": []interface{}{leaf}}, "", resolver, featurevisor.DatafileContent{}, "")
		if !invalidOutcome.HadError {
			t.Fatalf("expected invalid segment leaf to fail: %#v", leaf)
		}
	}
}

func TestSegmentValidationDoesNotEvaluateSkippedBranchesOrDuplicateReachedLeaves(t *testing.T) {
	evaluator := &countingSegmentEvaluator{
		available:   map[string]bool{"first": true, "skipped": true},
		matches:     map[string]bool{"first": true, "skipped": true},
		validations: map[string]int{},
		resolutions: map[string]int{},
	}
	assertion := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"or": []interface{}{
		map[string]interface{}{"segment": "first", "matches": true},
		map[string]interface{}{"segment": "skipped", "matches": true},
	}}}}
	outcome := evaluateAssertionChecksDetailed(assertion, "", evaluator, featurevisor.DatafileContent{}, "")
	if len(outcome.Failures) != 0 {
		t.Fatalf("expected check to pass: %#v", outcome)
	}
	if evaluator.validations["first"] != 1 || evaluator.validations["skipped"] != 1 {
		t.Fatalf("expected presence validation for both branches, got %#v", evaluator.validations)
	}
	if evaluator.resolutions["first"] != 1 || evaluator.resolutions["skipped"] != 0 {
		t.Fatalf("expected one reached evaluation and no skipped evaluation, got %#v", evaluator.resolutions)
	}
}

func TestAssertionTreePreflightRejectsAllInvalidReferencesWithoutGetters(t *testing.T) {
	datafile := checkFixtureDatafile()
	cases := map[string]map[string]interface{}{
		"valid root invalid child": {
			"expectedValue": "unused",
			"checks":        []interface{}{map[string]interface{}{"operator": "exists"}},
			"children":      []interface{}{map[string]interface{}{"checks": []interface{}{map[string]interface{}{"feature": "missing", "enabled": false}}}},
		},
		"target omission": {
			"checks": []interface{}{map[string]interface{}{"feature": "omitted", "enabled": false}},
		},
		"compareTo": {
			"checks": []interface{}{map[string]interface{}{"operator": "equals", "compareTo": map[string]interface{}{"variable": "missing"}}},
		},
		"skipped logical reference": {
			"checks": []interface{}{map[string]interface{}{"or": []interface{}{
				map[string]interface{}{"operator": "exists"},
				map[string]interface{}{"not": []interface{}{map[string]interface{}{"feature": "missing", "enabled": false}}},
			}}},
		},
		"malformed child": {
			"checks":   []interface{}{map[string]interface{}{"operator": "exists"}},
			"children": []interface{}{"invalid"},
		},
	}
	for name, assertion := range cases {
		t.Run(name, func(t *testing.T) {
			spy := &noGetterCheckEvaluator{}
			result := preflightAssertionTree(assertion, "subject", spy, datafile, false)
			if !result.HasError {
				t.Fatalf("expected preflight failure")
			}
			if spy.calls != 0 {
				t.Fatalf("expected no getters, got %d", spy.calls)
			}
		})
	}
}

func TestSegmentChecksUseRootAndMergedChildContextOutsideTargetDatafile(t *testing.T) {
	var datafile featurevisor.DatafileContent
	if err := json.Unmarshal([]byte(`{"schemaVersion":"2","revision":"segment-checks","segments":{},"features":{},"variables":{"subject":{"type":"string","defaultValue":"ready"}}}`), &datafile); err != nil {
		t.Fatal(err)
	}
	definitions := map[string]interface{}{
		"dutch": map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"attribute": "country", "operator": "equals", "value": "nl"}}},
		"dutch-pro": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"attribute": "country", "operator": "equals", "value": "nl"},
			map[string]interface{}{"attribute": "plan", "operator": "equals", "value": "pro"},
		}},
	}
	level := featurevisor.LogLevelError
	diagnostics := &checkDiagnosticCollector{}
	instance := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{Datafile: datafile, LogLevel: &level, OnDiagnostic: diagnostics.record})
	defer instance.Close()
	scenario := map[string]interface{}{
		"context": map[string]interface{}{"country": "nl"},
		"target":  "thin",
		"at":      float64(0.1),
		"checks":  []interface{}{map[string]interface{}{"segment": "dutch", "matches": true}},
		"children": []interface{}{map[string]interface{}{
			"context":         map[string]interface{}{"plan": "pro"},
			"stickyVariables": map[string]interface{}{"subject": "ignored"},
			"checks":          []interface{}{map[string]interface{}{"segment": "dutch-pro", "matches": true}},
		}},
	}
	if result := RunTestScenario(scenario, instance, diagnostics, datafile, definitions, "error"); result.HasError {
		t.Fatalf("expected root and merged child segment contexts to pass: %s", result.Errors)
	}

	variable := map[string]interface{}{
		"context":       map[string]interface{}{"country": "nl"},
		"expectedValue": "ready",
		"checks":        []interface{}{map[string]interface{}{"segment": "dutch", "matches": true}},
	}
	if result := RunTestVariable(variable, "subject", instance, diagnostics, datafile, definitions, "error"); result.HasError {
		t.Fatalf("expected global variable segment check to pass: %s", result.Errors)
	}
}

func TestNativeFeatureGetterDiagnosticCannotSatisfyFalseExpectation(t *testing.T) {
	var datafile featurevisor.DatafileContent
	if err := json.Unmarshal([]byte(`{"schemaVersion":"2","revision":"bad-feature","segments":{},"features":{"broken":{"bucketBy":"userId","conditions":[{"variable":"source","operator":"semverGreaterThan","value":"1.0.0"}],"traffic":[{"key":"all","segments":"*","percentage":100000,"enabled":true}]}},"variables":{"source":{"type":"string","defaultValue":"invalid"}}}`), &datafile); err != nil {
		t.Fatal(err)
	}
	level := featurevisor.LogLevelError
	diagnostics := &checkDiagnosticCollector{}
	instance := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{Datafile: datafile, LogLevel: &level, OnDiagnostic: diagnostics.record})
	defer instance.Close()
	assertion := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"not": []interface{}{map[string]interface{}{"feature": "broken", "enabled": false}}}}}
	outcome := evaluateAssertionChecksDetailed(assertion, "", nativeCheckResolver{getter: instance, diagnostics: diagnostics}, datafile, "")
	if !outcome.HadError || len(outcome.Failures) != 1 || !strings.Contains(outcome.Failures[0], "evaluation_error") {
		t.Fatalf("expected feature getter diagnostic to escape NOT, got %#v", outcome)
	}
}

func TestVariableChecksCoverReferencesPathsGroupsAndErrors(t *testing.T) {
	evaluator := checkFixtureEvaluator{
		globals: map[string]interface{}{
			"subject": map[string]interface{}{"items": []interface{}{map[string]interface{}{"id": "free"}, map[string]interface{}{"id": "pro"}}, "literal.key": nil},
			"other":   "pro",
			"nil":     nil,
		},
		features: map[string]map[string]interface{}{"checkout": {"plan": "pro"}},
	}
	assertion := map[string]interface{}{"checks": []interface{}{
		map[string]interface{}{"path": "items.1.id", "operator": "equals", "compareTo": map[string]interface{}{"variable": "other"}},
		map[string]interface{}{"path": []interface{}{"literal.key"}, "operator": "isNull"},
		map[string]interface{}{"path": "missing", "operator": "notExists"},
		map[string]interface{}{"or": []interface{}{
			map[string]interface{}{"variable": "other", "operator": "equals", "value": "wrong"},
			map[string]interface{}{"feature": "checkout", "variable": "plan", "operator": "equals", "value": "pro"},
		}},
		map[string]interface{}{"not": []interface{}{map[string]interface{}{"variable": "other", "operator": "equals", "value": "wrong"}}},
	}}
	if errors := evaluateAssertionChecks(assertion, "subject", evaluator, checkFixtureDatafile(), ""); len(errors) != 0 {
		t.Fatalf("expected checks to pass, got %#v", errors)
	}

	fatal := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"not": []interface{}{
		map[string]interface{}{"operator": "greaterThan", "value": "not a number"},
	}}}}
	errors := evaluateAssertionChecks(fatal, "subject", evaluator, checkFixtureDatafile(), "")
	if len(errors) != 1 || !strings.Contains(errors[0], "numeric comparison") {
		t.Fatalf("expected typed failure to escape NOT, got %#v", errors)
	}
}

func TestScenarioChecksRequireExplicitVariables(t *testing.T) {
	evaluator := checkFixtureEvaluator{globals: map[string]interface{}{"subject": "ready"}}
	assertion := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"operator": "equals", "value": "ready"}}}
	errors := evaluateAssertionChecks(assertion, "", evaluator, checkFixtureDatafile(), "")
	if len(errors) != 1 || !strings.Contains(errors[0], "variable is required") {
		t.Fatalf("expected scenario reference error, got %#v", errors)
	}
}

func TestCheckMetadataAndSiblingKeys(t *testing.T) {
	checks := []interface{}{
		map[string]interface{}{"key": "primary", "description": "Primary expectation", "operator": "equals", "value": "ready"},
		map[string]interface{}{"and": []interface{}{
			map[string]interface{}{"key": "nested", "operator": "equals", "value": "ready"},
		}, "description": "Nested group"},
	}
	if err := validateCheckList(checks, "subject", "checks"); err != nil {
		t.Fatalf("expected metadata to be valid: %v", err)
	}
	duplicate := []interface{}{
		map[string]interface{}{"key": "same", "operator": "equals", "value": "ready"},
		map[string]interface{}{"key": "same", "operator": "equals", "value": "ready"},
	}
	if err := validateCheckList(duplicate, "subject", "checks"); err == nil || !strings.Contains(err.Error(), "duplicate sibling key") {
		t.Fatalf("expected duplicate key failure, got %v", err)
	}
	invalid := []interface{}{map[string]interface{}{"key": "", "operator": "equals", "value": "ready"}}
	if err := validateCheckList(invalid, "subject", "checks"); err == nil || !strings.Contains(err.Error(), "nonempty string") {
		t.Fatalf("expected invalid metadata failure, got %v", err)
	}
}

func TestNamedCheckFailureIncludesMetadataAndEvidence(t *testing.T) {
	var datafile featurevisor.DatafileContent
	if err := json.Unmarshal([]byte(`{"schemaVersion":"2","revision":"named","segments":{},"features":{},"variables":{"subject":{"type":"string","defaultValue":"ready"}}}`), &datafile); err != nil {
		t.Fatal(err)
	}
	level := featurevisor.LogLevelError
	diagnostics := &checkDiagnosticCollector{}
	instance := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{Datafile: datafile, LogLevel: &level, OnDiagnostic: diagnostics.record})
	defer instance.Close()
	assertion := map[string]interface{}{"checks": []interface{}{map[string]interface{}{
		"key": "status", "description": "Status is complete", "operator": "equals", "value": "wrong",
	}}}
	outcome := evaluateAssertionChecksDetailed(assertion, "subject", nativeCheckResolver{getter: instance, diagnostics: diagnostics}, datafile, "")
	if len(outcome.Failures) != 1 || !strings.Contains(outcome.Failures[0], `key="status"`) || !strings.Contains(outcome.Failures[0], `left="ready", right="wrong"`) {
		t.Fatalf("expected named evidence, got %#v", outcome.Failures)
	}
	if len(outcome.ReportErrors) != 1 || !valuesEqual(*outcome.ReportErrors[0].Expected, true) || !valuesEqual(*outcome.ReportErrors[0].Actual, false) {
		t.Fatalf("expected typed check wrapper, got %#v", outcome.ReportErrors)
	}
	evaluated, ok := outcome.ReportErrors[0].Details["evaluated"].([]map[string]interface{})
	if !ok || len(evaluated) != 1 || !valuesEqual(evaluated[0]["actual"], "ready") || !valuesEqual(evaluated[0]["expected"], "wrong") {
		t.Fatalf("expected typed operand evidence, got %#v", outcome.ReportErrors[0].Details)
	}
	absent := map[string]interface{}{"checks": []interface{}{map[string]interface{}{
		"key": "missing-value", "path": "absent", "operator": "equals", "value": "<missing>",
	}}}
	absentOutcome := evaluateAssertionChecksDetailed(absent, "subject", nativeCheckResolver{getter: instance, diagnostics: diagnostics}, datafile, "")
	absentTraces, ok := absentOutcome.ReportErrors[0].Details["evaluated"].([]map[string]interface{})
	if !ok || len(absentTraces) != 1 || absentTraces[0]["actualMissing"] != true || absentTraces[0]["expected"] != "<missing>" {
		t.Fatalf("expected explicit missing presence evidence, got %#v", absentOutcome.ReportErrors)
	}
	if marker, ok := absentTraces[0]["actual"].(map[string]interface{}); !ok || marker["$type"] != "undefined" {
		t.Fatalf("expected undefined marker, got %#v", absentTraces[0]["actual"])
	}
}

func TestScenarioSetOptionIsParsed(t *testing.T) {
	if set := ParseCLIOptions([]string{"--set=staging"}).Set; set != "staging" {
		t.Fatalf("expected staging set, got %q", set)
	}
}

func TestMalformedJSONGetterDiagnosticFailsNullCheckAtQuietLevel(t *testing.T) {
	var datafile featurevisor.DatafileContent
	if err := json.Unmarshal([]byte(`{"schemaVersion":"2","revision":"malformed","segments":{},"features":{},"variables":{"source":{"type":"json","defaultValue":"{"}}}`), &datafile); err != nil {
		t.Fatal(err)
	}
	level := featurevisor.LogLevelError
	diagnostics := &checkDiagnosticCollector{}
	instance := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{Datafile: datafile, LogLevel: &level, OnDiagnostic: diagnostics.record})
	defer instance.Close()
	assertError := func(name string, operator string, resolver checkVariableEvaluator) {
		t.Helper()
		assertion := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"operator": operator}}}
		outcome := evaluateAssertionChecksDetailed(assertion, "source", resolver, datafile, "")
		if !outcome.HadError || len(outcome.Failures) != 1 || !strings.Contains(outcome.Failures[0], "evaluation_error") {
			t.Fatalf("%s %s: expected getter diagnostic to fail check, got %#v", name, operator, outcome)
		}
	}
	child := instance.Spawn()
	defer child.Close()
	for _, operator := range []string{"isNull", "empty", "notEmpty"} {
		assertError("parent", operator, nativeCheckResolver{getter: instance, diagnostics: diagnostics})
		assertError("child", operator, nativeCheckResolver{getter: child, diagnostics: diagnostics})
	}
}

func TestNativeRunnerDispatchesScenariosAndReturnsFailureExitCode(t *testing.T) {
	temp := t.TempDir()
	npx := filepath.Join(temp, "npx")
	logPath := filepath.Join(temp, "calls.log")
	script := `#!/bin/sh
	printf '%s\n' "$*" >> "$RUNNER_LOG"
	case "$*" in
  *"config --json"*) printf '%s\n' '{"environments":["production"]}' ;;
  *"list --segments --json"*) printf '%s\n' '[]' ;;
  *"list --targets --json"*) printf '%s\n' '[]' ;;
  *"list --tests"*)
    case "$*" in *"--entityType=scenario"*) ;; *) exit 13 ;; esac
	case "$*" in *"--execution-protocol"*"--apply-matrix"*"--execution-metadata"*) ;; *) exit 15 ;; esac
	if [ "$RUNNER_CASE" = protocol ]; then printf '%s\n' '[]'; exit 0; fi
    if [ "$RUNNER_CASE" = malformed ]; then printf '%s\n' '{"kind":"featurevisor.test-execution","schemaVersion":1,"tests":[{"key":"scenario-native","scenario":"native","assertions":[{"environment":"production"}]}]}'; exit 0; fi
    if [ "$RUNNER_CASE" = empty ]; then printf '%s\n' '{"kind":"featurevisor.test-execution","schemaVersion":1,"tests":[{"key":"scenario-native","scenario":"native","assertions":[]}]}'; exit 0; fi
    if [ "$RUNNER_CASE" = mixed ]; then printf '%s\n' '{"kind":"featurevisor.test-execution","schemaVersion":1,"tests":[{"key":"scenario-native","scenario":"native","variable":"subject","assertions":[{"environment":"production","checks":[{"variable":"subject","operator":"equals","value":"ready"}]}]}]}'; exit 0; fi
    if [ "$RUNNER_CASE" = failure ]; then expected=wrong; else expected=ready; fi
    printf '{"kind":"featurevisor.test-execution","schemaVersion":1,"tests":[{"key":"scenario-native","scenario":"native","_test":{"source":"tests/scenarios/native.spec.yml","set":null,"entityType":"scenario"},"assertions":[{"environment":"production","description":"native scenario","context":{"market":"nl"},"_case":{"id":"id-1","assertionIndex":0,"matrixIndex":null,"matrixValues":{}},"checks":[{"key":"status","description":"Status is ready","variable":"subject","operator":"equals","value":"%s"}],"children":[{"stickyVariables":{"subject":"child"},"checks":[{"variable":"subject","operator":"equals","value":"child"}]}]}]}]}\n' "$expected" ;;
  *"build --environment=production --json"*) printf '%s\n' '{"schemaVersion":"2","revision":"1","segments":{},"features":{},"variables":{"subject":{"type":"string","defaultValue":"ready"}}}' ;;
  *) exit 14 ;;
esac
`
	if err := os.WriteFile(npx, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(temp, "featurevisor-go")
	build := exec.Command("go", "build", "-o", binary, "../main.go")
	build.Env = append(os.Environ(), "GOCACHE="+filepath.Join(temp, "go-cache"))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build runner: %v\n%s", err, output)
	}
	run := func(testCase string) ([]byte, error) {
		command := exec.Command(binary, "test", "--projectDirectoryPath="+temp, "--entityType=scenario", "--onlyFailures")
		command.Env = append(os.Environ(), "PATH="+temp+":"+os.Getenv("PATH"), "RUNNER_LOG="+logPath)
		if testCase != "" {
			command.Env = append(command.Env, "RUNNER_CASE="+testCase)
		}
		return command.CombinedOutput()
	}
	if output, err := run(""); err != nil || !strings.Contains(string(output), "Total cases: 1 passed, 0 failed") {
		t.Fatalf("passing native runner: %v\n%s", err, output)
	}
	if output, err := run("failure"); err == nil || !strings.Contains(string(output), "checks.1") || !strings.Contains(string(output), "Rerun (POSIX shell):") {
		t.Fatalf("failing native runner: %v\n%s", err, output)
	}
	jsonCommand := exec.Command(binary, "test", "--projectDirectoryPath="+temp, "--entityType=scenario", "--case=id-1", "--json", "--pretty")
	jsonCommand.Env = append(os.Environ(), "PATH="+temp+":"+os.Getenv("PATH"), "RUNNER_CASE=failure")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	jsonCommand.Stdout = &stdout
	jsonCommand.Stderr = &stderr
	if err := jsonCommand.Run(); err == nil {
		t.Fatalf("expected failing JSON run")
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected clean JSON stderr, got %q", stderr.String())
	}
	var report testReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("parse JSON report: %v\n%s", err, stdout.String())
	}
	if report.SchemaVersion != 1 || report.Runner != "go" || report.Passed || len(report.Results) != 1 || report.Results[0].Source != "tests/scenarios/native.spec.yml" {
		t.Fatalf("unexpected JSON report: %#v", report)
	}
	caseResult := report.Results[0].Cases[0]
	if caseResult.ID != "id-1" || caseResult.Rerun.Args[1] != "--case=id-1" || caseResult.Errors[0].Actual == nil || caseResult.Errors[0].Expected == nil {
		t.Fatalf("missing case identity or operand evidence: %#v", caseResult)
	}
	if got := formatReportPath(caseResult.Errors[0].Path); got != "checks.1" || caseResult.Errors[0].Message != "Check did not match" {
		t.Fatalf("unexpected scenario check naming: %#v", caseResult.Errors[0])
	}
	for _, malformed := range []string{"malformed", "empty", "mixed"} {
		if output, err := run(malformed); err == nil || !strings.Contains(string(output), "1 failed") {
			t.Fatalf("%s scenario should fail: %v\n%s", malformed, err, output)
		}
	}
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := run("protocol"); err == nil || !strings.Contains(string(output), "test execution protocol") {
		t.Fatalf("invalid protocol should fail: %v\n%s", err, output)
	}
	logOutput, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logOutput), "build ") {
		t.Fatalf("invalid protocol must fail before builds:\n%s", logOutput)
	}
}
