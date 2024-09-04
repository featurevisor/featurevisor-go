package sdk

// IsReady returns whether the instance is ready
func (f *FeaturevisorInstance) IsReady() bool {
	return f.statuses.Ready
}
