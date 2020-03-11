package deploylog

import (
	"time"

	"github.com/sirupsen/logrus"
)

// Ensure the Deploylog fits the interface
var _ Interface = (*DeployLog)(nil)

// Entry holds the details of a deployment.
type Entry struct {
	TimeStamp        time.Time
	PodName          string
	PodIP            string
	DeploySuccessful bool
	Reason           string
}

// DeployLog holds a slice of log entries.
type DeployLog struct {
	log        logrus.FieldLogger
	entries    []Entry
	maxEntries int
}

// Interface is an interface to interact with the REST API.
type Interface interface {
	LogDeploy(timeStamp time.Time, podName string, podIP string, deploySuccessful bool, reason string)
	GetLog() []Entry
}

// NewDeployLog returns an initialized DeployLog.
func NewDeployLog(log logrus.FieldLogger, maxEntries int) *DeployLog {
	d := &DeployLog{
		log:        log,
		maxEntries: maxEntries,
	}

	if err := d.init(); err != nil {
		log.Error("Could not initialize DeployLog")
	}

	return d
}

// init handles any DeployLog initialization.
func (d *DeployLog) init() error {
	d.log.Debug("DeployLog.Init")

	return nil
}

// LogDeploy adds a record to the entries list.
func (d *DeployLog) LogDeploy(timeStamp time.Time, podName string, podIP string, deploySuccessful bool, reason string) {
	newEntry := Entry{
		TimeStamp:        timeStamp,
		PodName:          podName,
		PodIP:            podIP,
		DeploySuccessful: deploySuccessful,
		Reason:           reason,
	}

	for len(d.entries) >= d.maxEntries {
		// Pull elements off the front of the slice to make sure that the newly appended record is one under the max record value.
		d.entries = d.entries[1:]
	}

	d.entries = append(d.entries, newEntry)
}

// GetLog returns a json representation of the entries list.
func (d *DeployLog) GetLog() []Entry {
	return d.entries
}
