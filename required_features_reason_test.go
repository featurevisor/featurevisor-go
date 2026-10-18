package featurevisor

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRequiredFeaturesUnmetReasonInEvaluationAndCallbacks(t *testing.T) {
	level := LogLevelFatal
	var observed []EvaluationReason
	f := CreateFeaturevisor(FeaturevisorOptions{
		Datafile: `{"schemaVersion":"2","revision":"reason","segments":{},"features":{
			"subject":{"bucketBy":"userId","requiredFeatures":["missing"],"traffic":[]}
		}}`,
		LogLevel: &level,
		Modules: []*FeaturevisorModule{{Name: "observe", AfterEvaluation: func(evaluation Evaluation, _ EvaluateOptions) Evaluation {
			if evaluation.FeatureKey == "subject" {
				observed = append(observed, evaluation.Reason)
			}
			return evaluation
		}}},
	})
	defer f.Close()
	evaluation := f.EvaluateFlag("subject", Context{}, OverrideOptions{})
	if evaluation.Reason != EvaluationReasonConditionsUnmet || evaluation.Enabled == nil || *evaluation.Enabled {
		t.Fatalf("unexpected unmet requirement: %#v", evaluation)
	}
	if !reflect.DeepEqual(observed, []EvaluationReason{EvaluationReasonConditionsUnmet}) {
		t.Fatalf("callback reasons: %#v", observed)
	}
	encoded, err := json.Marshal(evaluation)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]interface{}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["reason"] != "conditions_unmet" || !reflect.DeepEqual(wire["conditions"], []interface{}{map[string]interface{}{"feature": "missing"}}) {
		t.Fatalf("unexpected serialized evaluation: %s", encoded)
	}
}
