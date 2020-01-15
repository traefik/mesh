package integration

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"time"

	split "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha2"
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SMISuite
type SMISuite struct{ BaseSuite }

func (s *SMISuite) SetUpSuite(c *check.C) {
	s.startk3s(c)
	s.startAndWaitForCoreDNS(c)
}

func (s *SMISuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *SMISuite) TestSMIAccessControl(c *check.C) {
	err := s.installHelmMaesh(c, true, false)
	c.Assert(err, checker.IsNil)
	s.waitForMaeshControllerStarted(c)
	s.createResources(c, "resources/smi/access-control/", 10*time.Second)

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
			destination: "b.test.svc.cluster.local",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod C -> Service B.maesh /test returns 404",
			source:      "c-tools",
			destination: "b.test.maesh",
			path:        "/test",
			expected:    404,
		},
		{
			desc:        "Pod C -> Service B.maesh /foo returns 200",
			source:      "c-tools",
			destination: "b.test.maesh",
			path:        "/foo",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service B /test returns 200",
			source:      "a-tools",
			destination: "b.test.svc.cluster.local",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service B.maesh /test returns 404",
			source:      "a-tools",
			destination: "b.test.maesh",
			path:        "/test",
			expected:    404,
		},
		{
			desc:        "Pod A -> Service B.maesh /foo returns 200",
			source:      "a-tools",
			destination: "b.test.maesh",
			path:        "/foo",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service D.maesh /bar returns 403",
			source:      "a-tools",
			destination: "d.test.maesh",
			path:        "/bar",
			expected:    403,
		},
		{
			desc:        "Pod C -> Service D /test returns 200",
			source:      "c-tools",
			destination: "d.test.svc.cluster.local",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod C -> Service D.maesh /test returns 404",
			source:      "c-tools",
			destination: "d.test.maesh",
			path:        "/test",
			expected:    404,
		},
		{
			desc:        "Pod C -> Service D.maesh /bar returns 200",
			source:      "c-tools",
			destination: "d.test.maesh",
			path:        "/bar",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service E /test returns 200",
			source:      "a-tools",
			destination: "e.test.svc.cluster.local",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod B -> Service E /test returns 200",
			source:      "b-tools",
			destination: "e.test.svc.cluster.local",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod C -> Service E /test returns 200",
			source:      "c-tools",
			destination: "e.test.svc.cluster.local",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod D -> Service E /test returns 200",
			source:      "d-tools",
			destination: "e.test.svc.cluster.local",
			path:        "/test",
			expected:    200,
		},
		{
			desc:        "Pod A -> Service E.maesh /test returns 404",
			source:      "a-tools",
			destination: "e.test.maesh",
			path:        "/test",
			expected:    404,
		},
		{
			desc:        "Pod B -> Service E.maesh /test returns 404",
			source:      "b-tools",
			destination: "e.test.maesh",
			path:        "/test",
			expected:    404,
		},
		{
			desc:        "Pod C -> Service E.maesh /test returns 404",
			source:      "c-tools",
			destination: "e.test.maesh",
			path:        "/test",
			expected:    404,
		},
		{
			desc:        "Pod D -> Service E.maesh /test returns 404",
			source:      "d-tools",
			destination: "e.test.maesh",
			path:        "/test",
			expected:    404,
		},
		{
			desc:        "Pod D -> Service TCP.maesh returns something",
			source:      "d-tools",
			destination: "tcp.test.maesh",
			path:        "/",
			expected:    200,
		},
	}

	for _, test := range testCases {
		argSlice := []string{
			"exec", "-i", test.source, "-n", testNamespace, "--", "curl", "-v", test.destination + test.path, "--max-time", "5",
		}

		c.Log(test.desc)
		s.digHost(c, test.source, testNamespace, test.destination)
		s.waitKubectlExecCommand(c, argSlice, fmt.Sprintf("HTTP/1.1 %d", test.expected))
	}

	s.unInstallHelmMaesh(c)

	s.deleteResources(c, "resources/smi/access-control/", true)
}

func (s *SMISuite) TestSMITrafficSplit(c *check.C) {
	s.createResources(c, "resources/smi/traffic-split", 10*time.Second)

	err := s.installHelmMaesh(c, true, false)
	c.Assert(err, checker.IsNil)
	s.waitForMaeshControllerStarted(c)

	testCases := []struct {
		desc            string
		source          string
		iteration       int
		trafficSplit    *split.TrafficSplit
		destinationHost string
		destinationPath string
		expected        map[string]float64
	}{
		{
			desc:            "Pod A -> Service B /test returns 200",
			source:          "a-tools",
			iteration:       1,
			destinationHost: "b-v1.test.svc.cluster.local",
			destinationPath: "/test",
			expected: map[string]float64{
				"Hostname: b-v1": 100,
			},
		},
		{
			desc:            "Pod A -> Service B /foo returns 200",
			source:          "a-tools",
			iteration:       1,
			destinationHost: "b-v2.test.maesh",
			destinationPath: "/foo",
			expected: map[string]float64{
				"Hostname: b-v2": 100,
			},
		},
		{
			desc:            "Pod A -> Service B v1/foo returns 200",
			source:          "a-tools",
			iteration:       1,
			destinationHost: "b-v1.test.maesh",
			destinationPath: "/foo",
			expected: map[string]float64{
				"Hostname: b-v1": 100,
			},
		},
		{
			desc:            "Pod A -> Service B v2/foo returns 200",
			source:          "a-tools",
			iteration:       1,
			destinationHost: "b-v2.test.maesh",
			destinationPath: "/foo",
			expected: map[string]float64{
				"Hostname: b-v2": 100,
			},
		},
		{
			desc:      "Pod A -> Service B v2/foo returns 200 50-50",
			source:    "a-tools",
			iteration: 10,
			trafficSplit: &split.TrafficSplit{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "canary",
					Namespace: testNamespace,
				},
				Spec: split.TrafficSplitSpec{
					Service: "b",
					Backends: []split.TrafficSplitBackend{
						{
							Service: "b-v1",
							Weight:  500,
						},
						{
							Service: "b-v2",
							Weight:  500,
						},
					},
				},
			},
			destinationHost: "b.test.maesh",
			destinationPath: "/foo",
			expected: map[string]float64{
				"Hostname: b-v1": 50,
				"Hostname: b-v2": 50,
			},
		},
		{
			desc:      "Pod A -> Service B v2/foo returns 200 0-100",
			source:    "a-tools",
			iteration: 10,
			trafficSplit: &split.TrafficSplit{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "canary",
					Namespace: testNamespace,
				},
				Spec: split.TrafficSplitSpec{
					Service: "b",
					Backends: []split.TrafficSplitBackend{
						{
							Service: "b-v1",
							Weight:  0,
						},
						{
							Service: "b-v2",
							Weight:  1000,
						},
					},
				},
			},
			destinationHost: "b.test.maesh",
			destinationPath: "/foo",
			expected: map[string]float64{
				"Hostname: b-v1": 0,
				"Hostname: b-v2": 100,
			},
		},
		{
			desc:      "Pod A -> Service B v2/foo returns 200 100-0",
			source:    "a-tools",
			iteration: 10,
			trafficSplit: &split.TrafficSplit{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "canary",
					Namespace: testNamespace,
				},
				Spec: split.TrafficSplitSpec{
					Service: "b",
					Backends: []split.TrafficSplitBackend{
						{
							Service: "b-v1",
							Weight:  1000,
						},
						{
							Service: "b-v2",
							Weight:  0,
						},
					},
				},
			},
			destinationHost: "b.test.maesh",
			destinationPath: "/foo",
			expected: map[string]float64{
				"Hostname: b-v1": 100,
				"Hostname: b-v2": 0,
			},
		},
	}

	for _, test := range testCases {
		var trafficSplit *split.TrafficSplit
		if test.trafficSplit != nil {
			trafficSplit, err = s.client.SmiSplitClient.SplitV1alpha2().TrafficSplits(testNamespace).Create(test.trafficSplit)
			c.Assert(err, checker.IsNil)

			err = s.client.KubeClient.CoreV1().Services(testNamespace).Delete("b", &metav1.DeleteOptions{})
			c.Assert(err, checker.IsNil)
			s.createResources(c, "resources/smi/traffic-split", 10*time.Second)
		}

		argSlice := []string{
			"exec", "-i", test.source, "-n", testNamespace, "--", "curl", "-v", test.destinationHost + test.destinationPath, "--max-time", "5",
		}

		c.Log(test.desc)

		err := s.try.WaitFunction(func() error {
			percentageResult := make(map[string]float64)
			for i := 0; i < test.iteration; i++ {
				data, err := s.waitKubectlExecCommandReturn(c, argSlice)
				if err != nil {
					return err
				}
				result := s.getLineContent(data)
				if result == "" {
					c.Log(data)
				}
				percentageResult[result]++
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
			err := s.client.SmiSplitClient.SplitV1alpha2().TrafficSplits(testNamespace).Delete(trafficSplit.Name, &metav1.DeleteOptions{})
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
