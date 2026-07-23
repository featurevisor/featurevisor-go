module github.com/featurevisor/featurevisor-go/openfeature/v2

go 1.25.0

require (
	github.com/featurevisor/featurevisor-go/v2 v2.0.0
	github.com/open-feature/go-sdk v1.17.2
)

require (
	github.com/go-logr/logr v1.4.3 // indirect
	go.uber.org/mock v0.6.0 // indirect
)

replace github.com/featurevisor/featurevisor-go/v2 => ..
