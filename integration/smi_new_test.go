package integration

import (
	"time"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SMINewSuite
type SMINewSuite struct{ BaseSuite }

func (s *SMINewSuite) SetUpSuite(c *check.C) {
	s.startk3s(c)
	s.startAndWaitForCoreDNS(c)
	s.createResources(c, "resources/smi/crds/", 10*time.Second)
}

func (s *SMINewSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *SMINewSuite) TestSMIAccessControl(c *check.C) {
	s.createResources(c, "resources/smi/access-control/", 10*time.Second)

	cmd := s.startMaeshBinaryCmd(c, true)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	s.testConfiguration(c, "resources/smi/access-control.json")
	s.checkWhitelistSourceranges(c)
	s.checkServiceServerURLs(c)
	s.deleteResources(c, "resources/smi/access-control/", true)
}

func (s *SMINewSuite) checkWhitelistSourceranges(c *check.C) {
	config := s.getActiveConfiguration(c)
	for name, middleware := range config.HTTP.Middlewares {
		// test for block-all-middleware
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
		// Assert that the sourceRange is the correct length
		c.Assert(len(actual), checker.Equals, len(expected))
		c.Log("Middleware " + name + " has the correct length.")

		// Assert that the sourceRange contains the expected Values
		for _, expectedValue := range expected {
			c.Assert(contains(actual, expectedValue), checker.True)
		}

		c.Log("Middleware " + name + " has the correct expected values.")
	}
}

func (s *SMINewSuite) checkServiceServerURLs(c *check.C) {
}
