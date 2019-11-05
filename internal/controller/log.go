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
	Entries []logEntry
}

// NewDeployLog returns an initialized DeployLog.
func NewDeployLog() *DeployLog {
	d := &DeployLog{}

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

	d.Entries = append(d.Entries, newEntry)
}

// GetLog returns a json representation of the entries list.
func (d *DeployLog) GetLog() []byte {
	data, err := json.Marshal(d.Entries)
	if err != nil {
		log.Error("Could not marshal deploylog entries")
	}

	return data
}
