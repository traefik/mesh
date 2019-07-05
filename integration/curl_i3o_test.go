package integration

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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

}

func (s *CurlI3oSuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
}

func (s *CurlI3oSuite) TestSimpleCURL(c *check.C) {
	// Get the tools pod service in whoami namespace
	pod := s.getToolsPodI3o(c)
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

	stringOutput := string(output)
	fmt.Println(stringOutput)

	if !strings.Contains(stringOutput, "whoami") {
		c.Errorf("Curl response did not contain: whoami, got: %s", stringOutput)
	}
	c.Assert(err, checker.IsNil)

}
