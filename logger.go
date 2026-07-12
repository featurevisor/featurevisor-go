package featurevisor

import (
	"fmt"
	"log"
)

// LogLevel represents the different logging levels
type LogLevel string

const (
	LogLevelFatal LogLevel = "fatal"
	LogLevelError LogLevel = "error"
	LogLevelWarn  LogLevel = "warn"
	LogLevelInfo  LogLevel = "info"
	LogLevelDebug LogLevel = "debug"
)

// logMessage represents a log message string
type logMessage string

// logDetails represents additional details for logging
type logDetails map[string]interface{}

// logHandler is a function type for handling log messages
type logHandler func(level LogLevel, message logMessage, details logDetails)

// loggerOptions contains options for creating a logger
type loggerOptions struct {
	Level   *LogLevel
	Handler *logHandler
}

// loggerPrefix is the prefix used for all log messages
const loggerPrefix = "[Featurevisor]"

// defaultLogHandler is the default logging handler
func defaultLogHandler(level LogLevel, message logMessage, details logDetails) {
	var method string

	switch level {
	case LogLevelInfo:
		method = "INFO"
	case LogLevelWarn:
		method = "WARN"
	case LogLevelError:
		method = "ERROR"
	case LogLevelFatal:
		method = "FATAL"
	default:
		method = "LOG"
	}

	// Format the log message
	logMessage := fmt.Sprintf("%s %s: %s", loggerPrefix, method, message)

	// Add details if provided
	if len(details) > 0 {
		logMessage += fmt.Sprintf(" %+v", details)
	}

	// Use appropriate log level
	switch level {
	case LogLevelFatal:
		log.Printf("[FATAL] %s", logMessage)
	case LogLevelError:
		log.Printf("[ERROR] %s", logMessage)
	case LogLevelWarn:
		log.Printf("[WARN] %s", logMessage)
	case LogLevelInfo:
		log.Printf("[INFO] %s", logMessage)
	case LogLevelDebug:
		log.Printf("[DEBUG] %s", logMessage)
	default:
		log.Print(logMessage)
	}
}

// featurevisorLogger provides logging functionality
type featurevisorLogger struct {
	level  LogLevel
	handle logHandler
}

// allLevels contains all available log levels in order of severity
var allLevels = []LogLevel{
	LogLevelFatal,
	LogLevelError,
	LogLevelWarn,
	LogLevelInfo,
	LogLevelDebug, // not enabled by default
}

// defaultLevel is the default logging level
var defaultLevel = LogLevelInfo

// newLogger creates a new logger instance
func newLogger(options loggerOptions) *featurevisorLogger {
	level := defaultLevel
	if options.Level != nil {
		level = *options.Level
	}

	handler := defaultLogHandler
	if options.Handler != nil {
		handler = *options.Handler
	}

	return &featurevisorLogger{
		level:  level,
		handle: handler,
	}
}

// SetLevel sets the logging level
func (l *featurevisorLogger) SetLevel(level LogLevel) {
	l.level = level
}

// GetLevel returns the current logging level
func (l *featurevisorLogger) GetLevel() LogLevel {
	return l.level
}

// shouldHandle checks if a log level should be handled based on current level
func (l *featurevisorLogger) shouldHandle(level LogLevel) bool {
	currentIndex := -1
	targetIndex := -1

	// Find indices of current and target levels
	for i, logLevel := range allLevels {
		if logLevel == l.level {
			currentIndex = i
		}
		if logLevel == level {
			targetIndex = i
		}
	}

	// If either level is not found, default to not handling
	if currentIndex == -1 || targetIndex == -1 {
		return false
	}

	// Handle if target level is at or above current level
	return targetIndex <= currentIndex
}

// Log logs a message at the specified level
func (l *featurevisorLogger) Log(level LogLevel, message logMessage, details logDetails) {
	if !l.shouldHandle(level) {
		return
	}

	if details == nil {
		details = make(logDetails)
	}

	l.handle(level, message, details)
}

// Debug logs a debug message
func (l *featurevisorLogger) Debug(message logMessage, details logDetails) {
	l.Log(LogLevelDebug, message, details)
}

// Info logs an info message
func (l *featurevisorLogger) Info(message logMessage, details logDetails) {
	l.Log(LogLevelInfo, message, details)
}

// Warn logs a warning message
func (l *featurevisorLogger) Warn(message logMessage, details logDetails) {
	l.Log(LogLevelWarn, message, details)
}

// Error logs an error message
func (l *featurevisorLogger) Error(message logMessage, details logDetails) {
	l.Log(LogLevelError, message, details)
}

// Fatal logs a fatal message and exits
func (l *featurevisorLogger) Fatal(message logMessage, details logDetails) {
	l.Log(LogLevelFatal, message, details)
}
