package featurevisor

import (
	"testing"
)

func TestRequiredFeaturesSimple(t *testing.T) {
	// Test that feature is disabled because required is disabled
	jsonDatafile1 := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"requiredKey": {
				"key": "requiredKey",
				"bucketBy": "userId",
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 0,
						"allocation": []
					}
				]
			},
			"myKey": {
				"key": "myKey",
				"bucketBy": "userId",
				"required": ["requiredKey"],
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 100000,
						"allocation": []
					}
				]
			}
		},
		"segments": {}
	}`

	var datafile1 DatafileContent
	if err := datafile1.FromJSON(jsonDatafile1); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	sdk := CreateInstance(Options{
		Datafile: datafile1,
	})

	// Should be disabled because required is disabled
	if sdk.IsEnabled("myKey", Context{}, OverrideOptions{}) {
		t.Error("Expected feature to be disabled because required feature is disabled")
	}

	// Test that feature is enabled when required is enabled
	jsonDatafile2 := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"requiredKey": {
				"key": "requiredKey",
				"bucketBy": "userId",
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 100000,
						"allocation": []
					}
				]
			},
			"myKey": {
				"key": "myKey",
				"bucketBy": "userId",
				"required": ["requiredKey"],
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 100000,
						"allocation": []
					}
				]
			}
		},
		"segments": {}
	}`

	var datafile2 DatafileContent
	if err := datafile2.FromJSON(jsonDatafile2); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	sdk2 := CreateInstance(Options{
		Datafile: datafile2,
	})

	if !sdk2.IsEnabled("myKey", Context{}, OverrideOptions{}) {
		t.Error("Expected feature to be enabled when required feature is enabled")
	}
}

func TestRequiredFeaturesWithVariation(t *testing.T) {
	// Test that feature is disabled because required has different variation
	jsonDatafile1 := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"requiredKey": {
				"key": "requiredKey",
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
							{"variation": "control", "range": [0, 0]},
							{"variation": "treatment", "range": [0, 100000]}
						]
					}
				]
			},
			"myKey": {
				"key": "myKey",
				"bucketBy": "userId",
				"required": [
					{
						"key": "requiredKey",
						"variation": "control"
					}
				],
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 100000,
						"allocation": []
					}
				]
			}
		},
		"segments": {}
	}`

	var datafile1 DatafileContent
	if err := datafile1.FromJSON(jsonDatafile1); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	sdk := CreateInstance(Options{
		Datafile: datafile1,
	})

	if sdk.IsEnabled("myKey", Context{}, OverrideOptions{}) {
		t.Error("Expected feature to be disabled because required has different variation")
	}

	// Test that feature is enabled when required has desired variation
	jsonDatafile2 := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"requiredKey": {
				"key": "requiredKey",
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
							{"variation": "control", "range": [0, 0]},
							{"variation": "treatment", "range": [0, 100000]}
						]
					}
				]
			},
			"myKey": {
				"key": "myKey",
				"bucketBy": "userId",
				"required": [
					{
						"key": "requiredKey",
						"variation": "treatment"
					}
				],
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 100000,
						"allocation": []
					}
				]
			}
		},
		"segments": {}
	}`

	var datafile2 DatafileContent
	if err := datafile2.FromJSON(jsonDatafile2); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	sdk2 := CreateInstance(Options{
		Datafile: datafile2,
	})

	if !sdk2.IsEnabled("myKey", Context{}, OverrideOptions{}) {
		t.Error("Expected feature to be enabled when required has desired variation")
	}
}
