package featurevisor

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

type dependencyError struct{ message string }

func (e dependencyError) Error() string { return e.message }

var errVariableRequirementCycle error = dependencyError{message: "Circular dependency"}

type variableRequirementInputError struct{ message string }

func (e variableRequirementInputError) Error() string { return e.message }

func evaluationPanicError(recovered interface{}) error {
	if err, ok := recovered.(error); ok {
		var dependency dependencyError
		if errors.As(err, &dependency) {
			return err
		}
	}
	if err, ok := recovered.(variableRequirementInputError); ok {
		return err
	}
	return fmt.Errorf("panic: %v", recovered)
}

// isDependencyFailure reports whether err fails the complete public call. Nested
// getters pass such failures upwards, so only the public call reports them.
func isDependencyFailure(err error) bool {
	var dependency dependencyError
	var input variableRequirementInputError
	return errors.As(err, &dependency) || errors.As(err, &input)
}

func propagateRequirementError(evaluation Evaluation) {
	if evaluation.Reason != EvaluationReasonError {
		return
	}
	var dependency dependencyError
	if errors.As(evaluation.Error, &dependency) {
		panic(dependency)
	}
	var inputError variableRequirementInputError
	if errors.As(evaluation.Error, &inputError) {
		panic(inputError)
	}
}

// evaluateParams contains parameters for evaluation
type evaluateParams struct {
	Type           EvaluationType
	FeatureKey     FeatureKey
	VariableKey    *VariableKey
	GlobalVariable bool
}

// evaluateDependencies contains dependencies for evaluation
type evaluateDependencies struct {
	Context                        Context
	DependencyOf                   *EvaluationDependency
	DependencyPurpose              string
	diagnosticReporter             *diagnosticReporter
	modulesManager                 *modulesManager
	instanceEvaluationDataProvider *instanceEvaluationDataProvider

	// Instance-internal sticky state. Consumers configure it on an instance.
	stickyFeatures          *StickyFeatures
	stickyVariables         *StickyVariables
	session                 *evaluationSession
	currentEvaluation       EvaluationDependency
	currentEvaluationGlobal bool
	requesterGlobalVariable bool
	stateVersion            func() uint64
	refreshState            func(*evaluateDependencies)
	refreshStickyVariables  func() *StickyVariables
	storedContextSnapshot   Context
	seenStateVersion        uint64

	DefaultVariationValue   *VariationValue
	DefaultVariableValue    VariableValue
	DefaultVariableValueSet bool
}

// EvaluateOptions contains all options for evaluation
type EvaluateOptions struct {
	evaluateParams
	evaluateDependencies
}

// evaluateWithModules evaluates a feature with modules.
func evaluateWithModules(opts EvaluateOptions) (evaluation Evaluation) {
	if opts.session == nil {
		opts.session = &evaluationSession{}
	}
	publicCall := opts.DependencyOf == nil
	if publicCall {
		defer func() {
			opts.session.memo = nil
			opts.session.active = nil
			opts.session.frames = nil
			opts.session.rootFrames = [2]evaluationFrameIdentity{}
			opts.session.rootFrameCount = 0
			opts.session.segments = nil
			opts.session.depth = 0
			opts.session.stickyFeatures = nil
			opts.session.stickyVariables = nil
			opts.session.contextSnapshot = nil
		}()
	}
	if opts.DependencyOf != nil && opts.refreshState != nil {
		opts.refreshState(&opts.evaluateDependencies)
	}
	originalFrame := frameIdentity(opts)
	if opts.session.depth+len(opts.session.segments) > 256 {
		return Evaluation{Type: opts.Type, FeatureKey: opts.FeatureKey, VariableKey: opts.VariableKey, Reason: EvaluationReasonError, Error: dependencyError{message: "Evaluation dependency depth exceeded"}}
	}
	if opts.session.hasFrame(originalFrame) {
		return Evaluation{Type: opts.Type, FeatureKey: opts.FeatureKey, VariableKey: opts.VariableKey,
			Reason: EvaluationReasonError, Error: errVariableRequirementCycle}
	}
	cacheKey, cacheable := opts.session.key(opts)
	var contextSnapshot Context
	if cacheable {
		if opts.session.contextSnapshot != nil && sameContext(opts.session.contextSnapshot, opts.Context) {
			contextSnapshot = opts.session.contextSnapshot
		} else if snapshot, okay := snapshotContext(opts.Context); okay {
			contextSnapshot = snapshot
			opts.session.contextSnapshot = snapshot
		}
	}
	startVersion := cacheKey.version
	if !cacheable && opts.stateVersion != nil {
		startVersion = opts.stateVersion()
	}
	if contextSnapshot != nil && opts.session.memo != nil {
		for _, cached := range opts.session.memo[cacheKey] {
			if sameContext(cached.context, opts.Context) {
				// The snapshot is private to prerequisite comparisons.
				return cached.value
			}
		}
	}
	rootFrame := opts.session.depth == 0
	if rootFrame {
		opts.session.rootFrames[0] = originalFrame
		opts.session.rootFrameCount = 1
	} else {
		if opts.session.frames == nil {
			opts.session.frames = make(map[evaluationFrameIdentity]int)
		}
		opts.session.frames[originalFrame]++
	}
	opts.session.depth++
	defer func() {
		opts.session.depth--
		if rootFrame {
			opts.session.rootFrames = [2]evaluationFrameIdentity{}
			opts.session.rootFrameCount = 0
			return
		}
		opts.session.frames[originalFrame]--
		if opts.session.frames[originalFrame] == 0 {
			delete(opts.session.frames, originalFrame)
		}
	}()
	defer func() {
		if r := recover(); r != nil {
			// Return error evaluation when panic occurs (matching TypeScript behavior)
			evaluation = Evaluation{
				Type:        opts.Type,
				FeatureKey:  opts.FeatureKey,
				VariableKey: opts.VariableKey,
				Reason:      EvaluationReasonError,
				Error:       evaluationPanicError(r),
			}
			if !isDependencyFailure(evaluation.Error) {
				opts.diagnosticReporter.Error("panic during evaluation", logDetails{
					"error": r,
				})
			}
		}
		if publicCall && evaluation.Reason == EvaluationReasonError && isDependencyFailure(evaluation.Error) {
			opts.diagnosticReporter.Error("panic during evaluation", logDetails{
				"error": evaluation.Error,
			})
		}
		var inputError variableRequirementInputError
		var dependency dependencyError
		stateStable := opts.stateVersion == nil || opts.stateVersion() == startVersion
		if cacheable && contextSnapshot != nil && stateStable && sameContext(contextSnapshot, opts.Context) && !errors.As(evaluation.Error, &dependency) && !errors.As(evaluation.Error, &inputError) {
			if opts.session.memo == nil {
				opts.session.memo = make(map[evaluationMemoKey][]evaluationMemoEntry)
			}
			if snapshot, okay := memoDependencyEvaluation(evaluation, opts.DependencyPurpose == "composition"); okay {
				opts.session.memo[cacheKey] = append(opts.session.memo[cacheKey], evaluationMemoEntry{context: contextSnapshot, value: snapshot})
			}
		}
	}()

	modulesManager := opts.modulesManager
	modules := modulesManager.GetAll()

	options := opts
	for _, module := range modules {
		if module.BeforeEvaluation != nil {
			previous := options
			options = module.BeforeEvaluation(publicModuleOptions(options))
			restoreModuleInternals(&options, previous)
		}
	}
	transformedFrame := frameIdentity(options)
	if transformedFrame != originalFrame {
		if rootFrame {
			opts.session.rootFrames[1] = transformedFrame
			opts.session.rootFrameCount = 2
		} else {
			opts.session.frames[transformedFrame]++
			defer func() {
				opts.session.frames[transformedFrame]--
				if opts.session.frames[transformedFrame] == 0 {
					delete(opts.session.frames, transformedFrame)
				}
			}()
		}
	}
	if options.refreshState != nil {
		if options.DependencyOf != nil {
			options.refreshState(&options.evaluateDependencies)
		} else if options.GlobalVariable && options.refreshStickyVariables != nil {
			options.stickyVariables = options.refreshStickyVariables()
		}
	}
	options.currentEvaluation = *dependencyOf(options)
	options.currentEvaluationGlobal = options.GlobalVariable

	// evaluate
	if options.GlobalVariable {
		evaluation = evaluateGlobalVariable(options)
	} else {
		evaluation = evaluate(options)
	}

	// default: variation
	if options.DefaultVariationValue != nil &&
		evaluation.Type == EvaluationTypeVariation &&
		evaluation.VariationValue == nil {
		evaluation.VariationValue = options.DefaultVariationValue
	}

	// default: variable
	if options.DefaultVariableValueSet &&
		evaluation.Type == EvaluationTypeVariable &&
		!evaluation.variableValueSet && evaluation.VariableValue == nil {
		evaluation.VariableValue = options.DefaultVariableValue
		evaluation.variableValueSet = true
	}

	// run after modules
	for _, module := range modulesManager.GetAll() {
		if module.AfterEvaluation != nil {
			evaluation = module.AfterEvaluation(evaluation, publicModuleOptions(options))
		}
	}
	if options.DependencyOf != nil && evaluation.Type == EvaluationTypeVariable {
		evaluation.VariableValue = normalizedGetterValue(evaluation, options.diagnosticReporter)
		evaluation.getterValueNormalized = true
	}
	return evaluation
}

func cleanRequiredFeatureDependencies(dependencies evaluateDependencies) evaluateDependencies {
	if dependencies.refreshState != nil {
		dependencies.refreshState(&dependencies)
	}
	dependencies.DefaultVariationValue = nil
	dependencies.DefaultVariableValue = nil
	dependencies.DefaultVariableValueSet = false
	dependencies.DependencyOf = &dependencies.currentEvaluation
	dependencies.DependencyPurpose = "condition"
	dependencies.requesterGlobalVariable = dependencies.currentEvaluationGlobal
	return dependencies
}

func requiredFeatureParts(required RequiredFeature) (FeatureKey, bool, *VariationValue, bool) {
	switch value := required.(type) {
	case string:
		return FeatureKey(value), true, nil, true
	case RequiredFeatureOptions:
		expected := true
		if value.Enabled != nil {
			expected = *value.Enabled
		}
		return value.Feature, expected, value.Variation, value.Feature != ""
	case map[string]interface{}:
		if feature, ok := value["feature"].(string); ok {
			expected := true
			if enabled, ok := value["enabled"].(bool); ok {
				expected = enabled
			}
			var variation *VariationValue
			if raw, ok := value["variation"].(string); ok {
				parsed := VariationValue(raw)
				variation = &parsed
			}
			return FeatureKey(feature), expected, variation, feature != ""
		}
	}
	return "", true, nil, false
}

func groupValues(value interface{}, key string) ([]interface{}, bool) {
	group, ok := value.(map[string]interface{})
	if !ok {
		return nil, false
	}
	values, ok := group[key].([]interface{})
	return values, ok
}

func requiredFeaturesAreMatched(requiredFeatures []RequiredFeature, dependencies evaluateDependencies) bool {
	dependencies = cleanRequiredFeatureDependencies(dependencies)
	var match func(RequiredFeature) bool
	match = func(required RequiredFeature) bool {
		if values, ok := groupValues(required, "and"); ok {
			for _, value := range values {
				if !match(value) {
					return false
				}
			}
			return true
		}
		if values, ok := groupValues(required, "or"); ok {
			for _, value := range values {
				if match(value) {
					return true
				}
			}
			return false
		}
		if values, ok := groupValues(required, "not"); ok {
			if len(values) == 0 {
				return false
			}
			for _, value := range values {
				if !match(value) {
					return true
				}
			}
			return false
		}
		featureKey, expectedEnabled, expectedVariation, ok := requiredFeatureParts(required)
		if !ok {
			return false
		}
		flag := evaluateWithModules(EvaluateOptions{
			evaluateParams:       evaluateParams{Type: EvaluationTypeFlag, FeatureKey: featureKey},
			evaluateDependencies: dependencies,
		})
		propagateRequirementError(flag)
		if (flag.Enabled != nil && *flag.Enabled) != expectedEnabled {
			return false
		}
		if expectedVariation != nil {
			variation := evaluateWithModules(EvaluateOptions{
				evaluateParams:       evaluateParams{Type: EvaluationTypeVariation, FeatureKey: featureKey},
				evaluateDependencies: dependencies,
			})
			propagateRequirementError(variation)
			var actual *VariationValue
			if variation.VariationValue != nil {
				actual = variation.VariationValue
			} else if variation.Variation != nil {
				value := variation.Variation.Value
				actual = &value
			}
			if actual == nil || *actual != *expectedVariation {
				return false
			}
		}
		return true
	}
	for _, required := range requiredFeatures {
		if !match(required) {
			return false
		}
	}
	return true
}

func variableOverrideIsMatched(override VariableOverride, dependencies evaluateDependencies) bool {
	return override.Conditions != nil && conditionsAreMatched(override.Conditions, dependencies)
}

type variableRequirementIdentity struct {
	Feature  string
	Variable string
}

func requiredVariableParts(required RequiredVariable) (RequiredVariableOptions, bool) {
	if value, ok := required.(RequiredVariableOptions); ok {
		return value, value.Variable != ""
	}
	value, ok := required.(map[string]interface{})
	if !ok {
		return RequiredVariableOptions{}, false
	}
	if _, isGroup := value["and"]; isGroup {
		return RequiredVariableOptions{}, false
	}
	if _, isGroup := value["or"]; isGroup {
		return RequiredVariableOptions{}, false
	}
	if _, isGroup := value["not"]; isGroup {
		return RequiredVariableOptions{}, false
	}
	key, ok := value["variable"].(string)
	if !ok || key == "" {
		return RequiredVariableOptions{}, false
	}
	operator, _ := value["operator"].(string)
	parsed := RequiredVariableOptions{Variable: key, Operator: Operator(operator), Path: value["path"], Value: value["value"]}
	_, parsed.valueSet = value["value"]
	if feature, ok := value["feature"].(string); ok {
		parsed.Feature = &feature
	}
	if flags, ok := value["regexFlags"].(string); ok {
		parsed.RegexFlags = &flags
	}
	return parsed, true
}

func normalizedGetterValue(evaluation Evaluation, diagnostics *diagnosticReporter) VariableValue {
	if evaluation.getterValueNormalized {
		return evaluation.VariableValue
	}
	if !evaluation.variableValueSet && evaluation.VariableValue == nil {
		return nil
	}
	value := evaluation.VariableValue
	variableType := VariableType("")
	if evaluation.GlobalVariable != nil {
		variableType = evaluation.GlobalVariable.Type
	}
	if evaluation.VariableSchema != nil {
		variableType = evaluation.VariableSchema.Type
	}
	if variableType == VariableTypeJSON && (evaluation.GlobalVariable == nil || evaluation.GlobalVariable.Compose == nil) {
		raw, ok := value.(string)
		if !ok {
			return value
		}
		var parsed interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			if diagnostics != nil {
				details := logDetails{
					"variableKey": stringValue(evaluation.VariableKey),
					"error":       err,
				}
				if evaluation.FeatureKey != "" {
					details["featureKey"] = evaluation.FeatureKey
				}
				diagnostics.Error("getVariable failed", details)
			}
			return nil
		}
		return parsed
	}
	if evaluation.Reason == EvaluationReasonVariableDefault {
		return getValueByType(value, string(variableType))
	}
	return value
}

func evaluateRequiredVariable(required RequiredVariableOptions, dependencies evaluateDependencies) VariableValue {
	if dependencies.session == nil {
		dependencies.session = &evaluationSession{}
	}
	if dependencies.session.active == nil {
		dependencies.session.active = map[variableRequirementIdentity]bool{}
	}
	feature := ""
	if required.Feature != nil {
		feature = *required.Feature
	}
	identity := variableRequirementIdentity{Feature: feature, Variable: required.Variable}
	if dependencies.session.active[identity] {
		panic(errVariableRequirementCycle)
	}
	dependencies.session.active[identity] = true
	defer delete(dependencies.session.active, identity)
	clean := cleanRequiredFeatureDependencies(dependencies)
	key := VariableKey(required.Variable)
	params := evaluateParams{Type: EvaluationTypeVariable, VariableKey: &key}
	if required.Feature == nil {
		params.GlobalVariable = true
	} else {
		params.FeatureKey = *required.Feature
	}
	evaluation := evaluateWithModules(EvaluateOptions{evaluateParams: params, evaluateDependencies: clean})
	propagateRequirementError(evaluation)
	return normalizedGetterValue(evaluation, dependencies.diagnosticReporter)
}

func requiredVariablesAreMatched(requirements []RequiredVariable, dependencies evaluateDependencies) bool {
	var match func(RequiredVariable) bool
	match = func(required RequiredVariable) bool {
		if values, ok := groupValues(required, "and"); ok {
			for _, value := range values {
				if !match(value) {
					return false
				}
			}
			return true
		}
		if values, ok := groupValues(required, "or"); ok {
			for _, value := range values {
				if match(value) {
					return true
				}
			}
			return false
		}
		if values, ok := groupValues(required, "not"); ok {
			if len(values) == 0 {
				return false
			}
			for _, value := range values {
				if !match(value) {
					return true
				}
			}
			return false
		}
		leaf, ok := requiredVariableParts(required)
		if !ok {
			return false
		}
		value := evaluateRequiredVariable(leaf, dependencies)
		exists := true
		path, pathOK := leaf.Path.([]interface{})
		if leaf.Path != nil && !pathOK {
			panic(variableRequirementInputError{message: "Variable requirement path must be compiled"})
		}
		for _, part := range path {
			if key, ok := part.(string); ok {
				if unsafeMutationKeys[key] {
					panic(variableRequirementInputError{message: "Unsafe variable requirement path"})
				}
				if objectValue, objectOK := variableObjectEntry(value, key); objectOK {
					value = objectValue
					exists = true
					if !exists {
						break
					}
					continue
				}
				if values, arrayOK := variableArrayValues(value); arrayOK {
					if key == "length" {
						value = len(values)
						continue
					}
					index, err := strconv.ParseUint(key, 10, 64)
					if err == nil && strconv.FormatUint(index, 10) == key && index < uint64(len(values)) {
						value = values[index]
						continue
					}
				}
				exists = false
				value = nil
				break
			}
			index, indexOK := mutationIndex(part)
			values, arrayOK := variableArrayValues(value)
			if !indexOK || !arrayOK || index >= len(values) {
				exists = false
				value = nil
				break
			}
			value = values[index]
		}
		conditionValue := (*ConditionValue)(nil)
		if leaf.valueSet || leaf.Value != nil || (leaf.Operator != OperatorExists && leaf.Operator != OperatorNotExists && leaf.Operator != OperatorIsNull && leaf.Operator != OperatorNotNull) {
			conditionValue = &leaf.Value
		}
		context := Context{}
		if exists {
			context["value"] = value
		}
		return conditionIsMatched(PlainCondition{Attribute: "value", Operator: leaf.Operator, Value: conditionValue, RegexFlags: leaf.RegexFlags}, context, dependencies.instanceEvaluationDataProvider.getRegex)
	}
	for _, required := range requirements {
		if !match(required) {
			return false
		}
	}
	return true
}

func variableValueFromPointer(value *VariableValue) VariableValue {
	if value == nil {
		return nil
	}
	return *value
}

func evaluateGlobalVariable(options EvaluateOptions) Evaluation {
	key := GlobalVariableKey("")
	if options.VariableKey != nil {
		key = GlobalVariableKey(*options.VariableKey)
	}
	variable := options.instanceEvaluationDataProvider.GetGlobalVariable(key)
	base := Evaluation{Type: EvaluationTypeVariable, VariableKey: options.VariableKey, Reason: EvaluationReasonVariableNotFound}
	if options.stickyVariables != nil {
		if value, ok := (*options.stickyVariables)[key]; ok {
			base.Reason = EvaluationReasonSticky
			base.GlobalVariable = variable
			base.VariableValue = value
			base.variableValueSet = true
			return base
		}
	}
	if variable == nil {
		return base
	}
	base.GlobalVariable = variable
	if variable.Deprecated != nil && *variable.Deprecated {
		options.diagnosticReporter.Warn("variable is deprecated", logDetails{"variableKey": key})
	}
	if variable.Conditions != nil && !conditionsAreMatched(variable.Conditions, options.evaluateDependencies) {
		base.Reason = EvaluationReasonConditionsUnmet
		base.Conditions = variable.Conditions
		base.variableValueSet = variable.Compose != nil
		if variable.Compose == nil && variable.UseDefaultWhenDisabled {
			base.VariableValue = variable.DefaultValue
			base.variableValueSet = variable.defaultValueSet || variable.DefaultValue != nil
		} else if variable.disabledValueSet || variable.DisabledValue != nil {
			base.VariableValue = freshComposedValue(variable, variable.DisabledValue)
			base.variableValueSet = true
		}
		return base
	}
	initialValue := variable.DefaultValue
	if variable.Compose != nil {
		initialValue = composeValue(variable.Compose, options.evaluateDependencies)
	}
	if variable.OverrideProgram != nil {
		value := initialValue
		matchedPaths := [][]string{}
		for index := 0; index < len(variable.OverrideProgram); {
			step := variable.OverrideProgram[index]
			override := VariableOverride{Key: &step.Key, KeyPath: step.KeyPath, Conditions: step.Conditions}
			matched := variableOverrideIsMatched(override, options.evaluateDependencies)
			if matched {
				next, err := ApplyVariableOperations(value, step.Operations)
				if err != nil {
					panic(err)
				}
				value = next
				path := step.KeyPath
				if len(path) == 0 {
					path = []string{step.Key}
				}
				matchedPaths = append(matchedPaths, append([]string{}, path...))
				base.Reason = EvaluationReasonVariableOverrideRule
				base.VariableValue = freshComposedValue(variable, value)
				base.variableValueSet = true
				matchedIndex := index
				base.VariableOverrideIndex = &matchedIndex
				base.VariableOverrideKey = &step.Key
				base.VariableOverridePath = step.KeyPath
				base.VariableOverridePaths = append([][]string{}, matchedPaths...)
			}
			nextValue := step.OnMiss
			if matched {
				nextValue = step.OnMatch
			}
			next, validNext := mutationIndex(nextValue)
			if !validNext || next <= index || next > len(variable.OverrideProgram) {
				panic("Invalid variable override program jump")
			}
			index = next
		}
		if base.Reason == EvaluationReasonVariableOverrideRule {
			return base
		}
	} else {
		for index, override := range variable.Overrides {
			if variableOverrideIsMatched(override, options.evaluateDependencies) {
				base.Reason = EvaluationReasonVariableOverrideRule
				base.VariableValue = freshComposedValue(variable, override.Value)
				base.variableValueSet = true
				base.VariableOverrideIndex = &index
				base.VariableOverrideKey = override.Key
				base.VariableOverridePath = override.KeyPath
				return base
			}
		}
	}
	base.Reason = EvaluationReasonVariableDefault
	if variable.Compose != nil {
		base.Reason = EvaluationReasonVariableComposed
	}
	base.VariableValue = initialValue
	base.variableValueSet = variable.Compose != nil || variable.defaultValueSet || variable.DefaultValue != nil
	return base
}

// evaluate evaluates a feature
func evaluate(options EvaluateOptions) (evaluation Evaluation) {
	defer func() {
		if r := recover(); r != nil {
			// Return an error evaluation. evaluateWithModules reports dependency
			// failures once, for the public call.
			evaluation = Evaluation{
				Type:        options.Type,
				FeatureKey:  options.FeatureKey,
				VariableKey: options.VariableKey,
				Reason:      EvaluationReasonError,
				Error:       evaluationPanicError(r),
			}
			if !isDependencyFailure(evaluation.Error) {
				options.diagnosticReporter.Error("panic in evaluate", logDetails{
					"panic": r,
				})
			}
		}
	}()

	// feature not found
	feature := options.instanceEvaluationDataProvider.getFeatureForEvaluation(options.FeatureKey)
	if feature == nil {
		evaluation = Evaluation{
			Type:       options.Type,
			FeatureKey: options.FeatureKey,
			Reason:     EvaluationReasonFeatureNotFound,
		}

		options.diagnosticReporter.Warn("feature not found", logDetails{"evaluation": evaluation})

		return evaluation
	}

	// feature: deprecated
	if options.Type == EvaluationTypeFlag && feature.Deprecated != nil && *feature.Deprecated {
		options.diagnosticReporter.Warn("feature is deprecated", logDetails{
			"featureKey": options.FeatureKey,
		})
	}

	// Resolve disabled values before sticky payloads and variation availability,
	// matching the JavaScript evaluation order.
	/**
	 * Root flag evaluation
	 */
	var flag Evaluation
	if options.Type != EvaluationTypeFlag {
		// needed by variation and variable evaluations
		flag = evaluate(EvaluateOptions{
			evaluateParams: evaluateParams{
				Type:       EvaluationTypeFlag,
				FeatureKey: options.FeatureKey,
			},
			evaluateDependencies: options.evaluateDependencies,
		})
		propagateRequirementError(flag)

		if flag.Enabled == nil || !*flag.Enabled {
			evaluation = Evaluation{
				Type:       options.Type,
				FeatureKey: options.FeatureKey,
				Reason:     EvaluationReasonDisabled,
			}

			// serve variable default value if feature is disabled (if explicitly specified)
			if options.Type == EvaluationTypeVariable {
				if feature != nil && options.VariableKey != nil && feature.VariablesSchema != nil {
					if variableSchema, exists := feature.VariablesSchema[*options.VariableKey]; exists {
						if variableSchema.disabledValueSet {
							// disabledValue: <value>
							evaluation = Evaluation{
								Type:           options.Type,
								FeatureKey:     options.FeatureKey,
								Reason:         EvaluationReasonVariableDisabled,
								VariableKey:    options.VariableKey,
								VariableValue:  variableValueFromPointer(variableSchema.DisabledValue),
								VariableSchema: &variableSchema,
								Enabled:        &[]bool{false}[0],
							}
							evaluation.variableValueSet = true
						} else if variableSchema.UseDefaultWhenDisabled != nil && *variableSchema.UseDefaultWhenDisabled {
							// useDefaultWhenDisabled: true
							evaluation = Evaluation{
								Type:           options.Type,
								FeatureKey:     options.FeatureKey,
								Reason:         EvaluationReasonVariableDefault,
								VariableKey:    options.VariableKey,
								VariableValue:  variableSchema.DefaultValue,
								VariableSchema: &variableSchema,
								Enabled:        &[]bool{false}[0],
							}
							evaluation.variableValueSet = variableSchema.defaultValueSet
						}
					}
				}
			}

			// serve disabled variation value if feature is disabled (if explicitly specified)
			if options.Type == EvaluationTypeVariation && feature != nil && feature.DisabledVariationValue != nil {
				evaluation = Evaluation{
					Type:           options.Type,
					FeatureKey:     options.FeatureKey,
					Reason:         EvaluationReasonVariationDisabled,
					VariationValue: feature.DisabledVariationValue,
					Enabled:        &[]bool{false}[0],
				}
			}

			options.diagnosticReporter.debugEvaluation("feature is disabled", evaluation)

			return evaluation
		}
	}

	/**
	 * Sticky
	 */
	if options.stickyFeatures != nil {
		if stickyFeature, exists := (*options.stickyFeatures)[options.FeatureKey]; exists {
			// flag
			if options.Type == EvaluationTypeFlag {
				enabled := stickyFeature.Enabled
				evaluation = Evaluation{
					Type:          options.Type,
					FeatureKey:    options.FeatureKey,
					Reason:        EvaluationReasonSticky,
					StickyFeature: &stickyFeature,
					Enabled:       &enabled,
				}

				options.diagnosticReporter.debugEvaluation("using sticky feature", evaluation)

				return evaluation
			}

			// variation
			if options.Type == EvaluationTypeVariation && stickyFeature.Variation != nil {
				variationValue := *stickyFeature.Variation
				evaluation = Evaluation{
					Type:           options.Type,
					FeatureKey:     options.FeatureKey,
					Reason:         EvaluationReasonSticky,
					StickyFeature:  &stickyFeature,
					VariationValue: &variationValue,
				}

				options.diagnosticReporter.debugEvaluation("using sticky variation", evaluation)

				return evaluation
			}

			// variable
			if options.Type == EvaluationTypeVariable && options.VariableKey != nil && stickyFeature.Variables != nil {
				if variableValue, exists := stickyFeature.Variables[*options.VariableKey]; exists {
					evaluation = Evaluation{
						Type:          options.Type,
						FeatureKey:    options.FeatureKey,
						Reason:        EvaluationReasonSticky,
						StickyFeature: &stickyFeature,
						VariableKey:   options.VariableKey,
						VariableValue: variableValue,
					}
					evaluation.variableValueSet = true

					options.diagnosticReporter.debugEvaluation("using sticky variable", evaluation)

					return evaluation
				}
			}
		}
	}

	// variableSchema
	var variableSchema *VariableSchema

	if options.VariableKey != nil {
		if feature.VariablesSchema != nil {
			if schema, exists := feature.VariablesSchema[*options.VariableKey]; exists {
				variableSchema = &schema
			}
		}

		// variable schema not found
		if variableSchema == nil {
			evaluation = Evaluation{
				Type:        options.Type,
				FeatureKey:  options.FeatureKey,
				Reason:      EvaluationReasonVariableNotFound,
				VariableKey: options.VariableKey,
			}

			options.diagnosticReporter.Warn("variable schema not found", logDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}

		if variableSchema.Deprecated != nil && *variableSchema.Deprecated {
			options.diagnosticReporter.Warn("variable is deprecated", logDetails{
				"featureKey":  options.FeatureKey,
				"variableKey": *options.VariableKey,
			})
		}
	}

	// variation: no variations
	if options.Type == EvaluationTypeVariation && (feature.Variations == nil || len(feature.Variations) == 0) {
		evaluation = Evaluation{
			Type:       options.Type,
			FeatureKey: options.FeatureKey,
			Reason:     EvaluationReasonNoVariations,
		}

		options.diagnosticReporter.Warn("no variations", logDetails{
			"evaluation": evaluation,
		})

		return evaluation
	}

	/**
	 * Forced
	 */
	forceResult := matchedForce(feature, options.evaluateDependencies)

	if forceResult.Force != nil {
		force := forceResult.Force
		forceIndex := forceResult.ForceIndex

		// flag
		if options.Type == EvaluationTypeFlag && force.Enabled != nil {
			evaluation = Evaluation{
				Type:       options.Type,
				FeatureKey: options.FeatureKey,
				Reason:     EvaluationReasonForced,
				ForceIndex: forceIndex,
				Force:      force,
				Enabled:    force.Enabled,
			}

			options.diagnosticReporter.debugEvaluation("forced enabled found", evaluation)

			return evaluation
		}

		// variation
		if options.Type == EvaluationTypeVariation && force.Variation != nil && feature.Variations != nil {
			for _, variation := range feature.Variations {
				if variation.Value == *force.Variation {
					evaluation = Evaluation{
						Type:           options.Type,
						FeatureKey:     options.FeatureKey,
						Reason:         EvaluationReasonForced,
						ForceIndex:     forceIndex,
						Force:          force,
						Variation:      &variation,
						VariationValue: &variation.Value,
					}

					options.diagnosticReporter.debugEvaluation("forced variation found", evaluation)

					return evaluation
				}
			}
		}

		// variable
		if options.VariableKey != nil && force.Variables != nil {
			if variableValue, exists := force.Variables[string(*options.VariableKey)]; exists {
				evaluation = Evaluation{
					Type:           options.Type,
					FeatureKey:     options.FeatureKey,
					Reason:         EvaluationReasonForced,
					ForceIndex:     forceIndex,
					Force:          force,
					VariableKey:    options.VariableKey,
					VariableSchema: variableSchema,
					VariableValue:  variableValue,
				}
				evaluation.variableValueSet = true

				options.diagnosticReporter.debugEvaluation("forced variable", evaluation)

				return evaluation
			}
		}
	}

	if options.Type == EvaluationTypeFlag && feature.Conditions != nil && !conditionsAreMatched(feature.Conditions, options.evaluateDependencies) {
		evaluation = Evaluation{Type: options.Type, FeatureKey: options.FeatureKey, Reason: EvaluationReasonConditionsUnmet, Enabled: &[]bool{false}[0], Conditions: feature.Conditions}
		options.diagnosticReporter.debugEvaluation("root conditions did not match", evaluation)
		return evaluation
	}

	/**
	 * Bucketing
	 */
	// bucketKey
	bucketKey := getBucketKey(getBucketKeyOptions{
		FeatureKey:         options.FeatureKey,
		BucketBy:           feature.BucketBy,
		Context:            options.Context,
		diagnosticReporter: options.diagnosticReporter,
	})

	for _, module := range options.modulesManager.GetAll() {
		if module.BucketKey != nil {
			bucketKey = module.BucketKey(ConfigureBucketKeyOptions{
				FeatureKey: options.FeatureKey,
				Context:    options.Context,
				BucketBy:   feature.BucketBy,
				BucketKey:  bucketKey,
			})
		}
	}

	// bucketValue
	bucketValue := getBucketedNumber(bucketKey)

	for _, module := range options.modulesManager.GetAll() {
		if module.BucketValue != nil {
			bucketValue = module.BucketValue(ConfigureBucketValueOptions{
				FeatureKey:  options.FeatureKey,
				BucketKey:   bucketKey,
				Context:     options.Context,
				BucketValue: bucketValue,
			})
		}
	}

	var matchedTraffic *Traffic
	var matchedAllocation *Allocation

	if options.Type != EvaluationTypeFlag {
		matchedTraffic = matchedTrafficForEvaluation(feature.Traffic, options.evaluateDependencies)

		if matchedTraffic != nil {
			matchedAllocation = options.instanceEvaluationDataProvider.GetMatchedAllocation(matchedTraffic, bucketValue)
		}
	} else {
		matchedTraffic = matchedTrafficForEvaluation(feature.Traffic, options.evaluateDependencies)
	}

	if matchedTraffic != nil {
		// percentage: 0
		if matchedTraffic.Percentage == 0 {
			evaluation = Evaluation{
				Type:        options.Type,
				FeatureKey:  options.FeatureKey,
				Reason:      EvaluationReasonRule,
				BucketKey:   &bucketKey,
				BucketValue: &bucketValue,
				RuleKey:     &matchedTraffic.Key,
				Traffic:     matchedTraffic,
				Enabled:     &[]bool{false}[0],
			}

			options.diagnosticReporter.debugEvaluation("matched rule with 0 percentage", evaluation)

			return evaluation
		}

		// flag
		if options.Type == EvaluationTypeFlag {
			// flag: override from rule
			if matchedTraffic.Enabled != nil {
				evaluation = Evaluation{
					Type:        options.Type,
					FeatureKey:  options.FeatureKey,
					Reason:      EvaluationReasonRule,
					BucketKey:   &bucketKey,
					BucketValue: &bucketValue,
					RuleKey:     &matchedTraffic.Key,
					Traffic:     matchedTraffic,
					Enabled:     matchedTraffic.Enabled,
				}

				options.diagnosticReporter.debugEvaluation("override from rule", evaluation)

				return evaluation
			}

			// treated as enabled because of matched traffic
			if bucketValue <= matchedTraffic.Percentage {
				evaluation = Evaluation{
					Type:        options.Type,
					FeatureKey:  options.FeatureKey,
					Reason:      EvaluationReasonRule,
					BucketKey:   &bucketKey,
					BucketValue: &bucketValue,
					RuleKey:     &matchedTraffic.Key,
					Traffic:     matchedTraffic,
					Enabled:     &[]bool{true}[0],
				}

				options.diagnosticReporter.debugEvaluation("matched traffic", evaluation)

				return evaluation
			}
		}

		// variation
		if options.Type == EvaluationTypeVariation && feature.Variations != nil {
			// override from rule
			if matchedTraffic.Variation != nil {
				for _, variation := range feature.Variations {
					if variation.Value == *matchedTraffic.Variation {
						evaluation = Evaluation{
							Type:           options.Type,
							FeatureKey:     options.FeatureKey,
							Reason:         EvaluationReasonRule,
							BucketKey:      &bucketKey,
							BucketValue:    &bucketValue,
							RuleKey:        &matchedTraffic.Key,
							Traffic:        matchedTraffic,
							Variation:      &variation,
							VariationValue: &variation.Value,
						}

						options.diagnosticReporter.debugEvaluation("override from rule", evaluation)

						return evaluation
					}
				}
			}

			// Handle variationWeights
			if matchedTraffic.VariationWeights != nil && len(matchedTraffic.VariationWeights) > 0 {
				// Create custom allocation based on variationWeights
				totalWeight := 0
				for _, weight := range matchedTraffic.VariationWeights {
					totalWeight += int(weight)
				}

				if totalWeight > 0 {
					// Calculate which variation the bucket value falls into
					currentWeight := 0

					// Sort variation weights to ensure consistent processing order
					// This matches TypeScript's deterministic object property order
					type variationWeight struct {
						value  VariationValue
						weight int
					}
					var sortedWeights []variationWeight
					for variationValue, weight := range matchedTraffic.VariationWeights {
						sortedWeights = append(sortedWeights, variationWeight{
							value:  variationValue,
							weight: int(weight),
						})
					}

					// Sort by variation value to ensure consistent order
					sort.Slice(sortedWeights, func(i, j int) bool {
						return string(sortedWeights[i].value) < string(sortedWeights[j].value)
					})

					for _, vw := range sortedWeights {
						weightInt := vw.weight
						// Convert percentage to bucket range (0-100000)
						startRange := currentWeight * 100000 / totalWeight
						endRange := (currentWeight + weightInt) * 100000 / totalWeight

						options.diagnosticReporter.Debug("checking variation weight range", logDetails{
							"variationValue": vw.value,
							"weight":         weightInt,
							"startRange":     startRange,
							"endRange":       endRange,
							"bucketValue":    bucketValue,
							"totalWeight":    totalWeight,
						})

						if bucketValue >= startRange && bucketValue < endRange {
							// Find the variation object
							for _, variation := range feature.Variations {
								if variation.Value == vw.value {
									evaluation = Evaluation{
										Type:           options.Type,
										FeatureKey:     options.FeatureKey,
										Reason:         EvaluationReasonAllocated,
										BucketKey:      &bucketKey,
										BucketValue:    &bucketValue,
										RuleKey:        &matchedTraffic.Key,
										Traffic:        matchedTraffic,
										Variation:      &variation,
										VariationValue: &variation.Value,
									}

									options.diagnosticReporter.debugEvaluation("allocated variation with custom weights", evaluation)

									return evaluation
								}
							}
						}
						currentWeight += weightInt
					}
				}
			}

			// regular allocation
			if matchedAllocation != nil && matchedAllocation.Variation != "" {
				for _, variation := range feature.Variations {
					if variation.Value == matchedAllocation.Variation {
						evaluation = Evaluation{
							Type:           options.Type,
							FeatureKey:     options.FeatureKey,
							Reason:         EvaluationReasonAllocated,
							BucketKey:      &bucketKey,
							BucketValue:    &bucketValue,
							RuleKey:        &matchedTraffic.Key,
							Traffic:        matchedTraffic,
							Variation:      &variation,
							VariationValue: &variation.Value,
						}

						options.diagnosticReporter.debugEvaluation("allocated variation", evaluation)

						return evaluation
					}
				}
			}
		}
	}

	// variable
	if options.Type == EvaluationTypeVariable && options.VariableKey != nil {
		// Check if variableSchema is available
		if variableSchema == nil {
			evaluation = Evaluation{
				Type:        options.Type,
				FeatureKey:  options.FeatureKey,
				Reason:      EvaluationReasonVariableNotFound,
				VariableKey: options.VariableKey,
				BucketKey:   &bucketKey,
				BucketValue: &bucketValue,
			}

			options.diagnosticReporter.debugEvaluation("variable schema not found", evaluation)

			return evaluation
		}

		// override from rule
		if matchedTraffic != nil {
			if matchedTraffic.VariableOverrides != nil {
				if overrides, exists := matchedTraffic.VariableOverrides[*options.VariableKey]; exists {
					for index, override := range overrides {
						if variableOverrideIsMatched(override, options.evaluateDependencies) {
							overrideIndex := index
							evaluation = Evaluation{
								Type:                  options.Type,
								FeatureKey:            options.FeatureKey,
								Reason:                EvaluationReasonVariableOverrideRule,
								BucketKey:             &bucketKey,
								BucketValue:           &bucketValue,
								RuleKey:               &matchedTraffic.Key,
								Traffic:               matchedTraffic,
								VariableKey:           options.VariableKey,
								VariableSchema:        variableSchema,
								VariableValue:         override.Value,
								VariableOverrideIndex: &overrideIndex,
								VariableOverrideKey:   override.Key,
								VariableOverridePath:  override.KeyPath,
							}
							evaluation.variableValueSet = true

							options.diagnosticReporter.debugEvaluation("variable override from rule", evaluation)

							return evaluation
						}
					}
				}
			}

			if matchedTraffic.Variables != nil {
				if variableValue, exists := matchedTraffic.Variables[string(*options.VariableKey)]; exists {
					evaluation = Evaluation{
						Type:           options.Type,
						FeatureKey:     options.FeatureKey,
						Reason:         EvaluationReasonRule,
						BucketKey:      &bucketKey,
						BucketValue:    &bucketValue,
						RuleKey:        &matchedTraffic.Key,
						Traffic:        matchedTraffic,
						VariableKey:    options.VariableKey,
						VariableSchema: variableSchema,
						VariableValue:  variableValue,
					}
					evaluation.variableValueSet = true

					options.diagnosticReporter.debugEvaluation("override from rule", evaluation)

					return evaluation
				}
			}
		}

		// check variations
		var variationValue *VariationValue

		if forceResult.Force != nil && forceResult.Force.Variation != nil {
			variationValue = forceResult.Force.Variation
		} else if matchedTraffic != nil && matchedTraffic.Variation != nil {
			variationValue = matchedTraffic.Variation
		} else if matchedAllocation != nil && matchedAllocation.Variation != "" {
			variationValue = &matchedAllocation.Variation
		}

		if variationValue != nil && feature.Variations != nil {
			for _, variation := range feature.Variations {
				if variation.Value == *variationValue {
					if variation.VariableOverrides != nil {
						if overrides, exists := variation.VariableOverrides[*options.VariableKey]; exists {
							for index, override := range overrides {
								if variableOverrideIsMatched(override, options.evaluateDependencies) {
									overrideIndex := index
									evaluation = Evaluation{
										Type:        options.Type,
										FeatureKey:  options.FeatureKey,
										Reason:      EvaluationReasonVariableOverrideVariation,
										BucketKey:   &bucketKey,
										BucketValue: &bucketValue,
										RuleKey: func() *RuleKey {
											if matchedTraffic != nil {
												return &matchedTraffic.Key
											}
											return nil
										}(),
										Traffic:               matchedTraffic,
										VariableKey:           options.VariableKey,
										VariableSchema:        variableSchema,
										VariableValue:         override.Value,
										VariableOverrideIndex: &overrideIndex,
										VariableOverrideKey:   override.Key,
										VariableOverridePath:  override.KeyPath,
									}
									evaluation.variableValueSet = true

									options.diagnosticReporter.debugEvaluation("variable override from variation", evaluation)

									return evaluation
								}
							}
						}
					}

					if variation.Variables != nil {
						if variableValue, exists := variation.Variables[*options.VariableKey]; exists {
							evaluation = Evaluation{
								Type:        options.Type,
								FeatureKey:  options.FeatureKey,
								Reason:      EvaluationReasonAllocated,
								BucketKey:   &bucketKey,
								BucketValue: &bucketValue,
								RuleKey: func() *RuleKey {
									if matchedTraffic != nil {
										return &matchedTraffic.Key
									}
									return nil
								}(),
								Traffic:        matchedTraffic,
								VariableKey:    options.VariableKey,
								VariableSchema: variableSchema,
								VariableValue:  variableValue,
							}
							evaluation.variableValueSet = true

							options.diagnosticReporter.debugEvaluation("allocated variable", evaluation)

							return evaluation
						}
					}
				}
			}
		}

		// Check for default value from variable schema
		if variableSchema.defaultValueSet {
			evaluation = Evaluation{
				Type:           options.Type,
				FeatureKey:     options.FeatureKey,
				Reason:         EvaluationReasonVariableDefault,
				BucketKey:      &bucketKey,
				BucketValue:    &bucketValue,
				VariableKey:    options.VariableKey,
				VariableSchema: variableSchema,
				VariableValue:  variableSchema.DefaultValue,
			}
			evaluation.variableValueSet = variableSchema.defaultValueSet

			options.diagnosticReporter.debugEvaluation("using default value", evaluation)

			return evaluation
		}

		// Variable not found
		evaluation = Evaluation{
			Type:        options.Type,
			FeatureKey:  options.FeatureKey,
			Reason:      EvaluationReasonVariableNotFound,
			VariableKey: options.VariableKey,
			BucketKey:   &bucketKey,
			BucketValue: &bucketValue,
		}

		options.diagnosticReporter.debugEvaluation("variable not found", evaluation)

		return evaluation
	}

	/**
	 * Nothing matched
	 */
	if options.Type == EvaluationTypeVariation {
		evaluation = Evaluation{
			Type:        options.Type,
			FeatureKey:  options.FeatureKey,
			Reason:      EvaluationReasonNoMatch,
			BucketKey:   &bucketKey,
			BucketValue: &bucketValue,
		}

		options.diagnosticReporter.debugEvaluation("no matched variation", evaluation)

		return evaluation
	}

	if options.Type == EvaluationTypeVariable {
		if variableSchema != nil {
			evaluation = Evaluation{
				Type:           options.Type,
				FeatureKey:     options.FeatureKey,
				Reason:         EvaluationReasonVariableDefault,
				BucketKey:      &bucketKey,
				BucketValue:    &bucketValue,
				VariableKey:    options.VariableKey,
				VariableSchema: variableSchema,
				VariableValue:  variableSchema.DefaultValue,
			}
			evaluation.variableValueSet = variableSchema.defaultValueSet

			options.diagnosticReporter.debugEvaluation("using default value", evaluation)

			return evaluation
		}

		evaluation = Evaluation{
			Type:        options.Type,
			FeatureKey:  options.FeatureKey,
			Reason:      EvaluationReasonVariableNotFound,
			VariableKey: options.VariableKey,
			BucketKey:   &bucketKey,
			BucketValue: &bucketValue,
		}

		options.diagnosticReporter.debugEvaluation("variable not found", evaluation)

		return evaluation
	}

	evaluation = Evaluation{
		Type:        options.Type,
		FeatureKey:  options.FeatureKey,
		Reason:      EvaluationReasonNoMatch,
		BucketKey:   &bucketKey,
		BucketValue: &bucketValue,
		Enabled:     &[]bool{false}[0],
	}

	options.diagnosticReporter.debugEvaluation("nothing matched", evaluation)

	return evaluation
}
