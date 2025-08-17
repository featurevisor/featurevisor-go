package featurevisor

import (
	"sync"
	"testing"
)

func TestEventNames(t *testing.T) {
	eventNames := []EventName{
		EventNameDatafileSet,
		EventNameContextSet,
		EventNameStickySet,
	}

	for _, eventName := range eventNames {
		t.Run(string(eventName), func(t *testing.T) {
			if eventName == "" {
				t.Error("Event name should not be empty")
			}
		})
	}
}

func TestNewEmitter(t *testing.T) {
	emitter := NewEmitter()
	if emitter == nil {
		t.Error("NewEmitter should return a non-nil emitter")
	}
	if emitter.listeners == nil {
		t.Error("Emitter listeners should be initialized")
	}
}

func TestEmitterOn(t *testing.T) {
	emitter := NewEmitter()

	// Test subscribing to an event
	callback := func(details EventDetails) {
		// Callback implementation
	}

	unsubscribe := emitter.On(EventNameDatafileSet, callback)

	// Verify listener was added
	if emitter.GetListenerCount(EventNameDatafileSet) != 1 {
		t.Errorf("Expected 1 listener, got %d", emitter.GetListenerCount(EventNameDatafileSet))
	}

	// Test unsubscribe
	unsubscribe()

	// Verify listener was removed
	if emitter.GetListenerCount(EventNameDatafileSet) != 0 {
		t.Errorf("Expected 0 listeners after unsubscribe, got %d", emitter.GetListenerCount(EventNameDatafileSet))
	}
}

func TestEmitterTrigger(t *testing.T) {
	emitter := NewEmitter()

	// Test triggering an event with no listeners
	emitter.Trigger(EventNameDatafileSet, EventDetails{"test": "value"})
	// Should not panic - this tests that triggering with no listeners doesn't cause issues

	// Test triggering an event with listeners
	receivedDetails := EventDetails{}

	callback := func(details EventDetails) {
		receivedDetails = details
	}

	emitter.On(EventNameDatafileSet, callback)

	expectedDetails := EventDetails{"test": "value", "another": "data"}
	emitter.Trigger(EventNameDatafileSet, expectedDetails)

	if len(receivedDetails) != len(expectedDetails) {
		t.Errorf("Expected %d details, got %d", len(expectedDetails), len(receivedDetails))
	}

	for key, value := range expectedDetails {
		if receivedDetails[key] != value {
			t.Errorf("Expected details[%s] = %v, got %v", key, value, receivedDetails[key])
		}
	}
}

func TestEmitterTriggerDefault(t *testing.T) {
	emitter := NewEmitter()

	callbackCalled := false
	receivedDetails := EventDetails{}

	callback := func(details EventDetails) {
		callbackCalled = true
		receivedDetails = details
	}

	emitter.On(EventNameContextSet, callback)
	emitter.TriggerDefault(EventNameContextSet)

	if !callbackCalled {
		t.Error("Callback should have been called")
	}

	if receivedDetails == nil {
		t.Error("Received details should not be nil")
	}

	if len(receivedDetails) != 0 {
		t.Errorf("Expected empty details, got %d items", len(receivedDetails))
	}
}

func TestEmitterMultipleListeners(t *testing.T) {
	emitter := NewEmitter()

	callback1Called := false
	callback2Called := false

	callback1 := func(details EventDetails) {
		callback1Called = true
	}

	callback2 := func(details EventDetails) {
		callback2Called = true
	}

	emitter.On(EventNameStickySet, callback1)
	emitter.On(EventNameStickySet, callback2)

	emitter.Trigger(EventNameStickySet, EventDetails{"test": "value"})

	if !callback1Called {
		t.Error("First callback should have been called")
	}

	if !callback2Called {
		t.Error("Second callback should have been called")
	}

	if emitter.GetListenerCount(EventNameStickySet) != 2 {
		t.Errorf("Expected 2 listeners, got %d", emitter.GetListenerCount(EventNameStickySet))
	}
}

func TestEmitterUnsubscribe(t *testing.T) {
	emitter := NewEmitter()

	callback1Called := false
	callback2Called := false

	callback1 := func(details EventDetails) {
		callback1Called = true
	}

	callback2 := func(details EventDetails) {
		callback2Called = true
	}

	unsubscribe1 := emitter.On(EventNameDatafileSet, callback1)
	emitter.On(EventNameDatafileSet, callback2)

	// Unsubscribe first callback
	unsubscribe1()

	emitter.Trigger(EventNameDatafileSet, EventDetails{"test": "value"})

	if callback1Called {
		t.Error("First callback should not have been called after unsubscribe")
	}

	if !callback2Called {
		t.Error("Second callback should have been called")
	}

	if emitter.GetListenerCount(EventNameDatafileSet) != 1 {
		t.Errorf("Expected 1 listener after unsubscribe, got %d", emitter.GetListenerCount(EventNameDatafileSet))
	}
}

func TestEmitterMultipleUnsubscribe(t *testing.T) {
	emitter := NewEmitter()

	callbackCalled := false
	callback := func(details EventDetails) {
		callbackCalled = true
	}

	unsubscribe := emitter.On(EventNameContextSet, callback)

	// Call unsubscribe multiple times
	unsubscribe()
	unsubscribe()
	unsubscribe()

	emitter.Trigger(EventNameContextSet, EventDetails{"test": "value"})

	if callbackCalled {
		t.Error("Callback should not have been called after unsubscribe")
	}
}

func TestEmitterClearAll(t *testing.T) {
	emitter := NewEmitter()

	callback1Called := false
	callback2Called := false

	callback1 := func(details EventDetails) {
		callback1Called = true
	}

	callback2 := func(details EventDetails) {
		callback2Called = true
	}

	emitter.On(EventNameDatafileSet, callback1)
	emitter.On(EventNameContextSet, callback2)

	emitter.ClearAll()

	emitter.Trigger(EventNameDatafileSet, EventDetails{"test": "value"})
	emitter.Trigger(EventNameContextSet, EventDetails{"test": "value"})

	if callback1Called {
		t.Error("First callback should not have been called after ClearAll")
	}

	if callback2Called {
		t.Error("Second callback should not have been called after ClearAll")
	}

	if emitter.GetListenerCount(EventNameDatafileSet) != 0 {
		t.Errorf("Expected 0 listeners for datafile_set after ClearAll, got %d", emitter.GetListenerCount(EventNameDatafileSet))
	}

	if emitter.GetListenerCount(EventNameContextSet) != 0 {
		t.Errorf("Expected 0 listeners for context_set after ClearAll, got %d", emitter.GetListenerCount(EventNameContextSet))
	}
}

func TestEmitterHasListeners(t *testing.T) {
	emitter := NewEmitter()

	if emitter.HasListeners(EventNameDatafileSet) {
		t.Error("Should not have listeners initially")
	}

	callback := func(details EventDetails) {}
	emitter.On(EventNameDatafileSet, callback)

	if !emitter.HasListeners(EventNameDatafileSet) {
		t.Error("Should have listeners after adding callback")
	}

	emitter.ClearAll()

	if emitter.HasListeners(EventNameDatafileSet) {
		t.Error("Should not have listeners after ClearAll")
	}
}

func TestEmitterGetEventNames(t *testing.T) {
	emitter := NewEmitter()

	// Initially no events
	eventNames := emitter.GetEventNames()
	if len(eventNames) != 0 {
		t.Errorf("Expected 0 event names initially, got %d", len(eventNames))
	}

	// Add listeners to different events
	callback := func(details EventDetails) {}
	emitter.On(EventNameDatafileSet, callback)
	emitter.On(EventNameContextSet, callback)

	eventNames = emitter.GetEventNames()
	if len(eventNames) != 2 {
		t.Errorf("Expected 2 event names, got %d", len(eventNames))
	}

	// Check that both events are present
	foundDatafile := false
	foundContext := false
	for _, eventName := range eventNames {
		if eventName == EventNameDatafileSet {
			foundDatafile = true
		}
		if eventName == EventNameContextSet {
			foundContext = true
		}
	}

	if !foundDatafile {
		t.Error("EventNameDatafileSet should be in event names")
	}

	if !foundContext {
		t.Error("EventNameContextSet should be in event names")
	}
}

func TestEmitterConcurrentAccess(t *testing.T) {
	emitter := NewEmitter()

	var wg sync.WaitGroup
	numGoroutines := 10

	// Test concurrent subscription and triggering
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			callback := func(details EventDetails) {
				// Simulate some work
				_ = details
			}

			unsubscribe := emitter.On(EventNameDatafileSet, callback)
			defer unsubscribe()

			emitter.Trigger(EventNameDatafileSet, EventDetails{"id": id})
		}(i)
	}

	wg.Wait()

	// Verify no listeners remain (all unsubscribed)
	if emitter.GetListenerCount(EventNameDatafileSet) != 0 {
		t.Errorf("Expected 0 listeners after concurrent access, got %d", emitter.GetListenerCount(EventNameDatafileSet))
	}
}

func TestEmitterPanicRecovery(t *testing.T) {
	emitter := NewEmitter()

	// Create a callback that panics
	panicCallback := func(details EventDetails) {
		panic("test panic")
	}

	// Create a normal callback
	normalCallbackCalled := false
	normalCallback := func(details EventDetails) {
		normalCallbackCalled = true
	}

	emitter.On(EventNameDatafileSet, panicCallback)
	emitter.On(EventNameDatafileSet, normalCallback)

	// Trigger should not panic and should call the normal callback
	emitter.Trigger(EventNameDatafileSet, EventDetails{"test": "value"})

	if !normalCallbackCalled {
		t.Error("Normal callback should have been called even after panic")
	}
}

// TestEmitterOriginalSpec matches the original TypeScript test specification
func TestEmitterOriginalSpec(t *testing.T) {
	emitter := NewEmitter()
	var handledDetails []EventDetails

	handleDetails := func(details EventDetails) {
		handledDetails = append(handledDetails, details)
	}

	// Test adding a listener for an event
	unsubscribe := emitter.On(EventNameDatafileSet, handleDetails)

	// Verify the listener was added to the correct event
	if emitter.GetListenerCount(EventNameDatafileSet) != 1 {
		t.Errorf("Expected 1 listener for datafile_set, got %d", emitter.GetListenerCount(EventNameDatafileSet))
	}

	// Verify other events don't have listeners
	if emitter.GetListenerCount(EventNameContextSet) != 0 {
		t.Errorf("Expected 0 listeners for context_set, got %d", emitter.GetListenerCount(EventNameContextSet))
	}

	// Trigger already subscribed event
	emitter.Trigger(EventNameDatafileSet, EventDetails{"key": "value"})
	if len(handledDetails) != 1 {
		t.Errorf("Expected 1 handled detail, got %d", len(handledDetails))
	}
	if handledDetails[0]["key"] != "value" {
		t.Errorf("Expected handled detail to contain key=value, got %v", handledDetails[0])
	}

	// Trigger unsubscribed event
	emitter.Trigger(EventNameStickySet, EventDetails{"key": "value2"})
	if len(handledDetails) != 1 {
		t.Errorf("Expected still 1 handled detail after triggering unsubscribed event, got %d", len(handledDetails))
	}

	// Unsubscribe
	unsubscribe()
	if emitter.GetListenerCount(EventNameDatafileSet) != 0 {
		t.Errorf("Expected 0 listeners after unsubscribe, got %d", emitter.GetListenerCount(EventNameDatafileSet))
	}

	// Clear all
	emitter.ClearAll()
	if emitter.GetListenerCount(EventNameDatafileSet) != 0 {
		t.Errorf("Expected 0 listeners after ClearAll, got %d", emitter.GetListenerCount(EventNameDatafileSet))
	}
	if emitter.GetListenerCount(EventNameContextSet) != 0 {
		t.Errorf("Expected 0 listeners for context_set after ClearAll, got %d", emitter.GetListenerCount(EventNameContextSet))
	}
	if emitter.GetListenerCount(EventNameStickySet) != 0 {
		t.Errorf("Expected 0 listeners for sticky_set after ClearAll, got %d", emitter.GetListenerCount(EventNameStickySet))
	}
}

func BenchmarkEmitterTrigger(b *testing.B) {
	emitter := NewEmitter()

	callback := func(details EventDetails) {
		// Simulate some work
		_ = details
	}

	emitter.On(EventNameDatafileSet, callback)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		emitter.Trigger(EventNameDatafileSet, EventDetails{"benchmark": i})
	}
}

func BenchmarkEmitterMultipleListeners(b *testing.B) {
	emitter := NewEmitter()

	// Add multiple listeners
	for i := 0; i < 10; i++ {
		callback := func(details EventDetails) {
			_ = details
		}
		emitter.On(EventNameDatafileSet, callback)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		emitter.Trigger(EventNameDatafileSet, EventDetails{"benchmark": i})
	}
}
