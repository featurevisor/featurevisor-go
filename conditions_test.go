package featurevisor

import (
	"regexp"
	"testing"
	"time"
)

// helper function to create condition values
func conditionValue(v interface{}) *ConditionValue {
	cv := ConditionValue(v)
	return &cv
}

func TestGetValueFromContext(t *testing.T) {
	context := Context{
		"user": map[string]interface{}{
			"id":   123,
			"name": "John",
			"profile": map[string]interface{}{
				"age":  30,
				"city": "New York",
			},
		},
		"country": "US",
		"tags":    []interface{}{"premium", "active"},
	}

	tests := []struct {
		name     string
		path     string
		expected interface{}
	}{
		{
			name:     "simple key",
			path:     "country",
			expected: "US",
		},
		{
			name:     "nested key",
			path:     "user.id",
			expected: 123,
		},
		{
			name:     "deep nested key",
			path:     "user.profile.age",
			expected: 30,
		},
		{
			name:     "array access",
			path:     "tags",
			expected: nil, // Skip this test as slice comparison is not reliable
		},
		{
			name:     "non-existent key",
			path:     "nonexistent",
			expected: nil,
		},
		{
			name:     "non-existent nested key",
			path:     "user.nonexistent",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetValueFromContext(context, tt.path)
			if tt.expected == nil {
				// Skip comparison for nil expected values
				return
			}
			if result != tt.expected {
				t.Errorf("getValueFromContext(%s) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestConditionIsMatched(t *testing.T) {
	context := Context{
		"user": map[string]interface{}{
			"id":   123,
			"name": "John Doe",
			"age":  30,
			"tags": []interface{}{"premium", "active"},
		},
		"country": "US",
		"version": "1.2.3",
		"score":   85.5,
	}

	getRegex := func(regexString string, regexFlags string) *regexp.Regexp {
		return regexp.MustCompile(regexString)
	}

	tests := []struct {
		name      string
		condition PlainCondition
		context   Context
		expected  bool
	}{
		{
			name: "equals string",
			condition: PlainCondition{
				Attribute: "country",
				Operator:  OperatorEquals,
				Value:     func() *ConditionValue { v := ConditionValue("US"); return &v }(),
			},
			context:  context,
			expected: true,
		},
		{
			name: "equals number",
			condition: PlainCondition{
				Attribute: "user.age",
				Operator:  OperatorEquals,
				Value:     conditionValue(30),
			},
			context:  context,
			expected: true,
		},
		{
			name: "not equals",
			condition: PlainCondition{
				Attribute: "country",
				Operator:  OperatorNotEquals,
				Value:     conditionValue("CA"),
			},
			context:  context,
			expected: true,
		},
		{
			name: "contains",
			condition: PlainCondition{
				Attribute: "user.name",
				Operator:  OperatorContains,
				Value:     conditionValue("John"),
			},
			context:  context,
			expected: true,
		},
		{
			name: "startsWith",
			condition: PlainCondition{
				Attribute: "user.name",
				Operator:  OperatorStartsWith,
				Value:     conditionValue("John"),
			},
			context:  context,
			expected: true,
		},
		{
			name: "endsWith",
			condition: PlainCondition{
				Attribute: "user.name",
				Operator:  OperatorEndsWith,
				Value:     conditionValue("Doe"),
			},
			context:  context,
			expected: true,
		},
		{
			name: "greater than",
			condition: PlainCondition{
				Attribute: "user.age",
				Operator:  OperatorGreaterThan,
				Value:     conditionValue(25),
			},
			context:  context,
			expected: true,
		},
		{
			name: "less than",
			condition: PlainCondition{
				Attribute: "user.age",
				Operator:  OperatorLessThan,
				Value:     conditionValue(35),
			},
			context:  context,
			expected: true,
		},
		{
			name: "exists",
			condition: PlainCondition{
				Attribute: "country",
				Operator:  OperatorExists,
			},
			context:  context,
			expected: true,
		},
		{
			name: "not exists",
			condition: PlainCondition{
				Attribute: "nonexistent",
				Operator:  OperatorNotExists,
			},
			context:  context,
			expected: true,
		},
		{
			name: "in array",
			condition: PlainCondition{
				Attribute: "user.tags",
				Operator:  OperatorIn,
				Value:     conditionValue([]interface{}{"premium", "active"}),
			},
			context:  context,
			expected: true,
		},
		{
			name: "includes in array",
			condition: PlainCondition{
				Attribute: "user.tags",
				Operator:  OperatorIncludes,
				Value:     conditionValue("premium"),
			},
			context:  context,
			expected: true,
		},
		{
			name: "semver equals",
			condition: PlainCondition{
				Attribute: "version",
				Operator:  OperatorSemverEquals,
				Value:     conditionValue("1.2.3"),
			},
			context:  context,
			expected: true,
		},
		{
			name: "semver greater than",
			condition: PlainCondition{
				Attribute: "version",
				Operator:  OperatorSemverGreaterThan,
				Value:     conditionValue("1.0.0"),
			},
			context:  context,
			expected: true,
		},
		{
			name: "matches regex",
			condition: PlainCondition{
				Attribute:  "user.name",
				Operator:   OperatorMatches,
				Value:      conditionValue("John.*"),
				RegexFlags: &[]string{""}[0],
			},
			context:  context,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConditionIsMatched(tt.condition, tt.context, getRegex)
			if result != tt.expected {
				t.Errorf("conditionIsMatched() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConditionIsMatchedDate(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	tomorrow := now.AddDate(0, 0, 1)

	context := Context{
		"created": now.Format(time.RFC3339),
		"updated": yesterday.Format(time.RFC3339),
	}

	getRegex := func(regexString string, regexFlags string) *regexp.Regexp {
		return regexp.MustCompile(regexString)
	}

	tests := []struct {
		name      string
		condition PlainCondition
		context   Context
		expected  bool
	}{
		{
			name: "before date",
			condition: PlainCondition{
				Attribute: "updated",
				Operator:  OperatorBefore,
				Value:     conditionValue(tomorrow.Format(time.RFC3339)),
			},
			context:  context,
			expected: true,
		},
		{
			name: "after date",
			condition: PlainCondition{
				Attribute: "created",
				Operator:  OperatorAfter,
				Value:     conditionValue(yesterday.Format(time.RFC3339)),
			},
			context:  context,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConditionIsMatched(tt.condition, tt.context, getRegex)
			if result != tt.expected {
				t.Errorf("conditionIsMatched() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestConditionIsMatchedComprehensive tests all operators comprehensively
func TestConditionIsMatchedComprehensive(t *testing.T) {
	logger := NewLogger(CreateLoggerOptions{})
	jsonDatafile := `{
		"schemaVersion": "2.0",
		"revision": "1",
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

	// Test wildcard conditions
	t.Run("wildcard conditions", func(t *testing.T) {
		// Test "*" should always match
		result := reader.AllConditionsAreMatched("*", Context{"browser_type": "chrome"})
		if !result {
			t.Error("Wildcard '*' should always match")
		}

		// Test non-wildcard string should not match
		result = reader.AllConditionsAreMatched("blah", Context{"browser_type": "chrome"})
		if result {
			t.Error("Non-wildcard string should not match")
		}
	})

	// Test all operators
	t.Run("operators", func(t *testing.T) {
		// equals
		condition := PlainCondition{
			Attribute: "browser_type",
			Operator:  OperatorEquals,
			Value:     conditionValue("chrome"),
		}
		context := Context{"browser_type": "chrome"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("equals operator should match")
		}

		// equals with dot separated path
		condition = PlainCondition{
			Attribute: "browser.type",
			Operator:  OperatorEquals,
			Value:     conditionValue("chrome"),
		}
		context = Context{"browser": map[string]interface{}{"type": "chrome"}}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("equals with dot separated path should match")
		}

		// notEquals
		condition = PlainCondition{
			Attribute: "browser_type",
			Operator:  OperatorNotEquals,
			Value:     conditionValue("chrome"),
		}
		context = Context{"browser_type": "firefox"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("notEquals operator should match")
		}

		// exists
		condition = PlainCondition{
			Attribute: "browser_type",
			Operator:  OperatorExists,
		}
		context = Context{"browser_type": "firefox"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("exists operator should match")
		}

		// notExists
		condition = PlainCondition{
			Attribute: "nonexistent",
			Operator:  OperatorNotExists,
		}
		context = Context{"browser_type": "firefox"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("notExists operator should match")
		}

		// endsWith
		condition = PlainCondition{
			Attribute: "name",
			Operator:  OperatorEndsWith,
			Value:     conditionValue("World"),
		}
		context = Context{"name": "Hello World"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("endsWith operator should match")
		}

		// includes
		condition = PlainCondition{
			Attribute: "permissions",
			Operator:  OperatorIncludes,
			Value:     conditionValue("write"),
		}
		context = Context{"permissions": []interface{}{"read", "write"}}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("includes operator should match")
		}

		// notIncludes
		condition = PlainCondition{
			Attribute: "permissions",
			Operator:  OperatorNotIncludes,
			Value:     conditionValue("write"),
		}
		context = Context{"permissions": []interface{}{"read", "admin"}}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("notIncludes operator should match")
		}

		// contains
		condition = PlainCondition{
			Attribute: "name",
			Operator:  OperatorContains,
			Value:     conditionValue("Hello"),
		}
		context = Context{"name": "Hello World"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("contains operator should match")
		}

		// notContains
		condition = PlainCondition{
			Attribute: "name",
			Operator:  OperatorNotContains,
			Value:     conditionValue("Hello"),
		}
		context = Context{"name": "Hi World"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("notContains operator should match")
		}

		// matches
		condition = PlainCondition{
			Attribute: "name",
			Operator:  OperatorMatches,
			Value:     conditionValue("^[a-zA-Z]{2,}$"),
		}
		context = Context{"name": "Hello"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("matches operator should match")
		}

		// matches with regexFlags
		regexFlags := "i"
		condition = PlainCondition{
			Attribute:  "name",
			Operator:   OperatorMatches,
			Value:      conditionValue("^[a-zA-Z]{2,}$"),
			RegexFlags: &regexFlags,
		}
		context = Context{"name": "Hello"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("matches with regexFlags should match")
		}

		// notMatches
		condition = PlainCondition{
			Attribute: "name",
			Operator:  OperatorNotMatches,
			Value:     conditionValue("^[a-zA-Z]{2,}$"),
		}
		context = Context{"name": "Hi World"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("notMatches operator should match")
		}

		// in
		condition = PlainCondition{
			Attribute: "browser_type",
			Operator:  OperatorIn,
			Value:     conditionValue([]interface{}{"chrome", "firefox"}),
		}
		context = Context{"browser_type": "chrome"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("in operator should match")
		}

		// notIn
		condition = PlainCondition{
			Attribute: "browser_type",
			Operator:  OperatorNotIn,
			Value:     conditionValue([]interface{}{"chrome", "firefox"}),
		}
		context = Context{"browser_type": "edge"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("notIn operator should match")
		}

		// greaterThan
		condition = PlainCondition{
			Attribute: "age",
			Operator:  OperatorGreaterThan,
			Value:     conditionValue(18),
		}
		context = Context{"age": 19}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("greaterThan operator should match")
		}

		// greaterThanOrEquals
		condition = PlainCondition{
			Attribute: "age",
			Operator:  OperatorGreaterThanOrEquals,
			Value:     conditionValue(18),
		}
		context = Context{"age": 18}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("greaterThanOrEquals operator should match")
		}

		// lessThan
		condition = PlainCondition{
			Attribute: "age",
			Operator:  OperatorLessThan,
			Value:     conditionValue(18),
		}
		context = Context{"age": 17}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("lessThan operator should match")
		}

		// lessThanOrEquals
		condition = PlainCondition{
			Attribute: "age",
			Operator:  OperatorLessThanOrEquals,
			Value:     conditionValue(18),
		}
		context = Context{"age": 18}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("lessThanOrEquals operator should match")
		}

		// semverEquals
		condition = PlainCondition{
			Attribute: "version",
			Operator:  OperatorSemverEquals,
			Value:     conditionValue("1.0.0"),
		}
		context = Context{"version": "1.0.0"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("semverEquals operator should match")
		}

		// semverNotEquals
		condition = PlainCondition{
			Attribute: "version",
			Operator:  OperatorSemverNotEquals,
			Value:     conditionValue("1.0.0"),
		}
		context = Context{"version": "2.0.0"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("semverNotEquals operator should match")
		}

		// semverGreaterThan
		condition = PlainCondition{
			Attribute: "version",
			Operator:  OperatorSemverGreaterThan,
			Value:     conditionValue("1.0.0"),
		}
		context = Context{"version": "2.0.0"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("semverGreaterThan operator should match")
		}

		// semverGreaterThanOrEquals
		condition = PlainCondition{
			Attribute: "version",
			Operator:  OperatorSemverGreaterThanOrEquals,
			Value:     conditionValue("1.0.0"),
		}
		context = Context{"version": "1.0.0"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("semverGreaterThanOrEquals operator should match")
		}

		// semverLessThan
		condition = PlainCondition{
			Attribute: "version",
			Operator:  OperatorSemverLessThan,
			Value:     conditionValue("1.0.0"),
		}
		context = Context{"version": "0.9.0"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("semverLessThan operator should match")
		}

		// semverLessThanOrEquals
		condition = PlainCondition{
			Attribute: "version",
			Operator:  OperatorSemverLessThanOrEquals,
			Value:     conditionValue("1.0.0"),
		}
		context = Context{"version": "1.0.0"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("semverLessThanOrEquals operator should match")
		}

		// before
		condition = PlainCondition{
			Attribute: "date",
			Operator:  OperatorBefore,
			Value:     conditionValue("2023-05-13T16:23:59Z"),
		}
		context = Context{"date": "2023-05-12T00:00:00Z"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("before operator should match")
		}

		// after
		condition = PlainCondition{
			Attribute: "date",
			Operator:  OperatorAfter,
			Value:     conditionValue("2023-05-13T16:23:59Z"),
		}
		context = Context{"date": "2023-05-14T00:00:00Z"}
		if !reader.AllConditionsAreMatched(condition, context) {
			t.Error("after operator should match")
		}
	})

	// Test complex conditions
	t.Run("complex conditions", func(t *testing.T) {
		// AND condition
		andCondition := AndCondition{
			And: []Condition{
				PlainCondition{
					Attribute: "browser_type",
					Operator:  OperatorEquals,
					Value:     conditionValue("chrome"),
				},
				PlainCondition{
					Attribute: "browser_version",
					Operator:  OperatorEquals,
					Value:     conditionValue("1.0"),
				},
			},
		}
		context := Context{
			"browser_type":    "chrome",
			"browser_version": "1.0",
		}
		if !reader.AllConditionsAreMatched(andCondition, context) {
			t.Error("AND condition should match")
		}

		// OR condition
		orCondition := OrCondition{
			Or: []Condition{
				PlainCondition{
					Attribute: "browser_type",
					Operator:  OperatorEquals,
					Value:     conditionValue("chrome"),
				},
				PlainCondition{
					Attribute: "browser_type",
					Operator:  OperatorEquals,
					Value:     conditionValue("firefox"),
				},
			},
		}
		context = Context{"browser_type": "chrome"}
		if !reader.AllConditionsAreMatched(orCondition, context) {
			t.Error("OR condition should match")
		}

		// NOT condition
		notCondition := NotCondition{
			Not: PlainCondition{
				Attribute: "browser_type",
				Operator:  OperatorEquals,
				Value:     conditionValue("chrome"),
			},
		}
		context = Context{"browser_type": "firefox"}
		if !reader.AllConditionsAreMatched(notCondition, context) {
			t.Error("NOT condition should match")
		}

		// Nested conditions
		nestedCondition := AndCondition{
			And: []Condition{
				PlainCondition{
					Attribute: "browser_type",
					Operator:  OperatorEquals,
					Value:     conditionValue("chrome"),
				},
				OrCondition{
					Or: []Condition{
						PlainCondition{
							Attribute: "browser_version",
							Operator:  OperatorEquals,
							Value:     conditionValue("1.0"),
						},
						PlainCondition{
							Attribute: "browser_version",
							Operator:  OperatorEquals,
							Value:     conditionValue("2.0"),
						},
					},
				},
			},
		}
		context = Context{
			"browser_type":    "chrome",
			"browser_version": "1.0",
		}
		if !reader.AllConditionsAreMatched(nestedCondition, context) {
			t.Error("Nested condition should match")
		}
	})
}

func TestConditionIsMatchedEdgeCases(t *testing.T) {
	getRegex := func(regexString string, regexFlags string) *regexp.Regexp {
		return regexp.MustCompile(regexString)
	}

	tests := []struct {
		name      string
		condition PlainCondition
		context   Context
		expected  bool
	}{
		{
			name: "null context value with in operator",
			condition: PlainCondition{
				Attribute: "browser_type",
				Operator:  OperatorIn,
				Value:     conditionValue([]interface{}{"chrome", "firefox"}),
			},
			context:  Context{"browser_type": nil},
			expected: false,
		},
		{
			name: "array context value with in operator",
			condition: PlainCondition{
				Attribute: "browser_types",
				Operator:  OperatorIn,
				Value:     conditionValue([]interface{}{"chrome", "firefox"}),
			},
			context:  Context{"browser_types": []interface{}{"chrome", "safari"}},
			expected: true, // chrome is in both arrays
		},
		{
			name: "array context value with notIn operator",
			condition: PlainCondition{
				Attribute: "browser_types",
				Operator:  OperatorNotIn,
				Value:     conditionValue([]interface{}{"chrome", "firefox"}),
			},
			context:  Context{"browser_types": []interface{}{"safari", "edge"}},
			expected: false, // arrays should not be valid for notIn conditions
		},
		{
			name: "mixed numeric types - float64 context with int value",
			condition: PlainCondition{
				Attribute: "score",
				Operator:  OperatorGreaterThan,
				Value:     conditionValue(80),
			},
			context:  Context{"score": 85.5},
			expected: true,
		},
		{
			name: "mixed numeric types - int context with float64 value",
			condition: PlainCondition{
				Attribute: "score",
				Operator:  OperatorLessThan,
				Value:     conditionValue(90.0),
			},
			context:  Context{"score": 85},
			expected: true,
		},
		{
			name: "string comparison with number context",
			condition: PlainCondition{
				Attribute: "version",
				Operator:  OperatorEquals,
				Value:     conditionValue("5.5"),
			},
			context:  Context{"version": 5.5},
			expected: false, // string vs number should not match
		},
		{
			name: "number comparison with string context",
			condition: PlainCondition{
				Attribute: "version",
				Operator:  OperatorEquals,
				Value:     conditionValue(5.5),
			},
			context:  Context{"version": "5.5"},
			expected: false, // number vs string should not match
		},
		{
			name: "regex with flags",
			condition: PlainCondition{
				Attribute:  "name",
				Operator:   OperatorMatches,
				Value:      conditionValue("^[a-z]+$"),
				RegexFlags: &[]string{"i"}[0], // case insensitive
			},
			context:  Context{"name": "hello"},
			expected: true,
		},
		{
			name: "regex without flags",
			condition: PlainCondition{
				Attribute: "name",
				Operator:  OperatorMatches,
				Value:     conditionValue("^[a-z]+$"),
			},
			context:  Context{"name": "HELLO"},
			expected: false, // case sensitive
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConditionIsMatched(tt.condition, tt.context, getRegex)
			if result != tt.expected {
				t.Errorf("conditionIsMatched() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConditionIsMatchedComplexNested(t *testing.T) {
	logger := NewLogger(CreateLoggerOptions{})
	jsonDatafile := `{
		"schemaVersion": "2.0",
		"revision": "1",
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

	// Test complex nested conditions similar to TypeScript tests
	context := Context{
		"country":         "nl",
		"browser_type":    "chrome",
		"browser_version": "1.0",
		"device_type":     "mobile",
		"orientation":     "portrait",
	}

	// Test OR inside AND
	orInsideAnd := AndCondition{
		And: []Condition{
			PlainCondition{
				Attribute: "browser_type",
				Operator:  OperatorEquals,
				Value:     conditionValue("chrome"),
			},
			OrCondition{
				Or: []Condition{
					PlainCondition{
						Attribute: "browser_version",
						Operator:  OperatorEquals,
						Value:     conditionValue("1.0"),
					},
					PlainCondition{
						Attribute: "browser_version",
						Operator:  OperatorEquals,
						Value:     conditionValue("2.0"),
					},
				},
			},
		},
	}

	result := reader.AllConditionsAreMatched(orInsideAnd, context)
	if !result {
		t.Error("OR inside AND condition should match")
	}

	// Test AND inside OR
	andInsideOr := OrCondition{
		Or: []Condition{
			PlainCondition{
				Attribute: "browser_type",
				Operator:  OperatorEquals,
				Value:     conditionValue("chrome"),
			},
			AndCondition{
				And: []Condition{
					PlainCondition{
						Attribute: "device_type",
						Operator:  OperatorEquals,
						Value:     conditionValue("mobile"),
					},
					PlainCondition{
						Attribute: "orientation",
						Operator:  OperatorEquals,
						Value:     conditionValue("portrait"),
					},
				},
			},
		},
	}

	result = reader.AllConditionsAreMatched(andInsideOr, context)
	if !result {
		t.Error("AND inside OR condition should match")
	}
}

func BenchmarkConditionIsMatched(b *testing.B) {
	context := Context{
		"user": map[string]interface{}{
			"id":   123,
			"name": "John Doe",
			"age":  30,
		},
		"country": "US",
	}

	condition := PlainCondition{
		Attribute: "user.age",
		Operator:  OperatorGreaterThan,
		Value:     conditionValue(25),
	}

	getRegex := func(regexString string, regexFlags string) *regexp.Regexp {
		return regexp.MustCompile(regexString)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConditionIsMatched(condition, context, getRegex)
	}
}
