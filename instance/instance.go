package instance

import "github.com/featurevisor/featurevisor-go/config"

type Instance struct {
	ConfigManager config.ConfigManager

	logger interface{}
}

// GetRevision returns the revision of the datafile
func (instance *Instance) GetRevision() string {
	return ""
}

// GetBucketKey returns the bucket key for the given feature name
func (instance *Instance) GetBucketKey(featureName string) string {
	return ""
}

// Enabled returns true if the feature is enabled
func (instance *Instance) Enabled(featureName string) bool {
	return false
}
