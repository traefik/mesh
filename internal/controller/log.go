package controller

import (
	"encoding/json"
	"time"

	log "github.com/sirupsen/logrus"
)

type logEntry struct {
	TimeStamp        time.Time
	PodName          string
	PodIP            string
	DeploySuccessful bool
	Reason           string
}

// DeployLog holds a slice of log entries.
type DeployLog struct {
	entries    []logEntry
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
	newEntry := logEntry{
		TimeStamp:        timeStamp,
		PodName:          podName,
		PodIP:            podIP,
		DeploySuccessful: deploySuccessful,
		Reason:           reason,
	}

	if len(d.entries) >= d.maxEntries {
		// Pull elements off the front of the slice to make sure that the newly appended record is under the max record value.
		d.entries = d.entries[(len(d.entries)-d.maxEntries)+1:]
	}

	d.entries = append(d.entries, newEntry)
}

// GetLog returns a json representation of the entries list.
func (d *DeployLog) GetLog() []byte {
	data, err := json.Marshal(d.entries)
	if err != nil {
		log.Error("Could not marshal deploylog entries")
	}

	return data
}

// GetLogLength returns the number of records in the entries slice.
func (d *DeployLog) GetLogLength() int {
	return len(d.entries)
}
