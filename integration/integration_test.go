package integration

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
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
	images         []image
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type image struct {
	name string
	pull bool
}

func init() {
	flag.Parse()
	if !*integration {
		log.Info("Integration tests disabled.")
		return
	}

	images = append(images, image{"containous/i3o:latest", false})
	images = append(images, image{"containous/whoami:latest", true})
	images = append(images, image{"coredns/coredns:1.2.6", true})
	images = append(images, image{"coredns/coredns:1.3.1", true})
	images = append(images, image{"coredns/coredns:1.4.0", true})
	images = append(images, image{"gcr.io/kubernetes-helm/tiller:v2.14.1", true})

	check.Suite(&CurlI3oSuite{})
	check.Suite(&CoreDNSSuite{})

	dir, _ := os.Getwd()
	err := os.RemoveAll(path.Join(dir, fmt.Sprintf("resources/compose/images")))
	if err != nil {
		fmt.Printf("unable to cleanup: %v", err)
	}
}

type BaseSuite struct {
	composeProject string
	projectName    string
	dir            string
	try            *try.Try
	client         *k8s.ClientWrapper
}

func (s *BaseSuite) startk3s(_ *check.C, coreDNSDeploy bool) error {
	var err error
	s.dir, err = os.Getwd()
	if err != nil {
		return err
	}

	if err = os.MkdirAll(path.Join(s.dir, "resources/compose/images"), 0755); err != nil {
		return err
	}

	for _, image := range images {
		name := strings.ReplaceAll(image.name, "/", "-")
		name = strings.ReplaceAll(name, ":", "-")
		name = strings.ReplaceAll(name, ".", "-")
		p := path.Join(s.dir, fmt.Sprintf("resources/compose/images/%s.tar", name))
		err = saveDockerImage(image.name, p, image.pull)
		if err != nil {
			return err
		}
	}

	s.composeProject = path.Join(s.dir, "resources/compose/k3s.yaml")
	s.projectName = "integration-test-k3s"

	s.stopComposeProject()

	// Start k3s stack.
	cmd := exec.Command("docker-compose",
		"--file", s.composeProject, "--project-name", s.projectName,
		"up", "-d", "--scale", "node=0")
	if !coreDNSDeploy {
		_ = os.Setenv("K3S_OPTS", "--no-deploy coredns")
	}
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

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

func saveDockerImage(image string, p string, pull bool) error {
	if pull {
		cmd := exec.Command("docker", "pull", image)
		cmd.Env = os.Environ()

		output, err := cmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			return err
		}
	}
	cmd := exec.Command("docker", "save", image, "-o", p)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		return err
	}

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

func (s *BaseSuite) waitForCoreDNSDeleted(c *check.C) {
	err := s.try.WaitDeleteDeployment("coredns", metav1.NamespaceSystem, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitForI3oControllerStarted(c *check.C) {
	err := s.try.WaitReadyDeployment("i3o-controller", k8s.MeshNamespace, 60*time.Second)
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

func (s *BaseSuite) waitKubectlExecCommand(c *check.C, argSlice []string, data string) {
	err := s.try.WaitCommandExecute("kubectl", argSlice, data, 60*time.Second)
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

func (s *BaseSuite) installTiller(c *check.C) {
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
}

func (s *BaseSuite) installHelmI3o(_ *check.C) error {
	// Install the helm chart.
	argSlice := []string{"install", "../helm/chart/i3o", "--values", "resources/values.yaml", "--name", "powpow"}
	return s.try.WaitCommandExecute("helm", argSlice, "powpow", 60*time.Second)
}

func (s *BaseSuite) unInstallHelmI3o(c *check.C) {
	// Install the helm chart.
	argSlice := []string{"delete", "--purge", "powpow"}
	err := s.try.WaitCommandExecute("helm", argSlice, "", 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) installCoreDNS(c *check.C, version string) {
	// Create new tiny tools deployment.
	cmd := exec.Command("kubectl", "apply",
		"-f", path.Join(s.dir, fmt.Sprintf("resources/coredns/coredns-v%s.yaml", version)))
	cmd.Env = os.Environ()
	_, err := cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)

	// Wait for tools to be initialized.
	s.waitForCoreDNSStarted(c)
}

func (s *BaseSuite) unInstallCoreDNS(c *check.C, version string) {
	// Create new tiny tools deployment.
	cmd := exec.Command("kubectl", "delete",
		"-f", path.Join(s.dir, fmt.Sprintf("resources/coredns/coredns-v%s.yaml", version)))
	cmd.Env = os.Environ()
	_, err := cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)

	// Wait for tools to be initialized.
	s.waitForCoreDNSDeleted(c)
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
	podList, err := s.client.ListPodWithOptions(metav1.NamespaceDefault, metav1.ListOptions{
		LabelSelector: "app=tiny-tools",
	})
	c.Assert(err, checker.IsNil)
	c.Assert(len(podList.Items), checker.Equals, 1)

	return &podList.Items[0]
}
