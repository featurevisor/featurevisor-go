package featurevisor

import (
	"testing"
)

func TestVariables(t *testing.T) {
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"test": {
				"key": "test",
				"bucketBy": "userId",
				"variablesSchema": {
					"color": {
						"key": "color",
						"type": "string",
						"defaultValue": "red"
					},
					"showSidebar": {
						"key": "showSidebar",
						"type": "boolean",
						"defaultValue": false
					},
					"sidebarTitle": {
						"key": "sidebarTitle",
						"type": "string",
						"defaultValue": "sidebar title"
					},
					"count": {
						"key": "count",
						"type": "integer",
						"defaultValue": 0
					},
					"price": {
						"key": "price",
						"type": "double",
						"defaultValue": 9.99
					},
					"flatConfig": {
						"key": "flatConfig",
						"type": "object",
						"defaultValue": {"key": "value"}
					},
					"nestedConfig": {
						"key": "nestedConfig",
						"type": "json",
						"defaultValue": "{\"key\":{\"nested\":\"value\"}}"
					}
				},
				"variations": [
					{"value": "control"},
					{
						"value": "treatment",
						"variables": {
							"showSidebar": true,
							"sidebarTitle": "sidebar title from variation"
						}
					}
				],
				"force": [
					{
						"conditions": "[{\"attribute\":\"userId\",\"operator\":\"equals\",\"value\":\"user-ch\"}]",
						"enabled": true,
						"variation": "control",
						"variables": {
							"color": "red and white"
						}
					},
					{
						"conditions": "[{\"attribute\":\"userId\",\"operator\":\"equals\",\"value\":\"user-gb\"}]",
						"enabled": false
					}
				],
				"traffic": [
					{
						"key": "2",
						"segments": "belgium",
						"percentage": 100000,
						"allocation": [
							{"variation": "control", "range": [0, 0]},
							{"variation": "treatment", "range": [0, 100000]}
						],
						"variation": "control",
						"variables": {
							"color": "black"
						}
					},
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
		"segments": {
			"belgium": {
				"key": "belgium",
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"be\"}]"
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

	// Test basic variable retrieval
	variation := sdk.GetVariation("test", context, OverrideOptions{})
	if variation == nil || *variation != "treatment" {
		t.Errorf("Expected variation to be 'treatment', got '%v'", variation)
	}

	// Test variation for Belgian users
	variation = sdk.GetVariation("test", Context{"userId": "123", "country": "be"}, OverrideOptions{})
	if variation == nil || *variation != "control" {
		t.Errorf("Expected variation to be 'control' for Belgian users, got '%v'", variation)
	}

	// Test variation for Swiss users
	variation = sdk.GetVariation("test", Context{"userId": "user-ch"}, OverrideOptions{})
	if variation == nil || *variation != "control" {
		t.Errorf("Expected variation to be 'control' for Swiss users, got '%v'", variation)
	}

	// Test color variable
	color := sdk.GetVariable("test", "color", context, OverrideOptions{})
	if color != "red" {
		t.Errorf("Expected color to be 'red', got '%v'", color)
	}

	colorStr := sdk.GetVariableString("test", "color", context, OverrideOptions{})
	if colorStr == nil || *colorStr != "red" {
		t.Errorf("Expected color string to be 'red', got '%v'", colorStr)
	}

	// Test color for Belgian users
	color = sdk.GetVariable("test", "color", Context{"userId": "123", "country": "be"}, OverrideOptions{})
	if color != "black" {
		t.Errorf("Expected color to be 'black' for Belgian users, got '%v'", color)
	}

	// Test color for Swiss users
	color = sdk.GetVariable("test", "color", Context{"userId": "user-ch"}, OverrideOptions{})
	if color != "red and white" {
		t.Errorf("Expected color to be 'red and white' for Swiss users, got '%v'", color)
	}

	// Test showSidebar variable
	showSidebar := sdk.GetVariable("test", "showSidebar", context, OverrideOptions{})
	if showSidebar != true {
		t.Errorf("Expected showSidebar to be true, got '%v'", showSidebar)
	}

	showSidebarBool := sdk.GetVariableBoolean("test", "showSidebar", context, OverrideOptions{})
	if showSidebarBool == nil || *showSidebarBool != true {
		t.Errorf("Expected showSidebar boolean to be true, got '%v'", showSidebarBool)
	}

	// Test sidebarTitle variable
	sidebarTitle := sdk.GetVariable("test", "sidebarTitle", context, OverrideOptions{})
	if sidebarTitle != "sidebar title from variation" {
		t.Errorf("Expected sidebarTitle to be 'sidebar title from variation', got '%v'", sidebarTitle)
	}

	sidebarTitleStr := sdk.GetVariableString("test", "sidebarTitle", context, OverrideOptions{})
	if sidebarTitleStr == nil || *sidebarTitleStr != "sidebar title from variation" {
		t.Errorf("Expected sidebarTitle string to be 'sidebar title from variation', got '%v'", sidebarTitleStr)
	}

	// Test count variable
	count := sdk.GetVariable("test", "count", context, OverrideOptions{})
	if count != 0 {
		t.Errorf("Expected count to be 0, got '%v'", count)
	}

	countInt := sdk.GetVariableInteger("test", "count", context, OverrideOptions{})
	if countInt == nil || *countInt != 0 {
		t.Errorf("Expected count integer to be 0, got '%v'", countInt)
	}

	// Test price variable
	price := sdk.GetVariable("test", "price", context, OverrideOptions{})
	if price != 9.99 {
		t.Errorf("Expected price to be 9.99, got '%v'", price)
	}

	priceFloat := sdk.GetVariableDouble("test", "price", context, OverrideOptions{})
	if priceFloat == nil || *priceFloat != 9.99 {
		t.Errorf("Expected price double to be 9.99, got '%v'", priceFloat)
	}

	// Test flatConfig variable
	flatConfig := sdk.GetVariable("test", "flatConfig", context, OverrideOptions{})
	if flatConfig == nil {
		t.Error("Expected flatConfig to not be nil")
	} else {
		config, ok := flatConfig.(map[string]interface{})
		if !ok || config["key"] != "value" {
			t.Errorf("Expected flatConfig to be {'key': 'value'}, got '%v'", flatConfig)
		}
	}

	flatConfigObj := sdk.GetVariableObject("test", "flatConfig", context, OverrideOptions{})
	if flatConfigObj == nil || flatConfigObj["key"] != "value" {
		t.Errorf("Expected flatConfig object to be {'key': 'value'}, got '%v'", flatConfigObj)
	}

	// Test nestedConfig variable
	nestedConfig := sdk.GetVariable("test", "nestedConfig", context, OverrideOptions{})
	if nestedConfig == nil {
		t.Error("Expected nestedConfig to not be nil")
	} else {
		config, ok := nestedConfig.(map[string]interface{})
		if !ok {
			t.Errorf("Expected nestedConfig to be a map, got '%v'", nestedConfig)
		} else {
			key, ok := config["key"].(map[string]interface{})
			if !ok || key["nested"] != "value" {
				t.Errorf("Expected nestedConfig to have nested structure, got '%v'", nestedConfig)
			}
		}
	}

	nestedConfigJSON := sdk.GetVariableJSON("test", "nestedConfig", context, OverrideOptions{})
	if nestedConfigJSON == nil {
		t.Error("Expected nestedConfig JSON to not be nil")
	} else {
		config, ok := nestedConfigJSON.(map[string]interface{})
		if !ok {
			t.Errorf("Expected nestedConfig JSON to be a map, got '%v'", nestedConfigJSON)
		} else {
			key, ok := config["key"].(map[string]interface{})
			if !ok || key["nested"] != "value" {
				t.Errorf("Expected nestedConfig JSON to have nested structure, got '%v'", nestedConfigJSON)
			}
		}
	}

	// Test non-existing variable
	nonExisting := sdk.GetVariable("test", "nonExisting", context, OverrideOptions{})
	if nonExisting != nil {
		t.Errorf("Expected non-existing variable to be nil, got '%v'", nonExisting)
	}

	// Test non-existing feature
	nonExistingFeature := sdk.GetVariable("nonExistingFeature", "nonExisting", context, OverrideOptions{})
	if nonExistingFeature != nil {
		t.Errorf("Expected non-existing feature variable to be nil, got '%v'", nonExistingFeature)
	}

	// Test disabled feature
	disabledFeature := sdk.GetVariable("test", "color", Context{"userId": "user-gb"}, OverrideOptions{})
	if disabledFeature != nil {
		t.Errorf("Expected disabled feature variable to be nil, got '%v'", disabledFeature)
	}
}
