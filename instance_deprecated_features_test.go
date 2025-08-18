package featurevisor

import (
	"strings"
	"testing"
)

func TestDeprecatedFeatures(t *testing.T) {
	var deprecatedCount int
	var capturedLogs []string

	// Create a custom logger to capture warnings
	level := LogLevelWarn
	handler := LogHandler(func(level LogLevel, message LogMessage, details LogDetails) {
		if level == LogLevelWarn && strings.Contains(string(message), "is deprecated") {
			deprecatedCount++
		}
		capturedLogs = append(capturedLogs, string(message))
	})

	customLogger := NewLogger(CreateLoggerOptions{
		Level:   &level,
		Handler: &handler,
	})

	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"test": {
				"key": "test",
				"bucketBy": "userId",
				"variations": [
					{"value": "control"},
					{"value": "treatment"}
				],
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 100000,
						"allocation": [
							{"variation": "control", "range": [0, 100000]},
							{"variation": "treatment", "range": [0, 0]}
						]
					}
				]
			},
			"deprecatedTest": {
				"key": "deprecatedTest",
				"deprecated": true,
				"bucketBy": "userId",
				"variations": [
					{"value": "control"},
					{"value": "treatment"}
				],
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 100000,
						"allocation": [
							{"variation": "control", "range": [0, 100000]},
							{"variation": "treatment", "range": [0, 0]}
						]
					}
				]
			}
		},
		"segments": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	sdk := CreateInstance(Options{
		Datafile: datafile,
		Logger:   customLogger,
	})

	context := Context{"userId": "123"}

	testVariation := sdk.GetVariation("test", context, OverrideOptions{})
	deprecatedTestVariation := sdk.GetVariation("deprecatedTest", context, OverrideOptions{})

	if testVariation == nil || *testVariation != "control" {
		t.Errorf("Expected test variation to be 'control', got '%v'", testVariation)
	}

	if deprecatedTestVariation == nil || *deprecatedTestVariation != "control" {
		t.Errorf("Expected deprecated test variation to be 'control', got '%v'", deprecatedTestVariation)
	}

	if deprecatedCount != 1 {
		t.Errorf("Expected 1 deprecated warning, got %d", deprecatedCount)
	}

	// Check that the warning message contains the expected content
	foundDeprecatedWarning := false
	for _, log := range capturedLogs {
		if strings.Contains(log, "deprecated") {
			foundDeprecatedWarning = true
			break
		}
	}

	if !foundDeprecatedWarning {
		t.Error("Expected to find deprecated warning in logs")
	}
}

// Helper function to create bool pointers
func boolPtr(b bool) *bool {
	return &b
}
