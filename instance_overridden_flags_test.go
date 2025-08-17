package featurevisor

import (
	"testing"
)

func TestOverriddenFlagsFromRules(t *testing.T) {
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"test": {
				"key": "test",
				"bucketBy": "userId",
				"traffic": [
					{
						"key": "2",
						"segments": "netherlands",
						"percentage": 100000,
						"enabled": false,
						"allocation": []
					},
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

	sdk := CreateInstance(InstanceOptions{
		Datafile: datafile,
	})

	// Should be enabled for German users (no segment match)
	if !sdk.IsEnabled("test", Context{"userId": "user-123", "country": "de"}, OverrideOptions{}) {
		t.Error("Expected feature to be enabled for German users")
	}

	// Should be disabled for Dutch users (segment match with enabled: false)
	if sdk.IsEnabled("test", Context{"userId": "user-123", "country": "nl"}, OverrideOptions{}) {
		t.Error("Expected feature to be disabled for Dutch users")
	}
}
