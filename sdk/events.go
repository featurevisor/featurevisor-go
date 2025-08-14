package sdk

// getParamsForDatafileSetEvent gets parameters for datafile set event
func getParamsForDatafileSetEvent(previousDatafileReader *DatafileReader, newDatafileReader *DatafileReader) LogDetails {
	previousRevision := ""
	if previousDatafileReader != nil {
		previousRevision = previousDatafileReader.GetRevision()
	}

	newRevision := ""
	if newDatafileReader != nil {
		newRevision = newDatafileReader.GetRevision()
	}

	previousFeatureKeys := []string{}
	if previousDatafileReader != nil {
		previousFeatureKeys = previousDatafileReader.GetFeatureKeys()
	}

	newFeatureKeys := []string{}
	if newDatafileReader != nil {
		newFeatureKeys = newDatafileReader.GetFeatureKeys()
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
				previousFeature := previousDatafileReader.GetFeature(FeatureKey(previousFeatureKey))
				newFeature := newDatafileReader.GetFeature(FeatureKey(newFeatureKey))

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

	return LogDetails{
		"revision":         newRevision,
		"previousRevision": previousRevision,
		"revisionChanged":  previousRevision != newRevision,
		"features":         allAffectedFeatures,
		"removedFeatures":  removedFeatures,
		"changedFeatures":  changedFeatures,
		"addedFeatures":    addedFeatures,
	}
}

// getParamsForStickySetEvent gets parameters for sticky set event
func getParamsForStickySetEvent(previousStickyFeatures StickyFeatures, newStickyFeatures StickyFeatures, replace bool) LogDetails {
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

	return LogDetails{
		"features": uniqueFeaturesAffected,
		"replaced": replace,
	}
}
