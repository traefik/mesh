package integration

import (
	"flag"
	"log"
	"testing"

	"github.com/go-check/check"
	"github.com/sirupsen/logrus"
)

var (
	integration          = flag.Bool("integration", false, "run integration tests")
	debug                = flag.Bool("debug", false, "debug log level")
	masterURL            = "https://localhost:8443"
	k3dClusterName       = "traefik-mesh-integration"
	traefikMeshNamespace = "traefik-mesh"
	smiCRDs              = "./testdata/crds/"
	testNamespace        = "test"
)

func Test(t *testing.T) {
	if !*integration {
		log.Println("Integration tests disabled")
		return
	}

	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	check.Suite(&ACLDisabledSuite{})
	check.Suite(&ACLEnabledSuite{})
	check.Suite(&CoreDNSSuite{})
	check.Suite(&KubeDNSSuite{})

	check.TestingT(t)
}
