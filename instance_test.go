package featurevisor

import (
	"reflect"
	"testing"
)

func TestCreateInstance(t *testing.T) {
	// Test that CreateInstance is a function
	instance := CreateInstance(Options{})
	if instance == nil {
		t.Fatal("Expected instance to be created")
	}

	// Test creating an instance with datafile content
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {},
		"segments": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	instance = CreateInstance(Options{
		Datafile: datafile,
	})

	if instance == nil {
		t.Fatal("Expected instance to be created")
	}

	// Test that the instance has the correct methods
	// Note: GetVariation is a method, not a function, so we can't check if it's nil
	// Instead, we'll test that the method works correctly
	context := Context{"userId": "123"}
	variation := instance.GetVariation("test", context, OverrideOptions{})
	if variation != nil {
		t.Error("Expected GetVariation to return nil for non-existent feature")
	}
}

func TestPlainBucketBy(t *testing.T) {
	var capturedBucketKey string

	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"test": {
				"key": "test",
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
							{"variation": "control", "range": [0, 100000]},
							{"variation": "treatment", "range": [0, 0]}
						]
					}
				]
			}
		},
		"segments": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	instance := CreateInstance(Options{
		Datafile: datafile,
		Modules: []*FeaturevisorModule{
			{
				Name: "unit-test",
				BucketKey: func(options ConfigureBucketKeyOptions) string {
					capturedBucketKey = options.BucketKey
					return options.BucketKey
				},
			},
		},
	})

	context := Context{"userId": "123"}

	if !instance.IsEnabled("test", context, OverrideOptions{}) {
		t.Error("Expected feature to be enabled")
	}

	variation := instance.GetVariation("test", context, OverrideOptions{})
	if variation == nil || *variation != "control" {
		t.Errorf("Expected variation to be 'control', got '%v'", variation)
	}

	if capturedBucketKey != "123.test" {
		t.Errorf("Expected bucket key to be '123.test', got '%s'", capturedBucketKey)
	}
}

func TestAndBucketBy(t *testing.T) {
	var capturedBucketKey string

	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"test": {
				"key": "test",
				"bucketBy": ["userId", "organizationId"],
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
							{"variation": "control", "range": [0, 100000]},
							{"variation": "treatment", "range": [0, 0]}
						]
					}
				]
			}
		},
		"segments": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	instance := CreateInstance(Options{
		Datafile: datafile,
		Modules: []*FeaturevisorModule{
			{
				Name: "unit-test",
				BucketKey: func(options ConfigureBucketKeyOptions) string {
					capturedBucketKey = options.BucketKey
					return options.BucketKey
				},
			},
		},
	})

	context := Context{
		"userId":         "123",
		"organizationId": "456",
	}

	variation := instance.GetVariation("test", context, OverrideOptions{})
	if variation == nil || *variation != "control" {
		t.Errorf("Expected variation to be 'control', got '%v'", variation)
	}

	if capturedBucketKey != "123.456.test" {
		t.Errorf("Expected bucket key to be '123.456.test', got '%s'", capturedBucketKey)
	}
}

func TestOrBucketBy(t *testing.T) {
	var capturedBucketKey string

	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"test": {
				"key": "test",
				"bucketBy": {
					"or": ["userId", "deviceId"]
				},
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
							{"variation": "control", "range": [0, 100000]},
							{"variation": "treatment", "range": [0, 0]}
						]
					}
				]
			}
		},
		"segments": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	instance := CreateInstance(Options{
		Datafile: datafile,
		Modules: []*FeaturevisorModule{
			{
				Name: "unit-test",
				BucketKey: func(options ConfigureBucketKeyOptions) string {
					capturedBucketKey = options.BucketKey
					return options.BucketKey
				},
			},
		},
	})

	// Test with both userId and deviceId
	context := Context{
		"userId":   "123",
		"deviceId": "456",
	}

	if !instance.IsEnabled("test", context, OverrideOptions{}) {
		t.Error("Expected feature to be enabled")
	}

	variation := instance.GetVariation("test", context, OverrideOptions{})
	if variation == nil || *variation != "control" {
		t.Errorf("Expected variation to be 'control', got '%v'", variation)
	}

	if capturedBucketKey != "123.test" {
		t.Errorf("Expected bucket key to be '123.test', got '%s'", capturedBucketKey)
	}

	// Test with only deviceId
	context = Context{"deviceId": "456"}
	variation = instance.GetVariation("test", context, OverrideOptions{})
	if variation == nil || *variation != "control" {
		t.Errorf("Expected variation to be 'control', got '%v'", variation)
	}

	if capturedBucketKey != "456.test" {
		t.Errorf("Expected bucket key to be '456.test', got '%s'", capturedBucketKey)
	}
}

func TestBeforeModule(t *testing.T) {
	var intercepted bool
	var interceptedFeatureKey string
	var interceptedVariableKey string

	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"test": {
				"key": "test",
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
							{"variation": "control", "range": [0, 100000]},
							{"variation": "treatment", "range": [0, 0]}
						]
					}
				]
			}
		},
		"segments": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	instance := CreateInstance(Options{
		Datafile: datafile,
		Modules: []*FeaturevisorModule{
			{
				Name: "unit-test",
				Before: func(options EvaluateOptions) EvaluateOptions {
					intercepted = true
					interceptedFeatureKey = string(options.FeatureKey)
					if options.VariableKey != nil {
						interceptedVariableKey = string(*options.VariableKey)
					}
					return options
				},
			},
		},
	})

	context := Context{"userId": "123"}
	variation := instance.GetVariation("test", context, OverrideOptions{})

	if variation == nil || *variation != "control" {
		t.Errorf("Expected variation to be 'control', got '%v'", variation)
	}

	if !intercepted {
		t.Error("Expected before module to be called")
	}

	if interceptedFeatureKey != "test" {
		t.Errorf("Expected feature key to be 'test', got '%s'", interceptedFeatureKey)
	}

	if interceptedVariableKey != "" {
		t.Errorf("Expected variable key to be empty, got '%s'", interceptedVariableKey)
	}
}

func TestAfterModule(t *testing.T) {
	var intercepted bool
	var interceptedFeatureKey string
	var interceptedVariableKey string

	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"features": {
			"test": {
				"key": "test",
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
							{"variation": "control", "range": [0, 100000]},
							{"variation": "treatment", "range": [0, 0]}
						]
					}
				]
			}
		},
		"segments": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	instance := CreateInstance(Options{
		Datafile: datafile,
		Modules: []*FeaturevisorModule{
			{
				Name: "unit-test",
				After: func(evaluation Evaluation, options EvaluateOptions) Evaluation {
					intercepted = true
					interceptedFeatureKey = string(options.FeatureKey)
					if options.VariableKey != nil {
						interceptedVariableKey = string(*options.VariableKey)
					}
					// Manipulate the value
					interceptedValue := "control_intercepted"
					evaluation.VariationValue = &interceptedValue
					return evaluation
				},
			},
		},
	})

	context := Context{"userId": "123"}
	variation := instance.GetVariation("test", context, OverrideOptions{})

	if variation == nil || *variation != "control_intercepted" {
		t.Errorf("Expected variation to be 'control_intercepted', got '%v'", variation)
	}

	if !intercepted {
		t.Error("Expected after module to be called")
	}

	if interceptedFeatureKey != "test" {
		t.Errorf("Expected feature key to be 'test', got '%s'", interceptedFeatureKey)
	}

	if interceptedVariableKey != "" {
		t.Errorf("Expected variable key to be empty, got '%s'", interceptedVariableKey)
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

func TestModulesSetupDiagnosticsAndClose(t *testing.T) {
	var setupCalled bool
	var closeCalled bool
	var setupRevision string
	var moduleSawDatafileSet bool
	var diagnostics []FeaturevisorDiagnostic

	instance := CreateInstance(Options{
		Datafile: DatafileContent{
			SchemaVersion: "2",
			Revision:      "module-test",
			Segments:      map[SegmentKey]Segment{},
			Features:      map[FeatureKey]Feature{},
		},
		OnDiagnostic: func(diagnostic FeaturevisorDiagnostic) {
			diagnostics = append(diagnostics, diagnostic)
		},
		Modules: []*FeaturevisorModule{
			{
				Name: "observer",
				Setup: func(api FeaturevisorModuleApi) {
					setupCalled = true
					setupRevision = api.GetRevision()
					api.OnDiagnostic(func(diagnostic FeaturevisorDiagnostic) {
						if diagnostic.Code == "datafile_set" {
							moduleSawDatafileSet = true
						}
					})
					api.ReportDiagnostic(FeaturevisorModuleReportedDiagnostic{
						Level:   LogLevelInfo,
						Code:    "module_ready",
						Message: "Module ready",
					})
				},
				Close: func() {
					closeCalled = true
				},
			},
		},
	})

	if !setupCalled {
		t.Fatal("expected module setup to be called")
	}
	if setupRevision != "unknown" {
		t.Fatalf("expected setup API revision to be unknown before initial datafile is set, got %s", setupRevision)
	}
	if !moduleSawDatafileSet {
		t.Fatal("expected module diagnostic subscription to observe initial datafile_set")
	}

	foundModuleDiagnostic := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "module_ready" && diagnostic.Module == "observer" {
			foundModuleDiagnostic = true
			break
		}
	}
	if !foundModuleDiagnostic {
		t.Fatal("expected module reported diagnostic")
	}

	instance.AddModule(&FeaturevisorModule{Name: "observer"})

	foundDuplicateDiagnostic := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "duplicate_module" && diagnostic.Level == LogLevelError {
			foundDuplicateDiagnostic = true
			break
		}
	}

	if !foundDuplicateDiagnostic {
		t.Fatal("expected duplicate module diagnostic")
	}

	instance.Close()

	if !closeCalled {
		t.Fatal("expected module close to be called")
	}
}

func TestRemovedModulesAreClosed(t *testing.T) {
	closed := []string{}
	instance := CreateInstance(Options{})

	unsubscribe := instance.AddModule(&FeaturevisorModule{
		Name: "dynamic",
		Close: func() {
			closed = append(closed, "dynamic")
		},
	})

	if unsubscribe == nil {
		t.Fatal("expected add module to return unsubscribe")
	}

	unsubscribe()
	unsubscribe()

	instance.AddModule(&FeaturevisorModule{
		Name: "dynamic",
		Close: func() {
			closed = append(closed, "dynamic-again")
		},
	})
	instance.RemoveModule("dynamic")

	if len(closed) != 2 || closed[0] != "dynamic" || closed[1] != "dynamic-again" {
		t.Fatalf("expected removed modules to be closed once each, got %#v", closed)
	}
}

func TestModuleCloseErrorsAreReportedAndDoNotStopCleanup(t *testing.T) {
	var diagnostics []FeaturevisorDiagnostic
	var errorEvents []FeaturevisorDiagnostic
	closed := []string{}

	instance := CreateInstance(Options{
		OnDiagnostic: func(diagnostic FeaturevisorDiagnostic) {
			diagnostics = append(diagnostics, diagnostic)
		},
	})

	instance.On(EventNameError, func(details EventDetails) {
		if diagnostic, ok := details["diagnostic"].(FeaturevisorDiagnostic); ok {
			errorEvents = append(errorEvents, diagnostic)
		}
	})

	instance.AddModule(&FeaturevisorModule{
		Name: "first",
		Close: func() {
			closed = append(closed, "first")
			panic("first close failed")
		},
	})
	instance.AddModule(&FeaturevisorModule{
		Name: "second",
		Close: func() {
			closed = append(closed, "second")
		},
	})

	instance.Close()

	if len(closed) != 2 || closed[0] != "first" || closed[1] != "second" {
		t.Fatalf("expected close cleanup to continue after a module panics, got %#v", closed)
	}

	foundDiagnostic := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "module_close_error" &&
			diagnostic.Level == LogLevelError &&
			diagnostic.ModuleName == "first" &&
			diagnostic.OriginalError != nil {
			foundDiagnostic = true
			break
		}
	}
	if !foundDiagnostic {
		t.Fatalf("expected module_close_error diagnostic, got %#v", diagnostics)
	}

	foundErrorEvent := false
	for _, diagnostic := range errorEvents {
		if diagnostic.Code == "module_close_error" && diagnostic.ModuleName == "first" {
			foundErrorEvent = true
			break
		}
	}
	if !foundErrorEvent {
		t.Fatalf("expected module_close_error to trigger error event, got %#v", errorEvents)
	}
}

func TestModuleUnsubscribeReportsCloseErrors(t *testing.T) {
	var diagnostics []FeaturevisorDiagnostic
	instance := CreateInstance(Options{
		OnDiagnostic: func(diagnostic FeaturevisorDiagnostic) {
			diagnostics = append(diagnostics, diagnostic)
		},
	})

	unsubscribe := instance.AddModule(&FeaturevisorModule{
		Name: "dynamic",
		Close: func() {
			panic("dynamic close failed")
		},
	})
	if unsubscribe == nil {
		t.Fatal("expected add module to return unsubscribe")
	}

	unsubscribe()
	unsubscribe()

	foundDiagnostic := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "module_close_error" && diagnostic.ModuleName == "dynamic" {
			foundDiagnostic = true
			break
		}
	}
	if !foundDiagnostic {
		t.Fatalf("expected module_close_error diagnostic from unsubscribe, got %#v", diagnostics)
	}
}

func TestSetDatafileMergesByDefaultAndReplacesWhenRequested(t *testing.T) {
	instance := CreateInstance(Options{
		Datafile: DatafileContent{
			SchemaVersion: "2",
			Revision:      "1",
			Segments:      map[SegmentKey]Segment{},
			Features: map[FeatureKey]Feature{
				"first": {
					BucketBy: "userId",
					Traffic:  []Traffic{},
				},
			},
		},
	})

	secondDatafile := DatafileContent{
		SchemaVersion: "2",
		Revision:      "2",
		Segments:      map[SegmentKey]Segment{},
		Features: map[FeatureKey]Feature{
			"second": {
				BucketBy: "userId",
				Traffic:  []Traffic{},
			},
		},
	}

	instance.SetDatafile(secondDatafile)

	if instance.GetFeature("first") == nil {
		t.Fatal("expected default SetDatafile to preserve existing features")
	}
	if instance.GetFeature("second") == nil {
		t.Fatal("expected default SetDatafile to add incoming features")
	}

	instance.SetDatafile(secondDatafile, true)

	if instance.GetFeature("first") != nil {
		t.Fatal("expected replace SetDatafile to remove existing features")
	}
	if instance.GetFeature("second") == nil {
		t.Fatal("expected replace SetDatafile to keep incoming features")
	}
}

func TestGetAllEvaluations(t *testing.T) {
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
					"paymentMethods": {
						"key": "paymentMethods",
						"type": "array",
						"defaultValue": ["paypal", "creditcard"]
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
			},
			"anotherTest": {
				"key": "anotherTest",
				"bucketBy": "userId",
				"traffic": [
					{
						"key": "1",
						"segments": "*",
						"percentage": 100000,
						"allocation": []
					}
				]
			}
		},
		"segments": {}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("Failed to parse datafile JSON: %v", err)
	}

	instance := CreateInstance(Options{
		Datafile: datafile,
	})

	context := Context{"userId": "123"}

	// Test GetAllEvaluations with specific feature keys
	featureKeys := []string{"test", "anotherTest"}
	evaluatedFeatures := instance.GetAllEvaluations(context, featureKeys, OverrideOptions{})

	// Validate test feature evaluation
	testFeature, exists := evaluatedFeatures["test"]
	if !exists {
		t.Fatal("Expected 'test' feature to be in evaluated features")
	}

	if !testFeature.Enabled {
		t.Error("Expected 'test' feature to be enabled")
	}

	if testFeature.Variation == nil || *testFeature.Variation != "treatment" {
		t.Errorf("Expected 'test' feature variation to be 'treatment', got '%v'", testFeature.Variation)
	}

	// Validate variables
	if testFeature.Variables == nil {
		t.Fatal("Expected 'test' feature to have variables")
	}

	// Check color variable
	if color, exists := testFeature.Variables["color"]; !exists {
		t.Error("Expected 'color' variable to exist")
	} else if color != "red" {
		t.Errorf("Expected 'color' variable to be 'red', got '%v'", color)
	}

	// Check showSidebar variable
	if showSidebar, exists := testFeature.Variables["showSidebar"]; !exists {
		t.Error("Expected 'showSidebar' variable to exist")
	} else if showSidebar != true {
		t.Errorf("Expected 'showSidebar' variable to be true, got '%v'", showSidebar)
	}

	// Check sidebarTitle variable
	if sidebarTitle, exists := testFeature.Variables["sidebarTitle"]; !exists {
		t.Error("Expected 'sidebarTitle' variable to exist")
	} else if sidebarTitle != "sidebar title from variation" {
		t.Errorf("Expected 'sidebarTitle' variable to be 'sidebar title from variation', got '%v'", sidebarTitle)
	}

	// Check count variable
	if count, exists := testFeature.Variables["count"]; !exists {
		t.Error("Expected 'count' variable to exist")
	} else if count != 0 {
		t.Errorf("Expected 'count' variable to be 0, got '%v'", count)
	}

	// Check price variable
	if price, exists := testFeature.Variables["price"]; !exists {
		t.Error("Expected 'price' variable to exist")
	} else if price != 9.99 {
		t.Errorf("Expected 'price' variable to be 9.99, got '%v'", price)
	}

	// Check paymentMethods variable
	if paymentMethods, exists := testFeature.Variables["paymentMethods"]; !exists {
		t.Error("Expected 'paymentMethods' variable to exist")
	} else {
		expectedMethods := []interface{}{"paypal", "creditcard"}
		if !reflect.DeepEqual(paymentMethods, expectedMethods) {
			t.Errorf("Expected 'paymentMethods' variable to be %v, got '%v'", expectedMethods, paymentMethods)
		}
	}

	// Check flatConfig variable
	if flatConfig, exists := testFeature.Variables["flatConfig"]; !exists {
		t.Error("Expected 'flatConfig' variable to exist")
	} else {
		expectedConfig := map[string]interface{}{"key": "value"}
		if !reflect.DeepEqual(flatConfig, expectedConfig) {
			t.Errorf("Expected 'flatConfig' variable to be %v, got '%v'", expectedConfig, flatConfig)
		}
	}

	// Check nestedConfig variable
	if nestedConfig, exists := testFeature.Variables["nestedConfig"]; !exists {
		t.Error("Expected 'nestedConfig' variable to exist")
	} else {
		expectedNestedConfig := map[string]interface{}{
			"key": map[string]interface{}{"nested": "value"},
		}
		if !reflect.DeepEqual(nestedConfig, expectedNestedConfig) {
			t.Errorf("Expected 'nestedConfig' variable to be %v, got '%v'", expectedNestedConfig, nestedConfig)
		}
	}

	// Validate anotherTest feature evaluation
	anotherTestFeature, exists := evaluatedFeatures["anotherTest"]
	if !exists {
		t.Fatal("Expected 'anotherTest' feature to be in evaluated features")
	}

	if !anotherTestFeature.Enabled {
		t.Error("Expected 'anotherTest' feature to be enabled")
	}

	// anotherTest should not have variation or variables
	if anotherTestFeature.Variation != nil {
		t.Errorf("Expected 'anotherTest' feature to not have variation, got '%v'", anotherTestFeature.Variation)
	}

	if anotherTestFeature.Variables != nil {
		t.Errorf("Expected 'anotherTest' feature to not have variables, got '%v'", anotherTestFeature.Variables)
	}

	// Test GetAllEvaluations with empty feature keys (should return all features)
	allEvaluatedFeatures := instance.GetAllEvaluations(context, []string{}, OverrideOptions{})

	// Should contain both features
	if _, exists := allEvaluatedFeatures["test"]; !exists {
		t.Error("Expected 'test' feature to be in all evaluated features")
	}

	if _, exists := allEvaluatedFeatures["anotherTest"]; !exists {
		t.Error("Expected 'anotherTest' feature to be in all evaluated features")
	}

	// Test with non-existent feature keys
	nonExistentFeatures := instance.GetAllEvaluations(context, []string{"nonExistent"}, OverrideOptions{})
	if len(nonExistentFeatures) != 1 {
		t.Errorf("Expected 1 feature for non-existent key, got %d features", len(nonExistentFeatures))
	}

	// The non-existent feature should be disabled
	if nonExistentFeature, exists := nonExistentFeatures["nonExistent"]; !exists {
		t.Error("Expected 'nonExistent' feature to be in evaluated features")
	} else if nonExistentFeature.Enabled {
		t.Error("Expected 'nonExistent' feature to be disabled")
	}
}
