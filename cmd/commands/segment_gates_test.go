package commands

import (
	"encoding/json"
	"strings"
	"testing"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

func segmentTestContent(t *testing.T, raw string) featurevisor.DatafileContent {
	t.Helper()
	var content featurevisor.DatafileContent
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		t.Fatal(err)
	}
	return content
}

func TestDependentSegmentOwnerUsesSDKContextAtAndStickyState(t *testing.T) {
	content := segmentTestContent(t, `{"schemaVersion":"4","revision":"segments","segments":{},"features":{"gate":{"bucketBy":"userId","traffic":[{"key":"rule","conditions":"*","percentage":50000}]}},"variables":{"policy":{"type":"object","defaultValue":{"enabled":true}}}}`)
	definitions := map[string]interface{}{
		"subject": map[string]interface{}{"conditions": []interface{}{"nested", map[string]interface{}{"variable": "policy", "path": []interface{}{"enabled"}, "operator": "equals", "value": true}}},
		"nested":  map[string]interface{}{"conditions": map[string]interface{}{"feature": "gate"}},
	}
	for _, tc := range []struct {
		at     float64
		sticky bool
		want   bool
	}{{25, false, true}, {75, false, false}, {75, true, true}} {
		assertion := map[string]interface{}{"context": map[string]interface{}{"userId": "native"}, "at": tc.at, "expectedToMatch": tc.want}
		if tc.sticky {
			assertion["stickyFeatures"] = map[string]interface{}{"gate": map[string]interface{}{"enabled": true}}
		}
		if result := runSegmentWithSDK(assertion, "subject", definitions, content, "error"); result.HasError {
			t.Fatal(result.Errors)
		}
	}
	assertion := map[string]interface{}{"at": float64(25), "expectedToMatch": false, "stickyVariables": map[string]interface{}{"policy": map[string]interface{}{"enabled": false}}}
	if result := runSegmentWithSDK(assertion, "subject", definitions, content, "error"); result.HasError {
		t.Fatal(result.Errors)
	}
}

func TestSegmentChecksPreserveSelectedTargetAndRestoreOnlyMissingDefinitions(t *testing.T) {
	content := segmentTestContent(t, `{"schemaVersion":"4","revision":"target","segments":{"selected":{"conditions":"*"}},"features":{"__featurevisor_segment_gate__/selected":{"bucketBy":[],"traffic":[]}},"variables":{"subject":{"type":"boolean","defaultValue":true}}}`)
	definitions := map[string]interface{}{
		"selected": map[string]interface{}{"conditions": map[string]interface{}{"attribute": "country", "operator": "equals", "value": "never"}},
		"restored": map[string]interface{}{"conditions": []interface{}{"selected", map[string]interface{}{"attribute": "plan", "operator": "equals", "value": "pro"}}},
	}
	assertion := map[string]interface{}{"context": map[string]interface{}{"country": "nl"}, "checks": []interface{}{map[string]interface{}{"segment": "selected", "matches": true}}, "children": []interface{}{map[string]interface{}{"context": map[string]interface{}{"plan": "pro"}, "checks": []interface{}{map[string]interface{}{"segment": "restored", "matches": true}}}}}
	instance, diagnostics := buildInstanceForAssertion(content, "error", assertion)
	defer instance.Close()
	if result := RunTestScenario(assertion, instance, diagnostics, content, definitions, "error"); result.HasError {
		t.Fatal(result.Errors)
	}
	keys := segmentGateKeys(content, definitions)
	if keys["selected"] == segmentGatePrefix+"selected" {
		t.Fatal("synthetic gate collided with a real feature")
	}
	if instance.IsEnabled(segmentGatePrefix + "selected") {
		t.Fatal("real feature was replaced")
	}
	collector := newCoverageCollector("", assertion)
	collector.synthetic = map[string]bool{keys["selected"]: true}
	rule := "selected"
	collector.record(featurevisor.Evaluation{FeatureKey: keys["selected"], RuleKey: &rule})
	collector.record(featurevisor.Evaluation{FeatureKey: segmentGatePrefix + "selected", RuleKey: &rule})
	if len(collector.list()) != 1 {
		t.Fatal("coverage did not separate real features from synthetic gates")
	}
}

func TestSegmentPreflightWalksCompleteClosureWithoutGetters(t *testing.T) {
	for _, site := range []string{"feature root", "force", "traffic", "rule variable override", "variation variable override", "global root", "global override", "global program", "alias", "map composition"} {
		t.Run(site, func(t *testing.T) {
			content := segmentTestContent(t, `{"schemaVersion":"4","revision":"closure","segments":{},"features":{"gate":{"bucketBy":[],"variablesSchema":{"value":{"type":"boolean","defaultValue":true}},"traffic":[{"key":"all","conditions":"*","percentage":100000}]}},"variables":{"source":{"type":"boolean","defaultValue":true}}}`)
			missing := map[string]interface{}{"variable": "missing", "operator": "notNull"}
			feature := content.Features["gate"]
			variable := content.Variables["source"]
			condition := interface{}(map[string]interface{}{"feature": "gate"})
			switch site {
			case "feature root":
				feature.Conditions = missing
			case "force":
				feature.Force = []featurevisor.Force{{Conditions: missing}}
			case "traffic":
				feature.Traffic[0].Conditions = missing
			case "rule variable override":
				feature.Traffic[0].VariableOverrides = map[string][]featurevisor.VariableOverride{"value": {{Conditions: missing}}}
				condition = map[string]interface{}{"feature": "gate", "variable": "value", "operator": "notNull"}
			case "variation variable override":
				feature.Variations = []featurevisor.Variation{{Value: "on", VariableOverrides: map[string][]featurevisor.VariableOverride{"value": {{Conditions: missing}}}}}
				condition = map[string]interface{}{"feature": "gate", "variable": "value", "operator": "notNull"}
			default:
				condition = map[string]interface{}{"variable": "source", "operator": "notNull"}
				switch site {
				case "global root":
					variable.Conditions = missing
				case "global override":
					variable.Overrides = []featurevisor.VariableOverride{{Conditions: missing}}
				case "global program":
					variable.OverrideProgram = []featurevisor.VariableOverrideStep{{Conditions: missing}}
				case "alias":
					variable.Compose = "missing"
				case "map composition":
					variable.Compose = []featurevisor.ComposeEntry{{Key: "missing", Variable: "missing"}}
				}
			}
			content.Features["gate"], content.Variables["source"] = feature, variable
			definitions := map[string]interface{}{"subject": map[string]interface{}{"conditions": []interface{}{"middle"}}, "middle": map[string]interface{}{"conditions": condition}}
			// A specialized segment must not hide missing raw references during preflight.
			content.Segments["subject"] = featurevisor.Segment{Conditions: "*"}
			calls := 0
			level := featurevisor.LogLevelFatal
			instance := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{Datafile: content, LogLevel: &level, Modules: []*featurevisor.FeaturevisorModule{{BeforeEvaluation: func(o featurevisor.EvaluateOptions) featurevisor.EvaluateOptions { calls++; return o }}}})
			defer instance.Close()
			resolver := nativeCheckResolver{getter: instance, diagnostics: &checkDiagnosticCollector{}, segmentDefinitions: definitions, datafile: content}
			assertion := map[string]interface{}{"checks": []interface{}{map[string]interface{}{"or": []interface{}{map[string]interface{}{"feature": "gate", "enabled": true}, map[string]interface{}{"segment": "subject", "matches": false}}}}}
			if result := preflightAssertionTree(assertion, "", resolver, content, true); !result.HasError || !strings.Contains(result.Errors, "missing") {
				t.Fatalf("missing closure was not rejected: %#v", result)
			}
			if calls != 0 {
				t.Fatalf("preflight invoked %d getters", calls)
			}
		})
	}
}

func TestAttributeSegmentChainsCanOmitEnvironmentButDependentOnesCannot(t *testing.T) {
	definitions := map[string]interface{}{"owner": map[string]interface{}{"conditions": "attribute"}, "attribute": map[string]interface{}{"conditions": map[string]interface{}{"attribute": "country", "operator": "equals", "value": "nl"}}}
	test := map[string]interface{}{"_segments": definitions}
	assertion := map[string]interface{}{"context": map[string]interface{}{"country": "nl"}, "expectedToMatch": true}
	if result := runSegmentSpec(assertion, "owner", test, nil, map[string]interface{}{}, "error"); result.HasError {
		t.Fatal(result.Errors)
	}
	definitions["attribute"] = map[string]interface{}{"conditions": map[string]interface{}{"feature": "missing"}}
	if result := runSegmentSpec(assertion, "owner", test, nil, map[string]interface{}{}, "error"); !result.HasError || !strings.Contains(result.Errors, "datafile not found") {
		t.Fatal("dependent segment accepted an absent selected datafile")
	}
}
