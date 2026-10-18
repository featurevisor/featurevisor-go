module github.com/featurevisor/featurevisor-go/openfeature/v4

go 1.26.0

require (
	github.com/featurevisor/featurevisor-go/v4 v4.0.0
	github.com/open-feature/go-sdk v1.19.0
)

require go.uber.org/mock v0.6.0 // indirect

replace github.com/featurevisor/featurevisor-go/v4 => ..
