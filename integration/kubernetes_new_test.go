package integration

import (
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// KubernetesNewSuite
type KubernetesNewSuite struct{ BaseSuite }

func (s *KubernetesNewSuite) SetUpSuite(c *check.C) {
	s.startk3s(c)
	s.startAndWaitForCoreDNS(c)
	s.startWhoami(c)
}

func (s *KubernetesNewSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *KubernetesNewSuite) TestHTTP(c *check.C) {
	cmd := s.startMaeshBinaryCmd(c)
	err := cmd.Start()
	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)
}
