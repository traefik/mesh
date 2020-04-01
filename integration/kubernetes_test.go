package integration

import (
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// KubernetesSuite
type KubernetesSuite struct{ BaseSuite }

func (s *KubernetesSuite) SetUpSuite(c *check.C) {
	requiredImages := []string{
		"containous/maesh:latest",
		"containous/whoami:v1.0.1",
		"coredns/coredns:1.6.3",
		"traefik:v2.2.0",
	}
	s.startk3s(c, requiredImages)
	s.startAndWaitForCoreDNS(c)
	s.startWhoami(c)
	s.createResources(c, "resources/tcp-state-table/")
}

func (s *KubernetesSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *KubernetesSuite) TestProviderConfig(c *check.C) {
	cmd := s.startMaeshBinaryCmd(c, false)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	s.testConfiguration(c, "resources/kubernetes/config.json")
}
