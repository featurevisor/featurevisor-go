package featurevisor

import (
	"fmt"
	"testing"
)

func TestFullRefreshKeepsDiagnosticsAndDependencyEdgesIsolated(t *testing.T) {
	for _, schema := range []string{"2", "4"} {
		for _, replace := range []bool{false, true} {
			t.Run(fmt.Sprintf("schema%s/replace%v", schema, replace), func(t *testing.T) {
				raw := fmt.Sprintf(`{"schemaVersion":%q,"revision":"one","segments":{"s":{"conditions":"*"}},"features":{"flag":{"hash":"flag","conditions":"s","bucketBy":"userId","traffic":[]}},"variables":{"source":{"hash":"source","type":"string","defaultValue":"yes"},"alias":{"hash":"alias","type":"string","compose":"source"},"broken":{"hash":"broken","type":"string","conditions":"[broken","defaultValue":"no"}}}`, schema)
				var diagnostics int
				level := LogLevelError
				f := CreateFeaturevisor(FeaturevisorOptions{Datafile: raw, LogLevel: &level, OnDiagnostic: func(d FeaturevisorDiagnostic) {
					if d.Code == "conditions_parse_error" {
						diagnostics++
					}
				}})
				defer f.Close()
				if f.dependencyIndex != nil {
					t.Fatal("initial load indexed definitions")
				}
				old := f.dataProvider()
				old.GetGlobalVariable("broken")
				if diagnostics != 1 {
					t.Fatalf("initial diagnostics: %d", diagnostics)
				}
				for revision := 0; revision < 2; revision++ {
					if !f.SetDatafile(raw, replace) {
						t.Fatal("rejected identical definitions")
					}
					if len(f.dataProvider().entities) != 0 {
						t.Fatal("incoming readers were warmed")
					}
					f.dataProvider().GetGlobalVariable("broken")
					f.dataProvider().GetGlobalVariable("broken")
					if diagnostics != revision+2 {
						t.Fatalf("diagnostics per load: %d", diagnostics)
					}
				}
				old.GetGlobalVariable("broken")
				if diagnostics != 3 {
					t.Fatal("old reader diagnostic generation changed")
				}
				patch := fmt.Sprintf(`{"schemaVersion":%q,"revision":"changed","segments":{},"features":{},"variables":{"alias":{"hash":"alias","type":"string","compose":"other"}}}`, schema)
				if !f.SetDatafile(patch) {
					t.Fatal("rejected same hash changed edge")
				}
				owner := dependencyEntity{"variable", "alias"}
				if f.dependencyIndex.owners[owner][dependencyEntity{"variable", "source"}] || !f.dependencyIndex.owners[owner][dependencyEntity{"variable", "other"}] {
					t.Fatal("incorrect refreshed edges")
				}
				if !f.SetDatafile(fmt.Sprintf(`{"schemaVersion":%q,"revision":"empty","segments":{},"features":{},"variables":{}}`, schema), true) {
					t.Fatal("rejected removal")
				}
				if len(f.dependencyIndex.owners) != 0 || len(f.dependencyIndex.reverse) != 0 || len(f.dataProvider().entitySlots) != 0 {
					t.Fatal("replacement retained removed state")
				}
			})
		}
	}
}

func TestFullRefreshPreservesHashlessEventsAndIgnoresPayloadSelectors(t *testing.T) {
	for _, replace := range []bool{false, true} {
		raw := `{"schemaVersion":"4","revision":"one","segments":{},"features":{},"variables":{"payload":{"type":"object","defaultValue":{"conditions":"phantom","compose":"hidden","feature":"ghost","variable":"missing"}}}}`
		level := LogLevelError
		f := CreateFeaturevisor(FeaturevisorOptions{Datafile: raw, LogLevel: &level})
		var events []EventDetails
		f.On("datafile_set", func(details EventDetails) { events = append(events, details) })
		for load := 0; load < 2; load++ {
			if !f.SetDatafile(raw, replace) {
				t.Fatal("rejected hashless definition")
			}
			if len(f.dependencyIndex.reverse) != 0 {
				t.Fatal("payload properties became dependency edges")
			}
			keys := events[load]["variables"].([]string)
			if len(keys) != 1 || keys[0] != "payload" {
				t.Fatalf("hashless notifications changed: %v", keys)
			}
		}
		f.Close()
	}
}
