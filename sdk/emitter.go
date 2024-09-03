package sdk

import (
	"sync"
)

type EventName string

const (
	EventReady      EventName = "ready"
	EventRefresh    EventName = "refresh"
	EventUpdate     EventName = "update"
	EventActivation EventName = "activation"
)

type Listener func(...interface{})

type Emitter struct {
	listeners map[EventName][]Listener
	mu        sync.RWMutex
}

func NewEmitter() *Emitter {
	return &Emitter{
		listeners: make(map[EventName][]Listener),
	}
}

func (e *Emitter) AddListener(eventName EventName, fn Listener) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.listeners[eventName] == nil {
		e.listeners[eventName] = []Listener{}
	}
	e.listeners[eventName] = append(e.listeners[eventName], fn)
}

func (e *Emitter) RemoveListener(eventName EventName, fn Listener) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.listeners[eventName] == nil {
		return
	}

	for i, listener := range e.listeners[eventName] {
		if &listener == &fn {
			e.listeners[eventName] = append(e.listeners[eventName][:i], e.listeners[eventName][i+1:]...)
			break
		}
	}
}

func (e *Emitter) RemoveAllListeners(eventName *EventName) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if eventName != nil {
		e.listeners[*eventName] = nil
	} else {
		for name := range e.listeners {
			e.listeners[name] = nil
		}
	}
}

func (e *Emitter) Emit(eventName EventName, args ...interface{}) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.listeners[eventName] == nil {
		return
	}

	for _, listener := range e.listeners[eventName] {
		listener(args...)
	}
}
