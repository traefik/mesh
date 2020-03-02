package deploylog

import (
	"time"

	log "github.com/sirupsen/logrus"
)

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
	entries    []Entry
	maxEntries int
}

// NewDeployLog returns an initialized DeployLog.
func NewDeployLog(maxEntries int) *DeployLog {
	d := &DeployLog{
		maxEntries: maxEntries,
	}

	if err := d.Init(); err != nil {
		log.Error("Could not initialize DeployLog")
	}

	return d
}

// Init handles any DeployLog initialization.
func (d *DeployLog) Init() error {
	log.Debug("DeployLog.Init")

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
