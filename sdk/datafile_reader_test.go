package sdk

import (
	"testing"
)

func TestNewDatafileReader(t *testing.T) {
	logger := NewLogger(CreateLoggerOptions{})
	jsonDatafile := `{
		"schemaVersion": "1.0.0",
		"revision": "test-revision",
		"segments": {},
		"features": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	reader := NewDatafileReader(DatafileReaderOptions{
		Datafile: datafile,
		Logger:   logger,
	})

	if reader == nil {
		t.Error("NewDatafileReader should return a non-nil reader")
	}

	if reader.GetRevision() != "test-revision" {
		t.Errorf("Expected revision 'test-revision', got '%s'", reader.GetRevision())
	}

	if reader.GetSchemaVersion() != "1.0.0" {
		t.Errorf("Expected schema version '1.0.0', got '%s'", reader.GetSchemaVersion())
	}
}

func TestDatafileReaderGetRegex(t *testing.T) {
	logger := NewLogger(CreateLoggerOptions{})
	jsonDatafile := `{
		"schemaVersion": "1.0.0",
		"revision": "test-revision",
		"segments": {},
		"features": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	reader := NewDatafileReader(DatafileReaderOptions{
		Datafile: datafile,
		Logger:   logger,
	})

	// Test regex caching
	regex1 := reader.GetRegex("test", "")
	regex2 := reader.GetRegex("test", "")

	if regex1 != regex2 {
		t.Error("GetRegex should return the same regex object for the same pattern")
	}

	// Test different patterns
	regex3 := reader.GetRegex("test2", "")
	if regex1 == regex3 {
		t.Error("GetRegex should return different regex objects for different patterns")
	}
}

func TestDatafileReaderAllConditionsAreMatched(t *testing.T) {
	logger := NewLogger(CreateLoggerOptions{})
	jsonDatafile := `{
		"schemaVersion": "1.0.0",
		"revision": "test-revision",
		"segments": {},
		"features": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	reader := NewDatafileReader(DatafileReaderOptions{
		Datafile: datafile,
		Logger:   logger,
	})

	context := Context{
		"country": "US",
		"age":     25,
	}

	// Test wildcard condition
	wildcardCondition := "*"
	result := reader.AllConditionsAreMatched(wildcardCondition, context)
	if !result {
		t.Error("Wildcard condition should always match")
	}

	// Test plain condition
	plainCondition := PlainCondition{
		Attribute: "country",
		Operator:  OperatorEquals,
		Value:     func() *ConditionValue { v := ConditionValue("US"); return &v }(),
	}
	result = reader.AllConditionsAreMatched(plainCondition, context)
	if !result {
		t.Error("Plain condition should match when context matches")
	}

	// Test and condition
	andCondition := AndCondition{
		And: []Condition{
			PlainCondition{
				Attribute: "country",
				Operator:  OperatorEquals,
				Value:     func() *ConditionValue { v := ConditionValue("US"); return &v }(),
			},
			PlainCondition{
				Attribute: "age",
				Operator:  OperatorGreaterThan,
				Value:     func() *ConditionValue { v := ConditionValue(20); return &v }(),
			},
		},
	}
	result = reader.AllConditionsAreMatched(andCondition, context)
	if !result {
		t.Error("And condition should match when all sub-conditions match")
	}

	// Test or condition
	orCondition := OrCondition{
		Or: []Condition{
			PlainCondition{
				Attribute: "country",
				Operator:  OperatorEquals,
				Value:     func() *ConditionValue { v := ConditionValue("CA"); return &v }(),
			},
			PlainCondition{
				Attribute: "country",
				Operator:  OperatorEquals,
				Value:     func() *ConditionValue { v := ConditionValue("US"); return &v }(),
			},
		},
	}
	result = reader.AllConditionsAreMatched(orCondition, context)
	if !result {
		t.Error("Or condition should match when at least one sub-condition matches")
	}
}

// TestDatafileReaderComprehensive tests comprehensive datafile reader functionality
func TestDatafileReaderComprehensive(t *testing.T) {
	logger := NewLogger(CreateLoggerOptions{})

	// Create a comprehensive datafile with segments and features
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1",
		"segments": {
			"netherlands": {
				"key": "netherlands",
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"nl\"}]"
			},
			"germany": {
				"key": "germany",
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"de\"}]"
			},
			"mobileUsers": {
				"key": "mobileUsers",
				"conditions": "[{\"attribute\":\"deviceType\",\"operator\":\"equals\",\"value\":\"mobile\"}]"
			},
			"desktopUsers": {
				"key": "desktopUsers",
				"conditions": "[{\"attribute\":\"deviceType\",\"operator\":\"equals\",\"value\":\"desktop\"}]"
			}
		},
		"features": {
			"test": {
				"key": "test",
				"bucketBy": "userId",
				"variations": [
					{"value": "control"},
					{
						"value": "treatment",
						"variables": {
							"showSidebar": true
						}
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
			}
		}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	reader := NewDatafileReader(DatafileReaderOptions{
		Datafile: datafile,
		Logger:   logger,
	})

	t.Run("basic functionality", func(t *testing.T) {
		// Test basic getters
		if reader.GetRevision() != "1" {
			t.Errorf("Expected revision '1', got '%s'", reader.GetRevision())
		}
		if reader.GetSchemaVersion() != "2" {
			t.Errorf("Expected schema version '2', got '%s'", reader.GetSchemaVersion())
		}

		// Test segment retrieval
		netherlandsSegment := reader.GetSegment("netherlands")
		if netherlandsSegment == nil {
			t.Error("Should retrieve netherlands segment")
		}

		germanySegment := reader.GetSegment("germany")
		if germanySegment == nil {
			t.Error("Should retrieve germany segment")
		}

		// Test feature retrieval
		testFeature := reader.GetFeature("test")
		if testFeature == nil {
			t.Error("Should retrieve test feature")
		}

		// Test non-existent entities
		if reader.GetSegment("belgium") != nil {
			t.Error("Should return nil for non-existent segment")
		}
		if reader.GetFeature("test2") != nil {
			t.Error("Should return nil for non-existent feature")
		}
	})

	t.Run("segment matching", func(t *testing.T) {
		// Test everyone segment
		result := reader.AllSegmentsAreMatched("*", Context{})
		if !result {
			t.Error("Wildcard segment should always match")
		}

		// Test simple segment
		result = reader.AllSegmentsAreMatched("netherlands", Context{"country": "nl"})
		if !result {
			t.Error("Netherlands segment should match for nl country")
		}

		// Test AND segments
		andSegments := AndGroupSegment{
			And: []GroupSegment{"mobileUsers", "netherlands"},
		}
		result = reader.AllSegmentsAreMatched(andSegments, Context{
			"country":    "nl",
			"deviceType": "mobile",
		})
		if !result {
			t.Error("AND segments should match when all conditions are met")
		}

		// Test OR segments
		orSegments := OrGroupSegment{
			Or: []GroupSegment{"mobileUsers", "desktopUsers"},
		}
		result = reader.AllSegmentsAreMatched(orSegments, Context{
			"deviceType": "mobile",
		})
		if !result {
			t.Error("OR segments should match when at least one condition is met")
		}

		// Test NOT segments
		notSegments := NotGroupSegment{
			Not: "mobileUsers",
		}
		result = reader.AllSegmentsAreMatched(notSegments, Context{
			"deviceType": "desktop",
		})
		if !result {
			t.Error("NOT segments should match when the condition is not met")
		}
	})

	t.Run("traffic matching", func(t *testing.T) {
		feature := reader.GetFeature("test")
		if feature == nil {
			t.Fatal("Test feature should exist")
		}

		// Test matched traffic
		matchedTraffic := reader.GetMatchedTraffic(feature.Traffic, Context{})
		if matchedTraffic == nil {
			t.Error("Should match traffic for wildcard segments")
		}

		// Test allocation matching
		allocation := reader.GetMatchedAllocation(matchedTraffic, 50000)
		if allocation == nil {
			t.Error("Should match allocation for bucket value 50000")
		}
	})

	t.Run("feature queries", func(t *testing.T) {
		// Test variable keys - the test feature doesn't have VariablesSchema defined
		keys := reader.GetVariableKeys("test")
		if len(keys) != 0 {
			t.Error("Should return empty variable keys for test feature without VariablesSchema")
		}

		// Test variations
		hasVariations := reader.HasVariations("test")
		if !hasVariations {
			t.Error("Test feature should have variations")
		}
	})
}

func TestDatafileReaderSegmentMatching(t *testing.T) {
	logger := NewLogger(CreateLoggerOptions{})

	// Create segments for comprehensive testing
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1",
		"segments": {
			"mobileUsers": {
				"key": "mobileUsers",
				"conditions": "[{\"attribute\":\"deviceType\",\"operator\":\"equals\",\"value\":\"mobile\"}]"
			},
			"desktopUsers": {
				"key": "desktopUsers",
				"conditions": "[{\"attribute\":\"deviceType\",\"operator\":\"equals\",\"value\":\"desktop\"}]"
			},
			"netherlands": {
				"key": "netherlands",
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"nl\"}]"
			},
			"germany": {
				"key": "germany",
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"de\"}]"
			}
		},
		"features": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	reader := NewDatafileReader(DatafileReaderOptions{
		Datafile: datafile,
		Logger:   logger,
	})

	t.Run("dutch mobile users", func(t *testing.T) {
		segments := []GroupSegment{"mobileUsers", "netherlands"}

		// Should match
		result := reader.AllSegmentsAreMatched(segments, Context{
			"country":    "nl",
			"deviceType": "mobile",
		})
		if !result {
			t.Error("Dutch mobile users should match")
		}

		// Should not match
		result = reader.AllSegmentsAreMatched(segments, Context{
			"country":    "de",
			"deviceType": "mobile",
		})
		if result {
			t.Error("German mobile users should not match dutch mobile users")
		}
	})

	t.Run("dutch mobile or desktop users", func(t *testing.T) {
		segments := []GroupSegment{
			"netherlands",
			OrGroupSegment{
				Or: []GroupSegment{"mobileUsers", "desktopUsers"},
			},
		}

		// Should match mobile
		result := reader.AllSegmentsAreMatched(segments, Context{
			"country":    "nl",
			"deviceType": "mobile",
		})
		if !result {
			t.Error("Dutch mobile users should match")
		}

		// Should match desktop
		result = reader.AllSegmentsAreMatched(segments, Context{
			"country":    "nl",
			"deviceType": "desktop",
		})
		if !result {
			t.Error("Dutch desktop users should match")
		}

		// Should not match
		result = reader.AllSegmentsAreMatched(segments, Context{
			"country":    "de",
			"deviceType": "mobile",
		})
		if result {
			t.Error("German users should not match")
		}
	})

	t.Run("german non-mobile users", func(t *testing.T) {
		segments := []GroupSegment{
			AndGroupSegment{
				And: []GroupSegment{
					"germany",
					NotGroupSegment{
						Not: "mobileUsers",
					},
				},
			},
		}

		// Should match desktop
		result := reader.AllSegmentsAreMatched(segments, Context{
			"country":    "de",
			"deviceType": "desktop",
		})
		if !result {
			t.Error("German desktop users should match")
		}

		// Should not match mobile
		result = reader.AllSegmentsAreMatched(segments, Context{
			"country":    "de",
			"deviceType": "mobile",
		})
		if result {
			t.Error("German mobile users should not match")
		}
	})
}

func TestDatafileReaderForceMatching(t *testing.T) {
	logger := NewLogger(CreateLoggerOptions{})
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1",
		"segments": {
			"premium": {
				"key": "premium",
				"conditions": "[{\"attribute\":\"userType\",\"operator\":\"equals\",\"value\":\"premium\"}]"
			}
		},
		"features": {
			"testFeature": {
				"key": "testFeature",
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
							{"variation": "control", "range": [0, 50000]},
							{"variation": "treatment", "range": [50000, 100000]}
						]
					}
				],
				"force": [
					{
						"variation": "control",
						"conditions": "[{\"attribute\":\"environment\",\"operator\":\"equals\",\"value\":\"staging\"}]"
					},
					{
						"variation": "treatment",
						"segments": "premium"
					}
				]
			}
		}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	reader := NewDatafileReader(DatafileReaderOptions{
		Datafile: datafile,
		Logger:   logger,
	})

	t.Run("force by conditions", func(t *testing.T) {
		context := Context{"environment": "staging"}
		result := reader.GetMatchedForce("testFeature", context)

		if result.Force == nil {
			t.Error("Should find force by conditions")
		}
		if result.ForceIndex == nil || *result.ForceIndex != 0 {
			t.Error("Should return correct force index")
		}
		if result.Force.Variation == nil || *result.Force.Variation != "control" {
			t.Error("Should return control variation")
		}
	})

	t.Run("force by segments", func(t *testing.T) {
		context := Context{"userType": "premium"}
		result := reader.GetMatchedForce("testFeature", context)

		if result.Force == nil {
			t.Error("Should find force by segments")
		}
		if result.ForceIndex == nil || *result.ForceIndex != 1 {
			t.Error("Should return correct force index")
		}
		if result.Force.Variation == nil || *result.Force.Variation != "treatment" {
			t.Error("Should return treatment variation")
		}
	})

	t.Run("no force match", func(t *testing.T) {
		context := Context{"environment": "production", "userType": "free"}
		result := reader.GetMatchedForce("testFeature", context)

		if result.Force != nil {
			t.Error("Should not find force when no conditions match")
		}
		if result.ForceIndex != nil {
			t.Error("Should not return force index when no match")
		}
	})
}

func TestDatafileReaderStringifiedParsing(t *testing.T) {
	logger := NewLogger(CreateLoggerOptions{})
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1",
		"segments": {
			"stringifiedSegment": {
				"key": "stringifiedSegment",
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"us\"}]"
			}
		},
		"features": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	reader := NewDatafileReader(DatafileReaderOptions{
		Datafile: datafile,
		Logger:   logger,
	})

	t.Run("parse stringified conditions", func(t *testing.T) {
		segment := reader.GetSegment("stringifiedSegment")
		if segment == nil {
			t.Fatal("Should retrieve stringified segment")
		}

		// The conditions should be parsed from string to actual condition
		// Since parseConditionsIfStringified is called in GetSegment, we need to check
		// that the conditions are properly parsed
		conditions := segment.Conditions

		// The conditions should be parsed from the JSON string to actual conditions
		// We can verify this by checking that it's not a string anymore
		if _, ok := conditions.(string); ok {
			t.Error("Conditions should be parsed from string to actual condition structure")
		}

		// Check that we can access the conditions as an array
		// The JSON unmarshaling returns []interface{} initially
		if conditionArray, ok := conditions.([]interface{}); ok {
			if len(conditionArray) != 1 {
				t.Error("Should have one condition")
			}

			// Check that the first condition has the expected structure
			firstCondition, ok := conditionArray[0].(map[string]interface{})
			if !ok {
				t.Error("First condition should be a map")
			}

			if firstCondition["attribute"] != "country" {
				t.Error("Should have correct attribute")
			}

			if firstCondition["operator"] != "equals" {
				t.Error("Should have correct operator")
			}

			if firstCondition["value"] != "us" {
				t.Error("Should have correct value")
			}
		} else {
			t.Error("Conditions should be parseable as array of interface{}")
		}
	})

	t.Run("parse stringified segments", func(t *testing.T) {
		// Test parsing segments that are stringified
		stringifiedSegments := `["segment1", "segment2"]`
		parsed := reader.parseSegmentsIfStringified(stringifiedSegments)

		segments, ok := parsed.([]interface{})
		if !ok {
			t.Error("Should parse stringified segments to array")
		}

		if len(segments) != 2 {
			t.Error("Should have two segments")
		}
	})
}

func TestDatafileReaderErrorHandling(t *testing.T) {
	logger := NewLogger(CreateLoggerOptions{})
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1",
		"segments": {
			"invalidSegment": {
				"key": "invalidSegment",
				"conditions": "invalid json"
			}
		},
		"features": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	reader := NewDatafileReader(DatafileReaderOptions{
		Datafile: datafile,
		Logger:   logger,
	})

	t.Run("handle invalid JSON in conditions", func(t *testing.T) {
		segment := reader.GetSegment("invalidSegment")
		if segment == nil {
			t.Fatal("Should retrieve segment even with invalid conditions")
		}

		// Should return the original string when parsing fails
		conditions, ok := segment.Conditions.(string)
		if !ok {
			t.Error("Should return original string when parsing fails")
		}

		if conditions != "invalid json" {
			t.Error("Should preserve original invalid JSON string")
		}
	})
}
