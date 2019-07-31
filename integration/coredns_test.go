package integration

import (
	"os"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// CoreDNSSuite
type CoreDNSSuite struct{ BaseSuite }

func (s *CoreDNSSuite) SetUpSuite(c *check.C) {
	err := s.startk3s(c)
	c.Assert(err, checker.IsNil)
	c.Assert(os.Setenv("KUBECONFIG", kubeConfigPath), checker.IsNil)
	s.startWhoami(c)
	s.installTinyToolsMaesh(c)
	s.installTiller(c)
}

func (s *CoreDNSSuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
}

func (s *CoreDNSSuite) TestCoreDNSVersion126Fail(c *check.C) {
	// Test on CoreDNS 1.2
	s.setCoreDNSVersion(c, "1.2.6")
	err := s.installHelmMaesh(c, false)
	c.Assert(err, checker.NotNil)
}

func (s *CoreDNSSuite) TestCoreDNSVersion131(c *check.C) {
	pod := s.getToolsPodMaesh(c)
	c.Assert(pod, checker.NotNil)

	argSlice := []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "--", "curl", "whoami.whoami.maesh", "--max-time", "5",
	}

	// Test on CoreDNS 1.3.1
	s.setCoreDNSVersion(c, "1.3.1")
	err := s.installHelmMaesh(c, false)
	c.Assert(err, checker.IsNil)
	s.waitForMaeshControllerStarted(c)
	s.waitKubectlExecCommand(c, argSlice, "whoami")
	s.unInstallHelmMaesh(c)
}

func (s *CoreDNSSuite) TestCoreDNSVersion140(c *check.C) {
	pod := s.getToolsPodMaesh(c)
	c.Assert(pod, checker.NotNil)

	argSlice := []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "--", "curl", "whoami.whoami.maesh", "--max-time", "5",
	}

	// Test on CoreDNS 1.4.0
	s.setCoreDNSVersion(c, "1.4.0")
	err := s.installHelmMaesh(c, false)
	c.Assert(err, checker.IsNil)
	s.waitForMaeshControllerStarted(c)
	s.waitKubectlExecCommand(c, argSlice, "whoami")
	s.unInstallHelmMaesh(c)
}
