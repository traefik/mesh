package event

import "github.com/sirupsen/logrus"

// LogrusReporter implements the Reporter interface and uses logrus to log the
// events as they are reported.
type LogrusReporter struct {
	log logrus.FieldLogger

	subject  *Subject
	metadata Metadata
}

// NewLogrusReporter returns a LogrusReporter given a logrus FieldLogger.
func NewLogrusReporter(log logrus.FieldLogger) LogrusReporter {
	return LogrusReporter{log: log}
}

// Info reports an event with the Info severity.
func (r LogrusReporter) Info(desc string) {
	r.toLogrusEntry(r.subject).Info(desc)
}

// Infof reports an event with the Info severity with a formatted description.
func (r LogrusReporter) Infof(format string, a ...interface{}) {
	r.toLogrusEntry(r.subject).Infof(format, a...)
}

// Warn reports an event with the Warn severity.
func (r LogrusReporter) Warn(desc string) {
	r.toLogrusEntry(r.subject).Warn(desc)
}

// Warnf reports an event with the Warn severity with a formatted description.
func (r LogrusReporter) Warnf(format string, a ...interface{}) {
	r.toLogrusEntry(r.subject).Warnf(format, a...)
}

// Error reports an event with the Error severity.
func (r LogrusReporter) Error(desc string) {
	r.toLogrusEntry(r.subject).Error(desc)
}

// Errorf reports an event with the Error severity with a formatted description.
func (r LogrusReporter) Errorf(format string, a ...interface{}) {
	r.toLogrusEntry(r.subject).Errorf(format, a...)
}

// ForSubject adds context about a subject to a new LogrusReporter and returns
// it. It does not modify the original reporter.
func (r LogrusReporter) ForSubject(ns, kind, name string) Reporter {
	return LogrusReporter{
		log: r.log,
		subject: &Subject{
			Namespace: ns,
			Kind:      kind,
			Name:      name,
		},
		metadata: r.metadata,
	}
}

// WithMetadata adds context about a subject to a new LogrusReporter and returns
// it. It does not modify the original reporter.
func (r LogrusReporter) WithMetadata(metadata Metadata) Reporter {
	return LogrusReporter{
		log:      r.log,
		subject:  r.subject,
		metadata: metadata,
	}
}

// toLogrusEntry generates a logrus.Entry from a given subject.
func (r LogrusReporter) toLogrusEntry(s *Subject) *logrus.Entry {
	if s == nil {
		return r.log.WithFields(nil)
	}

	return r.log.WithFields(logrus.Fields{
		"namespace": s.Namespace,
		"kind":      s.Kind,
		"name":      s.Name,
	})
}
