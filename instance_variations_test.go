package featurevisor

import (
	"testing"
)

func TestVariationsWithForceRules(t *testing.T) {
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
				"force": [
					{
						"conditions": "[{\"attribute\":\"userId\",\"operator\":\"equals\",\"value\":\"user-gb\"}]",
						"enabled": false
					},
					{
						"segments": "netherlands",
						"enabled": false
					}
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
			"testWithNoVariation": {
				"key": "testWithNoVariation",
				"bucketBy": "userId",
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
		"segments": {
			"netherlands": {
				"key": "netherlands",
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"nl\"}]"
			}
		}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	sdk := CreateInstance(Options{
		Datafile: datafile,
	})

	context := Context{"userId": "123"}

	// Should get treatment variation for normal case
	variation := sdk.GetVariation("test", context, OverrideOptions{})
	if variation == nil || *variation != "treatment" {
		t.Errorf("Expected variation to be 'treatment', got '%v'", variation)
	}

	// Should get treatment variation for Swiss users
	variation = sdk.GetVariation("test", Context{"userId": "user-ch"}, OverrideOptions{})
	if variation == nil || *variation != "treatment" {
		t.Errorf("Expected variation to be 'treatment' for Swiss users, got '%v'", variation)
	}

	// Should return nil for non-existing feature
	variation = sdk.GetVariation("nonExistingFeature", context, OverrideOptions{})
	if variation != nil {
		t.Errorf("Expected variation to be nil for non-existing feature, got '%v'", variation)
	}

	// Should return nil for disabled feature (user-gb)
	variation = sdk.GetVariation("test", Context{"userId": "user-gb"}, OverrideOptions{})
	if variation != nil {
		t.Errorf("Expected variation to be nil for disabled feature (user-gb), got '%v'", variation)
	}

	// Should return nil for disabled feature (Dutch users)
	variation = sdk.GetVariation("test", Context{"userId": "123", "country": "nl"}, OverrideOptions{})
	if variation != nil {
		t.Errorf("Expected variation to be nil for disabled feature (Dutch users), got '%v'", variation)
	}

	// Should return nil for feature with no variations
	variation = sdk.GetVariation("testWithNoVariation", context, OverrideOptions{})
	if variation != nil {
		t.Errorf("Expected variation to be nil for feature with no variations, got '%v'", variation)
	}
}
