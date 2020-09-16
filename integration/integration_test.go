package integration

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
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
	traefikMeshBinary    = "../dist/traefik-mesh"
	smiCRDs              = "../helm/chart/mesh/crds/"
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
	check.Suite(&HelmSuite{})

	check.TestingT(t)
}

func traefikMeshPrepare() error {
	args := []string{
		"prepare",
		"--masterurl=" + masterURL,
		"--kubeconfig=" + os.Getenv("KUBECONFIG"),
		"--loglevel=debug",
		"--clusterdomain=cluster.local",
		"--namespace=" + traefikMeshNamespace,
	}

	cmd := exec.Command(traefikMeshBinary, args...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("traefik mesh prepare has failed - %s: %w", string(output), err)
	}

	return nil
}
