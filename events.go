package featurevisor

import (
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
		oldHash, newHash := hash(oldValue), hash(newValue)
		if !ok || oldHash == "" || newHash == "" || oldHash != newHash {
			changed[key] = true
		}
	}
	for key, newValue := range after {
		oldValue, ok := before[key]
		oldHash, newHash := hash(oldValue), hash(newValue)
		if !ok || oldHash == "" || newHash == "" || oldHash != newHash {
			changed[key] = true
		}
	}
	return changed
}

func collectConditionDependencies(value interface{}, segments, features, variables map[string]bool) {
	value = (&entityNormalizer{}).parse(value)
	switch v := value.(type) {
	case string:
		if v != "*" {
			segments[v] = true
		}
	case []interface{}:
		for _, child := range v {
			collectConditionDependencies(child, segments, features, variables)
		}
	case map[string]interface{}:
		if variable, ok := v["variable"].(string); ok {
			if feature, ok := v["feature"].(string); ok {
				features[feature] = true
			} else {
				variables[variable] = true
			}
		} else if feature, ok := v["feature"].(string); ok {
			features[feature] = true
		} else {
			for _, op := range []string{"and", "or", "not"} {
				if child, ok := v[op]; ok {
					collectConditionDependencies(child, segments, features, variables)
				}
			}
		}
	}
}

func featureDependencies(feature Feature) (map[string]bool, map[string]bool, map[string]bool) {
	value, _ := normalizeEntity("feature", "", feature, false)
	feature = value.(Feature)
	segments, features, variables := map[string]bool{}, map[string]bool{}, map[string]bool{}
	visit := func(value interface{}) { collectConditionDependencies(value, segments, features, variables) }
	visit(feature.Conditions)
	for _, traffic := range feature.Traffic {
		visit(traffic.Conditions)
		for _, entries := range traffic.VariableOverrides {
			for _, entry := range entries {
				visit(entry.Conditions)
			}
		}
	}
	for _, force := range feature.Force {
		visit(force.Conditions)
	}
	for _, variation := range feature.Variations {
		for _, entries := range variation.VariableOverrides {
			for _, entry := range entries {
				visit(entry.Conditions)
			}
		}
	}
	return segments, features, variables
}

func variableDependencies(variable GlobalVariable) (map[string]bool, map[string]bool, map[string]bool) {
	value, _ := normalizeEntity("variable", "", variable, false)
	variable = value.(GlobalVariable)
	segments, features, variables := map[string]bool{}, map[string]bool{}, map[string]bool{}
	visit := func(value interface{}) { collectConditionDependencies(value, segments, features, variables) }
	visit(variable.Conditions)
	if variable.OverrideProgram != nil {
		for _, entry := range variable.OverrideProgram {
			visit(entry.Conditions)
		}
	} else {
		for _, entry := range variable.Overrides {
			visit(entry.Conditions)
		}
	}
	if alias, ok := variable.Compose.(string); ok {
		variables[alias] = true
	} else if entries, ok := expressionValue(variable.Compose).([]interface{}); ok {
		for _, entry := range entries {
			if ref, ok := entry.(map[string]interface{}); ok {
				if feature, ok := ref["feature"].(string); ok {
					features[feature] = true
				} else if variable, ok := ref["variable"].(string); ok {
					variables[variable] = true
				}
			}
		}
	}
	return segments, features, variables
}

// Event graphs are owned by the instance writer, never by evaluation snapshots.
type dependencyEntity struct {
	kind, key string
}

type datafileDependencyIndex struct {
	extractions uint64
	owners      map[dependencyEntity]map[dependencyEntity]bool
	reverse     map[dependencyEntity]map[dependencyEntity]bool
}

func newDatafileDependencyIndex() *datafileDependencyIndex {
	return &datafileDependencyIndex{
		owners:  map[dependencyEntity]map[dependencyEntity]bool{},
		reverse: map[dependencyEntity]map[dependencyEntity]bool{},
	}
}

func (index *datafileDependencyIndex) remove(owner dependencyEntity) {
	for dependency := range index.owners[owner] {
		delete(index.reverse[dependency], owner)
		if len(index.reverse[dependency]) == 0 {
			delete(index.reverse, dependency)
		}
	}
	delete(index.owners, owner)
}

func (index *datafileDependencyIndex) add(owner dependencyEntity, segments, features, variables map[string]bool) {
	index.extractions++
	edges := map[dependencyEntity]bool{}
	for kind, keys := range map[string]map[string]bool{"segment": segments, "feature": features, "variable": variables} {
		for key := range keys {
			dependency := dependencyEntity{kind, key}
			edges[dependency] = true
			if index.reverse[dependency] == nil {
				index.reverse[dependency] = map[dependencyEntity]bool{}
			}
			index.reverse[dependency][owner] = true
		}
	}
	index.owners[owner] = edges
}

func (index *datafileDependencyIndex) addDatafile(data DatafileContent) {
	for key, value := range data.Features {
		segments, features, variables := featureDependencies(value)
		index.add(dependencyEntity{"feature", key}, segments, features, variables)
	}
	for key, value := range data.Variables {
		segments, features, variables := variableDependencies(value)
		index.add(dependencyEntity{"variable", key}, segments, features, variables)
	}
	for key, value := range data.Segments {
		segments, features, variables := map[string]bool{}, map[string]bool{}, map[string]bool{}
		collectConditionDependencies(value.Conditions, segments, features, variables)
		index.add(dependencyEntity{"segment", key}, segments, features, variables)
	}
}

func providerDatafile(provider *instanceEvaluationDataProvider) DatafileContent {
	if provider == nil {
		return DatafileContent{}
	}
	return DatafileContent{Features: provider.features, Variables: provider.variables, Segments: provider.segments}
}

func (index *datafileDependencyIndex) propagate(features, variables, segments map[string]bool) {
	seen := map[dependencyEntity]bool{}
	queue := []dependencyEntity{}
	for kind, keys := range map[string]map[string]bool{"feature": features, "variable": variables, "segment": segments} {
		for key := range keys {
			entity := dependencyEntity{kind, key}
			seen[entity] = true
			queue = append(queue, entity)
		}
	}
	for cursor := 0; cursor < len(queue); cursor++ {
		for owner := range index.reverse[queue[cursor]] {
			if seen[owner] {
				continue
			}
			seen[owner] = true
			queue = append(queue, owner)
			switch owner.kind {
			case "feature":
				features[owner.key] = true
			case "variable":
				variables[owner.key] = true
			}
		}
	}
}

func affectedDatafileEntities(previous, next *instanceEvaluationDataProvider) ([]string, []string) {
	var index *datafileDependencyIndex
	return updateAffectedDatafileEntities(previous, next, providerDatafile(next), true, &index)
}

func updateAffectedDatafileEntities(previous, next *instanceEvaluationDataProvider, incoming DatafileContent, replace bool, cached **datafileDependencyIndex) ([]string, []string) {
	before, after := providerDatafile(previous), providerDatafile(next)
	features := changedEntityKeys(before.Features, after.Features, func(value Feature) string { return stringValue(value.Hash) })
	variables := changedEntityKeys(before.Variables, after.Variables, func(value GlobalVariable) string { return stringValue(value.Hash) })
	segments := map[string]bool{}
	segmentCandidates := incoming.Segments
	if replace {
		segmentCandidates = before.Segments
	}
	for key := range segmentCandidates {
		old, oldOK := before.Segments[key]
		value, ok := after.Segments[key]
		if oldOK != ok || !reflect.DeepEqual(old, value) {
			segments[key] = true
		}
	}
	if replace {
		for key, value := range after.Segments {
			old, ok := before.Segments[key]
			if !ok || !reflect.DeepEqual(old, value) {
				segments[key] = true
			}
		}
	}
	if len(before.Features)+len(before.Variables)+len(before.Segments) > 0 {
		if *cached == nil {
			*cached = newDatafileDependencyIndex()
			(*cached).addDatafile(before)
		}
		index := *cached
		// Segment reachability is local to each traversal, as in JavaScript.
		index.propagate(features, variables, segments)
		if replace {
			for owner := range index.owners {
				index.remove(owner)
			}
		} else {
			for key := range incoming.Features {
				index.remove(dependencyEntity{"feature", key})
			}
			for key := range incoming.Variables {
				index.remove(dependencyEntity{"variable", key})
			}
			for key := range incoming.Segments {
				index.remove(dependencyEntity{"segment", key})
			}
		}
		index.addDatafile(incoming)
		index.propagate(features, variables, segments)
	} else {
		// Construction announces every incoming definition without indexing.
		*cached = nil
	}
	featureKeys, variableKeys := []string{}, []string{}
	for key := range features {
		featureKeys = append(featureKeys, key)
	}
	for key := range variables {
		variableKeys = append(variableKeys, key)
	}
	return featureKeys, variableKeys
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

// getParamsForStickyFeaturesSetEvent gets parameters for a sticky features set event.
func getParamsForStickyFeaturesSetEvent(previousStickyFeatures StickyFeatures, newStickyFeatures StickyFeatures, replace bool) logDetails {
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
