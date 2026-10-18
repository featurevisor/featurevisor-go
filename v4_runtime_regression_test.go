package featurevisor

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestEmptyOverrideProgramSuppressesLegacyOverridesAndMetadata(t *testing.T) {
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"empty-program","segments":{"legacy":{"conditions":"*"}},"features":{},
		"variables":{"subject":{"type":"object","defaultValue":{},"overrides":[{"key":"legacy","segments":"*","value":{"legacy":true}}],"overrideProgram":[]}}
	}`)
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile})
	evaluation := instance.EvaluateGlobalVariable("subject")
	if evaluation.Reason != EvaluationReasonVariableDefault || evaluation.VariableOverrideKey != nil || evaluation.VariableOverrideIndex != nil || evaluation.VariableOverridePath != nil || evaluation.VariableOverridePaths != nil {
		t.Fatalf("empty program leaked legacy override metadata: %#v", evaluation)
	}
	if value := instance.GetGlobalVariable("subject"); !reflect.DeepEqual(value, map[string]interface{}{}) {
		t.Fatalf("empty object default was collapsed or overridden: %#v", value)
	}

	variable := datafile.Variables["subject"]
	segments, features, variables := variableDependencies(variable)
	if len(segments) != 0 || len(features) != 0 || len(variables) != 0 {
		t.Fatalf("inactive legacy overrides contributed dependencies: %#v %#v %#v", segments, features, variables)
	}
}

func TestEmptyJSONContainersRemainAvailable(t *testing.T) {
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"empty-json","segments":{},"features":{},
		"variables":{"array":{"type":"json","defaultValue":"[]"},"object":{"type":"json","defaultValue":"{}"}}
	}`)
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile})
	if value := instance.GetGlobalVariable("array"); !reflect.DeepEqual(value, []interface{}{}) {
		t.Fatalf("empty JSON array was collapsed: %#v", value)
	}
	if value := instance.GetGlobalVariable("object"); !reflect.DeepEqual(value, map[string]interface{}{}) {
		t.Fatalf("empty JSON object was collapsed: %#v", value)
	}
	child := instance.Spawn()
	if value := child.GetGlobalVariable("array"); !reflect.DeepEqual(value, []interface{}{}) {
		t.Fatalf("child collapsed empty JSON array: %#v", value)
	}
}

func TestMalformedJSONGetterReportsDiagnosticAndStillSatisfiesIsNull(t *testing.T) {
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"malformed-json","segments":{},
		"features":{"feature":{"bucketBy":"userId","variablesSchema":{"config":{"type":"json","defaultValue":"{"}},"traffic":[{"key":"all","segments":"*","percentage":100000}]}},
		"variables":{"source":{"type":"json","defaultValue":"{"},"subject":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","operator":"isNull"}]}}
	}`)
	diagnostics := []FeaturevisorDiagnostic{}
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, OnDiagnostic: func(diagnostic FeaturevisorDiagnostic) {
		diagnostics = append(diagnostics, diagnostic)
	}})
	child := instance.Spawn()

	checks := []struct {
		name string
		get  func() VariableValue
	}{
		{name: "global", get: func() VariableValue { return instance.GetGlobalVariable("source") }},
		{name: "feature", get: func() VariableValue { return instance.GetVariable("feature", "config", Context{"userId": "1"}) }},
		{name: "child global", get: func() VariableValue { return child.GetGlobalVariable("source") }},
		{name: "child feature", get: func() VariableValue { return child.GetVariable("feature", "config", Context{"userId": "1"}) }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			before := len(diagnostics)
			if value := check.get(); value != nil {
				t.Fatalf("malformed JSON getter returned %#v", value)
			}
			if len(diagnostics) != before+1 {
				t.Fatalf("expected one getter diagnostic, got %#v", diagnostics[before:])
			}
			diagnostic := diagnostics[len(diagnostics)-1]
			if diagnostic.Code != "evaluation_error" || diagnostic.Message != "getVariable failed" || diagnostic.OriginalError == nil {
				t.Fatalf("unexpected getter diagnostic: %#v", diagnostic)
			}
		})
	}

	before := len(diagnostics)
	evaluation := instance.EvaluateGlobalVariable("subject")
	value := instance.GetGlobalVariable("subject")
	if evaluation.Reason != EvaluationReasonVariableDefault || value != true {
		t.Fatalf("source getter failure did not satisfy outer isNull: evaluation=%#v value=%#v", evaluation, value)
	}
	if len(diagnostics) != before+2 {
		t.Fatalf("expected malformed dependency to report once per outer evaluation, got %#v", diagnostics[before:])
	}
}

func TestMalformedGlobalOverrideSelectorsFailLocally(t *testing.T) {
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"malformed-selectors","segments":{},"features":{},"variables":{
			"segments":{"type":"string","defaultValue":"default","overrideProgram":[{"key":"broken","segments":"{","operations":[{"op":"set","path":[],"value":"wrong"}],"onMatch":1,"onMiss":1}]},
			"conditions":{"type":"string","defaultValue":"default","overrideProgram":[{"key":"broken","conditions":"{","operations":[{"op":"set","path":[],"value":"wrong"}],"onMatch":1,"onMiss":1}]}
		}
	}`)
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile})
	if evaluation := instance.EvaluateGlobalVariable("segments"); evaluation.Reason != EvaluationReasonVariableDefault || evaluation.Error != nil {
		t.Fatalf("malformed stringified segments did not fail locally: %#v", evaluation)
	}
	if evaluation := instance.EvaluateGlobalVariable("conditions"); evaluation.Reason != EvaluationReasonVariableDefault || evaluation.Error != nil {
		t.Fatalf("ordinary malformed conditions should remain a reported selector miss: %#v", evaluation)
	}
}

func TestNativeVariableContainersMatchJSONForRequirementsAndMutations(t *testing.T) {
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"native-containers","segments":{},"features":{},"variables":{
			"source":{"type":"object","defaultValue":{}},
			"required":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"source","path":["items",0],"operator":"equals","value":"ok"}]},
			"mutated":{"type":"object","defaultValue":{},"overrideProgram":[]}
		}
	}`)
	source := datafile.Variables["source"]
	source.DefaultValue = VariableObjectValue{"items": []VariableValue{"ok"}}
	datafile.Variables["source"] = source
	mutated := datafile.Variables["mutated"]
	mutated.DefaultValue = VariableObjectValue{
		"items":   []VariableValue{"a"},
		"records": []VariableValue{VariableObjectValue{"id": 1, "value": "old"}},
	}
	mutated.OverrideProgram = []VariableOverrideStep{{
		Key: "native", Segments: "*", OnMatch: 1, OnMiss: 1,
		Operations: []VariableMutationOperation{
			{Op: "append", Path: []VariableMutationPathPart{"items"}, Value: "b", ValueSet: true},
			{Op: "set", Path: []VariableMutationPathPart{"records", VariableObjectValue{"select": VariableObjectValue{"property": "id", "values": []VariableValue{1.0}}}, "value"}, Value: "new", ValueSet: true},
		},
	}}
	datafile.Variables["mutated"] = mutated

	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile})
	if evaluation := instance.EvaluateGlobalVariable("required"); evaluation.Reason != EvaluationReasonVariableDefault || evaluation.VariableValue != true {
		t.Fatalf("native required variable path did not match JSON behaviour: %#v", evaluation)
	}
	expected := map[string]interface{}{
		"items":   []interface{}{"a", "b"},
		"records": []interface{}{map[string]interface{}{"id": 1, "value": "new"}},
	}
	if evaluation := instance.EvaluateGlobalVariable("mutated"); evaluation.Reason != EvaluationReasonVariableOverrideRule || !reflect.DeepEqual(evaluation.VariableValue, expected) {
		t.Fatalf("native mutation containers did not match JSON behaviour: %#v", evaluation)
	}
	if !reflect.DeepEqual(mutated.DefaultValue, VariableObjectValue{"items": []VariableValue{"a"}, "records": []VariableValue{VariableObjectValue{"id": 1, "value": "old"}}}) {
		t.Fatalf("native mutation input was modified: %#v", mutated.DefaultValue)
	}
}

func TestAccumulationIsImmutableAndModulesWrapWholeEvaluation(t *testing.T) {
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"accumulation","segments":{},"features":{},
		"variables":{"subject":{"type":"object","defaultValue":{"items":[],"nested":{"untouched":true}},"overrideProgram":[
			{"key":"first","keyPath":["root","first"],"segments":"*","operations":[{"op":"set","path":["enabled"],"value":true}],"onMatch":1,"onMiss":1},
			{"key":"second","keyPath":["root","second"],"segments":"*","operations":[{"op":"append","path":["items"],"value":"a"}],"onMatch":2,"onMiss":2}
		]}}
	}`)
	before, after := 0, 0
	module := &FeaturevisorModule{
		Name: "counts",
		BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
			before++
			return options
		},
		AfterEvaluation: func(evaluation Evaluation, options EvaluateOptions) Evaluation {
			after++
			return evaluation
		},
	}
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Modules: []*FeaturevisorModule{module}})
	evaluation := instance.EvaluateGlobalVariable("subject")
	if before != 1 || after != 1 {
		t.Fatalf("module callbacks ran per operation: before=%d after=%d", before, after)
	}
	if evaluation.Reason != EvaluationReasonVariableOverrideRule || !reflect.DeepEqual(evaluation.VariableOverridePaths, [][]string{{"root", "first"}, {"root", "second"}}) || evaluation.VariableOverrideKey == nil || *evaluation.VariableOverrideKey != "second" || evaluation.VariableOverrideIndex == nil || *evaluation.VariableOverrideIndex != 1 {
		t.Fatalf("unexpected accumulation metadata: %#v", evaluation)
	}
	original := datafile.Variables["subject"].DefaultValue
	if !reflect.DeepEqual(original, map[string]interface{}{"items": []interface{}{}, "nested": map[string]interface{}{"untouched": true}}) {
		t.Fatalf("override program mutated its datafile input: %#v", original)
	}
}

func TestRequiredVariableCyclesAreChainLocalUnderConcurrency(t *testing.T) {
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"cycles","segments":{},"features":{},"variables":{
			"a":{"type":"boolean","defaultValue":true,"conditions":[{"not":[{"variable":"b","operator":"equals","value":true}]}]},
			"b":{"type":"boolean","defaultValue":true,"conditions":[{"variable":"a","operator":"equals","value":true}]},
			"safe":{"type":"json","defaultValue":"{}"}
		}
	}`)
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile})
	const workers = 32
	errors := make(chan string, workers*2)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(2)
		go func() {
			defer wait.Done()
			if evaluation := instance.EvaluateGlobalVariable("a"); evaluation.Reason != EvaluationReasonError {
				errors <- "cycle did not propagate as an error"
			}
		}()
		go func() {
			defer wait.Done()
			if value := instance.GetGlobalVariable("safe"); !reflect.DeepEqual(value, map[string]interface{}{}) {
				errors <- "unrelated evaluation inherited cycle state"
			}
		}()
	}
	wait.Wait()
	close(errors)
	for message := range errors {
		t.Error(message)
	}
}

func TestRequiredVariableEvaluationIsReentrantAndDoesNotInheritCallerDefault(t *testing.T) {
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"reentrant","segments":{},"features":{},"variables":{
			"helper":{"type":"string","defaultValue":"ready"},
			"subject":{"type":"string","defaultValue":"matched","disabledValue":"disabled","conditions":[{"variable":"missing","operator":"equals","value":"caller"}]}
		}
	}`)
	var instance *Featurevisor
	reentered := false
	module := &FeaturevisorModule{Name: "reentrant", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		if options.GlobalVariable && options.VariableKey != nil && *options.VariableKey == "subject" && !reentered {
			reentered = true
			if value := instance.GetGlobalVariable("helper"); value != "ready" {
				t.Fatalf("reentrant getter returned %#v", value)
			}
		}
		return options
	}}
	instance = CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Modules: []*FeaturevisorModule{module}})
	evaluation := instance.EvaluateGlobalVariable("subject", OverrideOptions{DefaultVariableValue: "caller", DefaultVariableValueSet: true})
	if !reentered || evaluation.Reason != EvaluationReasonConditionsUnmet || evaluation.VariableValue != "disabled" {
		t.Fatalf("unexpected reentrant requirement evaluation: %#v", evaluation)
	}
}

func TestRequiredVariableUsesOrdinaryGetterFailureAsNull(t *testing.T) {
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"getter-error","segments":{},"features":{},"variables":{
			"source":{"type":"string","defaultValue":"value"},
			"subject":{"type":"string","defaultValue":"matched","disabledValue":"disabled","conditions":[{"variable":"source","operator":"isNull"}]}
		}
	}`)
	module := &FeaturevisorModule{Name: "throw-source", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		if options.GlobalVariable && options.VariableKey != nil && *options.VariableKey == "source" {
			panic("source callback failed")
		}
		return options
	}}
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Modules: []*FeaturevisorModule{module}})
	evaluation := instance.EvaluateGlobalVariable("subject")
	if evaluation.Reason != EvaluationReasonVariableDefault || evaluation.VariableValue != "matched" {
		t.Fatalf("ordinary dependency getter failure did not normalize to null: %#v", evaluation)
	}
}

func TestRequiredVariablePathUsesJavaScriptArrayProperties(t *testing.T) {
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"array-paths","segments":{},"features":{},"variables":{
			"array":{"type":"array","defaultValue":[1]},
			"object":{"type":"object","defaultValue":{"0":1}},
			"stringIndex":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"array","path":["0"],"operator":"equals","value":1}]},
			"numericIndex":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"array","path":[0],"operator":"equals","value":1}]},
			"length":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"array","path":["length"],"operator":"equals","value":1}]},
			"numericObject":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"object","path":[0],"operator":"equals","value":1}]}
		}
	}`)
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile})
	for _, key := range []string{"stringIndex", "numericIndex", "length"} {
		if value := instance.GetGlobalVariable(key); value != true {
			t.Fatalf("%s did not match: %#v", key, value)
		}
	}
	if value := instance.GetGlobalVariable("numericObject"); value != false {
		t.Fatalf("numeric path read a string object property: %#v", value)
	}
}

func TestChildrenOwnStickyRequirementState(t *testing.T) {
	sticky := StickyFeatures{"gate": {Enabled: true}}
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"child-sticky","segments":{},
		"features":{"gate":{"bucketBy":"userId","traffic":[]}},
		"variables":{"subject":{"type":"string","defaultValue":"matched","disabledValue":"disabled","requiredFeatures":["gate"]},"direct":{"type":"string","defaultValue":"default"}}
	}`)
	stickyVariables := StickyVariables{"direct": "parent"}
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, StickyFeatures: &sticky, StickyVariables: &stickyVariables})
	if value := instance.GetGlobalVariable("subject"); value != "matched" {
		t.Fatalf("parent sticky requirement was ignored: %#v", value)
	}
	if value := instance.Spawn().GetGlobalVariable("subject"); value != "disabled" {
		t.Fatalf("child inherited parent sticky features: %#v", value)
	}
	if value := instance.Spawn().GetGlobalVariable("direct"); value != "default" {
		t.Fatalf("child inherited parent sticky variables: %#v", value)
	}
	if value := instance.Spawn(SpawnOptions{StickyFeatures: &sticky}).GetGlobalVariable("subject"); value != "matched" {
		t.Fatalf("child sticky requirement was ignored: %#v", value)
	}
}

func TestNullConditionsDistinguishMissingAndNumericEqualityExcludesBooleans(t *testing.T) {
	presentNull := PlainCondition{Attribute: "value", Operator: OperatorIsNull}
	missingNull := PlainCondition{Attribute: "missing", Operator: OperatorIsNull}
	presentNotNull := PlainCondition{Attribute: "value", Operator: OperatorNotNull}
	missingNotNull := PlainCondition{Attribute: "missing", Operator: OperatorNotNull}
	context := Context{"value": nil, "number": 1.0, "boolean": true}
	if !AllConditionsAreMatched(presentNull, context) || AllConditionsAreMatched(missingNull, context) || AllConditionsAreMatched(presentNotNull, context) || AllConditionsAreMatched(missingNotNull, context) || !AllConditionsAreMatched(PlainCondition{Attribute: "number", Operator: OperatorNotNull}, context) {
		t.Fatal("isNull/notNull did not distinguish missing from null")
	}
	one := ConditionValue(1)
	if !AllConditionsAreMatched(PlainCondition{Attribute: "number", Operator: OperatorEquals, Value: &one}, context) {
		t.Fatal("1 and 1.0 did not compare equally")
	}
	if AllConditionsAreMatched(PlainCondition{Attribute: "boolean", Operator: OperatorEquals, Value: &one}, context) {
		t.Fatal("true compared equal to 1")
	}
}

func TestDependencyIndexingSelectsProgramAndPropagatesMixedGraphs(t *testing.T) {
	oldDatafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"old","segments":{"legacy":{"conditions":{"attribute":"country","operator":"equals","value":"US"}}},
		"features":{"base":{"hash":"base-old","bucketBy":"userId","traffic":[]},"leaf":{"hash":"leaf","bucketBy":"userId","conditions":[{"variable":"bridge","operator":"equals","value":true}],"traffic":[]}},
		"variables":{"subject":{"hash":"subject","type":"string","defaultValue":"default","overrides":[{"segments":"legacy","value":"legacy"}],"overrideProgram":[]},"bridge":{"hash":"bridge","type":"boolean","defaultValue":true,"requiredFeatures":["base"]}}
	}`)
	newDatafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"new","segments":{"legacy":{"conditions":{"attribute":"country","operator":"equals","value":"NL"}}},
		"features":{"base":{"hash":"base-new","bucketBy":"userId","traffic":[{"key":"all","segments":"*","percentage":100000}]},"leaf":{"hash":"leaf","bucketBy":"userId","conditions":[{"variable":"bridge","operator":"equals","value":true}],"traffic":[]}},
		"variables":{"subject":{"hash":"subject","type":"string","defaultValue":"default","overrides":[{"segments":"legacy","value":"legacy"}],"overrideProgram":[]},"bridge":{"hash":"bridge","type":"boolean","defaultValue":true,"requiredFeatures":["base"]}}
	}`)
	diagnostics := newDiagnosticReporter(diagnosticReporterOptions{})
	oldProvider := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{Datafile: oldDatafile, diagnosticReporter: diagnostics})
	newProvider := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{Datafile: newDatafile, diagnosticReporter: diagnostics})
	features, variables := affectedDatafileEntities(oldProvider, newProvider)
	featureSet, variableSet := map[string]bool{}, map[string]bool{}
	for _, key := range features {
		featureSet[key] = true
	}
	for _, key := range variables {
		variableSet[key] = true
	}
	if !featureSet["base"] || !featureSet["leaf"] || !variableSet["bridge"] {
		t.Fatalf("mixed dependency graph did not propagate: features=%v variables=%v", features, variables)
	}
	if variableSet["subject"] {
		t.Fatalf("inactive legacy override produced an update dependency: %v", variables)
	}
}

func TestRequiredFeatureErrorsBehaveAsDisabledExceptForCycleSentinels(t *testing.T) {
	datafile := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"required-feature-errors","segments":{},
		"features":{
			"source":{"bucketBy":"userId","traffic":[{"key":"all","segments":"*","percentage":100000}]},
			"expectedDisabled":{"bucketBy":"userId","requiredFeatures":[{"feature":"source","enabled":false}],"traffic":[{"key":"all","segments":"*","percentage":100000}]},
			"notSource":{"bucketBy":"userId","requiredFeatures":[{"not":["source"]}],"traffic":[{"key":"all","segments":"*","percentage":100000}]},
			"mixed":{"bucketBy":"userId","conditions":[{"variable":"a","operator":"equals","value":true}],"variablesSchema":{"value":{"type":"boolean","defaultValue":true}},"traffic":[{"key":"all","segments":"*","percentage":100000}]}
		},
		"variables":{"a":{"type":"boolean","defaultValue":true,"conditions":[{"feature":"mixed","variable":"value","operator":"equals","value":true}]}}
	}`)
	module := &FeaturevisorModule{Name: "source-error", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		if options.FeatureKey == "source" {
			panic("source module failed")
		}
		return options
	}}
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Modules: []*FeaturevisorModule{module}})
	if !instance.IsEnabled("expectedDisabled") || !instance.IsEnabled("notSource") {
		t.Fatal("ordinary required feature errors did not behave as enabled=false")
	}
	if evaluation := instance.EvaluateGlobalVariable("a"); evaluation.Reason != EvaluationReasonError || !errors.Is(evaluation.Error, errVariableRequirementCycle) {
		t.Fatalf("mixed variable cycle sentinel did not propagate: %#v", evaluation)
	}
}

func TestHashlessAndEmptyHashEntitiesAlwaysNotifyAndPropagate(t *testing.T) {
	initial := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"initial","segments":{},
		"features":{
			"source":{"bucketBy":"userId","traffic":[]},
			"dependent":{"hash":"dependent","bucketBy":"userId","requiredFeatures":["source"],"traffic":[]}
		},
		"variables":{
			"sourceVariable":{"hash":"","type":"boolean","defaultValue":true},
			"dependentVariable":{"hash":"dependent-variable","type":"boolean","defaultValue":true,"conditions":[{"variable":"sourceVariable","operator":"equals","value":true}]}
		}
	}`)
	for _, testCase := range []struct {
		name     string
		replace  bool
		incoming DatafileContent
	}{
		{name: "replacement", replace: true, incoming: initial},
		{name: "partial merge", replace: false, incoming: alignmentDatafile(t, `{
			"schemaVersion":"2","revision":"partial","segments":{},
			"features":{"source":{"bucketBy":"userId","traffic":[]}},
			"variables":{"sourceVariable":{"hash":"","type":"boolean","defaultValue":true}}
		}`)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: initial})
			var details EventDetails
			unsubscribe := instance.On(EventNameDatafileSet, func(value EventDetails) { details = value })
			defer unsubscribe()
			instance.SetDatafile(testCase.incoming, testCase.replace)
			features := details["features"].([]string)
			variables := details["variables"].([]string)
			for _, key := range []string{"source", "dependent"} {
				if !sliceContains(features, key) {
					t.Fatalf("missing changed feature %q from %v", key, features)
				}
			}
			for _, key := range []string{"sourceVariable", "dependentVariable"} {
				if !sliceContains(variables, key) {
					t.Fatalf("missing changed variable %q from %v", key, variables)
				}
			}
		})
	}
}

func TestRemovedDependencyEdgesStillPropagateAcrossOldAndNewGraphs(t *testing.T) {
	previous := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"old","segments":{},"variables":{},"features":{
			"a":{"hash":"a-old","bucketBy":"userId","traffic":[]},
			"b":{"hash":"b-same","bucketBy":"userId","requiredFeatures":["a"],"traffic":[]},
			"c":{"hash":"c-same","bucketBy":"userId","requiredFeatures":["b"],"traffic":[]}
		}
	}`)
	next := alignmentDatafile(t, `{
		"schemaVersion":"2","revision":"new","segments":{},"variables":{},"features":{
			"a":{"hash":"a-new","bucketBy":"userId","traffic":[]},
			"b":{"hash":"b-same","bucketBy":"userId","traffic":[]},
			"c":{"hash":"c-same","bucketBy":"userId","requiredFeatures":["b"],"traffic":[]}
		}
	}`)
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: previous})
	var details EventDetails
	instance.On(EventNameDatafileSet, func(value EventDetails) { details = value })
	instance.SetDatafile(next, true)
	features := details["features"].([]string)
	for _, key := range []string{"a", "b", "c"} {
		if !sliceContains(features, key) {
			t.Fatalf("removed dependency edge failed to propagate %q through old/new graphs: %v", key, features)
		}
	}
}

func sliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
