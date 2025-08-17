package featurevisor

import (
	"testing"
)

func TestProductionTagAllDatafile(t *testing.T) {
	// Production featurevisor-tag-all.json datafile
	jsonData := `{
		"schemaVersion": "2",
		"revision": "1",
		"segments": {
			"blackFridayWeekend": {
				"conditions": "{\"and\":[{\"attribute\":\"date\",\"operator\":\"after\",\"value\":\"2023-11-24T00:00:00.000Z\"},{\"attribute\":\"date\",\"operator\":\"before\",\"value\":\"2023-11-27T00:00:00.000Z\"}]}"
			},
			"countries/germany": {
				"conditions": "{\"and\":[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"de\"}]}"
			},
			"countries/netherlands": {
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"nl\"}]"
			},
			"countries/switzerland": {
				"conditions": "{\"and\":[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"ch\"}]}"
			},
			"desktop": {
				"conditions": "[{\"attribute\":\"device\",\"operator\":\"equals\",\"value\":\"desktop\"}]"
			},
			"everyone": {
				"conditions": "*"
			},
			"mobile": {
				"conditions": "{\"and\":[{\"attribute\":\"device\",\"operator\":\"equals\",\"value\":\"mobile\"},{\"attribute\":\"phone\",\"operator\":\"notExists\"}]}"
			},
			"version_gt5": {
				"conditions": "[{\"attribute\":\"version\",\"operator\":\"semverGreaterThan\",\"value\":\"5.0.0\"}]"
			}
		},
		"features": {
			"allowSignup": {
				"bucketBy": "deviceId",
				"variations": [
					{
						"value": "control",
						"weight": 50
					},
					{
						"value": "treatment",
						"weight": 50,
						"variables": {
							"allowGoogleSignUp": true,
							"allowGitHubSignUp": true
						}
					}
				],
				"traffic": [
					{
						"key": "nl",
						"segments": "[\"countries/netherlands\"]",
						"percentage": 100000,
						"allocation": [
							{
								"variation": "control",
								"range": [0, 50000]
							},
							{
								"variation": "treatment",
								"range": [50000, 100000]
							}
						],
						"variation": "treatment"
					},
					{
						"key": "ch",
						"segments": "[\"countries/switzerland\"]",
						"percentage": 100000,
						"allocation": [
							{
								"variation": "control",
								"range": [0, 10000]
							},
							{
								"variation": "treatment",
								"range": [10000, 100000]
							}
						],
						"variationWeights": {
							"control": 10,
							"treatment": 90
						}
					},
					{
						"key": "everyone",
						"segments": "everyone",
						"percentage": 100000,
						"allocation": [
							{
								"variation": "control",
								"range": [0, 50000]
							},
							{
								"variation": "treatment",
								"range": [50000, 100000]
							}
						]
					}
				],
				"variablesSchema": {
					"allowRegularSignUp": {
						"type": "boolean",
						"defaultValue": true
					},
					"allowGoogleSignUp": {
						"type": "boolean",
						"defaultValue": false
					},
					"allowGitHubSignUp": {
						"type": "boolean",
						"defaultValue": false
					}
				},
				"hash": "8ZwSp88Vqf"
			}
		}
	}`

	var datafile DatafileContent
	err := datafile.FromJSON(jsonData)
	if err != nil {
		t.Fatalf("Failed to parse production tag-all datafile: %v", err)
	}

	// Assertions
	if datafile.SchemaVersion != "2" {
		t.Errorf("Expected schemaVersion to be '2', got '%s'", datafile.SchemaVersion)
	}

	if datafile.Revision != "1" {
		t.Errorf("Expected revision to be '1', got '%s'", datafile.Revision)
	}

	// Check segments
	expectedSegments := []string{
		"blackFridayWeekend", "countries/germany", "countries/netherlands",
		"countries/switzerland", "desktop", "everyone", "mobile", "version_gt5",
	}

	if len(datafile.Segments) != len(expectedSegments) {
		t.Errorf("Expected %d segments, got %d", len(expectedSegments), len(datafile.Segments))
	}

	for _, segmentKey := range expectedSegments {
		if _, exists := datafile.Segments[segmentKey]; !exists {
			t.Errorf("Expected segment '%s' to exist", segmentKey)
		}
	}

	// Check features
	if len(datafile.Features) != 1 {
		t.Errorf("Expected 1 feature, got %d", len(datafile.Features))
	}

	allowSignupFeature, exists := datafile.Features["allowSignup"]
	if !exists {
		t.Fatal("Expected 'allowSignup' feature to exist")
	}

	// Verify feature structure
	if allowSignupFeature.BucketBy != "deviceId" {
		t.Errorf("Expected bucketBy to be 'deviceId', got '%v'", allowSignupFeature.BucketBy)
	}

	if len(allowSignupFeature.Variations) != 2 {
		t.Errorf("Expected 2 variations, got %d", len(allowSignupFeature.Variations))
	}

	if len(allowSignupFeature.Traffic) != 3 {
		t.Errorf("Expected 3 traffic rules, got %d", len(allowSignupFeature.Traffic))
	}

	// Verify hash
	if allowSignupFeature.Hash == nil || *allowSignupFeature.Hash != "8ZwSp88Vqf" {
		t.Errorf("Expected hash to be '8ZwSp88Vqf', got '%v'", allowSignupFeature.Hash)
	}
}

func TestProductionTagCheckoutDatafile(t *testing.T) {
	// Production featurevisor-tag-checkout.json datafile
	jsonData := `{
		"schemaVersion": "2",
		"revision": "1",
		"segments": {
			"blackFridayWeekend": {
				"conditions": "{\"and\":[{\"attribute\":\"date\",\"operator\":\"after\",\"value\":\"2023-11-24T00:00:00.000Z\"},{\"attribute\":\"date\",\"operator\":\"before\",\"value\":\"2023-11-27T00:00:00.000Z\"}]}"
			},
			"countries/germany": {
				"conditions": "{\"and\":[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"de\"}]}"
			}
		},
		"features": {
			"discount": {
				"bucketBy": "userId",
				"required": ["sidebar"],
				"traffic": [
					{
						"key": "2",
						"segments": "[\"blackFridayWeekend\"]",
						"percentage": 100000
					},
					{
						"key": "1",
						"segments": "*",
						"percentage": 0
					}
				],
				"hash": "7Z4IN6SGV0"
			},
			"pricing": {
				"bucketBy": "userId",
				"disabledVariationValue": "control",
				"variations": [
					{
						"value": "control",
						"weight": 0
					},
					{
						"value": "treatment",
						"weight": 100
					}
				],
				"traffic": [
					{
						"key": "1",
						"segments": "countries/germany",
						"percentage": 100000,
						"allocation": [
							{
								"variation": "treatment",
								"range": [0, 100000]
							}
						]
					},
					{
						"key": "2",
						"segments": "*",
						"percentage": 0
					}
				],
				"hash": "FbJugwGcmm"
			},
			"showBanner": {
				"bucketBy": "userId",
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 0
					}
				],
				"hash": "9CrWAu7UWF"
			}
		}
	}`

	var datafile DatafileContent
	err := datafile.FromJSON(jsonData)
	if err != nil {
		t.Fatalf("Failed to parse production tag-checkout datafile: %v", err)
	}

	// Assertions
	if datafile.SchemaVersion != "2" {
		t.Errorf("Expected schemaVersion to be '2', got '%s'", datafile.SchemaVersion)
	}

	if datafile.Revision != "1" {
		t.Errorf("Expected revision to be '1', got '%s'", datafile.Revision)
	}

	// Check segments
	expectedSegments := []string{"blackFridayWeekend", "countries/germany"}
	if len(datafile.Segments) != len(expectedSegments) {
		t.Errorf("Expected %d segments, got %d", len(expectedSegments), len(datafile.Segments))
	}

	// Check features
	expectedFeatures := []string{"discount", "pricing", "showBanner"}
	if len(datafile.Features) != len(expectedFeatures) {
		t.Errorf("Expected %d features, got %d", len(expectedFeatures), len(datafile.Features))
	}

	// Verify discount feature
	discountFeature, exists := datafile.Features["discount"]
	if !exists {
		t.Fatal("Expected 'discount' feature to exist")
	}

	if discountFeature.BucketBy != "userId" {
		t.Errorf("Expected bucketBy to be 'userId', got '%v'", discountFeature.BucketBy)
	}

	if len(discountFeature.Required) != 1 {
		t.Errorf("Expected 1 required feature, got %d", len(discountFeature.Required))
	}

	if discountFeature.Hash == nil || *discountFeature.Hash != "7Z4IN6SGV0" {
		t.Errorf("Expected hash to be '7Z4IN6SGV0', got '%v'", discountFeature.Hash)
	}

	// Verify pricing feature
	pricingFeature, exists := datafile.Features["pricing"]
	if !exists {
		t.Fatal("Expected 'pricing' feature to exist")
	}

	if pricingFeature.DisabledVariationValue == nil || *pricingFeature.DisabledVariationValue != "control" {
		t.Errorf("Expected disabledVariationValue to be 'control', got '%v'", pricingFeature.DisabledVariationValue)
	}

	if len(pricingFeature.Variations) != 2 {
		t.Errorf("Expected 2 variations, got %d", len(pricingFeature.Variations))
	}

	if pricingFeature.Hash == nil || *pricingFeature.Hash != "FbJugwGcmm" {
		t.Errorf("Expected hash to be 'FbJugwGcmm', got '%v'", pricingFeature.Hash)
	}
}

func TestStagingTagAllDatafile(t *testing.T) {
	// Staging featurevisor-tag-all.json datafile
	jsonData := `{
		"schemaVersion": "2",
		"revision": "1",
		"segments": {
			"countries/germany": {
				"conditions": "{\"and\":[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"de\"}]}"
			},
			"countries/netherlands": {
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"nl\"}]"
			},
			"countries/switzerland": {
				"conditions": "{\"and\":[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"ch\"}]}"
			},
			"qa": {
				"conditions": "[{\"attribute\":\"userId\",\"operator\":\"in\",\"value\":[\"user-1\",\"user-2\"]}]"
			},
			"version_5.5": {
				"conditions": "[{\"or\":[{\"attribute\":\"version\",\"operator\":\"equals\",\"value\":5.5},{\"attribute\":\"version\",\"operator\":\"equals\",\"value\":\"5.5\"}]}]"
			}
		},
		"features": {
			"allowSignup": {
				"bucketBy": "deviceId",
				"variations": [
					{
						"value": "control",
						"weight": 50
					},
					{
						"value": "treatment",
						"weight": 50,
						"variables": {
							"allowGoogleSignUp": true,
							"allowGitHubSignUp": true
						}
					}
				],
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 100000,
						"allocation": [
							{
								"variation": "control",
								"range": [0, 50000]
							},
							{
								"variation": "treatment",
								"range": [50000, 100000]
							}
						]
					}
				],
				"variablesSchema": {
					"allowRegularSignUp": {
						"type": "boolean",
						"defaultValue": true
					},
					"allowGoogleSignUp": {
						"type": "boolean",
						"defaultValue": false
					},
					"allowGitHubSignUp": {
						"type": "boolean",
						"defaultValue": false
					}
				},
				"hash": "zX8bZtkm5V"
			},
			"bar": {
				"bucketBy": "userId",
				"variations": [
					{
						"value": "control",
						"weight": 33
					},
					{
						"value": "b",
						"weight": 33,
						"variables": {
							"hero": {
								"title": "Hero Title for B",
								"subtitle": "Hero Subtitle for B",
								"alignment": "center for B"
							}
						},
						"variableOverrides": {
							"hero": [
								{
									"segments": "{\"or\":[\"countries/germany\",\"countries/switzerland\"]}",
									"value": {
										"title": "Hero Title for B in DE or CH",
										"subtitle": "Hero Subtitle for B in DE of CH",
										"alignment": "center for B in DE or CH"
									}
								}
							]
						}
					},
					{
						"value": "c",
						"weight": 34
					}
				],
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 50000,
						"allocation": [
							{
								"variation": "control",
								"range": [0, 16500]
							},
							{
								"variation": "b",
								"range": [16500, 33000]
							},
							{
								"variation": "c",
								"range": [33000, 50000]
							}
						]
					}
				],
				"ranges": [[0, 100000]]
			}
		}
	}`

	var datafile DatafileContent
	err := datafile.FromJSON(jsonData)
	if err != nil {
		t.Fatalf("Failed to parse staging tag-all datafile: %v", err)
	}

	// Assertions
	if datafile.SchemaVersion != "2" {
		t.Errorf("Expected schemaVersion to be '2', got '%s'", datafile.SchemaVersion)
	}

	if datafile.Revision != "1" {
		t.Errorf("Expected revision to be '1', got '%s'", datafile.Revision)
	}

	// Check segments
	expectedSegments := []string{
		"countries/germany", "countries/netherlands", "countries/switzerland", "qa", "version_5.5",
	}

	if len(datafile.Segments) != len(expectedSegments) {
		t.Errorf("Expected %d segments, got %d", len(expectedSegments), len(datafile.Segments))
	}

	// Check features
	expectedFeatures := []string{"allowSignup", "bar"}
	if len(datafile.Features) != len(expectedFeatures) {
		t.Errorf("Expected %d features, got %d", len(expectedFeatures), len(datafile.Features))
	}

	// Verify allowSignup feature
	allowSignupFeature, exists := datafile.Features["allowSignup"]
	if !exists {
		t.Fatal("Expected 'allowSignup' feature to exist")
	}

	if allowSignupFeature.Hash == nil || *allowSignupFeature.Hash != "zX8bZtkm5V" {
		t.Errorf("Expected hash to be 'zX8bZtkm5V', got '%v'", allowSignupFeature.Hash)
	}

	// Verify bar feature
	barFeature, exists := datafile.Features["bar"]
	if !exists {
		t.Fatal("Expected 'bar' feature to exist")
	}

	if len(barFeature.Variations) != 3 {
		t.Errorf("Expected 3 variations, got %d", len(barFeature.Variations))
	}

	if len(barFeature.Ranges) != 1 {
		t.Errorf("Expected 1 range, got %d", len(barFeature.Ranges))
	}

	// Check for variable overrides
	if len(barFeature.Variations) > 1 {
		secondVariation := barFeature.Variations[1]
		if secondVariation.VariableOverrides == nil {
			t.Error("Expected variableOverrides to exist in second variation")
		}
	}
}

func TestStagingTagCheckoutDatafile(t *testing.T) {
	// Staging featurevisor-tag-checkout.json datafile
	jsonData := `{
		"schemaVersion": "2",
		"revision": "1",
		"segments": {
			"qa": {
				"conditions": "[{\"attribute\":\"userId\",\"operator\":\"in\",\"value\":[\"user-1\",\"user-2\"]}]"
			}
		},
		"features": {
			"discount": {
				"bucketBy": "userId",
				"required": ["sidebar"],
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 100000
					}
				],
				"hash": "8PTq2CkDyi"
			},
			"pricing": {
				"bucketBy": "userId",
				"disabledVariationValue": "control",
				"variations": [
					{
						"value": "control",
						"weight": 0
					},
					{
						"value": "treatment",
						"weight": 100
					}
				],
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 100000,
						"allocation": [
							{
								"variation": "treatment",
								"range": [0, 100000]
							}
						]
					}
				],
				"hash": "HGJQdeUNIu"
			},
			"showBanner": {
				"bucketBy": "userId",
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 0
					}
				],
				"force": [
					{
						"segments": "qa",
						"enabled": true
					},
					{
						"conditions": "[{\"attribute\":\"userId\",\"operator\":\"equals\",\"value\":\"user-3\"}]",
						"enabled": true
					}
				],
				"hash": "FaQN9OOIkm"
			}
		}
	}`

	var datafile DatafileContent
	err := datafile.FromJSON(jsonData)
	if err != nil {
		t.Fatalf("Failed to parse staging tag-checkout datafile: %v", err)
	}

	// Assertions
	if datafile.SchemaVersion != "2" {
		t.Errorf("Expected schemaVersion to be '2', got '%s'", datafile.SchemaVersion)
	}

	if datafile.Revision != "1" {
		t.Errorf("Expected revision to be '1', got '%s'", datafile.Revision)
	}

	// Check segments
	expectedSegments := []string{"qa"}
	if len(datafile.Segments) != len(expectedSegments) {
		t.Errorf("Expected %d segments, got %d", len(expectedSegments), len(datafile.Segments))
	}

	// Check features
	expectedFeatures := []string{"discount", "pricing", "showBanner"}
	if len(datafile.Features) != len(expectedFeatures) {
		t.Errorf("Expected %d features, got %d", len(expectedFeatures), len(datafile.Features))
	}

	// Verify discount feature
	discountFeature, exists := datafile.Features["discount"]
	if !exists {
		t.Fatal("Expected 'discount' feature to exist")
	}

	if discountFeature.Hash == nil || *discountFeature.Hash != "8PTq2CkDyi" {
		t.Errorf("Expected hash to be '8PTq2CkDyi', got '%v'", discountFeature.Hash)
	}

	// Verify pricing feature
	pricingFeature, exists := datafile.Features["pricing"]
	if !exists {
		t.Fatal("Expected 'pricing' feature to exist")
	}

	if pricingFeature.Hash == nil || *pricingFeature.Hash != "HGJQdeUNIu" {
		t.Errorf("Expected hash to be 'HGJQdeUNIu', got '%v'", pricingFeature.Hash)
	}

	// Verify showBanner feature with force rules
	showBannerFeature, exists := datafile.Features["showBanner"]
	if !exists {
		t.Fatal("Expected 'showBanner' feature to exist")
	}

	if showBannerFeature.Hash == nil || *showBannerFeature.Hash != "FaQN9OOIkm" {
		t.Errorf("Expected hash to be 'FaQN9OOIkm', got '%v'", showBannerFeature.Hash)
	}

	if len(showBannerFeature.Force) != 2 {
		t.Errorf("Expected 2 force rules, got %d", len(showBannerFeature.Force))
	}

	// Test JSON round-trip
	jsonStr, err := datafile.ToJSON()
	if err != nil {
		t.Fatalf("Failed to convert to JSON: %v", err)
	}

	var datafile2 DatafileContent
	err = datafile2.FromJSON(jsonStr)
	if err != nil {
		t.Fatalf("Failed to parse generated JSON: %v", err)
	}

	if datafile2.SchemaVersion != datafile.SchemaVersion {
		t.Errorf("SchemaVersion mismatch after round-trip: expected '%s', got '%s'",
			datafile.SchemaVersion, datafile2.SchemaVersion)
	}
}
