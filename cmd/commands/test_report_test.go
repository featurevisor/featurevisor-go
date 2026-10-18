package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

func TestHumaniseChildMessageUsesOneBasedLabel(t *testing.T) {
	if got := humaniseChildMessage("children[0].expectedValue: expected 1 but received 2"); got != "Child 1, expectedValue: expected 1 but received 2" {
		t.Fatalf("human child label = %q", got)
	}
}

func TestParseTestExecutionProtocolRejectsMalformedAndFutureEnvelopes(t *testing.T) {
	valid := `{"kind":"featurevisor.test-execution","schemaVersion":1,"tests":[{}]}`
	tests, err := parseTestExecutionProtocol(valid)
	if err != nil || len(tests) != 1 {
		t.Fatalf("expected valid protocol, tests=%#v err=%v", tests, err)
	}
	invalid := []string{
		`[]`,
		`{}`,
		`{"kind":null,"schemaVersion":1,"tests":[]}`,
		`{"kind":"wrong","schemaVersion":1,"tests":[]}`,
		`{"kind":"featurevisor.test-execution","tests":[]}`,
		`{"kind":"featurevisor.test-execution","schemaVersion":null,"tests":[]}`,
		`{"kind":"featurevisor.test-execution","schemaVersion":"1","tests":[]}`,
		`{"kind":"featurevisor.test-execution","schemaVersion":2,"tests":[]}`,
		`{"kind":"featurevisor.test-execution","schemaVersion":1,"tests":null}`,
		`{"kind":"featurevisor.test-execution","schemaVersion":1,"tests":[null]}`,
	}
	for index, input := range invalid {
		t.Run(fmt.Sprintf("invalid-%d", index), func(t *testing.T) {
			if _, err := parseTestExecutionProtocol(input); err == nil {
				t.Fatalf("expected rejection for %s", input)
			}
		})
	}
}

func TestCoverageCollectorUsesObservedEvaluationMetadata(t *testing.T) {
	assertion := map[string]interface{}{"environment": "production", "target": "checkout"}
	collector := newCoverageCollector("staging", assertion)
	rule := "rule-a"
	variable := featurevisor.VariableKey("policy")
	collector.record(featurevisor.Evaluation{
		Type: featurevisor.EvaluationTypeVariable, FeatureKey: "pricing", VariableKey: &variable,
		RuleKey: &rule, VariableOverridePaths: [][]string{{"root", "first"}, {"root", "second"}},
	})
	collector.record(featurevisor.Evaluation{Type: featurevisor.EvaluationTypeFlag})
	collector.record(featurevisor.Evaluation{
		Type: featurevisor.EvaluationTypeVariable, FeatureKey: "pricing", VariableKey: &variable,
		RuleKey: &rule, VariableOverridePaths: [][]string{{"root", "first"}, {"root", "second"}},
	})
	selections := collector.list()
	if len(selections) != 2 || selections[0].RuleKey != "rule-a" || selections[0].Target == nil || *selections[0].Target != "checkout" {
		t.Fatalf("unexpected selections: %#v", selections)
	}
}

func TestHumanReportLabelsAndCoverageAreReadable(t *testing.T) {
	target := "beta"
	index := 1
	caseResult := reportCase{Description: "runs", Target: &target, MatrixValues: map[string]interface{}{"country": "nl"}}
	if got := formatCaseLabel(caseResult); got != `runs (matrix country="nl"; target beta)` {
		t.Fatalf("case label = %q", got)
	}
	selection := coverageSelection{FeatureKey: "checkout", VariableKey: "label", RuleKey: "all", OverrideIndex: &index}
	if got := formatCoverageSelection(selection); got != "feature checkout variable label: rule all, override 2" {
		t.Fatalf("coverage label = %q", got)
	}
	if got := formatReportPath([]interface{}{"assertions", 0, "children", 1, "checks", 2}); got != "assertions.1.children.2.checks.3" {
		t.Fatalf("human path = %q", got)
	}
	if got := powerShellQuote("a'b"); got != "'a''b'" {
		t.Fatalf("PowerShell quote = %q", got)
	}
}

func TestDiscoverTestSetsReturnsSortedAggregate(t *testing.T) {
	root := t.TempDir()
	for _, set := range []string{"staging", "dev"} {
		if err := os.MkdirAll(filepath.Join(root, "sets", set), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sets, err := discoverTestSets(root, map[string]interface{}{"sets": true, "setsDirectoryPath": "sets"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 2 || sets[0] != "dev" || sets[1] != "staging" {
		t.Fatalf("unexpected sets: %#v", sets)
	}
}

func TestReportSummarySeparatesEntityAndScenarioCounts(t *testing.T) {
	report := newTestReport()
	report.appendSpec(reportSpec{Type: "feature", Passed: true, Cases: []reportCase{{Passed: true}}})
	report.appendSpec(reportSpec{Type: "scenario", Passed: false, Cases: []reportCase{{Passed: false}}})
	if report.Summary.Specs.Passed != 1 || report.Summary.Specs.Failed != 1 || report.Summary.EntityAssertions.Passed != 1 || report.Summary.ScenarioCases.Failed != 1 || report.Passed {
		t.Fatalf("unexpected summary: %#v", report)
	}
}

func TestScenarioReportUsesCheckPathsAndPreservesArbitraryPayloadKeys(t *testing.T) {
	expected := interface{}(true)
	actual := interface{}(false)
	result := AssertionResult{HasError: true, ReportErrors: []reportError{{
		Message: "original failure", Type: "check", Path: []interface{}{"checks", 0},
		Expected: &expected, Actual: &actual,
		Details: map[string]interface{}{
			"evaluated": []map[string]interface{}{{
				"path":      []interface{}{0},
				"reference": map[string]interface{}{"path": "custom"},
			}},
			"payload": map[string]interface{}{"checks": "keep checks", "custom": "keep custom"},
		},
	}}}

	errors := reportErrorsFromResult("scenario", result, 0)
	if len(errors) != 1 || formatReportPath(errors[0].Path) != "checks.1" || errors[0].Message != "Check did not match" {
		t.Fatalf("unexpected scenario error: %#v", errors)
	}
	evaluated := errors[0].Details["evaluated"].([]map[string]interface{})
	if formatReportPath(evaluated[0]["path"].([]interface{})) != "checks.1" {
		t.Fatalf("unexpected evaluated path: %#v", evaluated)
	}
	reference := evaluated[0]["reference"].(map[string]interface{})
	payload := errors[0].Details["payload"].(map[string]interface{})
	if reference["path"] != "custom" || payload["checks"] != "keep checks" || payload["custom"] != "keep custom" {
		t.Fatalf("arbitrary values were renamed: reference=%#v payload=%#v", reference, payload)
	}
}
