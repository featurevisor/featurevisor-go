package featurevisor

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

func TestLogLevels(t *testing.T) {
	levels := []LogLevel{
		LogLevelFatal,
		LogLevelError,
		LogLevelWarn,
		LogLevelInfo,
		LogLevelDebug,
	}

	for _, level := range levels {
		t.Run(string(level), func(t *testing.T) {
			if level == "" {
				t.Error("Log level should not be empty")
			}
		})
	}
}

func TestAllLevelsOrder(t *testing.T) {
	expectedOrder := []LogLevel{
		LogLevelFatal,
		LogLevelError,
		LogLevelWarn,
		LogLevelInfo,
		LogLevelDebug,
	}

	if len(allLevels) != len(expectedOrder) {
		t.Errorf("allLevels length = %d, expected %d", len(allLevels), len(expectedOrder))
	}

	for i, level := range allLevels {
		if level != expectedOrder[i] {
			t.Errorf("allLevels[%d] = %s, expected %s", i, level, expectedOrder[i])
		}
	}
}

func TestNewLogger(t *testing.T) {
	// Test default logger
	logger := newLogger(loggerOptions{})
	if logger.GetLevel() != defaultLevel {
		t.Errorf("Default level = %s, expected %s", logger.GetLevel(), defaultLevel)
	}

	// Test logger with custom level
	customLevel := LogLevelDebug
	logger = newLogger(loggerOptions{
		Level: &customLevel,
	})
	if logger.GetLevel() != customLevel {
		t.Errorf("Custom level = %s, expected %s", logger.GetLevel(), customLevel)
	}
}

func TestLoggerSetLevel(t *testing.T) {
	logger := newLogger(loggerOptions{})

	// Test setting different levels
	testLevels := []LogLevel{LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelFatal}

	for _, level := range testLevels {
		logger.SetLevel(level)
		if logger.GetLevel() != level {
			t.Errorf("Set level = %s, but GetLevel() returned %s", level, logger.GetLevel())
		}
	}
}

func TestLoggerShouldHandle(t *testing.T) {
	logger := newLogger(loggerOptions{})

	tests := []struct {
		name         string
		currentLevel LogLevel
		targetLevel  LogLevel
		shouldHandle bool
	}{
		{
			name:         "info level should handle info",
			currentLevel: LogLevelInfo,
			targetLevel:  LogLevelInfo,
			shouldHandle: true,
		},
		{
			name:         "info level should handle warn",
			currentLevel: LogLevelInfo,
			targetLevel:  LogLevelWarn,
			shouldHandle: true,
		},
		{
			name:         "info level should handle error",
			currentLevel: LogLevelInfo,
			targetLevel:  LogLevelError,
			shouldHandle: true,
		},
		{
			name:         "info level should handle fatal",
			currentLevel: LogLevelInfo,
			targetLevel:  LogLevelFatal,
			shouldHandle: true,
		},
		{
			name:         "info level should not handle debug",
			currentLevel: LogLevelInfo,
			targetLevel:  LogLevelDebug,
			shouldHandle: false,
		},
		{
			name:         "debug level should handle debug",
			currentLevel: LogLevelDebug,
			targetLevel:  LogLevelDebug,
			shouldHandle: true,
		},
		{
			name:         "debug level should handle info",
			currentLevel: LogLevelDebug,
			targetLevel:  LogLevelInfo,
			shouldHandle: true,
		},
		{
			name:         "warn level should not handle debug",
			currentLevel: LogLevelWarn,
			targetLevel:  LogLevelDebug,
			shouldHandle: false,
		},
		{
			name:         "warn level should not handle info",
			currentLevel: LogLevelWarn,
			targetLevel:  LogLevelInfo,
			shouldHandle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger.SetLevel(tt.currentLevel)
			result := logger.shouldHandle(tt.targetLevel)
			if result != tt.shouldHandle {
				t.Errorf("shouldHandle(%s) with level %s = %v, expected %v",
					tt.targetLevel, tt.currentLevel, result, tt.shouldHandle)
			}
		})
	}
}

func TestDefaultLogHandler(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Test different log levels
	testCases := []struct {
		level   LogLevel
		message logMessage
		details logDetails
		expect  string
	}{
		{
			level:   LogLevelInfo,
			message: "Test info message",
			details: logDetails{"key": "value"},
			expect:  "[INFO]",
		},
		{
			level:   LogLevelWarn,
			message: "Test warning message",
			details: logDetails{"warning": true},
			expect:  "[WARN]",
		},
		{
			level:   LogLevelError,
			message: "Test error message",
			details: logDetails{"error": "test"},
			expect:  "[ERROR]",
		},
		{
			level:   LogLevelDebug,
			message: "Test debug message",
			details: logDetails{"debug": "info"},
			expect:  "[DEBUG]",
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.level), func(t *testing.T) {
			buf.Reset()

			defaultLogHandler(tc.level, tc.message, tc.details)

			output := buf.String()
			if !strings.Contains(output, tc.expect) {
				t.Errorf("Expected output to contain %s, got: %s", tc.expect, output)
			}
			if !strings.Contains(output, string(tc.message)) {
				t.Errorf("Expected output to contain message '%s', got: %s", tc.message, output)
			}
		})
	}
}

func TestLoggerMethods(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	level := LogLevelInfo
	logger := newLogger(loggerOptions{
		Level: &level,
	})

	// Test all logging methods
	testCases := []struct {
		name   string
		method func()
		expect string
	}{
		{
			name: "Debug",
			method: func() {
				logger.Debug("debug message", logDetails{"debug": true})
			},
			expect: "", // Debug messages should be filtered out at info level
		},
		{
			name: "Info",
			method: func() {
				logger.Info("info message", logDetails{"info": true})
			},
			expect: "info",
		},
		{
			name: "Warn",
			method: func() {
				logger.Warn("warn message", logDetails{"warn": true})
			},
			expect: "warn",
		},
		{
			name: "Error",
			method: func() {
				logger.Error("error message", logDetails{"error": true})
			},
			expect: "error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()
			tc.method()

			output := buf.String()
			if tc.expect == "" {
				// If no output is expected, check that nothing was logged
				if output != "" {
					t.Errorf("Expected no output, got: %s", output)
				}
			} else {
				// If output is expected, check that it contains the expected string
				if !strings.Contains(strings.ToLower(output), tc.expect) {
					t.Errorf("Expected output to contain '%s', got: %s", tc.expect, output)
				}
			}
		})
	}
}

func TestCustomLogHandler(t *testing.T) {
	var capturedLevel LogLevel
	var capturedMessage logMessage
	var capturedDetails logDetails

	var customHandler logHandler = func(level LogLevel, message logMessage, details logDetails) {
		capturedLevel = level
		capturedMessage = message
		capturedDetails = details
	}

	logger := newLogger(loggerOptions{
		Handler: &customHandler,
	})

	expectedMessage := logMessage("test message")
	expectedDetails := logDetails{"key": "value"}

	logger.Info(expectedMessage, expectedDetails)

	if capturedLevel != LogLevelInfo {
		t.Errorf("Captured level = %s, expected %s", capturedLevel, LogLevelInfo)
	}
	if capturedMessage != expectedMessage {
		t.Errorf("Captured message = %s, expected %s", capturedMessage, expectedMessage)
	}
	if capturedDetails["key"] != expectedDetails["key"] {
		t.Errorf("Captured details = %v, expected %v", capturedDetails, expectedDetails)
	}
}

func TestInternalLoggerFactory(t *testing.T) {
	// Test with no options
	logger := newLogger(loggerOptions{})
	if logger.GetLevel() != defaultLevel {
		t.Errorf("CreateLogger default level = %s, expected %s", logger.GetLevel(), defaultLevel)
	}

	// Test with custom options
	customLevel := LogLevelDebug
	logger = newLogger(loggerOptions{
		Level: &customLevel,
	})
	if logger.GetLevel() != customLevel {
		t.Errorf("CreateLogger custom level = %s, expected %s", logger.GetLevel(), customLevel)
	}
}

func BenchmarkLoggerInfo(b *testing.B) {
	logger := newLogger(loggerOptions{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message", logDetails{"benchmark": i})
	}
}
