package integration

import (
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// HelmSuite
type HelmSuite struct{ BaseSuite }

func (s *HelmSuite) SetUpSuite(c *check.C) {
	requiredImages := []string{
		"containous/maesh:latest",
		"coredns/coredns:1.6.3",
		"traefik:v2.1.6",
	}
	s.startk3s(c, requiredImages)
	s.startAndWaitForCoreDNS(c)
	s.startWhoami(c)
}

func (s *HelmSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *HelmSuite) TestKubernetesInstall(c *check.C) {
	err := s.installHelmMaesh(c, false, false)
	c.Assert(err, checker.IsNil)

	defer s.unInstallHelmMaesh(c)

	s.waitForMaeshControllerStarted(c)
}

func (s *HelmSuite) TestSMIInstall(c *check.C) {
	s.createResources(c, "resources/smi/crds/")
	defer s.deleteResources(c, "resources/smi/crds/", true)

	err := s.installHelmMaesh(c, true, false)
	c.Assert(err, checker.IsNil)

	defer s.unInstallHelmMaesh(c)

	s.waitForMaeshControllerStarted(c)
}
