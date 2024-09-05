package sdk

// On adds a listener for a specific event
func (f *FeaturevisorInstance) On(eventName EventName, listener interface{}) {
	f.emitter.AddListener(eventName, listener)
}

// Off removes a listener for a specific event
func (f *FeaturevisorInstance) Off(eventName EventName, listener interface{}) {
	f.emitter.RemoveListener(eventName, listener)
}

// AddListener is an alias for On
func (f *FeaturevisorInstance) AddListener(eventName EventName, listener interface{}) {
	f.On(eventName, listener)
}

// RemoveListener is an alias for Off
func (f *FeaturevisorInstance) RemoveListener(eventName EventName, listener interface{}) {
	f.Off(eventName, listener)
}

// RemoveAllListeners removes all listeners for a specific event or all events if no event name is provided
func (f *FeaturevisorInstance) RemoveAllListeners(eventName *EventName) {
	f.emitter.RemoveAllListeners(eventName)
}
