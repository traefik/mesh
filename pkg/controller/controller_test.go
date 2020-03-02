package controller

import (
	"context"
	"testing"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/stretchr/testify/assert"
)

const (
	meshNamespace string = "maesh"
	minHTTPPort          = int32(5000)
	maxHTTPPort          = int32(5005)
	minTCPPort           = int32(10000)
	maxTCPPort           = int32(10005)
)

func TestNewController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(ctx.Done(), "mock.yaml", false)

	// Create a new controller with base HTTP mode.
	controller, err := NewMeshController(clientMock, MeshControllerConfig{
		SMIEnabled:       false,
		DefaultMode:      "http",
		Namespace:        meshNamespace,
		IgnoreNamespaces: []string{},
		APIPort:          9000,
		MinTCPPort:       minTCPPort,
		MaxTCPPort:       maxTCPPort,
		MinHTTPPort:      minHTTPPort,
		MaxHTTPPort:      maxHTTPPort,
	})
	assert.NoError(t, err)
	assert.NotNil(t, controller)
}

func TestNewControllerWithSMI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(ctx.Done(), "mock.yaml", true)

	// Create a new controller with base HTTP mode, in SMI mode.
	controller, err := NewMeshController(clientMock, MeshControllerConfig{
		SMIEnabled:       true,
		DefaultMode:      "http",
		Namespace:        meshNamespace,
		IgnoreNamespaces: []string{},
		APIPort:          9000,
		MinTCPPort:       minTCPPort,
		MaxTCPPort:       maxTCPPort,
		MinHTTPPort:      minHTTPPort,
		MaxHTTPPort:      maxHTTPPort,
	})
	assert.NoError(t, err)
	assert.NotNil(t, controller)
}
