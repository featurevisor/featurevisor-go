package sdk

import (
	"regexp"
	"strings"
	"time"
)

// GetRegex is a function type for getting regex patterns
type GetRegex func(regexString string, regexFlags string) *regexp.Regexp

// PathExists checks if a path exists in a context object
func PathExists(obj map[string]interface{}, path string) bool {
	if !strings.Contains(path, ".") {
		_, exists := obj[path]
		return exists
	}

	parts := strings.Split(path, ".")
	var current interface{} = obj

	for _, part := range parts {
		if current == nil {
			return false
		}

		if mapValue, ok := current.(map[string]interface{}); ok {
			if _, exists := mapValue[part]; !exists {
				return false
			}
			current = mapValue[part]
		} else {
			return false
		}
	}

	return true
}

// GetValueFromContext extracts a value from a context object using a dot-separated path
func GetValueFromContext(obj map[string]interface{}, path string) interface{} {
	if !strings.Contains(path, ".") {
		return obj[path]
	}

	parts := strings.Split(path, ".")
	var current interface{} = obj

	for _, part := range parts {
		if current == nil {
			return nil
		}

		if mapValue, ok := current.(map[string]interface{}); ok {
			current = mapValue[part]
		} else {
			return nil
		}
	}

	return current
}

// ConditionIsMatched checks if a condition is matched given a context
func ConditionIsMatched(
	condition PlainCondition,
	context Context,
	getRegex GetRegex,
) bool {
	contextValueFromPath := GetValueFromContext(context, string(condition.Attribute))

	// Handle nil values
	if condition.Value == nil {
		if condition.Operator == OperatorExists {
			return contextValueFromPath != nil
		} else if condition.Operator == OperatorNotExists {
			return contextValueFromPath == nil
		}
		return false
	}

	value := *condition.Value

	// equals / notEquals
	if condition.Operator == OperatorEquals {
		return contextValueFromPath == value
	} else if condition.Operator == OperatorNotEquals {
		return contextValueFromPath != value
	}

	// before / after (date comparisons)
	if condition.Operator == OperatorBefore || condition.Operator == OperatorAfter {
		var dateInContext time.Time
		var dateInCondition time.Time
		var err error

		// Parse context value
		switch v := contextValueFromPath.(type) {
		case time.Time:
			dateInContext = v
		case string:
			dateInContext, err = time.Parse(time.RFC3339, v)
			if err != nil {
				// Try other common formats
				dateInContext, err = time.Parse("2006-01-02", v)
				if err != nil {
					return false
				}
			}
		default:
			return false
		}

		// Parse condition value
		switch v := value.(type) {
		case time.Time:
			dateInCondition = v
		case string:
			dateInCondition, err = time.Parse(time.RFC3339, v)
			if err != nil {
				// Try other common formats
				dateInCondition, err = time.Parse("2006-01-02", v)
				if err != nil {
					return false
				}
			}
		default:
			return false
		}

		if condition.Operator == OperatorBefore {
			return dateInContext.Before(dateInCondition)
		} else {
			return dateInContext.After(dateInCondition)
		}
	}

	// in / notIn (where condition value is an array)
	if valueArray, ok := value.([]interface{}); ok {
		if contextValueFromPath == nil {
			if condition.Operator == OperatorIn {
				return false
			} else if condition.Operator == OperatorNotIn {
				// Check if the path exists in the context first (like PHP implementation)
				if !PathExists(context, string(condition.Attribute)) {
					return false
				}
				// null is not in the array, so return true
				return true
			}
		}

		// Handle case where context value is also an array
		if contextArray, ok := contextValueFromPath.([]interface{}); ok {
			// For arrays in context, check if any element from context array is in condition array
			if condition.Operator == OperatorIn {
				for _, contextItem := range contextArray {
					for _, conditionItem := range valueArray {
						if contextItem == conditionItem {
							return true
						}
					}
				}
				return false
			} else if condition.Operator == OperatorNotIn {
				// For notIn with array context values, return false (like PHP implementation)
				// PHP only handles notIn for string, numeric, or null context values
				return false
			}
		} else {
			// Context value is a single value
			valueInContext := contextValueFromPath

			// Only handle in/notIn for string, numeric, or null context values (like PHP implementation)
			switch valueInContext.(type) {
			case string, int, float64, bool:
				if condition.Operator == OperatorIn {
					// Check if context value is in the condition's array
					for _, item := range valueArray {
						if item == valueInContext {
							return true
						}
					}
					return false
				} else if condition.Operator == OperatorNotIn {
					// Check if the path exists in the context first (like PHP implementation)
					if !PathExists(context, string(condition.Attribute)) {
						return false
					}
					// Check if context value is NOT in the condition's array
					for _, item := range valueArray {
						if item == valueInContext {
							return false
						}
					}
					return true
				}
			default:
				// For other types (like objects, arrays), don't match in/notIn conditions
				return false
			}
		}
	}

	// String operations
	if contextValueStr, ok := contextValueFromPath.(string); ok {
		if valueStr, ok := value.(string); ok {
			switch condition.Operator {
			case OperatorContains:
				return strings.Contains(contextValueStr, valueStr)
			case OperatorNotContains:
				return !strings.Contains(contextValueStr, valueStr)
			case OperatorStartsWith:
				return strings.HasPrefix(contextValueStr, valueStr)
			case OperatorEndsWith:
				return strings.HasSuffix(contextValueStr, valueStr)
			case OperatorSemverEquals:
				result, err := CompareVersions(contextValueStr, valueStr)
				return err == nil && result == 0
			case OperatorSemverNotEquals:
				result, err := CompareVersions(contextValueStr, valueStr)
				return err == nil && result != 0
			case OperatorSemverGreaterThan:
				result, err := CompareVersions(contextValueStr, valueStr)
				return err == nil && result == 1
			case OperatorSemverGreaterThanOrEquals:
				result, err := CompareVersions(contextValueStr, valueStr)
				return err == nil && result >= 0
			case OperatorSemverLessThan:
				result, err := CompareVersions(contextValueStr, valueStr)
				return err == nil && result == -1
			case OperatorSemverLessThanOrEquals:
				result, err := CompareVersions(contextValueStr, valueStr)
				return err == nil && result <= 0
			case OperatorMatches:
				regexFlags := ""
				if condition.RegexFlags != nil {
					regexFlags = *condition.RegexFlags
				}
				regex := getRegex(valueStr, regexFlags)
				return regex.MatchString(contextValueStr)
			case OperatorNotMatches:
				regexFlags := ""
				if condition.RegexFlags != nil {
					regexFlags = *condition.RegexFlags
				}
				regex := getRegex(valueStr, regexFlags)
				return !regex.MatchString(contextValueStr)
			}
		}
	}

	// Numeric operations
	if contextValueNum, ok := contextValueFromPath.(float64); ok {
		if valueNum, ok := value.(float64); ok {
			switch condition.Operator {
			case OperatorGreaterThan:
				return contextValueNum > valueNum
			case OperatorGreaterThanOrEquals:
				return contextValueNum >= valueNum
			case OperatorLessThan:
				return contextValueNum < valueNum
			case OperatorLessThanOrEquals:
				return contextValueNum <= valueNum
			}
		}
	}

	// Handle integer types
	if contextValueInt, ok := contextValueFromPath.(int); ok {
		if valueInt, ok := value.(int); ok {
			switch condition.Operator {
			case OperatorGreaterThan:
				return contextValueInt > valueInt
			case OperatorGreaterThanOrEquals:
				return contextValueInt >= valueInt
			case OperatorLessThan:
				return contextValueInt < valueInt
			case OperatorLessThanOrEquals:
				return contextValueInt <= valueInt
			}
		}
	}

	// Handle mixed numeric types
	if contextValueFloat, ok := contextValueFromPath.(float64); ok {
		if valueInt, ok := value.(int); ok {
			switch condition.Operator {
			case OperatorGreaterThan:
				return contextValueFloat > float64(valueInt)
			case OperatorGreaterThanOrEquals:
				return contextValueFloat >= float64(valueInt)
			case OperatorLessThan:
				return contextValueFloat < float64(valueInt)
			case OperatorLessThanOrEquals:
				return contextValueFloat <= float64(valueInt)
			}
		}
	}

	if contextValueInt, ok := contextValueFromPath.(int); ok {
		if valueFloat, ok := value.(float64); ok {
			switch condition.Operator {
			case OperatorGreaterThan:
				return float64(contextValueInt) > valueFloat
			case OperatorGreaterThanOrEquals:
				return float64(contextValueInt) >= valueFloat
			case OperatorLessThan:
				return float64(contextValueInt) < valueFloat
			case OperatorLessThanOrEquals:
				return float64(contextValueInt) <= valueFloat
			}
		}
	}

	// exists / notExists
	if condition.Operator == OperatorExists {
		return contextValueFromPath != nil
	} else if condition.Operator == OperatorNotExists {
		return contextValueFromPath == nil
	}

	// includes / notIncludes (where context value is an array)
	if contextValueArray, ok := contextValueFromPath.([]interface{}); ok {
		if valueStr, ok := value.(string); ok {
			switch condition.Operator {
			case OperatorIncludes:
				for _, item := range contextValueArray {
					if itemStr, ok := item.(string); ok && itemStr == valueStr {
						return true
					}
				}
				return false
			case OperatorNotIncludes:
				for _, item := range contextValueArray {
					if itemStr, ok := item.(string); ok && itemStr == valueStr {
						return false
					}
				}
				return true
			}
		}
	}

	return false
}
