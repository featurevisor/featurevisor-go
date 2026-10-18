package featurevisor

import (
	"fmt"
	"sync"
	"testing"
)

func TestLateTrafficAndAllocationKeepFirstMatchAndValueOwnership(t *testing.T) {
	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           DatafileContent{SchemaVersion: "2", Revision: "late", Segments: map[SegmentKey]Segment{}},
		diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{}),
	})
	traffic := make([]Traffic, 128)
	for index := range traffic {
		traffic[index] = Traffic{Key: RuleKey(fmt.Sprintf("rule-%d", index)), Segments: "missing"}
	}
	traffic[126].Segments = "*"
	traffic[127].Segments = "*"
	traffic[126].Allocation = make([]Allocation, 128)
	for index := range traffic[126].Allocation {
		traffic[126].Allocation[index] = Allocation{Variation: VariationValue(fmt.Sprintf("variation-%d", index)), Range: Range{index, index}}
	}

	matched := reader.GetMatchedTraffic(traffic, Context{})
	if matched == nil || matched.Key != "rule-126" {
		t.Fatalf("expected first late rule, got %#v", matched)
	}
	allocation := reader.GetMatchedAllocation(matched, 127)
	if allocation == nil || allocation.Variation != "variation-127" {
		t.Fatalf("expected last allocation, got %#v", allocation)
	}
	matched.Key = "caller-change"
	allocation.Variation = "caller-change"
	if traffic[126].Key != "rule-126" || traffic[126].Allocation[127].Variation != "variation-127" {
		t.Fatal("matched metadata aliased candidate structs")
	}
}

func TestConcurrentConditionEvaluationsShareRegexCacheSafely(t *testing.T) {
	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile: DatafileContent{
			SchemaVersion: "2",
			Revision:      "concurrent",
			Segments:      map[SegmentKey]Segment{},
			Features:      map[FeatureKey]Feature{},
		},
		diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{}),
	})
	condition := map[string]interface{}{
		"attribute":  "browser",
		"operator":   "matches",
		"value":      "^chrome$",
		"regexFlags": "i",
	}

	var waitGroup sync.WaitGroup
	for index := 0; index < 100; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			if !reader.AllConditionsAreMatched(condition, Context{"browser": "Chrome"}) {
				t.Error("expected concurrent condition to match")
			}
		}()
	}
	waitGroup.Wait()
}

func TestNewInstanceEvaluationDataProvider(t *testing.T) {
	diagnostics := newDiagnosticReporter(diagnosticReporterOptions{})
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "test-revision",
		"segments": {},
		"features": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           datafile,
		diagnosticReporter: diagnostics,
	})

	if reader == nil {
		t.Error("newInstanceEvaluationDataProvider should return a non-nil reader")
	}

	if reader.GetRevision() != "test-revision" {
		t.Errorf("Expected revision 'test-revision', got '%s'", reader.GetRevision())
	}

	if reader.GetSchemaVersion() != "2" {
		t.Errorf("Expected schema version '2', got '%s'", reader.GetSchemaVersion())
	}
}

func TestInstanceEvaluationDataProviderGetRegex(t *testing.T) {
	diagnostics := newDiagnosticReporter(diagnosticReporterOptions{})
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "test-revision",
		"segments": {},
		"features": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           datafile,
		diagnosticReporter: diagnostics,
	})

	// Test regex caching
	regex1 := reader.getRegex("test", "")
	regex2 := reader.getRegex("test", "")

	if regex1 != regex2 {
		t.Error("GetRegex should return the same regex object for the same pattern")
	}

	// Test different patterns
	regex3 := reader.getRegex("test2", "")
	if regex1 == regex3 {
		t.Error("GetRegex should return different regex objects for different patterns")
	}
}

func TestInstanceEvaluationDataProviderAllConditionsAreMatched(t *testing.T) {
	diagnostics := newDiagnosticReporter(diagnosticReporterOptions{})
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "test-revision",
		"segments": {},
		"features": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           datafile,
		diagnosticReporter: diagnostics,
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

	notCondition := NotCondition{
		Not: []Condition{
			PlainCondition{
				Attribute: "country",
				Operator:  OperatorEquals,
				Value:     func() *ConditionValue { v := ConditionValue("US"); return &v }(),
			},
			PlainCondition{
				Attribute: "age",
				Operator:  OperatorGreaterThan,
				Value:     func() *ConditionValue { v := ConditionValue(30); return &v }(),
			},
		},
	}
	result = reader.AllConditionsAreMatched(notCondition, context)
	if !result {
		t.Error("NOT condition should match when not all direct children match")
	}

	result = reader.AllConditionsAreMatched(NotCondition{Not: []Condition{}}, context)
	if result {
		t.Error("Empty NOT condition should defensively return false")
	}
}

// TestInstanceEvaluationDataProviderComprehensive tests the complete evaluation data provider.
func TestInstanceEvaluationDataProviderComprehensive(t *testing.T) {
	diagnostics := newDiagnosticReporter(diagnosticReporterOptions{})

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

	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           datafile,
		diagnosticReporter: diagnostics,
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

func TestInstanceEvaluationDataProviderSegmentMatching(t *testing.T) {
	diagnostics := newDiagnosticReporter(diagnosticReporterOptions{})

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

	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           datafile,
		diagnosticReporter: diagnostics,
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

	t.Run("not segments negate implicit and", func(t *testing.T) {
		segments := NotGroupSegment{
			Not: []GroupSegment{"mobileUsers", "netherlands"},
		}

		result := reader.AllSegmentsAreMatched(segments, Context{
			"country":    "nl",
			"deviceType": "mobile",
		})
		if result {
			t.Error("NOT segments should not match when all direct children match")
		}

		result = reader.AllSegmentsAreMatched(segments, Context{
			"country":    "nl",
			"deviceType": "desktop",
		})
		if !result {
			t.Error("NOT segments should match when only some direct children match")
		}
	})

	t.Run("not with nested or means none match", func(t *testing.T) {
		segments := NotGroupSegment{
			Not: []GroupSegment{
				OrGroupSegment{
					Or: []GroupSegment{"mobileUsers", "desktopUsers"},
				},
			},
		}

		if reader.AllSegmentsAreMatched(segments, Context{"deviceType": "mobile"}) {
			t.Error("Nested OR under NOT should not match mobile users")
		}

		if !reader.AllSegmentsAreMatched(segments, Context{"deviceType": "tv"}) {
			t.Error("Nested OR under NOT should match when none of the OR children match")
		}
	})

	t.Run("empty not segments return false", func(t *testing.T) {
		segments := NotGroupSegment{Not: []GroupSegment{}}

		if reader.AllSegmentsAreMatched(segments, Context{}) {
			t.Error("Empty NOT segments should defensively return false")
		}
	})
}

func TestInstanceEvaluationDataProviderForceMatching(t *testing.T) {
	diagnostics := newDiagnosticReporter(diagnosticReporterOptions{})
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

	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           datafile,
		diagnosticReporter: diagnostics,
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

func TestInstanceEvaluationDataProviderStringifiedParsing(t *testing.T) {
	diagnostics := newDiagnosticReporter(diagnosticReporterOptions{})
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

	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           datafile,
		diagnosticReporter: diagnostics,
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

func TestInstanceEvaluationDataProviderCachesStringifiedExpressionsPerGeneration(t *testing.T) {
	condition := `[{"attribute":"country","operator":"equals","value":"us"}]`
	datafile := DatafileContent{
		SchemaVersion: "2",
		Revision:      "cached",
		Segments: map[SegmentKey]Segment{
			"country": {Conditions: condition},
		},
		Features: map[FeatureKey]Feature{},
	}
	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           datafile,
		diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{}),
	})

	for index := 0; index < 2; index++ {
		if !reader.AllSegmentsAreMatched("country", Context{"country": "us"}) {
			t.Fatal("expected cached segment condition to match")
		}
	}
	if count := len(reader.entities); count != 1 {
		t.Fatalf("expected one normalized segment entity, got %d", count)
	}

	group := `["country"]`
	for index := 0; index < 2; index++ {
		if !reader.AllSegmentsAreMatched(reader.parseSegmentsIfStringified(group), Context{"country": "us"}) {
			t.Fatal("expected cached segment group to match")
		}
	}
	if count := reader.getCachedExpressionCount(); count != 1 {
		t.Fatalf("expected the legacy helper segment expression in its cache, got %d", count)
	}

	replacement := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           datafile,
		diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{}),
	})
	if count := replacement.getCachedExpressionCount(); count != 0 {
		t.Fatalf("expected a fresh provider generation to start with an empty cache, got %d", count)
	}
}

func TestInstanceEvaluationDataProviderExpressionCacheIsConcurrent(t *testing.T) {
	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile: DatafileContent{
			SchemaVersion: "2",
			Revision:      "concurrent-expressions",
			Segments:      map[SegmentKey]Segment{},
			Features:      map[FeatureKey]Feature{},
		},
		diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{}),
	})
	expression := `[{"attribute":"country","operator":"equals","value":"us"}]`

	var waitGroup sync.WaitGroup
	for index := 0; index < 100; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			parsed := reader.parseConditionsIfStringified(expression)
			if !reader.AllConditionsAreMatched(parsed, Context{"country": "us"}) {
				t.Error("expected parsed condition to match")
			}
		}()
	}
	waitGroup.Wait()

	if count := reader.getCachedExpressionCount(); count != 1 {
		t.Fatalf("expected one shared cached expression, got %d", count)
	}
}

func TestGetSegmentKeepsParsedConditionOwnership(t *testing.T) {
	condition := `[{"attribute":"country","operator":"equals","value":"us"}]`
	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile: DatafileContent{
			SchemaVersion: "2",
			Revision:      "ownership",
			Segments: map[SegmentKey]Segment{
				"country": {Conditions: condition},
			},
			Features: map[FeatureKey]Feature{},
		},
		diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{}),
	})

	first := reader.GetSegment("country")
	firstConditions := first.Conditions.([]interface{})
	firstConditions[0].(map[string]interface{})["value"] = "nl"

	second := reader.GetSegment("country")
	secondConditions := second.Conditions.([]interface{})
	if value := secondConditions[0].(map[string]interface{})["value"]; value != "us" {
		t.Fatalf("expected each public segment result to own its parsed conditions, got %#v", value)
	}
}

func TestDiagnosticPayloadCannotMutateCachedExpression(t *testing.T) {
	var handler diagnosticOutputHandler = func(_ LogLevel, _ logMessage, details logDetails) {
		if segments, ok := details["segments"].(map[string]interface{}); ok {
			segments["or"] = []interface{}{"allowed"}
		}
	}
	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile: DatafileContent{
			SchemaVersion: "2",
			Revision:      "diagnostic-ownership",
			Segments: map[SegmentKey]Segment{
				"allowed": {Conditions: "*"},
			},
			Features: map[FeatureKey]Feature{},
		},
		diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{Handler: &handler}),
	})
	segments := reader.parseSegmentsIfStringified(`{"unexpected":[]}`)

	if reader.AllSegmentsAreMatched(segments, Context{}) {
		t.Fatal("expected unknown segment expression to remain unmatched")
	}
	if reader.AllSegmentsAreMatched(segments, Context{}) {
		t.Fatal("expected diagnostic callback mutation not to change a cached decision")
	}
}

func TestExpressionCacheIsReleasedWhenClosed(t *testing.T) {
	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           DatafileContent{SchemaVersion: "2", Revision: "close", Segments: map[SegmentKey]Segment{}, Features: map[FeatureKey]Feature{}},
		diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{}),
	})
	expression := `[{"attribute":"country","operator":"equals","value":"nl"}]`
	reader.parseConditionsIfStringified(expression)
	reader.closeExpressionCache()

	if count := reader.getCachedExpressionCount(); count != 0 {
		t.Fatalf("expected close to release cached expressions, got %d", count)
	}
	reader.parseConditionsIfStringified(expression)
	if count := reader.getCachedExpressionCount(); count != 0 {
		t.Fatalf("expected parsing after close not to retain expressions, got %d", count)
	}
}

func TestExpressionCacheDoesNotRetainParseErrors(t *testing.T) {
	reported := 0
	var handler diagnosticOutputHandler = func(level LogLevel, _ logMessage, _ logDetails) {
		if level == LogLevelError {
			reported++
		}
	}
	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           DatafileContent{SchemaVersion: "2", Revision: "invalid", Segments: map[SegmentKey]Segment{}, Features: map[FeatureKey]Feature{}},
		diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{Handler: &handler}),
	})

	reader.parseConditionsIfStringified(`[{`)
	reader.parseConditionsIfStringified(`[{`)
	if reported != 2 {
		t.Fatalf("expected every malformed parse to report an error, got %d", reported)
	}
	if count := reader.getCachedExpressionCount(); count != 0 {
		t.Fatalf("expected malformed expressions not to be retained, got %d", count)
	}
}

func TestInstanceEvaluationDataProviderErrorHandling(t *testing.T) {
	diagnostics := newDiagnosticReporter(diagnosticReporterOptions{})
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

	reader := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           datafile,
		diagnosticReporter: diagnostics,
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
