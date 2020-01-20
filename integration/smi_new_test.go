package integration

import (
	"fmt"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SMINewSuite
type SMINewSuite struct{ BaseSuite }

func (s *SMINewSuite) SetUpSuite(c *check.C) {
	s.startk3s(c)
	s.startAndWaitForCoreDNS(c)
	s.createResources(c, "resources/smi/crds/")
}

func (s *SMINewSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *SMINewSuite) TestSMIAccessControl(c *check.C) {
	s.createResources(c, "resources/smi/access-control/")
	defer s.deleteResources(c, "resources/smi/access-control/", true)

	cmd := s.startMaeshBinaryCmd(c, true)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	s.testConfiguration(c, "resources/smi/access-control.json")
	s.checkWhitelistSourceRanges(c)
	s.checkHTTPServiceServerURLs(c)
	s.checkTCPServiceServerURLs(c)
}

func (s *SMINewSuite) checkWhitelistSourceRanges(c *check.C) {
	config := s.getActiveConfiguration(c)
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
		c.Assert(len(actual), checker.Equals, len(expected))
		c.Log("Middleware " + name + " has the correct length.")

		// Assert that the sourceRange contains the expected values.
		for _, expectedValue := range expected {
			c.Assert(contains(actual, expectedValue), checker.True)
		}

		c.Log("Middleware " + name + " has the correct expected values.")
	}
}

func (s *SMINewSuite) checkHTTPServiceServerURLs(c *check.C) {
	config := s.getActiveConfiguration(c)
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

func (s *SMINewSuite) checkTCPServiceServerURLs(c *check.C) {
	config := s.getActiveConfiguration(c)
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
