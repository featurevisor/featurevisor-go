package featurevisor

import (
	"fmt"
)

type LogLevel string

const (
	Debug LogLevel = "debug"
	Info  LogLevel = "info"
	Warn  LogLevel = "warn"
	Error LogLevel = "error"
)

type LogMessage string

type LogDetails map[string]interface{}

type LogHandler func(level LogLevel, message LogMessage, details LogDetails)

type CreateLoggerOptions struct {
	Levels  []LogLevel
	Handler LogHandler
}

const loggerPrefix = "[Featurevisor]"

var defaultLogLevels = []LogLevel{
	Warn,
	Error,
}

var defaultLogHandler = func(level LogLevel, message LogMessage, details LogDetails) {
	switch level {
	case Debug:
		fmt.Println(loggerPrefix, message, details)
	case Info:
		fmt.Println(loggerPrefix, message, details)
	case Warn:
		fmt.Println(loggerPrefix, message, details)
	case Error:
		fmt.Println(loggerPrefix, message, details)
	}
}

type Logger struct {
	Levels  []LogLevel
	Handler LogHandler
}

func (l *Logger) SetLevels(levels []LogLevel) {
	l.Levels = levels
}

func (l *Logger) Log(level LogLevel, message LogMessage, details LogDetails) {
	for _, logLevel := range l.Levels {
		if logLevel == level {
			l.Handler(level, message, details)
		}
	}
}

func (l *Logger) Debug(message LogMessage, details LogDetails) {
	l.Log(Debug, message, details)
}

func (l *Logger) Info(message LogMessage, details LogDetails) {
	l.Log(Info, message, details)
}

func (l *Logger) Warn(message LogMessage, details LogDetails) {
	l.Log(Warn, message, details)
}

func (l *Logger) Error(message LogMessage, details LogDetails) {
	l.Log(Error, message, details)
}

func CreateLogger(options CreateLoggerOptions) Logger {
	levels := options.Levels
	if levels == nil {
		levels = defaultLogLevels
	}

	handler := options.Handler
	if handler == nil {
		handler = defaultLogHandler
	}

	return Logger{Levels: levels, Handler: handler}
}
