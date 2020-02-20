package controller

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLogDeploy(t *testing.T) {
	log := NewDeployLog(1000)
	log.LogDeploy(time.Now(), "foo", "bar", true, "blabla")
	assert.Equal(t, 1, len(log.entries))
}

func TestGetLog(t *testing.T) {
	log := NewDeployLog(1000)
	currentTime := time.Now()
	log.LogDeploy(currentTime, "foo", "bar", true, "blabla")

	data, err := currentTime.MarshalJSON()
	assert.NoError(t, err)

	currentTimeString := string(data)

	data, err = json.Marshal(log.GetLog())
	assert.NoError(t, err)

	actual := string(data)
	expected := fmt.Sprintf("[{\"TimeStamp\":%s,\"PodName\":\"foo\",\"PodIP\":\"bar\",\"DeploySuccessful\":true,\"Reason\":\"blabla\"}]", currentTimeString)
	assert.Equal(t, expected, actual)
}

func TestLogRotationAndGetLogLength(t *testing.T) {
	log := NewDeployLog(10)

	for i := 0; i < 10; i++ {
		log.LogDeploy(time.Now(), "foo", "bar", true, "blabla")
	}

	assert.Equal(t, 10, len(log.entries))

	log.LogDeploy(time.Now(), "foo", "bar", true, "blabla")

	assert.Equal(t, 10, len(log.entries))
}
