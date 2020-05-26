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

// BuildLogger returns a formatted fieldlogger from the provided format, level, and debug configurations.
func BuildLogger(format, level string, debug bool) (logrus.FieldLogger, error) {
	log := logrus.New()

	log.SetOutput(os.Stdout)

	logLevelStr := level
	if debug {
		logLevelStr = "debug"

		log.Warnf("Debug flag is deprecated, please consider using --loglevel=DEBUG instead")
	}

	logLevel, err := parseLogLevel(logLevelStr)
	if err != nil {
		return log, err
	}

	log.SetLevel(logLevel)

	logFormat, err := parseLogFormat(format)
	if err != nil {
		return log, err
	}

	log.SetFormatter(logFormat)

	return log, nil
}
