package integration

import (
	"fmt"
	"os"

	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SMISuite
type SMISuite struct{ BaseSuite }

func (s *SMISuite) SetUpSuite(c *check.C) {
	requiredImages := []string{
		"containous/maesh:latest",
		"containous/whoami:v1.0.1",
		"coredns/coredns:1.3.1",
		"traefik:v2.1.1",
	}
	s.startk3s(c, requiredImages)
	s.startAndWaitForCoreDNS(c)
	s.createResources(c, "resources/tcp-state-table/")
	s.createResources(c, "resources/smi/crds/")
}

func (s *SMISuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *SMISuite) TestSMIAccessControl(c *check.C) {
	s.createResources(c, "resources/smi/access-control/")
	defer s.deleteResources(c, "resources/smi/access-control/", true)

	cmd := s.startMaeshBinaryCmd(c, true)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	config := s.testConfigurationWithReturn(c, "resources/smi/access-control.json")

	s.checkWhitelistSourceRanges(c, config)
	s.checkHTTPServiceServerURLs(c, config)
	s.checkTCPServiceServerURLs(c, config)
}

func (s *SMISuite) TestSMIAccessControlPrepareFail(c *check.C) {
	s.createResources(c, "resources/smi/access-control-broken/")
	defer s.deleteResources(c, "resources/smi/access-control-broken/", false)

	args := []string{"--smi"}
	cmd := s.maeshPrepareWithArgs(args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()

	c.Log(string(output))
	c.Assert(err, checker.NotNil)
}

func (s *SMISuite) TestSMITrafficSplit(c *check.C) {
	s.createResources(c, "resources/smi/traffic-split/")
	defer s.deleteResources(c, "resources/smi/traffic-split/", true)

	cmd := s.startMaeshBinaryCmd(c, true)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	s.testConfiguration(c, "resources/smi/traffic-split.json")
}

func (s *SMISuite) checkWhitelistSourceRanges(c *check.C, config *dynamic.Configuration) {
	for name, middleware := range config.HTTP.Middlewares {
		// Test for block-all-middleware.
		if name == "smi-block-all-middleware" {
			c.Assert(middleware.IPWhiteList.SourceRange[0], checker.Equals, "255.255.255.255")
			c.Log("Middleware " + name + " has the correct source range.")

			continue
		}

		source := string(name[0])
		expected := []string{}

		podList, err := s.client.ListPodWithOptions(testNamespace, metav1.ListOptions{})
		c.Assert(err, checker.IsNil)

		for _, pod := range podList.Items {
			if pod.Spec.ServiceAccountName == source {
				expected = append(expected, pod.Status.PodIP)
			}
		}

		actual := middleware.IPWhiteList.SourceRange
		// Assert that the sourceRange is the correct length.
		c.Assert(len(actual), checker.Equals, len(expected), check.Commentf("Expected length %d, got %d for middleware %s in config: %v", len(expected), len(actual), name, config))
		c.Log("Middleware " + name + " has the correct length.")

		// Assert that the sourceRange contains the expected values.
		for _, expectedValue := range expected {
			c.Assert(contains(actual, expectedValue), checker.True)
		}

		c.Log("Middleware " + name + " has the correct expected values.")
	}
}

func (s *SMISuite) checkHTTPServiceServerURLs(c *check.C, config *dynamic.Configuration) {
	for name, service := range config.HTTP.Services {
		// Test for readiness.
		if name == "readiness" {
			c.Assert(service.LoadBalancer.Servers[0].URL, checker.Equals, "http://127.0.0.1:8080")
			c.Log("service " + name + " has the correct url.")

			continue
		}

		serviceName := string(name[0])

		endpoints, err := s.client.KubeClient.CoreV1().Endpoints(testNamespace).Get(serviceName, metav1.GetOptions{})
		c.Assert(err, checker.IsNil)

		for _, subset := range endpoints.Subsets {
			for _, address := range subset.Addresses {
				for _, port := range subset.Ports {
					actual := fmt.Sprintf("http://%s:%d", address.IP, port.Port)

					// Check if the actual URL is found in the service.
					found := false

					for _, server := range service.LoadBalancer.Servers {
						if actual == server.URL {
							found = true
						}
					}

					// We should have found a match.
					c.Assert(found, checker.True)
				}
			}
		}

		c.Log("Service " + name + " has the correct expected values.")
	}
}

func (s *SMISuite) checkTCPServiceServerURLs(c *check.C, config *dynamic.Configuration) {
	for name, service := range config.TCP.Services {
		serviceName := "tcp"

		endpoints, err := s.client.KubeClient.CoreV1().Endpoints(testNamespace).Get(serviceName, metav1.GetOptions{})
		c.Assert(err, checker.IsNil)

		for _, subset := range endpoints.Subsets {
			for _, address := range subset.Addresses {
				for _, port := range subset.Ports {
					actual := fmt.Sprintf("%s:%d", address.IP, port.Port)

					// Check if the actual URL is found in the service.
					found := false

					for _, server := range service.LoadBalancer.Servers {
						if actual == server.Address {
							found = true
						}
					}

					// We should have found a match.
					c.Assert(found, checker.True)
				}
			}
		}

		c.Log("Service " + name + " has the correct expected values.")
	}
}
