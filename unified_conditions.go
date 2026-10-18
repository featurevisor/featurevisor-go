package featurevisor

func conditionsAreMatched(value interface{}, dependencies evaluateDependencies) (matched bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, invalid := recovered.(selectorParseFailure); !invalid {
				panic(recovered)
			}
			matched = false
		}
	}()
	return matchConditions(value, &dependencies)
}

func matchConditions(value interface{}, dependencies *evaluateDependencies) bool {
	if _, invalid := value.(invalidSelector); invalid {
		panic(selectorParseFailure{})
	}
	d := dependencies.instanceEvaluationDataProvider
	switch v := value.(type) {
	case string:
		if v == "*" {
			return true
		}
		local := *dependencies
		dependencies = &local
		if dependencies.refreshState != nil {
			dependencies.refreshState(dependencies)
			d = dependencies.instanceEvaluationDataProvider
		}
		segment := d.getSegmentForEvaluation(v)
		if segment == nil {
			return false
		}
		if _, invalid := segment.Conditions.(invalidSelector); invalid {
			panic(selectorParseFailure{})
		}
		if attributeOnly(segment.Conditions) {
			return d.AllConditionsAreMatched(segment.Conditions, dependencies.Context)
		}
		if dependencies.session == nil {
			dependencies.session = &evaluationSession{}
		}
		session := dependencies.session
		if session.segments == nil {
			session.segments = map[string]bool{}
		}
		if session.segments[v] {
			panic(errVariableRequirementCycle)
		}
		if session.depth+len(session.segments) >= 257 {
			panic(dependencyError{message: "Evaluation dependency depth exceeded"})
		}
		session.segments[v] = true
		defer delete(session.segments, v)
		return matchConditions(segment.Conditions, dependencies)
	case []interface{}:
		if len(v) == 0 {
			return false
		}
		for _, child := range v {
			if !matchConditions(child, dependencies) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		if _, variable := v["variable"]; variable {
			return requiredVariablesAreMatched([]RequiredVariable{v}, *dependencies)
		}
		if _, feature := v["feature"]; feature {
			return requiredFeaturesAreMatched([]RequiredFeature{v}, *dependencies)
		}
		if _, attribute := v["attribute"]; attribute {
			return d.AllConditionsAreMatched(v, dependencies.Context)
		}
		for _, op := range []string{"and", "or", "not"} {
			if children, exists := v[op]; exists {
				list, ok := children.([]interface{})
				if !ok || len(list) == 0 {
					return false
				}
				if op == "or" {
					for _, child := range list {
						if matchConditions(child, dependencies) {
							return true
						}
					}
					return false
				}
				result := matchConditions(list, dependencies)
				if op == "not" {
					return !result
				}
				return result
			}
		}
	case PlainCondition:
		return conditionIsMatched(v, dependencies.Context, d.getRegex)
	case nil:
		return false
	default:
		converted := expressionValue(value)
		switch converted.(type) {
		case string, []interface{}, map[string]interface{}, invalidSelector:
			return matchConditions(converted, dependencies)
		}
		return false
	}
	return false
}

func attributeOnly(value interface{}) bool {
	switch v := value.(type) {
	case PlainCondition:
		return true
	case []interface{}:
		for _, child := range v {
			if !attributeOnly(child) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		if _, variable := v["variable"]; variable {
			return false
		}
		if _, feature := v["feature"]; feature {
			return false
		}
		if _, attribute := v["attribute"]; attribute {
			return true
		}
		for _, op := range []string{"and", "or", "not"} {
			if children, exists := v[op]; exists {
				return attributeOnly(children)
			}
		}
		return true
	}
	return false
}

func matchedTrafficForEvaluation(traffic []Traffic, dependencies evaluateDependencies) *Traffic {
	for _, entry := range traffic {
		if entry.Conditions != nil && conditionsAreMatched(entry.Conditions, dependencies) {
			return &entry
		}
	}
	return nil
}

func matchedForce(feature *Feature, dependencies evaluateDependencies) forceResult {
	for index, entry := range feature.Force {
		if entry.Conditions != nil && conditionsAreMatched(entry.Conditions, dependencies) {
			return forceResult{Force: &entry, ForceIndex: &index}
		}
	}
	return forceResult{}
}

// ComposeEntry names one literal output property and an ordinary getter source.
type ComposeEntry struct {
	Key       string `json:"key"`
	Feature   string `json:"feature,omitempty"`
	Variable  string `json:"variable,omitempty"`
	Variation bool   `json:"variation,omitempty"`
}

func compositionSource(entry ComposeEntry, dependencies evaluateDependencies) interface{} {
	clean := cleanRequiredFeatureDependencies(dependencies)
	clean.DependencyPurpose = "composition"
	params := evaluateParams{Type: EvaluationTypeFlag, FeatureKey: entry.Feature}
	if entry.Variable != "" {
		params.Type = EvaluationTypeVariable
		params.VariableKey = &entry.Variable
		params.GlobalVariable = entry.Feature == ""
	} else if entry.Variation {
		params.Type = EvaluationTypeVariation
	}
	evaluation := evaluateWithModules(EvaluateOptions{evaluateParams: params, evaluateDependencies: clean})
	propagateRequirementError(evaluation)
	if params.Type == EvaluationTypeFlag {
		return evaluation.Enabled != nil && *evaluation.Enabled
	}
	if params.Type == EvaluationTypeVariation {
		if evaluation.VariationValue != nil {
			return *evaluation.VariationValue
		}
		if evaluation.Variation != nil {
			return evaluation.Variation.Value
		}
		return nil
	}
	return normalizedGetterValue(evaluation, dependencies.diagnosticReporter)
}

func composeValue(compose interface{}, dependencies evaluateDependencies) interface{} {
	if alias, ok := compose.(string); ok {
		return compositionSource(ComposeEntry{Variable: alias}, dependencies)
	}
	entries := expressionValue(compose)
	list, ok := entries.([]interface{})
	if !ok || len(list) == 0 {
		panic("Invalid variable composition")
	}
	result := make(map[string]interface{}, len(list))
	for _, raw := range list {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			panic("Invalid variable composition entry")
		}
		key, _ := entry["key"].(string)
		if key == "" || unsafeMutationKeys[key] {
			panic(dependencyError{message: "Invalid variable composition property"})
		}
		feature, _ := entry["feature"].(string)
		variable, _ := entry["variable"].(string)
		variation, _ := entry["variation"].(bool)
		if feature == "" && variable == "" {
			panic("Invalid variable composition source")
		}
		result[key] = compositionSource(ComposeEntry{Key: key, Feature: feature, Variable: variable, Variation: variation}, dependencies)
	}
	return result
}

func freshComposedValue(variable *GlobalVariable, value interface{}) interface{} {
	if variable.Compose != nil {
		if _, alias := variable.Compose.(string); !alias {
			if result, ok := variableObjectCopy(value); ok {
				return result
			}
		}
	}
	return value
}
