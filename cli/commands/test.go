package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/featurevisor/featurevisor-go/sdk"
)

// TestFeature tests a feature with the given assertion
func RunTestFeature(assertion map[string]interface{}, featureKey string, instance *sdk.Featurevisor, level string) AssertionResult {
	context := sdk.Context{}
	if ctx, ok := assertion["context"].(map[string]interface{}); ok {
		context = sdk.Context(ctx)
	}

	// Set context and sticky for this assertion
	instance.SetContext(context, false)
	if sticky, ok := assertion["sticky"].(map[string]interface{}); ok {
		// Convert sticky to proper format
		stickyFeatures := sdk.StickyFeatures{}
		for key, value := range sticky {
			if featureSticky, ok := value.(map[string]interface{}); ok {
				evaluatedFeature := sdk.EvaluatedFeature{}
				if enabled, ok := featureSticky["enabled"].(bool); ok {
					evaluatedFeature.Enabled = enabled
				}
				if variation, ok := featureSticky["variation"]; ok {
					if variationStr, ok := variation.(string); ok {
						evaluatedFeature.Variation = &variationStr
					}
				}
				if variables, ok := featureSticky["variables"].(map[string]interface{}); ok {
					evaluatedFeature.Variables = make(map[sdk.VariableKey]sdk.VariableValue)
					for varKey, varValue := range variables {
						evaluatedFeature.Variables[sdk.VariableKey(varKey)] = varValue
					}
				}
				stickyFeatures[sdk.FeatureKey(key)] = evaluatedFeature
			}
		}
		instance.SetSticky(stickyFeatures, false)
	}

	// Create override options
	overrideOptions := sdk.OverrideOptions{
		DefaultVariationValue: getDefaultVariationValue(assertion),
	}

	hasError := false
	errors := ""
	startTime := time.Now()

	// Test expectedToBeEnabled
	if expectedToBeEnabled, ok := assertion["expectedToBeEnabled"].(bool); ok {
		isEnabled := instance.IsEnabled(featureKey, context, overrideOptions)

		if isEnabled != expectedToBeEnabled {
			hasError = true
			errors += fmt.Sprintf("      ✘ expectedToBeEnabled: expected %v but received %v\n", expectedToBeEnabled, isEnabled)
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
		}
	}

	// Test expectedVariables
	if expectedVariables, ok := assertion["expectedVariables"].(map[string]interface{}); ok {
		for variableKey, expectedValue := range expectedVariables {
			// Set default variable value for this specific variable
			if defaultValues, ok := assertion["defaultVariableValues"].(map[string]interface{}); ok {
				if defaultVal, ok := defaultValues[variableKey]; ok {
					overrideOptions.DefaultVariableValue = defaultVal
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
						}
						continue
					}
				}
			}

			// Regular comparison for non-JSON strings or when JSON parsing fails
			if !compareValues(actualValue, expectedValue) {
				hasError = true
				errors += fmt.Sprintf("      ✘ expectedVariables.%s: expected %v but received %v\n", variableKey, expectedValue, actualValue)
			}
		}
	}

	// Test expectedEvaluations (matching TypeScript implementation)
	if expectedEvaluations, ok := assertion["expectedEvaluations"].(map[string]interface{}); ok {
		// Test flag evaluations
		if flagEvals, ok := expectedEvaluations["flag"].(map[string]interface{}); ok {
			evaluation := instance.EvaluateFlag(featureKey, context, overrideOptions)
			for key, expectedValue := range flagEvals {
				actualValue := getEvaluationValue(evaluation, key)
				if !compareValues(actualValue, expectedValue) {
					hasError = true
					errors += fmt.Sprintf("      ✘ expectedEvaluations.flag.%s: expected %v but received %v\n", key, expectedValue, actualValue)
				}
			}
		}

		// Test variation evaluations
		if variationEvals, ok := expectedEvaluations["variation"].(map[string]interface{}); ok {
			evaluation := instance.EvaluateVariation(featureKey, context, overrideOptions)
			for key, expectedValue := range variationEvals {
				actualValue := getEvaluationValue(evaluation, key)
				if !compareValues(actualValue, expectedValue) {
					hasError = true
					errors += fmt.Sprintf("      ✘ expectedEvaluations.variation.%s: expected %v but received %v\n", key, expectedValue, actualValue)
				}
			}
		}

		// Test variable evaluations
		if variableEvals, ok := expectedEvaluations["variables"].(map[string]interface{}); ok {
			for variableKey, expectedEval := range variableEvals {
				if expectedEvalMap, ok := expectedEval.(map[string]interface{}); ok {
					evaluation := instance.EvaluateVariable(featureKey, sdk.VariableKey(variableKey), context, overrideOptions)
					for key, expectedValue := range expectedEvalMap {
						actualValue := getEvaluationValue(evaluation, key)
						if !compareValues(actualValue, expectedValue) {
							hasError = true
							errors += fmt.Sprintf("      ✘ expectedEvaluations.variables.%s.%s: expected %v but received %v\n", variableKey, key, expectedValue, actualValue)
						}
					}
				}
			}
		}
	}

	// Test children
	if children, ok := assertion["children"].([]interface{}); ok {
		for _, child := range children {
			if childMap, ok := child.(map[string]interface{}); ok {
				childContext := sdk.Context{}
				if childCtx, ok := childMap["context"].(map[string]interface{}); ok {
					childContext = sdk.Context(childCtx)
				}

				// Create override options for child with sticky values
				childOverrideOptions := sdk.OverrideOptions{
					DefaultVariationValue: getDefaultVariationValue(childMap),
				}

				// Pass sticky values to child instance (matching TypeScript implementation)
				childInstance := instance.Spawn(childContext, childOverrideOptions)

				// Set sticky values for child if they exist
				if sticky, ok := assertion["sticky"].(map[string]interface{}); ok {
					// Convert sticky to proper format
					stickyFeatures := sdk.StickyFeatures{}
					for key, value := range sticky {
						if featureSticky, ok := value.(map[string]interface{}); ok {
							evaluatedFeature := sdk.EvaluatedFeature{}
							if enabled, ok := featureSticky["enabled"].(bool); ok {
								evaluatedFeature.Enabled = enabled
							}
							if variation, ok := featureSticky["variation"]; ok {
								if variationStr, ok := variation.(string); ok {
									evaluatedFeature.Variation = &variationStr
								}
							}
							if variables, ok := featureSticky["variables"].(map[string]interface{}); ok {
								evaluatedFeature.Variables = make(map[sdk.VariableKey]sdk.VariableValue)
								for varKey, varValue := range variables {
									evaluatedFeature.Variables[sdk.VariableKey(varKey)] = varValue
								}
							}
							stickyFeatures[sdk.FeatureKey(key)] = evaluatedFeature
						}
					}
					childInstance.SetSticky(stickyFeatures, false)
				}

				childResult := RunTestFeatureChild(childMap, featureKey, childInstance, level)

				if childResult.HasError {
					hasError = true
					errors += childResult.Errors
				}
			}
		}
	}

	duration := time.Since(startTime).Seconds()

	return AssertionResult{
		HasError: hasError,
		Errors:   errors,
		Duration: duration,
	}
}

// TestFeatureChild tests a feature with child assertions
func RunTestFeatureChild(assertion map[string]interface{}, featureKey string, instance *sdk.FeaturevisorChild, level string) AssertionResult {
	context := sdk.Context{}
	if ctx, ok := assertion["context"].(map[string]interface{}); ok {
		context = sdk.Context(ctx)
	}

	// Create override options
	overrideOptions := sdk.OverrideOptions{
		DefaultVariationValue: getDefaultVariationValue(assertion),
	}

	hasError := false
	errors := ""
	startTime := time.Now()

	// Test expectedToBeEnabled
	if expectedToBeEnabled, ok := assertion["expectedToBeEnabled"].(bool); ok {
		isEnabled := instance.IsEnabled(featureKey, context, overrideOptions)

		if isEnabled != expectedToBeEnabled {
			hasError = true
			errors += fmt.Sprintf("      ✘ expectedToBeEnabled: expected %v but received %v\n", expectedToBeEnabled, isEnabled)
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
		}
	}

	// Test expectedVariables
	if expectedVariables, ok := assertion["expectedVariables"].(map[string]interface{}); ok {
		for variableKey, expectedValue := range expectedVariables {
			// Set default variable value for this specific variable
			if defaultValues, ok := assertion["defaultVariableValues"].(map[string]interface{}); ok {
				if defaultVal, ok := defaultValues[variableKey]; ok {
					overrideOptions.DefaultVariableValue = defaultVal
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
						}
						continue
					}
				}
			}

			// Regular comparison for non-JSON strings or when JSON parsing fails
			if !compareValues(actualValue, expectedValue) {
				hasError = true
				errors += fmt.Sprintf("      ✘ expectedVariables.%s: expected %v but received %v\n", variableKey, expectedValue, actualValue)
			}
		}
	}

	duration := time.Since(startTime).Seconds()

	return AssertionResult{
		HasError: hasError,
		Errors:   errors,
		Duration: duration,
	}
}

// TestSegment tests a segment with the given assertion
func RunTestSegment(assertion map[string]interface{}, segment map[string]interface{}, level string) AssertionResult {
	context := sdk.Context{}
	if ctx, ok := assertion["context"].(map[string]interface{}); ok {
		context = sdk.Context(ctx)
	}

	conditions := segment["conditions"]

	datafile := sdk.DatafileContent{
		SchemaVersion: "2",
		Revision:      "tester",
		Features:      make(map[sdk.FeatureKey]sdk.Feature),
		Segments:      make(map[sdk.SegmentKey]sdk.Segment),
	}

	levelStr := sdk.LogLevel(level)
	logger := sdk.NewLogger(sdk.CreateLoggerOptions{Level: &levelStr})
	datafileReader := sdk.NewDatafileReader(sdk.DatafileReaderOptions{
		Datafile: datafile,
		Logger:   logger,
	})

	hasError := false
	errors := ""
	startTime := time.Now()

	if expectedToMatch, ok := assertion["expectedToMatch"].(bool); ok {
		actual := datafileReader.AllConditionsAreMatched(conditions, context)
		if actual != expectedToMatch {
			hasError = true
			errors += fmt.Sprintf("      ✘ expectedToMatch: expected %v but received %v\n", expectedToMatch, actual)
		}
	}

	duration := time.Since(startTime).Seconds()
	return AssertionResult{
		HasError: hasError,
		Errors:   errors,
		Duration: duration,
	}
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
func getEvaluationValue(evaluation sdk.Evaluation, key string) interface{} {
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
	case "required":
		return evaluation.Required
	case "sticky":
		return evaluation.Sticky
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
	default:
		return nil
	}
}

func getDefaultVariableValue(assertion map[string]interface{}, variableKey string) sdk.VariableValue {
	if defaultValues, ok := assertion["defaultVariableValues"].(map[string]interface{}); ok {
		if defaultVal, ok := defaultValues[variableKey]; ok {
			if val, ok := defaultVal.(sdk.VariableValue); ok {
				return val
			}
		}
	}
	return nil
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
		// For uncomparable types, return false
		return false
	}
}

// Common functions
func executeCommand(command string) string {
	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func getConfig(featurevisorProjectPath string) map[string]interface{} {
	fmt.Println("Getting config...")
	configOutput := executeCommand(fmt.Sprintf("(cd %s && npx featurevisor config --json)", featurevisorProjectPath))
	var config map[string]interface{}
	json.Unmarshal([]byte(configOutput), &config)
	return config
}

func getSegments(featurevisorProjectPath string) map[string]interface{} {
	fmt.Println("Getting segments...")
	segmentsOutput := executeCommand(fmt.Sprintf("(cd %s && npx featurevisor list --segments --json)", featurevisorProjectPath))
	var segments []map[string]interface{}
	json.Unmarshal([]byte(segmentsOutput), &segments)

	segmentsByKey := make(map[string]interface{})
	for _, segment := range segments {
		if key, ok := segment["key"].(string); ok {
			segmentsByKey[key] = segment
		}
	}
	return segmentsByKey
}

func buildDatafiles(featurevisorProjectPath string, environments []string, schemaVersion string, inflate int) map[string]interface{} {
	datafilesByEnvironment := make(map[string]interface{})
	for _, environment := range environments {
		fmt.Printf("Building datafile for environment: %s...\n", environment)
		command := fmt.Sprintf("(cd %s && npx featurevisor build --environment=%s --json)", featurevisorProjectPath, environment)
		if schemaVersion != "" {
			command = fmt.Sprintf("(cd %s && npx featurevisor build --environment=%s --schemaVersion=%s --json)", featurevisorProjectPath, environment, schemaVersion)
		}
		if inflate > 0 {
			command = fmt.Sprintf("(cd %s && npx featurevisor build --environment=%s --inflate=%d --json)", featurevisorProjectPath, environment, inflate)
			if schemaVersion != "" {
				command = fmt.Sprintf("(cd %s && npx featurevisor build --environment=%s --schemaVersion=%s --inflate=%d --json)", featurevisorProjectPath, environment, schemaVersion, inflate)
			}
		}
		datafileOutput := executeCommand(command)
		var datafile interface{}
		json.Unmarshal([]byte(datafileOutput), &datafile)
		datafilesByEnvironment[environment] = datafile
	}
	return datafilesByEnvironment
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
	testsSuffix := ""
	if opts.KeyPattern != "" {
		testsSuffix = fmt.Sprintf(" --keyPattern=%s", opts.KeyPattern)
	}
	if opts.AssertionPattern != "" {
		testsSuffix += fmt.Sprintf(" --assertionPattern=%s", opts.AssertionPattern)
	}

	testsOutput := executeCommand(fmt.Sprintf("(cd %s && npx featurevisor list --tests --applyMatrix --json%s)", featurevisorProjectPath, testsSuffix))
	var tests []map[string]interface{}
	json.Unmarshal([]byte(testsOutput), &tests)
	return tests
}

func runTest(opts CLIOptions) {
	featurevisorProjectPath := opts.ProjectDirectoryPath

	config := getConfig(featurevisorProjectPath)
	environments := config["environments"].([]interface{})
	segmentsByKey := getSegments(featurevisorProjectPath)

	// Use CLI schemaVersion option or fallback to config
	schemaVersion := opts.SchemaVersion
	if schemaVersion == "" {
		if configSchemaVersion, ok := config["schemaVersion"].(string); ok {
			schemaVersion = configSchemaVersion
		}
	}

	datafilesByEnvironment := buildDatafiles(featurevisorProjectPath, convertToStringSlice(environments), schemaVersion, opts.Inflate)

	fmt.Println()

	level := getLoggerLevel(opts)
	tests := getTests(featurevisorProjectPath, opts)

	if len(tests) == 0 {
		fmt.Println("No tests found")
		return
	}

	// Create SDK instances for each environment
	sdkInstancesByEnvironment := make(map[string]*sdk.Featurevisor)

	for _, environment := range environments {
		if envStr, ok := environment.(string); ok {
			datafile := datafilesByEnvironment[envStr]

			// Convert datafile to proper format
			var datafileContent sdk.DatafileContent
			if datafileBytes, err := json.Marshal(datafile); err == nil {
				json.Unmarshal(datafileBytes, &datafileContent)
			}

			levelStr := sdk.LogLevel(level)
			instance := sdk.CreateInstance(sdk.InstanceOptions{
				Datafile: datafileContent,
				LogLevel: &levelStr,
				Hooks: []*sdk.Hook{
					{
						Name: "tester-hook",
						BucketValue: func(options sdk.ConfigureBucketValueOptions) int {
							// This will be overridden per assertion if needed
							return options.BucketValue
						},
					},
				},
			})

			sdkInstancesByEnvironment[envStr] = instance
		}
	}

	passedTestsCount := 0
	failedTestsCount := 0
	passedAssertionsCount := 0
	failedAssertionsCount := 0

	for _, test := range tests {
		testKey := test["key"].(string)
		assertions := test["assertions"].([]interface{})
		results := ""
		testHasError := false
		testDuration := 0.0

		for _, assertion := range assertions {
			if assertionMap, ok := assertion.(map[string]interface{}); ok {
				var testResult AssertionResult

				if _, hasFeature := test["feature"]; hasFeature {
					environment := assertionMap["environment"].(string)
					instance := sdkInstancesByEnvironment[environment]

					// Show datafile if requested (matching TypeScript implementation)
					if opts.ShowDatafile {
						datafile := datafilesByEnvironment[environment]
						fmt.Println("")
						datafileJSON, _ := json.MarshalIndent(datafile, "", "  ")
						fmt.Println(string(datafileJSON))
						fmt.Println("")
					}

					// If "at" parameter is provided, create a new instance with the specific hook
					if _, hasAt := assertionMap["at"]; hasAt {
						datafile := datafilesByEnvironment[environment]
						var datafileContent sdk.DatafileContent
						if datafileBytes, err := json.Marshal(datafile); err == nil {
							json.Unmarshal(datafileBytes, &datafileContent)
						}

						levelStr := sdk.LogLevel(level)
						instance = sdk.CreateInstance(sdk.InstanceOptions{
							Datafile: datafileContent,
							LogLevel: &levelStr,
							Hooks: []*sdk.Hook{
								{
									Name: "tester-hook",
									BucketValue: func(options sdk.ConfigureBucketValueOptions) int {
										if at, ok := assertionMap["at"].(float64); ok {
											// Match JavaScript implementation exactly: assertion.at * (MAX_BUCKETED_NUMBER / 100)
											// MAX_BUCKETED_NUMBER is 100000, so this gives us 0-100000 range
											// The JavaScript version uses: assertion.at * (MAX_BUCKETED_NUMBER / 100)
											// where MAX_BUCKETED_NUMBER = 100000, so this becomes assertion.at * 1000
											return int(at * 1000)
										}
										return options.BucketValue
									},
								},
							},
						})
					}

					testResult = RunTestFeature(assertionMap, test["feature"].(string), instance, level)
				} else if _, hasSegment := test["segment"]; hasSegment {
					segmentKey := test["segment"].(string)
					segment := segmentsByKey[segmentKey]
					if segmentMap, ok := segment.(map[string]interface{}); ok {
						testResult = RunTestSegment(assertionMap, segmentMap, level)
					}
				}

				testDuration += testResult.Duration

				if testResult.HasError {
					results += fmt.Sprintf("  ✘ %s (%.2fms)\n", assertionMap["description"], testResult.Duration*1000)
					results += testResult.Errors
					testHasError = true
					failedAssertionsCount++
				} else {
					results += fmt.Sprintf("  ✔ %s (%.2fms)\n", assertionMap["description"], testResult.Duration*1000)
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

func convertToStringSlice(interfaces []interface{}) []string {
	strings := make([]string, len(interfaces))
	for i, v := range interfaces {
		strings[i] = v.(string)
	}
	return strings
}
