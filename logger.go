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

// LogMessage represents a log message string
type LogMessage string

// LogDetails represents additional details for logging
type LogDetails map[string]interface{}

// LogHandler is a function type for handling log messages
type LogHandler func(level LogLevel, message LogMessage, details LogDetails)

// CreateLoggerOptions contains options for creating a logger
type CreateLoggerOptions struct {
	Level   *LogLevel
	Handler *LogHandler
}

// LoggerPrefix is the prefix used for all log messages
const LoggerPrefix = "[Featurevisor]"

// DefaultLogHandler is the default logging handler
func DefaultLogHandler(level LogLevel, message LogMessage, details LogDetails) {
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
	logMessage := fmt.Sprintf("%s %s: %s", LoggerPrefix, method, message)

	// Add details if provided
	if len(details) > 0 {
		logMessage += fmt.Sprintf(" %+v", details)
	}

	// Use appropriate log level
	switch level {
	case LogLevelFatal:
		log.Fatal(logMessage)
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

// Logger provides logging functionality
type Logger struct {
	level  LogLevel
	handle LogHandler
}

// AllLevels contains all available log levels in order of severity
var AllLevels = []LogLevel{
	LogLevelFatal,
	LogLevelError,
	LogLevelWarn,
	LogLevelInfo,
	LogLevelDebug, // not enabled by default
}

// DefaultLevel is the default logging level
var DefaultLevel = LogLevelInfo

// NewLogger creates a new logger instance
func NewLogger(options CreateLoggerOptions) *Logger {
	level := DefaultLevel
	if options.Level != nil {
		level = *options.Level
	}

	handler := DefaultLogHandler
	if options.Handler != nil {
		handler = *options.Handler
	}

	return &Logger{
		level:  level,
		handle: handler,
	}
}

// SetLevel sets the logging level
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// GetLevel returns the current logging level
func (l *Logger) GetLevel() LogLevel {
	return l.level
}

// shouldHandle checks if a log level should be handled based on current level
func (l *Logger) shouldHandle(level LogLevel) bool {
	currentIndex := -1
	targetIndex := -1

	// Find indices of current and target levels
	for i, logLevel := range AllLevels {
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
func (l *Logger) Log(level LogLevel, message LogMessage, details LogDetails) {
	if !l.shouldHandle(level) {
		return
	}

	if details == nil {
		details = make(LogDetails)
	}

	l.handle(level, message, details)
}

// Debug logs a debug message
func (l *Logger) Debug(message LogMessage, details LogDetails) {
	l.Log(LogLevelDebug, message, details)
}

// Info logs an info message
func (l *Logger) Info(message LogMessage, details LogDetails) {
	l.Log(LogLevelInfo, message, details)
}

// Warn logs a warning message
func (l *Logger) Warn(message LogMessage, details LogDetails) {
	l.Log(LogLevelWarn, message, details)
}

// Error logs an error message
func (l *Logger) Error(message LogMessage, details LogDetails) {
	l.Log(LogLevelError, message, details)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(message LogMessage, details LogDetails) {
	l.Log(LogLevelFatal, message, details)
}

// CreateLogger creates a new logger with the given options
func CreateLogger(options CreateLoggerOptions) *Logger {
	return NewLogger(options)
}
