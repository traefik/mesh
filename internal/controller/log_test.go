package controller

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLogDeploy(t *testing.T) {
	log := NewDeployLog()
	log.LogDeploy(time.Now(), "foo", "bar", true, "blabla")
	assert.Equal(t, 1, len(log.Entries))
}

func TestGetLog(t *testing.T) {
	log := NewDeployLog()
	currentTime := time.Now()
	log.LogDeploy(currentTime, "foo", "bar", true, "blabla")

	data, err := currentTime.MarshalJSON()
	assert.NoError(t, err)

	currentTimeString := string(data)
	actual := log.GetLog()
	expected := fmt.Sprintf("[{\"TimeStamp\":%s,\"PodName\":\"foo\",\"PodIP\":\"bar\",\"DeploySuccessful\":true,\"Reason\":\"blabla\"}]", currentTimeString)
	assert.Equal(t, expected, actual)
}
