package cmd

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

// parseLogLevel parses a given log level and returns a standardized level.
func parseLogLevel(level string) (logrus.Level, error) {
	return logrus.ParseLevel(level)
}

// parseLogFormat parses a log format and returns a formatter.
func parseLogFormat(format string) (logrus.Formatter, error) {
	switch format {
	case "json":
		return &logrus.JSONFormatter{}, nil
	case "common":
		return &logrus.TextFormatter{DisableColors: false, FullTimestamp: true, DisableSorting: true}, nil
	default:
		return nil, fmt.Errorf("invalid logging format: %s", format)
	}
}

// NewLogger returns a new field logger with the provided format and level.
func NewLogger(format, level string) (logrus.FieldLogger, error) {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)

	logLevel, err := parseLogLevel(level)
	if err != nil {
		return logger, err
	}

	logger.SetLevel(logLevel)

	logFormat, err := parseLogFormat(format)
	if err != nil {
		return logger, err
	}

	logger.SetFormatter(logFormat)

	return logger, nil
}
