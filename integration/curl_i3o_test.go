package integration

import (
	"os"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// CurlI3oSuite
type CurlI3oSuite struct{ BaseSuite }

func (s *CurlI3oSuite) SetUpSuite(c *check.C) {
	err := s.startk3s(c)
	c.Assert(err, checker.IsNil)
	s.waitForCoreDNSStarted(c)
	c.Assert(os.Setenv("KUBECONFIG", kubeConfigPath), checker.IsNil)
	s.installTiller(c)
	err = s.installHelmI3o(c)
	c.Assert(err, checker.IsNil)
	s.waitForI3oControllerStarted(c)
	s.startWhoami(c)
	s.installTinyToolsI3o(c)
}

func (s *CurlI3oSuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
}

func (s *CurlI3oSuite) TestSimpleCURL(c *check.C) {
	// Get the tools pod service in whoami namespace
	pod := s.getToolsPodI3o(c)
	c.Assert(pod, checker.NotNil)

	argSlice := []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "curl", "whoami.whoami.traefik.mesh",
	}
	s.waitKubectlExecCommand(c, argSlice, "whoami")

	argSlice = []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "curl", "whoami-http.whoami.traefik.mesh",
	}
	s.waitKubectlExecCommand(c, argSlice, "whoami-http")
}
