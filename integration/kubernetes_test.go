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
	err = s.installHelmI3o(c, false)
	c.Assert(err, checker.IsNil)
	s.waitForI3oControllerStarted(c)
	s.startWhoami(c)
	s.installTinyToolsI3o(c)
}

func (s *KubernetesSuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
}

func (s *KubernetesSuite) TestHTTPCURL(c *check.C) {
	// Get the tools pod service in whoami namespace
	pod := s.getToolsPodI3o(c)
	c.Assert(pod, checker.NotNil)

	argSlice := []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "--", "curl", "whoami.whoami.i3o", "--max-time", "5",
	}
	s.waitKubectlExecCommand(c, argSlice, "whoami")
}

func (s *KubernetesSuite) TestTCPCURL(c *check.C) {
	// Get the tools pod service in whoami namespace
	pod := s.getToolsPodI3o(c)
	c.Assert(pod, checker.NotNil)

	argSlice := []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "--", "curl", "whoami-tcp.whoami.i3o", "--max-time", "5",
	}
	s.waitKubectlExecCommand(c, argSlice, "whoami-tcp")

}
