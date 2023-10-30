package featurevisor

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := CreateLogger(CreateLoggerOptions{
		Levels: []LogLevel{Debug, Info, Warn, Error},
		Handler: func(level LogLevel, message LogMessage, details LogDetails) {
			buf.WriteString(string(level))
			buf.WriteString(" ")
			buf.WriteString(string(message))
			buf.WriteString(" ")
			buf.WriteString(fmt.Sprintf("%v", details))
			buf.WriteString("\n")
		},
	})

	t.Run("Test debug log", func(t *testing.T) {
		buf.Reset()
		logger.Debug("Debug message", LogDetails{"key": "value"})
		if !strings.Contains(buf.String(), "Debug message") {
			t.Errorf("Debug log did not work as expected")
		}
	})

	t.Run("Test info log", func(t *testing.T) {
		buf.Reset()
		logger.Info("Info message", LogDetails{"key": "value"})
		if !strings.Contains(buf.String(), "Info message") {
			t.Errorf("Info log did not work as expected")
		}
	})

	t.Run("Test warn log", func(t *testing.T) {
		buf.Reset()
		logger.Warn("Warn message", LogDetails{"key": "value"})
		if !strings.Contains(buf.String(), "Warn message") {
			t.Errorf("Warn log did not work as expected")
		}
	})

	t.Run("Test error log", func(t *testing.T) {
		buf.Reset()
		logger.Error("Error message", LogDetails{"key": "value"})
		if !strings.Contains(buf.String(), "Error message") {
			t.Errorf("Error log did not work as expected")
		}
	})
}
