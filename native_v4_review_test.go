package featurevisor

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestMalformedNotOperandsAgreeAcrossSelectorPaths(t *testing.T) {
	for _, operand := range []interface{}{nil, false, 42, "*", "missing", map[string]interface{}{}, map[string]interface{}{"attribute": "missing", "operator": "exists"}, PlainCondition{Attribute: "missing", Operator: OperatorExists}} {
		for _, schema := range []string{"2", "4"} {
			for _, typed := range []bool{false, true} {
				for _, stringified := range []bool{false, true} {
					selector := interface{}(map[string]interface{}{"not": operand})
					if typed {
						selector = NotCondition{Not: operand}
					}
					name, _ := json.Marshal(selector)
					t.Run(schema+"/"+string(name)+"/"+map[bool]string{false: "map", true: "typed"}[typed]+"/"+map[bool]string{false: "object", true: "string"}[stringified], func(t *testing.T) {
						if AllConditionsAreMatched(selector, Context{}) {
							t.Error("attribute helper matched a malformed NOT operand")
						}
						if stringified {
							selector = string(name)
						}
						data := alignmentV4Data()
						data.SchemaVersion = schema
						data.Segments["malformed"] = Segment{Conditions: selector}
						for key, condition := range map[string]interface{}{"direct": selector, "segment": "malformed", "alias": "alias"} {
							feature := data.Features["healthy"]
							feature.Conditions = condition
							data.Features[key] = feature
						}
						data.Segments["alias"] = Segment{Conditions: "malformed"}
						level := LogLevelFatal
						f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level})
						defer f.Close()
						for _, key := range []string{"direct", "segment", "alias"} {
							if f.IsEnabled(key) {
								t.Errorf("%s: malformed NOT getter enabled the feature", key)
							}
						}
					})
				}
			}
		}
	}
}

func TestJSONNullRootPresenceAcrossSchemasAndCompositions(t *testing.T) {
	for _, schema := range []string{"2", "4"} {
		t.Run(schema, func(t *testing.T) {
			raw := fmt.Sprintf(`{
				"schemaVersion":%q,"revision":"null-presence","segments":{},
				"features":{
					"null":{"conditions":null,"bucketBy":[],"traffic":[{"key":"all","conditions":"*","percentage":100000}]},
					"absent":{"bucketBy":[],"traffic":[{"key":"all","conditions":"*","percentage":100000}]}
				},
				"variables":{
					"source":{"type":"string","defaultValue":"source"},
					"null":{"type":"string","conditions":null,"defaultValue":"wrong","disabledValue":"blocked"},
					"absent":{"type":"string","defaultValue":"ordinary"},
					"nullAlias":{"type":"string","conditions":null,"compose":"source","overrides":[{"conditions":"*","value":"wrong"}]},
					"nullMap":{"type":"object","conditions":null,"compose":[{"key":"value","variable":"source"}],"disabledValue":{"blocked":true},
						"overrideProgram":[{"key":"wrong","conditions":"*","operations":[{"op":"set","path":[],"value":{"wrong":true}}],"onMatch":1,"onMiss":1}]},
					"absentAlias":{"type":"string","compose":"source"},
					"absentMap":{"type":"object","compose":[{"key":"value","variable":"source"}]}
				}
			}`, schema)
			level := LogLevelError
			reads, parseErrors := 0, 0
			f := CreateFeaturevisor(FeaturevisorOptions{Datafile: raw, LogLevel: &level,
				OnDiagnostic: func(d FeaturevisorDiagnostic) {
					if d.Code == "conditions_parse_error" {
						parseErrors++
					}
				},
				Modules: []*FeaturevisorModule{{BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
					if options.DependencyPurpose == "composition" && stringValue(options.VariableKey) == "source" {
						reads++
					}
					return options
				}}},
			})
			defer f.Close()
			assertWirePresence := func(evaluation Evaluation, present bool) {
				t.Helper()
				encoded, err := json.Marshal(evaluation)
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(encoded, &fields); err != nil {
					t.Fatal(err)
				}
				conditions, exists := fields["conditions"]
				if exists != present || (present && string(conditions) != "null") {
					t.Fatalf("conditions presence=%v: %s", present, encoded)
				}
			}
			for _, evaluation := range []Evaluation{f.EvaluateFlag("null"), f.EvaluateGlobalVariable("null"), f.EvaluateGlobalVariable("nullAlias"), f.EvaluateGlobalVariable("nullMap")} {
				if evaluation.Reason != EvaluationReasonConditionsUnmet {
					t.Fatalf("null gate was absent: %#v", evaluation)
				}
				assertWirePresence(evaluation, true)
			}
			defaults := OverrideOptions{DefaultVariableValue: "caller", DefaultVariableValueSet: true}
			if f.IsEnabled("null") || f.GetGlobalVariable("null", defaults) != "blocked" || f.GetGlobalVariable("nullAlias", defaults) != nil ||
				!sameJSONValue(f.GetGlobalVariable("nullMap", defaults), map[string]interface{}{"blocked": true}) {
				t.Fatal("null root getter fallbacks differed from the gate contract")
			}
			if reads != 0 {
				t.Fatalf("null composition gates read %d sources", reads)
			}
			if !f.IsEnabled("absent") || f.GetGlobalVariable("absent") != "ordinary" || f.GetGlobalVariable("absentAlias") != "source" ||
				!sameJSONValue(f.GetGlobalVariable("absentMap"), map[string]interface{}{"value": "source"}) {
				t.Fatal("absent root conditions introduced a gate")
			}
			for _, evaluation := range []Evaluation{f.EvaluateFlag("absent"), f.EvaluateGlobalVariable("absent"), f.EvaluateGlobalVariable("absentAlias"), f.EvaluateGlobalVariable("absentMap")} {
				assertWirePresence(evaluation, false)
			}
			if parseErrors != 0 {
				t.Fatalf("explicit null was diagnosed as malformed JSON: %d", parseErrors)
			}
		})
	}
}

func TestCompositionMemoInvalidatesOnModuleDrivenSourceChanges(t *testing.T) {
	for _, change := range []string{"datafile", "sticky", "context", "remove", "unsubscribe"} {
		t.Run(change, func(t *testing.T) {
			data := alignmentV4Data()
			data.Variables["source"] = GlobalVariable{Type: VariableTypeString, DefaultValue: "one"}
			if change == "remove" || change == "unsubscribe" {
				data.Variables["source"] = GlobalVariable{Type: VariableTypeString, DefaultValue: "two"}
			}
			data.Variables["map"] = GlobalVariable{Type: VariableTypeObject, Compose: []ComposeEntry{
				{Key: "a", Variable: "source"}, {Key: "pivot", Variable: "missing"},
				{Key: "b", Variable: "source"}, {Key: "c", Variable: "source"},
			}}
			var f *Featurevisor
			var unsubscribe FeaturevisorUnsubscribe
			reads := 0
			changed := false
			level := LogLevelFatal
			f = CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level, Modules: []*FeaturevisorModule{{
				Name: "change-source",
				BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
					if options.GlobalVariable && stringValue(options.VariableKey) == "map" {
						options.Context = Context{"country": "one"}
					}
					return options
				},
				AfterEvaluation: func(evaluation Evaluation, options EvaluateOptions) Evaluation {
					if stringValue(options.VariableKey) == "source" {
						reads++
						if change == "context" {
							evaluation.VariableValue = options.Context["country"]
						}
					}
					if stringValue(options.VariableKey) == "missing" && !changed {
						changed = true
						if f.GetGlobalVariable("healthy") != "healthy" {
							t.Error("callback reentry shared or lost request state")
						}
						switch change {
						case "remove":
							f.RemoveModule("temporary")
						case "unsubscribe":
							unsubscribe()
						case "datafile":
							next := alignmentV4Data()
							next.Variables["source"] = GlobalVariable{Type: VariableTypeString, DefaultValue: "two"}
							f.SetDatafile(next)
						case "sticky":
							f.SetStickyVariables(StickyVariables{"source": "two"})
						case "context":
							f.AddModule(&FeaturevisorModule{Name: "context", BeforeEvaluation: func(next EvaluateOptions) EvaluateOptions {
								if stringValue(next.VariableKey) == "source" {
									next.Context = Context{"country": "two"}
								}
								return next
							}})
						}
					}
					return evaluation
				},
			}}})
			defer f.Close()
			if change == "remove" || change == "unsubscribe" {
				unsubscribe = f.AddModule(&FeaturevisorModule{Name: "temporary", AfterEvaluation: func(evaluation Evaluation, options EvaluateOptions) Evaluation {
					if stringValue(options.VariableKey) == "source" {
						evaluation.VariableValue = "one"
					}
					return evaluation
				}})
			}
			got := f.GetGlobalVariable("map", OverrideOptions{DefaultVariableValue: "outer-default", DefaultVariableValueSet: true})
			want := map[string]interface{}{"a": "one", "pivot": nil, "b": "two", "c": "two"}
			if !sameJSONValue(got, want) || reads != 2 {
				t.Fatalf("source state or ordinary null fallback was memoized incorrectly: %#v, reads=%d", got, reads)
			}
		})
	}
}

func TestRootFeatureHooksKeepJSStickySnapshotUntilNextCall(t *testing.T) {
	for _, childMode := range []bool{false, true} {
		for _, kind := range []EvaluationType{EvaluationTypeFlag, EvaluationTypeVariation, EvaluationTypeVariable} {
			t.Run(fmt.Sprintf("child=%v/%s", childMode, kind), func(t *testing.T) {
				data := alignmentV4Data()
				data.Features["subject"] = Feature{BucketBy: []string{}, Traffic: []Traffic{}, VariablesSchema: map[string]VariableSchema{"value": {Type: VariableTypeInteger, DefaultValue: 1}}}
				var f *Featurevisor
				var child *FeaturevisorChild
				level := LogLevelFatal
				f = CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level, Modules: []*FeaturevisorModule{{BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
					if options.FeatureKey == "subject" {
						on := "on"
						sticky := StickyFeatures{"subject": {Enabled: true, Variation: &on, Variables: map[string]VariableValue{"value": 7}}}
						if childMode {
							child.SetStickyFeatures(sticky)
						} else {
							f.SetStickyFeatures(sticky)
						}
					}
					return options
				}}}})
				defer f.Close()
				if childMode {
					child = f.Spawn(Context{})
					defer child.Close()
				}
				switch kind {
				case EvaluationTypeFlag:
					got := false
					if childMode {
						got = child.IsEnabled("subject")
					} else {
						got = f.IsEnabled("subject")
					}
					if got {
						t.Fatal("root hook changed the current JS sticky snapshot")
					}
					if childMode {
						got = child.IsEnabled("subject")
					} else {
						got = f.IsEnabled("subject")
					}
					if !got {
						t.Fatal("next call did not observe sticky flag")
					}
				case EvaluationTypeVariation:
					var got *string
					if childMode {
						got = child.GetVariation("subject")
					} else {
						got = f.GetVariation("subject")
					}
					if got != nil {
						t.Fatalf("root hook changed the current JS sticky snapshot: %v", got)
					}
					if childMode {
						got = child.GetVariation("subject")
					} else {
						got = f.GetVariation("subject")
					}
					if got == nil || *got != "on" {
						t.Fatalf("next call did not observe sticky variation: %v", got)
					}
				case EvaluationTypeVariable:
					var got interface{}
					if childMode {
						got = child.GetVariable("subject", "value")
					} else {
						got = f.GetVariable("subject", "value")
					}
					if got != nil {
						t.Fatalf("root hook changed the current JS sticky snapshot: %#v", got)
					}
					if childMode {
						got = child.GetVariable("subject", "value")
					} else {
						got = f.GetVariable("subject", "value")
					}
					if got != 7 {
						t.Fatalf("next call did not observe sticky variable: %#v", got)
					}
				}
			})
		}
	}
}

func TestSegmentAfterGetterCallbackReadsCurrentDatafile(t *testing.T) {
	data := alignmentV4Data()
	data.Segments["gate"] = Segment{Conditions: []interface{}{}}
	feature := data.Features["healthy"]
	feature.Conditions = []interface{}{map[string]interface{}{"variable": "pivot", "operator": "equals", "value": true}, "gate"}
	data.Features["subject"] = feature
	data.Variables["pivot"] = GlobalVariable{Type: VariableTypeBoolean, DefaultValue: true}
	var f *Featurevisor
	level := LogLevelFatal
	f = CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level, Modules: []*FeaturevisorModule{{BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		if stringValue(options.VariableKey) == "pivot" {
			next := alignmentV4Data()
			next.Segments["gate"] = Segment{Conditions: "*"}
			f.SetDatafile(next)
		}
		return options
	}}}})
	defer f.Close()
	if !f.IsEnabled("subject") {
		t.Fatal("segment after getter callback used a superseded datafile")
	}
}

func TestTerminalAttributeFastPathMatchesJSActiveHandoffBound(t *testing.T) {
	for _, getters := range []int{0, 1, 128, 255, 256} {
		for _, offset := range []int{-1, 0, 1, 2} {
			segments := 256 - getters + offset
			if segments < 1 {
				continue
			}
			for _, terminal := range []string{"wildcard", "attribute", "attribute group"} {
				t.Run(fmt.Sprintf("getters=%d/segments=%d/%s", getters, segments, terminal), func(t *testing.T) {
					data := alignmentV4Data()
					feature := data.Features["healthy"]
					feature.Conditions = "s0"
					data.Features["subject"] = feature
					for i := 0; i <= getters; i++ {
						variable := GlobalVariable{Type: VariableTypeBoolean, DefaultValue: true, DisabledValue: false}
						if i < getters {
							variable.Compose = fmt.Sprintf("g%d", i+1)
						} else {
							variable.Conditions = "s0"
						}
						data.Variables[fmt.Sprintf("g%d", i)] = variable
					}
					for i := 0; i < segments; i++ {
						var conditions interface{} = fmt.Sprintf("s%d", i+1)
						if i == segments-1 {
							conditions = "*"
							if terminal != "wildcard" {
								conditions = map[string]interface{}{"attribute": "country", "operator": "equals", "value": "nl"}
								if terminal == "attribute group" {
									conditions = map[string]interface{}{"and": []interface{}{conditions}}
								}
							}
						}
						data.Segments[fmt.Sprintf("s%d", i)] = Segment{Conditions: conditions}
					}
					level := LogLevelFatal
					f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level})
					defer f.Close()
					active := getters + segments
					if terminal != "wildcard" {
						active--
					}
					for run := 0; run < 2; run++ {
						for _, country := range []string{"nl", "de"} {
							if getters == 0 {
								flag := f.EvaluateFlag("subject", Context{"country": country})
								wantReason := EvaluationReasonConditionsUnmet
								if active > 256 {
									wantReason = EvaluationReasonError
								} else if terminal == "wildcard" || country == "nl" {
									wantReason = EvaluationReasonRule
								}
								if flag.Reason != wantReason {
									t.Fatalf("flag boundary: %#v, want %s", flag, wantReason)
								}
							}
							e := f.EvaluateGlobalVariable("g0", Context{"country": country})
							if active > 256 {
								if e.Reason != EvaluationReasonError {
									t.Fatalf("overflow matched: %#v", e)
								}
								if f.GetGlobalVariable("g0", Context{"country": country}) != nil {
									t.Fatal("overflow getter did not fail closed")
								}
							} else {
								want := terminal == "wildcard" || country == "nl"
								if e.Reason == EvaluationReasonError || e.VariableValue != want {
									t.Fatalf("valid active handoff boundary: %#v, want %v", e, want)
								}
							}
						}
						if !f.IsEnabled("healthy") {
							t.Fatal("dependency session leaked into an independent root")
						}
					}
				})
			}
		}
	}
}
