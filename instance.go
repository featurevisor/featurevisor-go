package featurevisor

import (
	"encoding/json"
	"fmt"
)

// OverrideOptions contains options for overriding evaluation
type OverrideOptions struct {
	sticky *StickyFeatures

	DefaultVariationValue *VariationValue
	DefaultVariableValue  VariableValue
}

// SpawnOptions configures a child SDK instance.
type SpawnOptions struct {
	Sticky *StickyFeatures
}

// FeaturevisorOptions contains options for creating an instance.
type FeaturevisorOptions struct {
	Datafile     interface{} // DatafileContent | string
	Context      Context
	LogLevel     *LogLevel
	OnDiagnostic FeaturevisorDiagnosticHandler
	Sticky       *StickyFeatures
	Modules      []*FeaturevisorModule
}

type moduleDiagnosticSubscription struct {
	id       int
	module   *FeaturevisorModule
	handler  FeaturevisorDiagnosticHandler
	logLevel LogLevel
}

// Featurevisor represents a Featurevisor SDK instance
type Featurevisor struct {
	// from options
	context      Context
	logger       *featurevisorLogger
	logLevel     LogLevel
	onDiagnostic FeaturevisorDiagnosticHandler
	sticky       *StickyFeatures

	// internally created
	datafile                      DatafileContent
	datafileReader                *datafileReader
	modulesManager                *modulesManager
	moduleDiagnosticSubscriptions []moduleDiagnosticSubscription
	nextModuleDiagnosticID        int
	emitter                       *emitter
	closed                        bool
}

// CreateFeaturevisor creates a new Featurevisor instance.
func CreateFeaturevisor(options FeaturevisorOptions) *Featurevisor {
	// Set default context
	context := Context{}
	if options.Context != nil {
		context = options.Context
	}

	level := LogLevelInfo
	if options.LogLevel != nil {
		level = *options.LogLevel
	}
	var instance *Featurevisor
	handler := logHandler(func(logLevel LogLevel, message logMessage, details logDetails) {
		if instance == nil {
			return
		}
		code := string(message)
		if reason, ok := details["reason"].(EvaluationReason); ok {
			code = string(reason)
		} else if reason, ok := details["reason"].(string); ok {
			code = reason
		}
		if message == "feature is deprecated" {
			code = "deprecated_feature"
		} else if message == "variable is deprecated" {
			code = "deprecated_variable"
		} else if message == "feature not found" {
			code = "feature_not_found"
		} else if message == "variable schema not found" {
			code = "variable_not_found"
		} else if message == "no variations" {
			code = "no_variations"
		} else if message == "invalid bucketBy" {
			code = "invalid_bucket_by"
		}
		instance.reportDiagnostic(FeaturevisorDiagnostic{
			Level: logLevel, Code: code, Message: string(message), Details: details,
		}, nil)
	})
	logger := newLogger(loggerOptions{Level: &level, Handler: &handler})

	// Create emitter
	emitter := newEmitter()

	emptyDatafile := DatafileContent{
		SchemaVersion: "2",
		Revision:      "unknown",
		Segments:      make(map[SegmentKey]Segment),
		Features:      make(map[FeatureKey]Feature),
	}

	datafileReader := newDatafileReader(datafileReaderOptions{
		Datafile:           emptyDatafile,
		featurevisorLogger: logger,
	})

	instance = &Featurevisor{
		context:        context,
		logger:         logger,
		logLevel:       logger.GetLevel(),
		onDiagnostic:   options.OnDiagnostic,
		emitter:        emitter,
		datafile:       emptyDatafile,
		datafileReader: datafileReader,
		sticky:         options.Sticky,
	}

	instance.modulesManager = newModulesManager(modulesManagerOptions{
		Modules:                            options.Modules,
		ReportDiagnostic:                   instance.reportDiagnostic,
		GetModuleApi:                       instance.getModuleApi,
		ClearModuleDiagnosticSubscriptions: instance.clearModuleDiagnosticSubscriptions,
	})

	if options.Datafile != nil {
		instance.SetDatafile(options.Datafile, true)
	}

	instance.reportDiagnostic(FeaturevisorDiagnostic{
		Level:   LogLevelInfo,
		Code:    "sdk_initialized",
		Message: "SDK initialized",
	}, nil)

	return instance
}

// SetLogLevel sets the log level
func (i *Featurevisor) SetLogLevel(level LogLevel) {
	i.logLevel = level
	i.logger.SetLevel(level)
}

// SetDatafile sets the datafile
func (i *Featurevisor) SetDatafile(datafile interface{}, replace ...bool) {
	if i.closed {
		return
	}

	replaceValue := false
	if len(replace) > 0 {
		replaceValue = replace[0]
	}

	datafileContent, err := parseDatafileInput(datafile)
	if err != nil {
		i.reportDiagnostic(FeaturevisorDiagnostic{
			Level:         LogLevelError,
			Code:          "invalid_datafile",
			Message:       "Could not parse datafile",
			OriginalError: err,
		}, nil)
		return
	}

	storedDatafile := datafileContent
	if !replaceValue {
		storedDatafile = mergeStoredDatafile(i.datafile, datafileContent)
	}

	newDatafileReader := newDatafileReader(datafileReaderOptions{
		Datafile:           storedDatafile,
		featurevisorLogger: i.logger,
	})

	details := getParamsForDatafileSetEvent(i.datafileReader, newDatafileReader, replaceValue)

	i.datafile = storedDatafile
	i.datafileReader = newDatafileReader

	i.reportDiagnostic(FeaturevisorDiagnostic{
		Level:   LogLevelInfo,
		Code:    "datafile_set",
		Message: "Datafile set",
		Details: details,
	}, nil)
	i.emitter.Trigger(EventNameDatafileSet, EventDetails(details))
}

// SetSticky sets sticky features
func (i *Featurevisor) SetSticky(sticky StickyFeatures, replace ...bool) {
	if i.closed {
		return
	}

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

	i.reportDiagnostic(FeaturevisorDiagnostic{
		Level:   LogLevelInfo,
		Code:    "sticky_set",
		Message: "Sticky features set",
		Details: params,
	}, nil)
	i.emitter.Trigger(EventNameStickySet, EventDetails(params))
}

// GetRevision returns the revision
func (i *Featurevisor) GetRevision() string {
	return i.datafileReader.GetRevision()
}

func (i *Featurevisor) GetSchemaVersion() string {
	return i.datafileReader.GetSchemaVersion()
}

func (i *Featurevisor) GetSegment(segmentKey string) *Segment {
	return i.datafileReader.GetSegment(SegmentKey(segmentKey))
}

func (i *Featurevisor) GetFeatureKeys() []string {
	return i.datafileReader.GetFeatureKeys()
}

func (i *Featurevisor) GetVariableKeys(featureKey string) []string {
	return i.datafileReader.GetVariableKeys(FeatureKey(featureKey))
}

func (i *Featurevisor) HasVariations(featureKey string) bool {
	return i.datafileReader.HasVariations(FeatureKey(featureKey))
}

// GetFeature returns a feature by key
func (i *Featurevisor) GetFeature(featureKey string) *Feature {
	return i.datafileReader.GetFeature(FeatureKey(featureKey))
}

// AddModule adds a module.
func (i *Featurevisor) AddModule(module *FeaturevisorModule) FeaturevisorUnsubscribe {
	if i.closed {
		return nil
	}

	return i.modulesManager.Add(module)
}

// RemoveModule removes modules by name.
func (i *Featurevisor) RemoveModule(name string) {
	if i.closed {
		return
	}

	i.modulesManager.Remove(name)
}

// On adds an event listener
func (i *Featurevisor) On(eventName EventName, callback EventCallback) Unsubscribe {
	if i.closed {
		return func() {}
	}

	return i.emitter.On(eventName, callback)
}

// Close closes the instance
func (i *Featurevisor) Close() {
	if i.closed {
		return
	}

	i.closed = true
	i.modulesManager.CloseAll()
	i.moduleDiagnosticSubscriptions = nil
	i.emitter.ClearAll()
}

func (i *Featurevisor) reportDiagnostic(
	diagnostic FeaturevisorDiagnostic,
	sourceModule *FeaturevisorModule,
) {
	if diagnostic.Details == nil {
		diagnostic.Details = map[string]interface{}{}
	}

	for _, subscription := range append([]moduleDiagnosticSubscription{}, i.moduleDiagnosticSubscriptions...) {
		if subscription.module == sourceModule {
			continue
		}
		if !shouldLogDiagnostic(subscription.logLevel, diagnostic.Level) {
			continue
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					_ = recovered
				}
			}()
			subscription.handler(diagnostic)
		}()
	}

	if shouldLogDiagnostic(i.logLevel, diagnostic.Level) {
		if i.onDiagnostic != nil {
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						_ = recovered
					}
				}()
				i.onDiagnostic(diagnostic)
			}()
		} else {
			details := logDetails{}
			if diagnostic.Details != nil {
				for key, value := range diagnostic.Details {
					details[key] = value
				}
			}
			if diagnostic.Code != "" {
				details["code"] = diagnostic.Code
			}
			if diagnostic.Module != "" {
				details["module"] = diagnostic.Module
			}
			if diagnostic.ModuleName != "" {
				details["moduleName"] = diagnostic.ModuleName
			}
			if diagnostic.OriginalError != nil {
				details["originalError"] = diagnostic.OriginalError
			}
			defaultLogHandler(diagnostic.Level, logMessage(diagnostic.Message), details)
		}
	}

	if diagnostic.Level == LogLevelError {
		i.emitter.Trigger(EventNameError, EventDetails{"diagnostic": diagnostic})
	}
}

func (i *Featurevisor) getModuleApi(module *FeaturevisorModule) FeaturevisorModuleApi {
	return FeaturevisorModuleApi{
		GetRevision: func() string {
			return i.GetRevision()
		},
		OnDiagnostic: func(
			handler FeaturevisorDiagnosticHandler,
			options ...FeaturevisorModuleDiagnosticOptions,
		) FeaturevisorUnsubscribe {
			logLevel := LogLevelInfo
			if len(options) > 0 && options[0].LogLevel != "" {
				logLevel = options[0].LogLevel
			}

			subscription := moduleDiagnosticSubscription{
				id:       i.nextModuleDiagnosticID,
				module:   module,
				handler:  handler,
				logLevel: logLevel,
			}
			i.nextModuleDiagnosticID++

			i.moduleDiagnosticSubscriptions = append(i.moduleDiagnosticSubscriptions, subscription)
			subscriptionID := subscription.id

			return func() {
				filtered := []moduleDiagnosticSubscription{}
				for _, currentSubscription := range i.moduleDiagnosticSubscriptions {
					if currentSubscription.id != subscriptionID {
						filtered = append(filtered, currentSubscription)
					}
				}
				i.moduleDiagnosticSubscriptions = filtered
			}
		},
		ReportDiagnostic: func(diagnostic FeaturevisorModuleReportedDiagnostic) {
			if module != nil && module.Name != "" {
				diagnostic.Module = module.Name
			}
			i.reportDiagnostic(diagnostic, module)
		},
	}
}

func (i *Featurevisor) clearModuleDiagnosticSubscriptions(module *FeaturevisorModule) {
	filtered := []moduleDiagnosticSubscription{}
	for _, subscription := range i.moduleDiagnosticSubscriptions {
		if subscription.module != module {
			filtered = append(filtered, subscription)
		}
	}
	i.moduleDiagnosticSubscriptions = filtered
}

// SetContext sets the context
func (i *Featurevisor) SetContext(context Context, replace ...bool) {
	if i.closed {
		return
	}

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

	message := "Context updated"
	if replaceValue {
		message = "Context replaced"
	}

	i.reportDiagnostic(FeaturevisorDiagnostic{
		Level:   LogLevelDebug,
		Code:    "context_set",
		Message: message,
		Details: logDetails{
			"context":  i.context,
			"replaced": replaceValue,
		},
	}, nil)
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
	optionsValue := SpawnOptions{}

	// Parse variadic arguments
	for _, arg := range args {
		switch v := arg.(type) {
		case Context:
			contextValue = v
		case SpawnOptions:
			optionsValue = v
		}
	}

	return newFeaturevisorChild(ChildOptions{
		Parent:  i,
		Context: i.GetContext(contextValue),
		Sticky:  optionsValue.Sticky,
	})
}

// getEvaluationDependencies gets evaluation dependencies
func (i *Featurevisor) getEvaluationDependencies(context Context, options OverrideOptions) EvaluateDependencies {
	var sticky *StickyFeatures
	if options.sticky != nil {
		sticky = options.sticky
	} else {
		sticky = i.sticky
	}

	return EvaluateDependencies{
		Context:               i.GetContext(context),
		featurevisorLogger:    i.logger,
		modulesManager:        i.modulesManager,
		datafileReader:        i.datafileReader,
		sticky:                sticky,
		DefaultVariationValue: options.DefaultVariationValue,
		DefaultVariableValue:  options.DefaultVariableValue,
	}
}

// EvaluateFlag evaluates a feature flag
func (i *Featurevisor) EvaluateFlag(featureKey string, context Context, options OverrideOptions) Evaluation {
	return EvaluateWithModules(EvaluateOptions{
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
			i.logger.Error("isEnabled", logDetails{
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
	return EvaluateWithModules(EvaluateOptions{
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
			i.logger.Error("getVariation", logDetails{
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
	return EvaluateWithModules(EvaluateOptions{
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
			i.logger.Error("getVariable", logDetails{
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
					i.logger.Error("could not parse JSON variable", logDetails{
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

	return ToTypedArray[string](GetValueByType(value, "array"))
}

// GetVariableObject gets an object variable
func (i *Featurevisor) GetVariableObject(featureKey string, variableKey string, args ...interface{}) map[string]interface{} {
	value := i.GetVariable(featureKey, variableKey, args...)
	if value == nil {
		return nil
	}

	typedValue := ToTypedObject[map[string]interface{}](GetValueByType(value, "object"))
	if typedValue == nil {
		return nil
	}

	return *typedValue
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

// GetVariableArrayInto decodes an array variable into the provided pointer output.
// Supported argument order (after featureKey, variableKey): out OR context, out OR context, options, out.
func (i *Featurevisor) GetVariableArrayInto(featureKey string, variableKey string, args ...interface{}) error {
	context, options, out, err := parseVariableIntoArgs(args...)
	if err != nil {
		return err
	}

	value := i.GetVariable(featureKey, variableKey, context, options)
	if value == nil {
		return decodeInto(nil, out)
	}

	arrayValue := GetValueByType(value, "array")
	if arrayValue == nil {
		return fmt.Errorf("variable %q is not an array", variableKey)
	}

	return decodeInto(arrayValue, out)
}

// GetVariableObjectInto decodes an object variable into the provided pointer output.
// Supported argument order (after featureKey, variableKey): out OR context, out OR context, options, out.
func (i *Featurevisor) GetVariableObjectInto(featureKey string, variableKey string, args ...interface{}) error {
	context, options, out, err := parseVariableIntoArgs(args...)
	if err != nil {
		return err
	}

	value := i.GetVariable(featureKey, variableKey, context, options)
	if value == nil {
		return decodeInto(nil, out)
	}

	objectValue := GetValueByType(value, "object")
	if objectValue == nil {
		return fmt.Errorf("variable %q is not an object", variableKey)
	}

	return decodeInto(objectValue, out)
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

func mergeStoredDatafile(existing DatafileContent, incoming DatafileContent) DatafileContent {
	mergedSegments := map[SegmentKey]Segment{}
	for key, value := range existing.Segments {
		mergedSegments[key] = value
	}
	for key, value := range incoming.Segments {
		mergedSegments[key] = value
	}

	mergedFeatures := map[FeatureKey]Feature{}
	for key, value := range existing.Features {
		mergedFeatures[key] = value
	}
	for key, value := range incoming.Features {
		mergedFeatures[key] = value
	}

	return DatafileContent{
		SchemaVersion:       incoming.SchemaVersion,
		Revision:            incoming.Revision,
		FeaturevisorVersion: incoming.FeaturevisorVersion,
		Segments:            mergedSegments,
		Features:            mergedFeatures,
	}
}

func parseDatafileInput(datafile interface{}) (DatafileContent, error) {
	var datafileContent DatafileContent

	switch value := datafile.(type) {
	case string:
		if err := datafileContent.FromJSON(value); err != nil {
			return DatafileContent{}, fmt.Errorf("invalid datafile string: %w", err)
		}
		return validateDatafileContent(datafileContent)
	case map[string]interface{}:
		bytes, err := json.Marshal(value)
		if err != nil {
			return DatafileContent{}, fmt.Errorf("failed to marshal datafile map: %w", err)
		}

		if err := datafileContent.FromJSON(string(bytes)); err != nil {
			return DatafileContent{}, fmt.Errorf("invalid datafile map: %w", err)
		}

		return validateDatafileContent(datafileContent)
	case DatafileContent:
		return validateDatafileContent(value)
	case *DatafileContent:
		if value == nil {
			return DatafileContent{}, fmt.Errorf("datafile pointer is nil")
		}
		return validateDatafileContent(*value)
	default:
		return DatafileContent{}, fmt.Errorf("unsupported datafile input type: %T", datafile)
	}
}

func validateDatafileContent(datafile DatafileContent) (DatafileContent, error) {
	if datafile.SchemaVersion == "" || datafile.Revision == "" || datafile.Segments == nil || datafile.Features == nil {
		return DatafileContent{}, fmt.Errorf("invalid datafile")
	}

	return datafile, nil
}
