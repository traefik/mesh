package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// SMISuite
type SMISuite struct{ BaseSuite }

func (s *SMISuite) SetUpSuite(c *check.C) {
	err := s.startk3s(c)
	c.Assert(err, checker.IsNil)
	s.waitForCoreDNSStarted(c)
	c.Assert(os.Setenv("KUBECONFIG", kubeConfigPath), checker.IsNil)
	s.installTiller(c)
	err = s.installHelmMaesh(c, true)
	c.Assert(err, checker.IsNil)
	s.waitForMaeshControllerStarted(c)

}

func (s *SMISuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
}

func (s *SMISuite) TestSMIAccessControl(c *check.C) {
	// Get the tools pod service in whoami namespace
	// This test needs to test the following requests result in the following responses:
	// Pod C -> Service B /test returns 200
	// Pod C -> Service B.mesh /test returns 401
	// Pod C -> Service B.mesh /foo returns 200
	// Pod A -> Service B /test returns 200
	// Pod A -> Service B.mesh /test returns 401
	// Pod A -> Service B.mesh /foo returns 200
	// Pod A -> Service D /test returns 200
	// Pod A -> Service D.mesh /bar returns 401
	// Pod C -> Service D /test returns 200
	// Pod C -> Service D.mesh /test returns 401
	// Pod C -> Service D.mesh /bar returns 200
	// Pod A -> Service E /test returns 200
	// Pod B -> Service E /test returns 200
	// Pod C -> Service E /test returns 200
	// Pod D -> Service E /test returns 200
	// Pod A -> Service E.mesh /test returns 401
	// Pod B -> Service E.mesh /test returns 401
	// Pod C -> Service E.mesh /test returns 401
	// Pod D -> Service E.mesh /test returns 401

	s.createResources(c, "resources/smi")
	s.createResources(c, "resources/smi/access-control")

	time.Sleep(10 * time.Second)

	testCases := []struct {
		desc        string
		source      string
		destination string
		path        string
		expected    int
	}{
		{
			desc:        "Pod C -> Service B /test returns 200",
			source:      "c-tools",
			destination: "b.default",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod C -> Service B.mesh /test returns 401",
			source:      "c-tools",
			destination: "b.default.maesh",
			path:        "/test",
			expected:    401,
		},
		{
			desc:        "Pod C -> Service B.mesh /foo returns 200",
			source:      "c-tools",
			destination: "b.default.maesh",
			path:        "/foo",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service B /test returns 200",
			source:      "a-tools",
			destination: "b.default",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service B.mesh /test returns 401",
			source:      "a-tools",
			destination: "b.default.maesh",
			path:        "/test",
			expected:    401,
		},
		{
			desc:        "Pod A -> Service B.mesh /foo returns 200",
			source:      "a-tools",
			destination: "b.default.maesh",
			path:        "/foo",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service D.mesh /bar returns 401",
			source:      "a-tools",
			destination: "d.default.maesh",
			path:        "/bar",
			expected:    401,
		},
		{
			desc:        "Pod C -> Service D /test returns 200",
			source:      "c-tools",
			destination: "d.default",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod C -> Service D.mesh /test returns 401",
			source:      "c-tools",
			destination: "d.default.maesh",
			path:        "/test",
			expected:    401,
		},
		{
			desc:        "Pod C -> Service D.mesh /bar returns 200",
			source:      "c-tools",
			destination: "d.default.maesh",
			path:        "/bar",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service E /test returns 200",
			source:      "a-tools",
			destination: "e.default",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod B -> Service E /test returns 200",
			source:      "b-tools",
			destination: "e.default",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod C -> Service E /test returns 200",
			source:      "c-tools",
			destination: "e.default",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod D -> Service E /test returns 200",
			source:      "d-tools",
			destination: "e.default",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service E.mesh /test returns 401",
			source:      "a-tools",
			destination: "e.default.maesh",
			path:        "/test",
			expected:    401,
		},
		{
			desc:        "Pod B -> Service E.mesh /test returns 401",
			source:      "b-tools",
			destination: "e.default.maesh",
			path:        "/test",
			expected:    401,
		},
		{
			desc:        "Pod C -> Service E.mesh /test returns 401",
			source:      "c-tools",
			destination: "e.default.maesh",
			path:        "/test",
			expected:    401,
		},
		{
			desc:        "Pod D -> Service E.mesh /test returns 401",
			source:      "d-tools",
			destination: "e.default.maesh",
			path:        "/test",
			expected:    401,
		},
	}

	for _, test := range testCases {
		argSlice := []string{
			"exec", "-it", test.source, "--", "curl", "-v", test.destination + test.path, "--max-time", "5",
		}
		s.waitKubectlExecCommand(c, argSlice, fmt.Sprintf("HTTP/1.1 %d", test.expected))
	}

	s.deleteResources(c, "resources/smi/access-control")
}

func (s *SMISuite) TestSMITrafficSplit(c *check.C) {
	s.createResources(c, "resources/smi")
	s.createResources(c, "resources/smi/traffic-split")

	time.Sleep(10 * time.Second)

	testCases := []struct {
		desc        string
		source      string
		destination string
		expected    string
	}{
		{
			desc:        "Pod A -> Service B /test returns 200",
			source:      "a-tools",
			destination: "b.default/test",
			expected:    "HTTP/1.1 200",
		},
		{
			desc:        "Pod A -> Service B /foo returns 200",
			source:      "a-tools",
			destination: "b.default.maesh/foo",
			expected:    "Hostname: b",
		},
		{
			desc:        "Pod A -> Service B v1/foo returns 200",
			source:      "a-tools",
			destination: "b-v1.default.maesh/foo",
			expected:    "Hostname: b-v1",
		},
		{
			desc:        "Pod A -> Service B v2/foo returns 200",
			source:      "a-tools",
			destination: "b-v2.default.maesh/foo",
			expected:    "Hostname: b-v2",
		},
	}

	for _, test := range testCases {
		argSlice := []string{
			"exec", "-it", test.source, "--", "curl", "-v", test.destination, "--max-time", "5",
		}
		c.Log(test.desc)
		s.waitKubectlExecCommand(c, argSlice, test.expected)
	}

	s.deleteResources(c, "resources/smi/traffic-split")
}

func (s *SMISuite) createResources(c *check.C, dirPath string) {
	// Create the required objects from the smi directory
	cmd := exec.Command("kubectl", "apply",
		"-f", path.Join(s.dir, dirPath))
	cmd.Env = os.Environ()
	_, err := cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)
}

func (s *SMISuite) deleteResources(c *check.C, dirPath string) {
	// Create the required objects from the smi directory
	cmd := exec.Command("kubectl", "delete",
		"-f", path.Join(s.dir, dirPath))
	cmd.Env = os.Environ()
	_, err := cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)
}
