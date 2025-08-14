package sdk

import (
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
		switch v := value.(type) {
		case bool:
			return v
		case string:
			return v == "true"
		case int:
			return v != 0
		}
		return false
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
