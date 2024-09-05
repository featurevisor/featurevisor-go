package sdk

import "time"

// StartRefreshing starts automatic refreshing of the datafile
func (f *FeaturevisorInstance) StartRefreshing() {
	if f.refreshInterval <= 0 {
		return
	}

	f.refreshTicker = time.NewTicker(time.Duration(f.refreshInterval) * time.Second)
	f.refreshDone = make(chan bool)

	go func() {
		for {
			select {
			case <-f.refreshTicker.C:
				f.Refresh()
			case <-f.refreshDone:
				return
			}
		}
	}()
}
