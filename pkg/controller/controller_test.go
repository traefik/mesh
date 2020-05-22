package controller

import (
	"context"
	"os"
	"testing"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	meshNamespace string = "maesh"
	minHTTPPort          = int32(5000)
	maxHTTPPort          = int32(5005)
	minTCPPort           = int32(10000)
	maxTCPPort           = int32(10005)
	minUDPPort           = int32(15000)
	maxUDPPort           = int32(15005)
)

func TestController_NewMeshController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(t, ctx.Done(), "mock.yaml", false)
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	// Create a new controller with base HTTP mode.
	controller, err := NewMeshController(clientMock, Config{
		ACLEnabled:       false,
		DefaultMode:      "http",
		Namespace:        meshNamespace,
		IgnoreNamespaces: []string{},
		APIPort:          9000,
		MinHTTPPort:      minHTTPPort,
		MaxHTTPPort:      maxHTTPPort,
		MinTCPPort:       minTCPPort,
		MaxTCPPort:       maxTCPPort,
		MinUDPPort:       minUDPPort,
		MaxUDPPort:       maxUDPPort,
	}, log)

	require.NoError(t, err)
	assert.NotNil(t, controller)
}

func TestController_NewMeshControllerWithSMI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(t, ctx.Done(), "mock.yaml", true)
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	// Create a new controller with base HTTP mode, in SMI mode.
	controller, err := NewMeshController(clientMock, Config{
		ACLEnabled:       true,
		DefaultMode:      "http",
		Namespace:        meshNamespace,
		IgnoreNamespaces: []string{},
		APIPort:          9000,
		MinHTTPPort:      minHTTPPort,
		MaxHTTPPort:      maxHTTPPort,
		MinTCPPort:       minTCPPort,
		MaxTCPPort:       maxTCPPort,
		MinUDPPort:       minUDPPort,
		MaxUDPPort:       maxUDPPort,
	}, log)

	require.NoError(t, err)
	assert.NotNil(t, controller)
}
