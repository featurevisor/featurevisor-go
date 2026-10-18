package featurevisor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func alignmentV4Data() DatafileContent {
	return DatafileContent{SchemaVersion: "4", Revision: "v4", Features: map[string]Feature{
		"healthy": {BucketBy: []string{}, Traffic: []Traffic{{Key: "all", Conditions: "*", Percentage: 100000}}},
	}, Segments: map[string]Segment{}, Variables: map[string]GlobalVariable{"healthy": {Type: VariableTypeString, DefaultValue: "healthy"}}}
}

func TestNormalizationDiagnosticTimingReentryAndCacheLifetime(t *testing.T) {
	for _, schema := range []string{"2", "4"} {
		t.Run(schema, func(t *testing.T) {
			level := LogLevelError
			var diagnostics []FeaturevisorDiagnostic
			var f *Featurevisor
			f = CreateFeaturevisor(FeaturevisorOptions{LogLevel: &level, OnDiagnostic: func(d FeaturevisorDiagnostic) {
				if d.Code != "conditions_parse_error" {
					return
				}
				diagnostics = append(diagnostics, d)
				if f.GetRevision() != "accepted" {
					t.Errorf("callback saw old revision: %s", f.GetRevision())
				}
				if f.EvaluateGlobalVariable("broken").Reason != EvaluationReasonConditionsUnmet {
					t.Error("reentry did not see cached invalid root")
				}
				f.SetContext(Context{"callback": true})
			}})
			defer f.Close()
			data := alignmentV4Data()
			data.SchemaVersion = schema
			data.Revision = "accepted"
			data.Variables["broken"] = GlobalVariable{Conditions: " \n {oops", Type: VariableTypeString, DefaultValue: "wrong", DisabledValue: "disabled"}
			if !f.SetDatafile(data, true) {
				t.Fatal("valid datafile rejected")
			}
			if schema == "4" && len(diagnostics) != 0 {
				t.Fatal("v4 normalized eagerly")
			}
			if schema == "2" && len(diagnostics) != 1 {
				t.Fatal("v3 did not diagnose during acceptance")
			}
			for i := 0; i < 2; i++ {
				if value := f.GetGlobalVariable("broken"); value != "disabled" {
					t.Fatalf("root fallback: %#v", value)
				}
			}
			if len(diagnostics) != 1 {
				t.Fatalf("diagnostics repeated: %d", len(diagnostics))
			}
			d := diagnostics[0]
			if d.Level != LogLevelError || !reflect.DeepEqual(d.Details, map[string]interface{}{"conditions": " \n {oops", "entityType": "variable", "entityKey": "broken"}) {
				t.Fatalf("diagnostic details: %#v", d)
			}
			partial := alignmentV4Data()
			partial.SchemaVersion = "4"
			partial.Revision = "accepted"
			f.SetDatafile(partial)
			f.GetGlobalVariable("broken")
			if len(diagnostics) != 1 {
				t.Fatal("unrelated update invalidated entity cache")
			}
			f.SetDatafile(data)
			f.GetGlobalVariable("broken")
			if len(diagnostics) != 2 {
				t.Fatal("incoming replacement did not reset diagnostics")
			}
			f.SetDatafile(data, true)
			f.GetGlobalVariable("broken")
			if len(diagnostics) != 3 {
				t.Fatal("replacement did not reset caches")
			}
		})
	}
}

func TestMalformedSelectorsEverySiteFailLocally(t *testing.T) {
	for _, schema := range []string{"2", "4"} {
		for _, site := range []string{"feature", "segment", "force", "traffic", "rule override", "variation override", "variable", "override", "program", "legacy segments"} {
			t.Run(schema+"/"+site, func(t *testing.T) {
				data := alignmentV4Data()
				data.SchemaVersion = schema
				feature := data.Features["healthy"]
				feature.Traffic = append([]Traffic{}, feature.Traffic...)
				feature.VariablesSchema = map[string]VariableSchema{"copy": {Type: VariableTypeString, DefaultValue: "fallback"}}
				broken := "{oops"
				switch site {
				case "feature":
					feature.Conditions = broken
				case "segment":
					data.Segments["broken"] = Segment{Conditions: broken}
					feature.Conditions = map[string]interface{}{"not": []interface{}{"broken"}}
				case "force":
					enabled := true
					feature.Force = []Force{{Conditions: broken, Enabled: &enabled, Variables: map[string]VariableValue{"copy": "wrong"}}}
				case "traffic":
					feature.Traffic = append([]Traffic{{Key: "broken", Conditions: broken, Percentage: 100000, Variables: map[string]VariableValue{"copy": "wrong"}}}, feature.Traffic...)
				case "rule override":
					feature.Traffic[0].VariableOverrides = map[string][]VariableOverride{"copy": {{Conditions: broken, Value: "wrong"}}}
				case "variation override":
					feature.Variations = []Variation{{Value: "on", Variables: map[string]VariableValue{"copy": "fallback"}, VariableOverrides: map[string][]VariableOverride{"copy": {{Conditions: broken, Value: "wrong"}}}}}
					feature.Traffic[0].Allocation = []Allocation{{Variation: "on", Range: Range{0, 100000}}}
				case "legacy segments":
					feature.Traffic = append([]Traffic{{Key: "broken", Segments: broken, Percentage: 100000, Variables: map[string]VariableValue{"copy": "wrong"}}}, feature.Traffic...)
				default:
					variable := GlobalVariable{Type: VariableTypeString, DefaultValue: "fallback", DisabledValue: "fallback"}
					if site == "variable" {
						variable.Conditions = broken
					} else if site == "override" {
						variable.Overrides = []VariableOverride{{Conditions: broken, Value: "wrong"}}
					} else {
						variable.OverrideProgram = []VariableOverrideStep{{Key: "broken", Conditions: broken, OnMatch: 1, OnMiss: 1, Operations: []VariableMutationOperation{{Op: "set", Path: []interface{}{}, Value: "wrong"}}}}
					}
					data.Variables["subject"] = variable
				}
				data.Features["subject"] = feature
				level := LogLevelError
				count := 0
				f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level, OnDiagnostic: func(d FeaturevisorDiagnostic) {
					if d.Code == "conditions_parse_error" {
						count++
					}
				}})
				defer f.Close()
				if schema == "2" && count != 1 {
					t.Fatalf("eager count %d", count)
				}
				if schema == "4" && count != 0 {
					t.Fatal("premature lazy diagnostic")
				}
				for i := 0; i < 2; i++ {
					if _, exists := data.Variables["subject"]; exists {
						if got := f.GetGlobalVariable("subject"); got != "fallback" {
							t.Fatalf("value %#v", got)
						}
					} else if site == "feature" || site == "segment" {
						if e := f.EvaluateFlag("subject"); e.Reason != EvaluationReasonConditionsUnmet {
							t.Fatalf("root: %#v", e)
						}
						raw, _ := json.Marshal(f.EvaluateFlag("subject"))
						if site == "feature" && !strings.Contains(string(raw), `"conditions":{"or":[]}`) {
							t.Fatalf("invalid marker wire form: %s", raw)
						}
					} else if got := f.GetVariable("subject", "copy"); got != "fallback" {
						t.Fatalf("fallback: %#v", got)
					}
				}
				if count != 1 {
					t.Fatalf("parse count %d", count)
				}
				if !f.IsEnabled("healthy") || f.GetGlobalVariable("healthy") != "healthy" {
					t.Error("unrelated entity broken")
				}
			})
		}
	}
}

func TestMixedSelectorsExactMetadataAndLegacyEmptySemantics(t *testing.T) {
	for _, schema := range []string{"2", "3", "4"} {
		for _, empty := range []bool{false, true} {
			data := alignmentV4Data()
			data.SchemaVersion = schema
			conditions := []interface{}{map[string]interface{}{"attribute": "country", "operator": "equals", "value": "nl"}}
			if empty {
				conditions = []interface{}{}
			}
			feature := data.Features["healthy"]
			feature.Traffic = append([]Traffic{}, feature.Traffic...)
			feature.Conditions = conditions
			feature.RequiredFeatures = []RequiredFeature{"missing"}
			data.Features["subject"] = feature
			level := LogLevelFatal
			f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level})
			e := f.EvaluateFlag("subject", Context{"country": "nl"})
			first := interface{}(map[string]interface{}{"attribute": "country", "operator": "equals", "value": "nl"})
			if empty {
				first = map[string]interface{}{"and": []interface{}{}}
			}
			want := []interface{}{first, map[string]interface{}{"feature": "missing"}}
			if e.Reason != EvaluationReasonConditionsUnmet || !sameJSONValue(e.Conditions, want) {
				t.Fatalf("%s empty=%v: %#v want %#v", schema, empty, e.Conditions, want)
			}
			f.Close()
		}
	}
	for _, value := range []interface{}{[]interface{}{}, map[string]interface{}{"and": []interface{}{}}, map[string]interface{}{"not": []interface{}{}}, AndCondition{}, NotCondition{Not: []Condition{}}} {
		if AllConditionsAreMatched(value, Context{}) {
			t.Fatalf("empty v4 helper matched %#v", value)
		}
	}
	for _, value := range []interface{}{true, false, float64(42), []interface{}{true}, []interface{}{float64(42)}} {
		data := alignmentV4Data()
		feature := data.Features["healthy"]
		feature.Conditions = value
		data.Features["subject"] = feature
		level := LogLevelFatal
		f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level})
		if e := f.EvaluateFlag("subject"); e.Reason != EvaluationReasonConditionsUnmet {
			t.Fatalf("unsupported condition: %#v", e)
		}
		f.Close()
	}
	data := alignmentV4Data()
	data.SchemaVersion = "2"
	data.Segments["empty"] = Segment{Conditions: []interface{}{}}
	feature := data.Features["healthy"]
	feature.Conditions = "empty"
	data.Features["subject"] = feature
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data})
	defer f.Close()
	if !f.IsEnabled("subject") {
		t.Error("v3 normalization lost historical empty truth")
	}
}

func TestCombinedDependencyDepthBoundariesAndCleanup(t *testing.T) {
	for _, getters := range []int{0, 1, 128, 256, 257} {
		for _, segments := range []int{0, 1, 128, 255, 256, 257} {
			if getters == 0 && segments == 0 {
				continue
			}
			t.Run(fmt.Sprintf("getters=%d/segments=%d", getters, segments), func(t *testing.T) {
				data := alignmentV4Data()
				for i := 0; i <= getters; i++ {
					variable := GlobalVariable{Type: VariableTypeBoolean, DefaultValue: true}
					if i < getters {
						variable.Compose = fmt.Sprintf("g%d", i+1)
					} else if segments > 0 {
						variable.Conditions = "s0"
					}
					data.Variables[fmt.Sprintf("g%d", i)] = variable
				}
				for i := 0; i < segments; i++ {
					next := "*"
					if i+1 < segments {
						next = fmt.Sprintf("s%d", i+1)
					}
					data.Segments[fmt.Sprintf("s%d", i)] = Segment{Conditions: next}
				}
				level := LogLevelFatal
				f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level})
				defer f.Close()
				e := f.EvaluateGlobalVariable("g0")
				wantError := getters > 256 || getters+segments > 256
				if (e.Reason == EvaluationReasonError) != wantError {
					t.Fatalf("boundary mismatch: %#v", e)
				}
				if !f.IsEnabled("healthy") || f.GetGlobalVariable("healthy") != "healthy" {
					t.Error("failed session leaked")
				}
			})
		}
	}
}

func TestCompositionPurposeOwnershipMemoAndJSONOnce(t *testing.T) {
	data := alignmentV4Data()
	data.Variables["source"] = GlobalVariable{Type: VariableTypeJSON, DefaultValue: `{"items":[1]}`}
	data.Variables["snapshot"] = GlobalVariable{Type: VariableTypeBoolean, DefaultValue: true, Conditions: []interface{}{map[string]interface{}{"variable": "source", "operator": "notNull"}, map[string]interface{}{"variable": "source", "operator": "notNull"}}}
	data.Variables["view"] = GlobalVariable{Type: VariableTypeObject, Compose: []ComposeEntry{{Key: "a", Variable: "source"}, {Key: "b", Variable: "source"}, {Key: "gate", Variable: "snapshot"}}}
	level := LogLevelFatal
	purposes := []string{}
	var held []EvaluateOptions
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level, Modules: []*FeaturevisorModule{{BeforeEvaluation: func(o EvaluateOptions) EvaluateOptions {
		held = append(held, o)
		if o.VariableKey != nil && *o.VariableKey == "source" {
			purposes = append(purposes, o.DependencyPurpose)
			if o.DependencyOf == nil {
				t.Error("missing requester")
			}
		}
		return o
	}}}})
	defer f.Close()
	view := f.GetGlobalVariable("view").(map[string]interface{})
	if !reflect.DeepEqual(purposes, []string{"composition", "condition"}) {
		t.Fatalf("purpose memo: %#v", purposes)
	}
	if reflect.ValueOf(view["a"]).Pointer() != reflect.ValueOf(view["b"]).Pointer() {
		t.Error("composition JSON memo did not borrow the decoded source")
	}
	for _, options := range held {
		if options.session != nil || options.refreshState != nil || options.stateVersion != nil {
			t.Error("private session leaked")
		}
	}
	for _, value := range []interface{}{false, float64(0), "", "null", "123", nil, []interface{}{}, map[string]interface{}{"nested": true}} {
		raw, _ := json.Marshal(value)
		d := alignmentV4Data()
		d.Variables["source"] = GlobalVariable{Type: VariableTypeJSON, DefaultValue: string(raw)}
		d.Variables["alias"] = GlobalVariable{Type: VariableTypeJSON, Compose: "source"}
		d.Variables["chain"] = GlobalVariable{Type: VariableTypeJSON, Compose: "alias"}
		f.SetDatafile(d, true)
		if got := f.GetGlobalVariable("chain"); !sameJSONValue(got, value) {
			t.Fatalf("JSON once got %#v want %#v", got, value)
		}
	}
}

func TestLazyNormalizationConcurrentReads(t *testing.T) {
	data := alignmentV4Data()
	feature := data.Features["healthy"]
	feature.Conditions = `[{"attribute":"country","operator":"equals","value":"nl"}]`
	data.Features["subject"] = feature
	level := LogLevelFatal
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level})
	defer f.Close()
	var group sync.WaitGroup
	for i := 0; i < 50; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if !f.IsEnabled("subject", Context{"country": "nl"}) {
				t.Error("concurrent condition miss")
			}
		}()
	}
	group.Wait()
}

func TestMixedVersionUnreadEntitiesKeepHistoricalSemantics(t *testing.T) {
	for _, legacyFirst := range []bool{false, true} {
		legacy, modern := alignmentV4Data(), alignmentV4Data()
		legacy.SchemaVersion = "2"
		legacy.Segments["legacy"] = Segment{Conditions: []interface{}{}}
		feature := legacy.Features["healthy"]
		feature.Conditions = "legacy"
		legacy.Features["legacy"] = feature
		modern.Segments["modern"] = Segment{Conditions: []interface{}{}}
		feature = modern.Features["healthy"]
		feature.Conditions = "modern"
		modern.Features["modern"] = feature
		first, second := modern, legacy
		if legacyFirst {
			first, second = legacy, modern
		}
		beforeLegacy, _ := json.Marshal(legacy)
		beforeModern, _ := json.Marshal(modern)
		level := LogLevelFatal
		f := CreateFeaturevisor(FeaturevisorOptions{Datafile: first, LogLevel: &level})
		f.SetDatafile(second)
		if !f.IsEnabled("legacy") || f.IsEnabled("modern") {
			t.Fatal("mixed merge changed an unread entity's semantics")
		}
		afterLegacy, _ := json.Marshal(legacy)
		afterModern, _ := json.Marshal(modern)
		if string(beforeLegacy) != string(afterLegacy) || string(beforeModern) != string(afterModern) {
			t.Error("normalization changed caller definitions")
		}
		f.Close()
	}
}

func TestCompositionGateNullAndFreshDisabledMap(t *testing.T) {
	data := alignmentV4Data()
	data.Variables["null"] = GlobalVariable{Type: VariableTypeJSON, Compose: "healthy", Conditions: []interface{}{}}
	data.Variables["map"] = GlobalVariable{Type: VariableTypeObject, Compose: []ComposeEntry{{Key: "value", Variable: "healthy"}}, Conditions: []interface{}{}, DisabledValue: map[string]interface{}{"value": "disabled"}}
	level := LogLevelFatal
	sources := 0
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level, Modules: []*FeaturevisorModule{{BeforeEvaluation: func(o EvaluateOptions) EvaluateOptions {
		if o.DependencyPurpose == "composition" {
			sources++
		}
		return o
	}}}})
	defer f.Close()
	if got := f.GetGlobalVariable("null", OverrideOptions{DefaultVariableValue: "caller", DefaultVariableValueSet: true}); got != nil {
		t.Fatalf("caller default replaced a deliberate null gate result: %#v", got)
	}
	a, b := f.GetGlobalVariable("map"), f.GetGlobalVariable("map")
	if reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer() {
		t.Error("disabled composed map outer object was shared")
	}
	if sources != 0 {
		t.Fatalf("missed gate read %d composition sources", sources)
	}
}

func TestCompositionChildEffectiveContextAndStickyState(t *testing.T) {
	data := alignmentV4Data()
	data.Variables["source"] = GlobalVariable{Type: VariableTypeString, DefaultValue: "default", Overrides: []VariableOverride{{Conditions: map[string]interface{}{"attribute": "country", "operator": "equals", "value": "nl"}, Value: "dutch"}}}
	data.Variables["screen"] = GlobalVariable{Type: VariableTypeObject, Compose: []ComposeEntry{{Key: "flag", Feature: "healthy"}, {Key: "source", Variable: "source"}}}
	level := LogLevelFatal
	parentSticky := StickyVariables{"source": "parent"}
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level, Context: Context{"country": "de"}, StickyVariables: &parentSticky})
	defer f.Close()
	child := f.Spawn(Context{"country": "nl"}, SpawnOptions{StickyVariables: &StickyVariables{}, StickyFeatures: &StickyFeatures{"healthy": {Enabled: false}}})
	defer child.Close()
	if got := child.GetGlobalVariable("screen"); !sameJSONValue(got, map[string]interface{}{"flag": false, "source": "dutch"}) {
		t.Fatalf("child source state: %#v", got)
	}
	if got := f.GetGlobalVariable("screen"); !sameJSONValue(got, map[string]interface{}{"flag": true, "source": "parent"}) {
		t.Fatalf("parent source state: %#v", got)
	}
	child.SetStickyVariables(StickyVariables{"screen": nil}, true)
	if got := child.GetGlobalVariable("screen"); got != nil {
		t.Fatalf("whole sticky did not bypass assembly: %#v", got)
	}
}

func TestConditionSnapshotsOwnedWhileCompositionBorrows(t *testing.T) {
	data := alignmentV4Data()
	policy := map[string]interface{}{"limit": float64(50)}
	data.Variables["policy"] = GlobalVariable{Type: VariableTypeObject, DefaultValue: policy}
	data.Variables["raw"] = GlobalVariable{Type: VariableTypeBoolean, DefaultValue: true}
	feature := data.Features["healthy"]
	feature.Conditions = []interface{}{
		map[string]interface{}{"variable": "policy", "path": []interface{}{"limit"}, "operator": "equals", "value": float64(50)},
		map[string]interface{}{"variable": "raw", "operator": "notNull"},
		map[string]interface{}{"variable": "policy", "path": []interface{}{"limit"}, "operator": "equals", "value": float64(50)},
	}
	data.Features["gate"] = feature
	data.Variables["screen"] = GlobalVariable{Type: VariableTypeObject, Compose: []ComposeEntry{{Key: "first", Variable: "policy"}, {Key: "flag", Feature: "gate"}, {Key: "last", Variable: "policy"}}}
	level := LogLevelFatal
	purposes := []string{}
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level, Modules: []*FeaturevisorModule{{BeforeEvaluation: func(o EvaluateOptions) EvaluateOptions {
		if o.VariableKey != nil && *o.VariableKey == "policy" {
			purposes = append(purposes, o.DependencyPurpose)
		}
		if o.VariableKey != nil && *o.VariableKey == "raw" && o.DependencyPurpose == "condition" {
			policy["limit"] = float64(80)
		}
		return o
	}}}})
	defer f.Close()
	if got := f.GetGlobalVariable("screen"); !sameJSONValue(got, map[string]interface{}{"first": policy, "flag": true, "last": policy}) {
		t.Fatalf("snapshot ownership: %#v", got)
	}
	if !reflect.DeepEqual(purposes, []string{"composition", "condition"}) {
		t.Fatalf("purpose separation: %#v", purposes)
	}
}

func TestMixedDependencyEventsWithoutWarmingLazyCaches(t *testing.T) {
	data := alignmentV4Data()
	data.Segments["bridge"] = Segment{Conditions: `{"feature":"healthy"}`}
	feature := data.Features["healthy"]
	hash := "dependent"
	feature.Hash = &hash
	feature.Conditions = "bridge"
	data.Features["dependent"] = feature
	data.Variables["source"] = GlobalVariable{Hash: &hash, Type: VariableTypeBoolean, DefaultValue: true, Conditions: "bridge"}
	data.Variables["alias"] = GlobalVariable{Hash: &hash, Type: VariableTypeBoolean, Compose: "source"}
	data.Segments["last"] = Segment{Conditions: map[string]interface{}{"variable": "alias", "operator": "equals", "value": true}}
	feature.Conditions = "last"
	data.Features["last"] = feature
	data.Variables["broken"] = GlobalVariable{Hash: &hash, Conditions: "{broken", DefaultValue: true}
	level := LogLevelError
	diagnostics := 0
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level, OnDiagnostic: func(d FeaturevisorDiagnostic) {
		if d.Code == "conditions_parse_error" {
			diagnostics++
		}
	}})
	defer f.Close()
	var changed EventDetails
	f.On(EventNameDatafileSet, func(details EventDetails) { changed = details })
	patch := alignmentV4Data()
	patch.Revision = "changed"
	source := patch.Features["healthy"]
	newHash := "new"
	source.Hash = &newHash
	patch.Features["healthy"] = source
	f.SetDatafile(patch)
	if !sameStringSet(changed["features"], []string{"healthy", "dependent", "last"}) || !sameStringSet(changed["variables"], []string{"healthy", "source", "alias"}) {
		t.Fatalf("mixed graph notifications: %#v", changed)
	}
	if diagnostics != 0 || len(f.dataProvider().entities) != 0 {
		t.Fatal("dependency indexing warmed lazy definitions")
	}
	removed := alignmentV4Data()
	removed.Revision = "removed"
	f.SetDatafile(removed, true)
	if !sameStringSet(changed["features"], []string{"healthy", "dependent", "last"}) || !sameStringSet(changed["variables"], []string{"healthy", "source", "alias", "broken"}) {
		t.Fatalf("removal closure: %#v", changed)
	}
}

func TestAlternatingSegmentGetterDepthBoundary(t *testing.T) {
	for _, depth := range []int{127, 128, 129, 140} {
		data := alignmentV4Data()
		root := data.Features["healthy"]
		root.Conditions = "s0"
		data.Features["subject"] = root
		for i := 0; i < depth; i++ {
			data.Segments[fmt.Sprintf("s%d", i)] = Segment{Conditions: map[string]interface{}{"feature": fmt.Sprintf("f%d", i)}}
			feature := data.Features["healthy"]
			feature.Conditions = "*"
			if i+1 < depth {
				feature.Conditions = fmt.Sprintf("s%d", i+1)
			}
			data.Features[fmt.Sprintf("f%d", i)] = feature
		}
		level := LogLevelFatal
		f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level})
		e := f.EvaluateFlag("subject")
		if (e.Reason == EvaluationReasonError) != (depth > 128) {
			t.Fatalf("alternating depth %d: %#v", depth, e)
		}
		f.Close()
	}
}

func TestExplicitNullRootConditionsAreNotAbsent(t *testing.T) {
	level := LogLevelFatal
	f := CreateFeaturevisor(FeaturevisorOptions{LogLevel: &level, Datafile: `{"schemaVersion":"4","revision":"null","segments":{},"features":{"subject":{"bucketBy":[],"conditions":null,"traffic":[{"key":"all","conditions":"*","percentage":100000}]}},"variables":{"subject":{"type":"string","defaultValue":"wrong","conditions":null}}}`})
	defer f.Close()
	if e := f.EvaluateFlag("subject"); e.Reason != EvaluationReasonConditionsUnmet {
		t.Fatalf("null feature root matched: %#v", e)
	}
	if e := f.EvaluateGlobalVariable("subject"); e.Reason != EvaluationReasonConditionsUnmet {
		t.Fatalf("null variable root matched: %#v", e)
	}
}

func TestCompositionMemoDoesNotTraverseBorrowedHostValues(t *testing.T) {
	data := alignmentV4Data()
	source := map[string]interface{}{"host": func() {}, "nested": map[string]interface{}{"value": true}}
	data.Variables["source"] = GlobalVariable{Type: VariableTypeObject, DefaultValue: source}
	data.Variables["alias"] = GlobalVariable{Type: VariableTypeObject, Compose: "source"}
	data.Variables["map"] = GlobalVariable{Type: VariableTypeObject, Compose: []ComposeEntry{{Key: "a", Variable: "alias"}, {Key: "b", Variable: "alias"}}}
	level := LogLevelFatal
	reads := 0
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level, Modules: []*FeaturevisorModule{{BeforeEvaluation: func(o EvaluateOptions) EvaluateOptions {
		if o.VariableKey != nil && *o.VariableKey == "alias" {
			reads++
		}
		return o
	}}}})
	defer f.Close()
	value := f.GetGlobalVariable("map").(map[string]interface{})
	if reads != 1 || reflect.ValueOf(value["a"]).Pointer() != reflect.ValueOf(source).Pointer() || reflect.ValueOf(value["b"]).Pointer() != reflect.ValueOf(source).Pointer() {
		t.Fatalf("borrowed host value was copied or not memoized: %d %#v", reads, value)
	}
}

func TestMalformedSegmentAtDepthBoundaryStillFailsOnlyItsSelector(t *testing.T) {
	data := alignmentV4Data()
	data.Segments["broken"] = Segment{Conditions: "{broken"}
	for i := 0; i < 256; i++ {
		data.Variables[fmt.Sprintf("g%d", i)] = GlobalVariable{Type: VariableTypeString, Compose: fmt.Sprintf("g%d", i+1)}
	}
	data.Variables["g256"] = GlobalVariable{Type: VariableTypeString, DefaultValue: "wrong", DisabledValue: "blocked", Conditions: map[string]interface{}{"not": []interface{}{"broken"}}}
	level := LogLevelFatal
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: data, LogLevel: &level})
	defer f.Close()
	if got := f.GetGlobalVariable("g0"); got != "blocked" {
		t.Fatalf("malformed selector became a depth failure: %#v", got)
	}
}

func TestUnreleasedVariablePrerequisiteWireFieldIsCompletelyIgnored(t *testing.T) {
	level := LogLevelFatal
	f := CreateFeaturevisor(FeaturevisorOptions{LogLevel: &level})
	defer f.Close()
	if !f.SetDatafile(`{"schemaVersion":"4","revision":"ignored","segments":{},"features":{"subject":{"bucketBy":[],"requiredVariables":false,"traffic":[{"key":"all","conditions":"*","percentage":100000}]}},"variables":{"subject":{"type":"string","defaultValue":"healthy","requiredVariables":"invalid and ignored"}}}`, true) {
		t.Fatal("ignored unreleased field rejected a valid datafile")
	}
	if !f.IsEnabled("subject") || f.GetGlobalVariable("subject") != "healthy" {
		t.Error("unreleased field influenced evaluation")
	}
}
