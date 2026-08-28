package featurevisor

import (
	"encoding/json"
	"reflect"
)

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func changedEntityKeys[T any](before map[string]T, after map[string]T, hash func(T) string) map[string]bool {
	changed := map[string]bool{}
	for key, oldValue := range before {
		newValue, ok := after[key]
		if !ok || hash(oldValue) != hash(newValue) {
			changed[key] = true
		}
	}
	for key, newValue := range after {
		oldValue, ok := before[key]
		if !ok || hash(oldValue) != hash(newValue) {
			changed[key] = true
		}
	}
	return changed
}

func collectSegmentKeys(value interface{}, result map[string]bool) {
	if raw, ok := value.(string); ok {
		if raw == "*" {
			return
		}
		if len(raw) > 0 && (raw[0] == '{' || raw[0] == '[') {
			var parsed interface{}
			if json.Unmarshal([]byte(raw), &parsed) == nil {
				collectSegmentKeys(parsed, result)
				return
			}
		}
		result[raw] = true
		return
	}
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			collectSegmentKeys(item, result)
		}
	case map[string]interface{}:
		for _, item := range typed {
			collectSegmentKeys(item, result)
		}
	}
}

func collectRequiredFeatureKeys(values []Required, result map[string]bool) {
	for _, value := range values {
		key, _, _, ok := requiredFeatureParts(value)
		if ok {
			result[key] = true
		}
	}
}

func featureDependencies(feature Feature) (map[string]bool, map[string]bool) {
	segments, features := map[string]bool{}, map[string]bool{}
	required := feature.RequiredFeatures
	if len(required) == 0 {
		required = feature.Required
	}
	collectRequiredFeatureKeys(required, features)
	for _, traffic := range feature.Traffic {
		collectSegmentKeys(traffic.Segments, segments)
		for _, overrides := range traffic.VariableOverrides {
			for _, override := range overrides {
				collectSegmentKeys(override.Segments, segments)
				collectRequiredFeatureKeys(override.RequiredFeatures, features)
			}
		}
	}
	for _, force := range feature.Force {
		collectSegmentKeys(force.Segments, segments)
	}
	for _, variation := range feature.Variations {
		for _, overrides := range variation.VariableOverrides {
			for _, override := range overrides {
				collectSegmentKeys(override.Segments, segments)
				collectRequiredFeatureKeys(override.RequiredFeatures, features)
			}
		}
	}
	return segments, features
}

func variableDependencies(variable GlobalVariable) (map[string]bool, map[string]bool) {
	segments, features := map[string]bool{}, map[string]bool{}
	collectRequiredFeatureKeys(variable.RequiredFeatures, features)
	for _, override := range variable.Overrides {
		collectSegmentKeys(override.Segments, segments)
		collectRequiredFeatureKeys(override.RequiredFeatures, features)
	}
	return segments, features
}

func affectedDatafileEntities(previous, next *instanceEvaluationDataProvider) ([]string, []string) {
	oldFeatures, newFeatures := map[string]Feature{}, map[string]Feature{}
	oldVariables, newVariables := map[string]GlobalVariable{}, map[string]GlobalVariable{}
	oldSegments, newSegments := map[string]Segment{}, map[string]Segment{}
	if previous != nil {
		for key, value := range previous.features {
			oldFeatures[key] = value
		}
		for key, value := range previous.variables {
			oldVariables[key] = value
		}
		for key, value := range previous.segments {
			oldSegments[key] = value
		}
	}
	if next != nil {
		for key, value := range next.features {
			newFeatures[key] = value
		}
		for key, value := range next.variables {
			newVariables[key] = value
		}
		for key, value := range next.segments {
			newSegments[key] = value
		}
	}
	changedFeatures := changedEntityKeys(oldFeatures, newFeatures, func(value Feature) string {
		if value.Hash != nil {
			return *value.Hash
		}
		raw, _ := json.Marshal(value)
		return string(raw)
	})
	changedVariables := changedEntityKeys(oldVariables, newVariables, func(value GlobalVariable) string {
		if value.Hash != nil {
			return *value.Hash
		}
		raw, _ := json.Marshal(value)
		return string(raw)
	})
	changedSegments := map[string]bool{}
	for key, value := range oldSegments {
		if nextValue, ok := newSegments[key]; !ok || !reflect.DeepEqual(value, nextValue) {
			changedSegments[key] = true
		}
	}
	for key, value := range newSegments {
		if oldValue, ok := oldSegments[key]; !ok || !reflect.DeepEqual(value, oldValue) {
			changedSegments[key] = true
		}
	}
	allFeatures := map[string]Feature{}
	for key, value := range oldFeatures {
		allFeatures[key] = value
	}
	for key, value := range newFeatures {
		allFeatures[key] = value
	}
	for updated := true; updated; {
		updated = false
		for key, feature := range allFeatures {
			if changedFeatures[key] {
				continue
			}
			segments, required := featureDependencies(feature)
			dependent := false
			for segment := range segments {
				if changedSegments[segment] {
					dependent = true
				}
			}
			for requiredKey := range required {
				if changedFeatures[requiredKey] {
					dependent = true
				}
			}
			if dependent {
				changedFeatures[key] = true
				updated = true
			}
		}
	}
	allVariables := map[string]GlobalVariable{}
	for key, value := range oldVariables {
		allVariables[key] = value
	}
	for key, value := range newVariables {
		allVariables[key] = value
	}
	for key, variable := range allVariables {
		if changedVariables[key] {
			continue
		}
		segments, required := variableDependencies(variable)
		dependent := false
		for segment := range segments {
			if changedSegments[segment] {
				dependent = true
			}
		}
		for requiredKey := range required {
			if changedFeatures[requiredKey] {
				dependent = true
			}
		}
		if dependent {
			changedVariables[key] = true
		}
	}
	features, variables := []string{}, []string{}
	for key := range changedFeatures {
		features = append(features, key)
	}
	for key := range changedVariables {
		variables = append(variables, key)
	}
	return features, variables
}

// getParamsForDatafileSetEvent gets parameters for datafile set event
func getParamsForDatafileSetEvent(
	previousInstanceEvaluationDataProvider *instanceEvaluationDataProvider,
	newInstanceEvaluationDataProvider *instanceEvaluationDataProvider,
	replace bool,
) logDetails {
	previousRevision := ""
	if previousInstanceEvaluationDataProvider != nil {
		previousRevision = previousInstanceEvaluationDataProvider.GetRevision()
	}

	newRevision := ""
	if newInstanceEvaluationDataProvider != nil {
		newRevision = newInstanceEvaluationDataProvider.GetRevision()
	}

	allAffectedFeatures, allAffectedVariables := affectedDatafileEntities(previousInstanceEvaluationDataProvider, newInstanceEvaluationDataProvider)

	return logDetails{
		"revision":         newRevision,
		"previousRevision": previousRevision,
		"revisionChanged":  previousRevision != newRevision,
		"features":         allAffectedFeatures,
		"variables":        allAffectedVariables,
		"replaced":         replace,
	}
}

// getParamsForStickySetEvent gets parameters for sticky set event
func getParamsForStickySetEvent(previousStickyFeatures StickyFeatures, newStickyFeatures StickyFeatures, replace bool) logDetails {
	keysBefore := make([]string, 0, len(previousStickyFeatures))
	for key := range previousStickyFeatures {
		keysBefore = append(keysBefore, string(key))
	}

	keysAfter := make([]string, 0, len(newStickyFeatures))
	for key := range newStickyFeatures {
		keysAfter = append(keysAfter, string(key))
	}

	// Get unique features affected (combine both sets and remove duplicates)
	allKeys := append(keysBefore, keysAfter...)
	uniqueFeaturesAffected := make([]string, 0)
	seen := make(map[string]bool)

	for _, key := range allKeys {
		if !seen[key] {
			seen[key] = true
			uniqueFeaturesAffected = append(uniqueFeaturesAffected, key)
		}
	}

	return logDetails{
		"features": uniqueFeaturesAffected,
		"replaced": replace,
	}
}
