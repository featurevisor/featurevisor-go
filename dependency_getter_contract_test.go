package featurevisor

import "testing"

func TestInternalFlagGateKeepsTheOuterRequester(t *testing.T) {
	for _, kind := range []EvaluationType{EvaluationTypeVariable, EvaluationTypeVariation} {
		t.Run(string(kind), func(t *testing.T) {
			data := alignmentV4Data()
			variation := "on"
			data.Features["subject"] = Feature{BucketBy: []string{}, Conditions: map[string]interface{}{"feature": "healthy"},
				Variations: []Variation{{Value: variation}}, VariablesSchema: map[string]VariableSchema{"value": {Type: VariableTypeString, DefaultValue: "hello"}},
				Traffic: []Traffic{{Key: "all", Conditions: "*", Percentage: 100000, Variation: &variation}}}
			var seen []*EvaluationDependency
			level := LogLevelFatal
			f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level, Modules: []*FeaturevisorModule{{
				Name: "requester", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
					if options.FeatureKey == "healthy" && options.DependencyOf != nil {
						seen = append(seen, options.DependencyOf)
					}
					return options
				},
			}}})
			defer f.Close()
			if kind == EvaluationTypeVariable {
				f.GetVariable("subject", "value")
			} else {
				f.GetVariation("subject")
			}
			if len(seen) != 1 || seen[0].Type != kind || seen[0].FeatureKey != "subject" {
				t.Fatalf("outer requester was replaced: %#v", seen)
			}
			if kind == EvaluationTypeVariable && (seen[0].VariableKey == nil || *seen[0].VariableKey != "value") {
				t.Fatalf("variable requester key was lost: %#v", seen[0])
			}
			if kind == EvaluationTypeVariation && seen[0].VariableKey != nil {
				t.Fatal("variation acquired a variable key")
			}
		})
	}
}

func TestConditionMemoRetainsNormalizedJSONFailurePerRoot(t *testing.T) {
	data := alignmentV4Data()
	data.Variables["source"] = GlobalVariable{Type: VariableTypeJSON, DefaultValue: "{oops"}
	leaf := map[string]interface{}{"variable": "source", "operator": "isNull"}
	subject := data.Features["healthy"]
	subject.Conditions = []interface{}{leaf, leaf}
	data.Features["subject"] = subject
	failures := 0
	level := LogLevelError
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level, OnDiagnostic: func(d FeaturevisorDiagnostic) {
		if d.Message == "getVariable failed" {
			failures++
		}
	}})
	defer f.Close()
	for run := 1; run <= 2; run++ {
		if !f.IsEnabled("subject") {
			t.Fatal("ordinary null getter fallback was not preserved")
		}
		if failures != run {
			t.Fatalf("run %d: JSON was normalized %d times", run, failures)
		}
	}
}
