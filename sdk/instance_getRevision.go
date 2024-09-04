package sdk

// GetRevision returns the revision of the current datafile
func (f *FeaturevisorInstance) GetRevision() string {
	if f.datafileReader != nil {
		return f.datafileReader.GetRevision()
	}
	return ""
}
