package featurevisor

import (
	"fmt"
	"math"
	"reflect"
)

var unsafeMutationKeys = map[string]bool{
	"__proto__":   true,
	"prototype":   true,
	"constructor": true,
}

func mutationIndex(value interface{}) (int, bool) {
	number, ok := numericValue(value)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || math.Trunc(number) != number {
		return 0, false
	}
	return int(number), true
}

func variableObjectEntry(value interface{}, key string) (interface{}, bool) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || reflected.Kind() != reflect.Map || reflected.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	mapKey := reflect.ValueOf(key).Convert(reflected.Type().Key())
	item := reflected.MapIndex(mapKey)
	if !item.IsValid() {
		return nil, false
	}
	return item.Interface(), true
}

func variableObjectCopy(value interface{}) (map[string]interface{}, bool) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || reflected.Kind() != reflect.Map || reflected.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	result := make(map[string]interface{}, reflected.Len())
	iterator := reflected.MapRange()
	for iterator.Next() {
		result[iterator.Key().String()] = iterator.Value().Interface()
	}
	return result, true
}

func variableArrayValues(value interface{}) ([]interface{}, bool) {
	if values, ok := value.([]interface{}); ok {
		return values, true
	}
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || (reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array) {
		return nil, false
	}
	values := make([]interface{}, reflected.Len())
	for index := range values {
		values[index] = reflected.Index(index).Interface()
	}
	return values, true
}

func mutationSelector(value interface{}) (string, []interface{}, bool) {
	selectValue, ok := variableObjectEntry(value, "select")
	if !ok {
		return "", nil, false
	}
	propertyValue, ok := variableObjectEntry(selectValue, "property")
	if !ok {
		return "", nil, false
	}
	property, ok := propertyValue.(string)
	if !ok {
		return "", nil, false
	}
	valuesValue, ok := variableObjectEntry(selectValue, "values")
	if !ok {
		return "", nil, false
	}
	values, ok := primitiveSliceValues(valuesValue)
	return property, values, ok
}

func validateVariableMutation(operation VariableMutationOperation) error {
	switch operation.Op {
	case "set", "append", "prepend", "appendUnique", "removeValue", "before", "after", "remove":
	default:
		return fmt.Errorf("Invalid variable mutation operation")
	}
	valueSet := operation.ValueSet || operation.Value != nil || !operation.decoded
	if operation.Op != "remove" && !valueSet {
		return fmt.Errorf("Variable mutation operation requires a value")
	}
	if operation.Op == "appendUnique" || operation.Op == "removeValue" {
		if !isConditionPrimitive(operation.Value) {
			return fmt.Errorf("Variable mutation operation requires a primitive value")
		}
		if number, ok := numericValue(operation.Value); ok && (math.IsNaN(number) || math.IsInf(number, 0)) {
			return fmt.Errorf("Variable mutation operation requires a primitive value")
		}
	}
	path, ok := mutationPathParts(operation.Path)
	if !ok {
		return fmt.Errorf("Invalid variable mutation path")
	}
	for _, part := range path {
		if key, ok := part.(string); ok {
			if unsafeMutationKeys[key] {
				return fmt.Errorf("Unsafe variable mutation key")
			}
			continue
		}
		if _, ok := mutationIndex(part); ok {
			continue
		}
		property, _, ok := mutationSelector(part)
		if !ok {
			return fmt.Errorf("Invalid variable mutation path")
		}
		if unsafeMutationKeys[property] {
			return fmt.Errorf("Unsafe variable mutation key")
		}
	}
	return nil
}

func mutationPathParts(path interface{}) ([]interface{}, bool) {
	switch value := path.(type) {
	case []interface{}:
		return value, true
	case []VariableMutationPathPart:
		result := make([]interface{}, len(value))
		for index, part := range value {
			result[index] = part
		}
		return result, true
	default:
		return nil, false
	}
}

func selectedMutationIndex(values []interface{}, property string, candidates []interface{}) int {
	for index, item := range values {
		value, exists := variableObjectEntry(item, property)
		if !exists || !isConditionPrimitive(value) {
			continue
		}
		for _, candidate := range candidates {
			if strictPrimitiveEqual(value, candidate) {
				return index
			}
		}
	}
	return -1
}

func mutationPathKey(container interface{}, part interface{}) (interface{}, error) {
	if key, ok := part.(string); ok {
		if _, ok := variableObjectCopy(container); !ok {
			return nil, fmt.Errorf("Variable mutation expected an object")
		}
		return key, nil
	}
	values, ok := variableArrayValues(container)
	if !ok {
		return nil, fmt.Errorf("Variable mutation expected an array")
	}
	if index, ok := mutationIndex(part); ok {
		return index, nil
	}
	property, candidates, _ := mutationSelector(part)
	return selectedMutationIndex(values, property, candidates), nil
}

func mutationChild(container interface{}, key interface{}) (interface{}, bool) {
	if stringKey, ok := key.(string); ok {
		return variableObjectEntry(container, stringKey)
	}
	values, _ := variableArrayValues(container)
	index := key.(int)
	if index < 0 || index >= len(values) {
		return nil, false
	}
	return values[index], true
}

func mutationReplace(container interface{}, key interface{}, value interface{}) interface{} {
	if object, ok := variableObjectCopy(container); ok {
		object[key.(string)] = value
		return object
	}
	values, _ := variableArrayValues(container)
	next := append([]interface{}{}, values...)
	index := key.(int)
	if index == len(next) {
		return append(next, value)
	}
	next[index] = value
	return next
}

func updateMutationArray(values []interface{}, operation string, item interface{}) []interface{} {
	found := false
	for _, value := range values {
		if strictPrimitiveEqual(value, item) {
			found = true
			break
		}
	}
	if operation == "removeValue" {
		if !found {
			return values
		}
		next := make([]interface{}, 0, len(values))
		for _, value := range values {
			if !strictPrimitiveEqual(value, item) {
				next = append(next, value)
			}
		}
		return next
	}
	if operation == "appendUnique" && found {
		return values
	}
	if operation == "prepend" {
		return append([]interface{}{item}, values...)
	}
	return append(append([]interface{}{}, values...), item)
}

func updateMutationFinal(container interface{}, part interface{}, operation VariableMutationOperation) (interface{}, error) {
	if container == nil {
		return container, nil
	}
	if operation.Op == "before" || operation.Op == "after" {
		if _, ok := part.(string); ok {
			return nil, fmt.Errorf("Variable mutation before/after requires an array element path")
		}
	}
	key, err := mutationPathKey(container, part)
	if err != nil {
		return nil, err
	}
	current, found := mutationChild(container, key)
	if operation.Op == "remove" {
		if !found {
			return container, nil
		}
		if object, ok := variableObjectCopy(container); ok {
			delete(object, key.(string))
			return object, nil
		}
		containerValues, _ := variableArrayValues(container)
		values := append([]interface{}{}, containerValues...)
		index := key.(int)
		return append(values[:index], values[index+1:]...), nil
	}
	if operation.Op == "set" {
		containerValues, isArray := variableArrayValues(container)
		if index, ok := key.(int); ok && (!isArray || index < 0 || index > len(containerValues)) {
			return container, nil
		}
		if found && reflect.DeepEqual(current, operation.Value) {
			return container, nil
		}
		return mutationReplace(container, key, operation.Value), nil
	}
	if operation.Op == "before" || operation.Op == "after" {
		if !found {
			return container, nil
		}
		containerValues, _ := variableArrayValues(container)
		values := append([]interface{}{}, containerValues...)
		index := key.(int)
		if operation.Op == "after" {
			index++
		}
		values = append(values, nil)
		copy(values[index+1:], values[index:])
		values[index] = operation.Value
		return values, nil
	}
	if current == nil {
		if _, isIndex := key.(int); isIndex || operation.Op == "removeValue" {
			return container, nil
		}
	} else if _, ok := variableArrayValues(current); !ok {
		return nil, fmt.Errorf("Variable mutation expected an array")
	}
	values := []interface{}{}
	if current != nil {
		values, _ = variableArrayValues(current)
	}
	next := updateMutationArray(values, operation.Op, operation.Value)
	if reflect.ValueOf(next).Pointer() == reflect.ValueOf(values).Pointer() {
		return container, nil
	}
	return mutationReplace(container, key, next), nil
}

func updateVariableAtPath(value interface{}, operation VariableMutationOperation, offset int) (interface{}, error) {
	path, _ := mutationPathParts(operation.Path)
	if offset == len(path) {
		if operation.Op == "set" {
			return operation.Value, nil
		}
		if operation.Op == "remove" {
			return nil, fmt.Errorf("Cannot remove the root variable value")
		}
		if operation.Op == "before" || operation.Op == "after" {
			return nil, fmt.Errorf("Variable mutation before/after requires an array element path")
		}
		if value == nil {
			return value, nil
		}
		values, ok := variableArrayValues(value)
		if !ok {
			return nil, fmt.Errorf("Variable mutation expected an array")
		}
		return updateMutationArray(values, operation.Op, operation.Value), nil
	}
	if offset == len(path)-1 {
		return updateMutationFinal(value, path[offset], operation)
	}
	if value == nil {
		return value, nil
	}
	key, err := mutationPathKey(value, path[offset])
	if err != nil {
		return nil, err
	}
	current, found := mutationChild(value, key)
	if !found || current == nil {
		return value, nil
	}
	next, err := updateVariableAtPath(current, operation, offset+1)
	if err != nil {
		return nil, err
	}
	if reflect.DeepEqual(next, current) {
		return value, nil
	}
	return mutationReplace(value, key, next), nil
}

// ApplyVariableOperations applies compiled mutations without changing the input value.
func ApplyVariableOperations(value VariableValue, operations []VariableMutationOperation) (VariableValue, error) {
	if operations == nil {
		return nil, fmt.Errorf("Invalid variable mutation operations")
	}
	current := value
	for _, operation := range operations {
		if err := validateVariableMutation(operation); err != nil {
			return nil, err
		}
		next, err := updateVariableAtPath(current, operation, 0)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}
