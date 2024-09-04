package sdk

// StopRefreshing stops the automatic refreshing of the datafile
func (f *FeaturevisorInstance) StopRefreshing() {
	// Since we're using a ticker in a goroutine for refreshing,
	// we need to add a way to stop it. We'll need to modify our
	// FeaturevisorInstance struct to include a ticker and a done channel.

	// This is a placeholder implementation. To make it work correctly,
	// you'll need to modify the FeaturevisorInstance struct and the
	// StartRefreshing method. Here's what you should do:

	// 1. Add these fields to FeaturevisorInstance:
	//    refreshTicker *time.Ticker
	//    refreshDone   chan bool

	// 2. In StartRefreshing, initialize these:
	//    f.refreshTicker = time.NewTicker(time.Duration(f.refreshInterval) * time.Second)
	//    f.refreshDone = make(chan bool)

	// 3. Modify the goroutine in StartRefreshing:
	//    go func() {
	//        for {
	//            select {
	//            case <-f.refreshTicker.C:
	//                f.Refresh()
	//            case <-f.refreshDone:
	//                return
	//            }
	//        }
	//    }()

	// 4. Then, implement StopRefreshing like this:
	if f.refreshTicker != nil {
		f.refreshTicker.Stop()
		f.refreshDone <- true
		close(f.refreshDone)
		f.refreshTicker = nil
		f.refreshDone = nil
	}
}
