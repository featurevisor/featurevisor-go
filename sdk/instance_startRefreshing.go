package sdk

import "time"

// StartRefreshing starts the automatic refreshing of the datafile
func (f *FeaturevisorInstance) StartRefreshing() {
	if f.datafileURL == "" {
		f.logger.Error("cannot start refreshing since `datafileUrl` is not provided", nil)
		return
	}

	if f.refreshInterval <= 0 {
		f.logger.Warn("no `refreshInterval` option provided", nil)
		return
	}

	ticker := time.NewTicker(time.Duration(f.refreshInterval) * time.Second)
	go func() {
		for range ticker.C {
			f.Refresh()
		}
	}()
}
