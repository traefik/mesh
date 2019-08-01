package integration

import (
	"os"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// KubernetesSuite
type KubernetesSuite struct{ BaseSuite }

func (s *KubernetesSuite) SetUpSuite(c *check.C) {
	err := s.startk3s(c)
	c.Assert(err, checker.IsNil)
	s.waitForCoreDNSStarted(c)
	c.Assert(os.Setenv("KUBECONFIG", kubeConfigPath), checker.IsNil)
	s.installTiller(c)
	err = s.installHelmMaesh(c, false)
	c.Assert(err, checker.IsNil)
	s.waitForMaeshControllerStarted(c)
	s.startWhoami(c)
	s.installTinyToolsMaesh(c)
}

func (s *KubernetesSuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
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
