package featurevisor

import "testing"

func TestChildEventIsolationAndProxying(t *testing.T) {
	instance := CreateInstance(Options{
		Datafile: DatafileContent{
			SchemaVersion: "2",
			Revision:      "1",
			Features:      map[FeatureKey]Feature{},
			Segments:      map[SegmentKey]Segment{},
		},
	})
	child := instance.Spawn()

	childContextEvents := 0
	parentContextEvents := 0
	childDatafileEvents := 0

	childUnsubscribe := child.On(EventNameContextSet, func(details EventDetails) {
		childContextEvents++
	})
	defer childUnsubscribe()

	parentUnsubscribe := instance.On(EventNameContextSet, func(details EventDetails) {
		parentContextEvents++
	})
	defer parentUnsubscribe()

	datafileUnsubscribe := child.On(EventNameDatafileSet, func(details EventDetails) {
		childDatafileEvents++
	})
	defer datafileUnsubscribe()

	child.SetContext(Context{"userId": "123"})
	if childContextEvents != 1 {
		t.Fatalf("expected child context_set listener to be called once, got %d", childContextEvents)
	}
	if parentContextEvents != 0 {
		t.Fatalf("expected parent context_set listener not to be called by child.setContext, got %d", parentContextEvents)
	}

	instance.SetDatafile(DatafileContent{
		SchemaVersion: "2",
		Revision:      "2",
		Features:      map[FeatureKey]Feature{},
		Segments:      map[SegmentKey]Segment{},
	})
	if childDatafileEvents != 1 {
		t.Fatalf("expected child datafile_set listener to proxy parent event, got %d", childDatafileEvents)
	}

	child.Close()
	child.SetContext(Context{"country": "nl"})
	if childContextEvents != 1 {
		t.Fatalf("expected child listeners to be cleared after close, got %d", childContextEvents)
	}
}
