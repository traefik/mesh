package controller

import (
	"encoding/json"
	"time"

	log "github.com/sirupsen/logrus"
)

type logEntry struct {
	TimeStamp        time.Time
	PodName          string
	DeploySuccessful bool
	Reason           string
}

type DeployLog struct {
	Entries []logEntry
}

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
func (d *DeployLog) LogDeploy(timeStamp time.Time, podName string, deploySuccessful bool, reason string) {
	newEntry := logEntry{
		TimeStamp:        timeStamp,
		PodName:          podName,
		DeploySuccessful: deploySuccessful,
		Reason:           reason,
	}

	d.Entries = append(d.Entries, newEntry)
}

// GetLog returns a json representation of the entries list.
func (d *DeployLog) GetLog() string {
	data, err := json.Marshal(d.Entries)
	if err != nil {
		log.Error("Could not marshal deploylog entries")
	}

	return string(data)
}
