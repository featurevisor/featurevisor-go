package featurevisor

import (
	"encoding/json"
	"fmt"
	"testing"
)

func dependencyChainDatafile(t *testing.T, length int) string {
	t.Helper()
	features := map[string]interface{}{}
	for index := 0; index < length; index++ {
		conditions := "*"
		if index > 0 {
			conditions = fmt.Sprintf(`[{"feature":"f%d"}]`, index-1)
		}
		features[fmt.Sprintf("f%d", index)] = map[string]interface{}{
			"bucketBy": "userId",
			"traffic":  []interface{}{map[string]interface{}{"key": "all", "conditions": conditions, "percentage": 100000}},
		}
	}
	data, err := json.Marshal(map[string]interface{}{"schemaVersion": "4", "revision": "depth", "segments": map[string]interface{}{}, "features": features})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDependencyFailuresReportOnceForEachPublicCall(t *testing.T) {
	var errors []string
	level := LogLevelError
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: dependencyChainDatafile(t, 300), LogLevel: &level, OnDiagnostic: func(d FeaturevisorDiagnostic) {
		if d.Level == LogLevelError {
			errors = append(errors, d.Code)
		}
	}})
	defer f.Close()
	context := Context{"userId": "u"}
	if !f.IsEnabled("f256", context) {
		t.Fatal("expected the 256 handoff chain to evaluate")
	}
	if f.IsEnabled("f299", context) {
		t.Fatal("expected the deep chain to fail")
	}
	if variation := f.GetVariation("f299", context); variation != nil {
		t.Fatalf("expected no variation, got %v", *variation)
	}
	if len(errors) != 2 || errors[0] != "evaluation_error" || errors[1] != "evaluation_error" {
		t.Fatalf("expected one evaluation_error per public call, got %v", errors)
	}
}
