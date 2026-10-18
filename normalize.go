package featurevisor

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
)

// A private marker cannot become a successful operand through negation.
type invalidSelector struct{}
type selectorParseFailure struct{}
type nullSelector struct{}

type normalizedEntitySlot struct {
	mu    sync.Mutex
	value interface{}
}

func (invalidSelector) MarshalJSON() ([]byte, error) { return []byte(`{"or":[]}`), nil }
func (nullSelector) MarshalJSON() ([]byte, error)    { return []byte(`null`), nil }

func expressionValue(value interface{}) interface{} {
	switch value.(type) {
	case nil, string, map[string]interface{}, []interface{}, invalidSelector, nullSelector:
		return value
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Slice {
		result := make([]interface{}, rv.Len())
		for index := range result {
			result[index] = expressionValue(rv.Index(index).Interface())
		}
		return result
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var result interface{}
	if json.Unmarshal(raw, &result) != nil {
		return value
	}
	return result
}

func legacyExpression(value interface{}, features bool) interface{} {
	value = expressionValue(value)
	switch v := value.(type) {
	case string:
		if features {
			return map[string]interface{}{"feature": v}
		}
	case []interface{}:
		if len(v) == 0 {
			return "*"
		}
		result := make([]interface{}, len(v))
		for i, child := range v {
			result[i] = legacyExpression(child, features)
		}
		return result
	case map[string]interface{}:
		for _, op := range []string{"and", "or", "not"} {
			if children, ok := v[op]; ok {
				children = expressionValue(children)
				if op == "and" {
					if list, ok := children.([]interface{}); ok && len(list) == 0 {
						return "*"
					}
				}
				if list, ok := children.([]interface{}); ok {
					result := make([]interface{}, len(list))
					for i, child := range list {
						result[i] = legacyExpression(child, features)
					}
					return map[string]interface{}{op: result}
				}
				return map[string]interface{}{op: legacyExpression(children, features)}
			}
		}
	}
	return value
}

type entityNormalizer struct {
	kind, key string
	errors    []FeaturevisorDiagnostic
}

func (n *entityNormalizer) parse(value interface{}) interface{} {
	if raw, ok := value.(string); ok {
		trimmed := strings.TrimSpace(raw)
		if len(trimmed) != 0 && strings.ContainsRune("{[\"", rune(trimmed[0])) {
			var parsed interface{}
			if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
				n.errors = append(n.errors, FeaturevisorDiagnostic{Level: LogLevelError, Code: "conditions_parse_error", Message: "Error parsing conditions", OriginalError: err,
					Details: map[string]interface{}{"conditions": raw, "entityType": n.kind, "entityKey": n.key}})
				return invalidSelector{}
			}
			return parsed
		}
	}
	return expressionValue(value)
}

func (n *entityNormalizer) selectConditions(conditions, segments interface{}, required []RequiredFeature, legacy bool) interface{} {
	parts := []interface{}{}
	if conditions != nil {
		value := n.parse(conditions)
		if legacy {
			value = legacyExpression(value, false)
		}
		parts = append(parts, value)
	}
	if segments != nil {
		parts = append(parts, legacyExpression(n.parse(segments), false))
	}
	if len(required) > 0 {
		parts = append(parts, legacyExpression(required, true))
	}
	for _, part := range parts {
		if _, bad := part.(invalidSelector); bad {
			return part
		}
	}
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 {
		return parts[0]
	}
	combined := []interface{}{}
	for _, part := range parts {
		if entries, ok := part.([]interface{}); ok {
			if len(entries) == 0 {
				combined = append(combined, map[string]interface{}{"and": []interface{}{}})
			} else {
				combined = append(combined, entries...)
			}
		} else {
			combined = append(combined, part)
		}
	}
	return combined
}

func (n *entityNormalizer) overrides(entries []VariableOverride, legacy bool) []VariableOverride {
	if entries == nil {
		return nil
	}
	result := append([]VariableOverride{}, entries...)
	for i := range result {
		o := &result[i]
		o.Conditions = n.selectConditions(o.Conditions, o.Segments, o.RequiredFeatures, legacy)
		o.Segments, o.RequiredFeatures, o.RequiredVariables = nil, nil, nil
	}
	return result
}

func (n *entityNormalizer) overrideMap(entries map[string][]VariableOverride, legacy bool) map[string][]VariableOverride {
	if entries == nil {
		return nil
	}
	result := make(map[string][]VariableOverride, len(entries))
	for key, value := range entries {
		result[key] = n.overrides(value, legacy)
	}
	return result
}

func normalizeEntity(kind, key string, entity interface{}, legacy bool) (interface{}, []FeaturevisorDiagnostic) {
	n := &entityNormalizer{kind: kind, key: key}
	var result interface{}
	switch value := entity.(type) {
	case Feature:
		if value.VariablesSchema != nil {
			schemas := make(map[string]VariableSchema, len(value.VariablesSchema))
			for key, schema := range value.VariablesSchema {
				schema.defaultValueSet = schema.defaultValueSet || schema.DefaultValue != nil
				schema.disabledValueSet = schema.disabledValueSet || schema.DisabledValue != nil
				schemas[key] = schema
			}
			value.VariablesSchema = schemas
		}
		value.Conditions = n.selectConditions(value.Conditions, nil, value.RequiredFeatures, false)
		value.RequiredFeatures, value.RequiredVariables = nil, nil
		value.Traffic = append([]Traffic{}, value.Traffic...)
		for i := range value.Traffic {
			t := &value.Traffic[i]
			t.Conditions = n.selectConditions(t.Conditions, t.Segments, nil, false)
			t.Segments = nil
			t.VariableOverrides = n.overrideMap(t.VariableOverrides, legacy)
		}
		value.Force = append([]Force(nil), value.Force...)
		for i := range value.Force {
			f := &value.Force[i]
			f.Conditions = n.selectConditions(f.Conditions, f.Segments, nil, legacy)
			f.Segments = nil
		}
		value.Variations = append([]Variation(nil), value.Variations...)
		for i := range value.Variations {
			value.Variations[i].VariableOverrides = n.overrideMap(value.Variations[i].VariableOverrides, legacy)
		}
		result = value
	case Segment:
		value.Conditions = n.parse(value.Conditions)
		if legacy {
			value.Conditions = legacyExpression(value.Conditions, false)
		}
		result = value
	case GlobalVariable:
		value.Conditions = n.selectConditions(value.Conditions, nil, value.RequiredFeatures, false)
		value.RequiredFeatures, value.RequiredVariables = nil, nil
		value.Overrides = n.overrides(value.Overrides, legacy)
		if value.OverrideProgram != nil {
			value.OverrideProgram = append([]VariableOverrideStep{}, value.OverrideProgram...)
			for i := range value.OverrideProgram {
				o := &value.OverrideProgram[i]
				o.Conditions = n.selectConditions(o.Conditions, o.Segments, o.RequiredFeatures, false)
				o.Segments, o.RequiredFeatures, o.RequiredVariables = nil, nil, nil
			}
		}
		result = value
	}
	return result, n.errors
}

func normalizeLegacyDatafile(data DatafileContent) (DatafileContent, []FeaturevisorDiagnostic) {
	if data.SchemaVersion != "2" {
		return data, nil
	}
	errors := []FeaturevisorDiagnostic{}
	features, segments, variables := map[string]Feature{}, map[string]Segment{}, map[string]GlobalVariable{}
	for k, v := range data.Features {
		value, diagnostics := normalizeEntity("feature", k, v, true)
		features[k] = value.(Feature)
		errors = append(errors, diagnostics...)
	}
	for k, v := range data.Segments {
		value, diagnostics := normalizeEntity("segment", k, v, true)
		segments[k] = value.(Segment)
		errors = append(errors, diagnostics...)
	}
	for k, v := range data.Variables {
		value, diagnostics := normalizeEntity("variable", k, v, true)
		variables[k] = value.(GlobalVariable)
		errors = append(errors, diagnostics...)
	}
	data.Features, data.Segments, data.Variables = features, segments, variables
	return data, errors
}

func (d *instanceEvaluationDataProvider) normalizedEntity(kind, key string, raw interface{}) interface{} {
	identity := kind + ":" + key
	d.entityMu.Lock()
	if value, exists := d.entities[identity]; exists {
		d.entityMu.Unlock()
		return value
	}
	slot := d.entitySlots[identity]
	if slot == nil {
		slot = &normalizedEntitySlot{}
		d.entitySlots[identity] = slot
	}
	d.entityMu.Unlock()
	slot.mu.Lock()
	var diagnostics []FeaturevisorDiagnostic
	if slot.value == nil {
		value, errors := normalizeEntity(kind, key, raw, false)
		diagnostics = errors
		switch entity := value.(type) {
		case Feature:
			slot.value = &entity
		case Segment:
			slot.value = &entity
		case GlobalVariable:
			slot.value = &entity
		}
	}
	value := slot.value
	slot.mu.Unlock()
	d.entityMu.Lock()
	d.entities[identity] = value
	d.entityMu.Unlock()
	for _, diagnostic := range diagnostics {
		d.diagnostics.Error(logMessage(diagnostic.Message), logDetails{"details": diagnostic.Details, "originalError": diagnostic.OriginalError})
	}
	return value
}
