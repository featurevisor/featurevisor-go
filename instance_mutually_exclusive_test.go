package featurevisor

import (
	"testing"
)

func TestMutuallyExclusiveFeatures(t *testing.T) {
	var bucketValue int = 10000

	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"mutex": {
				"key": "mutex",
				"bucketBy": "userId",
				"ranges": [[0, 50000]],
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 50000,
						"allocation": []
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
		Hooks: []*Hook{
			{
				Name: "unit-test",
				BucketValue: func(options ConfigureBucketValueOptions) int {
					return bucketValue
				},
			},
		},
		Datafile: datafile,
	})

	// Should be disabled for non-existent features
	if sdk.IsEnabled("test", Context{}, OverrideOptions{}) {
		t.Error("Expected non-existent feature to be disabled")
	}

	if sdk.IsEnabled("test", Context{"userId": "123"}, OverrideOptions{}) {
		t.Error("Expected non-existent feature to be disabled even with context")
	}

	// Test with bucket value 40000 (should be enabled)
	bucketValue = 40000
	if !sdk.IsEnabled("mutex", Context{"userId": "123"}, OverrideOptions{}) {
		t.Error("Expected mutex feature to be enabled with bucket value 40000")
	}

	// Test with bucket value 60000 (should be disabled)
	bucketValue = 60000
	if sdk.IsEnabled("mutex", Context{"userId": "123"}, OverrideOptions{}) {
		t.Error("Expected mutex feature to be disabled with bucket value 60000")
	}
}
