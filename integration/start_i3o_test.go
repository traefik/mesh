package integration

import (
	"os"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// StartI3oSuite
type StartI3oSuite struct{ BaseSuite }

func (s *StartI3oSuite) SetUpSuite(c *check.C) {
	s.createComposeProject(c, "k3s")
	s.waitForCoreDNS(c)
	err := os.Setenv("KUBECONFIG", kubeConfigPath)
	c.Assert(err, checker.IsNil)
}

func (s *StartI3oSuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
}

func (s *StartI3oSuite) TestSimpleStart(c *check.C) {
	s.startI3o(c)
	c.Assert(os.Getenv("KUBECONFIG"), checker.Equals, kubeConfigPath)
}
