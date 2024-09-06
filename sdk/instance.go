package sdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/featurevisor/featurevisor-go/types"
)

type ReadyCallback func()
type ActivationCallback func(featureName string, variation types.VariationValue, context types.Context, captureContext types.Context)
type ConfigureBucketKey func(feature types.Feature, context types.Context, bucketKey types.BucketKey) types.BucketKey
type ConfigureBucketValue func(feature types.Feature, context types.Context, bucketValue types.BucketValue) types.BucketValue
type InterceptContext func(context types.Context) types.Context

type Statuses struct {
	Ready             bool
	RefreshInProgress bool
}

type InstanceOptions struct {
	BucketKeySeparator   string
	ConfigureBucketKey   ConfigureBucketKey
	ConfigureBucketValue ConfigureBucketValue
	Datafile             *types.DatafileContent
	DatafileURL          string
	HandleDatafileFetch  func(datafileURL string) (types.DatafileContent, error)
	InitialFeatures      types.InitialFeatures
	InterceptContext     InterceptContext
	Logger               Logger
	OnActivation         ActivationCallback
	OnReady              ReadyCallback
	OnRefresh            func()
	OnUpdate             func()
	RefreshInterval      int // seconds
	StickyFeatures       types.StickyFeatures
}

type FeaturevisorInstance struct {
	bucketKeySeparator   string
	configureBucketKey   ConfigureBucketKey
	configureBucketValue ConfigureBucketValue
	datafileURL          string
	handleDatafileFetch  func(datafileURL string) (types.DatafileContent, error)
	initialFeatures      types.InitialFeatures
	interceptContext     InterceptContext
	logger               Logger
	refreshInterval      int
	stickyFeatures       types.StickyFeatures

	datafileReader *DatafileReader
	emitter        *Emitter
	statuses       Statuses
	refreshTicker  *time.Ticker
	refreshDone    chan bool
}

func NewDatafileContent(jsonString string) (*types.DatafileContent, error) {
	var datafileContent types.DatafileContent
	err := json.Unmarshal([]byte(jsonString), &datafileContent)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal datafile content: %v", err)
	}
	return &datafileContent, nil
}

func CreateInstance(options InstanceOptions) (*FeaturevisorInstance, error) {
	instance := &FeaturevisorInstance{
		bucketKeySeparator:   options.BucketKeySeparator,
		configureBucketKey:   options.ConfigureBucketKey,
		configureBucketValue: options.ConfigureBucketValue,
		datafileURL:          options.DatafileURL,
		handleDatafileFetch:  options.HandleDatafileFetch,
		initialFeatures:      options.InitialFeatures,
		interceptContext:     options.InterceptContext,
		logger:               options.Logger,
		refreshInterval:      options.RefreshInterval,
		stickyFeatures:       options.StickyFeatures,

		emitter:  NewEmitter(),
		statuses: Statuses{Ready: false, RefreshInProgress: false},
	}

	if options.OnReady != nil {
		instance.emitter.AddListener(EventReady, func(...interface{}) { options.OnReady() })
	}

	if options.OnRefresh != nil {
		instance.emitter.AddListener(EventRefresh, func(...interface{}) { options.OnRefresh() })
	}

	if options.OnUpdate != nil {
		instance.emitter.AddListener(EventUpdate, func(...interface{}) { options.OnUpdate() })
	}

	if options.OnActivation != nil {
		instance.emitter.AddListener(EventActivation, func(args ...interface{}) {
			if len(args) == 4 {
				options.OnActivation(
					args[0].(string),
					args[1].(types.VariationValue),
					args[2].(types.Context),
					args[3].(types.Context),
				)
			}
		})
	}

	if options.DatafileURL != "" {
		if err := instance.setDatafile(options.Datafile); err != nil {
			return nil, err
		}

		go func() {
			datafile, err := instance.fetchDatafileContent(options.DatafileURL, options.HandleDatafileFetch)
			if err != nil {
				instance.logger.Error("failed to fetch datafile", LogDetails{"error": err})
				return
			}

			if err := instance.setDatafile(datafile); err != nil {
				instance.logger.Error("failed to set datafile", LogDetails{"error": err})
				return
			}

			instance.statuses.Ready = true
			instance.emitter.Emit(EventReady)

			if instance.refreshInterval > 0 {
				instance.startRefreshing()
			}
		}()
	} else if options.Datafile != nil {
		if err := instance.setDatafile(options.Datafile); err != nil {
			return nil, err
		}
		instance.statuses.Ready = true
		go instance.emitter.Emit(EventReady)
	} else {
		return nil, errors.New("Featurevisor SDK instance cannot be created without both `datafile` and `datafileUrl` options")
	}

	return instance, nil
}

// Private helper functions

func (f *FeaturevisorInstance) startRefreshing() {
	ticker := time.NewTicker(time.Duration(f.refreshInterval) * time.Second)
	go func() {
		for range ticker.C {
			f.Refresh()
		}
	}()
}
