package integration

import (
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// HelmSuite.
type HelmSuite struct{ BaseSuite }

func (s *HelmSuite) SetUpSuite(c *check.C) {
	requiredImages := []string{
		"containous/maesh:latest",
		"coredns/coredns:1.6.3",
	}
	s.startk3s(c, requiredImages)
	s.startAndWaitForCoreDNS(c)
	s.startWhoami(c)
	s.createResources(c, "testdata/smi/crds/")
}

func (s *HelmSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *HelmSuite) TestACLDisabled(c *check.C) {
	err := s.installHelmMaesh(c, false, false)
	c.Assert(err, checker.IsNil)

	defer s.uninstallHelmMaesh(c)

	s.waitForMaeshControllerStarted(c)
	s.waitForMaeshProxyStarted(c)
}

func (s *HelmSuite) TestACLEnabled(c *check.C) {
	err := s.installHelmMaesh(c, true, false)
	c.Assert(err, checker.IsNil)

	defer s.uninstallHelmMaesh(c)

	s.waitForMaeshControllerStarted(c)
	s.waitForMaeshProxyStarted(c)
}

func (s *HelmSuite) TestKubeDNSEnabled(c *check.C) {
	err := s.installHelmMaesh(c, false, true)
	c.Assert(err, checker.IsNil)

	defer s.uninstallHelmMaesh(c)

	s.waitForMaeshControllerStarted(c)
	s.waitForMaeshProxyStarted(c)
}
