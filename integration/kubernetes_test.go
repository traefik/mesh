package integration

import (
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// KubernetesSuite
type KubernetesSuite struct{ BaseSuite }

func (s *KubernetesSuite) SetUpSuite(c *check.C) {
	s.startk3s(c)
	s.startAndWaitForCoreDNS(c)

	err := s.installHelmMaesh(c, false, false)
	c.Assert(err, checker.IsNil)
	s.waitForMaeshControllerStarted(c)
	s.startWhoami(c)
	s.installTinyToolsMaesh(c)
}

func (s *KubernetesSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *KubernetesSuite) TestHTTPCURL(c *check.C) {
	// Get the tools pod service in whoami namespace
	pod := s.getToolsPodMaesh(c)
	c.Assert(pod, checker.NotNil)

	argSlice := []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "--", "curl", "whoami.whoami.maesh", "--max-time", "5",
	}
	s.waitKubectlExecCommand(c, argSlice, "whoami")
}

func (s *KubernetesSuite) TestTCPCURL(c *check.C) {
	// Get the tools pod service in whoami namespace
	pod := s.getToolsPodMaesh(c)
	c.Assert(pod, checker.NotNil)

	argSlice := []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "--", "curl", "whoami-tcp.whoami.maesh", "--max-time", "5",
	}
	s.waitKubectlExecCommand(c, argSlice, "whoami-tcp")
}
