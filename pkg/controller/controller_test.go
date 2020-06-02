package controller

import (
	"context"
	"os"
	"testing"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/topology"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
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

type storeMock struct{}

func (a *storeMock) SetConfig(cfg *dynamic.Configuration) {}
func (a *storeMock) SetTopology(topo *topology.Topology)  {}
func (a *storeMock) SetReadiness(isReady bool)            {}

func TestController_NewMeshController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := &storeMock{}
	clientMock := k8s.NewClientMock(t, ctx.Done(), "mock.yaml", false)
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	// Create a new controller with base HTTP mode.
	controller := NewMeshController(clientMock, Config{
		ACLEnabled:       false,
		DefaultMode:      "http",
		Namespace:        meshNamespace,
		IgnoreNamespaces: []string{},
		MinHTTPPort:      minHTTPPort,
		MaxHTTPPort:      maxHTTPPort,
		MinTCPPort:       minTCPPort,
		MaxTCPPort:       maxTCPPort,
		MinUDPPort:       minUDPPort,
		MaxUDPPort:       maxUDPPort,
	}, store, log)

	assert.NotNil(t, controller)
}

func TestController_NewMeshControllerWithSMI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := &storeMock{}
	clientMock := k8s.NewClientMock(t, ctx.Done(), "mock.yaml", true)
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	// Create a new controller with base HTTP mode, in SMI mode.
	controller := NewMeshController(clientMock, Config{
		ACLEnabled:       true,
		DefaultMode:      "http",
		Namespace:        meshNamespace,
		IgnoreNamespaces: []string{},
		MinHTTPPort:      minHTTPPort,
		MaxHTTPPort:      maxHTTPPort,
		MinTCPPort:       minTCPPort,
		MaxTCPPort:       maxTCPPort,
		MinUDPPort:       minUDPPort,
		MaxUDPPort:       maxUDPPort,
	}, store, log)

	assert.NotNil(t, controller)
}
