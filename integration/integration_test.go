package integration

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/containous/maesh/integration/try"
	"github.com/containous/maesh/internal/k8s"
	"github.com/go-check/check"
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
	if !*integration {
		log.Println("Integration tests disabled.")
		return
	}

	images = append(images, image{"containous/maesh:latest", false})
	images = append(images, image{"containous/whoami:v1.0.1", true})
	images = append(images, image{"coredns/coredns:1.2.6", true})
	images = append(images, image{"coredns/coredns:1.3.1", true})
	images = append(images, image{"coredns/coredns:1.4.0", true})
	images = append(images, image{"coredns/coredns:1.5.2", true})
	images = append(images, image{"coredns/coredns:1.6.3", true})
	images = append(images, image{"gcr.io/kubernetes-helm/tiller:v2.14.1", true})
	images = append(images, image{"giantswarm/tiny-tools:3.9", true})
	images = append(images, image{"traefik:v2.0.0-rc1", true})

	check.Suite(&SMISuite{})
	check.Suite(&KubernetesSuite{})
	check.Suite(&CoreDNSSuite{})

	dir, _ := os.Getwd()
	err := os.RemoveAll(path.Join(dir, fmt.Sprintf("resources/compose/images")))
	if err != nil {
		fmt.Printf("unable to cleanup: %v", err)
	}

	check.TestingT(t)
}

type image struct {
	name string
	pull bool
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

	for _, image := range images {
		name := strings.ReplaceAll(image.name, "/", "-")
		name = strings.ReplaceAll(name, ":", "-")
		name = strings.ReplaceAll(name, ".", "-")
		p := path.Join(s.dir, fmt.Sprintf("resources/compose/images/%s.tar", name))
		if err = saveDockerImage(image.name, p, image.pull); err != nil {
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

func (s *BaseSuite) startAndWaitForCoreDNS(c *check.C) {
	cmd := exec.Command("kubectl", "apply", "-f", path.Join(s.dir, "resources/coredns"))
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	c.Assert(err, checker.IsNil)
	err = s.try.WaitReadyDeployment("coredns", metav1.NamespaceSystem, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitForMaeshControllerStarted(c *check.C) {
	err := s.try.WaitReadyDeployment("maesh-controller", "maesh", 30*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitForTiller(c *check.C) {
	err := s.try.WaitReadyDeployment("tiller-deploy", metav1.NamespaceSystem, 30*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitForTools(c *check.C) {
	err := s.try.WaitReadyDeployment("tiny-tools", metav1.NamespaceDefault, 30*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitKubectlExecCommand(c *check.C, argSlice []string, data string) {
	err := s.try.WaitCommandExecute("kubectl", argSlice, data, 10*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitKubectlExecCommandReturn(_ *check.C, argSlice []string) (string, error) {
	return s.try.WaitCommandExecuteReturn("kubectl", argSlice, 10*time.Second)
}

func (s *BaseSuite) startWhoami(c *check.C) {
	// Init helm with the service account created before.
	cmd := exec.Command("kubectl", "apply",
		"-f", path.Join(s.dir, "resources/whoami"))
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	c.Assert(err, checker.IsNil)

	err = s.try.WaitReadyDeployment("whoami", "whoami", 30*time.Second)
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

func (s *BaseSuite) installHelmMaesh(_ *check.C, smi bool) error {
	// Install the helm chart.
	argSlice := []string{"install", "../helm/chart/maesh", "--values", "resources/values.yaml", "--name", "powpow", "--namespace", "maesh"}

	if smi {
		argSlice = append(argSlice, "--set", "smi=true")
	}

	return s.try.WaitCommandExecute("helm", argSlice, "powpow", 10*time.Second)
}

func (s *BaseSuite) unInstallHelmMaesh(c *check.C) {
	// Install the helm chart.
	argSlice := []string{"delete", "--purge", "powpow"}
	err := s.try.WaitCommandExecute("helm", argSlice, "deleted", 10*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) setCoreDNSVersion(c *check.C, version string) {
	// Get current coreDNS deployment.

	deployment, exists, err := s.client.GetDeployment(metav1.NamespaceSystem, "coredns")
	c.Assert(err, checker.IsNil)
	c.Assert(exists, checker.True)

	newDeployment := deployment.DeepCopy()
	c.Assert(len(newDeployment.Spec.Template.Spec.Containers), checker.Equals, 1)

	newDeployment.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("coredns/coredns:%s", version)

	err = s.try.WaitUpdateDeployment(newDeployment, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) installTinyToolsMaesh(c *check.C) {
	// Create new tiny tools deployment.
	cmd := exec.Command("kubectl", "apply",
		"-f", path.Join(s.dir, "resources/tools/deployment.yaml"))
	cmd.Env = os.Environ()
	_, err := cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)

	// Wait for tools to be initialized.
	s.waitForTools(c)
}

func (s *BaseSuite) getToolsPodMaesh(c *check.C) *corev1.Pod {
	podList, err := s.client.ListPodWithOptions(metav1.NamespaceDefault, metav1.ListOptions{
		LabelSelector: "app=tiny-tools",
	})
	c.Assert(err, checker.IsNil)
	c.Assert(len(podList.Items), checker.Equals, 1)

	return &podList.Items[0]
}
