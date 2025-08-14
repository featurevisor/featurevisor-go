package sdk

import (
	"testing"
)

func TestAPICompatibilityWithStringFeatureKey(t *testing.T) {
	// Test data using JSON string like working tests
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "test",
		"features": {
			"test-feature": {
				"key": "test-feature",
				"bucketBy": "userId",
				"variations": [
					{"value": "control"},
					{"value": "treatment"}
				],
				"traffic": [
					{
						"key": "1-100",
						"segments": ["all"],
						"percentage": 100000,
						"allocation": [
							{"variation": "control", "range": [0, 50000]},
							{"variation": "treatment", "range": [50000, 100000]}
						]
					}
				],
				"variablesSchema": {
					"color": {
						"key": "color",
						"type": "string",
						"defaultValue": "blue"
					}
				}
			}
		},
		"segments": {
			"all": {
				"key": "all",
				"conditions": [{"attribute": "userId", "operator": "exists"}]
			}
		}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	// Create instance
	instance := NewFeaturevisor(InstanceOptions{
		Datafile: datafile,
		Context:  Context{"userId": "123"},
	})

	// Test IsEnabled with string featureKey
	enabled := instance.IsEnabled("test-feature")
	if !enabled {
		t.Error("Expected feature to be enabled")
	}

	// Test GetVariation with string featureKey
	variation := instance.GetVariation("test-feature")
	if variation == nil {
		t.Error("Expected variation to be returned")
	}

	// Test GetVariable with string featureKey
	variable := instance.GetVariable("test-feature", "color")
	if variable == nil {
		t.Error("Expected variable to be returned")
	}
	if variable != "blue" {
		t.Errorf("Expected variable value 'blue', got %v", variable)
	}

	// Test GetFeature with string featureKey
	feature := instance.GetFeature("test-feature")
	if feature == nil {
		t.Error("Expected feature to be returned")
	}
	if feature.Key == nil || *feature.Key != "test-feature" {
		t.Errorf("Expected feature key 'test-feature', got %v", feature.Key)
	}

	// Test child instance
	child := instance.Spawn(Context{"userId": "456"})

	// Test child methods with string featureKey
	childEnabled := child.IsEnabled("test-feature")
	if !childEnabled {
		t.Error("Expected child feature to be enabled")
	}

	childVariation := child.GetVariation("test-feature")
	if childVariation == nil {
		t.Error("Expected child variation to be returned")
	}

	childVariable := child.GetVariable("test-feature", "color")
	if childVariable == nil {
		t.Error("Expected child variable to be returned")
	}
}

func TestAPICompatibilityWithNonExistentFeature(t *testing.T) {
	// Create instance with empty datafile
	instance := NewFeaturevisor(InstanceOptions{
		Datafile: DatafileContent{
			SchemaVersion: "2",
			Revision:      "test",
			Features:      make(map[FeatureKey]Feature),
			Segments:      make(map[SegmentKey]Segment),
		},
	})

	// Test with non-existent feature
	enabled := instance.IsEnabled("non-existent")
	if enabled {
		t.Error("Expected non-existent feature to be disabled")
	}

	variation := instance.GetVariation("non-existent")
	if variation != nil {
		t.Error("Expected non-existent feature variation to be nil")
	}

	variable := instance.GetVariable("non-existent", "color")
	if variable != nil {
		t.Error("Expected non-existent feature variable to be nil")
	}

	feature := instance.GetFeature("non-existent")
	if feature != nil {
		t.Error("Expected non-existent feature to be nil")
	}
}

func TestProductionDatafileFeatures(t *testing.T) {
	// Production datafile content from example-1 (using integer weights to avoid type issues)
	jsonDatafile := `{
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
								"range": [
									0,
									50000
								]
							},
							{
								"variation": "treatment",
								"range": [
									50000,
									100000
								]
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
								"range": [
									0,
									10000
								]
							},
							{
								"variation": "treatment",
								"range": [
									10000,
									100000
								]
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
								"range": [
									0,
									50000
								]
							},
							{
								"variation": "treatment",
								"range": [
									50000,
									100000
								]
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
								"range": [
									0,
									16500
								]
							},
							{
								"variation": "b",
								"range": [
									16500,
									33000
								]
							},
							{
								"variation": "c",
								"range": [
									33000,
									50000
								]
							}
						]
					}
				],
				"ranges": [
					[
						0,
						50000
					]
				],
				"variablesSchema": {
					"color": {
						"type": "string",
						"defaultValue": "red"
					},
					"hero": {
						"type": "object",
						"defaultValue": {
							"title": "Hero Title",
							"subtitle": "Hero Subtitle",
							"alignment": "center"
						}
					}
				},
				"hash": "2bGkQN1GnW"
			},
			"foo": {
				"bucketBy": "userId",
				"variations": [
					{
						"value": "control",
						"weight": 50
					},
					{
						"value": "treatment",
						"weight": 50,
						"variables": {
							"bar": "bar_here",
							"baz": "baz_here"
						},
						"variableOverrides": {
							"bar": [
								{
									"segments": "{\"or\":[\"countries/germany\",\"countries/switzerland\"]}",
									"value": "bar for DE or CH"
								}
							],
							"baz": [
								{
									"segments": "countries/netherlands",
									"value": "baz for NL"
								}
							]
						}
					}
				],
				"traffic": [
					{
						"key": "1",
						"segments": "{\"and\":[\"mobile\",{\"or\":[\"countries/germany\",\"countries/switzerland\"]}]}",
						"percentage": 80000,
						"allocation": [
							{
								"variation": "control",
								"range": [
									0,
									40000
								]
							},
							{
								"variation": "treatment",
								"range": [
									40000,
									80000
								]
							}
						],
						"variables": {
							"qux": true
						}
					},
					{
						"key": "2",
						"segments": "*",
						"percentage": 50000,
						"allocation": [
							{
								"variation": "control",
								"range": [
									0,
									25000
								]
							},
							{
								"variation": "treatment",
								"range": [
									25000,
									50000
								]
							}
						]
					}
				],
				"variablesSchema": {
					"bar": {
						"type": "string",
						"defaultValue": ""
					},
					"baz": {
						"type": "string",
						"defaultValue": ""
					},
					"qux": {
						"type": "boolean",
						"defaultValue": false
					}
				},
				"force": [
					{
						"conditions": "{\"and\":[{\"attribute\":\"userId\",\"operator\":\"equals\",\"value\":\"123\"},{\"attribute\":\"device\",\"operator\":\"equals\",\"value\":\"mobile\"}]}",
						"variation": "treatment",
						"variables": {
							"bar": "yoooooo"
						}
					}
				],
				"hash": "sjCzQ7BZZa"
			},
			"sidebar": {
				"bucketBy": "userId",
				"variations": [
					{
						"value": "control",
						"weight": 10
					},
					{
						"value": "treatment",
						"weight": 90,
						"variables": {
							"position": "right",
							"color": "red",
							"sections": [
								"home",
								"about",
								"contact"
							]
						},
						"variableOverrides": {
							"color": [
								{
									"segments": "[\"countries/germany\"]",
									"value": "yellow"
								},
								{
									"segments": "[\"countries/switzerland\"]",
									"value": "white"
								}
							],
							"sections": [
								{
									"segments": "[\"countries/germany\"]",
									"value": [
										"home",
										"about",
										"contact",
										"imprint"
									]
								},
								{
									"segments": "[\"countries/netherlands\"]",
									"value": [
										"home",
										"about",
										"contact",
										"bitterballen"
									]
								}
							]
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
								"range": [
									0,
									10000
								]
							},
							{
								"variation": "treatment",
								"range": [
									10000,
									100000
								]
							}
						],
						"variables": {
							"title": "Sidebar Title for production"
						}
					}
				],
				"variablesSchema": {
					"position": {
						"type": "string",
						"defaultValue": "left"
					},
					"color": {
						"type": "string",
						"defaultValue": "red"
					},
					"sections": {
						"type": "array",
						"defaultValue": []
					},
					"title": {
						"type": "string",
						"defaultValue": "Sidebar Title"
					}
				},
				"hash": "prntaZC9Qu"
			},
			"qux": {
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
							"fooConfig": "{\"foo\": \"bar b\"}"
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
						"segments": "[\"countries/netherlands\"]",
						"percentage": 50000,
						"allocation": [
							{
								"variation": "control",
								"range": [
									50001,
									66671
								]
							},
							{
								"variation": "b",
								"range": [
									66671,
									83336
								]
							},
							{
								"variation": "c",
								"range": [
									83336,
									100000
								]
							}
						],
						"variation": "b"
					},
					{
						"key": "2",
						"segments": "*",
						"percentage": 50000,
						"allocation": [
							{
								"variation": "control",
								"range": [
									50001,
									66671
								]
							},
							{
								"variation": "b",
								"range": [
									66671,
									83336
								]
							},
							{
								"variation": "c",
								"range": [
									83336,
									100000
								]
							}
						]
					}
				],
				"ranges": [
					[
						50001,
						100000
					]
				],
				"variablesSchema": {
					"fooConfig": {
						"type": "json",
						"defaultValue": "{\"foo\": \"bar\"}"
					}
				},
				"hash": "gEdJ1btAXy"
			}
		}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse production datafile JSON: %v", err)
	}

	// Test allowSignup feature based on test specs
	t.Run("allowSignup", func(t *testing.T) {
		// Test Netherlands (NL) - should always get treatment variation
		// Using bucket value 60000 (60%) to ensure we get treatment variation
		instance := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"country":  "nl",
				"deviceId": "test-device-123",
			},
			Hooks: []*Hook{
				{
					Name: "test-hook",
					BucketValue: func(options ConfigureBucketValueOptions) int {
						// Force bucket value to 60000 (60%) to get treatment variation
						return 60000
					},
				},
			},
		})

		enabled := instance.IsEnabled("allowSignup")
		if !enabled {
			t.Error("Expected allowSignup to be enabled for NL")
		}

		variation := instance.GetVariation("allowSignup")
		if variation == nil {
			t.Error("Expected variation to be returned for allowSignup")
		}
		if *variation != "treatment" {
			t.Errorf("Expected treatment variation for NL, got %s", *variation)
		}

		// Test variables
		allowGoogle := instance.GetVariable("allowSignup", "allowGoogleSignUp")
		if allowGoogle != true {
			t.Errorf("Expected allowGoogleSignUp to be true for NL, got %v", allowGoogle)
		}

		allowGitHub := instance.GetVariable("allowSignup", "allowGitHubSignUp")
		if allowGitHub != true {
			t.Errorf("Expected allowGitHubSignUp to be true for NL, got %v", allowGitHub)
		}

		allowRegular := instance.GetVariable("allowSignup", "allowRegularSignUp")
		if allowRegular != true {
			t.Errorf("Expected allowRegularSignUp to be true, got %v", allowRegular)
		}

		// Test Switzerland (CH) - should get treatment variation based on weight
		instanceCH := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"country":  "ch",
				"deviceId": "test-device-ch",
			},
			Hooks: []*Hook{
				{
					Name: "test-hook",
					BucketValue: func(options ConfigureBucketValueOptions) int {
						// Force bucket value to 60000 (60%) to get treatment variation
						return 60000
					},
				},
			},
		})

		enabledCH := instanceCH.IsEnabled("allowSignup")
		if !enabledCH {
			t.Error("Expected allowSignup to be enabled for CH")
		}

		// Test Germany (DE) - should get control variation in everyone segment
		instanceDE := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"country":  "de",
				"deviceId": "test-device-de",
			},
			Hooks: []*Hook{
				{
					Name: "test-hook",
					BucketValue: func(options ConfigureBucketValueOptions) int {
						// Force bucket value to 40000 (40%) to get control variation
						return 40000
					},
				},
			},
		})

		enabledDE := instanceDE.IsEnabled("allowSignup")
		if !enabledDE {
			t.Error("Expected allowSignup to be enabled for DE")
		}
	})

	// Test bar feature based on test specs
	t.Run("bar", func(t *testing.T) {
		// Test with US context (should get control variation at low bucket values)
		// Using bucket value 15000 (15%) to get control variation
		instance := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"country": "us",
				"userId":  "test-user-15",
			},
			Hooks: []*Hook{
				{
					Name: "test-hook",
					BucketValue: func(options ConfigureBucketValueOptions) int {
						// Force bucket value to 15000 (15%) to get control variation
						return 15000
					},
				},
			},
		})

		enabled := instance.IsEnabled("bar")
		if !enabled {
			t.Error("Expected bar to be enabled")
		}

		variation := instance.GetVariation("bar")
		if variation == nil {
			t.Error("Expected variation to be returned for bar")
		}

		// Test variables
		color := instance.GetVariable("bar", "color")
		if color != "red" {
			t.Errorf("Expected color to be 'red', got %v", color)
		}

		hero := instance.GetVariable("bar", "hero")
		if hero == nil {
			t.Error("Expected hero variable to be returned")
		}

		// Test with Germany context (should get variation 'b' with overrides)
		// Using bucket value 20000 (20%) to get variation 'b'
		instanceDE := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"country": "de",
				"userId":  "test-user-de",
			},
			Hooks: []*Hook{
				{
					Name: "test-hook",
					BucketValue: func(options ConfigureBucketValueOptions) int {
						// Force bucket value to 20000 (20%) to get variation 'b'
						return 20000
					},
				},
			},
		})

		enabledDE := instanceDE.IsEnabled("bar")
		if !enabledDE {
			t.Error("Expected bar to be enabled for DE")
		}
	})

	// Test foo feature based on test specs
	t.Run("foo", func(t *testing.T) {
		// Test with mobile + Germany context (should get treatment variation)
		// Using bucket value 60000 (60%) to get treatment variation
		instance := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"country": "de",
				"device":  "mobile",
				"userId":  "test-user-foo",
			},
			Hooks: []*Hook{
				{
					Name: "test-hook",
					BucketValue: func(options ConfigureBucketValueOptions) int {
						// Force bucket value to 60000 (60%) to get treatment variation
						return 60000
					},
				},
			},
		})

		enabled := instance.IsEnabled("foo")
		if !enabled {
			t.Error("Expected foo to be enabled for mobile + DE")
		}

		variation := instance.GetVariation("foo")
		if variation == nil {
			t.Error("Expected variation to be returned for foo")
		}

		// Test variables with overrides
		bar := instance.GetVariable("foo", "bar")
		if bar != "bar for DE or CH" {
			t.Errorf("Expected bar to be 'bar for DE or CH', got %v", bar)
		}

		baz := instance.GetVariable("foo", "baz")
		if baz != "baz_here" {
			t.Errorf("Expected baz to be 'baz_here', got %v", baz)
		}

		qux := instance.GetVariable("foo", "qux")
		if qux != true {
			t.Errorf("Expected qux to be true, got %v", qux)
		}

		// Test force rule
		instanceForce := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"userId": "123",
				"device": "mobile",
			},
		})

		enabledForce := instanceForce.IsEnabled("foo")
		if !enabledForce {
			t.Error("Expected foo to be enabled with force rule")
		}

		variationForce := instanceForce.GetVariation("foo")
		if variationForce == nil {
			t.Error("Expected variation to be returned with force rule")
		}
		if *variationForce != "treatment" {
			t.Errorf("Expected treatment variation with force rule, got %s", *variationForce)
		}

		barForce := instanceForce.GetVariable("foo", "bar")
		if barForce != "yoooooo" {
			t.Errorf("Expected bar to be 'yoooooo' with force rule, got %v", barForce)
		}
	})

	// Test sidebar feature based on test specs
	t.Run("sidebar", func(t *testing.T) {
		// Test with Netherlands context (should get treatment variation)
		// Using bucket value 90000 (90%) to get treatment variation
		instance := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"country": "nl",
				"userId":  "test-user-nl",
			},
			Hooks: []*Hook{
				{
					Name: "test-hook",
					BucketValue: func(options ConfigureBucketValueOptions) int {
						// Force bucket value to 90000 (90%) to get treatment variation
						return 90000
					},
				},
			},
		})

		enabled := instance.IsEnabled("sidebar")
		if !enabled {
			t.Error("Expected sidebar to be enabled for NL")
		}

		variation := instance.GetVariation("sidebar")
		if variation == nil {
			t.Error("Expected variation to be returned for sidebar")
		}
		if *variation != "treatment" {
			t.Errorf("Expected treatment variation for NL, got %s", *variation)
		}

		// Test variables
		position := instance.GetVariable("sidebar", "position")
		if position != "right" {
			t.Errorf("Expected position to be 'right', got %v", position)
		}

		color := instance.GetVariable("sidebar", "color")
		if color != "red" {
			t.Errorf("Expected color to be 'red' for NL, got %v", color)
		}

		title := instance.GetVariable("sidebar", "title")
		if title != "Sidebar Title for production" {
			t.Errorf("Expected title to be 'Sidebar Title for production', got %v", title)
		}

		sections := instance.GetVariable("sidebar", "sections")
		if sections == nil {
			t.Error("Expected sections variable to be returned")
		}

		// Test with Germany context (should get color override)
		// Using bucket value 90000 (90%) to get treatment variation
		instanceDE := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"country": "de",
				"userId":  "test-user-de",
			},
			Hooks: []*Hook{
				{
					Name: "test-hook",
					BucketValue: func(options ConfigureBucketValueOptions) int {
						// Force bucket value to 90000 (90%) to get treatment variation
						return 90000
					},
				},
			},
		})

		colorDE := instanceDE.GetVariable("sidebar", "color")
		if colorDE != "yellow" {
			t.Errorf("Expected color to be 'yellow' for DE, got %v", colorDE)
		}

		sectionsDE := instanceDE.GetVariable("sidebar", "sections")
		if sectionsDE == nil {
			t.Error("Expected sections variable to be returned for DE")
		}
	})

	// Test qux feature based on test specs
	t.Run("qux", func(t *testing.T) {
		// Test with Netherlands context (should get variation 'b' based on allocation)
		// Using bucket value 70000 (70%) to get variation 'b'
		instance := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"country": "nl",
				"userId":  "test-user-qux",
			},
			Hooks: []*Hook{
				{
					Name: "test-hook",
					BucketValue: func(options ConfigureBucketValueOptions) int {
						// Force bucket value to 70000 (70%) to get variation 'b'
						return 70000
					},
				},
			},
		})

		enabled := instance.IsEnabled("qux")
		if !enabled {
			t.Error("Expected qux to be enabled for NL")
		}

		variation := instance.GetVariation("qux")
		if variation == nil {
			t.Error("Expected variation to be returned for qux")
		}

		// Test variables
		fooConfig := instance.GetVariable("qux", "fooConfig")
		if fooConfig == nil {
			t.Error("Expected fooConfig variable to be returned")
		}

		// Test with Germany context (should get variation 'b' based on allocation)
		// Using bucket value 70000 (70%) to get variation 'b'
		instanceDE := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"country": "de",
				"userId":  "test-user-qux-de",
			},
			Hooks: []*Hook{
				{
					Name: "test-hook",
					BucketValue: func(options ConfigureBucketValueOptions) int {
						// Force bucket value to 70000 (70%) to get variation 'b'
						return 70000
					},
				},
			},
		})

		enabledDE := instanceDE.IsEnabled("qux")
		if !enabledDE {
			t.Error("Expected qux to be enabled for DE")
		}
	})
}

func TestProductionDatafileSegments(t *testing.T) {
	// Test segment evaluation with the same production datafile
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1",
		"segments": {
			"countries/germany": {
				"conditions": "{\"and\":[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"de\"}]}"
			},
			"countries/netherlands": {
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"nl\"}]"
			},
			"mobile": {
				"conditions": "{\"and\":[{\"attribute\":\"device\",\"operator\":\"equals\",\"value\":\"mobile\"},{\"attribute\":\"phone\",\"operator\":\"notExists\"}]}"
			},
			"everyone": {
				"conditions": "*"
			}
		},
		"features": {
			"testSegment": {
				"bucketBy": "userId",
				"traffic": [
					{
						"key": "1",
						"segments": "[\"countries/germany\"]",
						"percentage": 100000
					},
					{
						"key": "2",
						"segments": "*",
						"percentage": 100000
					}
				]
			}
		}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse segment test datafile JSON: %v", err)
	}

	// Test segment evaluation
	t.Run("segmentEvaluation", func(t *testing.T) {
		// Test Germany segment
		instance := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"country": "de",
				"userId":  "test-user",
			},
		})

		enabled := instance.IsEnabled("testSegment")
		if !enabled {
			t.Error("Expected testSegment to be enabled for DE")
		}

		// Test Netherlands segment (should match because of everyone segment rule)
		instanceNL := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"country": "nl",
				"userId":  "test-user",
			},
		})

		enabledNL := instanceNL.IsEnabled("testSegment")
		if !enabledNL {
			t.Error("Expected testSegment to be enabled for NL (everyone segment)")
		}

		// Test mobile segment
		_ = NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"device": "mobile",
				"userId": "test-user",
			},
		})

		// Test everyone segment
		instanceEveryone := NewFeaturevisor(InstanceOptions{
			Datafile: datafile,
			Context: Context{
				"userId": "test-user",
			},
		})

		// Everyone segment should always match
		if !instanceEveryone.IsEnabled("testSegment") {
			t.Error("Expected testSegment to be enabled for everyone segment")
		}
	})
}
