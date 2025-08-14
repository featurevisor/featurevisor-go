package sdk

// ConfigureBucketKeyOptions contains options for configuring bucket key
type ConfigureBucketKeyOptions struct {
	FeatureKey FeatureKey `json:"featureKey"`
	Context    Context    `json:"context"`
	BucketBy   BucketBy   `json:"bucketBy"`
	BucketKey  string     `json:"bucketKey"` // the initial bucket key, which can be modified by hooks
}

// ConfigureBucketKey is a function type for configuring bucket key
type ConfigureBucketKey func(options ConfigureBucketKeyOptions) BucketKey

// ConfigureBucketValueOptions contains options for configuring bucket value
type ConfigureBucketValueOptions struct {
	FeatureKey  FeatureKey `json:"featureKey"`
	BucketKey   string     `json:"bucketKey"`
	Context     Context    `json:"context"`
	BucketValue int        `json:"bucketValue"` // the initial bucket value, which can be modified by hooks
}

// ConfigureBucketValue is a function type for configuring bucket value
type ConfigureBucketValue func(options ConfigureBucketValueOptions) BucketValue

// Hook represents a hook that can be executed during evaluation
type Hook struct {
	Name string `json:"name"`

	Before      func(options EvaluateOptions) EvaluateOptions                   `json:"before,omitempty"`
	BucketKey   ConfigureBucketKey                                              `json:"bucketKey,omitempty"`
	BucketValue ConfigureBucketValue                                            `json:"bucketValue,omitempty"`
	After       func(evaluation Evaluation, options EvaluateOptions) Evaluation `json:"after,omitempty"`
}

// HooksManagerOptions contains options for creating a hooks manager
type HooksManagerOptions struct {
	Hooks  []*Hook `json:"hooks,omitempty"`
	Logger *Logger `json:"logger"`
}

// HooksManager manages hooks for evaluation
type HooksManager struct {
	hooks  []*Hook
	logger *Logger
}

// NewHooksManager creates a new hooks manager instance
func NewHooksManager(options HooksManagerOptions) *HooksManager {
	hm := &HooksManager{
		hooks:  make([]*Hook, 0),
		logger: options.Logger,
	}

	if options.Hooks != nil {
		for _, hook := range options.Hooks {
			hm.Add(hook)
		}
	}

	return hm
}

// Add adds a hook to the hooks manager
func (hm *HooksManager) Add(hook *Hook) func() {
	// Check if hook with same name already exists
	for _, existingHook := range hm.hooks {
		if existingHook.Name == hook.Name {
			hm.logger.Error("Hook with name already exists", LogDetails{
				"name": hook.Name,
				"hook": hook,
			})
			return nil
		}
	}

	hm.hooks = append(hm.hooks, hook)

	// Return a function to remove the hook
	return func() {
		hm.Remove(hook.Name)
	}
}

// Remove removes a hook by name
func (hm *HooksManager) Remove(name string) {
	newHooks := make([]*Hook, 0)
	for _, hook := range hm.hooks {
		if hook.Name != name {
			newHooks = append(newHooks, hook)
		}
	}
	hm.hooks = newHooks
}

// GetAll returns all hooks
func (hm *HooksManager) GetAll() []*Hook {
	return hm.hooks
}
