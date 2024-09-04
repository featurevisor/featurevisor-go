package sdk

import (
	"reflect"
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
	listeners map[EventName][]interface{}
	mu        sync.RWMutex
}

func NewEmitter() *Emitter {
	return &Emitter{
		listeners: make(map[EventName][]interface{}),
	}
}

func (e *Emitter) AddListener(eventName EventName, listener interface{}) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.listeners[eventName] == nil {
		e.listeners[eventName] = []interface{}{}
	}
	e.listeners[eventName] = append(e.listeners[eventName], listener)
}

func (e *Emitter) RemoveListener(eventName EventName, listener interface{}) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.listeners[eventName] == nil {
		return
	}

	for i, l := range e.listeners[eventName] {
		if reflect.ValueOf(l).Pointer() == reflect.ValueOf(listener).Pointer() {
			e.listeners[eventName] = append(e.listeners[eventName][:i], e.listeners[eventName][i+1:]...)
			break
		}
	}
}

func (e *Emitter) RemoveAllListeners(eventName *EventName) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if eventName != nil {
		delete(e.listeners, *eventName)
	} else {
		e.listeners = make(map[EventName][]interface{})
	}
}

func (e *Emitter) Emit(eventName EventName, args ...interface{}) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.listeners[eventName] == nil {
		return
	}

	for _, listener := range e.listeners[eventName] {
		reflect.ValueOf(listener).Call(makeArgs(args))
	}
}

func makeArgs(args []interface{}) []reflect.Value {
	var reflectArgs []reflect.Value
	for _, arg := range args {
		reflectArgs = append(reflectArgs, reflect.ValueOf(arg))
	}
	return reflectArgs
}
