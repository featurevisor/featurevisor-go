package sdk

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

type Logger interface {
	SetLevels(levels []LogLevel)
	Log(level LogLevel, message LogMessage, details LogDetails)
	Debug(message LogMessage, details LogDetails)
	Info(message LogMessage, details LogDetails)
	Warn(message LogMessage, details LogDetails)
	Error(message LogMessage, details LogDetails)
}

type logger struct {
	levels  []LogLevel
	handler LogHandler
}

func (l *logger) SetLevels(levels []LogLevel) {
	l.levels = levels
}

func (l *logger) Log(level LogLevel, message LogMessage, details LogDetails) {
	for _, logLevel := range l.levels {
		if logLevel == level {
			l.handler(level, message, details)
			break
		}
	}
}

func (l *logger) Debug(message LogMessage, details LogDetails) {
	l.Log(Debug, message, details)
}

func (l *logger) Info(message LogMessage, details LogDetails) {
	l.Log(Info, message, details)
}

func (l *logger) Warn(message LogMessage, details LogDetails) {
	l.Log(Warn, message, details)
}

func (l *logger) Error(message LogMessage, details LogDetails) {
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

	return &logger{levels: levels, handler: handler}
}
