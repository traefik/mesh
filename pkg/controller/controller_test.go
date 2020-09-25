package controller

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/traefik/mesh/v2/pkg/k8s"
	"github.com/traefik/mesh/v2/pkg/topology"
	"github.com/traefik/traefik/v2/pkg/config/dynamic"
)

const (
	traefikMeshNamespace string = "traefik-mesh"
	minHTTPPort                 = int32(5000)
	maxHTTPPort                 = int32(5005)
	minTCPPort                  = int32(10000)
	maxTCPPort                  = int32(10005)
	minUDPPort                  = int32(15000)
	maxUDPPort                  = int32(15005)
)

type storeMock struct{}

func (a *storeMock) SetConfig(cfg *dynamic.Configuration) {}
func (a *storeMock) SetTopology(topo *topology.Topology)  {}
func (a *storeMock) SetReadiness(isReady bool)            {}

func TestController_NewMeshController(t *testing.T) {
	store := &storeMock{}
	clientMock := k8s.NewClientMock(t, "mock.yaml")
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	// Create a new controller with base HTTP mode.
	controller := NewMeshController(clientMock, Config{
		ACLEnabled:       false,
		DefaultMode:      "http",
		Namespace:        traefikMeshNamespace,
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
	store := &storeMock{}
	clientMock := k8s.NewClientMock(t, "mock.yaml")
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	// Create a new controller with base HTTP mode, in SMI mode.
	controller := NewMeshController(clientMock, Config{
		ACLEnabled:       true,
		DefaultMode:      "http",
		Namespace:        traefikMeshNamespace,
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
