package integration

import (
	"os"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// KubeDNSSuite
type KubeDNSSuite struct{ BaseSuite }

func (s *KubeDNSSuite) SetUpSuite(c *check.C) {
	s.startk3s(c)
	c.Assert(os.Setenv("KUBECONFIG", s.kubeConfigPath), checker.IsNil)
	s.startAndWaitForKubeDNS(c)
	s.startWhoami(c)
	s.installTinyToolsMaesh(c)
	s.installTiller(c)
}

func (s *KubeDNSSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *KubeDNSSuite) TestKubeDNS(c *check.C) {
	pod := s.getToolsPodMaesh(c)
	c.Assert(pod, checker.NotNil)

	argSlice := []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "--", "curl", "whoami.whoami.svc.cluster.local", "--max-time", "5",
	}

	err := s.installHelmMaesh(c, false, true)
	c.Assert(err, checker.IsNil)
	s.waitForMaeshControllerStarted(c)
	s.waitKubectlExecCommand(c, argSlice, "whoami")

	argSlice = []string{
		"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "--", "curl", "whoami.whoami.maesh", "--max-time", "5",
	}
	s.waitKubectlExecCommand(c, argSlice, "whoami")
	s.unInstallHelmMaesh(c)
}
