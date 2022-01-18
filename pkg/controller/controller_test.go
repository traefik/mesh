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

func (a *storeMock) SetConfiguration(_ *dynamic.Configuration) {}
func (a *storeMock) SetTopology(_ *topology.Topology)          {}
func (a *storeMock) SetReadiness(_ bool)                       {}

func TestController_NewMeshController(t *testing.T) {
	store := &storeMock{}
	clientMock := k8s.NewClientMock("mock.yaml")

	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.DebugLevel)

	// Create a new controller with HTTP as a default traffic type.
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
	}, store, logger)

	assert.NotNil(t, controller)
}

func TestController_NewMeshControllerWithACLEnabled(t *testing.T) {
	store := &storeMock{}
	clientMock := k8s.NewClientMock("mock.yaml")

	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.DebugLevel)

	// Create a new controller with HTTP as a default traffic type and ACL enabled.
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
	}, store, logger)

	assert.NotNil(t, controller)
}
