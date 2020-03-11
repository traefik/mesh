package deploylog

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestLogDeploy(t *testing.T) {
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	deploylog := NewDeployLog(log, 1000)
	deploylog.LogDeploy(time.Now(), "foo", "bar", true, "blabla")
	assert.Equal(t, 1, len(deploylog.entries))
}

func TestGetLog(t *testing.T) {
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	deploylog := NewDeployLog(log, 1000)
	currentTime := time.Now()
	deploylog.LogDeploy(currentTime, "foo", "bar", true, "blabla")

	data, err := currentTime.MarshalJSON()
	assert.NoError(t, err)

	currentTimeString := string(data)

	data, err = json.Marshal(deploylog.GetLog())
	assert.NoError(t, err)

	actual := string(data)
	expected := fmt.Sprintf("[{\"TimeStamp\":%s,\"PodName\":\"foo\",\"PodIP\":\"bar\",\"DeploySuccessful\":true,\"Reason\":\"blabla\"}]", currentTimeString)
	assert.Equal(t, expected, actual)
}

func TestLogRotationAndGetLogLength(t *testing.T) {
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	deploylog := NewDeployLog(log, 10)

	for i := 0; i < 10; i++ {
		deploylog.LogDeploy(time.Now(), "foo", "bar", true, "blabla")
	}

	assert.Equal(t, 10, len(deploylog.entries))

	deploylog.LogDeploy(time.Now(), "foo", "bar", true, "blabla")

	assert.Equal(t, 10, len(deploylog.entries))
}
