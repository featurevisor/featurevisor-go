package sdk

import (
	"time"
)

func (i *FeaturevisorInstance) Refresh() {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.logger.Debug("refreshing datafile")

	if i.statuses.RefreshInProgress {
		i.logger.Warn("refresh in progress, skipping")
		return
	}

	if i.datafileURL == "" {
		i.logger.Error("cannot refresh since `datafileURL` is not provided")
		return
	}

	i.statuses.RefreshInProgress = true

	go func() {
		content, err := i.handleDatafileFetch(i.datafileURL)
		if err != nil {
			i.logger.Error("failed to refresh datafile", LogDetails{"error": err})
			i.mu.Lock()
			i.statuses.RefreshInProgress = false
			i.mu.Unlock()
			return
		}

		i.mu.Lock()
		currentRevision := i.datafileReader.GetRevision()
		err = i.setDatafile(content)
		if err != nil {
			i.logger.Error("failed to set datafile", LogDetails{"error": err})
			i.statuses.RefreshInProgress = false
			i.mu.Unlock()
			return
		}

		newRevision := i.datafileReader.GetRevision()
		isNotSameRevision := currentRevision != newRevision

		i.logger.Info("refreshed datafile")
		i.emitter.Emit(EventRefresh)

		if isNotSameRevision {
			i.emitter.Emit(EventUpdate)
		}

		i.statuses.RefreshInProgress = false
		i.mu.Unlock()
	}()
}

func (i *FeaturevisorInstance) StartRefreshing() {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.datafileURL == "" {
		i.logger.Error("cannot start refreshing since `datafileURL` is not provided")
		return
	}

	if i.refreshTicker != nil {
		i.logger.Warn("refreshing has already started")
		return
	}

	if i.refreshInterval == 0 {
		i.logger.Warn("no `refreshInterval` option provided")
		return
	}

	i.refreshTicker = time.NewTicker(i.refreshInterval)

	go func() {
		for range i.refreshTicker.C {
			i.Refresh()
		}
	}()
}

func (i *FeaturevisorInstance) StopRefreshing() {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.refreshTicker == nil {
		i.logger.Warn("refreshing has not started yet")
		return
	}

	i.refreshTicker.Stop()
	i.refreshTicker = nil
}
