package featurevisor

import (
	"testing"
)

func TestIndividuallyNamedSegments(t *testing.T) {
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"test": {
				"key": "test",
				"bucketBy": "userId",
				"traffic": [
					{
						"key": "1",
						"segments": "netherlands",
						"percentage": 100000,
						"allocation": []
					},
					{
						"key": "2",
						"segments": "[\"iphone\",\"unitedStates\"]",
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
			},
			"iphone": {
				"key": "iphone",
				"conditions": "[{\"attribute\":\"device\",\"operator\":\"equals\",\"value\":\"iphone\"}]"
			},
			"unitedStates": {
				"key": "unitedStates",
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"us\"}]"
			}
		}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	sdk := CreateInstance(InstanceOptions{
		Datafile: datafile,
	})

	// Should be disabled for no context
	if sdk.IsEnabled("test", Context{}, OverrideOptions{}) {
		t.Error("Expected feature to be disabled for no context")
	}

	// Should be disabled for users without matching segments
	if sdk.IsEnabled("test", Context{"userId": "123"}, OverrideOptions{}) {
		t.Error("Expected feature to be disabled for users without matching segments")
	}

	// Should be disabled for German users
	if sdk.IsEnabled("test", Context{"userId": "123", "country": "de"}, OverrideOptions{}) {
		t.Error("Expected feature to be disabled for German users")
	}

	// Should be disabled for US users without iPhone
	if sdk.IsEnabled("test", Context{"userId": "123", "country": "us"}, OverrideOptions{}) {
		t.Error("Expected feature to be disabled for US users without iPhone")
	}

	// Should be enabled for Dutch users
	if !sdk.IsEnabled("test", Context{"userId": "123", "country": "nl"}, OverrideOptions{}) {
		t.Error("Expected feature to be enabled for Dutch users")
	}

	// Should be enabled for US users with iPhone
	if !sdk.IsEnabled("test", Context{"userId": "123", "country": "us", "device": "iphone"}, OverrideOptions{}) {
		t.Error("Expected feature to be enabled for US users with iPhone")
	}
}
