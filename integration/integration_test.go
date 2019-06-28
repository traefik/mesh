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
	"github.com/containous/i3o/internal/k8s"
	"github.com/go-check/check"
	log "github.com/sirupsen/logrus"
	checker "github.com/vdemeester/shakers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	integration    = flag.Bool("integration", false, "run integration tests")
	kubeConfigPath = "/tmp/k3s-output/kubeconfig.yaml"
	masterURL      = "https://localhost:8443"
	meshNamespace  = "traefik-mesh"
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

type BaseSuite struct {
	composeProject string
	projectName    string
	dir            string
	try            *try.Try
	client         *k8s.ClientWrapper
}

func (s *BaseSuite) startk3s(_ *check.C) error {
	var err error
	s.dir, err = os.Getwd()
	if err != nil {
		return err
	}

	if err = os.MkdirAll(path.Join(s.dir, "resources/compose/images"), 0755); err != nil {
		return err
	}
	// Save i3o image in k3s.
	cmd := exec.Command("docker",
		"save", "containous/i3o:latest", "-o", path.Join(s.dir, "resources/compose/images/i3o.tar"))
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	if err != nil {
		return err
	}

	s.composeProject = path.Join(s.dir, "resources/compose/k3s.yaml")
	s.projectName = "integration-test-k3s"

	s.stopComposeProject()

	// Start k3s stack.
	cmd = exec.Command("docker-compose",
		"--file", s.composeProject, "--project-name", s.projectName,
		"up", "-d", "--scale", "node=2")
	cmd.Env = os.Environ()

	output, err = cmd.CombinedOutput()

	fmt.Println(string(output))
	if err != nil {
		return err
	}

	s.client, err = s.try.WaitClientCreated(masterURL, kubeConfigPath, 30*time.Second)
	if err != nil {
		return err
	}

	s.try = try.NewTry(s.client)
	return nil
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

func (s *BaseSuite) waitForCoreDNSStarted(c *check.C) {
	err := s.try.WaitReadyDeployment("coredns", metav1.NamespaceSystem, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitForI3oControllerStarted(c *check.C) {
	err := s.try.WaitReadyDeployment("i3o-controller", meshNamespace, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitForTiller(c *check.C) {
	err := s.try.WaitReadyDeployment("tiller-deploy", metav1.NamespaceSystem, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitForTools(c *check.C) {
	err := s.try.WaitReadyDeployment("tiny-tools", metav1.NamespaceDefault, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) startWhoami(c *check.C) {
	// Init helm with the service account created before.
	cmd := exec.Command("kubectl", "apply",
		"-f", path.Join(s.dir, "resources/whoami"))
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	c.Assert(err, checker.IsNil)

	err = s.try.WaitReadyDeployment("whoami", "whoami", 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) installHelmI3o(c *check.C) {
	// create tiller service account.
	cmd := exec.Command("kubectl", "apply",
		"-f", path.Join(s.dir, "resources/helm/serviceaccount.yaml"))
	cmd.Env = os.Environ()
	_, err := cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)

	// create tiller cluster role binding account.
	cmd = exec.Command("kubectl", "apply",
		"-f", path.Join(s.dir, "resources/helm/clusterrolebinding.yaml"))
	cmd.Env = os.Environ()
	_, err = cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)

	// Init helm with the service account created before.
	cmd = exec.Command("helm", "init",
		"--service-account", "tiller", "--upgrade")
	cmd.Env = os.Environ()
	_, err = cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)

	// Wait for tiller initialized.
	s.waitForTiller(c)

	// Install the helm chart.
	cmd = exec.Command("helm", "install",
		"../helm/chart/i3o", "--values", "resources/values.yaml")
	cmd.Env = os.Environ()
	_, err = cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) installTinyToolsI3o(c *check.C) {
	// Create new tiny tools deployment.
	cmd := exec.Command("kubectl", "apply",
		"-f", path.Join(s.dir, "resources/tools/deployment.yaml"))
	cmd.Env = os.Environ()
	_, err := cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)

	// Wait for tools to be initialized.
	s.waitForTools(c)
}

func (s *BaseSuite) getToolsPodI3o(c *check.C) *corev1.Pod {
	pod, exists, err := s.client.GetPod(metav1.NamespaceDefault, "tiny-tools")
	c.Assert(err, checker.IsNil)
	c.Assert(exists, checker.True)

	return pod
}
