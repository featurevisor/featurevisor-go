package commands

import (
	"testing"

	"github.com/featurevisor/featurevisor-go/v3"
)

func TestGetEvaluationValueVariableOverrideIndex(t *testing.T) {
	index := 2
	evaluation := featurevisor.Evaluation{
		Type:                  featurevisor.EvaluationTypeVariable,
		FeatureKey:            "test",
		Reason:                featurevisor.EvaluationReasonVariableOverrideRule,
		VariableOverrideIndex: &index,
	}

	value := getEvaluationValue(evaluation, "variableOverrideIndex")
	if value != 2 {
		t.Fatalf("expected variableOverrideIndex to be 2, got %#v", value)
	}
}
