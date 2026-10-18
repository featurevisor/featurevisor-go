package featurevisor

import "testing"

func TestNumberAndEarlierDoubleVariablesReadTheSame(t *testing.T) {
	jsonDatafile := `{
		"schemaVersion": "4",
		"revision": "1",
		"segments": {},
		"features": {},
		"variables": {
			"rate": { "type": "number", "defaultValue": 2.5 },
			"ratio": { "type": "double", "defaultValue": 1.5 }
		}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	sdk := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile})
	defer sdk.Close()

	for key, expected := range map[string]float64{"rate": 2.5, "ratio": 1.5} {
		value := sdk.GetGlobalVariableDouble(key)
		if value == nil || *value != expected {
			t.Errorf("Expected %s to be %v, got %v", key, expected, value)
		}
	}
}
