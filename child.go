package featurevisor

import "fmt"

// childOptions contains options for creating a child instance
type childOptions struct {
	Parent          *Featurevisor
	Context         Context
	Sticky          *StickyFeatures
	StickyVariables *StickyVariables
}

type childParentSubscription struct {
	id          uint64
	unsubscribe Unsubscribe
}

// FeaturevisorChild represents a child Featurevisor instance
type FeaturevisorChild struct {
	parent              *Featurevisor
	context             Context
	sticky              *StickyFeatures
	stickyVariables     *StickyVariables
	emitter             *emitter
	parentSubscriptions []childParentSubscription
	nextSubscriptionID  uint64
}

// newFeaturevisorChild creates a new child instance.
func newFeaturevisorChild(options childOptions) *FeaturevisorChild {
	return &FeaturevisorChild{
		parent:          options.Parent,
		context:         options.Context,
		sticky:          options.Sticky,
		stickyVariables: options.StickyVariables,
		emitter:         newEmitter(),
	}
}

// On adds an event listener
func (c *FeaturevisorChild) On(eventName EventName, callback EventCallback) Unsubscribe {
	if eventName == EventNameContextSet || eventName == EventNameStickyFeaturesSet || eventName == EventNameStickyVariablesSet {
		return c.emitter.On(eventName, callback)
	}

	parentUnsubscribe := c.parent.On(eventName, callback)
	active := true
	c.nextSubscriptionID++
	subscriptionID := c.nextSubscriptionID
	var unsubscribe Unsubscribe
	unsubscribe = func() {
		if !active {
			return
		}
		active = false
		parentUnsubscribe()
		for index, subscription := range c.parentSubscriptions {
			if subscription.id == subscriptionID {
				c.parentSubscriptions = append(c.parentSubscriptions[:index], c.parentSubscriptions[index+1:]...)
				break
			}
		}
	}
	c.parentSubscriptions = append(c.parentSubscriptions, childParentSubscription{
		id:          subscriptionID,
		unsubscribe: unsubscribe,
	})
	return unsubscribe
}

// SetStickyFeatures sets sticky feature evaluations on the child.
func (c *FeaturevisorChild) SetStickyFeatures(sticky StickyFeatures, replace ...bool) {
	replaceValue := len(replace) > 0 && replace[0]
	previousStickyFeatures := StickyFeatures{}
	if c.sticky != nil {
		previousStickyFeatures = *c.sticky
	}
	if replaceValue {
		c.sticky = &sticky
	} else {
		newSticky := StickyFeatures{}
		if c.sticky != nil {
			for key, value := range *c.sticky {
				newSticky[key] = value
			}
		}
		for key, value := range sticky {
			newSticky[key] = value
		}
		c.sticky = &newSticky
	}
	c.emitter.Trigger(EventNameStickyFeaturesSet, EventDetails(getParamsForStickyFeaturesSetEvent(previousStickyFeatures, *c.sticky, replaceValue)))
}

// SetStickyVariables sets sticky global variable values on the child.
func (c *FeaturevisorChild) SetStickyVariables(sticky StickyVariables, replace ...bool) {
	replaceValue := len(replace) > 0 && replace[0]
	next := StickyVariables{}
	if !replaceValue && c.stickyVariables != nil {
		for key, value := range *c.stickyVariables {
			next[key] = value
		}
	}
	for key, value := range sticky {
		next[key] = value
	}
	c.stickyVariables = &next
	c.emitter.Trigger(EventNameStickyVariablesSet, EventDetails{"variables": mapKeys(next), "replaced": replaceValue})
}

// Close closes child instance listeners
func (c *FeaturevisorChild) Close() {
	for _, subscription := range append([]childParentSubscription{}, c.parentSubscriptions...) {
		subscription.unsubscribe()
	}
	c.parentSubscriptions = nil
	c.emitter.ClearAll()
}

// SetContext sets the context
func (c *FeaturevisorChild) SetContext(context Context, replace ...bool) {
	replaceValue := false
	if len(replace) > 0 {
		replaceValue = replace[0]
	}

	if replaceValue {
		c.context = context
	} else {
		// Merge context
		for key, value := range context {
			c.context[key] = value
		}
	}

	c.emitter.Trigger(EventNameContextSet, EventDetails{
		"context":  c.context,
		"replaced": replaceValue,
	})
}

// GetContext returns the context
func (c *FeaturevisorChild) GetContext(context Context) Context {
	merged := Context{}
	for key, value := range c.context {
		merged[key] = value
	}
	for key, value := range context {
		merged[key] = value
	}

	return c.parent.GetContext(merged)
}

// getEvaluationDependencies gets evaluation dependencies
func (c *FeaturevisorChild) getEvaluationDependencies(context Context, options OverrideOptions) evaluateDependencies {
	var sticky *StickyFeatures
	if options.sticky != nil {
		sticky = options.sticky
	} else {
		sticky = c.sticky
	}

	return evaluateDependencies{
		Context:                        c.GetContext(context),
		diagnosticReporter:             c.parent.diagnostics,
		modulesManager:                 c.parent.modulesManager,
		instanceEvaluationDataProvider: c.parent.instanceEvaluationDataProvider,
		sticky:                         sticky,
		stickyVariables: func() *StickyVariables {
			if options.stickyVariables != nil {
				return options.stickyVariables
			}
			return c.stickyVariables
		}(),
		DefaultVariationValue:   options.DefaultVariationValue,
		DefaultVariableValue:    options.DefaultVariableValue,
		DefaultVariableValueSet: options.DefaultVariableValueSet,
	}
}

func mapKeys(values StickyVariables) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

// EvaluateGlobalVariable evaluates an independently defined variable on the child.
func (c *FeaturevisorChild) EvaluateGlobalVariable(variableKey string, args ...interface{}) Evaluation {
	context, options := parseEvaluationArgs(args)
	key := VariableKey(variableKey)
	return evaluateWithModules(EvaluateOptions{evaluateParams: evaluateParams{Type: EvaluationTypeVariable, VariableKey: &key, GlobalVariable: true}, evaluateDependencies: c.getEvaluationDependencies(context, options)})
}

// GetGlobalVariable gets an independently defined variable on the child.
func (c *FeaturevisorChild) GetGlobalVariable(variableKey string, args ...interface{}) VariableValue {
	return getGlobalVariableValue(c.EvaluateGlobalVariable(variableKey, args...))
}

func (c *FeaturevisorChild) GetGlobalVariableBoolean(variableKey string, args ...interface{}) *bool {
	value, ok := c.GetGlobalVariable(variableKey, args...).(bool)
	if !ok {
		return nil
	}
	return &value
}
func (c *FeaturevisorChild) GetGlobalVariableString(variableKey string, args ...interface{}) *string {
	value, ok := c.GetGlobalVariable(variableKey, args...).(string)
	if !ok {
		return nil
	}
	return &value
}
func (c *FeaturevisorChild) GetGlobalVariableInteger(variableKey string, args ...interface{}) *int {
	value := c.GetGlobalVariable(variableKey, args...)
	if number, ok := value.(float64); ok {
		result := int(number)
		return &result
	}
	if number, ok := value.(int); ok {
		return &number
	}
	return nil
}
func (c *FeaturevisorChild) GetGlobalVariableDouble(variableKey string, args ...interface{}) *float64 {
	value, ok := c.GetGlobalVariable(variableKey, args...).(float64)
	if !ok {
		return nil
	}
	return &value
}
func (c *FeaturevisorChild) GetGlobalVariableArray(variableKey string, args ...interface{}) []string {
	value := c.GetGlobalVariable(variableKey, args...)
	if result, ok := value.([]string); ok {
		return result
	}
	if values, ok := value.([]interface{}); ok {
		result := make([]string, 0, len(values))
		for _, item := range values {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	}
	return nil
}
func (c *FeaturevisorChild) GetGlobalVariableObject(variableKey string, args ...interface{}) map[string]interface{} {
	value, _ := c.GetGlobalVariable(variableKey, args...).(map[string]interface{})
	return value
}
func (c *FeaturevisorChild) GetGlobalVariableJSON(variableKey string, args ...interface{}) interface{} {
	return c.GetGlobalVariable(variableKey, args...)
}

// GetGlobalVariableArrayInto decodes an array global variable into out.
func (c *FeaturevisorChild) GetGlobalVariableArrayInto(variableKey string, args ...interface{}) error {
	context, options, out, err := parseVariableIntoArgs(args...)
	if err != nil {
		return err
	}
	value := c.GetGlobalVariable(variableKey, context, options)
	if value == nil {
		return fmt.Errorf("global variable %q is unavailable", variableKey)
	}
	return decodeInto(value, out)
}

// GetGlobalVariableObjectInto decodes an object global variable into out.
func (c *FeaturevisorChild) GetGlobalVariableObjectInto(variableKey string, args ...interface{}) error {
	return c.GetGlobalVariableArrayInto(variableKey, args...)
}

// GetVariableEvaluations evaluates a global variable snapshot on the child.
func (c *FeaturevisorChild) GetVariableEvaluations(context Context, variableKeys []string, options OverrideOptions) EvaluatedVariables {
	result := EvaluatedVariables{}
	keys := variableKeys
	if len(keys) == 0 {
		keys = c.parent.GetGlobalVariableKeys()
	}
	for _, key := range keys {
		result[key] = c.GetGlobalVariable(key, context, options)
	}
	return result
}

func (c *FeaturevisorChild) evaluateFlag(featureKey string, context Context, options OverrideOptions) Evaluation {
	return evaluateWithModules(EvaluateOptions{
		evaluateParams: evaluateParams{
			Type:       EvaluationTypeFlag,
			FeatureKey: FeatureKey(featureKey),
		},
		evaluateDependencies: c.getEvaluationDependencies(context, options),
	})
}

// EvaluateFlag evaluates a feature flag and returns its full evaluation details.
func (c *FeaturevisorChild) EvaluateFlag(featureKey string, args ...interface{}) Evaluation {
	contextValue := Context{}
	optionsValue := OverrideOptions{}
	for _, arg := range args {
		switch v := arg.(type) {
		case Context:
			contextValue = v
		case OverrideOptions:
			optionsValue = v
		}
	}

	return c.evaluateFlag(featureKey, contextValue, optionsValue)
}

// IsEnabled checks if a feature is enabled
func (c *FeaturevisorChild) IsEnabled(featureKey string, args ...interface{}) bool {
	defer func() {
		if r := recover(); r != nil {
			c.parent.diagnostics.Error("isEnabled", logDetails{
				"featureKey": featureKey,
				"error":      r,
			})
		}
	}()

	// Default values
	contextValue := Context{}
	optionsValue := OverrideOptions{}

	// Parse variadic arguments
	for _, arg := range args {
		switch v := arg.(type) {
		case Context:
			contextValue = v
		case OverrideOptions:
			optionsValue = v
		}
	}

	evaluation := c.evaluateFlag(featureKey, contextValue, optionsValue)

	if evaluation.Enabled != nil {
		return *evaluation.Enabled
	}

	return false
}

func (c *FeaturevisorChild) evaluateVariation(featureKey string, context Context, options OverrideOptions) Evaluation {
	return evaluateWithModules(EvaluateOptions{
		evaluateParams: evaluateParams{
			Type:       EvaluationTypeVariation,
			FeatureKey: FeatureKey(featureKey),
		},
		evaluateDependencies: c.getEvaluationDependencies(context, options),
	})
}

// EvaluateVariation evaluates a variation and returns its full evaluation details.
func (c *FeaturevisorChild) EvaluateVariation(featureKey string, args ...interface{}) Evaluation {
	contextValue := Context{}
	optionsValue := OverrideOptions{}
	for _, arg := range args {
		switch v := arg.(type) {
		case Context:
			contextValue = v
		case OverrideOptions:
			optionsValue = v
		}
	}

	return c.evaluateVariation(featureKey, contextValue, optionsValue)
}

// GetVariation gets a feature variation
func (c *FeaturevisorChild) GetVariation(featureKey string, args ...interface{}) *string {
	defer func() {
		if r := recover(); r != nil {
			c.parent.diagnostics.Error("getVariation", logDetails{
				"featureKey": featureKey,
				"error":      r,
			})
		}
	}()

	// Default values
	contextValue := Context{}
	optionsValue := OverrideOptions{}

	// Parse variadic arguments
	for _, arg := range args {
		switch v := arg.(type) {
		case Context:
			contextValue = v
		case OverrideOptions:
			optionsValue = v
		}
	}

	evaluation := c.evaluateVariation(featureKey, contextValue, optionsValue)

	if evaluation.VariationValue != nil {
		// VariationValue is already a string type alias
		variationValue := string(*evaluation.VariationValue)
		return &variationValue
	}

	if evaluation.Variation != nil {
		// Variation.Value is already a VariationValue (string)
		variationValue := string(evaluation.Variation.Value)
		return &variationValue
	}

	return nil
}

func (c *FeaturevisorChild) evaluateVariable(featureKey string, variableKey VariableKey, context Context, options OverrideOptions) Evaluation {
	return evaluateWithModules(EvaluateOptions{
		evaluateParams: evaluateParams{
			Type:        EvaluationTypeVariable,
			FeatureKey:  FeatureKey(featureKey),
			VariableKey: &variableKey,
		},
		evaluateDependencies: c.getEvaluationDependencies(context, options),
	})
}

// EvaluateVariable evaluates a variable and returns its full evaluation details.
func (c *FeaturevisorChild) EvaluateVariable(featureKey string, variableKey string, args ...interface{}) Evaluation {
	contextValue := Context{}
	optionsValue := OverrideOptions{}
	for _, arg := range args {
		switch v := arg.(type) {
		case Context:
			contextValue = v
		case OverrideOptions:
			optionsValue = v
		}
	}

	return c.evaluateVariable(featureKey, VariableKey(variableKey), contextValue, optionsValue)
}

// GetVariable gets a feature variable
func (c *FeaturevisorChild) GetVariable(featureKey string, variableKey string, args ...interface{}) VariableValue {
	defer func() {
		if r := recover(); r != nil {
			c.parent.diagnostics.Error("getVariable", logDetails{
				"featureKey":  featureKey,
				"variableKey": variableKey,
				"error":       r,
			})
		}
	}()

	// Default values
	contextValue := Context{}
	optionsValue := OverrideOptions{}

	// Parse variadic arguments
	for _, arg := range args {
		switch v := arg.(type) {
		case Context:
			contextValue = v
		case OverrideOptions:
			optionsValue = v
		}
	}

	evaluation := c.evaluateVariable(featureKey, VariableKey(variableKey), contextValue, optionsValue)

	if evaluation.VariableValue != nil {
		return evaluation.VariableValue
	}

	return nil
}

// GetVariableBoolean gets a boolean variable
func (c *FeaturevisorChild) GetVariableBoolean(featureKey string, variableKey string, args ...interface{}) *bool {
	value := c.GetVariable(featureKey, variableKey, args...)
	if value == nil {
		return nil
	}

	typedValue := getValueByType(value, "boolean")
	if boolValue, ok := typedValue.(bool); ok {
		return &boolValue
	}

	return nil
}

// GetVariableString gets a string variable
func (c *FeaturevisorChild) GetVariableString(featureKey string, variableKey string, args ...interface{}) *string {
	value := c.GetVariable(featureKey, variableKey, args...)
	if value == nil {
		return nil
	}

	typedValue := getValueByType(value, "string")
	if stringValue, ok := typedValue.(string); ok {
		return &stringValue
	}

	return nil
}

// GetVariableInteger gets an integer variable
func (c *FeaturevisorChild) GetVariableInteger(featureKey string, variableKey string, args ...interface{}) *int {
	value := c.GetVariable(featureKey, variableKey, args...)
	if value == nil {
		return nil
	}

	typedValue := getValueByType(value, "integer")
	if intValue, ok := typedValue.(int); ok {
		return &intValue
	}

	return nil
}

// GetVariableDouble gets a double variable
func (c *FeaturevisorChild) GetVariableDouble(featureKey string, variableKey string, args ...interface{}) *float64 {
	value := c.GetVariable(featureKey, variableKey, args...)
	if value == nil {
		return nil
	}

	typedValue := getValueByType(value, "double")
	if floatValue, ok := typedValue.(float64); ok {
		return &floatValue
	}

	return nil
}

// GetVariableArray gets an array variable
func (c *FeaturevisorChild) GetVariableArray(featureKey string, variableKey string, args ...interface{}) []string {
	value := c.GetVariable(featureKey, variableKey, args...)
	if value == nil {
		return nil
	}

	return toTypedArray[string](getValueByType(value, "array"))
}

// GetVariableObject gets an object variable
func (c *FeaturevisorChild) GetVariableObject(featureKey string, variableKey string, args ...interface{}) map[string]interface{} {
	value := c.GetVariable(featureKey, variableKey, args...)
	if value == nil {
		return nil
	}

	typedValue := toTypedObject[map[string]interface{}](getValueByType(value, "object"))
	if typedValue == nil {
		return nil
	}

	return *typedValue
}

// GetVariableJSON gets a JSON variable
func (c *FeaturevisorChild) GetVariableJSON(featureKey string, variableKey string, args ...interface{}) interface{} {
	value := c.GetVariable(featureKey, variableKey, args...)
	if value == nil {
		return nil
	}

	return value
}

// GetVariableArrayInto decodes an array variable into the provided pointer output.
// Supported argument order (after featureKey, variableKey): out OR context, out OR context, options, out.
func (c *FeaturevisorChild) GetVariableArrayInto(featureKey string, variableKey string, args ...interface{}) error {
	context, options, out, err := parseVariableIntoArgs(args...)
	if err != nil {
		return err
	}

	value := c.GetVariable(featureKey, variableKey, context, options)
	if value == nil {
		return decodeInto(nil, out)
	}

	arrayValue := getValueByType(value, "array")
	if arrayValue == nil {
		return fmt.Errorf("variable %q is not an array", variableKey)
	}

	return decodeInto(arrayValue, out)
}

// GetVariableObjectInto decodes an object variable into the provided pointer output.
// Supported argument order (after featureKey, variableKey): out OR context, out OR context, options, out.
func (c *FeaturevisorChild) GetVariableObjectInto(featureKey string, variableKey string, args ...interface{}) error {
	context, options, out, err := parseVariableIntoArgs(args...)
	if err != nil {
		return err
	}

	value := c.GetVariable(featureKey, variableKey, context, options)
	if value == nil {
		return decodeInto(nil, out)
	}

	objectValue := getValueByType(value, "object")
	if objectValue == nil {
		return fmt.Errorf("variable %q is not an object", variableKey)
	}

	return decodeInto(objectValue, out)
}

// GetFeatureEvaluations evaluates a feature snapshot on the child.
func (c *FeaturevisorChild) GetFeatureEvaluations(context Context, featureKeys []string, options OverrideOptions) EvaluatedFeatures {
	result := EvaluatedFeatures{}

	keys := featureKeys
	if len(keys) == 0 {
		// Get all feature keys from parent
		allKeys := c.parent.instanceEvaluationDataProvider.GetFeatureKeys()
		keys = make([]string, len(allKeys))
		for j, key := range allKeys {
			keys[j] = string(key)
		}
	}

	for _, featureKey := range keys {
		// isEnabled
		evaluatedFeature := EvaluatedFeature{
			Enabled: c.IsEnabled(featureKey, context, options),
		}

		// variation
		if c.parent.instanceEvaluationDataProvider.HasVariations(FeatureKey(featureKey)) {
			variation := c.GetVariation(featureKey, context, options)
			if variation != nil {
				evaluatedFeature.Variation = variation
			}
		}

		// variables
		variableKeys := c.parent.instanceEvaluationDataProvider.GetVariableKeys(FeatureKey(featureKey))
		if len(variableKeys) > 0 {
			evaluatedFeature.Variables = make(map[VariableKey]VariableValue)
			for _, variableKey := range variableKeys {
				evaluatedFeature.Variables[variableKey] = c.GetVariable(
					featureKey,
					string(variableKey),
					context,
					options,
				)
			}
		}

		result[FeatureKey(featureKey)] = evaluatedFeature
	}

	return result
}
