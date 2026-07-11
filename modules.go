package featurevisor

// ConfigureBucketKeyOptions contains options for configuring bucket key
type ConfigureBucketKeyOptions struct {
	FeatureKey FeatureKey `json:"featureKey"`
	Context    Context    `json:"context"`
	BucketBy   BucketBy   `json:"bucketBy"`
	BucketKey  string     `json:"bucketKey"` // the initial bucket key, which can be modified by modules
}

// ConfigureBucketKey is a function type for configuring bucket key
type ConfigureBucketKey func(options ConfigureBucketKeyOptions) BucketKey

// ConfigureBucketValueOptions contains options for configuring bucket value
type ConfigureBucketValueOptions struct {
	FeatureKey  FeatureKey `json:"featureKey"`
	BucketKey   string     `json:"bucketKey"`
	Context     Context    `json:"context"`
	BucketValue int        `json:"bucketValue"` // the initial bucket value, which can be modified by modules
}

// ConfigureBucketValue is a function type for configuring bucket value
type ConfigureBucketValue func(options ConfigureBucketValueOptions) BucketValue

// FeaturevisorModuleApi is passed to modules during setup.
type FeaturevisorModuleApi struct {
	GetRevision      func() string
	OnDiagnostic     func(handler FeaturevisorDiagnosticHandler, options ...FeaturevisorModuleDiagnosticOptions) FeaturevisorUnsubscribe
	ReportDiagnostic func(diagnostic FeaturevisorModuleReportedDiagnostic)
}

// FeaturevisorModule represents a module that can participate in evaluation lifecycle.
type FeaturevisorModule struct {
	Name string `json:"name,omitempty"`

	Setup       func(api FeaturevisorModuleApi)                                 `json:"setup,omitempty"`
	Before      func(options EvaluateOptions) EvaluateOptions                   `json:"before,omitempty"`
	BucketKey   ConfigureBucketKey                                              `json:"bucketKey,omitempty"`
	BucketValue ConfigureBucketValue                                            `json:"bucketValue,omitempty"`
	After       func(evaluation Evaluation, options EvaluateOptions) Evaluation `json:"after,omitempty"`
	Close       func()                                                          `json:"close,omitempty"`
}

func getModuleName(module *FeaturevisorModule) string {
	if module == nil {
		return ""
	}

	return module.Name
}

// ModulesManagerOptions contains options for creating a modules manager.
type ModulesManagerOptions struct {
	Modules                            []*FeaturevisorModule
	ReportDiagnostic                   FeaturevisorDiagnosticReporter
	GetModuleApi                       func(module *FeaturevisorModule) FeaturevisorModuleApi
	ClearModuleDiagnosticSubscriptions func(module *FeaturevisorModule)
}

// ModulesManager manages Featurevisor modules.
type ModulesManager struct {
	modules                            []*FeaturevisorModule
	reportDiagnostic                   FeaturevisorDiagnosticReporter
	getModuleApi                       func(module *FeaturevisorModule) FeaturevisorModuleApi
	clearModuleDiagnosticSubscriptions func(module *FeaturevisorModule)
}

// NewModulesManager creates a new modules manager instance.
func NewModulesManager(options ModulesManagerOptions) *ModulesManager {
	mm := &ModulesManager{
		modules:                            make([]*FeaturevisorModule, 0),
		reportDiagnostic:                   options.ReportDiagnostic,
		getModuleApi:                       options.GetModuleApi,
		clearModuleDiagnosticSubscriptions: options.ClearModuleDiagnosticSubscriptions,
	}

	if options.Modules != nil {
		for _, module := range options.Modules {
			mm.Add(module)
		}
	}

	return mm
}

// Add adds a module to the modules manager.
func (mm *ModulesManager) Add(module *FeaturevisorModule) FeaturevisorUnsubscribe {
	if module == nil {
		return nil
	}

	if module.Name != "" {
		for _, existingModule := range mm.modules {
			if existingModule.Name == module.Name {
				mm.reportDiagnostic(FeaturevisorDiagnostic{
					Level:      LogLevelError,
					Code:       "duplicate_module",
					Message:    "Duplicate module name",
					ModuleName: module.Name,
				}, nil)
				return nil
			}
		}
	}

	if module.Setup != nil && mm.getModuleApi != nil {
		setupFailed := false
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					setupFailed = true
					if mm.clearModuleDiagnosticSubscriptions != nil {
						mm.clearModuleDiagnosticSubscriptions(module)
					}
					mm.reportDiagnostic(FeaturevisorDiagnostic{
						Level:         LogLevelError,
						Code:          "module_setup_error",
						Message:       "Module setup failed",
						ModuleName:    module.Name,
						OriginalError: recovered,
					}, nil)
					mm.closeModule(module)
				}
			}()
			module.Setup(mm.getModuleApi(module))
		}()
		if setupFailed {
			return nil
		}
	}

	mm.modules = append(mm.modules, module)

	return func() {
		moduleExists := false
		for _, existingModule := range mm.modules {
			if existingModule == module {
				moduleExists = true
				break
			}
		}

		mm.modules = filterModules(mm.modules, func(existingModule *FeaturevisorModule) bool {
			return existingModule != module
		})
		if mm.clearModuleDiagnosticSubscriptions != nil {
			mm.clearModuleDiagnosticSubscriptions(module)
		}
		if moduleExists {
			mm.closeModule(module)
		}
	}
}

func (mm *ModulesManager) closeModule(module *FeaturevisorModule) {
	if module == nil || module.Close == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil && mm.reportDiagnostic != nil {
			mm.reportDiagnostic(FeaturevisorDiagnostic{
				Level:         LogLevelError,
				Code:          "module_close_error",
				Message:       "Module close failed",
				ModuleName:    getModuleName(module),
				OriginalError: r,
			}, nil)
		}
	}()

	module.Close()
}

// Remove removes modules by name.
func (mm *ModulesManager) Remove(name string) {
	removedModules := []*FeaturevisorModule{}
	keptModules := []*FeaturevisorModule{}

	for _, module := range mm.modules {
		if module.Name == name {
			removedModules = append(removedModules, module)
		} else {
			keptModules = append(keptModules, module)
		}
	}

	mm.modules = keptModules

	if mm.clearModuleDiagnosticSubscriptions != nil {
		for _, module := range removedModules {
			mm.clearModuleDiagnosticSubscriptions(module)
		}
	}

	for _, module := range removedModules {
		mm.closeModule(module)
	}
}

// GetAll returns all modules.
func (mm *ModulesManager) GetAll() []*FeaturevisorModule {
	return mm.modules
}

// CloseAll closes and removes all modules.
func (mm *ModulesManager) CloseAll() {
	modules := append([]*FeaturevisorModule{}, mm.modules...)
	mm.modules = []*FeaturevisorModule{}

	for _, module := range modules {
		if mm.clearModuleDiagnosticSubscriptions != nil {
			mm.clearModuleDiagnosticSubscriptions(module)
		}
		mm.closeModule(module)
	}
}

func filterModules(
	modules []*FeaturevisorModule,
	keep func(module *FeaturevisorModule) bool,
) []*FeaturevisorModule {
	result := []*FeaturevisorModule{}
	for _, module := range modules {
		if keep(module) {
			result = append(result, module)
		}
	}
	return result
}
