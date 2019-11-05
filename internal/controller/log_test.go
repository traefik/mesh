package controller

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLogDeploy(t *testing.T) {
	log := NewDeployLog()
	log.LogDeploy(time.Now(), "foo", true, "")
	assert.Equal(t, 1, len(log.Entries))
}

func TestGetLog(t *testing.T) {
	log := NewDeployLog()
	currentTime := time.Now()
	log.LogDeploy(currentTime, "foo", true, "")

	data, err := currentTime.MarshalJSON()
	assert.NoError(t, err)

	currentTimeString := string(data)
	actual := log.GetLog()
	expected := fmt.Sprintf("[{\"TimeStamp\":%s,\"PodName\":\"foo\",\"DeploySuccessful\":true,\"Reason\":\"\"}]", currentTimeString)
	assert.Equal(t, expected, actual)
}
