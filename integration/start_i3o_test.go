package integration

import (
	"os"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// StartI3oSuite
type StartI3oSuite struct{ BaseSuite }

func (s *StartI3oSuite) SetUpSuite(c *check.C) {
	err := s.startk3s(c)
	c.Assert(err, checker.IsNil)
	s.waitForCoreDNSStarted(c)
	c.Assert(os.Setenv("KUBECONFIG", kubeConfigPath), checker.IsNil)
}

func (s *StartI3oSuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
}

func (s *StartI3oSuite) TestSimpleStart(c *check.C) {
	err := s.installHelmI3o(c)
	c.Assert(err, checker.IsNil)
	s.waitForI3oControllerStarted(c)
}
