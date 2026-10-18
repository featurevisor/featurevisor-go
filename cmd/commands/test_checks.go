package commands

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

type checkVariableEvaluator interface {
	ResolveGlobalVariable(variableKey string) (featurevisor.VariableValue, error)
	ResolveFeatureVariable(featureKey string, variableKey string) (featurevisor.VariableValue, error)
	ResolveFeatureEnabled(featureKey string) (bool, error)
	ResolveFeatureVariation(featureKey string) (*string, error)
}

type checkSegmentEvaluator interface {
	ResolveSegment(segmentKey string) (bool, error)
}

type checkSegmentValidator interface {
	ValidateSegment(segmentKey string) error
}

type checkDiagnosticCollector struct {
	mutex  sync.Mutex
	errors []string
}

func (c *checkDiagnosticCollector) record(diagnostic featurevisor.FeaturevisorDiagnostic) {
	if diagnostic.Level != featurevisor.LogLevelError && diagnostic.Level != featurevisor.LogLevelFatal {
		return
	}
	c.mutex.Lock()
	c.errors = append(c.errors, fmt.Sprintf("%s: %s", diagnostic.Code, diagnostic.Message))
	c.mutex.Unlock()
}
func (c *checkDiagnosticCollector) clear() { c.mutex.Lock(); c.errors = nil; c.mutex.Unlock() }
func (c *checkDiagnosticCollector) take() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if len(c.errors) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(c.errors, "; "))
}

type nativeCheckGetter interface {
	GetGlobalVariable(variableKey string, args ...interface{}) featurevisor.VariableValue
	GetVariable(featureKey string, variableKey string, args ...interface{}) featurevisor.VariableValue
	IsEnabled(featureKey string, args ...interface{}) bool
	GetVariation(featureKey string, args ...interface{}) *string
}
type nativeCheckResolver struct {
	datafile           featurevisor.DatafileContent
	segmentGates       map[string]string
	getter             nativeCheckGetter
	diagnostics        *checkDiagnosticCollector
	segmentDefinitions map[string]interface{}
	context            featurevisor.Context
}

func (r nativeCheckResolver) ResolveGlobalVariable(key string) (featurevisor.VariableValue, error) {
	r.diagnostics.clear()
	value := r.getter.GetGlobalVariable(key)
	return value, r.diagnostics.take()
}
func (r nativeCheckResolver) ResolveFeatureVariable(feature, variable string) (featurevisor.VariableValue, error) {
	r.diagnostics.clear()
	value := r.getter.GetVariable(feature, variable)
	return value, r.diagnostics.take()
}
func (r nativeCheckResolver) ResolveFeatureEnabled(feature string) (value bool, err error) {
	r.diagnostics.clear()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
			return
		}
		if diagnosticErr := r.diagnostics.take(); diagnosticErr != nil {
			err = diagnosticErr
		}
	}()
	value = r.getter.IsEnabled(feature)
	return value, nil
}
func (r nativeCheckResolver) ResolveFeatureVariation(feature string) (value *string, err error) {
	r.diagnostics.clear()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
			return
		}
		if diagnosticErr := r.diagnostics.take(); diagnosticErr != nil {
			err = diagnosticErr
		}
	}()
	value = r.getter.GetVariation(feature)
	return value, nil
}

func (r nativeCheckResolver) ValidateSegment(segmentKey string) error {
	return validateSegmentReferences(segmentKey, r.segmentDefinitions, r.datafile)
}

func (r nativeCheckResolver) ResolveSegment(segmentKey string) (bool, error) {
	if err := r.ValidateSegment(segmentKey); err != nil {
		return false, err
	}
	key, exists := r.segmentGates[segmentKey]
	if !exists {
		return false, fmt.Errorf("segment %q SDK gate is missing", segmentKey)
	}
	r.diagnostics.clear()
	value := r.getter.IsEnabled(key, r.context)
	return value, r.diagnostics.take()
}

type missingCheckValue struct{}

var missingValue = missingCheckValue{}

type checkAssertionOutcome struct {
	Failures     []string
	HadError     bool
	ReportErrors []reportError
}

func evaluateAssertionChecks(assertion map[string]interface{}, ownerVariable string, evaluator checkVariableEvaluator, datafile featurevisor.DatafileContent, prefix string) []string {
	return evaluateAssertionChecksAt(assertion, ownerVariable, evaluator, datafile, prefix, nil).Failures
}

func evaluateAssertionChecksDetailed(assertion map[string]interface{}, ownerVariable string, evaluator checkVariableEvaluator, datafile featurevisor.DatafileContent, prefix string) checkAssertionOutcome {
	return evaluateAssertionChecksAt(assertion, ownerVariable, evaluator, datafile, prefix, nil)
}

func preflightAssertionChecksAt(assertion map[string]interface{}, ownerVariable string, evaluator checkVariableEvaluator, datafile featurevisor.DatafileContent, prefix string, basePath []interface{}) checkAssertionOutcome {
	rawChecks, exists := assertion["checks"]
	if !exists {
		return checkAssertionOutcome{}
	}
	checks, ok := rawChecks.([]interface{})
	if !ok || len(checks) == 0 {
		message := fmt.Sprintf("%schecks: expected a nonempty array", prefix)
		return checkAssertionOutcome{Failures: []string{message}, HadError: true, ReportErrors: []reportError{{
			Message: message, Type: "check", Path: appendPath(basePath, "checks"),
			Expected: reportValuePointer("valid checks"), Actual: reportValuePointer(rawChecks),
		}}}
	}
	if err := validateCheckList(checks, ownerVariable, prefix+"checks"); err != nil {
		return checkAssertionOutcome{Failures: []string{err.Error()}, HadError: true, ReportErrors: []reportError{{
			Message: err.Error(), Type: "check", Expected: reportValuePointer("valid checks"), Actual: reportValuePointer(rawChecks),
		}}}
	}
	if err := validateCheckReferences(checks, ownerVariable, evaluator, datafile, prefix+"checks"); err != nil {
		return checkAssertionOutcome{Failures: []string{err.Error()}, HadError: true, ReportErrors: []reportError{{
			Message: err.Error(), Type: "check", Path: appendPath(basePath, "checks"), Expected: reportValuePointer("available check references"), Actual: reportValuePointer(nil),
		}}}
	}
	return checkAssertionOutcome{}
}

func evaluateAssertionChecksAt(assertion map[string]interface{}, ownerVariable string, evaluator checkVariableEvaluator, datafile featurevisor.DatafileContent, prefix string, basePath []interface{}) checkAssertionOutcome {
	if outcome := preflightAssertionChecksAt(assertion, ownerVariable, evaluator, datafile, prefix, basePath); outcome.HadError {
		return outcome
	}
	rawChecks, exists := assertion["checks"]
	if !exists {
		return checkAssertionOutcome{}
	}
	checks := rawChecks.([]interface{})
	errors := []string{}
	reportErrors := []reportError{}
	hadError := false
	for index, rawCheck := range checks {
		path := fmt.Sprintf("%schecks[%d]", prefix, index)
		evaluated := []map[string]interface{}{}
		matched, failure, err := evaluateCheck(rawCheck, ownerVariable, evaluator, datafile, path, []interface{}{index}, &evaluated)
		check, _ := rawCheck.(map[string]interface{})
		details := map[string]interface{}{"checkIndex": index, "evaluated": evaluated}
		if key, ok := check["key"].(string); ok {
			details["checkKey"] = key
		}
		if description, ok := check["description"].(string); ok {
			details["checkDescription"] = description
		}
		errorPath := appendPath(basePath, "checks", index)
		if err != nil {
			errors = append(errors, err.Error())
			hadError = true
			reportErrors = append(reportErrors, reportError{Message: err.Error(), Type: "check", Path: errorPath,
				Expected: reportValuePointer("successful comparison"), Actual: reportValuePointer(err.Error()), Details: details})
		} else if !matched {
			errors = append(errors, failure)
			reportErrors = append(reportErrors, reportError{Message: failure, Type: "check", Path: errorPath,
				Expected: reportValuePointer(true), Actual: reportValuePointer(false), Details: details})
		}
	}
	return checkAssertionOutcome{Failures: errors, HadError: hadError, ReportErrors: reportErrors}
}

func validateCheckReferences(checks []interface{}, ownerVariable string, evaluator checkVariableEvaluator, datafile featurevisor.DatafileContent, path string) error {
	var visit func(interface{}, string) error
	visit = func(raw interface{}, currentPath string) error {
		check := raw.(map[string]interface{})
		for _, group := range []string{"and", "or", "not"} {
			if children, ok := check[group].([]interface{}); ok {
				for index, child := range children {
					if err := visit(child, fmt.Sprintf("%s.%s[%d]", currentPath, group, index)); err != nil {
						return err
					}
				}
				return nil
			}
		}
		if segment, ok := check["segment"].(string); ok {
			validator, _ := evaluator.(checkSegmentValidator)
			if validator == nil {
				return fmt.Errorf("%s: segment %q definition is missing", currentPath, segment)
			}
			if err := validator.ValidateSegment(segment); err != nil {
				return fmt.Errorf("%s: %w", currentPath, err)
			}
			return nil
		}
		if _, featureCheck := check["enabled"]; featureCheck {
			return validateFeatureMembership(check, datafile, currentPath)
		}
		if _, featureCheck := check["variation"]; featureCheck {
			return validateFeatureMembership(check, datafile, currentPath)
		}
		if err := validateReferenceMembership(check, ownerVariable, datafile); err != nil {
			return fmt.Errorf("%s: %w", currentPath, err)
		}
		if compareTo, ok := check["compareTo"].(map[string]interface{}); ok {
			if err := validateReferenceMembership(compareTo, ownerVariable, datafile); err != nil {
				return fmt.Errorf("%s.compareTo: %w", currentPath, err)
			}
		}
		return nil
	}
	for index, check := range checks {
		if err := visit(check, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func validateFeatureMembership(check map[string]interface{}, datafile featurevisor.DatafileContent, path string) error {
	feature := check["feature"].(string)
	if _, exists := datafile.Features[feature]; !exists {
		return fmt.Errorf("%s: feature %q is absent from the selected datafile", path, feature)
	}
	return nil
}

func validateReferenceMembership(reference map[string]interface{}, ownerVariable string, datafile featurevisor.DatafileContent) error {
	variable, _ := reference["variable"].(string)
	feature, hasFeature := reference["feature"].(string)
	if variable == "" {
		variable = ownerVariable
	}
	if hasFeature {
		definition, exists := datafile.Features[feature]
		if !exists {
			return fmt.Errorf("feature %q is absent from the selected datafile", feature)
		}
		if _, exists := definition.VariablesSchema[variable]; !exists {
			return fmt.Errorf("feature variable %q.%q is absent from the selected datafile", feature, variable)
		}
		return nil
	}
	if _, exists := datafile.Variables[variable]; !exists {
		return fmt.Errorf("global variable %q is absent from the selected datafile", variable)
	}
	return nil
}

func appendPath(path []interface{}, values ...interface{}) []interface{} {
	result := append([]interface{}{}, path...)
	return append(result, values...)
}

func reportValuePointer(value interface{}) *interface{} { return &value }

func validateCheckList(checks []interface{}, ownerVariable string, path string) error {
	keys := map[string]bool{}
	for index, raw := range checks {
		check, ok := raw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%s[%d]: check must be an object", path, index)
		}
		if err := validateCheckMetadata(check, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
		if key, ok := check["key"].(string); ok {
			if keys[key] {
				return fmt.Errorf("%s[%d].key: duplicate sibling key %q", path, index, key)
			}
			keys[key] = true
		}
		if err := validateCheckShape(check, ownerVariable, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func validateCheckMetadata(check map[string]interface{}, path string) error {
	if raw, exists := check["key"]; exists {
		if value, ok := raw.(string); !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s.key: expected a nonempty string", path)
		}
	}
	if raw, exists := check["description"]; exists {
		if _, ok := raw.(string); !ok {
			return fmt.Errorf("%s.description: expected a string", path)
		}
	}
	return nil
}

func validateCheckShape(raw interface{}, ownerVariable string, path string) error {
	check, ok := raw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("%s: check must be an object", path)
	}
	groups := []string{}
	for _, key := range []string{"and", "or", "not"} {
		if _, exists := check[key]; exists {
			groups = append(groups, key)
		}
	}
	if len(groups) > 0 {
		allowedCount := 1
		if _, exists := check["key"]; exists {
			allowedCount++
		}
		if _, exists := check["description"]; exists {
			allowedCount++
		}
		if len(groups) != 1 || len(check) != allowedCount {
			return fmt.Errorf("%s: group must contain exactly one of and, or, or not", path)
		}
		group := groups[0]
		children, ok := check[group].([]interface{})
		if !ok || len(children) == 0 {
			return fmt.Errorf("%s.%s: expected a nonempty array", path, group)
		}
		return validateCheckList(children, ownerVariable, path+"."+group)
	}
	if _, hasSegment := check["segment"]; hasSegment {
		return validateSegmentCheckShape(check, path)
	}
	if _, hasMatches := check["matches"]; hasMatches {
		return validateSegmentCheckShape(check, path)
	}
	if _, hasEnabled := check["enabled"]; hasEnabled {
		return validateFeatureCheckShape(check, path)
	}
	if _, hasVariation := check["variation"]; hasVariation {
		return validateFeatureCheckShape(check, path)
	}
	if _, hasFeature := check["feature"]; hasFeature {
		if _, hasVariable := check["variable"]; !hasVariable {
			if _, hasOperator := check["operator"]; !hasOperator {
				return fmt.Errorf("%s: feature check requires enabled or variation", path)
			}
		}
	}
	allowed := map[string]bool{"variable": true, "feature": true, "path": true, "operator": true, "value": true, "compareTo": true, "regexFlags": true, "key": true, "description": true}
	for key := range check {
		if !allowed[key] {
			return fmt.Errorf("%s: unknown property %q", path, key)
		}
	}
	operator, ok := check["operator"].(string)
	if !ok || !knownCheckOperator(operator) {
		return fmt.Errorf("%s: unknown operator %q", path, operator)
	}
	if err := validateCheckReferenceShape(check, false, ownerVariable); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	hasValue := false
	if _, exists := check["value"]; exists {
		hasValue = true
	}
	compareTo, hasCompareTo := check["compareTo"]
	valueLess := operator == "exists" || operator == "notExists" || operator == "isNull" || operator == "notNull" || operator == "empty" || operator == "notEmpty"
	if valueLess && (hasValue || hasCompareTo) {
		return fmt.Errorf("%s: %s accepts neither value nor compareTo", path, operator)
	}
	if !valueLess && hasValue == hasCompareTo {
		return fmt.Errorf("%s: exactly one of value or compareTo is required", path)
	}
	if hasCompareTo {
		reference, ok := compareTo.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%s.compareTo: reference must be an object", path)
		}
		if err := validateCheckReferenceShape(reference, true, ownerVariable); err != nil {
			return fmt.Errorf("%s.compareTo: %w", path, err)
		}
	} else if hasValue {
		if err := validateCheckLiteral(operator, check["value"], check["regexFlags"]); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	flags, hasFlags := check["regexFlags"]
	if hasFlags {
		if operator != "matches" && operator != "notMatches" {
			return fmt.Errorf("%s: regexFlags are only allowed with regex operators", path)
		}
		value, ok := flags.(string)
		if !ok {
			return fmt.Errorf("%s: regexFlags must be a string", path)
		}
		seen := map[rune]bool{}
		for _, flag := range value {
			if !strings.ContainsRune("gims", flag) || seen[flag] {
				return fmt.Errorf("%s: invalid or duplicate regex flag %q", path, flag)
			}
			seen[flag] = true
		}
	}
	return nil
}

func validateSegmentCheckShape(check map[string]interface{}, path string) error {
	allowed := map[string]bool{"segment": true, "matches": true, "key": true, "description": true}
	for key := range check {
		if !allowed[key] {
			return fmt.Errorf("%s: segment check has unexpected property %q", path, key)
		}
	}
	segment, ok := check["segment"].(string)
	if !ok || strings.TrimSpace(segment) == "" {
		return fmt.Errorf("%s: segment must be a nonempty string", path)
	}
	if _, ok := check["matches"].(bool); !ok {
		return fmt.Errorf("%s: segment %q matches must be a boolean", path, segment)
	}
	return nil
}

func validateCheckSegmentReferences(checks []interface{}, evaluator checkVariableEvaluator, path string) error {
	segmentValidator, _ := evaluator.(checkSegmentValidator)
	var visit func(interface{}, string) error
	visit = func(raw interface{}, currentPath string) error {
		check, _ := raw.(map[string]interface{})
		for _, group := range []string{"and", "or", "not"} {
			if children, ok := check[group].([]interface{}); ok {
				for index, child := range children {
					if err := visit(child, fmt.Sprintf("%s.%s[%d]", currentPath, group, index)); err != nil {
						return err
					}
				}
				return nil
			}
		}
		segment, ok := check["segment"].(string)
		if !ok {
			return nil
		}
		if segmentValidator == nil {
			return fmt.Errorf("%s: segment %q definition is missing", currentPath, segment)
		}
		if err := segmentValidator.ValidateSegment(segment); err != nil {
			return fmt.Errorf("%s: %w", currentPath, err)
		}
		return nil
	}
	for index, check := range checks {
		if err := visit(check, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func validateFeatureCheckShape(check map[string]interface{}, path string) error {
	allowed := map[string]bool{"feature": true, "enabled": true, "variation": true, "key": true, "description": true}
	for key := range check {
		if !allowed[key] {
			return fmt.Errorf("%s: feature check has unexpected property %q", path, key)
		}
	}
	feature, ok := check["feature"].(string)
	if !ok || feature == "" {
		return fmt.Errorf("%s: feature must be a nonempty string", path)
	}
	_, hasEnabled := check["enabled"]
	_, hasVariation := check["variation"]
	if !hasEnabled && !hasVariation {
		return fmt.Errorf("%s: feature check requires enabled or variation", path)
	}
	if hasEnabled {
		if _, ok := check["enabled"].(bool); !ok {
			return fmt.Errorf("%s: enabled must be a boolean", path)
		}
	}
	if hasVariation && check["variation"] != nil {
		if _, ok := check["variation"].(string); !ok {
			return fmt.Errorf("%s: variation must be a string or null", path)
		}
	}
	return nil
}

func validateCheckReferenceShape(reference map[string]interface{}, nested bool, ownerVariable string) error {
	allowed := map[string]bool{"variable": true, "feature": true, "path": true}
	if !nested {
		allowed["operator"] = true
		allowed["value"] = true
		allowed["compareTo"] = true
		allowed["regexFlags"] = true
		allowed["key"] = true
		allowed["description"] = true
	}
	for key := range reference {
		if !allowed[key] {
			return fmt.Errorf("unknown reference property %q", key)
		}
	}
	if feature, exists := reference["feature"]; exists {
		if featureKey, ok := feature.(string); !ok || featureKey == "" {
			return fmt.Errorf("feature must be a nonempty string")
		}
		if variable, ok := reference["variable"].(string); !ok || variable == "" {
			return fmt.Errorf("variable is required for a feature reference")
		}
	}
	if variable, exists := reference["variable"]; exists {
		if value, ok := variable.(string); !ok || value == "" {
			return fmt.Errorf("variable must be a nonempty string")
		}
	}
	if _, hasVariable := reference["variable"]; !hasVariable && (ownerVariable == "" || reference["feature"] != nil) {
		return fmt.Errorf("an explicit variable is required in a scenario or feature reference")
	}
	if path, exists := reference["path"]; exists {
		if path == nil {
			return fmt.Errorf("path must be a string or array")
		}
		if _, err := parseCheckPath(path); err != nil {
			return err
		}
	}
	return nil
}

func validateCheckLiteral(operator string, value interface{}, rawFlags interface{}) error {
	switch operator {
	case "greaterThan", "greaterThanOrEquals", "lessThan", "lessThanOrEquals":
		if number, ok := checkNumber(value); !ok || math.IsInf(number, 0) || math.IsNaN(number) {
			return fmt.Errorf("numeric comparison requires a finite number")
		}
	case "contains", "notContains", "startsWith", "endsWith", "semverEquals", "semverNotEquals", "semverGreaterThan", "semverGreaterThanOrEquals", "semverLessThan", "semverLessThanOrEquals":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("string comparison requires a string")
		}
	case "matches", "notMatches":
		pattern, ok := value.(string)
		if !ok {
			return fmt.Errorf("string comparison requires a string")
		}
		flags, _ := rawFlags.(string)
		if _, err := compilePortableCheckRegex(pattern, flags); err != nil {
			return err
		}
	case "before", "after":
		date, ok := value.(string)
		if !ok {
			return fmt.Errorf("date comparison requires a timestamp string with a timezone")
		}
		if _, err := parsePortableCheckDate(date); err != nil {
			return fmt.Errorf("date comparison requires a timestamp with a timezone")
		}
	case "in", "notIn", "includesAny", "notIncludesAny":
		values, ok := value.([]interface{})
		if !ok || values == nil {
			return fmt.Errorf("membership comparison requires an array of primitive values")
		}
		for _, item := range values {
			if !isPrimitive(item) {
				return fmt.Errorf("membership comparison requires an array of primitive values")
			}
		}
	default:
		if !isPrimitive(value) {
			return fmt.Errorf("comparison requires a primitive value")
		}
	}
	return nil
}

func compilePortableCheckRegex(pattern, flags string) (*regexp.Regexp, error) {
	if strings.Contains(pattern, "(?") || regexp.MustCompile(`\\(?:[1-9]|k<|k'|g<|g')`).MatchString(pattern) || regexp.MustCompile(`(?:[?*+]|\{\d+(?:,\d*)?\})\+`).MatchString(pattern) {
		return nil, fmt.Errorf("unsupported portable regular expression")
	}
	prefix := ""
	for _, flag := range flags {
		if flag == 'i' || flag == 'm' || flag == 's' {
			prefix += string(flag)
		}
	}
	if prefix != "" {
		pattern = "(?" + prefix + ")" + pattern
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %w", err)
	}
	return compiled, nil
}

func knownCheckOperator(operator string) bool {
	switch operator {
	case "equals", "notEquals", "before", "after", "in", "notIn", "contains", "notContains", "startsWith", "endsWith", "greaterThan", "greaterThanOrEquals", "lessThan", "lessThanOrEquals", "semverEquals", "semverNotEquals", "semverGreaterThan", "semverGreaterThanOrEquals", "semverLessThan", "semverLessThanOrEquals", "matches", "notMatches", "exists", "notExists", "includes", "notIncludes", "includesAny", "notIncludesAny", "isNull", "notNull", "empty", "notEmpty":
		return true
	default:
		return false
	}
}

func evaluateCheck(raw interface{}, ownerVariable string, evaluator checkVariableEvaluator, datafile featurevisor.DatafileContent, checkPath string, tracePath []interface{}, evaluated *[]map[string]interface{}) (bool, string, error) {
	check, ok := raw.(map[string]interface{})
	if !ok {
		return false, "", fmt.Errorf("%s: check must be an object", checkPath)
	}
	displayPath := formatNamedCheckPath(checkPath, check)
	groups := []string{}
	for _, key := range []string{"and", "or", "not"} {
		if _, exists := check[key]; exists {
			groups = append(groups, key)
		}
	}
	if len(groups) > 0 {
		allowedCount := 1
		if _, exists := check["key"]; exists {
			allowedCount++
		}
		if _, exists := check["description"]; exists {
			allowedCount++
		}
		if len(groups) != 1 || len(check) != allowedCount {
			return false, "", fmt.Errorf("%s: group must contain exactly one of and, or, or not", checkPath)
		}
		operator := groups[0]
		children, ok := check[operator].([]interface{})
		if !ok || len(children) == 0 {
			return false, "", fmt.Errorf("%s.%s: expected a nonempty array", checkPath, operator)
		}
		switch operator {
		case "and":
			for index, child := range children {
				matched, failure, err := evaluateCheck(child, ownerVariable, evaluator, datafile, fmt.Sprintf("%s.and[%d]", checkPath, index), appendPath(tracePath, "and", index), evaluated)
				if err != nil || !matched {
					if err == nil && displayPath != checkPath {
						failure = fmt.Sprintf("%s: %s", displayPath, failure)
					}
					return matched, failure, err
				}
			}
			return true, "", nil
		case "or":
			failures := []string{}
			for index, child := range children {
				matched, failure, err := evaluateCheck(child, ownerVariable, evaluator, datafile, fmt.Sprintf("%s.or[%d]", checkPath, index), appendPath(tracePath, "or", index), evaluated)
				if err != nil {
					return false, "", err
				}
				if matched {
					return true, "", nil
				}
				failures = append(failures, failure)
			}
			return false, fmt.Sprintf("%s: no OR branch matched (%s)", displayPath, strings.Join(failures, "; ")), nil
		case "not":
			for index, child := range children {
				matched, _, err := evaluateCheck(child, ownerVariable, evaluator, datafile, fmt.Sprintf("%s.not[%d]", checkPath, index), appendPath(tracePath, "not", index), evaluated)
				if err != nil {
					return false, "", err
				}
				if !matched {
					return true, "", nil
				}
			}
			return false, fmt.Sprintf("%s: NOT group matched", displayPath), nil
		}
	}
	if _, hasSegment := check["segment"]; hasSegment {
		return evaluateSegmentCheck(check, evaluator, displayPath, tracePath, evaluated)
	}
	if _, hasMatches := check["matches"]; hasMatches {
		return evaluateSegmentCheck(check, evaluator, displayPath, tracePath, evaluated)
	}
	if _, hasEnabled := check["enabled"]; hasEnabled {
		return evaluateFeatureCheck(check, evaluator, datafile, displayPath, tracePath, evaluated)
	}
	if _, hasVariation := check["variation"]; hasVariation {
		return evaluateFeatureCheck(check, evaluator, datafile, displayPath, tracePath, evaluated)
	}
	if _, hasFeature := check["feature"]; hasFeature {
		if _, hasVariable := check["variable"]; !hasVariable {
			return false, "", fmt.Errorf("%s: feature check requires enabled or variation", checkPath)
		}
	}

	operator, ok := check["operator"].(string)
	if !ok || operator == "" {
		return false, "", fmt.Errorf("%s: leaf is missing operator", checkPath)
	}
	left, leftReference, err := resolveCheckReference(check, ownerVariable, evaluator, datafile)
	if err != nil {
		return false, "", fmt.Errorf("%s left %s: %w", displayPath, formatCheckReference(check, ownerVariable), err)
	}
	value, hasValue := check["value"]
	compareRaw, hasCompareTo := check["compareTo"]
	valueLess := operator == "exists" || operator == "notExists" || operator == "isNull" || operator == "notNull" || operator == "empty" || operator == "notEmpty"
	if valueLess {
		if hasValue || hasCompareTo {
			return false, "", fmt.Errorf("%s: %s accepts neither value nor compareTo", checkPath, operator)
		}
	} else if hasValue == hasCompareTo {
		return false, "", fmt.Errorf("%s: exactly one of value or compareTo is required", checkPath)
	}
	rightReference := "literal"
	if hasCompareTo {
		compareTo, ok := compareRaw.(map[string]interface{})
		if !ok {
			return false, "", fmt.Errorf("%s.compareTo: reference must be an object", checkPath)
		}
		value, rightReference, err = resolveCheckReference(compareTo, ownerVariable, evaluator, datafile)
		if err != nil {
			return false, "", fmt.Errorf("%s right %s: %w", displayPath, formatCheckReference(compareTo, ownerVariable), err)
		}
	}
	traceExpected := value
	if !hasValue && !hasCompareTo {
		traceExpected = missingValue
	}
	trace := map[string]interface{}{
		"path":      tracePath,
		"reference": normalizedCheckReference(check, ownerVariable),
		"operator":  operator,
		"expected":  checkReportValue(traceExpected),
		"actual":    checkReportValue(left),
	}
	if key, ok := check["key"].(string); ok {
		trace["key"] = key
	}
	if description, ok := check["description"].(string); ok {
		trace["description"] = description
	}
	if hasCompareTo {
		trace["compareTo"] = normalizedCheckReference(compareRaw.(map[string]interface{}), ownerVariable)
	}
	if _, missing := left.(missingCheckValue); missing {
		trace["actualMissing"] = true
	}
	if _, missing := value.(missingCheckValue); missing {
		trace["expectedMissing"] = true
	}
	*evaluated = append(*evaluated, trace)
	matched, err := matchCheckValues(left, operator, value, check["regexFlags"])
	if err != nil {
		return false, "", fmt.Errorf("%s: %s against %s failed: %w (left=%s, right=%s)", displayPath, leftReference, rightReference, err, formatCheckValue(left), formatCheckValue(value))
	}
	if matched {
		return true, "", nil
	}
	return false, fmt.Sprintf("%s: %s %s %s did not match (left=%s, right=%s)", displayPath, leftReference, operator, rightReference, formatCheckValue(left), formatCheckValue(value)), nil
}

func evaluateSegmentCheck(check map[string]interface{}, evaluator checkVariableEvaluator, checkPath string, tracePath []interface{}, evaluated *[]map[string]interface{}) (bool, string, error) {
	segment := check["segment"].(string)
	expected := check["matches"].(bool)
	segmentEvaluator, ok := evaluator.(checkSegmentEvaluator)
	if !ok {
		return false, "", fmt.Errorf("%s: segment %q definition is missing", checkPath, segment)
	}
	actual, err := segmentEvaluator.ResolveSegment(segment)
	if err != nil {
		return false, "", fmt.Errorf("%s: %w", checkPath, err)
	}
	trace := map[string]interface{}{
		"path": tracePath, "reference": map[string]interface{}{"segment": segment},
		"property": "matches", "operator": "equals", "expected": expected, "actual": actual,
	}
	appendCheckMetadata(trace, check)
	*evaluated = append(*evaluated, trace)
	if actual == expected {
		return true, "", nil
	}
	return false, fmt.Sprintf("%s: segment %q expected matches=%t, actual matches=%t", checkPath, segment, expected, actual), nil
}

func normalizedCheckReference(reference map[string]interface{}, owner string) map[string]interface{} {
	result := map[string]interface{}{}
	if feature, ok := reference["feature"]; ok {
		result["feature"] = feature
	}
	if variable, ok := reference["variable"]; ok {
		result["variable"] = variable
	} else if owner != "" {
		result["variable"] = owner
	}
	if path, ok := reference["path"]; ok {
		result["path"] = path
	}
	return result
}

func checkReportValue(value interface{}) interface{} {
	if _, missing := value.(missingCheckValue); missing {
		return map[string]interface{}{"$type": "undefined"}
	}
	return value
}

func formatNamedCheckPath(path string, check map[string]interface{}) string {
	key, _ := check["key"].(string)
	description, _ := check["description"].(string)
	if key == "" && description == "" {
		return path
	}
	metadata := []string{}
	if key != "" {
		metadata = append(metadata, fmt.Sprintf("key=%q", key))
	}
	if description != "" {
		metadata = append(metadata, fmt.Sprintf("description=%q", description))
	}
	return fmt.Sprintf("%s [%s]", path, strings.Join(metadata, ", "))
}

func evaluateFeatureCheck(check map[string]interface{}, evaluator checkVariableEvaluator, datafile featurevisor.DatafileContent, checkPath string, tracePath []interface{}, evaluated *[]map[string]interface{}) (bool, string, error) {
	feature, ok := check["feature"].(string)
	if !ok || feature == "" {
		return false, "", fmt.Errorf("%s: feature must be a nonempty string", checkPath)
	}
	if _, exists := datafile.Features[feature]; !exists {
		return false, "", fmt.Errorf("%s: feature %q is absent from the selected datafile", checkPath, feature)
	}
	parts := []string{}
	matched := true
	if expected, supplied := check["enabled"]; supplied {
		actual, err := evaluator.ResolveFeatureEnabled(feature)
		if err != nil {
			return false, "", fmt.Errorf("%s: feature %q enabled getter failed: %w", checkPath, feature, err)
		}
		parts = append(parts, fmt.Sprintf("expected enabled=%t, actual enabled=%t", expected.(bool), actual))
		trace := map[string]interface{}{"path": tracePath, "reference": map[string]interface{}{"feature": feature}, "property": "enabled", "operator": "equals", "expected": expected, "actual": actual}
		appendCheckMetadata(trace, check)
		*evaluated = append(*evaluated, trace)
		if actual != expected.(bool) {
			matched = false
		}
	}
	if expected, supplied := check["variation"]; supplied {
		actual, err := evaluator.ResolveFeatureVariation(feature)
		if err != nil {
			return false, "", fmt.Errorf("%s: feature %q variation getter failed: %w", checkPath, feature, err)
		}
		expectedVariation, _ := expected.(string)
		variationMatched := (expected == nil && actual == nil) || (expected != nil && actual != nil && *actual == expectedVariation)
		parts = append(parts, fmt.Sprintf("expected variation=%s, actual variation=%s", formatCheckValue(expected), formatStringPointer(actual)))
		var actualValue interface{}
		if actual != nil {
			actualValue = *actual
		}
		trace := map[string]interface{}{"path": tracePath, "reference": map[string]interface{}{"feature": feature}, "property": "variation", "operator": "equals", "expected": expected, "actual": actualValue}
		appendCheckMetadata(trace, check)
		*evaluated = append(*evaluated, trace)
		if !variationMatched {
			matched = false
		}
	}
	if matched {
		return true, "", nil
	}
	return false, fmt.Sprintf("%s: feature %q %s", checkPath, feature, strings.Join(parts, "; ")), nil
}

func appendCheckMetadata(trace map[string]interface{}, check map[string]interface{}) {
	if key, ok := check["key"].(string); ok {
		trace["key"] = key
	}
	if description, ok := check["description"].(string); ok {
		trace["description"] = description
	}
}

func formatStringPointer(value *string) string {
	if value == nil {
		return "null"
	}
	return formatCheckValue(*value)
}

func resolveCheckReference(reference map[string]interface{}, ownerVariable string, evaluator checkVariableEvaluator, datafile featurevisor.DatafileContent) (interface{}, string, error) {
	variable, _ := reference["variable"].(string)
	featureKey, hasFeature := reference["feature"].(string)
	if variable == "" {
		if hasFeature || ownerVariable == "" {
			return nil, "", fmt.Errorf("variable is required")
		}
		variable = ownerVariable
	}
	var value interface{}
	var err error
	if hasFeature {
		feature, exists := datafile.Features[featureKey]
		if !exists {
			return nil, "", fmt.Errorf("feature %q is absent from the selected datafile", featureKey)
		}
		if _, exists := feature.VariablesSchema[variable]; !exists {
			return nil, "", fmt.Errorf("feature variable %q.%q is absent from the selected datafile", featureKey, variable)
		}
		value, err = evaluator.ResolveFeatureVariable(featureKey, variable)
	} else {
		if _, exists := datafile.Variables[variable]; !exists {
			return nil, "", fmt.Errorf("global variable %q is absent from the selected datafile", variable)
		}
		value, err = evaluator.ResolveGlobalVariable(variable)
	}
	if err != nil {
		return nil, "", err
	}
	path, err := parseCheckPath(reference["path"])
	if err != nil {
		return nil, "", err
	}
	value = normalizeCheckValue(value)
	for _, part := range path {
		switch component := part.(type) {
		case string:
			if object, ok := value.(map[string]interface{}); ok {
				child, exists := object[component]
				if !exists {
					return missingValue, formatCheckReference(reference, ownerVariable), nil
				}
				value = child
				continue
			}
			if array, ok := value.([]interface{}); ok {
				if component == "length" {
					value = float64(len(array))
					continue
				}
				if index, ok := canonicalCheckIndex(component); ok && index < len(array) {
					value = array[index]
					continue
				}
			}
			return missingValue, formatCheckReference(reference, ownerVariable), nil
		case int:
			array, ok := value.([]interface{})
			if !ok || component >= len(array) {
				return missingValue, formatCheckReference(reference, ownerVariable), nil
			}
			value = array[component]
		}
	}
	return value, formatCheckReference(reference, ownerVariable), nil
}

func parseCheckPath(raw interface{}) ([]interface{}, error) {
	if raw == nil {
		return nil, nil
	}
	unsafe := func(value string) bool { return value == "__proto__" || value == "prototype" || value == "constructor" }
	if value, ok := raw.(string); ok {
		if value == "" {
			return nil, fmt.Errorf("path must not be empty")
		}
		parts := strings.Split(value, ".")
		result := make([]interface{}, 0, len(parts))
		for _, part := range parts {
			if part == "" || strings.Contains(part, "*") || unsafe(part) {
				return nil, fmt.Errorf("path contains invalid component %q", part)
			}
			if canonicalCheckIndexSyntax(part) {
				index, err := strconv.ParseUint(part, 10, 64)
				if err != nil || index > 9007199254740991 {
					return nil, fmt.Errorf("path index must be a safe integer")
				}
				result = append(result, int(index))
			} else {
				result = append(result, part)
			}
		}
		return result, nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("path must be a string or array")
	}
	result := make([]interface{}, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case string:
			if strings.Contains(value, "*") || unsafe(value) {
				return nil, fmt.Errorf("path contains invalid component %q", value)
			}
			result = append(result, value)
		case float64:
			if value < 0 || math.Trunc(value) != value || value > 9007199254740991 {
				return nil, fmt.Errorf("path contains invalid array index %v", value)
			}
			result = append(result, int(value))
		default:
			return nil, fmt.Errorf("path contains unsupported component %s", formatCheckValue(value))
		}
	}
	return result, nil
}

func matchCheckValues(left interface{}, operator string, right interface{}, rawFlags interface{}) (bool, error) {
	_, leftMissing := left.(missingCheckValue)
	if operator == "exists" {
		return !leftMissing, nil
	}
	if operator == "notExists" {
		return leftMissing, nil
	}
	if operator == "isNull" {
		return !leftMissing && left == nil, nil
	}
	if operator == "notNull" {
		return !leftMissing && left != nil, nil
	}
	if operator == "empty" || operator == "notEmpty" {
		if leftMissing || left == nil {
			return false, nil
		}
		isEmpty := false
		switch value := left.(type) {
		case string:
			isEmpty = len(value) == 0
		case []interface{}:
			isEmpty = len(value) == 0
		case map[string]interface{}:
			isEmpty = len(value) == 0
		default:
			return false, fmt.Errorf("%s requires a string, array, or object operand", operator)
		}
		if operator == "empty" {
			return isEmpty, nil
		}
		return !isEmpty, nil
	}
	if leftMissing {
		return false, nil
	}
	if _, rightMissing := right.(missingCheckValue); rightMissing {
		return false, nil
	}
	switch operator {
	case "equals":
		if !isPrimitive(left) || !isPrimitive(right) {
			return false, fmt.Errorf("equals requires scalar operands")
		}
		return scalarCheckEqual(left, right), nil
	case "notEquals":
		if !isPrimitive(left) || !isPrimitive(right) {
			return false, fmt.Errorf("notEquals requires scalar operands")
		}
		return !scalarCheckEqual(left, right), nil
	case "in", "notIn":
		if !isStringNumberOrNull(left) {
			return false, fmt.Errorf("%s requires a string, number, or null left operand", operator)
		}
		values, ok := right.([]interface{})
		if !ok || values == nil {
			return false, fmt.Errorf("%s requires an array right operand", operator)
		}
		found := false
		for _, item := range values {
			if scalarCheckEqual(left, item) {
				found = true
				break
			}
		}
		if operator == "in" {
			return found, nil
		}
		return !found, nil
	case "includesAny", "notIncludesAny":
		values, ok := left.([]interface{})
		if !ok || values == nil {
			return false, fmt.Errorf("%s requires an array left operand", operator)
		}
		candidates, ok := right.([]interface{})
		if !ok || candidates == nil {
			return false, fmt.Errorf("%s requires an array right operand", operator)
		}
		for _, candidate := range candidates {
			if !isPrimitive(candidate) {
				return false, fmt.Errorf("%s requires an array of primitive values", operator)
			}
		}
		for _, candidate := range candidates {
			for _, item := range values {
				if scalarCheckEqual(item, candidate) {
					return operator == "includesAny", nil
				}
			}
		}
		return operator == "notIncludesAny", nil
	case "includes", "notIncludes":
		values, ok := left.([]interface{})
		if !ok || values == nil {
			return false, fmt.Errorf("%s requires an array left operand", operator)
		}
		if !isPrimitive(right) {
			return false, fmt.Errorf("%s requires a primitive right operand", operator)
		}
		found := false
		for _, item := range values {
			if scalarCheckEqual(item, right) {
				found = true
				break
			}
		}
		if operator == "includes" {
			return found, nil
		}
		return !found, nil
	case "contains", "notContains", "startsWith", "endsWith", "matches", "notMatches", "semverEquals", "semverNotEquals", "semverGreaterThan", "semverGreaterThanOrEquals", "semverLessThan", "semverLessThanOrEquals":
		l, lok := left.(string)
		r, rok := right.(string)
		if !lok || !rok {
			return false, fmt.Errorf("%s requires string operands", operator)
		}
		switch operator {
		case "contains":
			return strings.Contains(l, r), nil
		case "notContains":
			return !strings.Contains(l, r), nil
		case "startsWith":
			return strings.HasPrefix(l, r), nil
		case "endsWith":
			return strings.HasSuffix(l, r), nil
		case "matches", "notMatches":
			flags, _ := rawFlags.(string)
			re, err := compilePortableCheckRegex(r, flags)
			if err != nil {
				return false, fmt.Errorf("invalid regular expression: %w", err)
			}
			matched := re.MatchString(l)
			if operator == "matches" {
				return matched, nil
			}
			return !matched, nil
		default:
			comparison, err := compareCheckVersions(l, r)
			if err != nil {
				return false, err
			}
			switch operator {
			case "semverEquals":
				return comparison == 0, nil
			case "semverNotEquals":
				return comparison != 0, nil
			case "semverGreaterThan":
				return comparison > 0, nil
			case "semverGreaterThanOrEquals":
				return comparison >= 0, nil
			case "semverLessThan":
				return comparison < 0, nil
			default:
				return comparison <= 0, nil
			}
		}
	case "greaterThan", "greaterThanOrEquals", "lessThan", "lessThanOrEquals":
		l, lok := checkNumber(left)
		r, rok := checkNumber(right)
		if !lok || !rok {
			return false, fmt.Errorf("%s requires numeric operands", operator)
		}
		switch operator {
		case "greaterThan":
			return l > r, nil
		case "greaterThanOrEquals":
			return l >= r, nil
		case "lessThan":
			return l < r, nil
		default:
			return l <= r, nil
		}
	case "before", "after":
		l, lok := left.(string)
		r, rok := right.(string)
		if !lok || !rok {
			return false, fmt.Errorf("%s requires RFC 3339 string operands", operator)
		}
		ld, lerr := parsePortableCheckDate(l)
		rd, rerr := parsePortableCheckDate(r)
		if lerr != nil || rerr != nil {
			return false, fmt.Errorf("%s requires valid RFC 3339 dates", operator)
		}
		if operator == "before" {
			return ld.Before(rd), nil
		}
		return ld.After(rd), nil
	default:
		return false, fmt.Errorf("unknown operator %q", operator)
	}
}

var portableCheckDatePattern = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,9})?(Z|([+-])(\d{2}):(\d{2}))$`)

func parsePortableCheckDate(value string) (time.Time, error) {
	parts := portableCheckDatePattern.FindStringSubmatch(value)
	if parts == nil {
		return time.Time{}, fmt.Errorf("invalid portable timestamp")
	}
	year, _ := strconv.Atoi(parts[1])
	if year < 1 || year > 9999 {
		return time.Time{}, fmt.Errorf("year is out of range")
	}
	if parts[7] != "Z" {
		offsetHours, _ := strconv.Atoi(parts[9])
		offsetMinutes, _ := strconv.Atoi(parts[10])
		if offsetHours > 14 || offsetMinutes > 59 || (offsetHours == 14 && offsetMinutes != 0) {
			return time.Time{}, fmt.Errorf("timezone offset is out of range")
		}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	utcYear := parsed.UTC().Year()
	if utcYear < 1 || utcYear > 9999 {
		return time.Time{}, fmt.Errorf("UTC instant is out of range")
	}
	return parsed.Truncate(time.Millisecond), nil
}

func normalizeCheckValue(value interface{}) interface{} {
	raw, _ := json.Marshal(value)
	var result interface{}
	if json.Unmarshal(raw, &result) == nil {
		return result
	}
	return value
}
func canonicalCheckIndex(value string) (int, bool) {
	if !canonicalCheckIndexSyntax(value) {
		return 0, false
	}
	index, err := strconv.ParseUint(value, 10, 64)
	return int(index), err == nil && index <= 9007199254740991
}

func canonicalCheckIndexSyntax(value string) bool {
	if value != "0" && (value == "" || value[0] == '0') {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return value != ""
}
func checkNumber(value interface{}) (float64, bool) { number, ok := value.(float64); return number, ok }
func isPrimitive(value interface{}) bool {
	if value == nil {
		return true
	}
	switch value.(type) {
	case string, float64, bool:
		return true
	}
	return false
}
func isStringNumberOrNull(value interface{}) bool {
	if value == nil {
		return true
	}
	switch value.(type) {
	case string, float64:
		return true
	}
	return false
}
func scalarCheckEqual(left, right interface{}) bool {
	if !isPrimitive(left) || !isPrimitive(right) {
		return false
	}
	return reflectScalarEqual(left, right)
}
func reflectScalarEqual(left, right interface{}) bool {
	return fmt.Sprintf("%T", left) == fmt.Sprintf("%T", right) && valuesEqual(left, right)
}
func formatCheckValue(value interface{}) string {
	if _, ok := value.(missingCheckValue); ok {
		return "<missing>"
	}
	return formatTestValue(value)
}
func formatCheckReference(reference map[string]interface{}, owner string) string {
	variable, _ := reference["variable"].(string)
	if variable == "" {
		variable = owner
	}
	feature, _ := reference["feature"].(string)
	base := variable
	if feature != "" {
		base = feature + "." + variable
	}
	if path, ok := reference["path"]; ok {
		base += "[" + formatCheckValue(path) + "]"
	}
	return base
}

var checkSemverPattern = regexp.MustCompile(`(?i)^[v^~<>=]*?(\d+)(?:\.([x*]|\d+)(?:\.([x*]|\d+)(?:\.([x*]|\d+))?(?:-([\da-z\-]+(?:\.[\da-z\-]+)*))?(?:\+[\da-z\-]+(?:\.[\da-z\-]+)*)?)?)?$`)

func compareCheckVersions(left, right string) (int, error) {
	parse := func(value string) ([]string, string, error) {
		for _, c := range value {
			if c > 127 {
				return nil, "", fmt.Errorf("invalid semver %q", value)
			}
		}
		match := checkSemverPattern.FindStringSubmatch(value)
		if match == nil {
			return nil, "", fmt.Errorf("invalid semver %q", value)
		}
		return match[1:5], match[5], nil
	}
	lc, lp, err := parse(left)
	if err != nil {
		return 0, err
	}
	rc, rp, err := parse(right)
	if err != nil {
		return 0, err
	}
	comparePart := func(a, b string, prerelease bool) int {
		if a == "" {
			a = "0"
		}
		if b == "" {
			b = "0"
		}
		if !prerelease && (a == "*" || b == "*" || strings.EqualFold(a, "x") || strings.EqualFold(b, "x")) {
			return 0
		}
		numeric := func(value string) bool {
			for _, c := range value {
				if c < '0' || c > '9' {
					return false
				}
			}
			return value != ""
		}
		an, bn := numeric(a), numeric(b)
		if an && bn {
			a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
			if len(a) < len(b) {
				return -1
			}
			if len(a) > len(b) {
				return 1
			}
		} else if prerelease && an != bn {
			if an {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	}
	for index := range lc {
		if compared := comparePart(lc[index], rc[index], false); compared != 0 {
			return compared, nil
		}
	}
	if lp == "" && rp != "" {
		return 1, nil
	}
	if lp != "" && rp == "" {
		return -1, nil
	}
	la, ra := strings.Split(lp, "."), strings.Split(rp, ".")
	count := len(la)
	if len(ra) > count {
		count = len(ra)
	}
	for index := 0; index < count; index++ {
		if index == len(la) {
			return -1, nil
		}
		if index == len(ra) {
			return 1, nil
		}
		a, b := "0", "0"
		if index < len(la) {
			a = la[index]
		}
		if index < len(ra) {
			b = ra[index]
		}
		if compared := comparePart(a, b, true); compared != 0 {
			return compared, nil
		}
	}
	return 0, nil
}
