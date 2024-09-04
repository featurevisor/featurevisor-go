package sdk

// Refresh triggers a manual refresh of the datafile
func (f *FeaturevisorInstance) Refresh() {
	f.logger.Debug("refreshing datafile", nil)

	if f.statuses.RefreshInProgress {
		f.logger.Warn("refresh in progress, skipping", nil)
		return
	}

	if f.datafileURL == "" {
		f.logger.Error("cannot refresh since `datafileUrl` is not provided", nil)
		return
	}

	f.statuses.RefreshInProgress = true

	datafile, err := f.fetchDatafileContent(f.datafileURL, f.handleDatafileFetch)
	if err != nil {
		f.logger.Error("failed to refresh datafile", LogDetails{"error": err})
		f.statuses.RefreshInProgress = false
		return
	}

	currentRevision := f.datafileReader.GetRevision()
	newRevision := datafile.Revision
	isNotSameRevision := currentRevision != newRevision

	if err := f.setDatafile(datafile); err != nil {
		f.logger.Error("failed to set refreshed datafile", LogDetails{"error": err})
		f.statuses.RefreshInProgress = false
		return
	}

	f.logger.Info("refreshed datafile", nil)

	f.emitter.Emit(EventRefresh)

	if isNotSameRevision {
		f.emitter.Emit(EventUpdate)
	}

	f.statuses.RefreshInProgress = false
}
