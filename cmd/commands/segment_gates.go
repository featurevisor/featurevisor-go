package commands

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

const segmentGatePrefix = "__featurevisor_segment_gate__/"

func segmentGateKeys(datafile featurevisor.DatafileContent, definitions map[string]interface{}) map[string]string {
	keys := map[string]string{}
	for segment := range definitions {
		key := segmentGatePrefix + segment
		for {
			if _, exists := datafile.Features[key]; !exists {
				break
			}
			key = "_" + key
		}
		keys[segment] = key
	}
	return keys
}

func installSegmentGates(instance *featurevisor.Featurevisor, datafile featurevisor.DatafileContent, definitions map[string]interface{}, keys map[string]string) {
	if len(keys) == 0 {
		return
	}
	patch := featurevisor.DatafileContent{SchemaVersion: "4", Revision: instance.GetRevision(), Features: map[string]featurevisor.Feature{}, Segments: map[string]featurevisor.Segment{}}
	for segment, key := range keys {
		if _, exists := datafile.Segments[segment]; !exists {
			if definition, ok := definitions[segment].(map[string]interface{}); ok {
				patch.Segments[segment] = featurevisor.Segment{Conditions: definition["conditions"]}
			}
		}
		patch.Features[key] = featurevisor.Feature{BucketBy: []string{}, Conditions: segment, Traffic: []featurevisor.Traffic{{Key: "segment", Conditions: "*", Percentage: 100000}}}
	}
	instance.SetDatafile(patch)
}

// Inspect the entire transitive expression without invoking getters or warming caches.
func validateSegmentReferences(key string, definitions map[string]interface{}, datafile featurevisor.DatafileContent) error {
	type reference struct {
		Kind, Key, Variable string
		HasVariable         bool
	}
	pending := []reference{{Kind: "segment", Key: key}}
	seen := map[reference]bool{}
	var walk func(interface{}, bool) error
	walk = func(value interface{}, legacyFeatures bool) error {
		switch v := value.(type) {
		case string:
			if v == "*" {
				return nil
			}
			trimmed := strings.TrimSpace(v)
			if len(trimmed) > 0 && strings.ContainsRune("{[\\\"", rune(trimmed[0])) {
				var parsed interface{}
				if err := json.Unmarshal([]byte(v), &parsed); err != nil {
					return err
				}
				return walk(parsed, legacyFeatures)
			}
			kind := "segment"
			if legacyFeatures {
				kind = "feature"
			}
			pending = append(pending, reference{Kind: kind, Key: v})
		case []interface{}:
			for _, child := range v {
				if err := walk(child, legacyFeatures); err != nil {
					return err
				}
			}
		case map[string]interface{}:
			if variable, ok := v["variable"].(string); ok {
				if feature, ok := v["feature"].(string); ok {
					pending = append(pending, reference{Kind: "feature", Key: feature, Variable: variable, HasVariable: true})
				} else {
					pending = append(pending, reference{Kind: "variable", Key: variable})
				}
			} else if feature, ok := v["feature"].(string); ok {
				pending = append(pending, reference{Kind: "feature", Key: feature})
			} else {
				for _, op := range []string{"and", "or", "not"} {
					if err := walk(v[op], legacyFeatures); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	selectors := func(value interface{}) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		var fields map[string]interface{}
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		for _, field := range []string{"conditions", "segments", "requiredFeatures"} {
			if err := walk(fields[field], field == "requiredFeatures"); err != nil {
				return err
			}
		}
		return nil
	}
	for index := 0; index < len(pending); index++ {
		ref := pending[index]
		if seen[ref] {
			continue
		}
		seen[ref] = true
		switch ref.Kind {
		case "segment":
			if definition, ok := definitions[ref.Key].(map[string]interface{}); ok {
				conditions, exists := definition["conditions"]
				if !exists {
					return fmt.Errorf("segment %q definition has no conditions", ref.Key)
				}
				if err := walk(conditions, false); err != nil {
					return err
				}
			} else if segment, ok := datafile.Segments[ref.Key]; ok {
				if err := walk(segment.Conditions, false); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("segment %q definition is missing or invalid", ref.Key)
			}
		case "feature":
			feature, exists := datafile.Features[ref.Key]
			if !exists {
				return fmt.Errorf("feature %q is missing from selected datafile", ref.Key)
			}
			if ref.HasVariable {
				if _, exists := feature.VariablesSchema[ref.Variable]; !exists {
					return fmt.Errorf("variable %q is missing from feature %q", ref.Variable, ref.Key)
				}
			}
			if err := selectors(feature); err != nil {
				return err
			}
			for _, force := range feature.Force {
				if err := selectors(force); err != nil {
					return err
				}
			}
			for _, traffic := range feature.Traffic {
				if err := selectors(traffic); err != nil {
					return err
				}
				if ref.HasVariable {
					for _, override := range traffic.VariableOverrides[ref.Variable] {
						if err := selectors(override); err != nil {
							return err
						}
					}
				}
			}
			if ref.HasVariable {
				for _, variation := range feature.Variations {
					for _, override := range variation.VariableOverrides[ref.Variable] {
						if err := selectors(override); err != nil {
							return err
						}
					}
				}
			}
		case "variable":
			variable, exists := datafile.Variables[ref.Key]
			if !exists {
				return fmt.Errorf("variable %q is missing from selected datafile", ref.Key)
			}
			if err := selectors(variable); err != nil {
				return err
			}
			for _, override := range variable.Overrides {
				if err := selectors(override); err != nil {
					return err
				}
			}
			for _, step := range variable.OverrideProgram {
				if err := selectors(step); err != nil {
					return err
				}
			}
			if alias, ok := variable.Compose.(string); ok {
				pending = append(pending, reference{Kind: "variable", Key: alias})
			} else if variable.Compose != nil {
				raw, err := json.Marshal(variable.Compose)
				if err != nil {
					return err
				}
				var entries []map[string]interface{}
				if err := json.Unmarshal(raw, &entries); err != nil {
					return err
				}
				for _, entry := range entries {
					feature, hasFeature := entry["feature"].(string)
					variable, hasVariable := entry["variable"].(string)
					if hasFeature {
						pending = append(pending, reference{Kind: "feature", Key: feature, Variable: variable, HasVariable: hasVariable})
					} else if hasVariable {
						pending = append(pending, reference{Kind: "variable", Key: variable})
					}
				}
			}
		}
	}
	return nil
}

func segmentNeedsDatafile(key string, definitions map[string]interface{}) bool {
	seen := map[string]bool{}
	var expression func(interface{}) bool
	expression = func(value interface{}) bool {
		switch v := value.(type) {
		case string:
			if seen[v] || v == "*" {
				return false
			}
			seen[v] = true
			var parsed interface{}
			if json.Unmarshal([]byte(v), &parsed) == nil {
				return expression(parsed)
			}
			if definition, ok := definitions[v].(map[string]interface{}); ok {
				return expression(definition["conditions"])
			}
		case []interface{}:
			for _, child := range v {
				if expression(child) {
					return true
				}
			}
		case map[string]interface{}:
			if _, exists := v["feature"]; exists {
				return true
			}
			if _, exists := v["variable"]; exists {
				return true
			}
			for _, op := range []string{"and", "or", "not"} {
				if expression(v[op]) {
					return true
				}
			}
		}
		return false
	}
	return expression(key)
}

func runSegmentSpec(assertion map[string]interface{}, key string, test map[string]interface{}, fallback map[string]interface{}, cache map[string]interface{}, level string) AssertionResult {
	definitions := segmentDefinitionsFromTest(test)
	if len(definitions) == 0 {
		definitions = fallback
	}
	selected := datafileCacheKeyForAssertion(assertion, cache)
	raw, found := cache[selected]
	environment, _ := assertion["environment"].(string)
	target, _ := assertion["target"].(string)
	if !found && (environment != "" || target != "" || segmentNeedsDatafile(key, definitions)) {
		return missingDatafileAssertion(assertion, selected)
	}
	content := featurevisor.DatafileContent{SchemaVersion: "4", Revision: "segments", Features: map[string]featurevisor.Feature{}, Segments: map[string]featurevisor.Segment{}}
	if found {
		content = datafileContentFromRaw(raw)
	}
	return runSegmentWithSDK(assertion, key, definitions, content, level)
}

func runSegmentWithSDK(assertion map[string]interface{}, key string, definitions map[string]interface{}, content featurevisor.DatafileContent, level string) AssertionResult {
	started := time.Now()
	if err := validateSegmentReferences(key, definitions, content); err != nil {
		return AssertionResult{HasError: true, Errors: "      ✘ " + err.Error() + "\n", Duration: time.Since(started).Seconds()}
	}
	instance, diagnostics := buildInstanceForAssertion(content, level, assertion)
	defer instance.Close()
	keys := segmentGateKeys(content, definitions)
	installSegmentGates(instance, content, definitions, keys)
	resolver := nativeCheckResolver{getter: instance, diagnostics: diagnostics, segmentDefinitions: definitions, datafile: content, segmentGates: keys}
	if result := preflightAssertionTree(assertion, "", resolver, content, false); result.HasError {
		return result
	}
	actual, err := resolver.ResolveSegment(key)
	result := AssertionResult{Duration: time.Since(started).Seconds()}
	if err != nil {
		result.HasError = true
		result.Errors = "      ✘ " + err.Error() + "\n"
		return result
	}
	if expected, exists := assertion["expectedToMatch"].(bool); exists && actual != expected {
		result.HasError = true
		result.Errors = fmt.Sprintf("      ✘ expectedToMatch: expected %v but received %v\n", expected, actual)
		result.ReportErrors = []reportError{expectationReportError("segment", []interface{}{"expectedToMatch"}, expected, actual)}
	}
	return result
}
