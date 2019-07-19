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
	err := s.startk3s(c, true)
	c.Assert(err, checker.IsNil)
	s.waitForCoreDNSStarted(c)
	c.Assert(os.Setenv("KUBECONFIG", kubeConfigPath), checker.IsNil)
	s.installTiller(c)
	err = s.installHelmI3o(c, true)
	c.Assert(err, checker.IsNil)
	s.waitForI3oControllerStarted(c)

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

	// Create the required objects from the smi directory
	cmd := exec.Command("kubectl", "apply",
		"-f", path.Join(s.dir, "resources/smi"))
	cmd.Env = os.Environ()
	_, err := cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)

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
			expected:    200, //Returns 200
		},
		{
			desc:        "Pod C -> Service B.mesh /test returns 401",
			source:      "c-tools",
			destination: "b.default.traefik.mesh",
			path:        "/test",
			expected:    401, // Returns 404
		},
		{
			desc:        "Pod C -> Service B.mesh /foo returns 200",
			source:      "c-tools",
			destination: "b.default.traefik.mesh",
			path:        "/foo",
			expected:    200, // Returns 403
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
			destination: "b.default.traefik.mesh",
			path:        "/test",
			expected:    401,
		},
		{
			desc:        "Pod A -> Service B.mesh /foo returns 200",
			source:      "a-tools",
			destination: "b.default.traefik.mesh",
			path:        "/foo",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service D.mesh /bar returns 401",
			source:      "a-tools",
			destination: "d.default.traefik.mesh",
			path:        "/bar",
			expected:    401,
		},
		{
			desc:        "Pod C -> Service D /test returns 200",
			source:      "c-tools",
			destination: "d.default",
			path:        "/test",
			expected:    200, // Returns 200
		},
		{
			desc:        "Pod C -> Service D.mesh /test returns 401",
			source:      "c-tools",
			destination: "d.default.traefik.mesh",
			path:        "/test",
			expected:    401, // Returns 404
		},
		{
			desc:        "Pod C -> Service D.mesh /bar returns 200",
			source:      "c-tools",
			destination: "d.default.traefik.mesh",
			path:        "/bar",
			expected:    200, // Returns 403
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
			expected:    200, // Returns 200
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
			destination: "e.default.traefik.mesh",
			path:        "/test",
			expected:    401,
		},
		{
			desc:        "Pod B -> Service E.mesh /test returns 401",
			source:      "b-tools",
			destination: "e.default.traefik.mesh",
			path:        "/test",
			expected:    401,
		},
		{
			desc:        "Pod C -> Service E.mesh /test returns 401",
			source:      "c-tools",
			destination: "e.default.traefik.mesh",
			path:        "/test",
			expected:    401, // Returns 404
		},
		{
			desc:        "Pod D -> Service E.mesh /test returns 401",
			source:      "d-tools",
			destination: "e.default.traefik.mesh",
			path:        "/test",
			expected:    401,
		},
	}

	for _, test := range testCases {
		argSlice := []string{
			"exec", "-it", test.source, "--", "curl", "-v", test.destination + test.path,
		}
		s.waitKubectlExecCommand(c, argSlice, fmt.Sprintf("HTTP/1.1 %d", test.expected))

	}

}
