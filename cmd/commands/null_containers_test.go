package commands

import (
	"encoding/json"
	"fmt"
	"testing"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

func TestNullChecksUseNativeGetterJSONIdentity(t *testing.T) {
	rows := []struct {
		name  string
		value interface{}
		null  bool
	}{
		{"nil strings", []string(nil), true},
		{"nil interfaces", []interface{}(nil), true},
		{"nil map", map[string]interface{}(nil), true},
		{"nil pointer", (*string)(nil), true},
		{"empty strings", []string{}, false},
		{"empty map", map[string]interface{}{}, false},
	}
	var datafile featurevisor.DatafileContent
	if err := json.Unmarshal([]byte(`{
		"schemaVersion":"2","revision":"null-checks","segments":{},
		"features":{"subject":{"bucketBy":"userId","traffic":[{"key":"all","segments":"*","percentage":100000}],"variablesSchema":{"value":{"type":"object","defaultValue":{}}}}},
		"variables":{"subject":{"type":"object","defaultValue":{}}}
	}`), &datafile); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		for _, feature := range []string{"", "subject"} {
			for _, op := range []string{"isNull", "notNull", "exists", "notExists", "empty", "notEmpty"} {
				t.Run(fmt.Sprintf("%s/%s/%s", row.name, feature, op), func(t *testing.T) {
					payload := map[string]interface{}{"nested": row.value}
					stickyVariables := featurevisor.StickyVariables{"subject": payload}
					stickyFeatures := featurevisor.StickyFeatures{
						"subject": featurevisor.EvaluatedFeature{Enabled: true, Variables: map[featurevisor.VariableKey]featurevisor.VariableValue{"value": payload}},
					}
					level := featurevisor.LogLevelFatal
					diagnostics := &checkDiagnosticCollector{}
					f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
						Datafile: datafile, StickyVariables: &stickyVariables, StickyFeatures: &stickyFeatures,
						LogLevel: &level, OnDiagnostic: diagnostics.record,
					})
					defer f.Close()
					resolver := nativeCheckResolver{getter: f, diagnostics: diagnostics}
					leaf := map[string]interface{}{"variable": "subject", "path": "nested", "operator": op}
					if feature != "" {
						leaf["feature"], leaf["variable"] = feature, "value"
					}
					result := evaluateAssertionChecksDetailed(map[string]interface{}{"checks": []interface{}{leaf}}, "", resolver, datafile, "")
					want := op == "exists" || (op == "isNull" && row.null) || ((op == "notNull" || op == "empty") && !row.null)
					if result.HadError || (len(result.Failures) == 0) != want {
						t.Fatalf("got %#v want pass=%v", result, want)
					}
					leaf["path"] = "nested.missing"
					missing := evaluateAssertionChecksDetailed(map[string]interface{}{"checks": []interface{}{leaf}}, "", resolver, datafile, "")
					if missing.HadError || (len(missing.Failures) == 0) != (op == "notExists") {
						t.Fatalf("missing path got %#v", missing)
					}
				})
			}
		}
	}
}
