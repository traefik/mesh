package integration

import (
	"os"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// CoreDNSSuite
type CoreDNSSuite struct{ BaseSuite }

func (s *CoreDNSSuite) SetUpSuite(c *check.C) {
	err := s.startk3s(c, false)
	c.Assert(err, checker.IsNil)
	c.Assert(os.Setenv("KUBECONFIG", kubeConfigPath), checker.IsNil)
	s.startWhoami(c)
	s.installTinyToolsI3o(c)
	s.installTiller(c)
}

func (s *CoreDNSSuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
}

func (s *CoreDNSSuite) TestCoreDNSSuiteVersion(c *check.C) {
	// Get the tools pod service in whoami namespace
	pod := s.getToolsPodI3o(c)
	c.Assert(pod, checker.NotNil)

	argSlice := []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "curl", "whoami.whoami.traefik.mesh",
	}

	// Test on CoreDNS 1.2
	s.installCoreDNS(c, "1.2")
	err := s.installHelmI3o(c)
	c.Assert(err, checker.NotNil)
	s.uninstallCoreDNS(c, "1.2")
	s.uninstallI3o(c)

	// Test on CoreDNS 1.3
	s.installCoreDNS(c, "1.3")
	err = s.installHelmI3o(c)
	c.Assert(err, checker.IsNil)
	s.waitForI3oControllerStarted(c)
	s.waitKubectlExecCommand(c, argSlice, "whoami")
	s.uninstallI3o(c)
	s.uninstallCoreDNS(c, "1.3")

	// Test on CoreDNS 1.4
	s.installCoreDNS(c, "1.4")
	err = s.installHelmI3o(c)
	c.Assert(err, checker.IsNil)
	s.waitForI3oControllerStarted(c)
	s.waitKubectlExecCommand(c, argSlice, "whoami")
	s.uninstallI3o(c)
	s.uninstallCoreDNS(c, "1.4")

	// Test on CoreDNS 1.5
	s.installCoreDNS(c, "1.5")
	err = s.installHelmI3o(c)
	c.Assert(err, checker.IsNil)
	s.waitForI3oControllerStarted(c)
	s.waitKubectlExecCommand(c, argSlice, "whoami")
	s.uninstallI3o(c)
	s.uninstallCoreDNS(c, "1.5")
}
