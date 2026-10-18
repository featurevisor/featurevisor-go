package featurevisor

import (
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"
)

func stickyHandoffFixture(size int, kind string) (*StickyFeatures, *StickyVariables) {
	features := StickyFeatures{}
	variables := StickyVariables{}
	for index := 0; index < size; index++ {
		key := fmt.Sprintf("unrelated%d", index)
		if kind == "features" {
			features[FeatureKey(key)] = EvaluatedFeature{Enabled: true, Variables: map[VariableKey]VariableValue{"unused": func() {}}}
		} else {
			variables[GlobalVariableKey(key)] = func() {}
		}
	}
	return &features, &variables
}

func TestEvaluationSessionOwnsReplacementStickyPointersUntilRootBoundary(t *testing.T) {
	first := StickyFeatures{"unrelated": {Enabled: true}}
	replacement := StickyFeatures{"unrelated": {Enabled: true}}
	session := &evaluationSession{}
	options := EvaluateOptions{
		evaluateParams: evaluateParams{Type: EvaluationTypeFlag, FeatureKey: "gate"},
		evaluateDependencies: evaluateDependencies{
			Context: Context{}, DependencyOf: &EvaluationDependency{Type: EvaluationTypeFlag, FeatureKey: "root"},
			modulesManager: &modulesManager{}, stickyFeatures: &first,
		},
	}
	firstKey, firstOK := session.key(options)
	options.stickyFeatures = &replacement
	replacementKey, replacementOK := session.key(options)
	if !firstOK || !replacementOK || firstKey == replacementKey {
		t.Fatalf("replacement did not get a distinct key: %t %t", firstOK, replacementOK)
	}
	if len(session.stickyFeatures) != 2 {
		t.Fatalf("session does not own both keyed maps: %#v", session.stickyFeatures)
	}
	if _, owned := session.stickyFeatures[&first]; !owned {
		t.Fatal("first sticky map was released")
	}
	if _, owned := session.stickyFeatures[&replacement]; !owned {
		t.Fatal("replacement sticky map was released")
	}

	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: stickyHandoffDatafile, StickyFeatures: &first, LogLevel: &level})
	rootSession := &evaluationSession{}
	rootKey := VariableKey("featureRoot")
	root := EvaluateOptions{
		evaluateParams:       evaluateParams{Type: EvaluationTypeVariable, VariableKey: &rootKey, GlobalVariable: true},
		evaluateDependencies: instance.getEvaluationDependencies(Context{"userId": "a"}, OverrideOptions{}),
	}
	root.session = rootSession
	if result := evaluateWithModules(root); result.VariableValue != true {
		t.Fatalf("root = %#v", result)
	}
	if rootSession.stickyFeatures != nil || rootSession.stickyVariables != nil {
		t.Fatal("root request retained sticky pointers")
	}
}

func TestEvaluationSessionStickyHandoffUsesIdentityAndReusesDependencies(t *testing.T) {
	for _, kind := range []string{"features", "variables"} {
		for _, childMode := range []bool{false, true} {
			for _, size := range []int{0, 100, 1000, 5000} {
				t.Run(fmt.Sprintf("%s/child=%t/size=%d", kind, childMode, size), func(t *testing.T) {
					features, variables := stickyHandoffFixture(size, kind)
					reads := 0
					module := &FeaturevisorModule{Name: "observe", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
						if (kind == "features" && options.FeatureKey == "gate") ||
							(kind == "variables" && options.VariableKey != nil && *options.VariableKey == "leaf") {
							reads++
						}
						return options
					}}
					level := LogLevelFatal
					config := FeaturevisorOptions{Datafile: stickyHandoffDatafile, Modules: []*FeaturevisorModule{module}, LogLevel: &level}
					if !childMode {
						config.StickyFeatures, config.StickyVariables = features, variables
					}
					instance := CreateFeaturevisor(config)
					var target interface {
						GetGlobalVariable(string, ...interface{}) VariableValue
					}
					target = instance
					if childMode {
						child := instance.Spawn(Context{}, SpawnOptions{StickyFeatures: features, StickyVariables: variables})
						defer child.Close()
						target = child
					}
					root := "variableRoot"
					if kind == "features" {
						root = "featureRoot"
					}
					for call := 1; call <= 2; call++ {
						if got := target.GetGlobalVariable(root, Context{"userId": "a"}); got != true {
							t.Fatalf("call %d = %#v", call, got)
						}
						if reads != call {
							t.Fatalf("call %d prerequisite reads = %d, want %d", call, reads, call)
						}
					}
				})
			}
		}
	}
}

const sessionGraphDatafile = `{
  "schemaVersion":"2","revision":"session","segments":{},"features":{},
  "variables":{
    "leaf":{"type":"boolean","defaultValue":true},
    "left":{"type":"boolean","defaultValue":true,"conditions":[{"variable":"leaf","operator":"equals","value":true}]},
    "right":{"type":"boolean","defaultValue":true,"conditions":[{"variable":"leaf","operator":"equals","value":true}]},
    "root":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[{"variable":"left","operator":"equals","value":true},{"variable":"right","operator":"equals","value":true}]}
  }
}`

func TestEvaluationSessionMemoizesDiamondWithinOneRequest(t *testing.T) {
	var visits []string
	var leafParent *EvaluationDependency
	module := &FeaturevisorModule{Name: "observe", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		if options.GlobalVariable && options.VariableKey != nil {
			visits = append(visits, string(*options.VariableKey))
			if *options.VariableKey == "leaf" {
				leafParent = options.DependencyOf
			}
		}
		return options
	}}
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: sessionGraphDatafile, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if value := instance.GetGlobalVariable("root"); value != true {
		t.Fatalf("root = %#v", value)
	}
	if !reflect.DeepEqual(visits, []string{"root", "left", "leaf", "right", "leaf"}) {
		t.Fatalf("unexpected dependency visits: %#v", visits)
	}
	if leafParent == nil || leafParent.Type != EvaluationTypeVariable || leafParent.VariableKey == nil || *leafParent.VariableKey != "right" {
		t.Fatalf("leaf did not identify its immediate parent: %#v", leafParent)
	}
	if value := instance.GetGlobalVariable("root"); value != true {
		t.Fatalf("second root = %#v", value)
	}
	if len(visits) != 10 {
		t.Fatalf("memo leaked across public calls: %#v", visits)
	}
}

func TestEvaluationSessionSeparatesTransformedContextsAndReentrantCalls(t *testing.T) {
	var instance *Featurevisor
	var countries []string
	reentered := false
	module := &FeaturevisorModule{Name: "transform", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		if options.VariableKey == nil {
			return options
		}
		switch *options.VariableKey {
		case "left", "right":
			context := Context{}
			for key, value := range options.Context {
				context[key] = value
			}
			context["country"] = string(*options.VariableKey)
			options.Context = context
		case "leaf":
			if country, ok := options.Context["country"].(string); ok {
				countries = append(countries, country)
			}
			if !reentered {
				reentered = true
				if value := instance.GetGlobalVariable("leaf"); value != true {
					t.Fatalf("reentrant getter = %#v", value)
				}
			}
		}
		return options
	}}
	level := LogLevelFatal
	instance = CreateFeaturevisor(FeaturevisorOptions{Datafile: sessionGraphDatafile, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if value := instance.GetGlobalVariable("root"); value != true {
		t.Fatalf("root = %#v", value)
	}
	if !reentered || !reflect.DeepEqual(countries, []string{"left", "right"}) {
		t.Fatalf("contexts or reentrant getter were conflated: %#v, reentered=%t", countries, reentered)
	}
}

func TestRequirementPathFailureDoesNotBecomeNullMatch(t *testing.T) {
	datafile := `{"schemaVersion":"2","revision":"invalid-requirement","segments":{},"features":{},"variables":{
		"leaf":{"type":"boolean","defaultValue":true},
		"source":{"type":"boolean","defaultValue":true,"conditions":[{"variable":"leaf","path":"uncompiled","operator":"isNull"}]},
		"subject":{"type":"boolean","defaultValue":true,"conditions":[{"variable":"source","operator":"isNull"}]}
	}}`
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, LogLevel: &level})
	if value := instance.GetGlobalVariable("source"); value != nil {
		t.Fatalf("ordinary failed getter = %#v", value)
	}
	if result := instance.EvaluateGlobalVariable("subject"); result.Reason != EvaluationReasonError {
		t.Fatalf("requirement failure became a null match: %#v", result)
	}
}

func TestEvaluationSessionInvalidatesMemoAfterStickySetter(t *testing.T) {
	datafile := `{"schemaVersion":"2","revision":"setter","segments":{},"features":{},"variables":{
		"leaf":{"type":"integer","defaultValue":1},
		"mutator":{"type":"boolean","defaultValue":true},
		"root":{"type":"integer","defaultValue":1,"disabledValue":0,"conditions":[
			{"variable":"leaf","operator":"equals","value":1},
			{"variable":"mutator","operator":"equals","value":true},
			{"variable":"leaf","operator":"equals","value":2}
		]}}}`
	var instance *Featurevisor
	reads := 0
	module := &FeaturevisorModule{Name: "mutate", AfterEvaluation: func(evaluation Evaluation, options EvaluateOptions) Evaluation {
		if evaluation.VariableKey == nil {
			return evaluation
		}
		switch *evaluation.VariableKey {
		case "leaf":
			reads++
		case "mutator":
			instance.SetStickyVariables(StickyVariables{"leaf": float64(2)})
		}
		return evaluation
	}}
	level := LogLevelFatal
	instance = CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if value := instance.GetGlobalVariable("root"); value != 1 {
		t.Fatalf("root = %#v", value)
	}
	if reads != 2 {
		t.Fatalf("leaf reads = %d", reads)
	}
}

func TestEvaluationSessionBeforeHookSeesLiveSticky(t *testing.T) {
	datafile := `{"schemaVersion":"2","revision":"setter","segments":{},"features":{},"variables":{"root":{"type":"integer","defaultValue":1}}}`
	var instance *Featurevisor
	module := &FeaturevisorModule{Name: "set-sticky", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		instance.SetStickyVariables(StickyVariables{"root": float64(2)})
		return options
	}}
	level := LogLevelFatal
	instance = CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if value := instance.GetGlobalVariable("root"); value != float64(2) {
		t.Fatalf("root = %#v", value)
	}
}

func TestEvaluationSessionChildStickyRestorationDoesNotResurrectMemo(t *testing.T) {
	datafile := `{"schemaVersion":"2","revision":"child","segments":{},"features":{},"variables":{
		"leaf":{"type":"integer","defaultValue":1},
		"restore":{"type":"integer","defaultValue":1},
		"root":{"type":"integer","defaultValue":1,"disabledValue":0,"conditions":[
			{"variable":"leaf","operator":"equals","value":2},
			{"variable":"restore","operator":"equals","value":1},
			{"variable":"leaf","operator":"equals","value":1}
		]}}}`
	var child *FeaturevisorChild
	reads := 0
	module := &FeaturevisorModule{Name: "child-sticky", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		if options.VariableKey == nil {
			return options
		}
		switch *options.VariableKey {
		case "leaf":
			reads++
			if reads == 1 {
				child.SetStickyVariables(StickyVariables{"leaf": 2})
			}
		case "restore":
			child.SetStickyVariables(StickyVariables{"leaf": 1}, true)
		}
		return options
	}}
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	child = instance.Spawn(Context{}, SpawnOptions{StickyVariables: &StickyVariables{"leaf": 1}})
	if value := child.GetGlobalVariable("root"); value != 1 {
		t.Fatalf("root = %#v", value)
	}
	if reads != 2 {
		t.Fatalf("leaf reads = %d", reads)
	}
}

func TestEvaluationSessionMemoUsesImmediateRequesterForHooks(t *testing.T) {
	datafile := `{"schemaVersion":"2","revision":"parent-aware","segments":{},"features":{},"variables":{
		"leaf":{"type":"integer","defaultValue":0},
		"left":{"type":"integer","defaultValue":1,"conditions":[
			{"variable":"leaf","operator":"equals","value":1},{"variable":"leaf","operator":"equals","value":1}]},
		"right":{"type":"integer","defaultValue":1,"conditions":[
			{"variable":"leaf","operator":"equals","value":2},{"variable":"leaf","operator":"equals","value":2}]},
		"root":{"type":"integer","defaultValue":1,"conditions":[
			{"variable":"left","operator":"equals","value":1},{"variable":"right","operator":"equals","value":1},
			{"variable":"left","operator":"equals","value":1},{"variable":"right","operator":"equals","value":1}]}
	}}`
	parents := []string{}
	module := &FeaturevisorModule{Name: "parent-aware", AfterEvaluation: func(evaluation Evaluation, options EvaluateOptions) Evaluation {
		if evaluation.VariableKey != nil && *evaluation.VariableKey == "leaf" && options.DependencyOf != nil && options.DependencyOf.VariableKey != nil {
			parent := string(*options.DependencyOf.VariableKey)
			parents = append(parents, parent)
			if parent == "left" {
				evaluation.VariableValue = 1
			} else {
				evaluation.VariableValue = 2
			}
		}
		return evaluation
	}}
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if value := instance.GetGlobalVariable("root"); value != 1 {
		t.Fatalf("root = %#v", value)
	}
	if !reflect.DeepEqual(parents, []string{"left", "right"}) {
		t.Fatalf("leaf parents = %#v", parents)
	}
}

func TestEvaluationSessionMemoOwnsNestedVariableResults(t *testing.T) {
	datafile := `{"schemaVersion":"2","revision":"snapshot","segments":{},"features":{},"variables":{
		"leaf":{"type":"object","defaultValue":{}},
		"mutator":{"type":"integer","defaultValue":1},
		"root":{"type":"integer","defaultValue":1,"disabledValue":0,"conditions":[
			{"variable":"leaf","path":["nested","value"],"operator":"equals","value":1},
			{"variable":"mutator","operator":"equals","value":1},
			{"variable":"leaf","path":["nested","value"],"operator":"equals","value":1}
		]}}}`
	result := map[string]interface{}{"nested": map[string]interface{}{"value": 1}}
	reads := 0
	module := &FeaturevisorModule{Name: "result", AfterEvaluation: func(evaluation Evaluation, options EvaluateOptions) Evaluation {
		if evaluation.VariableKey == nil {
			return evaluation
		}
		switch *evaluation.VariableKey {
		case "leaf":
			reads++
			evaluation.VariableValue = result
		case "mutator":
			result["nested"].(map[string]interface{})["value"] = 2
		}
		return evaluation
	}}
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if value := instance.GetGlobalVariable("root"); value != 1 {
		t.Fatalf("root = %#v", value)
	}
	if reads != 1 {
		t.Fatalf("leaf reads = %d", reads)
	}
}

func TestEvaluationSessionConcurrentReadsAndStateUpdates(t *testing.T) {
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: sessionGraphDatafile, LogLevel: &level})
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		for index := 0; index < 100; index++ {
			instance.GetGlobalVariable("root", Context{"index": index})
		}
	}()
	go func() {
		defer group.Done()
		for index := 0; index < 100; index++ {
			instance.SetContext(Context{"index": index})
			instance.SetStickyVariables(StickyVariables{"leaf": index%2 == 0})
			unsubscribe := instance.AddModule(&FeaturevisorModule{Name: "temporary"})
			if unsubscribe != nil {
				unsubscribe()
			}
		}
	}()
	group.Wait()
}

func TestEvaluationSessionGuardsOriginalAndTransformedFeatureCycles(t *testing.T) {
	level := LogLevelFatal
	self := `{"schemaVersion":"2","revision":"self-cycle","segments":{},"features":{"a":{"requiredFeatures":[{"not":["a"]}]}}}`
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: self, LogLevel: &level})
	if result := instance.EvaluateFlag("a"); result.Reason != EvaluationReasonError {
		t.Fatalf("self cycle = %#v", result)
	}
	redirected := `{"schemaVersion":"2","revision":"redirect-cycle","segments":{},"features":{"a":{"requiredFeatures":["b"]},"b":{}}}`
	module := &FeaturevisorModule{Name: "redirect", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		if options.FeatureKey == "b" {
			options.FeatureKey = "a"
		}
		return options
	}}
	instance = CreateFeaturevisor(FeaturevisorOptions{Datafile: redirected, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if result := instance.EvaluateFlag("a"); result.Reason != EvaluationReasonError {
		t.Fatalf("transformed cycle = %#v", result)
	}
}

func TestEvaluationSessionTransformedAncestorOnlyCyclesOnNextPrerequisite(t *testing.T) {
	module := &FeaturevisorModule{Name: "redirect-with-force", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		if options.FeatureKey == "b" {
			options.FeatureKey = "a"
			context := copyContext(options.Context)
			context["phase"] = "forced"
			options.Context = context
		}
		return options
	}}
	level := LogLevelFatal
	withoutRecursion := `{"schemaVersion":"2","revision":"alias-forced","segments":{},"features":{
		"a":{"bucketBy":"userId","requiredFeatures":["b"],"force":[{"conditions":{"attribute":"phase","operator":"equals","value":"forced"},"enabled":true}],"traffic":[{"key":"all","segments":"*","percentage":100000,"enabled":true}]},
		"b":{"bucketBy":"userId","traffic":[{"key":"all","segments":"*","percentage":100000,"enabled":true}]}}}`
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: withoutRecursion, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if result := instance.EvaluateFlag("a"); result.Reason == EvaluationReasonError || result.Enabled == nil || !*result.Enabled {
		t.Fatalf("transformed ancestor without recursion = %#v", result)
	}

	ancestorVisits := 0
	module.BeforeEvaluation = func(options EvaluateOptions) EvaluateOptions {
		if options.FeatureKey == "a" {
			ancestorVisits++
		}
		if options.FeatureKey == "b" {
			options.FeatureKey = "a"
			context := copyContext(options.Context)
			context["phase"] = "forced"
			options.Context = context
		}
		return options
	}
	withNextPrerequisite := `{"schemaVersion":"2","revision":"alias-next","segments":{},"features":{
		"a":{"bucketBy":"userId","requiredFeatures":["b","c"],"force":[{"conditions":{"attribute":"phase","operator":"equals","value":"forced"},"enabled":true}],"traffic":[{"key":"all","segments":"*","percentage":100000,"enabled":true}]},
		"b":{"bucketBy":"userId","traffic":[{"key":"all","segments":"*","percentage":100000,"enabled":true}]},
		"c":{"bucketBy":"userId","requiredFeatures":["a"],"traffic":[{"key":"all","segments":"*","percentage":100000,"enabled":true}]}}}`
	instance = CreateFeaturevisor(FeaturevisorOptions{Datafile: withNextPrerequisite, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if result := instance.EvaluateFlag("a"); result.Reason != EvaluationReasonError {
		t.Fatalf("next prerequisite did not detect active ancestor: %#v", result)
	}
	if ancestorVisits != 1 {
		t.Fatalf("ancestor reentered after alias frame finished: %d visits", ancestorVisits)
	}
}

func TestEvaluationSessionDoesNotCacheMutatedInput(t *testing.T) {
	datafile := `{"schemaVersion":"2","revision":"mutable-input","segments":{},"features":{},"variables":{
		"leaf":{"type":"integer","defaultValue":1},"mutator":{"type":"integer","defaultValue":1},
		"root":{"type":"integer","defaultValue":1,"disabledValue":0,"conditions":[
			{"variable":"leaf","operator":"equals","value":1},
			{"variable":"mutator","operator":"equals","value":1},
			{"variable":"leaf","operator":"equals","value":2}
		]}}}`
	reads := 0
	module := &FeaturevisorModule{Name: "input", AfterEvaluation: func(evaluation Evaluation, options EvaluateOptions) Evaluation {
		if evaluation.VariableKey == nil {
			return evaluation
		}
		switch *evaluation.VariableKey {
		case "leaf":
			reads++
			evaluation.VariableValue = reads
			if reads == 1 {
				options.Context["nested"].(map[string]interface{})["value"] = 2
			}
		case "mutator":
			options.Context["nested"].(map[string]interface{})["value"] = 1
		}
		return evaluation
	}}
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Context: Context{"nested": map[string]interface{}{"value": 1}}, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if value := instance.GetGlobalVariable("root"); value != 1 {
		t.Fatalf("root = %#v", value)
	}
	if reads != 2 {
		t.Fatalf("leaf reads = %d", reads)
	}
}

func TestEvaluationSessionContextSnapshotRejectsCyclesAndTracksTypedMutation(t *testing.T) {
	cycle := Context{}
	cycle["self"] = cycle
	if _, ok := snapshotContext(cycle); ok {
		t.Fatal("cyclic context became cacheable")
	}
	context := Context{"nested": map[string][]int{"values": {1, 2}}}
	snapshot, ok := snapshotContext(context)
	if !ok || !sameContext(snapshot, context) {
		t.Fatal("typed context did not snapshot")
	}
	context["nested"].(map[string][]int)["values"][1] = 3
	if sameContext(snapshot, context) {
		t.Fatal("nested typed mutation retained memo eligibility")
	}
	wide := Context{"items": make([]int, 10001)}
	if _, ok := snapshotContext(wide); ok {
		t.Fatal("oversized context bypassed the traversal bound")
	}
	negativeZero := Context{"number": math.Copysign(0, -1)}
	zeroSnapshot, ok := snapshotContext(negativeZero)
	if !ok || sameContext(zeroSnapshot, Context{"number": float64(0)}) {
		t.Fatal("signed zero was conflated")
	}
	if sameContext(Context{"number": int32(1)}, Context{"number": int64(1)}) {
		t.Fatal("distinct host numeric types were conflated")
	}
}

func TestEvaluationSessionRequesterGlobalIdentityIsDistinct(t *testing.T) {
	key := VariableKey("requester")
	options := EvaluateOptions{
		evaluateParams: evaluateParams{Type: EvaluationTypeVariable, VariableKey: &key, GlobalVariable: true},
		evaluateDependencies: evaluateDependencies{DependencyOf: &EvaluationDependency{Type: EvaluationTypeVariable,
			VariableKey: &key}, requesterGlobalVariable: true},
	}
	session := &evaluationSession{}
	global, ok := session.key(options)
	if !ok {
		t.Fatal("global requester was not keyable")
	}
	options.requesterGlobalVariable = false
	feature, ok := session.key(options)
	if !ok || global == feature {
		t.Fatal("global and feature requesters share a memo identity")
	}
}

func TestEvaluationSessionAllocationBoundForVariablePrerequisites(t *testing.T) {
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: sessionGraphDatafile, LogLevel: &level})
	allocations := testing.AllocsPerRun(100, func() {
		if result := instance.GetGlobalVariable("root", Context{"userId": "fixed"}); result != true {
			panic("unexpected gated result")
		}
	})
	t.Logf("variable prerequisite read: %.0f allocations per call", allocations)
	if allocations > 200 {
		t.Fatalf("gated read allocated %.0f times, want at most 200", allocations)
	}
}

func BenchmarkEvaluationSessionVariablePrerequisites(b *testing.B) {
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: sessionGraphDatafile, LogLevel: &level})
	context := Context{"userId": "fixed"}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if instance.GetGlobalVariable("root", context) != true {
			b.Fatal("unexpected gated result")
		}
	}
}

func TestRequiredVariableTreesCompileOnLoadAndRefreshOnReplaceOrMerge(t *testing.T) {
	first := `{"schemaVersion":"2","revision":"first","segments":{},"features":{},"variables":{
		"leaf":{"type":"integer","defaultValue":1},
		"root":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[
			{"and":[{"variable":"leaf","operator":"equals","value":1}]}
		]}}}`
	second := `{"schemaVersion":"2","revision":"second","segments":{},"features":{},"variables":{
		"leaf":{"type":"integer","defaultValue":2},
		"root":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[
			{"and":[{"variable":"leaf","operator":"equals","value":2}]}
		]}}}`
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: first, LogLevel: &level})
	check := func(want bool) {
		t.Helper()
		if got := instance.GetGlobalVariable("root"); got != want {
			t.Fatalf("root = %#v, want %t", got, want)
		}
		root := instance.instanceEvaluationDataProvider.GetGlobalVariable("root")
		group := root.Conditions.([]interface{})[0].(map[string]interface{})
		leaf := group["and"].([]interface{})[0]
		if _, ok := requiredVariableParts(leaf); !ok {
			t.Fatalf("requirement was not compiled: %T", leaf)
		}
	}
	check(true)
	instance.SetDatafile(second, true)
	check(true)
	merge := `{"schemaVersion":"2","revision":"merge","segments":{},"features":{},"variables":{
		"root":{"type":"boolean","defaultValue":true,"disabledValue":false,"conditions":[
			{"and":[{"variable":"leaf","operator":"equals","value":1}]}
		]}}}`
	instance.SetDatafile(merge)
	check(false)
}

func TestRequiredVariableCompilerCoversEveryDatafileSiteWithoutMutatingInput(t *testing.T) {
	leaf := map[string]interface{}{"variable": "leaf", "operator": "equals", "value": float64(1)}
	tree := []RequiredVariable{map[string]interface{}{"and": []interface{}{leaf}}}
	datafile := DatafileContent{
		SchemaVersion: "2", Revision: "compiled", Segments: map[SegmentKey]Segment{},
		Features: map[FeatureKey]Feature{"gate": {Conditions: tree,
			Traffic:    []Traffic{{VariableOverrides: map[VariableKey][]VariableOverride{"message": {{Conditions: tree}}}}},
			Variations: []Variation{{VariableOverrides: map[VariableKey][]VariableOverride{"message": {{Conditions: tree}}}}},
		}},
		Variables: map[GlobalVariableKey]GlobalVariable{
			"root": {Conditions: tree,
				Overrides:       []VariableOverride{{Conditions: tree}},
				OverrideProgram: []VariableOverrideStep{{Conditions: tree}}},
		},
	}
	provider := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{Datafile: datafile})
	feature := provider.GetFeature("gate")
	variable := provider.GetGlobalVariable("root")
	for _, values := range []interface{}{
		feature.Conditions, variable.Conditions,
		feature.Traffic[0].VariableOverrides["message"][0].Conditions,
		feature.Variations[0].VariableOverrides["message"][0].Conditions,
		variable.Overrides[0].Conditions, variable.OverrideProgram[0].Conditions,
	} {
		group := values.([]interface{})[0].(map[string]interface{})
		parsed, ok := requiredVariableParts(group["and"].([]interface{})[0])
		if !ok || !parsed.valueSet || parsed.Variable != "leaf" {
			t.Fatalf("site was not compiled: %#v", values)
		}
	}
	if _, ok := tree[0].(map[string]interface{})["and"].([]interface{})[0].(map[string]interface{}); !ok {
		t.Fatal("compiler changed the caller's datafile")
	}
}

func TestCompiledRequirementRetainsNumericPathAndMalformedShortCircuit(t *testing.T) {
	datafile := `{"schemaVersion":"2","revision":"paths","segments":{},"features":{},"variables":{
		"leaf":{"type":"array","defaultValue":[{"x":7}]},
		"good":{"type":"boolean","defaultValue":true,"conditions":[{"or":[
			{"variable":"leaf","path":[0,"x"],"operator":"equals","value":7},
			{"variable":"leaf","path":"uncompiled","operator":"isNull"}]}]},
		"bad":{"type":"boolean","defaultValue":true,"conditions":[{"or":[
			{"variable":"leaf","path":"uncompiled","operator":"isNull"},
			{"variable":"leaf","path":[0,"x"],"operator":"equals","value":7}]}]}
	}}`
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, LogLevel: &level})
	if result := instance.EvaluateGlobalVariable("good"); result.Reason == EvaluationReasonError || result.VariableValue != true {
		t.Fatalf("numeric path or short circuit changed: %#v", result)
	}
	if result := instance.EvaluateGlobalVariable("bad"); result.Reason != EvaluationReasonError {
		t.Fatalf("reached malformed path was ignored: %#v", result)
	}
	root := instance.instanceEvaluationDataProvider.GetGlobalVariable("good")
	group := root.Conditions.([]interface{})[0].(map[string]interface{})
	leaf, _ := requiredVariableParts(group["or"].([]interface{})[0])
	path, ok := leaf.Path.([]interface{})
	if !ok || len(path) != 2 || path[0] != float64(0) {
		t.Fatalf("numeric JSON path changed during compilation: %#v", leaf.Path)
	}
}

func TestEvaluationSessionFingerprintsValidStickyFeatures(t *testing.T) {
	datafile := `{"schemaVersion":"2","revision":"sticky","segments":{},"features":{},"variables":{
		"leaf":{"type":"integer","defaultValue":1},
		"root":{"type":"integer","defaultValue":1,"conditions":[
			{"variable":"leaf","operator":"equals","value":1},{"variable":"leaf","operator":"equals","value":1}]}
	}}`
	reads := 0
	module := &FeaturevisorModule{Name: "observe", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		if options.VariableKey != nil && *options.VariableKey == "leaf" {
			reads++
		}
		return options
	}}
	sticky := StickyFeatures{"unrelated": EvaluatedFeature{Enabled: true}}
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, StickyFeatures: &sticky, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if value := instance.GetGlobalVariable("root"); value != 1 {
		t.Fatalf("root = %#v", value)
	}
	if reads != 1 {
		t.Fatalf("leaf reads = %d", reads)
	}
}

func TestEvaluationSessionDoesNotExposeRequestInternalsToRetainedModuleOptions(t *testing.T) {
	var retained []EvaluateOptions
	module := &FeaturevisorModule{Name: "retain", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		retained = append(retained, options)
		return options
	}}
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: sessionGraphDatafile, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if value := instance.GetGlobalVariable("root"); value != true {
		t.Fatalf("root = %#v", value)
	}
	for _, options := range retained {
		if options.session != nil || options.stateVersion != nil || options.refreshState != nil || options.modulesManager != nil || options.instanceEvaluationDataProvider != nil {
			t.Fatalf("retained module options still own request internals: %#v", options)
		}
	}
}

func TestEvaluationSessionSnapshotsFlagPointerOperands(t *testing.T) {
	datafile := `{"schemaVersion":"2","revision":"pointer","segments":{},"features":{"leaf":{},"mutator":{}},"variables":{
		"root":{"type":"integer","defaultValue":1,"disabledValue":0,"requiredFeatures":["leaf","mutator","leaf"]}
	}}`
	shared := true
	leafReads := 0
	module := &FeaturevisorModule{Name: "mutable-pointer", AfterEvaluation: func(evaluation Evaluation, options EvaluateOptions) Evaluation {
		if evaluation.Type != EvaluationTypeFlag {
			return evaluation
		}
		switch evaluation.FeatureKey {
		case "leaf":
			leafReads++
			evaluation.Enabled = &shared
		case "mutator":
			shared = false
			enabled := true
			evaluation.Enabled = &enabled
		}
		return evaluation
	}}
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if value := instance.GetGlobalVariable("root"); value != 1 {
		t.Fatalf("root = %#v", value)
	}
	if leafReads != 1 {
		t.Fatalf("leaf reads = %d", leafReads)
	}
}

func TestEvaluationSessionSkipsUnsupportedContextValues(t *testing.T) {
	datafile := `{"schemaVersion":"2","revision":"unsupported-context","segments":{},"features":{},"variables":{
		"leaf":{"type":"integer","defaultValue":1},
		"root":{"type":"integer","defaultValue":1,"conditions":[
			{"variable":"leaf","operator":"equals","value":1},{"variable":"leaf","operator":"equals","value":1}]}
	}}`
	reads := 0
	module := &FeaturevisorModule{Name: "observe", BeforeEvaluation: func(options EvaluateOptions) EvaluateOptions {
		if options.VariableKey != nil && *options.VariableKey == "leaf" {
			reads++
		}
		return options
	}}
	level := LogLevelFatal
	instance := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, Context: Context{"host": func() {}}, Modules: []*FeaturevisorModule{module}, LogLevel: &level})
	if value := instance.GetGlobalVariable("root"); value != 1 {
		t.Fatalf("root = %#v", value)
	}
	if reads != 2 {
		t.Fatalf("leaf reads = %d", reads)
	}
}
