package integration

import (
	"time"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
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

	s.deleteResources(c, "resources/smi/access-control/", true)
}
