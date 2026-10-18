package featurevisor

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// OverrideOptions contains options for overriding evaluation
type OverrideOptions struct {
	stickyFeatures  *StickyFeatures
	stickyVariables *StickyVariables

	DefaultVariationValue   *VariationValue
	DefaultVariableValue    VariableValue
	DefaultVariableValueSet bool
}

// SpawnOptions configures a child SDK instance.
type SpawnOptions struct {
	StickyFeatures  *StickyFeatures
	StickyVariables *StickyVariables
}

// FeaturevisorOptions contains options for creating an instance.
type FeaturevisorOptions struct {
	Datafile        interface{} // DatafileContent | string
	Context         Context
	LogLevel        *LogLevel
	OnDiagnostic    FeaturevisorDiagnosticHandler
	StickyFeatures  *StickyFeatures
	StickyVariables *StickyVariables
	Modules         []*FeaturevisorModule
}

type moduleDiagnosticSubscription struct {
	id       int
	module   *FeaturevisorModule
	handler  FeaturevisorDiagnosticHandler
	logLevel LogLevel
}

// Featurevisor represents a Featurevisor SDK instance
type Featurevisor struct {
	stateMu         sync.RWMutex
	dependencyIndex *datafileDependencyIndex
	// from options
	context         Context
	diagnostics     *diagnosticReporter
	logLevel        LogLevel
	onDiagnostic    FeaturevisorDiagnosticHandler
	stickyFeatures  *StickyFeatures
	stickyVariables *StickyVariables

	// internally created
	datafile                       DatafileContent
	instanceEvaluationDataProvider *instanceEvaluationDataProvider
	modulesManager                 *modulesManager
	moduleDiagnosticSubscriptions  []moduleDiagnosticSubscription
	nextModuleDiagnosticID         int
	emitter                        *emitter
	closed                         bool
	stateGeneration                atomic.Uint64
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
	handler := diagnosticOutputHandler(func(logLevel LogLevel, message logMessage, details logDetails) {
		if instance == nil {
			return
		}
		normalizedDetails := make(logDetails, len(details))
		for key, value := range details {
			normalizedDetails[key] = value
		}
		details = normalizedDetails

		var originalError interface{}
		if value, ok := details["originalError"]; ok {
			originalError = value
			delete(details, "originalError")
		} else if value, ok := details["error"]; ok {
			originalError = value
			delete(details, "error")
		}
		if nested, ok := details["details"].(map[string]interface{}); ok {
			delete(details, "details")
			for key, value := range nested {
				details[key] = value
			}
		} else if nested, ok := details["details"].(logDetails); ok {
			delete(details, "details")
			for key, value := range nested {
				details[key] = value
			}
		}

		code := string(message)
		if explicitCode, ok := details["code"].(string); ok {
			code = explicitCode
			delete(details, "code")
		}
		if evaluation, ok := details["evaluation"].(Evaluation); ok {
			code = string(evaluation.Reason)
			details = logDetails{
				"featureKey":  evaluation.FeatureKey,
				"variableKey": evaluation.VariableKey,
				"reason":      evaluation.Reason,
				"evaluation":  evaluation,
			}
		} else if reason, ok := details["reason"].(EvaluationReason); ok {
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
		} else if message == "Error in condition matching" {
			code = "condition_match_error"
		} else if message == "Error parsing conditions" {
			code = "conditions_parse_error"
		} else if message == "getVariable failed" {
			code = "evaluation_error"
		} else if message == "panic during evaluation" || message == "panic in evaluate" {
			code = "evaluation_error"
		}
		instance.reportDiagnostic(FeaturevisorDiagnostic{
			Level: logLevel, Code: code, Message: string(message), OriginalError: originalError, Details: details,
		}, nil)
	})
	diagnostics := newDiagnosticReporter(diagnosticReporterOptions{
		Level:   &level,
		Handler: &handler,
		ShouldHandle: func(level LogLevel) bool {
			return instance == nil || instance.shouldReportDiagnostic(level)
		},
	})

	// Create emitter
	emitter := newEmitter()

	emptyDatafile := DatafileContent{
		SchemaVersion: "2",
		Revision:      "unknown",
		Segments:      make(map[SegmentKey]Segment),
		Features:      make(map[FeatureKey]Feature),
		Variables:     make(map[GlobalVariableKey]GlobalVariable),
	}

	instanceEvaluationDataProvider := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           emptyDatafile,
		diagnosticReporter: diagnostics,
	})

	instance = &Featurevisor{
		context:                        context,
		diagnostics:                    diagnostics,
		logLevel:                       diagnostics.GetLevel(),
		onDiagnostic:                   options.OnDiagnostic,
		emitter:                        emitter,
		datafile:                       emptyDatafile,
		instanceEvaluationDataProvider: instanceEvaluationDataProvider,
		stickyFeatures:                 options.StickyFeatures,
		stickyVariables:                options.StickyVariables,
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
	i.diagnostics.SetLevel(level)
}

// SetDatafile returns true when the datafile is accepted, even if unchanged.
// Rejected input and calls after Close return false.
func (i *Featurevisor) SetDatafile(datafile interface{}, replace ...bool) bool {
	i.stateMu.RLock()
	closed := i.closed
	i.stateMu.RUnlock()
	if closed {
		return false
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
		return false
	}

	datafileContent, normalizationErrors := normalizeLegacyDatafile(datafileContent)
	storedDatafile := datafileContent
	i.stateMu.Lock()
	if i.closed {
		i.stateMu.Unlock()
		return false
	}
	if !replaceValue {
		storedDatafile = mergeStoredDatafile(i.datafile, datafileContent)
	}

	newInstanceEvaluationDataProvider := newInstanceEvaluationDataProvider(instanceEvaluationDataProviderOptions{
		Datafile:           storedDatafile,
		diagnosticReporter: i.diagnostics,
	})

	if !replaceValue && i.instanceEvaluationDataProvider != nil {
		old := i.instanceEvaluationDataProvider
		old.entityMu.Lock()
		retain := func(kind, key string) {
			identity := kind + ":" + key
			slot := old.entitySlots[identity]
			if slot == nil {
				slot = &normalizedEntitySlot{value: old.entities[identity]}
				old.entitySlots[identity] = slot
			}
			newInstanceEvaluationDataProvider.entitySlots[identity] = slot
			if value, ok := old.entities[identity]; ok {
				newInstanceEvaluationDataProvider.entities[identity] = value
			}
		}
		for key := range old.features {
			if _, incoming := datafileContent.Features[key]; !incoming {
				retain("feature", key)
			}
		}
		for key := range old.variables {
			if _, incoming := datafileContent.Variables[key]; !incoming {
				retain("variable", key)
			}
		}
		for key := range old.segments {
			if _, incoming := datafileContent.Segments[key]; !incoming {
				retain("segment", key)
			}
		}
		old.entityMu.Unlock()
	}
	features, variables := updateAffectedDatafileEntities(i.instanceEvaluationDataProvider, newInstanceEvaluationDataProvider, datafileContent, replaceValue, &i.dependencyIndex)
	details := logDetails{
		"revision":         storedDatafile.Revision,
		"previousRevision": i.datafile.Revision,
		"revisionChanged":  i.datafile.Revision != storedDatafile.Revision,
		"features":         features,
		"variables":        variables,
		"replaced":         replaceValue,
	}

	i.datafile = storedDatafile
	i.instanceEvaluationDataProvider = newInstanceEvaluationDataProvider
	i.stateGeneration.Add(1)
	i.stateMu.Unlock()

	for _, diagnostic := range normalizationErrors {
		i.reportDiagnostic(diagnostic, nil)
	}
	i.reportDiagnostic(FeaturevisorDiagnostic{
		Level:   LogLevelInfo,
		Code:    "datafile_set",
		Message: "Datafile set",
		Details: details,
	}, nil)
	i.emitter.Trigger(EventNameDatafileSet, EventDetails(details))
	return true
}

// SetStickyFeatures sets sticky feature evaluations.
func (i *Featurevisor) SetStickyFeatures(sticky StickyFeatures, replace ...bool) {
	i.stateMu.Lock()
	if i.closed {
		i.stateMu.Unlock()
		return
	}

	replaceValue := false
	if len(replace) > 0 {
		replaceValue = replace[0]
	}

	previousStickyFeatures := StickyFeatures{}
	if i.stickyFeatures != nil {
		for key, value := range *i.stickyFeatures {
			previousStickyFeatures[key] = value
		}
	}

	if replaceValue {
		next := StickyFeatures{}
		for key, value := range sticky {
			next[key] = value
		}
		i.stickyFeatures = &next
	} else {
		newSticky := StickyFeatures{}
		if i.stickyFeatures != nil {
			for key, value := range *i.stickyFeatures {
				newSticky[key] = value
			}
		}
		// Merge sticky features
		for key, value := range sticky {
			newSticky[key] = value
		}
		i.stickyFeatures = &newSticky
	}
	i.stateGeneration.Add(1)

	params := getParamsForStickyFeaturesSetEvent(previousStickyFeatures, *i.stickyFeatures, replaceValue)
	i.stateMu.Unlock()

	i.reportDiagnostic(FeaturevisorDiagnostic{
		Level:   LogLevelInfo,
		Code:    "sticky_features_set",
		Message: "Sticky features set",
		Details: params,
	}, nil)
	i.emitter.Trigger(EventNameStickyFeaturesSet, EventDetails(params))
}

// SetStickyVariables sets sticky global variable values.
func (i *Featurevisor) SetStickyVariables(sticky StickyVariables, replace ...bool) {
	i.stateMu.Lock()
	if i.closed {
		i.stateMu.Unlock()
		return
	}
	replaceValue := len(replace) > 0 && replace[0]
	previous := StickyVariables{}
	if i.stickyVariables != nil {
		for key, value := range *i.stickyVariables {
			previous[key] = value
		}
	}
	next := StickyVariables{}
	if !replaceValue {
		for key, value := range previous {
			next[key] = value
		}
	}
	for key, value := range sticky {
		next[key] = value
	}
	i.stickyVariables = &next
	i.stateGeneration.Add(1)
	i.stateMu.Unlock()
	keys := make([]string, 0, len(previous)+len(next))
	seen := map[string]bool{}
	for key := range previous {
		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	for key := range next {
		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	details := logDetails{"variables": keys, "replaced": replaceValue}
	i.reportDiagnostic(FeaturevisorDiagnostic{Level: LogLevelInfo, Code: "sticky_variables_set", Message: "Sticky variables set", Details: details}, nil)
	i.emitter.Trigger(EventNameStickyVariablesSet, EventDetails(details))
}

// GetRevision returns the revision
func (i *Featurevisor) GetRevision() string {
	return i.dataProvider().GetRevision()
}

func (i *Featurevisor) dataProvider() *instanceEvaluationDataProvider {
	i.stateMu.RLock()
	defer i.stateMu.RUnlock()
	return i.instanceEvaluationDataProvider
}

func (i *Featurevisor) isClosed() bool {
	i.stateMu.RLock()
	defer i.stateMu.RUnlock()
	return i.closed
}

func (i *Featurevisor) GetSchemaVersion() string {
	return i.dataProvider().GetSchemaVersion()
}

func (i *Featurevisor) GetSegment(segmentKey string) *Segment {
	return i.dataProvider().GetSegment(SegmentKey(segmentKey))
}

func (i *Featurevisor) GetFeatureKeys() []string {
	return i.dataProvider().GetFeatureKeys()
}

func (i *Featurevisor) GetVariableKeys(featureKey string) []string {
	return i.dataProvider().GetVariableKeys(FeatureKey(featureKey))
}

// GetGlobalVariableKeys returns all global variable keys.
func (i *Featurevisor) GetGlobalVariableKeys() []string {
	return i.dataProvider().GetGlobalVariableKeys()
}

func (i *Featurevisor) HasVariations(featureKey string) bool {
	return i.dataProvider().HasVariations(FeatureKey(featureKey))
}

// GetFeature returns a feature by key
func (i *Featurevisor) GetFeature(featureKey string) *Feature {
	return i.dataProvider().GetFeature(FeatureKey(featureKey))
}

// AddModule adds a module.
func (i *Featurevisor) AddModule(module *FeaturevisorModule) FeaturevisorUnsubscribe {
	if i.isClosed() {
		return nil
	}

	unsubscribe := i.modulesManager.Add(module)
	if unsubscribe == nil {
		return nil
	}
	if i.isClosed() {
		unsubscribe()
		return nil
	}
	i.stateGeneration.Add(1)
	return func() {
		unsubscribe()
		i.stateGeneration.Add(1)
	}
}

// RemoveModule removes modules by name.
func (i *Featurevisor) RemoveModule(name string) {
	if i.isClosed() {
		return
	}

	i.modulesManager.Remove(name)
	i.stateGeneration.Add(1)
}

// On adds an event listener
func (i *Featurevisor) On(eventName EventName, callback EventCallback) Unsubscribe {
	if i.isClosed() {
		return func() {}
	}

	return i.emitter.On(eventName, callback)
}

// Close closes the instance
func (i *Featurevisor) Close() {
	i.stateMu.Lock()
	if i.closed {
		i.stateMu.Unlock()
		return
	}

	i.closed = true
	i.dependencyIndex = nil
	i.stateGeneration.Add(1)
	provider := i.instanceEvaluationDataProvider
	i.stateMu.Unlock()
	i.modulesManager.CloseAll()
	i.moduleDiagnosticSubscriptions = nil
	provider.closeExpressionCache()
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
			defaultDiagnosticHandler(diagnostic.Level, logMessage(diagnostic.Message), details)
		}
	}

	if diagnostic.Level == LogLevelError {
		i.emitter.Trigger(EventNameError, EventDetails{"diagnostic": diagnostic})
	}
}

func (i *Featurevisor) shouldReportDiagnostic(level LogLevel) bool {
	if level == LogLevelError || shouldLogDiagnostic(i.logLevel, level) {
		return true
	}
	for _, subscription := range i.moduleDiagnosticSubscriptions {
		if shouldLogDiagnostic(subscription.logLevel, level) {
			return true
		}
	}
	return false
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
	i.stateMu.Lock()
	if i.closed {
		i.stateMu.Unlock()
		return
	}

	replaceValue := false
	if len(replace) > 0 {
		replaceValue = replace[0]
	}

	if replaceValue {
		i.context = copyContext(context)
	} else {
		// Merge context
		for key, value := range context {
			i.context[key] = value
		}
	}
	i.stateGeneration.Add(1)
	updatedContext := copyContext(i.context)
	i.stateMu.Unlock()

	i.emitter.Trigger("context_set", map[string]interface{}{
		"context":  updatedContext,
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
			"context":  updatedContext,
			"replaced": replaceValue,
		},
	}, nil)
}

// GetContext returns the context
func (i *Featurevisor) GetContext(context Context) Context {
	i.stateMu.RLock()
	defer i.stateMu.RUnlock()

	// Merge contexts
	result := copyContext(i.context)
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

	return newFeaturevisorChild(childOptions{
		Parent:          i,
		Context:         i.GetContext(contextValue),
		StickyFeatures:  optionsValue.StickyFeatures,
		StickyVariables: optionsValue.StickyVariables,
	})
}

// getEvaluationDependencies gets evaluation dependencies
func (i *Featurevisor) getEvaluationDependencies(context Context, options OverrideOptions) evaluateDependencies {
	i.stateMu.RLock()
	storedContext := copyContext(i.context)
	effectiveContext := copyContext(storedContext)
	for key, value := range context {
		effectiveContext[key] = value
	}
	var stickyFeatures *StickyFeatures
	if options.stickyFeatures != nil {
		stickyFeatures = options.stickyFeatures
	} else {
		stickyFeatures = i.stickyFeatures
	}

	dependencies := evaluateDependencies{
		Context:                        effectiveContext,
		diagnosticReporter:             i.diagnostics,
		modulesManager:                 i.modulesManager,
		instanceEvaluationDataProvider: i.instanceEvaluationDataProvider,
		stickyFeatures:                 stickyFeatures,
		stickyVariables: func() *StickyVariables {
			if options.stickyVariables != nil {
				return options.stickyVariables
			}
			return i.stickyVariables
		}(),
		DefaultVariationValue:   options.DefaultVariationValue,
		DefaultVariableValue:    options.DefaultVariableValue,
		DefaultVariableValueSet: options.DefaultVariableValueSet,
	}
	dependencies.seenStateVersion = i.stateGeneration.Load()
	i.stateMu.RUnlock()
	dependencies.storedContextSnapshot = storedContext
	dependencies.refreshStickyVariables = func() *StickyVariables {
		if options.stickyVariables != nil {
			return options.stickyVariables
		}
		i.stateMu.RLock()
		defer i.stateMu.RUnlock()
		return i.stickyVariables
	}
	dependencies.stateVersion = i.stateGeneration.Load
	dependencies.refreshState = func(current *evaluateDependencies) {
		version := i.stateGeneration.Load()
		if version == current.seenStateVersion {
			return
		}
		i.stateMu.RLock()
		defer i.stateMu.RUnlock()
		version = i.stateGeneration.Load()
		newStored := copyContext(i.context)
		mergeChangedStoredContext(current.Context, current.storedContextSnapshot, newStored, context)
		current.storedContextSnapshot = newStored
		if options.stickyFeatures == nil {
			current.stickyFeatures = i.stickyFeatures
		}
		if options.stickyVariables == nil {
			current.stickyVariables = i.stickyVariables
		}
		current.instanceEvaluationDataProvider = i.instanceEvaluationDataProvider
		current.seenStateVersion = version
	}
	return dependencies
}

// EvaluateGlobalVariable evaluates an independently defined variable.
func (i *Featurevisor) EvaluateGlobalVariable(variableKey string, args ...interface{}) Evaluation {
	context, options := parseEvaluationArgs(args)
	return i.evaluateGlobalVariable(variableKey, context, options)
}

func (i *Featurevisor) evaluateGlobalVariable(variableKey string, context Context, options OverrideOptions) Evaluation {
	key := VariableKey(variableKey)
	return evaluateWithModules(EvaluateOptions{
		evaluateParams:       evaluateParams{Type: EvaluationTypeVariable, VariableKey: &key, GlobalVariable: true},
		evaluateDependencies: i.getEvaluationDependencies(context, options),
	})
}

// GetGlobalVariable gets an independently defined variable.
func (i *Featurevisor) GetGlobalVariable(variableKey string, args ...interface{}) VariableValue {
	context, options := parseEvaluationArgs(args)
	return normalizedGetterValue(i.evaluateGlobalVariable(variableKey, context, options), i.diagnostics)
}

func (i *Featurevisor) GetGlobalVariableBoolean(variableKey string, args ...interface{}) *bool {
	value, ok := i.GetGlobalVariable(variableKey, args...).(bool)
	if !ok {
		return nil
	}
	return &value
}
func (i *Featurevisor) GetGlobalVariableString(variableKey string, args ...interface{}) *string {
	value, ok := i.GetGlobalVariable(variableKey, args...).(string)
	if !ok {
		return nil
	}
	return &value
}
func (i *Featurevisor) GetGlobalVariableInteger(variableKey string, args ...interface{}) *int {
	value := i.GetGlobalVariable(variableKey, args...)
	if number, ok := value.(float64); ok {
		result := int(number)
		return &result
	}
	if number, ok := value.(int); ok {
		return &number
	}
	return nil
}
func (i *Featurevisor) GetGlobalVariableDouble(variableKey string, args ...interface{}) *float64 {
	value := i.GetGlobalVariable(variableKey, args...)
	if number, ok := value.(float64); ok {
		return &number
	}
	return nil
}
func (i *Featurevisor) GetGlobalVariableArray(variableKey string, args ...interface{}) []string {
	value := i.GetGlobalVariable(variableKey, args...)
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
func (i *Featurevisor) GetGlobalVariableObject(variableKey string, args ...interface{}) map[string]interface{} {
	value, _ := i.GetGlobalVariable(variableKey, args...).(map[string]interface{})
	return value
}
func (i *Featurevisor) GetGlobalVariableJSON(variableKey string, args ...interface{}) interface{} {
	return i.GetGlobalVariable(variableKey, args...)
}

// GetGlobalVariableArrayInto decodes an array global variable into out.
func (i *Featurevisor) GetGlobalVariableArrayInto(variableKey string, args ...interface{}) error {
	context, options, out, err := parseVariableIntoArgs(args...)
	if err != nil {
		return err
	}
	value := i.GetGlobalVariable(variableKey, context, options)
	if value == nil {
		return fmt.Errorf("global variable %q is unavailable", variableKey)
	}
	return decodeInto(value, out)
}

// GetGlobalVariableObjectInto decodes an object global variable into out.
func (i *Featurevisor) GetGlobalVariableObjectInto(variableKey string, args ...interface{}) error {
	return i.GetGlobalVariableArrayInto(variableKey, args...)
}

func parseEvaluationArgs(args []interface{}) (Context, OverrideOptions) {
	context := Context{}
	options := OverrideOptions{}
	for _, arg := range args {
		switch value := arg.(type) {
		case Context:
			context = value
		case OverrideOptions:
			options = value
		}
	}
	return context, options
}

// EvaluateFlag evaluates a feature flag.
func (i *Featurevisor) EvaluateFlag(featureKey string, args ...interface{}) Evaluation {
	context, options := parseEvaluationArgs(args)
	return i.evaluateFlag(featureKey, context, options)
}

func (i *Featurevisor) evaluateFlag(featureKey string, context Context, options OverrideOptions) Evaluation {
	return evaluateWithModules(EvaluateOptions{
		evaluateParams: evaluateParams{
			Type:       EvaluationTypeFlag,
			FeatureKey: FeatureKey(featureKey),
		},
		evaluateDependencies: i.getEvaluationDependencies(context, options),
	})
}

// IsEnabled checks if a feature is enabled
func (i *Featurevisor) IsEnabled(featureKey string, args ...interface{}) bool {
	defer func() {
		if r := recover(); r != nil {
			i.diagnostics.Error("isEnabled", logDetails{
				"featureKey": featureKey,
				"error":      r,
			})
		}
	}()

	context, options := parseEvaluationArgs(args)
	evaluation := i.evaluateFlag(featureKey, context, options)

	if evaluation.Enabled != nil {
		return *evaluation.Enabled
	}

	return false
}

// EvaluateVariation evaluates a feature variation
func (i *Featurevisor) EvaluateVariation(featureKey string, args ...interface{}) Evaluation {
	context, options := parseEvaluationArgs(args)
	return i.evaluateVariation(featureKey, context, options)
}

func (i *Featurevisor) evaluateVariation(featureKey string, context Context, options OverrideOptions) Evaluation {
	return evaluateWithModules(EvaluateOptions{
		evaluateParams: evaluateParams{
			Type:       EvaluationTypeVariation,
			FeatureKey: FeatureKey(featureKey),
		},
		evaluateDependencies: i.getEvaluationDependencies(context, options),
	})
}

// GetVariation gets a feature variation
func (i *Featurevisor) GetVariation(featureKey string, args ...interface{}) *string {
	defer func() {
		if r := recover(); r != nil {
			i.diagnostics.Error("getVariation", logDetails{
				"featureKey": featureKey,
				"error":      r,
			})
		}
	}()

	context, options := parseEvaluationArgs(args)
	evaluation := i.evaluateVariation(featureKey, context, options)

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
func (i *Featurevisor) EvaluateVariable(featureKey string, variableKey VariableKey, args ...interface{}) Evaluation {
	context, options := parseEvaluationArgs(args)
	return i.evaluateVariable(featureKey, variableKey, context, options)
}

func (i *Featurevisor) evaluateVariable(featureKey string, variableKey VariableKey, context Context, options OverrideOptions) Evaluation {
	return evaluateWithModules(EvaluateOptions{
		evaluateParams: evaluateParams{
			Type:        EvaluationTypeVariable,
			FeatureKey:  FeatureKey(featureKey),
			VariableKey: &variableKey,
		},
		evaluateDependencies: i.getEvaluationDependencies(context, options),
	})
}

// GetVariable gets a feature variable
func (i *Featurevisor) GetVariable(featureKey string, variableKey string, args ...interface{}) VariableValue {
	defer func() {
		if r := recover(); r != nil {
			i.diagnostics.Error("getVariable failed", logDetails{
				"featureKey":  featureKey,
				"variableKey": variableKey,
				"error":       r,
			})
		}
	}()

	context, options := parseEvaluationArgs(args)
	evaluation := i.evaluateVariable(featureKey, VariableKey(variableKey), context, options)
	return normalizedGetterValue(evaluation, i.diagnostics)
}

// GetVariableBoolean gets a boolean variable
func (i *Featurevisor) GetVariableBoolean(featureKey string, variableKey string, args ...interface{}) *bool {
	value := i.GetVariable(featureKey, variableKey, args...)
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
func (i *Featurevisor) GetVariableString(featureKey string, variableKey string, args ...interface{}) *string {
	value := i.GetVariable(featureKey, variableKey, args...)
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
func (i *Featurevisor) GetVariableInteger(featureKey string, variableKey string, args ...interface{}) *int {
	value := i.GetVariable(featureKey, variableKey, args...)
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
func (i *Featurevisor) GetVariableDouble(featureKey string, variableKey string, args ...interface{}) *float64 {
	value := i.GetVariable(featureKey, variableKey, args...)
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
func (i *Featurevisor) GetVariableArray(featureKey string, variableKey string, args ...interface{}) []string {
	value := i.GetVariable(featureKey, variableKey, args...)
	if value == nil {
		return nil
	}

	return toTypedArray[string](getValueByType(value, "array"))
}

// GetVariableObject gets an object variable
func (i *Featurevisor) GetVariableObject(featureKey string, variableKey string, args ...interface{}) map[string]interface{} {
	value := i.GetVariable(featureKey, variableKey, args...)
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

	arrayValue := getValueByType(value, "array")
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

	objectValue := getValueByType(value, "object")
	if objectValue == nil {
		return fmt.Errorf("variable %q is not an object", variableKey)
	}

	return decodeInto(objectValue, out)
}

// GetFeatureEvaluations evaluates a feature snapshot.
func (i *Featurevisor) GetFeatureEvaluations(context Context, featureKeys []string, options OverrideOptions) EvaluatedFeatures {
	result := EvaluatedFeatures{}

	keys := featureKeys
	if len(keys) == 0 {
		// Get all feature keys
		allKeys := i.dataProvider().GetFeatureKeys()
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
		if i.dataProvider().HasVariations(FeatureKey(featureKey)) {
			variation := i.GetVariation(featureKey, context, options)
			if variation != nil {
				evaluatedFeature.Variation = variation
			}
		}

		// variables
		variableKeys := i.dataProvider().GetVariableKeys(FeatureKey(featureKey))
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

// GetVariableEvaluations evaluates a global variable snapshot.
func (i *Featurevisor) GetVariableEvaluations(context Context, variableKeys []string, options OverrideOptions) EvaluatedVariables {
	result := EvaluatedVariables{}
	keys := variableKeys
	if len(keys) == 0 {
		keys = i.GetGlobalVariableKeys()
	}
	for _, key := range keys {
		result[key] = i.GetGlobalVariable(key, context, options)
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
	mergedVariables := map[GlobalVariableKey]GlobalVariable{}
	for key, value := range existing.Variables {
		mergedVariables[key] = value
	}
	for key, value := range incoming.Variables {
		mergedVariables[key] = value
	}

	return DatafileContent{
		SchemaVersion:       incoming.SchemaVersion,
		Revision:            incoming.Revision,
		FeaturevisorVersion: incoming.FeaturevisorVersion,
		Segments:            mergedSegments,
		Features:            mergedFeatures,
		Variables:           mergedVariables,
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
