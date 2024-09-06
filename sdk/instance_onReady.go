package sdk

import (
	"time"
)

// OnReady returns a channel that will be closed when the instance is ready
func (f *FeaturevisorInstance) OnReady() <-chan struct{} {
	readyChan := make(chan struct{})

	if f.statuses.Ready {
		close(readyChan)
		return readyChan
	}

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if f.statuses.Ready {
					close(readyChan)
					return
				}
			}
		}
	}()

	return readyChan
}
