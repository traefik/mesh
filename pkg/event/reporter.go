package event

// Reporter represents something that can report events for subjects. Events
// can be of three different severities: Info, Warn and Error.
type Reporter interface {
	Info(desc string)
	Infof(format string, a ...interface{})
	Warn(desc string)
	Warnf(format string, a ...interface{})
	Error(desc string)
	Errorf(format string, a ...interface{})

	// ForSubject adds context about a subject to a new reporter and returns
	// it. It does not modify the original reporter.
	ForSubject(ns, typ, name string) Reporter

	// WithMetadata adds metadata to a new reporter and returns it. It does not
	// modify the original reporter.
	WithMetadata(metadata Metadata) Reporter
}

// Subject contains information about the subject of a reported event.
type Subject struct {
	Namespace string
	Kind      string
	Name      string
}

// Metadata contains extra information about a reported event.
type Metadata map[string]string
