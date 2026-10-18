package featurevisor

import (
	"fmt"
	"math"
	"reflect"
)

// EvaluationDependency identifies the immediate evaluation which requested a prerequisite.
type EvaluationDependency struct {
	Type        EvaluationType `json:"type"`
	FeatureKey  FeatureKey     `json:"featureKey,omitempty"`
	VariableKey *VariableKey   `json:"variableKey,omitempty"`
}

type evaluationSession struct {
	segments        map[string]bool
	depth           int
	memo            map[evaluationMemoKey][]evaluationMemoEntry
	contextSnapshot Context
	active          map[variableRequirementIdentity]bool
	frames          map[evaluationFrameIdentity]int
	rootFrames      [2]evaluationFrameIdentity
	rootFrameCount  int
	stickyFeatures  map[*StickyFeatures]struct{}
	stickyVariables map[*StickyVariables]struct{}
}

func (s *evaluationSession) hasFrame(frame evaluationFrameIdentity) bool {
	for index := 0; index < s.rootFrameCount; index++ {
		if s.rootFrames[index] == frame {
			return true
		}
	}
	return s.frames[frame] > 0
}

type evaluationMemoKey struct {
	purpose   string
	frame     evaluationFrameIdentity
	requester evaluationFrameIdentity
	provider  *instanceEvaluationDataProvider
	modules   *modulesManager
	features  *StickyFeatures
	variables *StickyVariables
	version   uint64
}

type evaluationMemoEntry struct {
	context Context
	value   Evaluation
}

type evaluationFrameIdentity struct {
	Type        EvaluationType
	Feature     FeatureKey
	Variable    string
	HasVariable bool
	Global      bool
}

func frameIdentity(options EvaluateOptions) evaluationFrameIdentity {
	return evaluationFrameIdentity{Type: options.Type, Feature: options.FeatureKey,
		Variable: stringValue(options.VariableKey), HasVariable: options.VariableKey != nil,
		Global: options.GlobalVariable}
}

func publicModuleOptions(options EvaluateOptions) EvaluateOptions {
	if options.DependencyOf != nil {
		parent := *options.DependencyOf
		if parent.VariableKey != nil {
			key := *parent.VariableKey
			parent.VariableKey = &key
		}
		options.DependencyOf = &parent
	}
	options.diagnosticReporter = nil
	options.modulesManager = nil
	options.instanceEvaluationDataProvider = nil
	options.stickyFeatures = nil
	options.stickyVariables = nil
	options.session = nil
	options.stateVersion = nil
	options.refreshState = nil
	options.refreshStickyVariables = nil
	options.storedContextSnapshot = nil
	options.seenStateVersion = 0
	options.requesterGlobalVariable = false
	options.currentEvaluationGlobal = false
	return options
}

func restoreModuleInternals(options *EvaluateOptions, previous EvaluateOptions) {
	options.DependencyOf = previous.DependencyOf
	options.DependencyPurpose = previous.DependencyPurpose
	options.diagnosticReporter = previous.diagnosticReporter
	options.modulesManager = previous.modulesManager
	options.instanceEvaluationDataProvider = previous.instanceEvaluationDataProvider
	options.stickyFeatures = previous.stickyFeatures
	options.stickyVariables = previous.stickyVariables
	options.session = previous.session
	options.stateVersion = previous.stateVersion
	options.refreshState = previous.refreshState
	options.refreshStickyVariables = previous.refreshStickyVariables
	options.storedContextSnapshot = previous.storedContextSnapshot
	options.seenStateVersion = previous.seenStateVersion
	options.requesterGlobalVariable = previous.requesterGlobalVariable
	options.currentEvaluationGlobal = previous.currentEvaluationGlobal
}

func dependencyOf(options EvaluateOptions) *EvaluationDependency {
	return &EvaluationDependency{Type: options.Type, FeatureKey: options.FeatureKey, VariableKey: options.VariableKey}
}

func (s *evaluationSession) key(options EvaluateOptions) (evaluationMemoKey, bool) {
	if s == nil || options.DependencyOf == nil {
		return evaluationMemoKey{}, false
	}
	// Retain every keyed map until the root request ends. The pointer used in a
	// memo key cannot then be recycled after a module replaces its sticky map.
	if options.stickyFeatures != nil {
		if s.stickyFeatures == nil {
			s.stickyFeatures = make(map[*StickyFeatures]struct{})
		}
		s.stickyFeatures[options.stickyFeatures] = struct{}{}
	}
	if options.stickyVariables != nil {
		if s.stickyVariables == nil {
			s.stickyVariables = make(map[*StickyVariables]struct{})
		}
		s.stickyVariables[options.stickyVariables] = struct{}{}
	}
	version := uint64(0)
	if options.stateVersion != nil {
		version = options.stateVersion()
	}
	return evaluationMemoKey{
		purpose: options.DependencyPurpose,
		frame:   frameIdentity(options),
		requester: evaluationFrameIdentity{Type: options.DependencyOf.Type, Feature: options.DependencyOf.FeatureKey,
			Variable: stringValue(options.DependencyOf.VariableKey), HasVariable: options.DependencyOf.VariableKey != nil,
			Global: options.requesterGlobalVariable},
		provider: options.instanceEvaluationDataProvider, modules: options.modulesManager,
		features: options.stickyFeatures, variables: options.stickyVariables, version: version,
	}, true
}

// Context snapshots are owned by the request. Unsupported or cyclic host
// values bypass memoization, matching the existing fingerprint policy.
func snapshotContext(context Context) (Context, bool) {
	budget := 10000
	value, ok := cloneContextValue(reflect.ValueOf(context), 0, &budget, map[uintptr]bool{})
	if !ok {
		return nil, false
	}
	return value.Interface().(Context), true
}

func cloneContextValue(value reflect.Value, depth int, budget *int, ancestors map[uintptr]bool) (reflect.Value, bool) {
	if !value.IsValid() {
		return value, true
	}
	*budget -= 1
	if depth > 64 || *budget < 0 {
		return reflect.Value{}, false
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return value, true
		}
		if value.Kind() == reflect.Pointer {
			pointer := value.Pointer()
			if ancestors[pointer] {
				return reflect.Value{}, false
			}
			ancestors[pointer] = true
			defer delete(ancestors, pointer)
		}
		inner, ok := cloneContextValue(value.Elem(), depth+1, budget, ancestors)
		if !ok {
			return reflect.Value{}, false
		}
		result := reflect.New(value.Type()).Elem()
		if value.Kind() == reflect.Pointer {
			result = reflect.New(value.Type().Elem())
			result.Elem().Set(inner)
		} else {
			result.Set(inner)
		}
		return result, true
	case reflect.Map:
		if value.IsNil() {
			return value, true
		}
		if value.Type().Key().Kind() != reflect.String {
			return reflect.Value{}, false
		}
		if value.Len() > *budget {
			return reflect.Value{}, false
		}
		pointer := value.Pointer()
		if ancestors[pointer] {
			return reflect.Value{}, false
		}
		ancestors[pointer] = true
		defer delete(ancestors, pointer)
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			child, ok := cloneContextValue(iter.Value(), depth+1, budget, ancestors)
			if !ok {
				return reflect.Value{}, false
			}
			result.SetMapIndex(iter.Key(), child)
		}
		return result, true
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return value, true
		}
		if value.Len() > *budget {
			return reflect.Value{}, false
		}
		var result reflect.Value
		if value.Kind() == reflect.Slice {
			pointer := value.Pointer()
			if ancestors[pointer] {
				return reflect.Value{}, false
			}
			ancestors[pointer] = true
			defer delete(ancestors, pointer)
			result = reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		} else {
			result = reflect.New(value.Type()).Elem()
		}
		for index := 0; index < value.Len(); index++ {
			child, ok := cloneContextValue(value.Index(index), depth+1, budget, ancestors)
			if !ok {
				return reflect.Value{}, false
			}
			result.Index(index).Set(child)
		}
		return result, true
	case reflect.Struct:
		if value.NumField() > *budget {
			return reflect.Value{}, false
		}
		result := reflect.New(value.Type()).Elem()
		for index := 0; index < value.NumField(); index++ {
			if value.Type().Field(index).PkgPath != "" {
				return reflect.Value{}, false
			}
			child, ok := cloneContextValue(value.Field(index), depth+1, budget, ancestors)
			if !ok {
				return reflect.Value{}, false
			}
			result.Field(index).Set(child)
		}
		return result, true
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(value.Float()) || math.IsInf(value.Float(), 0) {
			return reflect.Value{}, false
		}
		return value, true
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value, true
	default:
		return reflect.Value{}, false
	}
}

func sameContext(snapshot, current Context) bool {
	budget := 10000
	return sameContextValue(reflect.ValueOf(snapshot), reflect.ValueOf(current), 0, &budget)
}

func sameContextValue(a, b reflect.Value, depth int, budget *int) bool {
	*budget -= 1
	if depth > 64 || *budget < 0 || !a.IsValid() || !b.IsValid() {
		return a.IsValid() == b.IsValid() && depth <= 64 && *budget >= 0
	}
	if a.Type() != b.Type() {
		return false
	}
	switch a.Kind() {
	case reflect.Interface, reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		return sameContextValue(a.Elem(), b.Elem(), depth+1, budget)
	case reflect.Map:
		if a.IsNil() != b.IsNil() || a.Len() != b.Len() {
			return false
		}
		if a.Len() > *budget {
			return false
		}
		iter := a.MapRange()
		for iter.Next() {
			other := b.MapIndex(iter.Key())
			if !other.IsValid() || !sameContextValue(iter.Value(), other, depth+1, budget) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Array:
		if a.Kind() == reflect.Slice && a.IsNil() != b.IsNil() {
			return false
		}
		if a.Len() != b.Len() {
			return false
		}
		if a.Len() > *budget {
			return false
		}
		for index := 0; index < a.Len(); index++ {
			if !sameContextValue(a.Index(index), b.Index(index), depth+1, budget) {
				return false
			}
		}
		return true
	case reflect.Struct:
		if a.NumField() > *budget {
			return false
		}
		for index := 0; index < a.NumField(); index++ {
			if !sameContextValue(a.Field(index), b.Field(index), depth+1, budget) {
				return false
			}
		}
		return true
	case reflect.Float32:
		return math.Float32bits(float32(a.Float())) == math.Float32bits(float32(b.Float()))
	case reflect.Float64:
		return math.Float64bits(a.Float()) == math.Float64bits(b.Float())
	default:
		return a.Interface() == b.Interface()
	}
}

func minimalDependencyEvaluation(evaluation Evaluation) (Evaluation, bool) {
	return memoDependencyEvaluation(evaluation, false)
}

func memoDependencyEvaluation(evaluation Evaluation, borrowed bool) (Evaluation, bool) {
	result := Evaluation{
		Type: evaluation.Type, FeatureKey: evaluation.FeatureKey, VariableKey: evaluation.VariableKey,
		Reason: evaluation.Reason, Error: evaluation.Error, Enabled: evaluation.Enabled,
		VariationValue: evaluation.VariationValue, VariableValue: evaluation.VariableValue,
		VariableSchema: evaluation.VariableSchema, GlobalVariable: evaluation.GlobalVariable,
		variableValueSet: evaluation.variableValueSet, getterValueNormalized: evaluation.getterValueNormalized,
	}
	if result.VariationValue == nil && evaluation.Variation != nil {
		value := evaluation.Variation.Value
		result.VariationValue = &value
	}
	if evaluation.VariableKey != nil {
		key := *evaluation.VariableKey
		result.VariableKey = &key
	}
	if evaluation.Enabled != nil {
		enabled := *evaluation.Enabled
		result.Enabled = &enabled
	}
	if result.VariationValue != nil {
		variation := *result.VariationValue
		result.VariationValue = &variation
	}
	if evaluation.VariableSchema != nil {
		result.VariableSchema = &VariableSchema{Type: evaluation.VariableSchema.Type}
	}
	if evaluation.GlobalVariable != nil {
		result.GlobalVariable = &GlobalVariable{Type: evaluation.GlobalVariable.Type}
		if evaluation.GlobalVariable.Compose != nil {
			result.GlobalVariable.Compose = true
		}
	}
	if evaluation.Error != nil {
		result.Error = fmt.Errorf("%s", evaluation.Error.Error())
	}
	if borrowed {
		return result, true
	}
	cloned, okay := cloneMemoValue(reflect.ValueOf(result.VariableValue), 0)
	if !okay {
		return Evaluation{}, false
	}
	if cloned.IsValid() {
		result.VariableValue = cloned.Interface()
	}
	return result, true
}

func cloneMemoValue(value reflect.Value, depth int) (reflect.Value, bool) {
	if !value.IsValid() {
		return value, true
	}
	if depth > 64 {
		return reflect.Value{}, false
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return value, true
		}
		inner, okay := cloneMemoValue(value.Elem(), depth+1)
		if !okay {
			return reflect.Value{}, false
		}
		copy := reflect.New(value.Type()).Elem()
		copy.Set(inner)
		return copy, true
	case reflect.Map:
		if value.IsNil() {
			return value, true
		}
		copy := reflect.MakeMapWithSize(value.Type(), value.Len())
		for _, key := range value.MapKeys() {
			child, okay := cloneMemoValue(value.MapIndex(key), depth+1)
			if !okay {
				return reflect.Value{}, false
			}
			copy.SetMapIndex(key, child)
		}
		return copy, true
	case reflect.Slice:
		if value.IsNil() {
			return value, true
		}
		copy := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			child, okay := cloneMemoValue(value.Index(index), depth+1)
			if !okay {
				return reflect.Value{}, false
			}
			copy.Index(index).Set(child)
		}
		return copy, true
	case reflect.Array:
		copy := reflect.New(value.Type()).Elem()
		for index := 0; index < value.Len(); index++ {
			child, okay := cloneMemoValue(value.Index(index), depth+1)
			if !okay {
				return reflect.Value{}, false
			}
			copy.Index(index).Set(child)
		}
		return copy, true
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return value, true
	default:
		return reflect.Value{}, false
	}
}

func mergeChangedStoredContext(effective Context, previous Context, current Context, callContext Context) {
	for key, oldValue := range previous {
		if _, overridden := callContext[key]; overridden {
			continue
		}
		newValue, exists := current[key]
		if reflect.DeepEqual(oldValue, newValue) && exists {
			continue
		}
		if reflect.DeepEqual(effective[key], oldValue) {
			if exists {
				effective[key] = newValue
			} else {
				delete(effective, key)
			}
		}
	}
	for key, newValue := range current {
		if _, existed := previous[key]; existed {
			continue
		}
		if _, overridden := callContext[key]; !overridden {
			if _, present := effective[key]; !present {
				effective[key] = newValue
			}
		}
	}
}

func copyContext(context Context) Context {
	copy := Context{}
	for key, value := range context {
		copy[key] = value
	}
	return copy
}
