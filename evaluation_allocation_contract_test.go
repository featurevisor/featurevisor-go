package featurevisor

import "testing"

func TestWarmFeatureReadBorrowsCachedDefinition(t *testing.T) {
	d := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile: DatafileContent{SchemaVersion: "4", Features: map[string]Feature{
			"flag": {BucketBy: "userId"},
		}},
		diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{}),
	})
	warm := d.getFeatureForEvaluation("flag")
	allocations := testing.AllocsPerRun(100, func() {
		if d.getFeatureForEvaluation("flag") != warm {
			t.Fatal("warm definition was copied")
		}
	})
	if allocations != 0 {
		t.Fatalf("warm feature definition allocated %v times", allocations)
	}
	public := d.GetFeature("flag")
	public.BucketBy = "other"
	if d.getFeatureForEvaluation("flag").BucketBy != "userId" {
		t.Fatal("public struct mutation reached the cached definition")
	}
}

func TestAttributeConditionAllocationsDoNotGrowWithTreeWidth(t *testing.T) {
	d := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           DatafileContent{SchemaVersion: "4"},
		diagnosticReporter: newDiagnosticReporter(diagnosticReporterOptions{}),
	})
	deps := evaluateDependencies{Context: Context{"country": "nl"}, instanceEvaluationDataProvider: d}
	deps.refreshState = func(current *evaluateDependencies) { current.instanceEvaluationDataProvider = d }
	leaf := map[string]interface{}{"attribute": "country", "operator": "exists"}
	measure := func(width int) float64 {
		children := make([]interface{}, width)
		for i := range children {
			children[i] = leaf
		}
		tree := map[string]interface{}{"and": children}
		unified := testing.AllocsPerRun(100, func() {
			if !conditionsAreMatched(tree, deps) {
				t.Fatal("matching predicate failed")
			}
		})
		plain := testing.AllocsPerRun(100, func() {
			if !d.AllConditionsAreMatched(tree, deps.Context) {
				t.Fatal("attribute matcher failed")
			}
		})
		return unified - plain
	}
	narrow, wide := measure(1), measure(64)
	if wide > narrow+1 {
		t.Fatalf("dependency struct escaped per predicate: narrow %v, wide %v", narrow, wide)
	}
}

func TestOrdinaryRootDoesNotAllocateDependencyFrameTables(t *testing.T) {
	session := &evaluationSession{}
	observed := false
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: DatafileContent{
		SchemaVersion: "4", Features: map[string]Feature{"flag": {BucketBy: "userId"}},
	}, Modules: []*FeaturevisorModule{{Name: "observe", AfterEvaluation: func(e Evaluation, _ EvaluateOptions) Evaluation {
		observed = true
		if session.frames != nil || session.memo != nil || session.segments != nil {
			t.Fatal("ordinary root allocated dependency tables")
		}
		return e
	}}}})
	defer f.Close()
	deps := f.getEvaluationDependencies(nil, OverrideOptions{})
	deps.session = session
	opts := EvaluateOptions{evaluateParams: evaluateParams{Type: EvaluationTypeFlag, FeatureKey: "flag"}, evaluateDependencies: deps}
	for attempt := 0; attempt < 2; attempt++ {
		if result := evaluateWithModules(opts); result.Reason == EvaluationReasonError {
			t.Fatal(result.Error)
		}
		if deps.session.frames != nil || deps.session.memo != nil || deps.session.segments != nil {
			t.Fatal("ordinary root allocated dependency tables")
		}
		if deps.session.depth != 0 || deps.session.rootFrameCount != 0 {
			t.Fatal("root frame retained after evaluation")
		}
	}
	if !observed {
		t.Fatal("root observer was not called")
	}
}
