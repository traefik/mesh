package integration

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	"github.com/containous/i3o/integration/try"
	"github.com/containous/i3o/utils"
	"github.com/go-check/check"
	log "github.com/sirupsen/logrus"
	checker "github.com/vdemeester/shakers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	integration    = flag.Bool("integration", true, "run integration tests")
	kubeConfigPath = "/tmp/k3s-output/kubeconfig.yaml"
	masterURL      = "https://localhost:6443"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

func init() {
	flag.Parse()
	if !*integration {
		log.Info("Integration tests disabled.")
		return
	}

	check.Suite(&StartI3oSuite{})
}

var i3oBinary = "../dist/i3o"

type BaseSuite struct {
	composeProject string
	projectName    string
	clients        *utils.ClientWrapper
}

func (s *BaseSuite) createComposeProject(c *check.C, name string) {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	projectName := fmt.Sprintf("integration-test-%s", name)
	composeFile := path.Join(dir, fmt.Sprintf("resources/compose/%s.yaml", name))

	fmt.Println(s.composeProject)

	cmd := exec.Command("docker-compose",
		"--file", composeFile, "--project-name", projectName,
		"up", "-d")
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

	s.composeProject = composeFile
	s.projectName = projectName
	s.clients, err = try.WaitClientCreated(masterURL, kubeConfigPath, 30*time.Second)
	c.Check(err, checker.IsNil)

}

func (s *BaseSuite) stopComposeProject() {
	// shutdown and delete compose project
	cmd := exec.Command("docker-compose", "--file", s.composeProject,
		"--project-name", s.projectName,
		"down", "--volumes", "--remove-orphans")
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	if err != nil {
		fmt.Println(err)
	}
}

func (s *BaseSuite) waitForCoreDNS(c *check.C) {
	err := try.WaitReadyReplica(s.clients, "coredns", metav1.NamespaceSystem, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) startPathI3o(_ *check.C) {
	cmd := exec.Command(i3oBinary, "patch",
		"--master", masterURL, "--kubeconfig", kubeConfigPath)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	if err != nil {
		fmt.Println(err)
	}
}
