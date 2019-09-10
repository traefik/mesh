package integration

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"time"

	splitv1alpha "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha1"
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SMISuite
type SMISuite struct{ BaseSuite }

func (s *SMISuite) SetUpSuite(c *check.C) {
	err := s.startk3s(c)
	c.Assert(err, checker.IsNil)
	c.Assert(os.Setenv("KUBECONFIG", kubeConfigPath), checker.IsNil)
	s.startAndWaitForCoreDNS(c)
	s.installTiller(c)
}

func (s *SMISuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
}

func (s *SMISuite) TestSMIAccessControl(c *check.C) {
	// Get the tools pod service in whoami namespace
	// This test needs to test the following requests result in the following responses:
	// Pod C -> Service B /test returns 200
	// Pod C -> Service B.maesh /test returns 404
	// Pod C -> Service B.maesh /foo returns 200
	// Pod A -> Service B /test returns 200
	// Pod A -> Service B.maesh /test returns 401
	// Pod A -> Service B.maesh /foo returns 200
	// Pod A -> Service D /test returns 200
	// Pod A -> Service D.maesh /bar returns 403
	// Pod C -> Service D /test returns 200
	// Pod C -> Service D.maesh /test returns 403
	// Pod C -> Service D.maesh /bar returns 200
	// Pod A -> Service E /test returns 200
	// Pod B -> Service E /test returns 200
	// Pod C -> Service E /test returns 200
	// Pod D -> Service E /test returns 200
	// Pod A -> Service E.maesh /test returns 404
	// Pod B -> Service E.maesh /test returns 404
	// Pod C -> Service E.maesh /test returns 404
	// Pod D -> Service E.maesh /test returns 404

	s.createResources(c, "resources/smi")
	s.createResources(c, "resources/smi/access-control/")

	time.Sleep(10 * time.Second)

	err := s.installHelmMaesh(c, true)
	c.Assert(err, checker.IsNil)
	s.waitForMaeshControllerStarted(c)

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
			desc:        "Pod C -> Service B.maesh /test returns 404",
			source:      "c-tools",
			destination: "b.default.maesh",
			path:        "/test",
			expected:    404,
		},
		//{
		//	desc:        "Pod C -> Service B.maesh /foo returns 200",
		//	source:      "c-tools",
		//	destination: "b.default.maesh",
		//	path:        "/foo",
		//	expected:    200,
		//},
		{
			desc:        "Pod A -> Service B /test returns 200",
			source:      "a-tools",
			destination: "b.default",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service B.maesh /test returns 404",
			source:      "a-tools",
			destination: "b.default.maesh",
			path:        "/test",
			expected:    404,
		},
		{
			desc:        "Pod A -> Service B.maesh /foo returns 200",
			source:      "a-tools",
			destination: "b.default.maesh",
			path:        "/foo",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service D.maesh /bar returns 403",
			source:      "a-tools",
			destination: "d.default.maesh",
			path:        "/bar",
			expected:    403,
		},
		{
			desc:        "Pod C -> Service D /test returns 200",
			source:      "c-tools",
			destination: "d.default",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod C -> Service D.maesh /test returns 404",
			source:      "c-tools",
			destination: "d.default.maesh",
			path:        "/test",
			expected:    404,
		},
		{
			desc:        "Pod C -> Service D.maesh /bar returns 200",
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
			desc:        "Pod A -> Service E.maesh /test returns 404",
			source:      "a-tools",
			destination: "e.default.maesh",
			path:        "/test",
			expected:    404,
		},
		{
			desc:        "Pod B -> Service E.maesh /test returns 404",
			source:      "b-tools",
			destination: "e.default.maesh",
			path:        "/test",
			expected:    404,
		},
		{
			desc:        "Pod C -> Service E.maesh /test returns 404",
			source:      "c-tools",
			destination: "e.default.maesh",
			path:        "/test",
			expected:    404,
		},
		{
			desc:        "Pod D -> Service E.maesh /test returns 404",
			source:      "d-tools",
			destination: "e.default.maesh",
			path:        "/test",
			expected:    404,
		},
	}

	for _, test := range testCases {
		argSlice := []string{
			"exec", "-it", test.source, "--", "curl", "-v", test.destination + test.path, "--max-time", "5",
		}
		c.Log(test.desc)
		s.waitKubectlExecCommand(c, argSlice, fmt.Sprintf("HTTP/1.1 %d", test.expected))
	}

	s.unInstallHelmMaesh(c)

	s.deleteResources(c, "resources/smi/access-control/", true)
}

func (s *SMISuite) TestSMITrafficSplit(c *check.C) {
	s.createResources(c, "resources/smi")
	s.createResources(c, "resources/smi/traffic-split")

	time.Sleep(10 * time.Second)

	err := s.installHelmMaesh(c, true)
	c.Assert(err, checker.IsNil)
	s.waitForMaeshControllerStarted(c)

	testCases := []struct {
		desc         string
		source       string
		iteration    int
		trafficSplit *splitv1alpha.TrafficSplit
		destination  string
		expected     map[string]float64
	}{
		{
			desc:        "Pod A -> Service B /test returns 200",
			source:      "a-tools",
			iteration:   1,
			destination: "b-v1.default/test",
			expected: map[string]float64{
				"Hostname: b-v1": 100,
			},
		},
		{
			desc:        "Pod A -> Service B /foo returns 200",
			source:      "a-tools",
			iteration:   1,
			destination: "b-v2.default.maesh/foo",
			expected: map[string]float64{
				"Hostname: b-v2": 100,
			},
		},
		{
			desc:        "Pod A -> Service B v1/foo returns 200",
			source:      "a-tools",
			iteration:   1,
			destination: "b-v1.default.maesh/foo",
			expected: map[string]float64{
				"Hostname: b-v1": 100,
			},
		},
		{
			desc:        "Pod A -> Service B v2/foo returns 200",
			source:      "a-tools",
			iteration:   1,
			destination: "b-v2.default.maesh/foo",
			expected: map[string]float64{
				"Hostname: b-v2": 100,
			},
		},
		{
			desc:      "Pod A -> Service B v2/foo returns 200 50-50",
			source:    "a-tools",
			iteration: 10,
			trafficSplit: &splitv1alpha.TrafficSplit{
				ObjectMeta: metav1.ObjectMeta{
					Name: "canary",
				},
				Spec: splitv1alpha.TrafficSplitSpec{
					Service: "b",
					Backends: []splitv1alpha.TrafficSplitBackend{
						{
							Service: "b-v1",
							Weight:  *resource.NewQuantity(int64(500), resource.DecimalSI),
						},
						{
							Service: "b-v2",
							Weight:  *resource.NewQuantity(int64(500), resource.DecimalSI),
						},
					},
				},
			},
			destination: "b.default.maesh/foo",
			expected: map[string]float64{
				"Hostname: b-v1": 50,
				"Hostname: b-v2": 50,
			},
		},
		{
			desc:      "Pod A -> Service B v2/foo returns 200 0-100",
			source:    "a-tools",
			iteration: 10,
			trafficSplit: &splitv1alpha.TrafficSplit{
				ObjectMeta: metav1.ObjectMeta{
					Name: "canary",
				},
				Spec: splitv1alpha.TrafficSplitSpec{
					Service: "b",
					Backends: []splitv1alpha.TrafficSplitBackend{
						{
							Service: "b-v1",
							Weight:  *resource.NewQuantity(int64(0), resource.DecimalSI),
						},
						{
							Service: "b-v2",
							Weight:  *resource.NewQuantity(int64(1000), resource.DecimalSI),
						},
					},
				},
			},
			destination: "b.default.maesh/foo",
			expected: map[string]float64{
				"Hostname: b-v1": 0,
				"Hostname: b-v2": 100,
			},
		},
		{
			desc:      "Pod A -> Service B v2/foo returns 200 100-0",
			source:    "a-tools",
			iteration: 10,
			trafficSplit: &splitv1alpha.TrafficSplit{
				ObjectMeta: metav1.ObjectMeta{
					Name: "canary",
				},
				Spec: splitv1alpha.TrafficSplitSpec{
					Service: "b",
					Backends: []splitv1alpha.TrafficSplitBackend{
						{
							Service: "b-v1",
							Weight:  *resource.NewQuantity(int64(1000), resource.DecimalSI),
						},
						{
							Service: "b-v2",
							Weight:  *resource.NewQuantity(int64(0), resource.DecimalSI),
						},
					},
				},
			},
			destination: "b.default.maesh/foo",
			expected: map[string]float64{
				"Hostname: b-v1": 100,
				"Hostname: b-v2": 0,
			},
		},
	}

	for _, test := range testCases {
		var trafficSplit *splitv1alpha.TrafficSplit
		if test.trafficSplit != nil {
			trafficSplit, err = s.client.SmiSplitClient.SplitV1alpha1().TrafficSplits("default").Create(test.trafficSplit)
			c.Assert(err, checker.IsNil)

			// FIXME - Service b have to be deleted and created to be able to detect change.
			err = s.client.KubeClient.CoreV1().Services("default").Delete("b", &metav1.DeleteOptions{})
			c.Assert(err, checker.IsNil)
			s.createResources(c, "resources/smi/traffic-split")

			time.Sleep(10 * time.Second)
		}
		argSlice := []string{
			"exec", "-it", test.source, "--", "curl", "-v", test.destination, "--max-time", "5",
		}
		c.Log(test.desc)

		err := s.try.WaitFunction(func() error {
			percentageResult := make(map[string]float64)
			for i := 0; i < test.iteration; i++ {
				data, err := s.waitKubectlExecCommandReturn(c, argSlice)
				if err != nil {
					return err
				}
				percentageResult[s.getLineContent(data)]++
			}

			fmt.Println(percentageResult)
			for key, value := range percentageResult {
				i := (value / float64(test.iteration)) * 100
				if i != test.expected[key] {
					return fmt.Errorf("%f and %f are not equals", i, test.expected[key])
				}
			}

			return nil
		}, 30*time.Second)

		c.Assert(err, check.IsNil)

		if trafficSplit != nil {
			err := s.client.SmiSplitClient.SplitV1alpha1().TrafficSplits("default").Delete(trafficSplit.Name, &metav1.DeleteOptions{})
			c.Assert(err, checker.IsNil)
		}
	}

	s.unInstallHelmMaesh(c)

	s.deleteResources(c, "resources/smi/traffic-split", true)
}

func (s *SMISuite) getLineContent(data string) string {
	scanner := bufio.NewScanner(bytes.NewReader([]byte(data)))

	for scanner.Scan() {
		rgx := regexp.MustCompile("^(?:.+)?(Hostname: (?:a|b)(?:-v[0-9])?)$")
		if m := rgx.FindStringSubmatch(scanner.Text()); m != nil {
			return m[1]
		}
	}

	return ""
}

func (s *SMISuite) createResources(c *check.C, dirPath string) {
	// Create the required objects from the smi directory
	cmd := exec.Command("kubectl", "apply",
		"-f", path.Join(s.dir, dirPath))
	cmd.Env = os.Environ()
	_, err := cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)
}

func (s *SMISuite) deleteResources(c *check.C, dirPath string, force bool) {
	// Create the required objects from the smi directory
	args := []string{"delete", "-f", path.Join(s.dir, dirPath)}
	if force {
		args = append(args, "--force", "--grace-period=0")
	}
	cmd := exec.Command("kubectl", args...)
	cmd.Env = os.Environ()
	_, err := cmd.CombinedOutput()
	c.Assert(err, checker.IsNil)
}
