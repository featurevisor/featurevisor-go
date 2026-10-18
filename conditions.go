package featurevisor

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var portableTimestamp = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$`)

func normalizePortableRegex(pattern string) string {
	inside := map[byte]string{'d': "0-9", 'D': `\x{0}-\x{2F}\x{3A}-\x{10FFFF}`, 'w': "A-Za-z0-9_", 'W': `\x{0}-\x{2F}\x{3A}-\x{40}\x{5B}-\x{5E}\x{60}\x{7B}-\x{10FFFF}`}
	outside := map[byte]string{'d': "[0-9]", 'D': "[^0-9]", 'w': "(?-i:[A-Za-z0-9_])", 'W': "(?-i:[^A-Za-z0-9_])"}
	var result strings.Builder
	result.Grow(len(pattern))
	inClass := false
	for index := 0; index < len(pattern); index++ {
		char := pattern[index]
		if char == '\\' && index+1 < len(pattern) {
			index++
			code := pattern[index]
			replacement := outside[code]
			if inClass {
				replacement = inside[code]
			}
			if replacement == "" {
				result.WriteByte('\\')
				result.WriteByte(code)
			} else {
				result.WriteString(replacement)
			}
			continue
		}
		if char == '[' && !inClass {
			inClass = true
		} else if char == ']' && inClass {
			inClass = false
		}
		result.WriteByte(char)
	}
	return result.String()
}

func parsePortableDate(value interface{}) (time.Time, bool) {
	var parsed time.Time
	switch v := value.(type) {
	case time.Time:
		_, offset := v.Zone()
		if v.Year() < 1 || v.Year() > 9999 || offset < -14*3600 || offset > 14*3600 {
			return time.Time{}, false
		}
		parsed = v
	case string:
		if !portableTimestamp.MatchString(v) {
			return time.Time{}, false
		}
		if v[:4] == "0000" {
			return time.Time{}, false
		}
		if len(v) >= 6 && v[len(v)-1] != 'Z' {
			hour, _ := strconv.Atoi(v[len(v)-5 : len(v)-3])
			minute, _ := strconv.Atoi(v[len(v)-2:])
			if hour > 14 || minute > 59 || (hour == 14 && minute != 0) {
				return time.Time{}, false
			}
		}
		var err error
		parsed, err = time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return time.Time{}, false
		}
	default:
		return time.Time{}, false
	}
	parsed = parsed.UTC().Truncate(time.Millisecond)
	if parsed.Year() < 1 || parsed.Year() > 9999 {
		return time.Time{}, false
	}
	return parsed, true
}

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
	reflected, ok := conditionArray(value)
	if !ok {
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
		return conditionValueIsNull(left) && conditionValueIsNull(right)
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
	return conditionValueIsNull(left) && conditionValueIsNull(right)
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
	if _, ok := numericValue(value); ok {
		return true
	}
	return conditionValueIsNull(value)
}

func conditionValueIsEmpty(value interface{}) (bool, bool) {
	if value == nil {
		return false, false
	}
	if stringValue, ok := value.(string); ok {
		return len(stringValue) == 0, true
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Array:
		return reflected.Len() == 0, true
	case reflect.Slice:
		if reflected.IsNil() {
			return false, false
		}
		return reflected.Len() == 0, true
	case reflect.Map:
		if reflected.Type().Key().Kind() == reflect.String && !reflected.IsNil() {
			return reflected.Len() == 0, true
		}
	}

	return false, false
}

func conditionValueIsNull(value interface{}) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Slice, reflect.Map, reflect.Ptr, reflect.Interface:
		return reflected.IsNil()
	}
	return false
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
	if condition.Operator == OperatorIsNull {
		return attributeExists && conditionValueIsNull(contextValueFromPath)
	}
	if condition.Operator == OperatorNotNull {
		return attributeExists && !conditionValueIsNull(contextValueFromPath)
	}
	if condition.Operator == OperatorEmpty || condition.Operator == OperatorNotEmpty {
		isEmpty, supported := conditionValueIsEmpty(contextValueFromPath)
		if !attributeExists || !supported {
			return false
		}
		if condition.Operator == OperatorEmpty {
			return isEmpty
		}
		return !isEmpty
	}

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

	if condition.Operator == OperatorIncludesAny || condition.Operator == OperatorNotIncludesAny {
		actual, actualOK := conditionArray(contextValueFromPath)
		candidates, candidatesOK := conditionArray(value)
		if !attributeExists || !actualOK || !candidatesOK {
			return false
		}
		for index := 0; index < candidates.Len(); index++ {
			if !isConditionPrimitive(candidates.Index(index).Interface()) {
				return false
			}
		}
		for index := 0; index < candidates.Len(); index++ {
			candidate := candidates.Index(index).Interface()
			for item := 0; item < actual.Len(); item++ {
				if strictPrimitiveEqual(actual.Index(item).Interface(), candidate) {
					return condition.Operator == OperatorIncludesAny
				}
			}
		}
		return condition.Operator == OperatorNotIncludesAny
	}

	// equals / notEquals
	if condition.Operator == OperatorEquals {
		return attributeExists && strictPrimitiveEqual(contextValueFromPath, value)
	} else if condition.Operator == OperatorNotEquals {
		return !attributeExists || !strictPrimitiveEqual(contextValueFromPath, value)
	}

	// before / after (date comparisons)
	if condition.Operator == OperatorBefore || condition.Operator == OperatorAfter {
		dateInContext, contextOK := parsePortableDate(contextValueFromPath)
		dateInCondition, conditionOK := parsePortableDate(value)
		if !contextOK || !conditionOK {
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
		if !attributeExists || (!isString && !isNumber && !conditionValueIsNull(contextValueFromPath)) {
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

func conditionArray(value interface{}) (reflect.Value, bool) {
	if value == nil {
		return reflect.Value{}, false
	}
	array := reflect.ValueOf(value)
	if array.Kind() == reflect.Array {
		return array, true
	}
	return array, array.Kind() == reflect.Slice && !array.IsNil()
}
