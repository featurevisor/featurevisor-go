package featurevisor_test

import (
	"testing"

	featurevisor "github.com/featurevisor/featurevisor-go/v3"
)

func TestPublicAPIContractsCompileForExternalConsumers(t *testing.T) {
	module := featurevisor.FeaturevisorModule{
		Name: "public-api",
		Before: func(options featurevisor.EvaluateOptions) featurevisor.EvaluateOptions {
			options.Context["fromModule"] = true
			return options
		},
		BucketKey: func(options featurevisor.ConfigureBucketKeyOptions) featurevisor.BucketKey {
			return featurevisor.BucketKey(options.BucketKey)
		},
	}
	level := featurevisor.LogLevelFatal
	instance := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
		LogLevel: &level,
		Modules:  []*featurevisor.FeaturevisorModule{&module},
	})
	defer instance.Close()

	unsubscribe := instance.On(featurevisor.EventNameError, func(details featurevisor.EventDetails) {})
	unsubscribe()
}
