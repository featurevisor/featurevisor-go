package featurevisor

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestDeprecatedFeaturesAndVariablesUseOneCodePerKind(t *testing.T) {
	var datafile DatafileContent
	if err := datafile.FromJSON(`{"schemaVersion":"4","revision":"1","segments":{},"features":{"legacy":{"deprecated":true,"bucketBy":"userId","variablesSchema":{"old":{"type":"string","defaultValue":"x","deprecated":true}},"traffic":[{"key":"all","conditions":"*","percentage":100000,"allocation":[]}]}},"variables":{"oldGlobal":{"type":"string","defaultValue":"y","deprecated":true}}}`); err != nil {
		t.Fatal(err)
	}
	var captured []string
	level := LogLevelWarn
	f := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile, LogLevel: &level, OnDiagnostic: func(d FeaturevisorDiagnostic) {
		if strings.Contains(d.Code, "deprecated") {
			captured = append(captured, d.Code)
		}
	}})
	defer f.Close()

	unique := func() []string {
		seen := map[string]bool{}
		var codes []string
		for _, code := range captured {
			if !seen[code] {
				seen[code] = true
				codes = append(codes, code)
			}
		}
		sort.Strings(codes)
		captured = nil
		return codes
	}

	f.IsEnabled("legacy")
	if got := unique(); !reflect.DeepEqual(got, []string{"deprecated_feature"}) {
		t.Fatalf("feature: %v", got)
	}
	f.GetVariable("legacy", "old")
	if got := unique(); !reflect.DeepEqual(got, []string{"deprecated_feature", "deprecated_variable"}) {
		t.Fatalf("feature variable: %v", got)
	}
	f.GetGlobalVariable("oldGlobal")
	if !reflect.DeepEqual(captured, []string{"deprecated_variable"}) {
		t.Fatalf("global variable: %v", captured)
	}
}
