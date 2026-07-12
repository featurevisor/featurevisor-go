package featurevisor

import (
	"testing"
)

func TestDeprecatedFeatures(t *testing.T) {
	var deprecatedCount int
	level := LogLevelWarn

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

	sdk := CreateFeaturevisor(FeaturevisorOptions{
		Datafile: datafile,
		LogLevel: &level,
		OnDiagnostic: func(diagnostic FeaturevisorDiagnostic) {
			if diagnostic.Code == "deprecated_feature" {
				deprecatedCount++
			}
		},
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

}

// Helper function to create bool pointers
func boolPtr(b bool) *bool {
	return &b
}
