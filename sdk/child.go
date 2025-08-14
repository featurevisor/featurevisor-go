package sdk

// ChildInstanceOptions contains options for creating a child instance
type ChildInstanceOptions struct {
	Parent  *Featurevisor
	Context Context
	Sticky  *StickyFeatures
}

// FeaturevisorChild represents a child Featurevisor instance
type FeaturevisorChild struct {
	parent  *Featurevisor
	context Context
	sticky  *StickyFeatures
}

// NewFeaturevisorChild creates a new child instance
func NewFeaturevisorChild(options ChildInstanceOptions) *FeaturevisorChild {
	return &FeaturevisorChild{
		parent:  options.Parent,
		context: options.Context,
		sticky:  options.Sticky,
	}
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
}

// GetContext returns the context
func (c *FeaturevisorChild) GetContext(context Context) Context {
	if context == nil {
		return c.context
	}

	// Merge contexts
	result := Context{}
	for key, value := range c.context {
		result[key] = value
	}
	for key, value := range context {
		result[key] = value
	}

	return result
}

// SetSticky sets sticky features
func (c *FeaturevisorChild) SetSticky(sticky StickyFeatures, replace ...bool) {
	replaceValue := false
	if len(replace) > 0 {
		replaceValue = replace[0]
	}

	previousStickyFeatures := StickyFeatures{}
	if c.sticky != nil {
		previousStickyFeatures = *c.sticky
	}

	if replaceValue {
		c.sticky = &sticky
	} else {
		newSticky := StickyFeatures{}
		if c.sticky != nil {
			newSticky = *c.sticky
		}
		// Merge sticky features
		for key, value := range sticky {
			newSticky[key] = value
		}
		c.sticky = &newSticky
	}

	params := getParamsForStickySetEvent(previousStickyFeatures, *c.sticky, replaceValue)

	c.parent.logger.Info("sticky features set", params)
	c.parent.emitter.Trigger(EventNameStickySet, EventDetails(params))
}

// getEvaluationDependencies gets evaluation dependencies
func (c *FeaturevisorChild) getEvaluationDependencies(context Context, options OverrideOptions) EvaluateDependencies {
	var sticky *StickyFeatures
	if options.Sticky != nil {
		if c.sticky != nil {
			// Merge sticky features
			mergedSticky := StickyFeatures{}
			for key, value := range *c.sticky {
				mergedSticky[key] = value
			}
			for key, value := range *options.Sticky {
				mergedSticky[key] = value
			}
			sticky = &mergedSticky
		} else {
			sticky = options.Sticky
		}
	} else {
		sticky = c.sticky
	}

	return EvaluateDependencies{
		Context:               c.GetContext(context),
		Logger:                c.parent.logger,
		HooksManager:          c.parent.hooksManager,
		DatafileReader:        c.parent.datafileReader,
		Sticky:                sticky,
		DefaultVariationValue: options.DefaultVariationValue,
		DefaultVariableValue:  options.DefaultVariableValue,
	}
}

// EvaluateFlag evaluates a feature flag
func (c *FeaturevisorChild) EvaluateFlag(featureKey string, context Context, options OverrideOptions) Evaluation {
	return EvaluateWithHooks(EvaluateOptions{
		EvaluateParams: EvaluateParams{
			Type:       EvaluationTypeFlag,
			FeatureKey: FeatureKey(featureKey),
		},
		EvaluateDependencies: c.getEvaluationDependencies(context, options),
	})
}

// IsEnabled checks if a feature is enabled
func (c *FeaturevisorChild) IsEnabled(featureKey string, args ...interface{}) bool {
	defer func() {
		if r := recover(); r != nil {
			c.parent.logger.Error("isEnabled", LogDetails{
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

	evaluation := c.EvaluateFlag(featureKey, contextValue, optionsValue)

	if evaluation.Enabled != nil {
		return *evaluation.Enabled
	}

	return false
}

// EvaluateVariation evaluates a feature variation
func (c *FeaturevisorChild) EvaluateVariation(featureKey string, context Context, options OverrideOptions) Evaluation {
	return EvaluateWithHooks(EvaluateOptions{
		EvaluateParams: EvaluateParams{
			Type:       EvaluationTypeVariation,
			FeatureKey: FeatureKey(featureKey),
		},
		EvaluateDependencies: c.getEvaluationDependencies(context, options),
	})
}

// GetVariation gets a feature variation
func (c *FeaturevisorChild) GetVariation(featureKey string, args ...interface{}) *string {
	defer func() {
		if r := recover(); r != nil {
			c.parent.logger.Error("getVariation", LogDetails{
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

	evaluation := c.EvaluateVariation(featureKey, contextValue, optionsValue)

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

// EvaluateVariable evaluates a feature variable
func (c *FeaturevisorChild) EvaluateVariable(featureKey string, variableKey VariableKey, context Context, options OverrideOptions) Evaluation {
	return EvaluateWithHooks(EvaluateOptions{
		EvaluateParams: EvaluateParams{
			Type:        EvaluationTypeVariable,
			FeatureKey:  FeatureKey(featureKey),
			VariableKey: &variableKey,
		},
		EvaluateDependencies: c.getEvaluationDependencies(context, options),
	})
}

// GetVariable gets a feature variable
func (c *FeaturevisorChild) GetVariable(featureKey string, variableKey string, args ...interface{}) VariableValue {
	defer func() {
		if r := recover(); r != nil {
			c.parent.logger.Error("getVariable", LogDetails{
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

	evaluation := c.EvaluateVariable(featureKey, VariableKey(variableKey), contextValue, optionsValue)

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

	typedValue := GetValueByType(value, "boolean")
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

	typedValue := GetValueByType(value, "string")
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

	typedValue := GetValueByType(value, "integer")
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

	typedValue := GetValueByType(value, "double")
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

	typedValue := GetValueByType(value, "array")
	if arrayValue, ok := typedValue.([]interface{}); ok {
		result := make([]string, len(arrayValue))
		for i, item := range arrayValue {
			if strItem, ok := item.(string); ok {
				result[i] = strItem
			}
		}
		return result
	}

	return nil
}

// GetVariableObject gets an object variable
func (c *FeaturevisorChild) GetVariableObject(featureKey string, variableKey string, args ...interface{}) map[string]interface{} {
	value := c.GetVariable(featureKey, variableKey, args...)
	if value == nil {
		return nil
	}

	typedValue := GetValueByType(value, "object")
	if objectValue, ok := typedValue.(map[string]interface{}); ok {
		return objectValue
	}

	return nil
}

// GetVariableJSON gets a JSON variable
func (c *FeaturevisorChild) GetVariableJSON(featureKey string, variableKey string, args ...interface{}) interface{} {
	value := c.GetVariable(featureKey, variableKey, args...)
	if value == nil {
		return nil
	}

	return value
}

// GetAllEvaluations gets all evaluations for features
func (c *FeaturevisorChild) GetAllEvaluations(context Context, featureKeys []string, options OverrideOptions) EvaluatedFeatures {
	result := EvaluatedFeatures{}

	keys := featureKeys
	if len(keys) == 0 {
		// Get all feature keys from parent
		allKeys := c.parent.datafileReader.GetFeatureKeys()
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
		if c.parent.datafileReader.HasVariations(FeatureKey(featureKey)) {
			variation := c.GetVariation(featureKey, context, options)
			if variation != nil {
				evaluatedFeature.Variation = variation
			}
		}

		// variables
		variableKeys := c.parent.datafileReader.GetVariableKeys(FeatureKey(featureKey))
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
