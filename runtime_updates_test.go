package featurevisor

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
)

const moduleMembershipDatafile = `{"schemaVersion":"4","revision":"modules","segments":{},
 "features":{
 "broken":{"hash":"broken","bucketBy":"userId","conditions":"[broken","traffic":[]},
 "gate":{"hash":"gate","bucketBy":"userId","traffic":[{"key":"all","conditions":"*","percentage":100000}]},
 "subject":{"hash":"subject","bucketBy":"userId","conditions":{"or":[{"feature":"broken"},{"feature":"gate"}]},"traffic":[{"key":"all","conditions":"*","percentage":100000}]}},
 "variables":{"subject":{"hash":"subject","type":"string","defaultValue":"matched","conditions":{"or":[{"feature":"broken"},{"feature":"gate"}]}}}}`

func TestModuleMembershipRefreshesBetweenPhases(t *testing.T) {
	for _, global := range []bool{false, true} {
		for _, disableGate := range []bool{false, true} {
			for _, removal := range []bool{false, true} {
				t.Run(fmt.Sprintf("global=%t/removal=%t/disableGate=%t", global, removal, disableGate), func(t *testing.T) {
					trace := []string{}
					label := func(o EvaluateOptions) string {
						if o.GlobalVariable {
							return *o.VariableKey
						}
						return o.FeatureKey
					}
					late := &FeaturevisorModule{Name: "late",
						BeforeEvaluation: func(o EvaluateOptions) EvaluateOptions {
							trace = append(trace, "before:"+label(o))
							return o
						},
						AfterEvaluation: func(e Evaluation, o EvaluateOptions) Evaluation {
							trace = append(trace, "after:"+label(o))
							if disableGate && o.FeatureKey == "gate" {
								disabled := false
								e.Enabled = &disabled
							}
							return e
						},
					}
					level := LogLevelError
					var f *Featurevisor
					f = CreateFeaturevisor(FeaturevisorOptions{Datafile: moduleMembershipDatafile, LogLevel: &level,
						OnDiagnostic: func(d FeaturevisorDiagnostic) {
							if d.Code == "conditions_parse_error" {
								if removal {
									f.RemoveModule("late")
								} else {
									f.AddModule(late)
								}
							}
						},
					})
					defer f.Close()
					if removal {
						f.AddModule(late)
					}
					var result Evaluation
					if global {
						result = f.EvaluateGlobalVariable("subject")
					} else {
						result = f.EvaluateFlag("subject")
					}
					if disableGate && !removal {
						if result.Reason != "conditions_unmet" {
							t.Fatalf("gate result ignored: %+v", result)
						}
						if global && result.VariableValue != nil {
							t.Fatal("blocked variable returned a value")
						}
						if !global && result.Enabled != nil && *result.Enabled {
							t.Fatal("blocked flag enabled")
						}
					} else if global {
						if result.VariableValue != "matched" {
							t.Fatal(result)
						}
					} else if result.Enabled == nil || !*result.Enabled {
						t.Fatal("subject disabled")
					}
					want := []string{"after:broken", "before:gate", "after:gate", "after:subject"}
					if removal {
						want = []string{"before:subject", "before:broken"}
					}
					if !reflect.DeepEqual(trace, want) {
						t.Fatalf("trace %v, want %v", trace, want)
					}
				})
			}
		}
	}
}

func TestBeforePhaseKeepsItsStartingMembership(t *testing.T) {
	level := LogLevelError
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: moduleMembershipDatafile, LogLevel: &level})
	defer f.Close()
	trace := []string{}
	f.AddModule(&FeaturevisorModule{Name: "first", BeforeEvaluation: func(o EvaluateOptions) EvaluateOptions {
		trace = append(trace, "before:first")
		f.RemoveModule("second")
		return o
	}, AfterEvaluation: func(e Evaluation, o EvaluateOptions) Evaluation { trace = append(trace, "after:first"); return e }})
	f.AddModule(&FeaturevisorModule{Name: "second", BeforeEvaluation: func(o EvaluateOptions) EvaluateOptions {
		trace = append(trace, "before:second")
		return o
	}, AfterEvaluation: func(e Evaluation, o EvaluateOptions) Evaluation { trace = append(trace, "after:second"); return e }})
	if !f.IsEnabled("gate") {
		t.Fatal("gate disabled")
	}
	if !reflect.DeepEqual(trace, []string{"before:first", "before:second", "after:first"}) {
		t.Fatal(trace)
	}
}

func TestIncrementalIndexExtractsOnlyIncomingOwners(t *testing.T) {
	for _, size := range []int{1, 64, 512} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			data := DatafileContent{SchemaVersion: "4", Revision: "initial", Features: map[string]Feature{}, Variables: map[string]GlobalVariable{}, Segments: map[string]Segment{}}
			for n := 0; n < size; n++ {
				key := fmt.Sprint(n)
				data.Features[key] = Feature{Hash: parityStringPtr(key), BucketBy: "userId", Conditions: `{"variable":"source","operator":"notNull"}`}
				data.Segments[key] = Segment{Conditions: `{"feature":"` + key + `"}`}
			}
			data.Variables["source"] = GlobalVariable{Hash: parityStringPtr("source"), Type: "string", DefaultValue: "initial"}
			level := LogLevelError
			f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level})
			defer f.Close()
			if f.dependencyIndex != nil {
				t.Fatal("construction indexed selectors")
			}
			for n := 0; n < 5; n++ {
				patch := DatafileContent{SchemaVersion: "4", Revision: fmt.Sprint(n), Features: map[string]Feature{}, Segments: map[string]Segment{}, Variables: map[string]GlobalVariable{
					"source": {Hash: parityStringPtr(fmt.Sprint(n)), Type: "string", DefaultValue: "next"},
				}}
				if !f.SetDatafile(patch) {
					t.Fatal("rejected update")
				}
				if got, want := f.dependencyIndex.extractions, uint64(2*size+1+n+1); got != want {
					t.Fatalf("extractions %d, want %d", got, want)
				}
				if len(f.dataProvider().entities) != 0 {
					t.Fatal("index warmed evaluation cache")
				}
			}
		})
	}
}

func TestEventTraversalDoesNotMergeGraphGenerations(t *testing.T) {
	// The old graph reaches middle through oldOnly. The new graph alone must
	// not walk from that old segment into impossible.
	before := alignmentDatafile(t, `{"schemaVersion":"4","revision":"before","segments":{
	"oldOnly":{"conditions":{"variable":"seed","operator":"notNull"}},
	"middle":{"conditions":"oldOnly"}},
	"features":{"impossible":{"hash":"same","bucketBy":"userId","conditions":"*","traffic":[]}},
	"variables":{"seed":{"hash":"old","type":"string","defaultValue":"old"}}}`)
	after := alignmentDatafile(t, `{"schemaVersion":"4","revision":"after","segments":{
	"oldOnly":{"conditions":"*"},"middle":{"conditions":"*"}},
	"features":{"impossible":{"hash":"same","bucketBy":"userId","conditions":"middle","traffic":[]}},
	"variables":{"seed":{"hash":"new","type":"string","defaultValue":"new"}}}`)
	// Exercise the graph boundary directly so only seed is an initial change.
	index := newDatafileDependencyIndex()
	index.addDatafile(before)
	features, variables, segments := map[string]bool{}, map[string]bool{"seed": true}, map[string]bool{}
	index.propagate(features, variables, segments)
	for owner := range index.owners {
		index.remove(owner)
	}
	index.addDatafile(after)
	index.propagate(features, variables, segments)
	if features["impossible"] {
		t.Fatal("old segment reachability leaked into the new graph")
	}
}

func TestConcurrentWritersRetainColdSlotsAndGraph(t *testing.T) {
	level := LogLevelError
	var errors atomic.Int64
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: moduleMembershipDatafile, LogLevel: &level, OnDiagnostic: func(d FeaturevisorDiagnostic) {
		if d.Code == "conditions_parse_error" {
			errors.Add(1)
		}
	}})
	defer f.Close()
	old := f.dataProvider()
	var wg sync.WaitGroup
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprint(n)
			if !f.SetDatafile(DatafileContent{SchemaVersion: "4", Revision: key, Features: map[string]Feature{}, Segments: map[string]Segment{}, Variables: map[string]GlobalVariable{
				key: {Hash: parityStringPtr(key), Type: "string", DefaultValue: key},
			}}) {
				t.Error("rejected update")
			}
			old.getFeatureForEvaluation("broken")
			f.IsEnabled("broken")
		}(n)
	}
	wg.Wait()
	if errors.Load() != 1 {
		t.Fatalf("retained parse errors: %d", errors.Load())
	}
	keys := f.dataProvider().GetGlobalVariableKeys()
	sort.Strings(keys)
	if len(keys) != 21 {
		t.Fatalf("lost writer definitions: %v", keys)
	}
	if len(f.dependencyIndex.owners) != 24 {
		t.Fatalf("lost graph owners: %d", len(f.dependencyIndex.owners))
	}
	if !f.SetDatafile(moduleMembershipDatafile, true) {
		t.Fatal("replace rejected")
	}
	f.IsEnabled("broken")
	if errors.Load() != 2 {
		t.Fatal("replacement retained an obsolete cold slot")
	}
}
