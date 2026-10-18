package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"

	"github.com/featurevisor/featurevisor-go/v4"
)

func valuesEqual(left, right interface{}) bool {
	normalize := func(value interface{}) interface{} {
		raw, _ := json.Marshal(value)
		var result interface{}
		_ = json.Unmarshal(raw, &result)
		return result
	}
	return reflect.DeepEqual(normalize(left), normalize(right))
}

func formatTestValue(value interface{}) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(raw)
}

type globalVariableEvaluator interface {
	GetGlobalVariable(variableKey string, args ...interface{}) featurevisor.VariableValue
	EvaluateGlobalVariable(variableKey string, args ...interface{}) featurevisor.Evaluation
}

func evaluateVariableAssertion(assertion map[string]interface{}, variableKey string, evaluator globalVariableEvaluator, checkEvaluator checkVariableEvaluator, datafile featurevisor.DatafileContent, prefix string, basePath []interface{}) (string, []reportError) {
	options := featurevisor.OverrideOptions{}
	if value, ok := assertion["defaultVariableValue"]; ok {
		options.DefaultVariableValue = value
		options.DefaultVariableValueSet = true
	}
	evaluation := evaluator.EvaluateGlobalVariable(variableKey, featurevisor.Context{}, options)
	errors := ""
	reportErrors := []reportError{}
	if expected, ok := assertion["expectedValue"]; ok {
		actualValue := evaluator.GetGlobalVariable(variableKey, featurevisor.Context{}, options)
		if !valuesEqual(actualValue, expected) {
			message := fmt.Sprintf("%sexpectedValue: expected %s but received %s", prefix, formatTestValue(expected), formatTestValue(actualValue))
			errors += "      ✘ " + message + "\n"
			reportErrors = append(reportErrors, reportError{Message: message, Type: "variable", Path: appendPath(basePath, "expectedValue"), Expected: reportValuePointer(expected), Actual: reportValuePointer(actualValue)})
		}
	}
	if expectedFields, ok := assertion["expectedEvaluation"].(map[string]interface{}); ok {
		for key, expected := range expectedFields {
			actual := getEvaluationValue(evaluation, key)
			if !valuesEqual(actual, expected) {
				message := fmt.Sprintf("%sexpectedEvaluation.%s: expected %s but received %s", prefix, key, formatTestValue(expected), formatTestValue(actual))
				errors += "      ✘ " + message + "\n"
				reportErrors = append(reportErrors, reportError{Message: message, Type: "evaluation", Path: appendPath(basePath, "expectedEvaluation", key), Expected: reportValuePointer(expected), Actual: reportValuePointer(actual)})
			}
		}
	}
	checkOutcome := evaluateAssertionChecksAt(assertion, variableKey, checkEvaluator, datafile, prefix, basePath)
	for _, checkError := range checkOutcome.Failures {
		errors += fmt.Sprintf("      ✘ %s\n", checkError)
	}
	reportErrors = append(reportErrors, checkOutcome.ReportErrors...)
	return errors, reportErrors
}

func preflightAssertionTree(assertion map[string]interface{}, ownerVariable string, evaluator checkVariableEvaluator, datafile featurevisor.DatafileContent, scenario bool) AssertionResult {
	failures := []string{}
	reportErrors := []reportError{}
	appendOutcome := func(outcome checkAssertionOutcome) {
		failures = append(failures, outcome.Failures...)
		reportErrors = append(reportErrors, outcome.ReportErrors...)
	}
	children := []interface{}{}
	if rawChildren, exists := assertion["children"]; exists {
		var ok bool
		children, ok = rawChildren.([]interface{})
		if !ok {
			failures = append(failures, "children: expected an array")
		}
	}
	if scenario {
		_, hasChecks := assertion["checks"]
		if !hasChecks && len(children) == 0 {
			failures = append(failures, "scenario assertion needs checks or children")
		}
	}
	appendOutcome(preflightAssertionChecksAt(assertion, ownerVariable, evaluator, datafile, "", nil))
	for index, rawChild := range children {
		child, ok := rawChild.(map[string]interface{})
		if !ok {
			failures = append(failures, fmt.Sprintf("children[%d]: child assertion must be an object", index))
			continue
		}
		if scenario {
			if _, hasChecks := child["checks"]; !hasChecks {
				failures = append(failures, fmt.Sprintf("children[%d].checks: expected a nonempty array", index))
				continue
			}
		}
		appendOutcome(preflightAssertionChecksAt(child, ownerVariable, evaluator, datafile, fmt.Sprintf("children[%d].", index), []interface{}{"children", index}))
	}
	if len(failures) == 0 {
		return AssertionResult{}
	}
	errors := ""
	for _, failure := range failures {
		errors += "      ✘ " + failure + "\n"
	}
	return AssertionResult{HasError: true, Errors: errors, ReportErrors: reportErrors}
}

// RunTestVariable tests a global variable assertion.
func RunTestVariable(assertion map[string]interface{}, variableKey string, instance *featurevisor.Featurevisor, diagnostics *checkDiagnosticCollector, datafile featurevisor.DatafileContent, segmentDefinitions map[string]interface{}, level string) AssertionResult {
	started := time.Now()
	rootContext := contextFromAssertion(assertion)
	segmentGates := segmentGateKeys(datafile, segmentDefinitions)
	rootResolver := nativeCheckResolver{getter: instance, diagnostics: diagnostics, segmentDefinitions: segmentDefinitions, context: rootContext, datafile: datafile, segmentGates: segmentGates}
	if result := preflightAssertionTree(assertion, variableKey, rootResolver, datafile, false); result.HasError {
		result.Duration = time.Since(started).Seconds()
		return result
	}
	installSegmentGates(instance, datafile, segmentDefinitions, segmentGates)
	errors, reportErrors := evaluateVariableAssertion(assertion, variableKey, instance, rootResolver, datafile, "", nil)
	if children, ok := assertion["children"].([]interface{}); ok {
		for index, rawChild := range children {
			childAssertion, ok := rawChild.(map[string]interface{})
			if !ok {
				continue
			}
			childContext := contextFromAssertion(childAssertion)
			childStickyFeatures := stickyFeaturesFromAssertion(childAssertion, "stickyFeatures")
			childStickyVariables := stickyVariablesFromAssertion(childAssertion)
			child := instance.Spawn(childContext, featurevisor.SpawnOptions{
				StickyFeatures:  childStickyFeatures,
				StickyVariables: childStickyVariables,
			})
			func() {
				defer child.Close()
				childErrors, childReportErrors := evaluateVariableAssertion(childAssertion, variableKey, child, nativeCheckResolver{getter: child, diagnostics: diagnostics, segmentDefinitions: segmentDefinitions, datafile: datafile, segmentGates: segmentGates, context: mergeContexts(rootContext, childContext)}, datafile, fmt.Sprintf("children[%d].", index), []interface{}{"children", index})
				errors += childErrors
				reportErrors = append(reportErrors, childReportErrors...)
			}()
		}
	}
	return AssertionResult{HasError: errors != "", Errors: errors, Duration: time.Since(started).Seconds(), ReportErrors: reportErrors}
}

// RunTestScenario evaluates a scenario assertion through the native SDK.
func RunTestScenario(assertion map[string]interface{}, instance *featurevisor.Featurevisor, diagnostics *checkDiagnosticCollector, datafile featurevisor.DatafileContent, segmentDefinitions map[string]interface{}, level string) AssertionResult {
	started := time.Now()
	errors := ""
	reportErrors := []reportError{}
	children, _ := assertion["children"].([]interface{})
	rootContext := contextFromAssertion(assertion)
	segmentGates := segmentGateKeys(datafile, segmentDefinitions)
	rootResolver := nativeCheckResolver{getter: instance, diagnostics: diagnostics, segmentDefinitions: segmentDefinitions, context: rootContext, datafile: datafile, segmentGates: segmentGates}
	if result := preflightAssertionTree(assertion, "", rootResolver, datafile, true); result.HasError {
		result.Duration = time.Since(started).Seconds()
		return result
	}
	installSegmentGates(instance, datafile, segmentDefinitions, segmentGates)
	if _, hasChecks := assertion["checks"]; !hasChecks && len(children) == 0 {
		message := "scenario assertion needs checks or children"
		errors += "      ✘ " + message + "\n"
		reportErrors = append(reportErrors, reportError{Message: message, Type: "expectation", Expected: reportValuePointer(true), Actual: reportValuePointer(false)})
	}
	rootOutcome := evaluateAssertionChecksAt(assertion, "", rootResolver, datafile, "", nil)
	for _, checkError := range rootOutcome.Failures {
		errors += fmt.Sprintf("      ✘ %s\n", checkError)
	}
	reportErrors = append(reportErrors, rootOutcome.ReportErrors...)
	if children != nil {
		for index, rawChild := range children {
			childAssertion, ok := rawChild.(map[string]interface{})
			if !ok {
				errors += fmt.Sprintf("      ✘ children[%d]: child assertion must be an object\n", index)
				continue
			}
			if _, hasChecks := childAssertion["checks"]; !hasChecks {
				message := fmt.Sprintf("children[%d].checks: expected a nonempty array", index)
				errors += "      ✘ " + message + "\n"
				reportErrors = append(reportErrors, reportError{Message: message, Type: "check", Path: []interface{}{"children", index, "checks"}, Expected: reportValuePointer("valid checks"), Actual: reportValuePointer(nil)})
			}
			child := instance.Spawn(contextFromAssertion(childAssertion), featurevisor.SpawnOptions{
				StickyFeatures:  stickyFeaturesFromAssertion(childAssertion, "stickyFeatures"),
				StickyVariables: stickyVariablesFromAssertion(childAssertion),
			})
			func() {
				defer child.Close()
				childOutcome := evaluateAssertionChecksAt(childAssertion, "", nativeCheckResolver{getter: child, diagnostics: diagnostics, segmentDefinitions: segmentDefinitions, datafile: datafile, segmentGates: segmentGates, context: mergeContexts(rootContext, contextFromAssertion(childAssertion))}, datafile, fmt.Sprintf("children[%d].", index), []interface{}{"children", index})
				for _, checkError := range childOutcome.Failures {
					errors += fmt.Sprintf("      ✘ %s\n", checkError)
				}
				reportErrors = append(reportErrors, childOutcome.ReportErrors...)
			}()
		}
	}
	return AssertionResult{HasError: errors != "", Errors: errors, Duration: time.Since(started).Seconds(), ReportErrors: reportErrors}
}

func contextFromAssertion(assertion map[string]interface{}) featurevisor.Context {
	if value, ok := assertion["context"].(map[string]interface{}); ok {
		return featurevisor.Context(value)
	}
	return featurevisor.Context{}
}

func mergeContexts(root, child featurevisor.Context) featurevisor.Context {
	merged := featurevisor.Context{}
	for key, value := range root {
		merged[key] = value
	}
	for key, value := range child {
		merged[key] = value
	}
	return merged
}

func segmentDefinitionsFromTest(test map[string]interface{}) map[string]interface{} {
	definitions, _ := test["_segments"].(map[string]interface{})
	return definitions
}

func stickyVariablesFromAssertion(assertion map[string]interface{}) *featurevisor.StickyVariables {
	raw, ok := assertion["stickyVariables"].(map[string]interface{})
	if !ok {
		return nil
	}
	sticky := featurevisor.StickyVariables{}
	for key, value := range raw {
		sticky[key] = value
	}
	return &sticky
}

func stickyFeaturesFromAssertion(assertion map[string]interface{}, field string) *featurevisor.StickyFeatures {
	raw, ok := assertion[field].(map[string]interface{})
	if !ok {
		return nil
	}
	sticky := featurevisor.StickyFeatures{}
	for key, value := range raw {
		featureValue, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		evaluated := featurevisor.EvaluatedFeature{}
		if enabled, ok := featureValue["enabled"].(bool); ok {
			evaluated.Enabled = enabled
		}
		if variation, ok := featureValue["variation"].(string); ok {
			evaluated.Variation = &variation
		}
		if variables, ok := featureValue["variables"].(map[string]interface{}); ok {
			evaluated.Variables = map[featurevisor.VariableKey]featurevisor.VariableValue{}
			for variableKey, variableValue := range variables {
				evaluated.Variables[featurevisor.VariableKey(variableKey)] = variableValue
			}
		}
		sticky[featurevisor.FeatureKey(key)] = evaluated
	}
	return &sticky
}

// TestFeature tests a feature with the given assertion
func RunTestFeature(assertion map[string]interface{}, featureKey string, instance *featurevisor.Featurevisor, level string) AssertionResult {
	context := featurevisor.Context{}
	if ctx, ok := assertion["context"].(map[string]interface{}); ok {
		context = featurevisor.Context(ctx)
	}

	// Set context and sticky for this assertion
	instance.SetContext(context, false)
	if sticky, ok := assertion["stickyFeatures"].(map[string]interface{}); ok {
		// Convert sticky to proper format
		stickyFeatures := featurevisor.StickyFeatures{}
		for key, value := range sticky {
			if featureSticky, ok := value.(map[string]interface{}); ok {
				evaluatedFeature := featurevisor.EvaluatedFeature{}
				if enabled, ok := featureSticky["enabled"].(bool); ok {
					evaluatedFeature.Enabled = enabled
				}
				if variation, ok := featureSticky["variation"]; ok {
					if variationStr, ok := variation.(string); ok {
						evaluatedFeature.Variation = &variationStr
					}
				}
				if variables, ok := featureSticky["variables"].(map[string]interface{}); ok {
					evaluatedFeature.Variables = make(map[featurevisor.VariableKey]featurevisor.VariableValue)
					for varKey, varValue := range variables {
						evaluatedFeature.Variables[featurevisor.VariableKey(varKey)] = varValue
					}
				}
				stickyFeatures[featurevisor.FeatureKey(key)] = evaluatedFeature
			}
		}
		instance.SetStickyFeatures(stickyFeatures, false)
	}

	// Create override options
	overrideOptions := featurevisor.OverrideOptions{
		DefaultVariationValue: getDefaultVariationValue(assertion),
	}

	hasError := false
	errors := ""
	reportErrors := []reportError{}
	startTime := time.Now()

	// Test expectedToBeEnabled
	if expectedToBeEnabled, ok := assertion["expectedToBeEnabled"].(bool); ok {
		isEnabled := instance.IsEnabled(featureKey, context, overrideOptions)

		if isEnabled != expectedToBeEnabled {
			hasError = true
			errors += fmt.Sprintf("      ✘ expectedToBeEnabled: expected %v but received %v\n", expectedToBeEnabled, isEnabled)
			reportErrors = append(reportErrors, expectationReportError("flag", []interface{}{"expectedToBeEnabled"}, expectedToBeEnabled, isEnabled))
		}
	}

	// Test expectedVariation
	if expectedVariation, ok := assertion["expectedVariation"]; ok {
		variation := instance.GetVariation(featureKey, context, overrideOptions)

		var variationValue interface{}
		if variation != nil {
			variationValue = *variation
		} else {
			variationValue = nil
		}

		if !compareValues(variationValue, expectedVariation) {
			hasError = true
			errors += fmt.Sprintf("      ✘ expectedVariation: expected %v but received %v\n", expectedVariation, variationValue)
			reportErrors = append(reportErrors, expectationReportError("variation", []interface{}{"expectedVariation"}, expectedVariation, variationValue))
		}
	}

	// Test expectedVariables
	if expectedVariables, ok := assertion["expectedVariables"].(map[string]interface{}); ok {
		for variableKey, expectedValue := range expectedVariables {
			// Set default variable value for this specific variable
			if defaultValues, ok := assertion["defaultVariableValues"].(map[string]interface{}); ok {
				if defaultVal, ok := defaultValues[variableKey]; ok {
					overrideOptions.DefaultVariableValue = defaultVal
					overrideOptions.DefaultVariableValueSet = true
				}
			}

			actualValue := instance.GetVariable(featureKey, variableKey, context, overrideOptions)

			// Check if this is a JSON-type variable by looking at the feature definition
			// This matches the JavaScript test runner's logic for handling JSON variables
			var passed bool
			if expectedStr, ok := expectedValue.(string); ok {
				// Check if expected looks like a JSON string
				if len(expectedStr) > 0 && (expectedStr[0] == '{' || expectedStr[0] == '[') {
					// Parse the expected JSON string
					var parsedExpectedValue interface{}
					if err := json.Unmarshal([]byte(expectedStr), &parsedExpectedValue); err == nil {
						// For JSON variables, do deep comparison like JavaScript test runner
						if actualMap, ok := actualValue.(map[string]interface{}); ok {
							// Compare parsed JSON with actual map
							passed = compareMaps(parsedExpectedValue.(map[string]interface{}), actualMap)
						} else if actualSlice, ok := actualValue.([]interface{}); ok {
							// Compare parsed JSON with actual slice
							passed = compareSlices(parsedExpectedValue.([]interface{}), actualSlice)
						} else {
							// Fallback to general comparison
							passed = compareValues(actualValue, parsedExpectedValue)
						}

						if !passed {
							hasError = true
							// Show expected as-is (like JavaScript test runner) and serialize actual
							actualJSON, _ := json.Marshal(actualValue)
							errors += fmt.Sprintf("      ✘ expectedVariables.%s: expected %s but received %s\n",
								variableKey, expectedStr, string(actualJSON))
							reportErrors = append(reportErrors, expectationReportError("variable", []interface{}{"expectedVariables", variableKey}, expectedValue, string(actualJSON)))
						}
						continue
					}
				}
			}

			// Regular comparison for non-JSON strings or when JSON parsing fails
			if !compareValues(actualValue, expectedValue) {
				hasError = true
				errors += fmt.Sprintf("      ✘ expectedVariables.%s: expected %v but received %v\n", variableKey, expectedValue, actualValue)
				reportErrors = append(reportErrors, expectationReportError("variable", []interface{}{"expectedVariables", variableKey}, expectedValue, actualValue))
			}
		}
	}

	// Test children
	if children, ok := assertion["children"].([]interface{}); ok {
		for childIndex, child := range children {
			if childMap, ok := child.(map[string]interface{}); ok {
				childContext := featurevisor.Context{}
				if childCtx, ok := childMap["context"].(map[string]interface{}); ok {
					childContext = featurevisor.Context(childCtx)
				}

				childInstance := instance.Spawn(childContext, featurevisor.SpawnOptions{
					StickyFeatures:  stickyFeaturesFromAssertion(childMap, "stickyFeatures"),
					StickyVariables: stickyVariablesFromAssertion(childMap),
				})
				childResult := func() AssertionResult {
					defer childInstance.Close()
					return RunTestFeatureChild(childMap, featureKey, childInstance, level)
				}()

				if childResult.HasError {
					hasError = true
					errors += childResult.Errors
					for _, childError := range childResult.ReportErrors {
						childError.Path = appendPath([]interface{}{"children", childIndex}, childError.Path...)
						reportErrors = append(reportErrors, childError)
					}
				}
			}
		}
	}

	// Test expectedEvaluations
	if expectedEvaluations, ok := assertion["expectedEvaluations"].(map[string]interface{}); ok {
		if flagEvals, ok := expectedEvaluations["flag"].(map[string]interface{}); ok {
			evaluation := instance.EvaluateFlag(featureKey, context, overrideOptions)
			for key, expectedValue := range flagEvals {
				actualValue := getEvaluationValue(evaluation, key)
				if !compareValues(actualValue, expectedValue) {
					hasError = true
					errors += fmt.Sprintf("      ✘ expectedEvaluations.flag.%s: expected %v but received %v\n", key, expectedValue, actualValue)
					reportErrors = append(reportErrors, expectationReportError("evaluation", []interface{}{"expectedEvaluations", "flag", key}, expectedValue, actualValue))
				}
			}
		}

		if variationEvals, ok := expectedEvaluations["variation"].(map[string]interface{}); ok {
			evaluation := instance.EvaluateVariation(featureKey, context, overrideOptions)
			for key, expectedValue := range variationEvals {
				actualValue := getEvaluationValue(evaluation, key)
				if !compareValues(actualValue, expectedValue) {
					hasError = true
					errors += fmt.Sprintf("      ✘ expectedEvaluations.variation.%s: expected %v but received %v\n", key, expectedValue, actualValue)
					reportErrors = append(reportErrors, expectationReportError("evaluation", []interface{}{"expectedEvaluations", "variation", key}, expectedValue, actualValue))
				}
			}
		}

		if variableEvals, ok := expectedEvaluations["variables"].(map[string]interface{}); ok {
			for variableKey, expectedEval := range variableEvals {
				expectedEvalMap, ok := expectedEval.(map[string]interface{})
				if !ok {
					continue
				}

				variableOptions := overrideOptions
				variableOptions.DefaultVariableValue, variableOptions.DefaultVariableValueSet =
					getDefaultVariableValue(assertion, variableKey)
				evaluation := instance.EvaluateVariable(featureKey, variableKey, context, variableOptions)
				for key, expectedValue := range expectedEvalMap {
					actualValue := getEvaluationValue(evaluation, key)
					if !compareValues(actualValue, expectedValue) {
						hasError = true
						errors += fmt.Sprintf("      ✘ expectedEvaluations.variables.%s.%s: expected %v but received %v\n", variableKey, key, expectedValue, actualValue)
						reportErrors = append(reportErrors, expectationReportError("evaluation", []interface{}{"expectedEvaluations", "variables", variableKey, key}, expectedValue, actualValue))
					}
				}
			}
		}
	}

	duration := time.Since(startTime).Seconds()

	return AssertionResult{
		HasError:     hasError,
		Errors:       errors,
		Duration:     duration,
		ReportErrors: reportErrors,
	}
}

// TestFeatureChild tests a feature with child assertions
func RunTestFeatureChild(assertion map[string]interface{}, featureKey string, instance *featurevisor.FeaturevisorChild, level string) AssertionResult {
	context := featurevisor.Context{}
	if ctx, ok := assertion["context"].(map[string]interface{}); ok {
		context = featurevisor.Context(ctx)
	}

	// Create override options
	overrideOptions := featurevisor.OverrideOptions{
		DefaultVariationValue: getDefaultVariationValue(assertion),
	}

	hasError := false
	errors := ""
	reportErrors := []reportError{}
	startTime := time.Now()

	// Test expectedToBeEnabled
	if expectedToBeEnabled, ok := assertion["expectedToBeEnabled"].(bool); ok {
		isEnabled := instance.IsEnabled(featureKey, context, overrideOptions)

		if isEnabled != expectedToBeEnabled {
			hasError = true
			errors += fmt.Sprintf("      ✘ expectedToBeEnabled: expected %v but received %v\n", expectedToBeEnabled, isEnabled)
			reportErrors = append(reportErrors, expectationReportError("flag", []interface{}{"expectedToBeEnabled"}, expectedToBeEnabled, isEnabled))
		}
	}

	// Test expectedVariation
	if expectedVariation, ok := assertion["expectedVariation"]; ok {
		variation := instance.GetVariation(featureKey, context, overrideOptions)

		var variationValue interface{}
		if variation != nil {
			variationValue = *variation
		} else {
			variationValue = nil
		}

		if !compareValues(variationValue, expectedVariation) {
			hasError = true
			errors += fmt.Sprintf("      ✘ expectedVariation: expected %v but received %v\n", expectedVariation, variationValue)
			reportErrors = append(reportErrors, expectationReportError("variation", []interface{}{"expectedVariation"}, expectedVariation, variationValue))
		}
	}

	// Test expectedVariables
	if expectedVariables, ok := assertion["expectedVariables"].(map[string]interface{}); ok {
		for variableKey, expectedValue := range expectedVariables {
			// Set default variable value for this specific variable
			if defaultValues, ok := assertion["defaultVariableValues"].(map[string]interface{}); ok {
				if defaultVal, ok := defaultValues[variableKey]; ok {
					overrideOptions.DefaultVariableValue = defaultVal
					overrideOptions.DefaultVariableValueSet = true
				}
			}

			actualValue := instance.GetVariable(featureKey, variableKey, context, overrideOptions)

			// Check if this is a JSON-type variable by looking at the feature definition
			// This matches the JavaScript test runner's logic for handling JSON variables
			var passed bool
			if expectedStr, ok := expectedValue.(string); ok {
				// Check if expected looks like a JSON string
				if len(expectedStr) > 0 && (expectedStr[0] == '{' || expectedStr[0] == '[') {
					// Parse the expected JSON string
					var parsedExpectedValue interface{}
					if err := json.Unmarshal([]byte(expectedStr), &parsedExpectedValue); err == nil {
						// For JSON variables, do deep comparison like JavaScript test runner
						if actualMap, ok := actualValue.(map[string]interface{}); ok {
							// Compare parsed JSON with actual map
							passed = compareMaps(parsedExpectedValue.(map[string]interface{}), actualMap)
						} else if actualSlice, ok := actualValue.([]interface{}); ok {
							// Compare parsed JSON with actual slice
							passed = compareSlices(parsedExpectedValue.([]interface{}), actualSlice)
						} else {
							// Fallback to general comparison
							passed = compareValues(actualValue, parsedExpectedValue)
						}

						if !passed {
							hasError = true
							// Show expected as-is (like JavaScript test runner) and serialize actual
							actualJSON, _ := json.Marshal(actualValue)
							errors += fmt.Sprintf("      ✘ expectedVariables.%s: expected %s but received %s\n",
								variableKey, expectedStr, string(actualJSON))
							reportErrors = append(reportErrors, expectationReportError("variable", []interface{}{"expectedVariables", variableKey}, expectedValue, string(actualJSON)))
						}
						continue
					}
				}
			}

			// Regular comparison for non-JSON strings or when JSON parsing fails
			if !compareValues(actualValue, expectedValue) {
				hasError = true
				errors += fmt.Sprintf("      ✘ expectedVariables.%s: expected %v but received %v\n", variableKey, expectedValue, actualValue)
				reportErrors = append(reportErrors, expectationReportError("variable", []interface{}{"expectedVariables", variableKey}, expectedValue, actualValue))
			}
		}
	}

	if expectedEvaluations, ok := assertion["expectedEvaluations"].(map[string]interface{}); ok {
		if flagEvaluations, ok := expectedEvaluations["flag"].(map[string]interface{}); ok {
			evaluation := instance.EvaluateFlag(featureKey, context, overrideOptions)
			for key, expectedValue := range flagEvaluations {
				actualValue := getEvaluationValue(evaluation, key)
				if !compareValues(actualValue, expectedValue) {
					hasError = true
					errors += fmt.Sprintf("      ✘ expectedEvaluations.flag.%s: expected %v but received %v\n", key, expectedValue, actualValue)
					reportErrors = append(reportErrors, expectationReportError("evaluation", []interface{}{"expectedEvaluations", "flag", key}, expectedValue, actualValue))
				}
			}
		}

		if variationEvaluations, ok := expectedEvaluations["variation"].(map[string]interface{}); ok {
			evaluation := instance.EvaluateVariation(featureKey, context, overrideOptions)
			for key, expectedValue := range variationEvaluations {
				actualValue := getEvaluationValue(evaluation, key)
				if !compareValues(actualValue, expectedValue) {
					hasError = true
					errors += fmt.Sprintf("      ✘ expectedEvaluations.variation.%s: expected %v but received %v\n", key, expectedValue, actualValue)
					reportErrors = append(reportErrors, expectationReportError("evaluation", []interface{}{"expectedEvaluations", "variation", key}, expectedValue, actualValue))
				}
			}
		}

		if variableEvaluations, ok := expectedEvaluations["variables"].(map[string]interface{}); ok {
			for variableKey, expectedEvaluationValue := range variableEvaluations {
				expectedEvaluation, ok := expectedEvaluationValue.(map[string]interface{})
				if !ok {
					continue
				}

				variableOptions := overrideOptions
				variableOptions.DefaultVariableValue, variableOptions.DefaultVariableValueSet =
					getDefaultVariableValue(assertion, variableKey)
				evaluation := instance.EvaluateVariable(featureKey, variableKey, context, variableOptions)
				for key, expectedValue := range expectedEvaluation {
					actualValue := getEvaluationValue(evaluation, key)
					if !compareValues(actualValue, expectedValue) {
						hasError = true
						errors += fmt.Sprintf("      ✘ expectedEvaluations.variables.%s.%s: expected %v but received %v\n", variableKey, key, expectedValue, actualValue)
						reportErrors = append(reportErrors, expectationReportError("evaluation", []interface{}{"expectedEvaluations", "variables", variableKey, key}, expectedValue, actualValue))
					}
				}
			}
		}
	}

	duration := time.Since(startTime).Seconds()

	return AssertionResult{
		HasError:     hasError,
		Errors:       errors,
		Duration:     duration,
		ReportErrors: reportErrors,
	}
}

// TestSegment tests a segment with the given assertion
func RunTestSegment(assertion map[string]interface{}, segment map[string]interface{}, level string) AssertionResult {
	content := featurevisor.DatafileContent{SchemaVersion: "4", Revision: "segments", Features: map[string]featurevisor.Feature{}, Segments: map[string]featurevisor.Segment{}}
	return runSegmentWithSDK(assertion, "subject", map[string]interface{}{"subject": segment}, content, level)
}

// Helper functions
func getDefaultVariationValue(assertion map[string]interface{}) *string {
	if defaultVal, ok := assertion["defaultVariationValue"]; ok {
		if val, ok := defaultVal.(string); ok {
			return &val
		}
	}
	return nil
}

// getEvaluationValue extracts a value from an evaluation based on the key
func getEvaluationValue(evaluation featurevisor.Evaluation, key string) interface{} {
	switch key {
	case "type":
		return string(evaluation.Type)
	case "featureKey":
		return string(evaluation.FeatureKey)
	case "reason":
		return string(evaluation.Reason)
	case "bucketKey":
		if evaluation.BucketKey != nil {
			return string(*evaluation.BucketKey)
		}
		return nil
	case "bucketValue":
		if evaluation.BucketValue != nil {
			return int(*evaluation.BucketValue)
		}
		return nil
	case "ruleKey":
		if evaluation.RuleKey != nil {
			return string(*evaluation.RuleKey)
		}
		return nil
	case "error":
		return evaluation.Error
	case "enabled":
		if evaluation.Enabled != nil {
			return *evaluation.Enabled
		}
		return nil
	case "traffic":
		return evaluation.Traffic
	case "forceIndex":
		if evaluation.ForceIndex != nil {
			return *evaluation.ForceIndex
		}
		return nil
	case "force":
		return evaluation.Force
	case "conditions":
		return evaluation.Conditions
	case "stickyFeature":
		return evaluation.StickyFeature
	case "variation":
		return evaluation.Variation
	case "variationValue":
		if evaluation.VariationValue != nil {
			return string(*evaluation.VariationValue)
		}
		return nil
	case "variableKey":
		if evaluation.VariableKey != nil {
			return string(*evaluation.VariableKey)
		}
		return nil
	case "variableValue":
		return evaluation.VariableValue
	case "variableSchema":
		return evaluation.VariableSchema
	case "variableOverrideIndex":
		if evaluation.VariableOverrideIndex != nil {
			return *evaluation.VariableOverrideIndex
		}
		return nil
	case "variableOverrideKey":
		if evaluation.VariableOverrideKey != nil {
			return *evaluation.VariableOverrideKey
		}
		return nil
	case "variableOverridePath":
		return evaluation.VariableOverridePath
	case "variableOverridePaths":
		return evaluation.VariableOverridePaths
	default:
		return nil
	}
}

func getDefaultVariableValue(assertion map[string]interface{}, variableKey string) (featurevisor.VariableValue, bool) {
	if defaultValues, ok := assertion["defaultVariableValues"].(map[string]interface{}); ok {
		if defaultVal, ok := defaultValues[variableKey]; ok {
			return defaultVal, true
		}
	}
	return nil, false
}

// compareSlices compares two slices for equality
func compareSlices(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if !compareValues(v, b[i]) {
			return false
		}
	}
	return true
}

// compareMaps compares two maps for equality
func compareMaps(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bVal, exists := b[k]; !exists || !compareValues(v, bVal) {
			return false
		}
	}
	return true
}

// compareValues compares two values, handling type conversions for numeric types
func compareValues(actual, expected interface{}) bool {
	// Handle nil cases
	if actual == nil && expected == nil {
		return true
	}
	if actual == nil || expected == nil {
		return false
	}

	// Handle empty string vs nil for variation values
	if actualStr, ok := actual.(string); ok && actualStr == "" && expected == nil {
		return true
	}
	if expectedStr, ok := expected.(string); ok && expectedStr == "" && actual == nil {
		return true
	}

	// Handle numeric type conversions
	switch actualVal := actual.(type) {
	case int:
		switch expectedVal := expected.(type) {
		case int:
			return actualVal == expectedVal
		case float64:
			return float64(actualVal) == expectedVal
		}
	case float64:
		switch expectedVal := expected.(type) {
		case int:
			return actualVal == float64(expectedVal)
		case float64:
			return actualVal == expectedVal
		}
	}

	// Handle JSON string comparison
	// If expected is a JSON string and actual is a map, serialize the map to JSON string
	if expectedStr, ok := expected.(string); ok {
		if actualMap, ok := actual.(map[string]interface{}); ok {
			// Check if expected looks like a JSON string
			if len(expectedStr) > 0 && (expectedStr[0] == '{' || expectedStr[0] == '[') {
				// Serialize the actual map to JSON string
				if actualJSON, err := json.Marshal(actualMap); err == nil {
					// Normalize whitespace for comparison
					expectedNormalized := strings.ReplaceAll(strings.ReplaceAll(expectedStr, " ", ""), "\n", "")
					actualNormalized := strings.ReplaceAll(strings.ReplaceAll(string(actualJSON), " ", ""), "\n", "")
					return expectedNormalized == actualNormalized
				}
			}
		}
	}

	// Handle slice comparison
	if actualSlice, ok := actual.([]interface{}); ok {
		if expectedSlice, ok := expected.([]interface{}); ok {
			return compareSlices(actualSlice, expectedSlice)
		}
	}

	// Handle map comparison
	if actualMap, ok := actual.(map[string]interface{}); ok {
		if expectedMap, ok := expected.(map[string]interface{}); ok {
			return compareMaps(actualMap, expectedMap)
		}
	}

	// For other types, use direct comparison (only for comparable types)
	switch actual.(type) {
	case string, bool, int, float64:
		return actual == expected
	default:
		return valuesEqual(actual, expected)
	}
}

// Common functions
func executeFeaturevisorCommand(featurevisorProjectPath string, args ...string) (string, error) {
	cmdArgs := append([]string{"featurevisor"}, args...)
	cmd := exec.Command("npx", cmdArgs...)
	cmd.Dir = featurevisorProjectPath
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("command failed: npx %s\n%s", strings.Join(cmdArgs, " "), strings.TrimSpace(diagnostics.String()+"\n"+string(output)))
	}
	// Delegated warnings belong on stderr, never inside a JSON datafile.
	if diagnostics.Len() > 0 {
		fmt.Fprint(os.Stderr, diagnostics.String())
	}

	return strings.TrimSpace(string(output)), nil
}

func mustExecuteFeaturevisorCommand(featurevisorProjectPath string, args ...string) string {
	output, err := executeFeaturevisorCommand(featurevisorProjectPath, args...)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	return output
}

func getConfig(featurevisorProjectPath string) map[string]interface{} {
	fmt.Println("Getting config...")
	configOutput := mustExecuteFeaturevisorCommand(featurevisorProjectPath, "config", "--json")
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configOutput), &config); err != nil {
		fmt.Printf("failed to parse config json: %v\n", err)
		os.Exit(1)
	}
	return config
}

func getSegments(featurevisorProjectPath string, set ...string) map[string]interface{} {
	fmt.Println("Getting segments...")
	args := []string{"list", "--segments", "--json"}
	if len(set) > 0 && set[0] != "" {
		args = append(args, "--set="+set[0])
	}
	segmentsOutput := mustExecuteFeaturevisorCommand(featurevisorProjectPath, args...)
	var segments []map[string]interface{}
	if err := json.Unmarshal([]byte(segmentsOutput), &segments); err != nil {
		fmt.Printf("failed to parse segments json: %v\n", err)
		os.Exit(1)
	}

	segmentsByKey := make(map[string]interface{})
	for _, segment := range segments {
		if key, ok := segment["key"].(string); ok {
			segmentsByKey[key] = segment
		}
	}
	return segmentsByKey
}

func getTargets(featurevisorProjectPath string, set ...string) []string {
	fmt.Println("Getting targets...")
	args := []string{"list", "--targets", "--json"}
	if len(set) > 0 && set[0] != "" {
		args = append(args, "--set="+set[0])
	}
	targetsOutput := mustExecuteFeaturevisorCommand(featurevisorProjectPath, args...)
	var targets []map[string]interface{}
	if err := json.Unmarshal([]byte(targetsOutput), &targets); err != nil {
		fmt.Printf("failed to parse targets json: %v\n", err)
		os.Exit(1)
	}

	targetKeys := []string{}
	for _, target := range targets {
		if key, ok := target["key"].(string); ok {
			targetKeys = append(targetKeys, key)
		}
	}

	return targetKeys
}

func buildDatafileJSON(featurevisorProjectPath string, environment *string, inflate int, target *string, set ...string) interface{} {
	args := []string{"build"}
	if environment != nil {
		args = append(args, fmt.Sprintf("--environment=%s", *environment))
	}
	if inflate > 0 {
		args = append(args, fmt.Sprintf("--inflate=%d", inflate))
	}
	if target != nil {
		args = append(args, fmt.Sprintf("--target=%s", *target))
	}
	if len(set) > 0 && set[0] != "" {
		args = append(args, "--set="+set[0])
	}
	args = append(args, "--json")

	datafileOutput := mustExecuteFeaturevisorCommand(featurevisorProjectPath, args...)
	var datafile featurevisor.DatafileContent
	if err := json.Unmarshal([]byte(datafileOutput), &datafile); err != nil {
		fmt.Printf("failed to parse datafile json: %v\n", err)
		os.Exit(1)
	}

	return datafile
}

func buildDatafiles(featurevisorProjectPath string, environments []string, inflate int) map[string]interface{} {
	datafilesByEnvironment := make(map[string]interface{})
	for _, environment := range environments {
		envCopy := environment
		datafilesByEnvironment[datafileCacheKey(&envCopy)] = buildDatafileJSON(
			featurevisorProjectPath,
			&envCopy,
			inflate,
			nil,
		)
	}
	return datafilesByEnvironment
}

func datafileCacheKey(environment *string) string {
	return selectionDatafileCacheKey(environment, nil)
}

func targetDatafileCacheKey(environment *string, target string) string {
	return selectionDatafileCacheKey(environment, &target)
}

func selectionDatafileCacheKey(environment, target *string) string {
	// A pair of nullable strings cannot fail JSON serialization.
	encoded, _ := json.Marshal([2]*string{environment, target})
	return string(encoded)
}

func datafileCacheKeyForAssertion(assertion map[string]interface{}, datafileCache map[string]interface{}) string {
	var environment *string
	if rawEnvironment, exists := assertion["environment"]; exists {
		if env, ok := rawEnvironment.(string); ok {
			envCopy := env
			environment = &envCopy
		}
	}

	if targetValue, ok := assertion["target"].(string); ok && targetValue != "" {
		return targetDatafileCacheKey(environment, targetValue)
	}

	return datafileCacheKey(environment)
}

func buildDatafileCache(
	featurevisorProjectPath string,
	config map[string]interface{},
	inflate int,
	selectedTargets []string,
	set string,
) map[string]interface{} {
	cache := make(map[string]interface{})
	targetKeys := selectedTargets
	if len(targetKeys) == 0 {
		targetKeys = getTargets(featurevisorProjectPath, set)
	}

	environments := []*string{nil}
	if envList, ok := config["environments"].([]interface{}); ok {
		environments = make([]*string, 0, len(envList))
		for _, raw := range envList {
			if env, ok := raw.(string); ok {
				envCopy := env
				environments = append(environments, &envCopy)
			}
		}
	}

	for _, environment := range environments {
		baseKey := datafileCacheKey(environment)
		cache[baseKey] = buildDatafileJSON(featurevisorProjectPath, environment, inflate, nil, set)

		for _, targetKey := range targetKeys {
			targetCopy := targetKey
			cache[targetDatafileCacheKey(environment, targetKey)] = buildDatafileJSON(
				featurevisorProjectPath,
				environment,
				inflate,
				&targetCopy,
				set,
			)
		}
	}

	return cache
}

func getLoggerLevel(opts CLIOptions) string {
	level := "warn"
	if opts.Verbose {
		level = "debug"
	} else if opts.Quiet {
		level = "error"
	}
	return level
}

func getTests(featurevisorProjectPath string, opts CLIOptions) []map[string]interface{} {
	args := []string{"list", "--tests", "--execution-protocol", "--applyMatrix", "--execution-metadata", "--json"}
	if opts.KeyPattern != "" {
		args = append(args, fmt.Sprintf("--keyPattern=%s", opts.KeyPattern))
	}
	if opts.AssertionPattern != "" {
		args = append(args, fmt.Sprintf("--assertionPattern=%s", opts.AssertionPattern))
	}
	if opts.EntityType != "" {
		args = append(args, fmt.Sprintf("--entityType=%s", opts.EntityType))
	}
	if opts.Set != "" {
		args = append(args, "--set="+opts.Set)
	}

	testsOutput := mustExecuteFeaturevisorCommand(featurevisorProjectPath, args...)
	tests, err := parseTestExecutionProtocol(testsOutput)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	return tests
}

func datafileContentFromRaw(datafile interface{}) featurevisor.DatafileContent {
	if content, ok := datafile.(featurevisor.DatafileContent); ok {
		return content
	}
	var datafileContent featurevisor.DatafileContent
	if datafileBytes, err := json.Marshal(datafile); err == nil {
		if err := json.Unmarshal(datafileBytes, &datafileContent); err != nil {
			fmt.Printf("failed to parse datafile for assertion: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("failed to marshal datafile for assertion: %v\n", err)
		os.Exit(1)
	}
	return datafileContent
}

func buildInstanceForAssertion(datafile interface{}, level string, assertion map[string]interface{}) (*featurevisor.Featurevisor, *checkDiagnosticCollector) {
	datafileContent := datafileContentFromRaw(datafile)

	levelStr := featurevisor.LogLevel(level)
	diagnostics := &checkDiagnosticCollector{}
	instance := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
		Datafile:        datafileContent,
		LogLevel:        &levelStr,
		Context:         contextFromAssertion(assertion),
		StickyFeatures:  stickyFeaturesFromAssertion(assertion, "stickyFeatures"),
		StickyVariables: stickyVariablesFromAssertion(assertion),
		OnDiagnostic:    diagnostics.record,
		Modules: []*featurevisor.FeaturevisorModule{
			{
				Name: "tester-module",
				BucketValue: func(options featurevisor.ConfigureBucketValueOptions) int {
					if at, ok := assertion["at"].(float64); ok {
						return int(at * 1000)
					}
					return options.BucketValue
				},
			},
		},
	})
	return instance, diagnostics
}

func toContextMap(value interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	if context, ok := value.(map[string]interface{}); ok {
		return context
	}
	return map[string]interface{}{}
}

func cloneAssertion(assertion map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(assertion))
	for key, value := range assertion {
		cloned[key] = value
	}
	return cloned
}

func runTestLegacy(opts CLIOptions) {
	featurevisorProjectPath := opts.ProjectDirectoryPath

	config := getConfig(featurevisorProjectPath)
	segmentsByKey := getSegments(featurevisorProjectPath, opts.Set)

	datafileCache := buildDatafileCache(
		featurevisorProjectPath,
		config,
		opts.Inflate,
		opts.Targets,
		opts.Set,
	)

	fmt.Println()

	level := getLoggerLevel(opts)
	tests := getTests(featurevisorProjectPath, opts)

	if len(tests) == 0 {
		fmt.Println("No tests found")
		return
	}

	passedTestsCount := 0
	failedTestsCount := 0
	passedAssertionsCount := 0
	failedAssertionsCount := 0

	for _, test := range tests {
		testKey := test["key"].(string)
		discriminatorCount := 0
		for _, field := range []string{"feature", "segment", "variable", "scenario"} {
			if _, exists := test[field]; exists {
				discriminatorCount++
			}
		}
		if discriminatorCount != 1 {
			failedTestsCount++
			failedAssertionsCount++
			if !opts.Quiet {
				fmt.Printf("\nTesting: %s (0.00ms)\n", testKey)
				fmt.Println("      ✘ test must contain exactly one of feature, segment, variable, or scenario")
			}
			continue
		}
		assertions, assertionsOK := test["assertions"].([]interface{})
		if !assertionsOK || len(assertions) == 0 {
			if _, scenario := test["scenario"]; scenario {
				failedTestsCount++
				failedAssertionsCount++
				continue
			}
			continue
		}
		if _, hasFeature := test["feature"]; (hasFeature || test["variable"] != nil || test["scenario"] != nil) && len(opts.Targets) > 0 {
			filtered := make([]interface{}, 0, len(assertions))
			for _, raw := range assertions {
				assertion, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				target, _ := assertion["target"].(string)
				if target == "" || containsString(opts.Targets, target) {
					filtered = append(filtered, raw)
				}
			}
			assertions = filtered
			if len(assertions) == 0 {
				continue
			}
		}
		results := ""
		testHasError := false
		testDuration := 0.0

		for _, assertion := range assertions {
			if assertionMap, ok := assertion.(map[string]interface{}); ok {
				var testResult AssertionResult

				if _, hasFeature := test["feature"]; hasFeature {
					selectedDatafileKey := datafileCacheKeyForAssertion(assertionMap, datafileCache)

					datafile, ok := datafileCache[selectedDatafileKey]
					if !ok {
						testResult = missingDatafileAssertion(assertionMap, selectedDatafileKey)
					} else {

						effectiveAssertion := cloneAssertion(assertionMap)
						instance, _ := buildInstanceForAssertion(datafile, level, effectiveAssertion)

						// Show datafile if requested (matching TypeScript implementation)
						if opts.ShowDatafile {
							fmt.Println("")
							datafileJSON, _ := json.MarshalIndent(datafile, "", "  ")
							fmt.Println(string(datafileJSON))
							fmt.Println("")
						}

						testResult = RunTestFeature(effectiveAssertion, test["feature"].(string), instance, level)
						instance.Close()
					}
				} else if variableKey, hasVariable := test["variable"].(string); hasVariable {
					selectedDatafileKey := datafileCacheKeyForAssertion(assertionMap, datafileCache)
					datafile, ok := datafileCache[selectedDatafileKey]
					if !ok {
						testResult = missingDatafileAssertion(assertionMap, selectedDatafileKey)
					} else {
						if opts.ShowDatafile {
							fmt.Println("")
							datafileJSON, _ := json.MarshalIndent(datafile, "", "  ")
							fmt.Println(string(datafileJSON))
							fmt.Println("")
						}
						datafileContent := datafileContentFromRaw(datafile)
						instance, diagnostics := buildInstanceForAssertion(datafile, level, assertionMap)
						testResult = RunTestVariable(assertionMap, variableKey, instance, diagnostics, datafileContent, segmentDefinitionsFromTest(test), level)
						instance.Close()
					}
				} else if _, hasScenario := test["scenario"].(string); hasScenario {
					selectedDatafileKey := datafileCacheKeyForAssertion(assertionMap, datafileCache)
					datafile, ok := datafileCache[selectedDatafileKey]
					if !ok {
						testResult = missingDatafileAssertion(assertionMap, selectedDatafileKey)
					} else {
						if opts.ShowDatafile {
							fmt.Println("")
							datafileJSON, _ := json.MarshalIndent(datafile, "", "  ")
							fmt.Println(string(datafileJSON))
							fmt.Println("")
						}
						datafileContent := datafileContentFromRaw(datafile)
						instance, diagnostics := buildInstanceForAssertion(datafile, level, assertionMap)
						testResult = RunTestScenario(assertionMap, instance, diagnostics, datafileContent, segmentDefinitionsFromTest(test), level)
						instance.Close()
					}
				} else if _, hasSegment := test["segment"]; hasSegment {
					segmentKey := test["segment"].(string)
					testResult = runSegmentSpec(assertionMap, segmentKey, test, segmentsByKey, datafileCache, level)
				}

				testDuration += testResult.Duration

				description := ""
				if desc, ok := assertionMap["description"].(string); ok {
					description = desc
				}
				if testResult.HasError {
					results += fmt.Sprintf("  ✘ %s (%.2fms)\n", description, testResult.Duration*1000)
					results += testResult.Errors
					testHasError = true
					failedAssertionsCount++
				} else {
					results += fmt.Sprintf("  ✔ %s (%.2fms)\n", description, testResult.Duration*1000)
					passedAssertionsCount++
				}
			}
		}

		if !opts.OnlyFailures || (opts.OnlyFailures && testHasError) {
			fmt.Printf("\nTesting: %s (%.2fms)\n", testKey, testDuration*1000)
			fmt.Print(results)
		}

		if testHasError {
			failedTestsCount++
		} else {
			passedTestsCount++
		}
	}

	fmt.Println()
	fmt.Printf("Test specs: %d passed, %d failed\n", passedTestsCount, failedTestsCount)
	fmt.Printf("Assertions: %d passed, %d failed\n", passedAssertionsCount, failedAssertionsCount)
	fmt.Println()

	if failedTestsCount > 0 {
		// Exit with error code 1
		os.Exit(1)
	}
}

func missingDatafileAssertion(assertion map[string]interface{}, cacheKey string) AssertionResult {
	environment, _ := assertion["environment"].(string)
	if environment == "" {
		environment = "none"
	}
	target, _ := assertion["target"].(string)
	message := fmt.Sprintf("datafile not found for environment %q", environment)
	if target != "" {
		message += fmt.Sprintf(" and target %q", target)
	}
	return AssertionResult{HasError: true, Errors: fmt.Sprintf("      ✘ %s (cache key: %s)\n", message, cacheKey)}
}

func convertToStringSlice(interfaces []interface{}) []string {
	strings := make([]string, len(interfaces))
	for i, v := range interfaces {
		strings[i] = v.(string)
	}
	return strings
}
