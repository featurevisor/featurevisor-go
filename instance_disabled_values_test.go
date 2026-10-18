package featurevisor

import (
	"fmt"
	"testing"
)

type disabledValueEvaluator interface {
	IsEnabled(string, ...interface{}) bool
	GetVariation(string, ...interface{}) *string
	EvaluateVariation(string, ...interface{}) Evaluation
	GetVariable(string, string, ...interface{}) VariableValue
}

func TestDisabledVariationPrecedesStickyPayloadAndVariationAvailability(t *testing.T) {
	for _, test := range []struct {
		name    string
		feature string
		sticky  *StickyFeatures
		want    *string
		reason  EvaluationReason
	}{
		{
			name:    "disabled value without variations",
			feature: `{"bucketBy":["userId"],"disabledVariationValue":"control","traffic":[]}`,
			want:    stringPtr("control"), reason: EvaluationReasonVariationDisabled,
		},
		{
			name:    "disabled value with variations",
			feature: `{"bucketBy":["userId"],"disabledVariationValue":"control","variations":[{"value":"treatment"}],"traffic":[]}`,
			want:    stringPtr("control"), reason: EvaluationReasonVariationDisabled,
		},
		{
			name:    "disabled value wins over sticky variation",
			feature: `{"bucketBy":["userId"],"disabledVariationValue":"control","variations":[{"value":"treatment"}],"traffic":[]}`,
			sticky:  &StickyFeatures{"checkout": {Enabled: false, Variation: stringPtr("treatment")}},
			want:    stringPtr("control"), reason: EvaluationReasonVariationDisabled,
		},
		{
			name:    "disabled sticky variation without a disabled value returns nil",
			feature: `{"bucketBy":["userId"],"variations":[{"value":"treatment"}],"traffic":[]}`,
			sticky:  &StickyFeatures{"checkout": {Enabled: false, Variation: stringPtr("treatment")}},
			reason:  EvaluationReasonDisabled,
		},
		{
			name:    "enabled sticky variation remains authoritative",
			feature: `{"bucketBy":["userId"],"disabledVariationValue":"control","traffic":[]}`,
			sticky:  &StickyFeatures{"checkout": {Enabled: true, Variation: stringPtr("treatment")}},
			want:    stringPtr("treatment"), reason: EvaluationReasonSticky,
		},
		{
			name:    "enabled feature without variations still returns nil",
			feature: `{"bucketBy":["userId"],"disabledVariationValue":"control","traffic":[{"key":"all","segments":"*","percentage":100000}]}`,
			reason:  EvaluationReasonNoVariations,
		},
		{
			name:    "disabled feature without variations reports disabled",
			feature: `{"bucketBy":["userId"],"traffic":[]}`,
			reason:  EvaluationReasonDisabled,
		},
	} {
		for _, child := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/child=%t", test.name, child), func(t *testing.T) {
				var datafile DatafileContent
				if err := datafile.FromJSON(`{"schemaVersion":"2","revision":"disabled-values","segments":{},"features":{"checkout":` + test.feature + `}}`); err != nil {
					t.Fatal(err)
				}
				f := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, StickyFeatures: test.sticky})
				defer f.Close()
				var evaluator disabledValueEvaluator = f
				if child {
					c := f.Spawn(Context{}, SpawnOptions{StickyFeatures: test.sticky})
					defer c.Close()
					evaluator = c
				}
				actual := evaluator.GetVariation("checkout", Context{"userId": "customer"})
				if (actual == nil) != (test.want == nil) || (actual != nil && test.want != nil && *actual != *test.want) {
					t.Fatalf("variation: got %v, want %v", actual, test.want)
				}
				details := evaluator.EvaluateVariation("checkout", Context{"userId": "customer"})
				if details.Reason != test.reason {
					t.Fatalf("reason: got %q, want %q", details.Reason, test.reason)
				}
			})
		}
	}
}

func TestDisabledFeatureVariablePrecedesStickyPayload(t *testing.T) {
	var datafile DatafileContent
	if err := datafile.FromJSON(`{
		"schemaVersion":"2","revision":"disabled-variables","segments":{},
		"features":{"checkout":{"bucketBy":["userId"],"traffic":[],"variablesSchema":{
			"disabled":{"type":"string","defaultValue":"default","disabledValue":"disabled"},
			"defaulted":{"type":"string","defaultValue":"default","useDefaultWhenDisabled":true},
			"unset":{"type":"string","defaultValue":"default"},
			"nullable":{"type":"json","defaultValue":"true","disabledValue":null}
		}}}
	}`); err != nil {
		t.Fatal(err)
	}
	sticky := &StickyFeatures{"checkout": {Enabled: false, Variables: map[VariableKey]VariableValue{
		"disabled": "sticky", "defaulted": "sticky", "unset": "sticky", "nullable": "true",
	}}}
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, StickyFeatures: sticky})
	defer f.Close()
	c := f.Spawn(Context{}, SpawnOptions{StickyFeatures: sticky})
	defer c.Close()
	for _, evaluator := range []disabledValueEvaluator{f, c} {
		for key, want := range map[string]VariableValue{"disabled": "disabled", "defaulted": "default", "unset": nil, "nullable": nil} {
			if actual := evaluator.GetVariable("checkout", key); actual != want {
				t.Errorf("%s: got %#v, want %#v", key, actual, want)
			}
		}
	}
}

func TestGlobalRequirementUsesDisabledVariationValue(t *testing.T) {
	var datafile DatafileContent
	if err := datafile.FromJSON(`{
		"schemaVersion":"2","revision":"disabled-requirement","segments":{},
		"features":{"checkout":{"bucketBy":["userId"],"disabledVariationValue":"control","traffic":[]}},
		"variables":{"message":{"type":"string","defaultValue":"available","disabledValue":"blocked",
			"requiredFeatures":[{"feature":"checkout","enabled":false,"variation":"control"}]}}
	}`); err != nil {
		t.Fatal(err)
	}
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile})
	defer f.Close()
	if actual := f.GetGlobalVariable("message"); actual != "available" {
		t.Fatalf("got %#v, want available", actual)
	}
}
