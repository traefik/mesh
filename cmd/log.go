package cmd

import (
	"os"

	"github.com/sirupsen/logrus"
)

// parseLogLevel parses a given log level and return a standardized level.
func parseLogLevel(log logrus.FieldLogger, level string, debug bool) (logrus.Level, error) {
	logLevelStr := level
	if debug {
		logLevelStr = "debug"

		log.Warnf("Debug flag is deprecated, please consider using --loglevel=DEBUG instead")
	}

	return logrus.ParseLevel(logLevelStr)
}

// parseLogFormat parses a log format, and will return a formatter.
func parseLogFormat(format string) logrus.Formatter {
	if format == "json" {
		return &logrus.JSONFormatter{}
	}

	return &logrus.TextFormatter{DisableColors: false, FullTimestamp: true, DisableSorting: true}
}

// BuildLogger returns a formatted fieldlogger from the provided format, level, and debug configurations.
func BuildLogger(format, level string, debug bool) (logrus.FieldLogger, error) {
	log := logrus.New()

	log.SetOutput(os.Stdout)

	logLevel, err := parseLogLevel(log, level, debug)
	if err != nil {
		return log, err
	}

	log.SetLevel(logLevel)
	log.SetFormatter(parseLogFormat(format))

	return log, nil
}
