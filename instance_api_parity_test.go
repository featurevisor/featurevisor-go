package featurevisor

import "testing"

func TestSetDatafileAcceptsJSONString(t *testing.T) {
	instance := CreateInstance(Options{})
	instance.SetDatafile(`{
		"schemaVersion": "2",
		"revision": "json-revision",
		"segments": {},
		"features": {}
	}`)

	if revision := instance.GetRevision(); revision != "json-revision" {
		t.Fatalf("expected revision from json string datafile, got %s", revision)
	}
}

func TestInstanceOnReturnsUnsubscribe(t *testing.T) {
	instance := CreateInstance(Options{})
	calls := 0

	unsubscribe := instance.On(EventNameContextSet, func(details EventDetails) {
		calls++
	})
	instance.SetContext(Context{"a": 1})
	if calls != 1 {
		t.Fatalf("expected listener call count 1, got %d", calls)
	}

	unsubscribe()
	instance.SetContext(Context{"b": 2})
	if calls != 1 {
		t.Fatalf("expected listener to be removed after unsubscribe, got %d", calls)
	}
}
