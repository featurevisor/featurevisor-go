package featurevisor

import (
	"fmt"
	"reflect"
	"testing"
)

func TestModulePhaseSnapshotMatrix(t *testing.T) {
	phases := []string{"before", "key", "value", "after"}
	for _, global := range []bool{false, true} {
		for changedPhase, phase := range phases {
			for _, action := range []string{"add", "remove_add", "remove", "unsubscribe"} {
				t.Run(fmt.Sprintf("global=%t/%s/%s", global, phase, action), func(t *testing.T) {
					level := LogLevelError
					f := CreateFeaturevisor(FeaturevisorOptions{LogLevel: &level, Datafile: `{
"schemaVersion":"4","revision":"phases","segments":{},"features":{
"gate":{"bucketBy":"userId","traffic":[{"key":"all","conditions":"*","percentage":100000}]}},
"variables":{"subject":{"type":"string","defaultValue":"matched","conditions":{"feature":"gate"}}}}`})
					defer f.Close()
					trace, closed := []string{}, []string{}
					changed := false
					var unsubscribe FeaturevisorUnsubscribe
					var module func(string) *FeaturevisorModule
					record := func(name string, index int, active bool) {
						if !active {
							return
						}
						trace = append(trace, phases[index]+":"+name)
						if name == "first" && index == changedPhase && !changed {
							changed = true
							if action == "unsubscribe" {
								unsubscribe()
							} else if action != "add" {
								f.RemoveModule("other")
							}
							if action == "add" || action == "remove_add" {
								f.AddModule(module("late"))
							}
						}
					}
					active := func(o EvaluateOptions) bool {
						if global {
							return o.GlobalVariable && o.VariableKey != nil && *o.VariableKey == "subject"
						}
						return o.FeatureKey == "gate"
					}
					module = func(name string) *FeaturevisorModule {
						return &FeaturevisorModule{Name: name,
							BeforeEvaluation: func(o EvaluateOptions) EvaluateOptions { record(name, 0, active(o)); return o },
							BucketKey:        func(o ConfigureBucketKeyOptions) BucketKey { record(name, 1, true); return o.BucketKey },
							BucketValue:      func(o ConfigureBucketValueOptions) BucketValue { record(name, 2, true); return o.BucketValue },
							AfterEvaluation:  func(e Evaluation, o EvaluateOptions) Evaluation { record(name, 3, active(o)); return e },
							Close:            func() { closed = append(closed, name) },
						}
					}
					f.AddModule(module("first"))
					unsubscribe = f.AddModule(module("other"))
					evaluate := func() {
						if global {
							if got := f.EvaluateGlobalVariable("subject").VariableValue; got != "matched" {
								t.Fatalf("value: %v", got)
							}
						} else if !f.IsEnabled("gate") {
							t.Fatal("gate disabled")
						}
					}
					evaluate()
					after := []string{"first"}
					wantClosed := []string{}
					if action == "add" {
						after = append(after, "other")
					} else {
						wantClosed = append(wantClosed, "other")
					}
					if action == "add" || action == "remove_add" {
						after = append(after, "late")
					}
					want := []string{}
					for index, phase := range phases {
						members := after
						if index <= changedPhase {
							members = []string{"first", "other"}
						}
						for _, name := range members {
							want = append(want, phase+":"+name)
						}
					}
					if !reflect.DeepEqual(trace, want) || !reflect.DeepEqual(closed, wantClosed) {
						t.Fatalf("trace=%v want=%v closed=%v want=%v", trace, want, closed, wantClosed)
					}
					trace = nil
					evaluate()
					want = nil
					for _, phase := range phases {
						for _, name := range after {
							want = append(want, phase+":"+name)
						}
					}
					if !reflect.DeepEqual(trace, want) {
						t.Fatalf("next evaluation: %v want %v", trace, want)
					}
				})
			}
		}
	}
}
