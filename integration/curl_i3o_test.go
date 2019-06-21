package integration

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/containous/i3o/integration/try"
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
	s.installHelmI3o(c)
	s.waitForI3oControllerStarted(c)
	s.startWhoami(c)
	s.installTinyToolsI3o(c)

	// Check that ingressroutetcps is created for the whoami service
	err = s.try.ListIngressRouteTCPs("whoami", 20*time.Second, try.HasIngressRouteTCPListLength(1), try.HasNamesIngressRouteTCPList(try.List{"whoami-whoami"}))
	c.Assert(err, checker.IsNil)

	// Check that ingressroutes is created for the whoami-http service
	err = s.try.ListIngressRoutes("whoami", 20*time.Second, try.HasIngressRouteListLength(1), try.HasNamesIngressRouteList(try.List{"whoami-whoami-http"}))
	c.Assert(err, checker.IsNil)

}

func (s *CurlI3oSuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
}

func (s *CurlI3oSuite) TestSimpleCURL(c *check.C) {
	// Get the tools pod service in whoami namespace
	pod, err := s.getToolsPodI3o(c)
	c.Assert(err, checker.IsNil)
	c.Assert(pod, checker.NotNil)

	argSlice := []string{
		"exec",
		"-it",
		pod.Name,
		"-n",
		pod.Namespace,
		"-c",
		pod.Spec.Containers[0].Name,
		"curl",
		"whoami.whoami.traefik.mesh",
	}

	cmd := exec.Command("kubectl", argSlice...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	c.Assert(err, checker.IsNil)

}
