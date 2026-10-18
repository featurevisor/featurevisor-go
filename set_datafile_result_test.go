package featurevisor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const acceptedDatafileJSON = `{"schemaVersion":"2","revision":"accepted","segments":{},"features":{"flag":{"key":"flag","hash":"stable","bucketBy":"userId","traffic":[{"key":"all","segments":"*","percentage":100000,"enabled":true}]}},"variables":{"colour":{"key":"colour","hash":"stable","type":"string","defaultValue":"blue"}}}`

func TestSetDatafileAcceptanceAcrossInputsAndModes(t *testing.T) {
	var typed DatafileContent
	if err := typed.FromJSON(acceptedDatafileJSON); err != nil {
		t.Fatal(err)
	}
	var object map[string]interface{}
	if err := json.Unmarshal([]byte(acceptedDatafileJSON), &object); err != nil {
		t.Fatal(err)
	}
	for name, input := range map[string]interface{}{"json": acceptedDatafileJSON, "typed": typed, "pointer": &typed, "map": object} {
		for _, mode := range []struct {
			name string
			args []bool
		}{{"default", nil}, {"merge", []bool{false}}, {"replace", []bool{true}}} {
			t.Run(name+"/"+mode.name, func(t *testing.T) {
				diagnostics := []FeaturevisorDiagnostic{}
				initial := strings.NewReplacer("flag", "retainedFlag", "colour", "retainedColour", "accepted", "initial").Replace(acceptedDatafileJSON)
				f := CreateFeaturevisor(FeaturevisorOptions{Datafile: initial, OnDiagnostic: func(d FeaturevisorDiagnostic) { diagnostics = append(diagnostics, d) }})
				defer f.Close()
				events := []EventDetails{}
				f.On(EventNameDatafileSet, func(d EventDetails) { events = append(events, d) })
				for i := 0; i < 2; i++ {
					if !f.SetDatafile(input, mode.args...) {
						t.Fatal("valid and unchanged datafiles must both be accepted")
					}
				}
				if len(events) != 2 || events[1]["revisionChanged"] != false || events[1]["replaced"] != (mode.name == "replace") {
					t.Fatalf("unexpected events: %#v", events)
				}
				if !reflect.DeepEqual(events[1]["features"], []string{}) || !reflect.DeepEqual(events[1]["variables"], []string{}) {
					t.Fatalf("unchanged input must have empty affected entities: %#v", events[1])
				}
				if diagnostics[len(diagnostics)-1].Code != "datafile_set" || !f.IsEnabled("flag") || *f.GetGlobalVariableString("colour") != "blue" {
					t.Fatal("accepted state and diagnostics must remain available")
				}
				if (f.GetFeature("retainedFlag") != nil) != (mode.name != "replace") || (f.GetGlobalVariableString("retainedColour") != nil) != (mode.name != "replace") {
					t.Fatal("acceptance must preserve merge and replacement semantics")
				}
				f.Close()
				before := len(diagnostics)
				if f.SetDatafile(input, mode.args...) || f.SetDatafile("{", mode.args...) {
					t.Fatal("closed instance must reject all inputs")
				}
				if len(diagnostics) != before || len(events) != 2 || f.GetRevision() != "accepted" {
					t.Fatal("closed setters must have no effects")
				}
			})
		}
	}
}

func TestSetDatafileRejectionPreservesStateAndEventBaseline(t *testing.T) {
	var nilPointer *DatafileContent
	for index, input := range []interface{}{"{", "null", "[]", `{}`, `{"schemaVersion":"2","revision":"bad","segments":{},"features":null}`, nil, nilPointer, 42, DatafileContent{}, map[string]interface{}{"bad": make(chan int)}} {
		for _, replace := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%t", index, replace), func(t *testing.T) {
				diagnostics := []FeaturevisorDiagnostic{}
				f := CreateFeaturevisor(FeaturevisorOptions{Datafile: acceptedDatafileJSON, OnDiagnostic: func(d FeaturevisorDiagnostic) { diagnostics = append(diagnostics, d) }})
				defer f.Close()
				diagnostics = nil
				events := []EventDetails{}
				errors := 0
				f.On(EventNameDatafileSet, func(d EventDetails) { events = append(events, d) })
				f.On(EventNameError, func(d EventDetails) {
					errors++
					if d["diagnostic"].(FeaturevisorDiagnostic).Code != "invalid_datafile" {
						t.Error("expected invalid_datafile error event")
					}
				})
				if f.SetDatafile(input, replace) {
					t.Fatal("invalid datafile must be rejected")
				}
				if len(diagnostics) != 1 || diagnostics[0].Code != "invalid_datafile" || diagnostics[0].OriginalError == nil || errors != 1 || len(events) != 0 {
					t.Fatalf("unexpected rejection diagnostics or events: %#v, %d, %#v", diagnostics, errors, events)
				}
				if f.GetRevision() != "accepted" || !f.IsEnabled("flag") || *f.GetGlobalVariableString("colour") != "blue" {
					t.Fatal("rejection must retain the last accepted state")
				}
				if !f.SetDatafile(acceptedDatafileJSON, replace) || len(events) != 1 || events[0]["previousRevision"] != "accepted" || events[0]["revisionChanged"] != false || !reflect.DeepEqual(events[0]["features"], []string{}) {
					t.Fatalf("rejection must retain the event baseline: %#v", events)
				}
			})
		}
	}
}

func TestSetDatafileResultSurvivesCallbackFailures(t *testing.T) {
	f := CreateFeaturevisor(FeaturevisorOptions{OnDiagnostic: func(FeaturevisorDiagnostic) { panic("diagnostic") }})
	defer f.Close()
	calls := 0
	f.On(EventNameDatafileSet, func(EventDetails) { panic("listener") })
	f.On(EventNameDatafileSet, func(EventDetails) { calls++ })
	f.On(EventNameError, func(EventDetails) { panic("error listener") })
	if !f.SetDatafile(acceptedDatafileJSON) || f.SetDatafile("{") || calls != 1 || f.GetRevision() != "accepted" {
		t.Fatal("callback failures must not alter acceptance")
	}
}

func TestSetDatafileResultBelongsToItsOwnCall(t *testing.T) {
	level := LogLevelFatal
	f := CreateFeaturevisor(FeaturevisorOptions{LogLevel: &level})
	f.On(EventNameDatafileSet, func(EventDetails) {
		if f.SetDatafile("{") {
			t.Error("nested rejection must return false")
		}
		f.Close()
	})
	if !f.SetDatafile(acceptedDatafileJSON) || f.SetDatafile(acceptedDatafileJSON) {
		t.Fatal("closing in a callback must not undo the accepted call")
	}

	g := CreateFeaturevisor(FeaturevisorOptions{LogLevel: &level})
	defer g.Close()
	g.On(EventNameError, func(EventDetails) {
		if !g.SetDatafile(acceptedDatafileJSON) {
			t.Error("nested recovery must be accepted")
		}
	})
	if g.SetDatafile("{") || g.GetRevision() != "accepted" {
		t.Fatal("nested recovery must not turn rejection into acceptance")
	}
}

func TestSetDatafileDiagnosticCallbackCanClose(t *testing.T) {
	var f *Featurevisor
	f = CreateFeaturevisor(FeaturevisorOptions{OnDiagnostic: func(d FeaturevisorDiagnostic) {
		if d.Code == "datafile_set" {
			f.Close()
		}
	}})
	if !f.SetDatafile(acceptedDatafileJSON) || f.SetDatafile(acceptedDatafileJSON) || f.GetRevision() != "accepted" {
		t.Fatal("closing in a diagnostic callback must not undo acceptance")
	}
}
