package integration

import (
	"os"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// CurlMaeshSuite
type CurlMaeshSuite struct{ BaseSuite }

func (s *CurlMaeshSuite) SetUpSuite(c *check.C) {
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

func (s *CurlMaeshSuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
}

func (s *CurlMaeshSuite) TestSimpleCURL(c *check.C) {
	// Get the tools pod service in whoami namespace
	pod := s.getToolsPodMaesh(c)
	c.Assert(pod, checker.NotNil)

	argSlice := []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "--", "curl", "whoami.whoami.maesh", "--max-time", "5",
	}
	s.waitKubectlExecCommand(c, argSlice, "whoami")

	argSlice = []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "--", "curl", "whoami-http.whoami.maesh", "--max-time", "5",
	}
	s.waitKubectlExecCommand(c, argSlice, "whoami-http")
}
