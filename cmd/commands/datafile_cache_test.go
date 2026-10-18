package commands

import (
	"testing"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

func TestTypedDatafileCacheSupportsIsolatedAssertionInstances(t *testing.T) {
	var content featurevisor.DatafileContent = segmentTestContent(t, `{"schemaVersion":"4","revision":"cached","segments":{},"features":{},"variables":{"source":{"type":"object","defaultValue":{"limit":5}},"alias":{"type":"object","compose":"source"}}}`)
	for _, sticky := range []bool{true, false} {
		assertion := map[string]interface{}{}
		if sticky {
			assertion["stickyVariables"] = map[string]interface{}{"source": map[string]interface{}{"limit": float64(9)}}
		}
		instance, _, err := buildInstanceForReport(content, "fatal", assertion, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := float64(5)
		if sticky {
			want = 9
		}
		value := instance.GetGlobalVariable("alias").(map[string]interface{})
		if value["limit"] != want {
			t.Fatalf("sticky=%v: got %v", sticky, value)
		}
		instance.Close()
	}
	if content.Variables["source"].DefaultValue.(map[string]interface{})["limit"] != float64(5) {
		t.Fatal("an assertion changed the shared generated datafile")
	}
	if got := datafileContentFromRaw(content); got.Revision != content.Revision {
		t.Fatal("typed datafile was not preserved")
	}
	// Raw map inputs remain accepted by private test helpers.
	raw := map[string]interface{}{"schemaVersion": "4", "revision": "map", "segments": map[string]interface{}{}, "features": map[string]interface{}{}}
	instance, _, err := buildInstanceForReport(raw, "fatal", map[string]interface{}{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	if instance.GetRevision() != "map" {
		t.Fatal("raw map input was not decoded")
	}
}
