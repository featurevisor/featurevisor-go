package featurevisor

import (
	"encoding/json"
)

// OverrideOptions contains options for overriding evaluation
type OverrideOptions struct {
	Sticky *StickyFeatures

	DefaultVariationValue *VariationValue
	DefaultVariableValue  VariableValue
}

// InstanceOptions contains options for creating an instance
type InstanceOptions struct {
	Datafile interface{} // DatafileContent | string
	Context  Context
	LogLevel *LogLevel
	Logger   *Logger
	Sticky   *StickyFeatures
	Hooks    []*Hook
}

// Featurevisor represents a Featurevisor SDK instance
type Featurevisor struct {
	// from options
	context Context
	logger  *Logger
	sticky  *StickyFeatures

	// internally created
	datafileReader *DatafileReader
	hooksManager   *HooksManager
	emitter        *Emitter
}

// NewFeaturevisor creates a new Featurevisor instance
func NewFeaturevisor(options InstanceOptions) *Featurevisor {
	// Set default context
	context := Context{}
	if options.Context != nil {
		context = options.Context
	}

	// Set default logger
	var logger *Logger
	if options.Logger != nil {
		logger = options.Logger
	} else {
		level := LogLevelWarn
		if options.LogLevel != nil {
			level = *options.LogLevel
		}
		logger = NewLogger(CreateLoggerOptions{Level: &level})
	}

	// Create hooks manager
	hooksManager := NewHooksManager(HooksManagerOptions{
		Logger: logger,
		Hooks:  options.Hooks,
	})

	// Create emitter
	emitter := NewEmitter()

	// Create datafile reader
	emptyDatafile := DatafileContent{
		SchemaVersion: "2",
		Revision:      "unknown",
		Segments:      make(map[SegmentKey]Segment),
		Features:      make(map[FeatureKey]Feature),
	}

	datafileReader := NewDatafileReader(DatafileReaderOptions{
		Datafile: emptyDatafile,
		Logger:   logger,
	})

	// If datafile is provided, set it
	if options.Datafile != nil {
		var datafileContent DatafileContent

		if datafileStr, ok := options.Datafile.(string); ok {
			// Parse JSON string using DatafileContent.FromJSON
			if err := datafileContent.FromJSON(datafileStr); err == nil {
				datafileReader = NewDatafileReader(DatafileReaderOptions{
					Datafile: datafileContent,
					Logger:   logger,
				})
			}
		} else if datafileMap, ok := options.Datafile.(map[string]interface{}); ok {
			// Convert map to DatafileContent
			if datafileBytes, err := json.Marshal(datafileMap); err == nil {
				if err := datafileContent.FromJSON(string(datafileBytes)); err == nil {
					datafileReader = NewDatafileReader(DatafileReaderOptions{
						Datafile: datafileContent,
						Logger:   logger,
					})
				}
			}
		} else if datafileContent, ok := options.Datafile.(DatafileContent); ok {
			// Direct DatafileContent
			datafileReader = NewDatafileReader(DatafileReaderOptions{
				Datafile: datafileContent,
				Logger:   logger,
			})
		}
	}

	instance := &Featurevisor{
		context:        context,
		logger:         logger,
		hooksManager:   hooksManager,
		emitter:        emitter,
		datafileReader: datafileReader,
		sticky:         options.Sticky,
	}

	logger.Info("Featurevisor SDK initialized", LogDetails{})

	return instance
}

// SetLogLevel sets the log level
func (i *Featurevisor) SetLogLevel(level LogLevel) {
	i.logger.SetLevel(level)
}

// SetDatafile sets the datafile
func (i *Featurevisor) SetDatafile(datafile DatafileContent) {
	datafileContent := datafile

	newDatafileReader := NewDatafileReader(DatafileReaderOptions{
		Datafile: datafileContent,
		Logger:   i.logger,
	})

	// Get details for datafile set event
	details := getParamsForDatafileSetEvent(i.datafileReader, newDatafileReader)

	i.datafileReader = newDatafileReader

	i.logger.Info("datafile set", details)
	i.emitter.Trigger(EventNameDatafileSet, EventDetails(details))
}

// SetSticky sets sticky features
func (i *Featurevisor) SetSticky(sticky StickyFeatures, replace ...bool) {
	replaceValue := false
	if len(replace) > 0 {
		replaceValue = replace[0]
	}

	previousStickyFeatures := StickyFeatures{}
	if i.sticky != nil {
		previousStickyFeatures = *i.sticky
	}

	if replaceValue {
		i.sticky = &sticky
	} else {
		newSticky := StickyFeatures{}
		if i.sticky != nil {
			newSticky = *i.sticky
		}
		// Merge sticky features
		for key, value := range sticky {
			newSticky[key] = value
		}
		i.sticky = &newSticky
	}

	params := getParamsForStickySetEvent(previousStickyFeatures, *i.sticky, replaceValue)

	i.logger.Info("sticky features set", params)
	i.emitter.Trigger(EventNameStickySet, EventDetails(params))
}

// GetRevision returns the revision
func (i *Featurevisor) GetRevision() string {
	return i.datafileReader.GetRevision()
}

// GetFeature returns a feature by key
func (i *Featurevisor) GetFeature(featureKey string) *Feature {
	return i.datafileReader.GetFeature(FeatureKey(featureKey))
}

// AddHook adds a hook
func (i *Featurevisor) AddHook(hook *Hook) {
	i.hooksManager.Add(hook)
}

// On adds an event listener
func (i *Featurevisor) On(eventName EventName, callback EventCallback) {
	i.emitter.On(eventName, callback)
}

// Close closes the instance
func (i *Featurevisor) Close() {
	i.emitter.ClearAll()
}

// SetContext sets the context
func (i *Featurevisor) SetContext(context Context, replace ...bool) {
	replaceValue := false
	if len(replace) > 0 {
		replaceValue = replace[0]
	}

	if replaceValue {
		i.context = context
	} else {
		// Merge context
		for key, value := range context {
			i.context[key] = value
		}
	}

	i.emitter.Trigger("context_set", map[string]interface{}{
		"context":  i.context,
		"replaced": replaceValue,
	})

	if replaceValue {
		i.logger.Debug("context replaced", LogDetails{"context": i.context})
	} else {
		i.logger.Debug("context updated", LogDetails{"context": i.context})
	}
}

// GetContext returns the context
func (i *Featurevisor) GetContext(context Context) Context {
	if context == nil {
		return i.context
	}

	// Merge contexts
	result := Context{}
	for key, value := range i.context {
		result[key] = value
	}
	for key, value := range context {
		result[key] = value
	}

	return result
}

// Spawn creates a child instance
func (i *Featurevisor) Spawn(args ...interface{}) *FeaturevisorChild {
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

	return NewFeaturevisorChild(ChildInstanceOptions{
		Parent:  i,
		Context: i.GetContext(contextValue),
		Sticky:  optionsValue.Sticky,
	})
}

// getEvaluationDependencies gets evaluation dependencies
func (i *Featurevisor) getEvaluationDependencies(context Context, options OverrideOptions) EvaluateDependencies {
	var sticky *StickyFeatures
	if options.Sticky != nil {
		if i.sticky != nil {
			// Merge sticky features
			mergedSticky := StickyFeatures{}
			for key, value := range *i.sticky {
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
		sticky = i.sticky
	}

	return EvaluateDependencies{
		Context:               i.GetContext(context),
		Logger:                i.logger,
		HooksManager:          i.hooksManager,
		DatafileReader:        i.datafileReader,
		Sticky:                sticky,
		DefaultVariationValue: options.DefaultVariationValue,
		DefaultVariableValue:  options.DefaultVariableValue,
	}
}

// EvaluateFlag evaluates a feature flag
func (i *Featurevisor) EvaluateFlag(featureKey string, context Context, options OverrideOptions) Evaluation {
	return EvaluateWithHooks(EvaluateOptions{
		EvaluateParams: EvaluateParams{
			Type:       EvaluationTypeFlag,
			FeatureKey: FeatureKey(featureKey),
		},
		EvaluateDependencies: i.getEvaluationDependencies(context, options),
	})
}

// IsEnabled checks if a feature is enabled
func (i *Featurevisor) IsEnabled(featureKey string, args ...interface{}) bool {
	defer func() {
		if r := recover(); r != nil {
			i.logger.Error("isEnabled", LogDetails{
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

	evaluation := i.EvaluateFlag(featureKey, contextValue, optionsValue)

	if evaluation.Enabled != nil {
		return *evaluation.Enabled
	}

	return false
}

// EvaluateVariation evaluates a feature variation
func (i *Featurevisor) EvaluateVariation(featureKey string, context Context, options OverrideOptions) Evaluation {
	return EvaluateWithHooks(EvaluateOptions{
		EvaluateParams: EvaluateParams{
			Type:       EvaluationTypeVariation,
			FeatureKey: FeatureKey(featureKey),
		},
		EvaluateDependencies: i.getEvaluationDependencies(context, options),
	})
}

// GetVariation gets a feature variation
func (i *Featurevisor) GetVariation(featureKey string, args ...interface{}) *string {
	defer func() {
		if r := recover(); r != nil {
			i.logger.Error("getVariation", LogDetails{
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

	evaluation := i.EvaluateVariation(featureKey, contextValue, optionsValue)

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
func (i *Featurevisor) EvaluateVariable(featureKey string, variableKey VariableKey, context Context, options OverrideOptions) Evaluation {
	return EvaluateWithHooks(EvaluateOptions{
		EvaluateParams: EvaluateParams{
			Type:        EvaluationTypeVariable,
			FeatureKey:  FeatureKey(featureKey),
			VariableKey: &variableKey,
		},
		EvaluateDependencies: i.getEvaluationDependencies(context, options),
	})
}

// GetVariable gets a feature variable
func (i *Featurevisor) GetVariable(featureKey string, variableKey string, args ...interface{}) VariableValue {
	defer func() {
		if r := recover(); r != nil {
			i.logger.Error("getVariable", LogDetails{
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

	evaluation := i.EvaluateVariable(featureKey, VariableKey(variableKey), contextValue, optionsValue)

	if evaluation.VariableValue != nil {
		// Handle JSON variables
		if evaluation.VariableSchema != nil && evaluation.VariableSchema.Type == "json" {
			if variableStr, ok := evaluation.VariableValue.(string); ok {
				var parsedJSON interface{}
				if err := json.Unmarshal([]byte(variableStr), &parsedJSON); err == nil {
					return parsedJSON
				} else {
					// Log error if JSON parsing fails
					i.logger.Error("could not parse JSON variable", LogDetails{
						"featureKey":  featureKey,
						"variableKey": variableKey,
						"error":       err,
					})
				}
			}
		}

		// Apply type conversion for default values
		if evaluation.VariableSchema != nil && evaluation.Reason == EvaluationReasonVariableDefault {
			return GetValueByType(evaluation.VariableValue, string(evaluation.VariableSchema.Type))
		}

		return evaluation.VariableValue
	}

	return nil
}

// GetVariableBoolean gets a boolean variable
func (i *Featurevisor) GetVariableBoolean(featureKey string, variableKey string, args ...interface{}) *bool {
	value := i.GetVariable(featureKey, variableKey, args...)
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
func (i *Featurevisor) GetVariableString(featureKey string, variableKey string, args ...interface{}) *string {
	value := i.GetVariable(featureKey, variableKey, args...)
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
func (i *Featurevisor) GetVariableInteger(featureKey string, variableKey string, args ...interface{}) *int {
	value := i.GetVariable(featureKey, variableKey, args...)
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
func (i *Featurevisor) GetVariableDouble(featureKey string, variableKey string, args ...interface{}) *float64 {
	value := i.GetVariable(featureKey, variableKey, args...)
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
func (i *Featurevisor) GetVariableArray(featureKey string, variableKey string, args ...interface{}) []string {
	value := i.GetVariable(featureKey, variableKey, args...)
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
func (i *Featurevisor) GetVariableObject(featureKey string, variableKey string, args ...interface{}) map[string]interface{} {
	value := i.GetVariable(featureKey, variableKey, args...)
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
func (i *Featurevisor) GetVariableJSON(featureKey string, variableKey string, args ...interface{}) interface{} {
	value := i.GetVariable(featureKey, variableKey, args...)
	if value == nil {
		return nil
	}

	// JSON variables are already parsed in GetVariable
	return value
}

// GetAllEvaluations gets all evaluations for features
func (i *Featurevisor) GetAllEvaluations(context Context, featureKeys []string, options OverrideOptions) EvaluatedFeatures {
	result := EvaluatedFeatures{}

	keys := featureKeys
	if len(keys) == 0 {
		// Get all feature keys
		allKeys := i.datafileReader.GetFeatureKeys()
		keys = make([]string, len(allKeys))
		for j, key := range allKeys {
			keys[j] = string(key)
		}
	}

	for _, featureKey := range keys {
		// isEnabled
		evaluatedFeature := EvaluatedFeature{
			Enabled: i.IsEnabled(featureKey, context, options),
		}

		// variation
		if i.datafileReader.HasVariations(FeatureKey(featureKey)) {
			variation := i.GetVariation(featureKey, context, options)
			if variation != nil {
				evaluatedFeature.Variation = variation
			}
		}

		// variables
		variableKeys := i.datafileReader.GetVariableKeys(FeatureKey(featureKey))
		if len(variableKeys) > 0 {
			evaluatedFeature.Variables = make(map[VariableKey]VariableValue)
			for _, variableKey := range variableKeys {
				evaluatedFeature.Variables[variableKey] = i.GetVariable(
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

// CreateInstance creates a new Featurevisor instance
func CreateInstance(options InstanceOptions) *Featurevisor {
	return NewFeaturevisor(options)
}
