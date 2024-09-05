package sdk

// StopRefreshing stops the automatic refreshing of the datafile
func (f *FeaturevisorInstance) StopRefreshing() {
	if f.refreshTicker != nil {
		f.refreshTicker.Stop()
		f.refreshDone <- true
		close(f.refreshDone)
		f.refreshTicker = nil
		f.refreshDone = nil
	}
}
