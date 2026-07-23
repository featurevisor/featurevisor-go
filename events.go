package featurevisor

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

	previousFeatureKeys := []string{}
	if previousInstanceEvaluationDataProvider != nil {
		previousFeatureKeys = previousInstanceEvaluationDataProvider.GetFeatureKeys()
	}

	newFeatureKeys := []string{}
	if newInstanceEvaluationDataProvider != nil {
		newFeatureKeys = newInstanceEvaluationDataProvider.GetFeatureKeys()
	}

	// Find removed features
	removedFeatures := []string{}
	for _, previousFeatureKey := range previousFeatureKeys {
		found := false
		for _, newFeatureKey := range newFeatureKeys {
			if previousFeatureKey == newFeatureKey {
				found = true
				break
			}
		}
		if !found {
			removedFeatures = append(removedFeatures, previousFeatureKey)
		}
	}

	// Find changed features
	changedFeatures := []string{}
	for _, previousFeatureKey := range previousFeatureKeys {
		for _, newFeatureKey := range newFeatureKeys {
			if previousFeatureKey == newFeatureKey {
				// Check if feature was changed by comparing hashes
				previousFeature := previousInstanceEvaluationDataProvider.GetFeature(FeatureKey(previousFeatureKey))
				newFeature := newInstanceEvaluationDataProvider.GetFeature(FeatureKey(newFeatureKey))

				if previousFeature != nil && newFeature != nil {
					// Compare hashes if available, otherwise assume changed
					if previousFeature.Hash != newFeature.Hash {
						changedFeatures = append(changedFeatures, previousFeatureKey)
					}
				}
				break
			}
		}
	}

	// Find added features
	addedFeatures := []string{}
	for _, newFeatureKey := range newFeatureKeys {
		found := false
		for _, previousFeatureKey := range previousFeatureKeys {
			if newFeatureKey == previousFeatureKey {
				found = true
				break
			}
		}
		if !found {
			addedFeatures = append(addedFeatures, newFeatureKey)
		}
	}

	// Combine all affected feature keys
	allAffectedFeatures := append(append(removedFeatures, changedFeatures...), addedFeatures...)

	return logDetails{
		"revision":         newRevision,
		"previousRevision": previousRevision,
		"revisionChanged":  previousRevision != newRevision,
		"features":         allAffectedFeatures,
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
