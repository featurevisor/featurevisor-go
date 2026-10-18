package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

type reportCounter struct {
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

type reportSummary struct {
	Specs            reportCounter `json:"specs"`
	Assertions       reportCounter `json:"assertions"`
	EntitySpecs      reportCounter `json:"entitySpecs"`
	EntityAssertions reportCounter `json:"entityAssertions"`
	ScenarioSpecs    reportCounter `json:"scenarioSpecs"`
	ScenarioCases    reportCounter `json:"scenarioCases"`
}

type reportError struct {
	Message  string                 `json:"message"`
	Type     string                 `json:"type,omitempty"`
	Path     []interface{}          `json:"path,omitempty"`
	Expected *interface{}           `json:"expected,omitempty"`
	Actual   *interface{}           `json:"actual,omitempty"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

type reportRerun struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type reportCase struct {
	ID           string                 `json:"id"`
	Description  string                 `json:"description"`
	Environment  *string                `json:"environment"`
	Target       *string                `json:"target"`
	Context      map[string]interface{} `json:"context"`
	MatrixValues map[string]interface{} `json:"matrixValues"`
	Passed       bool                   `json:"passed"`
	Errors       []reportError          `json:"errors"`
	Rerun        reportRerun            `json:"rerun"`
}

type reportSpec struct {
	Type   string       `json:"type"`
	Key    string       `json:"key"`
	Source string       `json:"source"`
	Set    *string      `json:"set"`
	Passed bool         `json:"passed"`
	Cases  []reportCase `json:"cases"`
}

type uncoveredEntry struct {
	Set       *string  `json:"set"`
	Features  []string `json:"features"`
	Variables []string `json:"variables"`
}

type coverageSelection struct {
	Set           *string  `json:"set"`
	Environment   *string  `json:"environment"`
	Target        *string  `json:"target"`
	FeatureKey    string   `json:"featureKey,omitempty"`
	VariableKey   string   `json:"variableKey,omitempty"`
	RuleKey       string   `json:"ruleKey,omitempty"`
	Variation     *string  `json:"variation,omitempty"`
	OverridePath  []string `json:"overridePath,omitempty"`
	OverrideIndex *int     `json:"overrideIndex,omitempty"`
}

type coverageReport struct {
	Mode       string              `json:"mode"`
	Selections []coverageSelection `json:"selections"`
}

type testReport struct {
	SchemaVersion int              `json:"schemaVersion"`
	Runner        string           `json:"runner"`
	Passed        bool             `json:"passed"`
	Summary       reportSummary    `json:"summary"`
	Results       []reportSpec     `json:"results"`
	Errors        []reportError    `json:"errors"`
	Uncovered     []uncoveredEntry `json:"uncovered,omitempty"`
	Coverage      *coverageReport  `json:"coverage,omitempty"`
}

func newTestReport() testReport {
	return testReport{SchemaVersion: 1, Runner: "go", Passed: true, Results: []reportSpec{}, Errors: []reportError{}}
}

func (r *testReport) appendSpec(spec reportSpec) {
	r.Results = append(r.Results, spec)
	status := &r.Summary.Specs.Passed
	entityStatus := &r.Summary.EntitySpecs.Passed
	if !spec.Passed {
		r.Passed = false
		status = &r.Summary.Specs.Failed
		entityStatus = &r.Summary.EntitySpecs.Failed
	}
	(*status)++
	if spec.Type == "scenario" {
		if spec.Passed {
			r.Summary.ScenarioSpecs.Passed++
		} else {
			r.Summary.ScenarioSpecs.Failed++
		}
	} else {
		(*entityStatus)++
	}
	for _, testCase := range spec.Cases {
		if testCase.Passed {
			r.Summary.Assertions.Passed++
			if spec.Type == "scenario" {
				r.Summary.ScenarioCases.Passed++
			} else {
				r.Summary.EntityAssertions.Passed++
			}
		} else {
			r.Summary.Assertions.Failed++
			if spec.Type == "scenario" {
				r.Summary.ScenarioCases.Failed++
			} else {
				r.Summary.EntityAssertions.Failed++
			}
		}
	}
}

func (r *testReport) appendRootError(err error) {
	r.Passed = false
	r.Errors = append(r.Errors, reportError{Message: err.Error()})
}

func executeFeaturevisorJSON(featurevisorProjectPath string, args ...string) (string, error) {
	cmdArgs := append([]string{"featurevisor"}, args...)
	cmd := execCommand("npx", cmdArgs...)
	cmd.Dir = featurevisorProjectPath
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message != "" {
			return "", fmt.Errorf("command failed: npx %s\n%s", strings.Join(cmdArgs, " "), message)
		}
		return "", fmt.Errorf("command failed: npx %s: %w", strings.Join(cmdArgs, " "), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Assigned for focused command tests without changing production process execution.
var execCommand = exec.Command

func readFeaturevisorJSON(featurevisorProjectPath string, destination interface{}, args ...string) error {
	output, err := executeFeaturevisorJSON(featurevisorProjectPath, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(output), destination); err != nil {
		return fmt.Errorf("failed to parse JSON from npx featurevisor %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func parseTestExecutionProtocol(output string) ([]map[string]interface{}, error) {
	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse test execution protocol: %w", err)
	}
	if envelope["kind"] != "featurevisor.test-execution" {
		return nil, fmt.Errorf("invalid test execution protocol kind")
	}
	version, ok := envelope["schemaVersion"].(float64)
	if !ok || version != 1 {
		return nil, fmt.Errorf("unsupported test execution protocol schemaVersion %v", envelope["schemaVersion"])
	}
	rawTests, ok := envelope["tests"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("test execution protocol tests must be an array")
	}
	tests := make([]map[string]interface{}, 0, len(rawTests))
	for index, rawTest := range rawTests {
		test, ok := rawTest.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("test execution protocol tests[%d] must be an object", index)
		}
		tests = append(tests, test)
	}
	return tests, nil
}

func discoverTestSets(featurevisorProjectPath string, config map[string]interface{}, selected string, supplied bool) ([]string, error) {
	setsEnabled, _ := config["sets"].(bool)
	if (selected != "" || supplied) && !setsEnabled {
		return nil, fmt.Errorf("this project does not use Sets. Omit --set")
	}
	if selected != "" {
		return []string{selected}, nil
	}
	if !setsEnabled {
		return []string{""}, nil
	}
	setsPath, _ := config["setsDirectoryPath"].(string)
	if setsPath == "" {
		setsPath = filepath.Join(featurevisorProjectPath, "sets")
	} else if !filepath.IsAbs(setsPath) {
		setsPath = filepath.Join(featurevisorProjectPath, setsPath)
	}
	entries, err := os.ReadDir(setsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list project sets: %w", err)
	}
	sets := []string{}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			sets = append(sets, entry.Name())
		}
	}
	sort.Strings(sets)
	if len(sets) == 0 {
		return nil, fmt.Errorf("no project sets found")
	}
	return sets, nil
}

func commandSetArg(set string) []string {
	if set == "" {
		return nil
	}
	return []string{"--set=" + set}
}

func getTestsForReport(featurevisorProjectPath string, opts CLIOptions, set string) ([]map[string]interface{}, error) {
	args := []string{"list", "--tests", "--execution-protocol", "--apply-matrix", "--execution-metadata", "--json"}
	if opts.Case != "" {
		args = append(args, "--case="+opts.Case)
	}
	if opts.KeyPattern != "" {
		args = append(args, "--keyPattern="+opts.KeyPattern)
	}
	if opts.AssertionPattern != "" {
		args = append(args, "--assertionPattern="+opts.AssertionPattern)
	}
	if opts.EntityType != "" {
		args = append(args, "--entityType="+opts.EntityType)
	}
	args = append(args, commandSetArg(set)...)
	output, err := executeFeaturevisorJSON(featurevisorProjectPath, args...)
	if err != nil {
		return nil, err
	}
	return parseTestExecutionProtocol(output)
}

func getConfigForReport(featurevisorProjectPath, set string) (map[string]interface{}, error) {
	args := []string{"config", "--json"}
	args = append(args, commandSetArg(set)...)
	config := map[string]interface{}{}
	if err := readFeaturevisorJSON(featurevisorProjectPath, &config, args...); err != nil {
		return nil, err
	}
	return config, nil
}

func getSegmentsForReport(featurevisorProjectPath, set string) (map[string]interface{}, error) {
	args := []string{"list", "--segments", "--json"}
	args = append(args, commandSetArg(set)...)
	var segments []map[string]interface{}
	if err := readFeaturevisorJSON(featurevisorProjectPath, &segments, args...); err != nil {
		return nil, err
	}
	byKey := map[string]interface{}{}
	for _, segment := range segments {
		if key, ok := segment["key"].(string); ok {
			byKey[key] = segment
		}
	}
	return byKey, nil
}

func getTargetsForReport(featurevisorProjectPath, set string) ([]string, error) {
	args := []string{"list", "--targets", "--json"}
	args = append(args, commandSetArg(set)...)
	var targets []map[string]interface{}
	if err := readFeaturevisorJSON(featurevisorProjectPath, &targets, args...); err != nil {
		return nil, err
	}
	keys := []string{}
	for _, target := range targets {
		if key, ok := target["key"].(string); ok {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func buildDatafileForReport(featurevisorProjectPath string, environment *string, target *string, inflate int, set string) (interface{}, error) {
	args := []string{"build"}
	if environment != nil {
		args = append(args, "--environment="+*environment)
	}
	if inflate > 0 {
		args = append(args, fmt.Sprintf("--inflate=%d", inflate))
	}
	if target != nil {
		args = append(args, "--target="+*target)
	}
	args = append(args, commandSetArg(set)...)
	args = append(args, "--json")
	var datafile featurevisor.DatafileContent
	if err := readFeaturevisorJSON(featurevisorProjectPath, &datafile, args...); err != nil {
		return nil, err
	}
	return datafile, nil
}

func buildDatafileCacheForReport(featurevisorProjectPath string, config map[string]interface{}, opts CLIOptions, set string) (map[string]interface{}, error) {
	targets := opts.Targets
	if len(targets) == 0 {
		var err error
		targets, err = getTargetsForReport(featurevisorProjectPath, set)
		if err != nil {
			return nil, err
		}
	}
	environments := []*string{nil}
	if raw, ok := config["environments"].([]interface{}); ok {
		environments = []*string{}
		for _, item := range raw {
			if environment, ok := item.(string); ok {
				copy := environment
				environments = append(environments, &copy)
			}
		}
	}
	cache := map[string]interface{}{}
	for _, environment := range environments {
		datafile, err := buildDatafileForReport(featurevisorProjectPath, environment, nil, opts.Inflate, set)
		if err != nil {
			return nil, err
		}
		cache[datafileCacheKey(environment)] = datafile
		for _, target := range targets {
			copy := target
			datafile, err := buildDatafileForReport(featurevisorProjectPath, environment, &copy, opts.Inflate, set)
			if err != nil {
				return nil, err
			}
			cache[targetDatafileCacheKey(environment, target)] = datafile
		}
	}
	return cache, nil
}

func getUncoveredForReport(featurevisorProjectPath, set string) (uncoveredEntry, error) {
	readKeys := func(entity string) ([]string, error) {
		args := []string{"list", "--" + entity, "--without-tests", "--json"}
		args = append(args, commandSetArg(set)...)
		var entities []map[string]interface{}
		if err := readFeaturevisorJSON(featurevisorProjectPath, &entities, args...); err != nil {
			return nil, err
		}
		keys := []string{}
		for _, entity := range entities {
			if key, ok := entity["key"].(string); ok {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		return keys, nil
	}
	features, err := readKeys("features")
	if err != nil {
		return uncoveredEntry{}, err
	}
	variables, err := readKeys("variables")
	if err != nil {
		return uncoveredEntry{}, err
	}
	return uncoveredEntry{Set: nullableString(set), Features: features, Variables: variables}, nil
}

type coverageCollector struct {
	synthetic   map[string]bool
	mutex       sync.Mutex
	set         *string
	environment *string
	target      *string
	selections  map[string]coverageSelection
}

func newCoverageCollector(set string, assertion map[string]interface{}) *coverageCollector {
	return &coverageCollector{
		set: nullableString(set), environment: optionalString(assertion, "environment"), target: optionalString(assertion, "target"),
		selections: map[string]coverageSelection{},
	}
}

func (c *coverageCollector) record(evaluation featurevisor.Evaluation) {
	if c == nil || c.synthetic[evaluation.FeatureKey] {
		return
	}
	base := coverageSelection{Set: c.set, Environment: c.environment, Target: c.target, FeatureKey: string(evaluation.FeatureKey)}
	if evaluation.VariableKey != nil {
		base.VariableKey = string(*evaluation.VariableKey)
	}
	if evaluation.RuleKey != nil {
		base.RuleKey = string(*evaluation.RuleKey)
	}
	if evaluation.Variation != nil {
		value := string(evaluation.Variation.Value)
		base.Variation = &value
	}
	paths := evaluation.VariableOverridePaths
	if len(paths) == 0 && len(evaluation.VariableOverridePath) > 0 {
		paths = [][]string{evaluation.VariableOverridePath}
	}
	if len(paths) == 0 && evaluation.VariableOverrideKey != nil {
		paths = [][]string{{*evaluation.VariableOverrideKey}}
	}
	if base.RuleKey == "" && len(paths) == 0 && evaluation.VariableOverrideIndex == nil {
		return
	}
	if len(paths) == 0 {
		if evaluation.VariableOverrideIndex != nil {
			base.OverrideIndex = evaluation.VariableOverrideIndex
		}
		c.add(base)
		return
	}
	for _, path := range paths {
		entry := base
		entry.OverridePath = append([]string{}, path...)
		c.add(entry)
	}
}

func (c *coverageCollector) add(selection coverageSelection) {
	raw, _ := json.Marshal(selection)
	c.mutex.Lock()
	c.selections[string(raw)] = selection
	c.mutex.Unlock()
}

func (c *coverageCollector) list() []coverageSelection {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	keys := make([]string, 0, len(c.selections))
	for key := range c.selections {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]coverageSelection, 0, len(keys))
	for _, key := range keys {
		result = append(result, c.selections[key])
	}
	return result
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func optionalString(object map[string]interface{}, key string) *string {
	value, ok := object[key].(string)
	if !ok || value == "" {
		return nil
	}
	return nullableString(value)
}

func testTypeAndKey(test map[string]interface{}) (string, string, error) {
	typeName := ""
	key := ""
	for _, candidate := range []string{"feature", "segment", "variable", "scenario"} {
		if value, exists := test[candidate]; exists {
			if typeName != "" {
				return "", "", fmt.Errorf("test must contain exactly one of feature, segment, variable, or scenario")
			}
			typeName = candidate
			key, _ = value.(string)
		}
	}
	if typeName == "" {
		return "", "", fmt.Errorf("test must contain exactly one of feature, segment, variable, or scenario")
	}
	return typeName, key, nil
}

func reportMetadata(test map[string]interface{}, fallbackSet string) (string, *string) {
	source := ""
	set := nullableString(fallbackSet)
	if metadata, ok := test["_test"].(map[string]interface{}); ok {
		source, _ = metadata["source"].(string)
		if rawSet, exists := metadata["set"]; exists {
			if rawSet == nil {
				set = nil
			} else if value, ok := rawSet.(string); ok {
				set = nullableString(value)
			}
		}
	}
	if source == "" {
		source, _ = test["key"].(string)
	}
	return source, set
}

func reportCaseMetadata(assertion map[string]interface{}, set *string, typeName, storageKey string, index int) (string, map[string]interface{}) {
	if metadata, ok := assertion["_case"].(map[string]interface{}); ok {
		id, _ := metadata["id"].(string)
		matrixValues, _ := metadata["matrixValues"].(map[string]interface{})
		if matrixValues == nil {
			matrixValues = map[string]interface{}{}
		}
		if id != "" {
			return id, matrixValues
		}
	}
	var setValue interface{}
	if set != nil {
		setValue = *set
	}
	raw, _ := json.Marshal([]interface{}{setValue, typeName, storageKey, index, nil})
	return string(raw), map[string]interface{}{}
}

func expectationReportError(kind string, path []interface{}, expected, actual interface{}) reportError {
	return reportError{
		Message: kind + " expectation did not match", Type: kind, Path: path,
		Expected: reportValuePointer(expected), Actual: reportValuePointer(actual),
	}
}

func reportErrorsFromResult(typeName string, result AssertionResult, assertionIndex int) []reportError {
	if len(result.ReportErrors) == 0 {
		if result.HasError {
			return []reportError{{Message: "Native assertion failed"}}
		}
		return []reportError{}
	}
	errors := make([]reportError, 0, len(result.ReportErrors))
	for _, original := range result.ReportErrors {
		error := original
		base := []interface{}{}
		if typeName != "scenario" {
			base = append(base, "assertions", assertionIndex)
		}
		error.Path = reportAuthoredPath(base, error.Path)
		if error.Type == "check" && reportBoolean(error.Expected, true) && reportBoolean(error.Actual, false) {
			error.Message = "Check did not match"
		}
		if error.Details != nil {
			details := cloneReportDetails(error.Details)
			childBase := append([]interface{}{}, base...)
			if childIndex, ok := details["childIndex"].(int); ok {
				childBase = append(childBase, "children", childIndex)
			}
			if evaluated, ok := details["evaluated"].([]map[string]interface{}); ok {
				converted := make([]map[string]interface{}, 0, len(evaluated))
				for _, originalTrace := range evaluated {
					trace := cloneReportDetails(originalTrace)
					if path, ok := trace["path"].([]interface{}); ok {
						trace["path"] = appendPath(childBase, append([]interface{}{"checks"}, path...)...)
					}
					converted = append(converted, trace)
				}
				details["evaluated"] = converted
			}
			error.Details = details
		}
		errors = append(errors, error)
	}
	return errors
}

func reportBoolean(value *interface{}, expected bool) bool {
	if value == nil {
		return false
	}
	actual, ok := (*value).(bool)
	return ok && actual == expected
}

func reportAuthoredPath(base, local []interface{}) []interface{} {
	if len(local) == 0 {
		return nil
	}
	return appendPath(base, local...)
}

func cloneReportDetails(value map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func powerShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func formatRerun(rerun reportRerun, powerShell bool) string {
	quoted := make([]string, len(rerun.Args))
	for index, arg := range rerun.Args {
		if powerShell {
			quoted[index] = powerShellQuote(arg)
		} else {
			quoted[index] = shellQuote(arg)
		}
	}
	return strings.TrimSpace(rerun.Command + " " + strings.Join(quoted, " "))
}

func formatCaseLabel(testCase reportCase) string {
	label := testCase.Description
	if label == "" {
		label = "assertion"
	}
	details := []string{}
	if len(testCase.MatrixValues) > 0 {
		keys := make([]string, 0, len(testCase.MatrixValues))
		for key := range testCase.MatrixValues {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		values := make([]string, 0, len(keys))
		for _, key := range keys {
			raw, _ := json.Marshal(testCase.MatrixValues[key])
			values = append(values, key+"="+string(raw))
		}
		details = append(details, "matrix "+strings.Join(values, ", "))
	}
	if testCase.Target != nil {
		details = append(details, "target "+*testCase.Target)
	}
	if len(details) > 0 {
		label += " (" + strings.Join(details, "; ") + ")"
	}
	return label
}

func formatCoverageSelection(selection coverageSelection) string {
	owner := "global variable " + selection.VariableKey
	if selection.FeatureKey != "" {
		owner = "feature " + selection.FeatureKey
		if selection.VariableKey != "" {
			owner += " variable " + selection.VariableKey
		}
	}
	scope := []string{}
	if selection.Set != nil && *selection.Set != "" {
		scope = append(scope, "set "+*selection.Set)
	}
	if selection.Environment != nil && *selection.Environment != "" {
		scope = append(scope, "environment "+*selection.Environment)
	}
	if selection.Target != nil && *selection.Target != "" {
		scope = append(scope, "target "+*selection.Target)
	}
	if len(scope) > 0 {
		owner += " (" + strings.Join(scope, ", ") + ")"
	}
	parts := []string{}
	if selection.RuleKey != "" {
		parts = append(parts, "rule "+selection.RuleKey)
	}
	if selection.Variation != nil {
		raw, _ := json.Marshal(*selection.Variation)
		parts = append(parts, "variation "+string(raw))
	}
	if len(selection.OverridePath) > 0 {
		parts = append(parts, "override "+strings.Join(selection.OverridePath, " > "))
	} else if selection.OverrideIndex != nil {
		parts = append(parts, fmt.Sprintf("override %d", *selection.OverrideIndex+1))
	}
	return owner + ": " + strings.Join(parts, ", ")
}

func printHumanReport(report testReport, opts CLIOptions) {
	for _, spec := range report.Results {
		if opts.OnlyFailures && spec.Passed {
			continue
		}
		fmt.Printf("\nTesting: %s\n", spec.Key)
		for _, testCase := range spec.Cases {
			if opts.OnlyFailures && testCase.Passed {
				continue
			}
			mark := "✔"
			if !testCase.Passed {
				mark = "✘"
			}
			fmt.Printf("  %s %s\n", mark, formatCaseLabel(testCase))
			for _, err := range testCase.Errors {
				label := humaniseChildMessage(err.Message)
				if err.Type == "check" && len(err.Path) > 0 {
					label = formatReportPath(err.Path)
					if key, ok := err.Details["checkKey"].(string); ok {
						label += " " + fmt.Sprintf("%q", key)
					}
					if description, ok := err.Details["checkDescription"].(string); ok {
						label += ": " + description
					}
					if segment := segmentKeyFromCheckDetails(err.Details); segment != "" {
						label += fmt.Sprintf(" [segment %q]", segment)
					}
					label += ": " + humaniseChildMessage(err.Message)
				}
				fmt.Printf("      ✘ %s\n", label)
			}
			if !testCase.Passed {
				powerShell := runtime.GOOS == "windows"
				if powerShell {
					fmt.Printf("      Rerun (PowerShell): %s\n", formatRerun(testCase.Rerun, true))
				} else {
					fmt.Printf("      Rerun (POSIX shell): %s\n", formatRerun(testCase.Rerun, false))
				}
			}
		}
	}
	fmt.Println()
	fmt.Printf("Entity specs: %d passed, %d failed\n", report.Summary.EntitySpecs.Passed, report.Summary.EntitySpecs.Failed)
	fmt.Printf("Entity assertions: %d passed, %d failed\n", report.Summary.EntityAssertions.Passed, report.Summary.EntityAssertions.Failed)
	fmt.Printf("Scenario specs: %d passed, %d failed\n", report.Summary.ScenarioSpecs.Passed, report.Summary.ScenarioSpecs.Failed)
	fmt.Printf("Scenario cases: %d passed, %d failed\n", report.Summary.ScenarioCases.Passed, report.Summary.ScenarioCases.Failed)
	fmt.Printf("Total cases: %d passed, %d failed\n", report.Summary.Assertions.Passed, report.Summary.Assertions.Failed)
	if report.Uncovered != nil {
		fmt.Println("\nDefinitions without associated specs (not evaluation coverage):")
		for _, entry := range report.Uncovered {
			if entry.Set != nil {
				fmt.Printf("  Set: %s\n", *entry.Set)
			}
			fmt.Printf("  Features: %s\n", strings.Join(entry.Features, ", "))
			fmt.Printf("  Global variables: %s\n", strings.Join(entry.Variables, ", "))
		}
	}
	if report.Coverage != nil {
		fmt.Println("\nObserved rule and override selections (not branch coverage):")
		if len(report.Coverage.Selections) == 0 {
			fmt.Println("  No selections observed")
		}
		for _, selection := range report.Coverage.Selections {
			fmt.Printf("  %s\n", formatCoverageSelection(selection))
		}
	}
	for _, err := range report.Errors {
		fmt.Fprintln(os.Stderr, err.Message)
	}
	fmt.Println()
}

func humaniseChildMessage(message string) string {
	start := strings.Index(message, "children[")
	if start < 0 {
		return message
	}
	indexStart := start + len("children[")
	end := strings.IndexByte(message[indexStart:], ']')
	if end < 0 {
		return message
	}
	end += indexStart
	index, err := strconv.Atoi(message[indexStart:end])
	if err != nil {
		return message
	}
	rest := message[end+1:]
	if strings.HasPrefix(rest, ".") {
		rest = ", " + strings.TrimPrefix(rest, ".")
	}
	return message[:start] + fmt.Sprintf("Child %d", index+1) + rest
}

func segmentKeyFromCheckDetails(details map[string]interface{}) string {
	evaluated, _ := details["evaluated"].([]map[string]interface{})
	for _, trace := range evaluated {
		reference, _ := trace["reference"].(map[string]interface{})
		if segment, ok := reference["segment"].(string); ok {
			return segment
		}
	}
	return ""
}

func formatReportPath(path []interface{}) string {
	var result strings.Builder
	for _, part := range path {
		switch value := part.(type) {
		case string:
			if result.Len() > 0 {
				result.WriteByte('.')
			}
			result.WriteString(value)
		case int:
			fmt.Fprintf(&result, ".%d", value+1)
		case float64:
			fmt.Fprintf(&result, ".%d", int(value)+1)
		}
	}
	return result.String()
}

func printJSONReport(report testReport, pretty bool) {
	var raw []byte
	if pretty {
		raw, _ = json.MarshalIndent(report, "", "  ")
	} else {
		raw, _ = json.Marshal(report)
	}
	fmt.Println(string(raw))
}

func buildInstanceForReport(datafile interface{}, level string, assertion map[string]interface{}, collector *coverageCollector) (*featurevisor.Featurevisor, *checkDiagnosticCollector, error) {
	content, cached := datafile.(featurevisor.DatafileContent)
	if !cached {
		raw, err := json.Marshal(datafile)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal datafile for assertion: %w", err)
		}
		if err := json.Unmarshal(raw, &content); err != nil {
			return nil, nil, fmt.Errorf("failed to parse datafile for assertion: %w", err)
		}
	}
	levelValue := featurevisor.LogLevel(level)
	diagnostics := &checkDiagnosticCollector{}
	module := &featurevisor.FeaturevisorModule{
		Name: "tester-module",
		BucketValue: func(options featurevisor.ConfigureBucketValueOptions) int {
			if at, ok := assertion["at"].(float64); ok {
				return int(at * 1000)
			}
			return options.BucketValue
		},
	}
	if collector != nil {
		module.AfterEvaluation = func(evaluation featurevisor.Evaluation, options featurevisor.EvaluateOptions) featurevisor.Evaluation {
			collector.record(evaluation)
			return evaluation
		}
	}
	instance := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
		Datafile: content, LogLevel: &levelValue, Context: contextFromAssertion(assertion),
		StickyFeatures:  stickyFeaturesFromAssertion(assertion, "stickyFeatures"),
		StickyVariables: stickyVariablesFromAssertion(assertion), OnDiagnostic: diagnostics.record,
		Modules: []*featurevisor.FeaturevisorModule{module},
	})
	return instance, diagnostics, nil
}

func runTest(opts CLIOptions) {
	report := newTestReport()
	allCoverage := map[string]coverageSelection{}
	rootConfig, err := getConfigForReport(opts.ProjectDirectoryPath, "")
	if err != nil {
		report.appendRootError(err)
		finishTestReport(report, opts)
		return
	}
	sets, err := discoverTestSets(opts.ProjectDirectoryPath, rootConfig, opts.Set, opts.SetProvided)
	if err != nil {
		report.appendRootError(err)
		finishTestReport(report, opts)
		return
	}
	caseLookupErrors := []error{}
	matchedExactCase := false
	for _, set := range sets {
		tests, testErr := getTestsForReport(opts.ProjectDirectoryPath, opts, set)
		if testErr != nil {
			if opts.Case != "" && len(sets) > 1 {
				caseLookupErrors = append(caseLookupErrors, testErr)
				continue
			}
			report.appendRootError(testErr)
			break
		}
		if opts.Case != "" && len(tests) == 0 {
			continue
		}
		if opts.Case != "" {
			matchedExactCase = true
		}
		config := rootConfig
		segments, err := getSegmentsForReport(opts.ProjectDirectoryPath, set)
		if err != nil {
			report.appendRootError(err)
			break
		}
		cache, err := buildDatafileCacheForReport(opts.ProjectDirectoryPath, config, opts, set)
		if err != nil {
			report.appendRootError(err)
			break
		}
		for _, test := range tests {
			spec, collectors := executeReportSpec(test, segments, cache, opts, set)
			report.appendSpec(spec)
			for _, collector := range collectors {
				for _, selection := range collector.list() {
					raw, _ := json.Marshal(selection)
					allCoverage[string(raw)] = selection
				}
			}
		}
		if opts.Uncovered {
			entry, err := getUncoveredForReport(opts.ProjectDirectoryPath, set)
			if err != nil {
				report.appendRootError(err)
				break
			}
			report.Uncovered = append(report.Uncovered, entry)
		}
	}
	if opts.Case != "" && !matchedExactCase && len(report.Errors) == 0 {
		if len(caseLookupErrors) > 0 {
			report.appendRootError(caseLookupErrors[0])
		} else {
			report.appendRootError(fmt.Errorf("no test case matched --case=%s", opts.Case))
		}
	}
	if opts.Coverage {
		keys := make([]string, 0, len(allCoverage))
		for key := range allCoverage {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		selections := make([]coverageSelection, 0, len(keys))
		for _, key := range keys {
			selections = append(selections, allCoverage[key])
		}
		report.Coverage = &coverageReport{Mode: "observed-selections", Selections: selections}
	}
	finishTestReport(report, opts)
}

func executeReportSpec(test map[string]interface{}, segments map[string]interface{}, cache map[string]interface{}, opts CLIOptions, fallbackSet string) (reportSpec, []*coverageCollector) {
	typeName, entityKey, discriminatorErr := testTypeAndKey(test)
	storageKey, _ := test["key"].(string)
	if storageKey == "" {
		storageKey = entityKey
	}
	source, set := reportMetadata(test, fallbackSet)
	spec := reportSpec{Type: typeName, Key: entityKey, Source: source, Set: set, Passed: true, Cases: []reportCase{}}
	collectors := []*coverageCollector{}
	assertions, assertionsOK := test["assertions"].([]interface{})
	if discriminatorErr != nil || !assertionsOK || len(assertions) == 0 {
		message := "test must contain a nonempty assertions array"
		if discriminatorErr != nil {
			message = discriminatorErr.Error()
		}
		id, matrixValues := reportCaseMetadata(map[string]interface{}{}, set, typeName, storageKey, 0)
		spec.Cases = append(spec.Cases, newReportCase(map[string]interface{}{}, id, matrixValues, set, opts, []reportError{{Message: message}}))
		spec.Passed = false
		return spec, collectors
	}
	for index, rawAssertion := range assertions {
		assertion, ok := rawAssertion.(map[string]interface{})
		if !ok {
			id, matrixValues := reportCaseMetadata(map[string]interface{}{}, set, typeName, storageKey, index)
			spec.Cases = append(spec.Cases, newReportCase(map[string]interface{}{}, id, matrixValues, set, opts, []reportError{{Message: "assertion must be an object"}}))
			spec.Passed = false
			continue
		}
		if len(opts.Targets) > 0 && typeName != "segment" {
			target, _ := assertion["target"].(string)
			if target != "" && !containsString(opts.Targets, target) {
				continue
			}
		}
		id, matrixValues := reportCaseMetadata(assertion, set, typeName, storageKey, index)
		selectedDatafileKey := datafileCacheKeyForAssertion(assertion, cache)
		var result AssertionResult
		collector := (*coverageCollector)(nil)
		if typeName == "segment" {
			result = runSegmentSpec(assertion, entityKey, test, segments, cache, getLoggerLevel(opts))
		} else if datafile, found := cache[selectedDatafileKey]; !found {
			result = missingDatafileAssertion(assertion, selectedDatafileKey)
		} else {
			if opts.ShowDatafile && !opts.JSON {
				raw, _ := json.MarshalIndent(datafile, "", "  ")
				fmt.Fprintln(os.Stderr, string(raw))
			}
			if opts.Coverage {
				collector = newCoverageCollector(fallbackSet, assertion)
				collector.synthetic = map[string]bool{}
				for _, key := range segmentGateKeys(datafileContentFromRaw(datafile), segmentDefinitionsFromTest(test)) {
					collector.synthetic[key] = true
				}
				collectors = append(collectors, collector)
			}
			instance, diagnostics, err := buildInstanceForReport(datafile, getLoggerLevel(opts), assertion, collector)
			if err != nil {
				result = AssertionResult{HasError: true, Errors: "      ✘ " + err.Error() + "\n"}
			} else {
				content := datafileContentFromRaw(datafile)
				switch typeName {
				case "feature":
					result = RunTestFeature(cloneAssertion(assertion), entityKey, instance, getLoggerLevel(opts))
				case "variable":
					result = RunTestVariable(assertion, entityKey, instance, diagnostics, content, segmentDefinitionsFromTest(test), getLoggerLevel(opts))
				case "scenario":
					result = RunTestScenario(assertion, instance, diagnostics, content, segmentDefinitionsFromTest(test), getLoggerLevel(opts))
				}
				instance.Close()
			}
		}
		errors := reportErrorsFromResult(typeName, result, reportAssertionIndex(assertion, index))
		testCase := newReportCase(assertion, id, matrixValues, set, opts, errors)
		spec.Cases = append(spec.Cases, testCase)
		if !testCase.Passed {
			spec.Passed = false
		}
	}
	return spec, collectors
}

func reportAssertionIndex(assertion map[string]interface{}, fallback int) int {
	if metadata, ok := assertion["_case"].(map[string]interface{}); ok {
		if value, ok := metadata["assertionIndex"].(float64); ok {
			return int(value)
		}
		if value, ok := metadata["assertionIndex"].(int); ok {
			return value
		}
	}
	return fallback
}

func newReportCase(assertion map[string]interface{}, id string, matrixValues map[string]interface{}, set *string, opts CLIOptions, errors []reportError) reportCase {
	description, _ := assertion["description"].(string)
	args := []string{"test", "--case=" + id}
	if opts.ProjectDirectoryPath != "" {
		args = append(args, "--projectDirectoryPath="+opts.ProjectDirectoryPath)
	}
	if set != nil {
		args = append(args, "--set="+*set)
	}
	for _, target := range opts.Targets {
		args = append(args, "--target="+target)
	}
	context := toContextMap(assertion["context"])
	return reportCase{
		ID: id, Description: description, Environment: optionalString(assertion, "environment"),
		Target: optionalString(assertion, "target"), Context: context, MatrixValues: matrixValues,
		Passed: len(errors) == 0, Errors: errors,
		Rerun: reportRerun{Command: "go run cmd/main.go", Args: args},
	}
}

func finishTestReport(report testReport, opts CLIOptions) {
	if opts.JSON {
		printJSONReport(report, opts.Pretty)
	} else {
		printHumanReport(report, opts)
	}
	if !report.Passed {
		os.Exit(1)
	}
}
