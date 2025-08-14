package sdk

import (
	"testing"
)

func TestVariablesWithoutVariations(t *testing.T) {
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"segments": {
			"netherlands": {
				"key": "netherlands",
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"nl\"}]"
			}
		},
		"features": {
			"test": {
				"key": "test",
				"bucketBy": "userId",
				"variablesSchema": {
					"color": {
						"key": "color",
						"type": "string",
						"defaultValue": "red"
					}
				},
				"traffic": [
					{
						"key": "1",
						"segments": "netherlands",
						"percentage": 100000,
						"variables": {
							"color": "orange"
						},
						"allocation": []
					},
					{
						"key": "2",
						"segments": "*",
						"percentage": 100000,
						"allocation": []
					}
				]
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

	defaultContext := Context{"userId": "123"}

	// Test default value
	color := sdk.GetVariable("test", "color", defaultContext, OverrideOptions{})
	if color != "red" {
		t.Errorf("Expected color to be 'red' for default context, got '%v'", color)
	}

	// Test override for Dutch users
	color = sdk.GetVariable("test", "color", Context{"userId": "123", "country": "nl"}, OverrideOptions{})
	if color != "orange" {
		t.Errorf("Expected color to be 'orange' for Dutch users, got '%v'", color)
	}
}
