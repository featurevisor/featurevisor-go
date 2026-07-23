package featurevisor

// FeaturevisorDiagnostic is emitted by the SDK and modules for logs/errors.
type FeaturevisorDiagnostic struct {
	Level         LogLevel               `json:"level"`
	Code          string                 `json:"code"`
	Message       string                 `json:"message"`
	Module        string                 `json:"module,omitempty"`
	ModuleName    string                 `json:"moduleName,omitempty"`
	OriginalError interface{}            `json:"originalError,omitempty"`
	Details       map[string]interface{} `json:"details"`
}

// FeaturevisorModuleReportedDiagnostic is a diagnostic reported by a module.
type FeaturevisorModuleReportedDiagnostic = FeaturevisorDiagnostic

// FeaturevisorDiagnosticHandler handles diagnostics.
type FeaturevisorDiagnosticHandler func(diagnostic FeaturevisorDiagnostic)

type featurevisorDiagnosticReporter func(
	diagnostic FeaturevisorDiagnostic,
	sourceModule *FeaturevisorModule,
)

// FeaturevisorModuleDiagnosticOptions configures module diagnostic subscriptions.
type FeaturevisorModuleDiagnosticOptions struct {
	LogLevel LogLevel `json:"logLevel,omitempty"`
}

// FeaturevisorUnsubscribe unsubscribes from an SDK/module subscription.
type FeaturevisorUnsubscribe func()

func shouldLogDiagnostic(currentLevel LogLevel, targetLevel LogLevel) bool {
	currentIndex := -1
	targetIndex := -1
	for index, level := range allLevels {
		if level == currentLevel {
			currentIndex = index
		}
		if level == targetLevel {
			targetIndex = index
		}
	}
	return currentIndex != -1 && targetIndex != -1 && targetIndex <= currentIndex
}
