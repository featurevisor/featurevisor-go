package sdk

import (
	"encoding/json"

	"github.com/featurevisor/featurevisor-go/types"
)

// GetVariable returns the variable value for a given feature, variable key, and context
func (f *FeaturevisorInstance) GetVariable(featureKey interface{}, variableKey types.VariableKey, context types.Context) interface{} {
	evaluation := f.EvaluateVariable(featureKey, variableKey, context)

	if evaluation.VariableValue != nil {
		if evaluation.VariableSchema != nil && evaluation.VariableSchema.Type == "json" {
			if strValue, ok := evaluation.VariableValue.(string); ok {
				var jsonValue interface{}
				if err := json.Unmarshal([]byte(strValue), &jsonValue); err == nil {
					return jsonValue
				}
				// If JSON parsing fails, return the original string value
			}
		}
		return evaluation.VariableValue
	}

	return nil
}

// GetVariableBoolean returns the boolean variable value for a given feature, variable key, and context
func (f *FeaturevisorInstance) GetVariableBoolean(featureKey interface{}, variableKey types.VariableKey, context types.Context) *bool {
	value := f.GetVariable(featureKey, variableKey, context)
	if boolValue, ok := value.(bool); ok {
		return &boolValue
	}
	return nil
}

// GetVariableString returns the string variable value for a given feature, variable key, and context
func (f *FeaturevisorInstance) GetVariableString(featureKey interface{}, variableKey types.VariableKey, context types.Context) *string {
	value := f.GetVariable(featureKey, variableKey, context)
	if strValue, ok := value.(string); ok {
		return &strValue
	}
	return nil
}

// GetVariableInteger returns the integer variable value for a given feature, variable key, and context
func (f *FeaturevisorInstance) GetVariableInteger(featureKey interface{}, variableKey types.VariableKey, context types.Context) *int {
	value := f.GetVariable(featureKey, variableKey, context)
	if intValue, ok := value.(int); ok {
		return &intValue
	}
	if floatValue, ok := value.(float64); ok {
		intValue := int(floatValue)
		return &intValue
	}
	return nil
}

// GetVariableDouble returns the double variable value for a given feature, variable key, and context
func (f *FeaturevisorInstance) GetVariableDouble(featureKey interface{}, variableKey types.VariableKey, context types.Context) *float64 {
	value := f.GetVariable(featureKey, variableKey, context)
	if floatValue, ok := value.(float64); ok {
		return &floatValue
	}
	return nil
}

// GetVariableArray returns the array variable value for a given feature, variable key, and context
func (f *FeaturevisorInstance) GetVariableArray(featureKey interface{}, variableKey types.VariableKey, context types.Context) []string {
	value := f.GetVariable(featureKey, variableKey, context)
	if arrayValue, ok := value.([]interface{}); ok {
		result := make([]string, len(arrayValue))
		for i, v := range arrayValue {
			if strValue, ok := v.(string); ok {
				result[i] = strValue
			}
		}
		return result
	}
	return nil
}

// GetVariableObject returns the object variable value for a given feature, variable key, and context
func (f *FeaturevisorInstance) GetVariableObject(featureKey interface{}, variableKey types.VariableKey, context types.Context) map[string]interface{} {
	value := f.GetVariable(featureKey, variableKey, context)
	if objValue, ok := value.(map[string]interface{}); ok {
		return objValue
	}
	return nil
}

// GetVariableJSON returns the JSON variable value for a given feature, variable key, and context
func (f *FeaturevisorInstance) GetVariableJSON(featureKey interface{}, variableKey types.VariableKey, context types.Context) interface{} {
	return f.GetVariable(featureKey, variableKey, context)
}
