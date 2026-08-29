package featurevisor

import (
	"reflect"
	"testing"
)

func alignmentDatafile(t *testing.T, raw string) DatafileContent {
	t.Helper()
	var datafile DatafileContent
	if err := datafile.FromJSON(raw); err != nil {
		t.Fatal(err)
	}
	return datafile
}

func TestModulePipelineUsesCanonicalPhaseOrder(t *testing.T) {
	order := []string{}
	module := func(name string) *FeaturevisorModule {
		return &FeaturevisorModule{
			Name:   name,
			Before: func(options EvaluateOptions) EvaluateOptions { order = append(order, "before:"+name); return options },
			BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
				order = append(order, "beforeEvaluation:"+name)
				return options
			},
			AfterEvaluation: func(evaluation Evaluation, options EvaluateOptions) Evaluation {
				order = append(order, "afterEvaluation:"+name)
				return evaluation
			},
			After: func(evaluation Evaluation, options EvaluateOptions) Evaluation {
				order = append(order, "after:"+name)
				return evaluation
			},
		}
	}
	f := CreateFeaturevisor(FeaturevisorOptions{Modules: []*FeaturevisorModule{module("first"), module("second")}})
	f.EvaluateFlag("missing", Context{}, OverrideOptions{})
	expected := []string{"before:first", "before:second", "beforeEvaluation:first", "beforeEvaluation:second", "afterEvaluation:first", "afterEvaluation:second", "after:first", "after:second"}
	if !reflect.DeepEqual(order, expected) {
		t.Fatalf("unexpected module order: %#v", order)
	}
}

func TestModulesApplyToRequiredFeaturesAndTransformedDefaults(t *testing.T) {
	datafile := alignmentDatafile(t, `{"schemaVersion":"2","revision":"modules","segments":{"allowed":{"conditions":{"attribute":"allow","operator":"equals","value":true}}},"features":{"required":{"bucketBy":"userId","traffic":[{"key":"all","segments":"allowed","percentage":100000}]},"dependent":{"bucketBy":"userId","requiredFeatures":["required"],"traffic":[{"key":"all","segments":"*","percentage":100000}]}},"variables":{}}`)
	module := &FeaturevisorModule{Name: "required-context", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		if options.FeatureKey == "required" {
			options.Context["allow"] = true
		}
		if options.Type == EvaluationTypeVariable {
			options.DefaultVariableValue = "module-default"
			options.DefaultVariableValueSet = true
		}
		return options
	}}
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Modules: []*FeaturevisorModule{module}})
	if !f.IsEnabled("dependent", Context{"userId": "u"}, OverrideOptions{}) {
		t.Fatal("required feature did not use the module pipeline")
	}
	if value := f.GetGlobalVariable("missing"); value != "module-default" {
		t.Fatalf("module default was ignored: %#v", value)
	}
}

func TestExplicitNullBeatsCallerDefaultsAndChildNormalizesGlobalJSON(t *testing.T) {
	datafile := alignmentDatafile(t, `{"schemaVersion":"2","revision":"nulls","segments":{},"features":{"feature":{"bucketBy":"userId","variablesSchema":{"nullable":{"type":"json","defaultValue":null}},"traffic":[{"key":"all","segments":"*","percentage":100000}]}},"variables":{"nullable":{"type":"json","defaultValue":null},"settings":{"type":"json","defaultValue":"{\"enabled\":true}"}}}`)
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile})
	options := OverrideOptions{DefaultVariableValue: "caller", DefaultVariableValueSet: true}
	featureEvaluation := f.EvaluateVariable("feature", "nullable", Context{"userId": "u"}, options)
	if !featureEvaluation.variableValueSet || featureEvaluation.VariableValue != nil {
		t.Fatalf("feature null was not preserved: %#v", featureEvaluation)
	}
	globalEvaluation := f.EvaluateGlobalVariable("nullable", options)
	if !globalEvaluation.variableValueSet || globalEvaluation.VariableValue != nil {
		t.Fatalf("global null was not preserved: %#v", globalEvaluation)
	}
	child := f.Spawn()
	if value := child.GetGlobalVariable("settings"); !reflect.DeepEqual(value, map[string]interface{}{"enabled": true}) {
		t.Fatalf("child global JSON was not normalized: %#v", value)
	}
	var decoded struct {
		Enabled bool `json:"enabled"`
	}
	if err := child.GetGlobalVariableObjectInto("settings", &decoded); err != nil || !decoded.Enabled {
		t.Fatalf("child object decoder failed: %#v, %v", decoded, err)
	}
}
