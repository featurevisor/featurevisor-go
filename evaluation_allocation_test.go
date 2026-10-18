package featurevisor

import (
	"reflect"
	"testing"
)

func TestDebugEvaluationRetainsLiveAudienceAndMetadata(t *testing.T) {
	observed := false
	var received []logDetails
	handler := diagnosticOutputHandler(func(level LogLevel, message logMessage, details logDetails) {
		if level != LogLevelDebug || message != "matched" {
			t.Fatal("changed diagnostic envelope")
		}
		received = append(received, details)
	})
	reporter := newDiagnosticReporter(diagnosticReporterOptions{
		Handler:      &handler,
		ShouldHandle: func(level LogLevel) bool { return observed },
	})
	evaluation := Evaluation{FeatureKey: "checkout", VariableValue: map[string]interface{}{"nested": true}}
	reporter.debugEvaluation("matched", evaluation)
	if len(received) != 0 {
		t.Fatal("unobserved diagnostic was emitted")
	}
	observed = true
	reporter.Debug("matched", logDetails{"evaluation": evaluation})
	reporter.debugEvaluation("matched", evaluation)
	if len(received) != 2 || !reflect.DeepEqual(received[0], received[1]) {
		t.Fatalf("metadata changed: %#v", received)
	}
	observed = false
	reporter.debugEvaluation("matched", evaluation)
	if len(received) != 2 {
		t.Fatal("diagnostic audience was cached")
	}
}

func TestMutationReplaceOwnsTheCopiedObject(t *testing.T) {
	original := map[string]interface{}{"value": "before", "untouched": true}
	result := mutationReplace(original, "value", "after").(map[string]interface{})
	delete(result, "untouched")
	if original["value"] != "before" || original["untouched"] != true || result["value"] != "after" {
		t.Fatalf("mutation changed caller ownership: %#v, %#v", original, result)
	}
}
