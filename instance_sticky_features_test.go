package featurevisor

import (
	"testing"
	"time"
)

func TestStickyFeaturesInitialization(t *testing.T) {
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
							{"variation": "control", "range": [0, 0]},
							{"variation": "treatment", "range": [0, 100000]}
						]
					}
				]
			}
		},
		"segments": {}
	}`

	var datafileContent DatafileContent
	if err := datafileContent.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	// Create instance with sticky features and datafile
	instance := CreateInstance(Options{
		Datafile: datafileContent,
		Sticky: &StickyFeatures{
			"test": EvaluatedFeature{
				Enabled:   true,
				Variation: stringPtr("control"),
				Variables: map[VariableKey]VariableValue{
					"color": "red",
				},
			},
		},
	})

	context := Context{"userId": "123"}

	// Initially should be control due to sticky features
	variation := instance.GetVariation("test", context, OverrideOptions{})
	if variation == nil || *variation != "control" {
		t.Errorf("Expected variation to be 'control', got '%v'", variation)
	}

	// Should get sticky variable
	variable := instance.GetVariable("test", "color", context, OverrideOptions{})
	if variable != "red" {
		t.Errorf("Expected variable to be 'red', got '%v'", variable)
	}

	// Set a new datafile with different traffic allocation
	newJsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.1",
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
							{"variation": "control", "range": [0, 0]},
							{"variation": "treatment", "range": [0, 100000]}
						]
					}
				]
			}
		},
		"segments": {}
	}`

	var newDatafileContent DatafileContent
	if err := newDatafileContent.FromJSON(newJsonDatafile); err != nil {
		t.Fatalf("Failed to parse new datafile JSON: %v", err)
	}

	instance.SetDatafile(newDatafileContent)

	// Wait a bit for async operations (if any)
	time.Sleep(100 * time.Millisecond)

	// Should still be control after setting datafile due to sticky features
	variation = instance.GetVariation("test", context, OverrideOptions{})
	if variation == nil || *variation != "control" {
		t.Errorf("Expected variation to still be 'control' after datafile set, got '%v'", variation)
	}

	// Unset sticky features
	instance.SetSticky(StickyFeatures{}, true)

	// Should now be treatment (from datafile)
	variation = instance.GetVariation("test", context, OverrideOptions{})
	if variation == nil || *variation != "treatment" {
		t.Errorf("Expected variation to be 'treatment' after unsetting sticky, got '%v'", variation)
	}
}

// TestSetStickyVariadicSignature tests that the new variadic signature works correctly
func TestSetStickyVariadicSignature(t *testing.T) {
	instance := CreateInstance(Options{})

	// Test calling without replace parameter (should default to false)
	sticky1 := StickyFeatures{"test1": EvaluatedFeature{Enabled: true}}
	instance.SetSticky(sticky1)

	// Test calling with replace parameter
	sticky2 := StickyFeatures{"test2": EvaluatedFeature{Enabled: false}}
	instance.SetSticky(sticky2, true)

	// Verify that the second call replaced the first (since replace=true)
	if instance.sticky == nil {
		t.Fatal("Expected sticky features to be set")
	}

	if len(*instance.sticky) != 1 {
		t.Errorf("Expected 1 sticky feature, got %d", len(*instance.sticky))
	}

	if _, exists := (*instance.sticky)["test2"]; !exists {
		t.Error("Expected 'test2' to exist in sticky features")
	}

	if _, exists := (*instance.sticky)["test1"]; exists {
		t.Error("Expected 'test1' to not exist in sticky features (should have been replaced)")
	}
}
