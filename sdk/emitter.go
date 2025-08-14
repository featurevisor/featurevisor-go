package sdk

import (
	"log"
	"sync"
)

// EventName represents the different event types
type EventName string

const (
	EventNameDatafileSet EventName = "datafile_set"
	EventNameContextSet  EventName = "context_set"
	EventNameStickySet   EventName = "sticky_set"
)

// EventDetails represents additional details for events
type EventDetails map[string]interface{}

// EventCallback is a function type for handling events
type EventCallback func(details EventDetails)

// ListenerEntry represents a listener with a unique ID
type ListenerEntry struct {
	ID       int
	Callback EventCallback
}

// Listeners represents a map of event names to their listener entries
type Listeners map[EventName][]ListenerEntry

// Unsubscribe is a function type for unsubscribing from events
type Unsubscribe func()

// Emitter provides event handling functionality
type Emitter struct {
	listeners Listeners
	nextID    int
	mu        sync.RWMutex
}

// NewEmitter creates a new emitter instance
func NewEmitter() *Emitter {
	return &Emitter{
		listeners: make(Listeners),
		nextID:    1,
	}
}

// On subscribes to an event with a callback function
// Returns an unsubscribe function that can be called to remove the listener
func (e *Emitter) On(eventName EventName, callback EventCallback) Unsubscribe {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.listeners[eventName] == nil {
		e.listeners[eventName] = make([]ListenerEntry, 0)
	}

	entry := ListenerEntry{
		ID:       e.nextID,
		Callback: callback,
	}
	e.nextID++

	listeners := e.listeners[eventName]
	listeners = append(listeners, entry)
	e.listeners[eventName] = listeners

	isActive := true
	listenerID := entry.ID

	return func() {
		if !isActive {
			return
		}

		isActive = false

		e.mu.Lock()
		defer e.mu.Unlock()

		// Find and remove the callback from the listeners slice
		currentListeners := e.listeners[eventName]
		for i, listener := range currentListeners {
			if listener.ID == listenerID {
				// Remove the callback by slicing
				if i < len(currentListeners)-1 {
					e.listeners[eventName] = append(currentListeners[:i], currentListeners[i+1:]...)
				} else {
					e.listeners[eventName] = currentListeners[:i]
				}
				break
			}
		}
	}
}

// Trigger fires an event with the given details
func (e *Emitter) Trigger(eventName EventName, details EventDetails) {
	if details == nil {
		details = make(EventDetails)
	}

	e.mu.RLock()
	listeners := e.listeners[eventName]
	if listeners == nil {
		e.mu.RUnlock()
		return
	}

	// Create a copy of the listeners slice to avoid issues if callbacks modify the slice
	listenersCopy := make([]ListenerEntry, len(listeners))
	copy(listenersCopy, listeners)
	e.mu.RUnlock()

	for _, listener := range listenersCopy {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Error in event listener for %s: %v", eventName, r)
				}
			}()
			listener.Callback(details)
		}()
	}
}

// TriggerDefault fires an event with empty details
func (e *Emitter) TriggerDefault(eventName EventName) {
	e.Trigger(eventName, make(EventDetails))
}

// ClearAll removes all event listeners
func (e *Emitter) ClearAll() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners = make(Listeners)
}

// GetListenerCount returns the number of listeners for a specific event
func (e *Emitter) GetListenerCount(eventName EventName) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	listeners := e.listeners[eventName]
	if listeners == nil {
		return 0
	}
	return len(listeners)
}

// HasListeners returns true if there are any listeners for the given event
func (e *Emitter) HasListeners(eventName EventName) bool {
	return e.GetListenerCount(eventName) > 0
}

// GetEventNames returns all event names that have listeners
func (e *Emitter) GetEventNames() []EventName {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var eventNames []EventName
	for eventName := range e.listeners {
		if e.HasListeners(eventName) {
			eventNames = append(eventNames, eventName)
		}
	}
	return eventNames
}
