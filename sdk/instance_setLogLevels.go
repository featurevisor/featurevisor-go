package sdk

// SetLogLevels sets the log levels for the instance's logger
func (f *FeaturevisorInstance) SetLogLevels(levels []LogLevel) {
	if f.logger != nil {
		f.logger.SetLevels(levels)
	}
}
