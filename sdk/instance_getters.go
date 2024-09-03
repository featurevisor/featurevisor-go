package sdk

import (
	"github.com/featurevisor/featurevisor-go/types"
)

func (i *FeaturevisorInstance) GetRevision() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.datafileReader.GetRevision()
}

func (i *FeaturevisorInstance) GetFeature(featureKey types.FeatureKey) *types.Feature {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.datafileReader.GetFeature(featureKey)
}

func (i *FeaturevisorInstance) GetVariableBoolean(featureKey types.FeatureKey, variableKey types.VariableKey, context types.Context) bool {
	value := i.GetVariable(featureKey, variableKey, context)
	if boolValue, ok := value.(bool); ok {
		return boolValue
	}
	return false
}

func (i *FeaturevisorInstance) GetVariableString(featureKey types.FeatureKey, variableKey types.VariableKey, context types.Context) string {
	value := i.GetVariable(featureKey, variableKey, context)
	if strValue, ok := value.(string); ok {
		return strValue
	}
	return ""
}

func (i *FeaturevisorInstance) GetVariableInteger(featureKey types.FeatureKey, variableKey types.VariableKey, context types.Context) int {
	value := i.GetVariable(featureKey, variableKey, context)
	if intValue, ok := value.(int); ok {
		return intValue
	}
	if floatValue, ok := value.(float64); ok {
		return int(floatValue)
	}
	return 0
}

func (i *FeaturevisorInstance) GetVariableDouble(featureKey types.FeatureKey, variableKey types.VariableKey, context types.Context) float64 {
	value := i.GetVariable(featureKey, variableKey, context)
	if floatValue, ok := value.(float64); ok {
		return floatValue
	}
	if intValue, ok := value.(int); ok {
		return float64(intValue)
	}
	return 0.0
}

func (i *FeaturevisorInstance) GetVariableArray(featureKey types.FeatureKey, variableKey types.VariableKey, context types.Context) []string {
	value := i.GetVariable(featureKey, variableKey, context)
	if arrayValue, ok := value.([]string); ok {
		return arrayValue
	}
	return nil
}

func (i *FeaturevisorInstance) GetVariableObject(featureKey types.FeatureKey, variableKey types.VariableKey, context types.Context) map[string]interface{} {
	value := i.GetVariable(featureKey, variableKey, context)
	if objectValue, ok := value.(map[string]interface{}); ok {
		return objectValue
	}
	return nil
}

func (i *FeaturevisorInstance) GetVariableJSON(featureKey types.FeatureKey, variableKey types.VariableKey, context types.Context) interface{} {
	return i.GetVariable(featureKey, variableKey, context)
}
