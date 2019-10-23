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

	"github.com/cenkalti/backoff/v3"
	"github.com/containous/maesh/integration/try"
	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	integration    = flag.Bool("integration", false, "run integration tests")
	masterURL      = "https://localhost:8443"
	images         []image
	k3dClusterName = "maesh-integration"
)

func Test(t *testing.T) {
	if !*integration {
		log.Println("Integration tests disabled.")
		return
	}

	check.Suite(&SMISuite{})
	check.Suite(&KubernetesSuite{})
	check.Suite(&CoreDNSSuite{})
	check.Suite(&KubeDNSSuite{})

	images = append(images, image{"containous/maesh:latest", false})
	images = append(images, image{"containous/whoami:v1.0.1", true})
	images = append(images, image{"coredns/coredns:1.2.6", true})
	images = append(images, image{"coredns/coredns:1.3.1", true})
	images = append(images, image{"coredns/coredns:1.4.0", true})
	images = append(images, image{"coredns/coredns:1.5.2", true})
	images = append(images, image{"coredns/coredns:1.6.3", true})
	images = append(images, image{"gcr.io/kubernetes-helm/tiller:v2.15.1", true})
	images = append(images, image{"giantswarm/tiny-tools:3.9", true})
	images = append(images, image{"gcr.io/google_containers/k8s-dns-kube-dns-amd64:1.14.7", true})
	images = append(images, image{"gcr.io/google_containers/k8s-dns-dnsmasq-nanny-amd64:1.14.7", true})
	images = append(images, image{"gcr.io/google_containers/k8s-dns-sidecar-amd64:1.14.7", true})
	images = append(images, image{"traefik:v2.0.0", true})

	for _, image := range images {
		if image.pull {
			cmd := exec.Command("docker", "pull", image.name)
			cmd.Env = os.Environ()

			output, err := cmd.CombinedOutput()
			fmt.Println(string(output))

			if err != nil {
				fmt.Printf("unable to pull docker image: %v", err)
			}
		}
	}

	check.TestingT(t)
}

type image struct {
	name string
	pull bool
}

type BaseSuite struct {
	dir            string
	kubeConfigPath string
	try            *try.Try
	client         *k8s.ClientWrapper
}

func (s *BaseSuite) startk3s(c *check.C) {
	c.Log("Starting k3s...")
	// Set the base directory for the test suite
	var err error
	s.dir, err = os.Getwd()
	c.Assert(err, checker.IsNil)

	// Create a k3s cluster.
	cmd := exec.Command("k3d", "create", "--name", k3dClusterName,
		"--api-port", "8443",
		"--workers", "1",
		"--server-arg", "--no-deploy=traefik",
		"--server-arg", "--no-deploy=coredns",
	)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	c.Assert(err, checker.IsNil)

	// Load images into k3s
	c.Log("Importing docker images in to k3s...")

	err = s.loadK3sImages()
	c.Assert(err, checker.IsNil)

	// Get kubeconfig path.
	cmd = exec.Command("k3d", "get-kubeconfig", "--name", k3dClusterName)
	cmd.Env = os.Environ()

	output, err = cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)

	s.kubeConfigPath = strings.TrimSuffix(string(output), "\n")

	s.client, err = s.try.WaitClientCreated(masterURL, s.kubeConfigPath, 30*time.Second)
	c.Assert(err, checker.IsNil)

	s.try = try.NewTry(s.client)

	c.Log("k3s start successfully.")
}

func (s *BaseSuite) loadK3sImages() error {
	for _, image := range images {
		err := loadK3sImage(k3dClusterName, image.name, 1*time.Minute)
		if err != nil {
			return err
		}
	}

	return nil
}

func loadK3sImage(clusterName, imageName string, timeout time.Duration) error {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = timeout

	return backoff.Retry(safe.OperationWithRecover(func() error {
		cmd := exec.Command("k3d", "import-images", "--name", clusterName, imageName)
		cmd.Env = os.Environ()

		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Println(string(output))

			logCmd := exec.Command("docker", "events", "--since", "5m")
			logCmd.Env = os.Environ()
			logOutput, _ := cmd.CombinedOutput()
			fmt.Println(string(logOutput))
		}
		return err
	}), ebo)
}

func (s *BaseSuite) stopK3s() {
	// delete the k3s cluster.
	cmd := exec.Command("k3d", "delete", "--name", k3dClusterName)
	cmd.Env = os.Environ()

	output, _ := cmd.CombinedOutput()

	fmt.Println(string(output))
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

func (s *BaseSuite) startAndWaitForKubeDNS(c *check.C) {
	cmd := exec.Command("kubectl", "apply", "-f", path.Join(s.dir, "resources/kubedns"))
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	c.Assert(err, checker.IsNil)
	err = s.try.WaitReadyDeployment("kube-dns", metav1.NamespaceSystem, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitForMaeshControllerStarted(c *check.C) {
	err := s.try.WaitReadyDeployment("maesh-controller", "maesh", 30*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitForMaeshControllerStartedWithReturn() error {
	return s.try.WaitReadyDeployment("maesh-controller", "maesh", 30*time.Second)
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

func (s *BaseSuite) installHelmMaesh(_ *check.C, smi bool, kubeDNS bool) error {
	// Install the helm chart.
	argSlice := []string{"install", "../helm/chart/maesh", "--values", "resources/values.yaml", "--name", "powpow", "--namespace", "maesh"}

	if smi {
		argSlice = append(argSlice, "--set", "smi=true")
	}

	if kubeDNS {
		argSlice = append(argSlice, "--set", "kubedns=true")
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
