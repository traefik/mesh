package integration

import (
	"os"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// CoreDNSSuite
type CoreDNSSuite struct{ BaseSuite }

func (s *CoreDNSSuite) SetUpSuite(c *check.C) {
	s.startk3s(c)
	c.Assert(os.Setenv("KUBECONFIG", s.kubeConfigPath), checker.IsNil)
	s.startAndWaitForCoreDNS(c)
	s.startWhoami(c)
	s.installTinyToolsMaesh(c)
	s.installTiller(c)
}

func (s *CoreDNSSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *CoreDNSSuite) TestCoreDNSVersion(c *check.C) {
	testCases := []struct {
		desc          string
		version       string
		expectedError bool
	}{
		{
			desc:          "CoreDNS 1.2.6",
			version:       "1.2.6",
			expectedError: true,
		},
		{
			desc:          "CoreDNS 1.3.1",
			version:       "1.3.1",
			expectedError: false,
		},
		{
			desc:          "CoreDNS 1.4.0",
			version:       "1.4.0",
			expectedError: false,
		},
		{
			desc:          "CoreDNS 1.6.3",
			version:       "1.6.3",
			expectedError: false,
		},
	}

	for _, test := range testCases {
		pod := s.getToolsPodMaesh(c)
		c.Assert(pod, checker.NotNil)

		argSlice := []string{
			"exec", "-it", pod.Name, "-n", pod.Namespace, "-c", pod.Spec.Containers[0].Name, "--", "curl", "whoami.whoami.maesh", "--max-time", "5",
		}

		c.Log(test.desc)
		s.setCoreDNSVersion(c, test.version)
		err := s.installHelmMaesh(c, false, false)

		if test.expectedError {
			err = s.waitForMaeshControllerStartedWithReturn()
			c.Assert(err, checker.NotNil)
		} else {
			c.Assert(err, checker.IsNil)
			s.waitForMaeshControllerStarted(c)
			s.waitKubectlExecCommand(c, argSlice, "whoami")
		}

		s.unInstallHelmMaesh(c)
	}
}
