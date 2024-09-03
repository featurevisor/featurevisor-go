package sdk

import (
	"sync"
	"time"

	"github.com/featurevisor/featurevisor-go/types"
)

type ConfigureBucketKey func(feature *types.Feature, context types.Context, bucketKey string) string
type ConfigureBucketValue func(feature *types.Feature, context types.Context, bucketValue int) int

type Statuses struct {
	Ready             bool
	RefreshInProgress bool
}

type FeaturevisorInstance struct {
	bucketKeySeparator  string
	configureBucketKey  ConfigureBucketKey
	configureBucketValue ConfigureBucketValue
	datafileURL         string
	handleDatafileFetch func(string) (types.DatafileContent, error)
	initialFeatures     types.InitialFeatures
	interceptContext    func(types.Context) types.Context
	logger              Logger
	refreshInterval     time.Duration
	stickyFeatures      types.StickyFeatures

	datafileReader *DatafileReader
	emitter        *Emitter
	statuses       Statuses
	refreshTicker  *time.Ticker
	mu             sync.RWMutex
}

type InstanceOptions struct {
	BucketKeySeparator  string
	ConfigureBucketKey  ConfigureBucketKey
	ConfigureBucketValue ConfigureBucketValue
	Datafile            interface{}
	DatafileURL         string
	HandleDatafileFetch func(string) (types.DatafileContent, error)
	InitialFeatures     types.InitialFeatures
	InterceptContext    func(types.Context) types.Context
	Logger              Logger
	OnActivation        func(string, types.VariationValue, types.Context, types.Context)
	OnReady             func()
	OnRefresh           func()
	OnUpdate            func()
	RefreshInterval     time.Duration
	StickyFeatures      types.StickyFeatures
}

func NewInstance(options InstanceOptions) (*FeaturevisorInstance, error) {
	instance := &FeaturevisorInstance{
		bucketKeySeparator:  options.BucketKeySeparator,
		configureBucketKey:  options.ConfigureBucketKey,
		configureBucketValue: options.ConfigureBucketValue,
		datafileURL:         options.DatafileURL,
		handleDatafileFetch: options.HandleDatafileFetch,
		initialFeatures:     options.InitialFeatures,
		interceptContext:    options.InterceptContext,
		logger:              options.Logger,
		refreshInterval:     options.RefreshInterval,
		stickyFeatures:      options.StickyFeatures,
		emitter:             NewEmitter(),
	}

	if options.OnReady != nil {
		instance.emitter.AddListener(EventReady, func(...interface{}) {
			options.OnReady()
		})
	}

	if options.OnRefresh != nil {
		instance.emitter.AddListener(EventRefresh, func(...interface{}) {
			options.OnRefresh()
		})
	}

	if options.OnUpdate != nil {
		instance.emitter.AddListener(EventUpdate, func(...interface{}) {
			options.OnUpdate()
		})
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

	// Initialize datafile
	if options.DatafileURL != "" {
		if err := instance.setDatafile(options.Datafile); err != nil {
			return nil, err
		}

		go instance.fetchAndSetDatafile()
	} else if options.Datafile != nil {
		if err := instance.setDatafile(options.Datafile); err != nil {
			return nil, err
		}
		instance.statuses.Ready = true
		instance.emitter.Emit(EventReady)
	} else {
		return nil, ErrNoDatafile
	}

	return instance, nil
}

func (i *FeaturevisorInstance) setDatafile(datafile interface{}) error {
	var content types.DatafileContent
	var err error

	switch d := datafile.(type) {
	case string:
		err = json.Unmarshal([]byte(d), &content)
	case types.DatafileContent:
		content = d
	default:
		return ErrInvalidDatafileType
	}

	if err != nil {
		i.logger.Error("could not parse datafile", LogDetails{"error": err})
		return err
	}

	i.datafileReader = NewDatafileReader(content)
	return nil
}

func (i *FeaturevisorInstance) fetchAndSetDatafile() {
	content, err := i.handleDatafileFetch(i.datafileURL)
	if err != nil {
		i.logger.Error("failed to fetch datafile", LogDetails{"error": err})
		return
	}

	if err := i.setDatafile(content); err != nil {
		return
	}

	i.statuses.Ready = true
	i.emitter.Emit(EventReady)

	if i.refreshInterval > 0 {
		i.startRefreshing()
	}
}

func (i *FeaturevisorInstance) SetLogLevels(levels []LogLevel) {
	i.logger.SetLevels(levels)
}

func (i *FeaturevisorInstance) OnReady() <-chan struct{} {
	readyChan := make(chan struct{})

	if i.statuses.Ready {
		close(readyChan)
		return readyChan
	}

	i.emitter.AddListener(EventReady, func(...interface{}) {
		close(readyChan)
	})

	return readyChan
}

func (i *FeaturevisorInstance) SetStickyFeatures(stickyFeatures types.StickyFeatures) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.stickyFeatures = stickyFeatures
}

func (i *FeaturevisorInstance) Activate(featureKey types.FeatureKey, context types.Context) types.VariationValue {
	i.mu.RLock()
	defer i.mu.RUnlock()

	evaluation := i.EvaluateVariation(featureKey, context)
	variationValue := evaluation.VariationValue
	if evaluation.Variation != nil {
		variationValue = evaluation.Variation.Value
	}

	if variationValue == "" {
		return ""
	}

	finalContext := context
	if i.interceptContext != nil {
		finalContext = i.interceptContext(context)
	}

	captureContext := types.Context{}
	attributes := i.datafileReader.GetAllAttributes()
	for _, attr := range attributes {
		if attr.Capture != nil && *attr.Capture {
			if value, ok := finalContext[attr.Key]; ok {
				captureContext[attr.Key] = value
			}
		}
	}

	i.emitter.Emit(EventActivation, featureKey, variationValue, finalContext, captureContext, evaluation)

	return variationValue
}
