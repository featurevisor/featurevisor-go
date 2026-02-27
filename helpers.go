package featurevisor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
)

// GetValueByType converts a value to the specified type
// This function mirrors the TypeScript getValueByType function
func GetValueByType(value interface{}, fieldType string) interface{} {
	if value == nil {
		return nil
	}

	switch fieldType {
	case "string":
		if str, ok := value.(string); ok {
			return str
		}
		return nil
	case "integer":
		switch v := value.(type) {
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		case int:
			return v
		case float64:
			return int(v)
		}
		return nil
	case "double":
		switch v := value.(type) {
		case string:
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				return n
			}
		case float64:
			return v
		case int:
			return float64(v)
		}
		return nil
	case "boolean":
		return value == true
	case "array":
		if arr, ok := value.([]interface{}); ok {
			return arr
		}
		return nil
	case "object":
		if obj, ok := value.(map[string]interface{}); ok {
			return obj
		}
		return nil
	case "json":
		// JSON type is handled specially in the calling code
		return value
	default:
		return value
	}
}

func convertToTypedValue[T any](value interface{}) (T, bool) {
	var zero T
	if typed, ok := value.(T); ok {
		return typed, true
	}

	bytes, err := json.Marshal(value)
	if err != nil {
		return zero, false
	}

	var converted T
	if err := json.Unmarshal(bytes, &converted); err != nil {
		return zero, false
	}

	return converted, true
}

func ToTypedArray[T any](value interface{}) []T {
	if value == nil {
		return nil
	}

	if typed, ok := value.([]T); ok {
		return typed
	}

	values, ok := value.([]interface{})
	if !ok {
		return nil
	}

	result := make([]T, len(values))
	for i, item := range values {
		typedItem, ok := convertToTypedValue[T](item)
		if !ok {
			return nil
		}
		result[i] = typedItem
	}

	return result
}

func ToTypedObject[T any](value interface{}) *T {
	if value == nil {
		return nil
	}

	typed, ok := convertToTypedValue[T](value)
	if !ok {
		return nil
	}

	return &typed
}

func setPointerTargetToZero(out interface{}) error {
	if out == nil {
		return fmt.Errorf("output argument is required")
	}

	outValue := reflect.ValueOf(out)
	if outValue.Kind() != reflect.Ptr || outValue.IsNil() {
		return fmt.Errorf("output argument must be a non-nil pointer")
	}

	target := outValue.Elem()
	if !target.CanSet() {
		return fmt.Errorf("output argument cannot be set")
	}

	target.Set(reflect.Zero(target.Type()))
	return nil
}

func decodeInto(value interface{}, out interface{}) error {
	if err := setPointerTargetToZero(out); err != nil {
		return err
	}
	if value == nil {
		return nil
	}

	bytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal variable value: %w", err)
	}

	if err := json.Unmarshal(bytes, out); err != nil {
		return fmt.Errorf("failed to decode variable value into output: %w", err)
	}

	return nil
}

func parseVariableIntoArgs(args ...interface{}) (Context, OverrideOptions, interface{}, error) {
	context := Context{}
	options := OverrideOptions{}
	var out interface{}

	for _, arg := range args {
		switch value := arg.(type) {
		case Context:
			context = value
		case map[string]interface{}:
			context = Context(value)
		case OverrideOptions:
			options = value
		default:
			if out != nil {
				return Context{}, OverrideOptions{}, nil, fmt.Errorf("multiple output arguments provided")
			}
			out = arg
		}
	}

	if out == nil {
		return Context{}, OverrideOptions{}, nil, fmt.Errorf("missing output pointer argument")
	}

	return context, options, out, nil
}
