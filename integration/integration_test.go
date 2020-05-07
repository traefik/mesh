package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/containous/maesh/integration/try"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/go-check/check"
	"github.com/pmezard/go-difflib/difflib"
	access "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha1"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	checker "github.com/vdemeester/shakers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	integration           = flag.Bool("integration", false, "run integration tests")
	masterURL             = "https://localhost:8443"
	images                []image
	k3dClusterName        = "maesh-integration"
	k3sImage              = "rancher/k3s"
	k3sVersion            = "v0.10.1"
	maeshNamespace        = "maesh"
	maeshBinary           = "../dist/maesh"
	maeshAPIPort          = 9000
	testNamespace         = "test"
	kubectlCreateWaitTime = 1 * time.Second
)

func Test(t *testing.T) {
	if !*integration {
		log.Println("Integration tests disabled")
		return
	}

	check.Suite(&ACLDisabledSuite{})
	check.Suite(&ACLEnabledSuite{})
	check.Suite(&CoreDNSSuite{})
	check.Suite(&KubeDNSSuite{})
	check.Suite(&HelmSuite{})

	images = append(images, image{"containous/maesh:latest", false})
	images = append(images, image{"containous/whoami:v1.0.1", true})
	images = append(images, image{"containous/whoamitcp:v0.0.2", true})
	images = append(images, image{"containous/whoamiudp:v0.0.1", true})
	images = append(images, image{"coredns/coredns:1.2.6", true})
	images = append(images, image{"coredns/coredns:1.3.1", true})
	images = append(images, image{"coredns/coredns:1.4.0", true})
	images = append(images, image{"coredns/coredns:1.5.2", true})
	images = append(images, image{"coredns/coredns:1.6.3", true})
	images = append(images, image{"giantswarm/tiny-tools:3.9", true})
	images = append(images, image{"gcr.io/google_containers/k8s-dns-kube-dns-amd64:1.14.7", true})
	images = append(images, image{"gcr.io/google_containers/k8s-dns-dnsmasq-nanny-amd64:1.14.7", true})
	images = append(images, image{"gcr.io/google_containers/k8s-dns-sidecar-amd64:1.14.7", true})

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
	client         k8s.Client
}

func (s *BaseSuite) maeshStartControllerWithArgsCmd(args ...string) *exec.Cmd {
	controllerArgSlice := []string{
		fmt.Sprintf("--masterurl=%s", masterURL),
		fmt.Sprintf("--kubeconfig=%s", os.Getenv("KUBECONFIG")),
		"--logLevel=debug",
		fmt.Sprintf("--namespace=%s", maeshNamespace),
		"--apihost=127.0.0.1",
	}
	args = append(controllerArgSlice, args...)

	return exec.Command(maeshBinary, args...)
}

func (s *BaseSuite) maeshPrepareWithArgs(args ...string) *exec.Cmd {
	prepareArgSlice := []string{"prepare", fmt.Sprintf("--masterurl=%s", masterURL), fmt.Sprintf("--kubeconfig=%s", os.Getenv("KUBECONFIG")), "--loglevel=debug", "--clusterdomain=cluster.local", fmt.Sprintf("--namespace=%s", maeshNamespace)}
	args = append(prepareArgSlice, args...)

	return exec.Command(maeshBinary, args...)
}

func (s *BaseSuite) startMaeshBinaryCmd(c *check.C, smi bool, acl bool) *exec.Cmd {
	args := []string{}
	if smi {
		args = append(args, "--smi")
	}

	cmd := s.maeshPrepareWithArgs(args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	c.Log(string(output))
	c.Assert(err, checker.IsNil)

	// Ignore the kube-system namespace since we don't care about system events.
	args = append(args, "--ignoreNamespaces=kube-system")
	if acl {
		args = append(args, "--acl")
	}

	return s.maeshStartControllerWithArgsCmd(args...)
}

func (s *BaseSuite) stopMaeshBinary(c *check.C, process *os.Process) {
	err := process.Kill()
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) startk3s(c *check.C, requiredImages []string) {
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
		"--image", fmt.Sprintf("%s:%s", k3sImage, k3sVersion),
		"--wait", "30",
	)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	c.Assert(err, checker.IsNil)

	// Load images into k3s
	c.Log("Importing docker images in to k3s...")

	err = s.loadK3sImages(c, requiredImages)
	c.Assert(err, checker.IsNil)

	s.createK8sClient(c)
	s.createRequiredNamespaces(c)
	c.Log("k3s start successfully.")
}

func (s *BaseSuite) createK8sClient(c *check.C) {
	// Get kubeconfig path.
	cmd := exec.Command("k3d", "get-kubeconfig", "--name", k3dClusterName)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)

	s.kubeConfigPath = strings.TrimSuffix(string(output), "\n")

	c.Log("Creating kube client...")

	s.client, err = s.try.WaitClientCreated(masterURL, s.kubeConfigPath, 30*time.Second)
	c.Assert(err, checker.IsNil)

	s.try = try.NewTry(s.client)

	c.Log("Setting new kubeconfig path...")
	c.Assert(os.Setenv("KUBECONFIG", s.kubeConfigPath), checker.IsNil)
}

func (s *BaseSuite) loadK3sImages(c *check.C, requiredImages []string) error {
	for _, image := range requiredImages {
		c.Log("Importing image: " + image)

		err := loadK3sImage(k3dClusterName, image, 1*time.Minute)
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

func (s *BaseSuite) kubectlCommand(c *check.C, args ...string) {
	args = append(args, fmt.Sprintf("--kubeconfig=%s", os.Getenv("KUBECONFIG")))
	cmd := exec.Command("kubectl", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	c.Log(string(output))
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) createResources(c *check.C, dirPath string) {
	// Create the required objects from the configured directory
	s.kubectlCommand(c, "apply", "-f", path.Join(s.dir, dirPath))
	time.Sleep(kubectlCreateWaitTime)
}

func (s *BaseSuite) deleteResources(c *check.C, dirPath string) {
	// Delete the required objects from the configured directory
	args := []string{"delete", "-f", path.Join(s.dir, dirPath)}
	args = append(args, "--force", "--grace-period=0")

	s.kubectlCommand(c, args...)
}

func (s *BaseSuite) deleteShadowServices(c *check.C) {
	opts := metav1.ListOptions{
		LabelSelector: "app=maesh",
	}
	svcs, err := s.client.GetKubernetesClient().CoreV1().Services(maeshNamespace).List(opts)
	c.Assert(err, checker.IsNil)

	for _, svc := range svcs.Items {
		c.Logf("Deleting shadow service %s.", svc.Name)
		err = s.client.GetKubernetesClient().CoreV1().Services(maeshNamespace).Delete(svc.Name, &metav1.DeleteOptions{})
		c.Assert(err, checker.IsNil)
	}
}

func (s *BaseSuite) waitForPods(c *check.C, pods []string) {
	for _, pod := range pods {
		c.Logf("Waiting for pod: %q to have IP assigned.", pod)
		err := s.try.WaitPodIPAssigned(pod, testNamespace, 30*time.Second)
		c.Assert(err, checker.IsNil)
	}
}

func (s *BaseSuite) startAndWaitForCoreDNS(c *check.C) {
	s.createResources(c, "testdata/coredns/coredns.yaml")
	s.WaitForCoreDNS(c)
}

func (s *BaseSuite) WaitForCoreDNS(c *check.C) {
	c.Assert(s.try.WaitReadyDeployment("coredns", metav1.NamespaceSystem, 60*time.Second), checker.IsNil)
}

func (s *BaseSuite) startAndWaitForKubeDNS(c *check.C) {
	s.createResources(c, "testdata/kubedns")
	c.Assert(s.try.WaitReadyDeployment("kube-dns", metav1.NamespaceSystem, 60*time.Second), checker.IsNil)
}

func (s *BaseSuite) waitForMaeshControllerStarted(c *check.C) {
	c.Assert(s.try.WaitReadyDeployment("maesh-controller", maeshNamespace, 30*time.Second), checker.IsNil)
}

func (s *BaseSuite) waitForMaeshProxyStarted(c *check.C) {
	c.Assert(s.try.WaitReadyDaemonset("maesh-mesh", maeshNamespace, 30*time.Second), checker.IsNil)
}

func (s *BaseSuite) waitForTools(c *check.C) {
	c.Assert(s.try.WaitReadyDeployment("tiny-tools", testNamespace, 30*time.Second), checker.IsNil)
}

func (s *BaseSuite) waitKubectlExecCommand(c *check.C, argSlice []string, data string) {
	c.Assert(s.try.WaitCommandExecute("kubectl", argSlice, data, 10*time.Second), checker.IsNil)
}

func (s *BaseSuite) waitKubectlExecCommandReturn(_ *check.C, argSlice []string) (string, error) {
	return s.try.WaitCommandExecuteReturn("kubectl", argSlice, 10*time.Second)
}

func (s *BaseSuite) startWhoami(c *check.C) {
	s.createResources(c, "testdata/whoami")
	c.Assert(s.try.WaitReadyDeployment("whoami", "whoami", 30*time.Second), checker.IsNil)
}

func (s *BaseSuite) createRequiredNamespaces(c *check.C) {
	c.Log("Creating required namespaces...")
	// Create maesh namespace, required by helm v3.
	s.kubectlCommand(c, "create", "namespace", maeshNamespace)

	// Create test namespace, for testing objects.
	s.kubectlCommand(c, "create", "namespace", testNamespace)
}

func (s *BaseSuite) installHelmMaesh(c *check.C, acl bool, kubeDNS bool) error {
	c.Log("Installing Maesh via helm...")
	// Install the helm chart.
	argSlice := []string{"install", "powpow", "../helm/chart/maesh", "--values", "testdata/values.yaml", "--namespace", maeshNamespace}

	if kubeDNS {
		argSlice = append(argSlice, "--set", "kubedns=true")
	}

	if acl {
		argSlice = append(argSlice, "--set", "acl=true")
	}

	return s.try.WaitCommandExecute("helm", argSlice, "powpow", 10*time.Second)
}

func (s *BaseSuite) unInstallHelmMaesh(c *check.C) {
	c.Log("Uninstalling Maesh via helm...")
	// Install the helm chart.
	argSlice := []string{"uninstall", "powpow", "--namespace", maeshNamespace}
	err := s.try.WaitCommandExecute("helm", argSlice, "uninstalled", 10*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) setCoreDNSVersion(c *check.C, version string) {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = 60 * time.Second

	err := backoff.Retry(safe.OperationWithRecover(func() error {
		// Get current coreDNS deployment.
		deployment, err := s.client.GetKubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get("coredns", metav1.GetOptions{})
		c.Assert(err, checker.IsNil)

		newDeployment := deployment.DeepCopy()
		c.Assert(len(newDeployment.Spec.Template.Spec.Containers), checker.Equals, 1)

		newDeployment.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("coredns/coredns:%s", version)

		return s.try.WaitUpdateDeployment(newDeployment, 10*time.Second)
	}), ebo)

	c.Assert(err, checker.IsNil)

	s.WaitForCoreDNS(c)
}

func (s *BaseSuite) installTinyToolsMaesh(c *check.C) {
	// Create new tiny tools deployment.
	s.kubectlCommand(c, "apply", "-f", path.Join(s.dir, "testdata/tools/deployment.yaml"))

	// Wait for tools to be initialized.
	s.waitForTools(c)
}

func (s *BaseSuite) getToolsPodMaesh(c *check.C) *corev1.Pod {
	podList, err := s.client.GetKubernetesClient().CoreV1().Pods(testNamespace).List(metav1.ListOptions{
		LabelSelector: "app=tiny-tools",
	})
	c.Assert(err, checker.IsNil)
	c.Assert(len(podList.Items), checker.Equals, 1)

	return &podList.Items[0]
}

func (s *BaseSuite) testConfigurationWithReturn(c *check.C, path string) *dynamic.Configuration {
	err := try.GetRequest(fmt.Sprintf("http://127.0.0.1:%d/api/configuration/current", maeshAPIPort), 20*time.Second, try.BodyContains(`"service":"readiness"`))
	c.Assert(err, checker.IsNil)

	expectedJSON := filepath.FromSlash(path)

	var buf bytes.Buffer

	err = try.GetRequest(fmt.Sprintf("http://127.0.0.1:%d/api/configuration/current", maeshAPIPort), 10*time.Second, try.StatusCodeIs(http.StatusOK), matchesConfig(expectedJSON, &buf))
	if err != nil {
		c.Error(err)
	}

	var result *dynamic.Configuration

	err = json.Unmarshal(buf.Bytes(), &result)
	c.Assert(err, checker.IsNil)

	return result
}

func matchesConfig(wantConfig string, buf *bytes.Buffer) try.ResponseCondition {
	return func(res *http.Response) error {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %s", err)
		}

		if err = res.Body.Close(); err != nil {
			return err
		}

		var obtained dynamic.Configuration

		err = json.Unmarshal(body, &obtained)
		if err != nil {
			return err
		}

		if buf != nil {
			buf.Reset()

			if _, err = io.Copy(buf, bytes.NewReader(body)); err != nil {
				return err
			}
		}

		got, err := json.MarshalIndent(obtained, "", "  ")
		if err != nil {
			return err
		}

		expected, err := ioutil.ReadFile(wantConfig)
		if err != nil {
			return err
		}

		// The pods IPs are dynamic, so we cannot predict them,
		// which is why we have to ignore them in the comparison.
		rxURL := regexp.MustCompile(`"(url|address)":\s+(".*")`)
		sanitizedExpected := rxURL.ReplaceAll(expected, []byte(`"$1": "XXXX"`))
		sanitizedGot := rxURL.ReplaceAll(got, []byte(`"$1": "XXXX"`))

		rxHostRule := regexp.MustCompile("Host\\(\\`(\\d+)\\.(\\d+)\\.(\\d+)\\.(\\d+)\\`\\)")
		sanitizedExpected = rxHostRule.ReplaceAll(sanitizedExpected, []byte("Host(`XXXX`)"))
		sanitizedGot = rxHostRule.ReplaceAll(sanitizedGot, []byte("Host(`XXXX`)"))

		rxServerStatus := regexp.MustCompile(`"http://.*?":\s+(".*")`)
		sanitizedExpected = rxServerStatus.ReplaceAll(sanitizedExpected, []byte(`"http://XXXX": $1`))
		sanitizedGot = rxServerStatus.ReplaceAll(sanitizedGot, []byte(`"http://XXXX": $1`))

		// The tcp entrypoint assignments are dynamic, so we cannot predict them,
		// which is why we have to ignore them in the comparison.
		rxTCPEntrypoints := regexp.MustCompile(`"tcp-1000(\d)"`)
		sanitizedExpected = rxTCPEntrypoints.ReplaceAll(sanitizedExpected, []byte(`"tcp-1000X"`))
		sanitizedGot = rxTCPEntrypoints.ReplaceAll(sanitizedGot, []byte(`"tcp-1000X"`))

		// The source range of whitelist middlewares is dynamic, so we cannot predict them,
		// which is why we have to ignore them in the comparison.
		rxSourceRange := regexp.MustCompile(`"sourceRange":\s*\[(\s*"((\d+)\.(\d+)\.(\d+)\.(\d+))",?)*\s*\]`)
		sanitizedExpected = rxSourceRange.ReplaceAll(sanitizedExpected, []byte(`"sourceRange":["XXXX"]`))
		sanitizedGot = rxSourceRange.ReplaceAll(sanitizedGot, []byte(`"sourceRange":["XXXX"]`))

		if bytes.Equal(sanitizedExpected, sanitizedGot) {
			return nil
		}

		diff := difflib.UnifiedDiff{
			FromFile: "Expected",
			A:        difflib.SplitLines(string(sanitizedExpected)),
			ToFile:   "Got",
			B:        difflib.SplitLines(string(sanitizedGot)),
			Context:  3,
		}

		text, err := difflib.GetUnifiedDiffString(diff)
		if err != nil {
			return err
		}

		return errors.New(text)
	}
}

func (s *BaseSuite) digHost(c *check.C, source, namespace, destination string) {
	// Dig the host, with a short response for the A record
	argSlice := []string{
		"exec", "-i", source, "-n", namespace, "--", "dig", destination, "+short",
	}

	output, err := s.waitKubectlExecCommandReturn(c, argSlice)
	c.Assert(err, checker.IsNil)
	c.Log(fmt.Sprintf("Dig %s: %s", destination, strings.TrimSpace(output)))
	IP := net.ParseIP(strings.TrimSpace(output))
	c.Assert(IP, checker.NotNil)
}

func (s *BaseSuite) getPod(c *check.C, name string) *corev1.Pod {
	pod, err := s.client.GetKubernetesClient().CoreV1().Pods(testNamespace).Get(name, metav1.GetOptions{})
	c.Assert(err, checker.IsNil)

	return pod
}

func (s *BaseSuite) getService(c *check.C, name string) *corev1.Service {
	svc, err := s.client.GetKubernetesClient().CoreV1().Services(testNamespace).Get(name, metav1.GetOptions{})
	c.Assert(err, checker.IsNil)

	return svc
}

func (s *BaseSuite) getTrafficTarget(c *check.C, name string) *access.TrafficTarget {
	tt, err := s.client.GetAccessClient().AccessV1alpha1().TrafficTargets(testNamespace).Get(name, metav1.GetOptions{})
	c.Assert(err, checker.IsNil)

	return tt
}

func (s *BaseSuite) getTrafficSplit(c *check.C, name string) *split.TrafficSplit {
	ts, err := s.client.GetSplitClient().SplitV1alpha2().TrafficSplits(testNamespace).Get(name, metav1.GetOptions{})
	c.Assert(err, checker.IsNil)

	return ts
}

func (s *BaseSuite) checkBlockAllMiddleware(c *check.C, config *dynamic.Configuration) {
	c.Log("Checking if the block-all-middleware blocks everything")

	middleware := config.HTTP.Middlewares["block-all-middleware"]
	c.Assert(middleware, checker.NotNil)

	c.Assert(middleware.IPWhiteList.SourceRange, checker.HasLen, 1)
	c.Assert(middleware.IPWhiteList.SourceRange[0], checker.Equals, "255.255.255.255")
}

func (s *BaseSuite) checkHTTPReadinessService(c *check.C, config *dynamic.Configuration) {
	c.Log("Checking if the readiness service is correctly defined")

	service := config.HTTP.Services["readiness"]
	c.Assert(service, checker.NotNil)

	c.Assert(service.LoadBalancer.Servers, checker.HasLen, 1)
	c.Assert(service.LoadBalancer.Servers[0].URL, checker.Equals, "http://127.0.0.1:8080")
}

func (s *BaseSuite) checkHTTPServiceLoadBalancer(c *check.C, config *dynamic.Configuration, svc *corev1.Service, pods []*corev1.Pod) {
	for _, port := range svc.Spec.Ports {
		svcKey := fmt.Sprintf("%s-%s-%d", svc.Namespace, svc.Name, port.Port)

		service := config.HTTP.Services[svcKey]
		c.Assert(service, checker.NotNil)

		c.Assert(service.LoadBalancer.Servers, checker.HasLen, len(pods))

		for _, pod := range pods {
			wantURL := fmt.Sprintf("http://%s:%d", pod.Status.PodIP, port.TargetPort.IntVal)
			c.Logf("Checking if HTTP service %q loadbalancer contains an URL for pod %q: %s", svcKey, pod.Name, wantURL)

			var found bool

			for _, server := range service.LoadBalancer.Servers {
				if wantURL == server.URL {
					found = true
					break
				}
			}

			c.Assert(found, checker.True)
		}
	}
}

func (s *BaseSuite) checkTCPServiceLoadBalancer(c *check.C, config *dynamic.Configuration, svc *corev1.Service, pods []*corev1.Pod) {
	for _, port := range svc.Spec.Ports {
		svcKey := fmt.Sprintf("%s-%s-%d", svc.Namespace, svc.Name, port.Port)

		service := config.TCP.Services[svcKey]
		c.Assert(service, checker.NotNil)

		c.Assert(service.LoadBalancer.Servers, checker.HasLen, len(pods))

		for _, pod := range pods {
			wantURL := fmt.Sprintf("%s:%d", pod.Status.PodIP, port.TargetPort.IntVal)
			c.Logf("Checking if TCP service %q loadbalancer contains an URL for pod %q: %s", svcKey, pod.Name, wantURL)

			var found bool

			for _, server := range service.LoadBalancer.Servers {
				if wantURL == server.Address {
					found = true
					break
				}
			}

			c.Assert(found, checker.True)
		}
	}
}

func (s *BaseSuite) checkUDPServiceLoadBalancer(c *check.C, config *dynamic.Configuration, svc *corev1.Service, pods []*corev1.Pod) {
	for _, port := range svc.Spec.Ports {
		svcKey := fmt.Sprintf("%s-%s-%d", svc.Namespace, svc.Name, port.Port)

		service := config.UDP.Services[svcKey]
		c.Assert(service, checker.NotNil)

		c.Assert(service.LoadBalancer.Servers, checker.HasLen, len(pods))

		for _, pod := range pods {
			wantURL := fmt.Sprintf("%s:%d", pod.Status.PodIP, port.TargetPort.IntVal)
			c.Logf("Checking if UDP service %q loadbalancer contains an URL for pod %q: %s", svcKey, pod.Name, wantURL)

			var found bool

			for _, server := range service.LoadBalancer.Servers {
				if wantURL == server.Address {
					found = true
					break
				}
			}

			c.Assert(found, checker.True)
		}
	}
}

func (s *BaseSuite) checkTrafficTargetLoadBalancer(c *check.C, config *dynamic.Configuration, tt *access.TrafficTarget, svc *corev1.Service, pods []*corev1.Pod) {
	for _, port := range svc.Spec.Ports {
		svcKey := fmt.Sprintf("%s-%s-%s-%d-traffic-target", svc.Namespace, svc.Name, tt.Name, port.Port)

		service := config.HTTP.Services[svcKey]
		c.Assert(service, checker.NotNil)

		c.Assert(service.LoadBalancer.Servers, checker.HasLen, len(pods))

		for _, pod := range pods {
			wantURL := fmt.Sprintf("http://%s:%d", pod.Status.PodIP, port.TargetPort.IntVal)
			c.Logf("Checking if traffic-target service %q loadbalancer contains an URL for pod %q: %s", svcKey, pod.Name, wantURL)

			var found bool

			for _, server := range service.LoadBalancer.Servers {
				if wantURL == server.URL {
					found = true
					break
				}
			}

			c.Assert(found, checker.True)
		}
	}
}

func (s *BaseSuite) checkTrafficTargetWhitelistDirect(c *check.C, config *dynamic.Configuration, tt *access.TrafficTarget, svc *corev1.Service, pods []*corev1.Pod) {
	middlewareKey := fmt.Sprintf("%s-%s-%s-whitelist-traffic-target-direct", svc.Namespace, svc.Name, tt.Name)

	s.checkWhitelistSourceRange(c, config, middlewareKey, pods)
}

func (s *BaseSuite) checkTrafficTargetWhitelistIndirect(c *check.C, config *dynamic.Configuration, tt *access.TrafficTarget, svc *corev1.Service, pods []*corev1.Pod) {
	middlewareKey := fmt.Sprintf("%s-%s-%s-whitelist-traffic-target-indirect", svc.Namespace, svc.Name, tt.Name)

	s.checkWhitelistSourceRange(c, config, middlewareKey, pods)
}

func (s *BaseSuite) checkTrafficSplitWhitelistDirect(c *check.C, config *dynamic.Configuration, ts *split.TrafficSplit, svc *corev1.Service, pods []*corev1.Pod) {
	middlewareKey := fmt.Sprintf("%s-%s-%s-whitelist-traffic-split-direct", svc.Namespace, svc.Name, ts.Name)

	s.checkWhitelistSourceRange(c, config, middlewareKey, pods)
}

func (s *BaseSuite) checkWhitelistSourceRange(c *check.C, config *dynamic.Configuration, key string, pods []*corev1.Pod) {
	middleware := config.HTTP.Middlewares[key]
	c.Assert(middleware, checker.NotNil)

	c.Assert(middleware.IPWhiteList.SourceRange, checker.HasLen, 1)

	for _, pod := range pods {
		c.Logf("Checking if traffic-target indirect whitelist middleware %q allows pod %q: %s", key, pod.Name, pod.Status.PodIP)

		var found bool

		for _, ip := range middleware.IPWhiteList.SourceRange {
			if pod.Status.PodIP == ip {
				found = true
				break
			}
		}

		c.Assert(found, checker.True)
	}
}
