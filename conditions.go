package featurevisor

import (
	"reflect"
	"regexp"
	"strings"
	"time"
)

func numericValue(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func primitiveSliceValues(value interface{}) ([]interface{}, bool) {
	if value == nil {
		return nil, false
	}

	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Array && reflected.Kind() != reflect.Slice {
		return nil, false
	}

	values := make([]interface{}, reflected.Len())
	for index := 0; index < reflected.Len(); index++ {
		item := reflected.Index(index).Interface()
		if !isConditionPrimitive(item) {
			return nil, false
		}
		values[index] = item
	}

	return values, true
}

func strictPrimitiveEqual(left interface{}, right interface{}) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if leftNumber, ok := numericValue(left); ok {
		rightNumber, rightIsNumber := numericValue(right)
		return rightIsNumber && leftNumber == rightNumber
	}
	switch leftValue := left.(type) {
	case string:
		rightValue, ok := right.(string)
		return ok && leftValue == rightValue
	case bool:
		rightValue, ok := right.(bool)
		return ok && leftValue == rightValue
	}
	return false
}

func isConditionPrimitive(value interface{}) bool {
	if value == nil {
		return true
	}
	if _, ok := value.(string); ok {
		return true
	}
	if _, ok := value.(bool); ok {
		return true
	}
	_, ok := numericValue(value)
	return ok
}

type getRegex func(regexString string, regexFlags string) *regexp.Regexp

// pathExists checks if a path exists in a context object
func pathExists(obj map[string]interface{}, path string) bool {
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

// getValueFromContext extracts a value from a context object using a dot-separated path
func getValueFromContext(obj map[string]interface{}, path string) interface{} {
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

// conditionIsMatched checks if a condition is matched given a context
func conditionIsMatched(
	condition PlainCondition,
	context Context,
	getRegex getRegex,
) bool {
	attribute := string(condition.Attribute)
	contextValueFromPath := getValueFromContext(context, attribute)
	attributeExists := pathExists(context, attribute)

	// Handle nil values
	if condition.Value == nil {
		if condition.Operator == OperatorExists {
			return attributeExists
		} else if condition.Operator == OperatorNotExists {
			return !attributeExists
		}
		return false
	}

	value := *condition.Value

	// equals / notEquals
	if condition.Operator == OperatorEquals {
		return attributeExists && strictPrimitiveEqual(contextValueFromPath, value)
	} else if condition.Operator == OperatorNotEquals {
		return !attributeExists || !strictPrimitiveEqual(contextValueFromPath, value)
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
				return false
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
				return false
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
	if valueArray, ok := primitiveSliceValues(value); ok {
		_, isString := contextValueFromPath.(string)
		_, isNumber := numericValue(contextValueFromPath)
		if !attributeExists || (!isString && !isNumber && contextValueFromPath != nil) {
			return false
		}
		matched := false
		for _, item := range valueArray {
			if strictPrimitiveEqual(item, contextValueFromPath) {
				matched = true
				break
			}
		}
		if condition.Operator == OperatorIn {
			return matched
		} else if condition.Operator == OperatorNotIn {
			return !matched
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
				result, err := compareVersions(contextValueStr, valueStr)
				if err != nil {
					panic(err)
				}
				return result == 0
			case OperatorSemverNotEquals:
				result, err := compareVersions(contextValueStr, valueStr)
				if err != nil {
					panic(err)
				}
				return result != 0
			case OperatorSemverGreaterThan:
				result, err := compareVersions(contextValueStr, valueStr)
				if err != nil {
					panic(err)
				}
				return result == 1
			case OperatorSemverGreaterThanOrEquals:
				result, err := compareVersions(contextValueStr, valueStr)
				if err != nil {
					panic(err)
				}
				return result >= 0
			case OperatorSemverLessThan:
				result, err := compareVersions(contextValueStr, valueStr)
				if err != nil {
					panic(err)
				}
				return result == -1
			case OperatorSemverLessThanOrEquals:
				result, err := compareVersions(contextValueStr, valueStr)
				if err != nil {
					panic(err)
				}
				return result <= 0
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

	// Numeric operations use the same cross-number comparison for every native Go number type.
	if contextValueNumber, contextIsNumber := numericValue(contextValueFromPath); contextIsNumber {
		if conditionValueNumber, conditionIsNumber := numericValue(value); conditionIsNumber {
			switch condition.Operator {
			case OperatorGreaterThan:
				return contextValueNumber > conditionValueNumber
			case OperatorGreaterThanOrEquals:
				return contextValueNumber >= conditionValueNumber
			case OperatorLessThan:
				return contextValueNumber < conditionValueNumber
			case OperatorLessThanOrEquals:
				return contextValueNumber <= conditionValueNumber
			}
		}
	}

	// exists / notExists
	if condition.Operator == OperatorExists {
		return attributeExists
	} else if condition.Operator == OperatorNotExists {
		return !attributeExists
	}

	// includes / notIncludes (where context value is an array)
	if contextValueArray, ok := primitiveSliceValues(contextValueFromPath); ok {
		if isConditionPrimitive(value) {
			switch condition.Operator {
			case OperatorIncludes:
				for _, item := range contextValueArray {
					if strictPrimitiveEqual(item, value) {
						return true
					}
				}
				return false
			case OperatorNotIncludes:
				for _, item := range contextValueArray {
					if strictPrimitiveEqual(item, value) {
						return false
					}
				}
				return true
			}
		}
	}

	return false
}
