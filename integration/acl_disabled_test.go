package integration

import (
	"net/http"
	"time"

	"github.com/go-check/check"
	"github.com/sirupsen/logrus"
	"github.com/traefik/mesh/v2/integration/k3d"
	"github.com/traefik/mesh/v2/integration/tool"
	"github.com/traefik/mesh/v2/integration/try"
	checker "github.com/vdemeester/shakers"
)

// ACLDisabledSuite.
type ACLDisabledSuite struct {
	logger  logrus.FieldLogger
	cluster *k3d.Cluster
	tool    *tool.Tool
}

func (s *ACLDisabledSuite) SetUpSuite(c *check.C) {
	var err error

	requiredImages := []k3d.DockerImage{
		{Name: "traefik/mesh:latest", Local: true},
		{Name: "traefik:v2.3"},
		{Name: "traefik/whoami:v1.6.0"},
		{Name: "traefik/whoamitcp:v0.1.0"},
		{Name: "traefik/whoamiudp:v0.1.0"},
		{Name: "giantswarm/tiny-tools:3.9"},
	}

	s.logger = logrus.New()
	s.cluster, err = k3d.NewCluster(s.logger, masterURL, k3dClusterName,
		k3d.WithoutTraefik(),
		k3d.WithImages(requiredImages...),
	)
	c.Assert(err, checker.IsNil)

	c.Assert(s.cluster.CreateNamespace(s.logger, traefikMeshNamespace), checker.IsNil)
	c.Assert(s.cluster.CreateNamespace(s.logger, testNamespace), checker.IsNil)

	c.Assert(s.cluster.Apply(s.logger, smiCRDs), checker.IsNil)
	c.Assert(s.cluster.Apply(s.logger, "testdata/tool/tool.yaml"), checker.IsNil)
	c.Assert(s.cluster.Apply(s.logger, "testdata/traefik-mesh/controller-acl-disabled.yaml"), checker.IsNil)
	c.Assert(s.cluster.Apply(s.logger, "testdata/traefik-mesh/proxy.yaml"), checker.IsNil)
	c.Assert(s.cluster.Apply(s.logger, "testdata/traefik-mesh/dns.yaml"), checker.IsNil)

	c.Assert(s.cluster.WaitReadyPod("tool", testNamespace, 60*time.Second), checker.IsNil)
	c.Assert(s.cluster.WaitReadyDeployment("traefik-mesh-controller", traefikMeshNamespace, 60*time.Second), checker.IsNil)
	c.Assert(s.cluster.WaitReadyDaemonSet("traefik-mesh-proxy", traefikMeshNamespace, 60*time.Second), checker.IsNil)
	c.Assert(s.cluster.WaitReadyDeployment("traefik-mesh-dns", traefikMeshNamespace, 60*time.Second), checker.IsNil)

	s.tool = tool.New(s.logger, "tool", testNamespace)
}

func (s *ACLDisabledSuite) TearDownSuite(c *check.C) {
	if s.cluster != nil {
		c.Assert(s.cluster.Stop(s.logger), checker.IsNil)
	}
}

// TestHTTPService deploys an HTTP service "server" with one Pod called "server" and asserts this service is
// reachable and responses are served by this Pod.
func (s *ACLDisabledSuite) TestHTTPService(c *check.C) {
	c.Assert(s.cluster.Apply(s.logger, "testdata/acl_disabled/http"), checker.IsNil)
	defer s.cluster.Delete(s.logger, "testdata/acl_disabled/http")

	s.assertHTTPServiceReachable(c, "server-http.test.traefik.mesh:8080", "server-http", 60*time.Second)
}

// TestTCPService deploys a TCP service "server" with one Pod called "server" and asserts this service is
// reachable and that a connection has been established with this Pod.
func (s *ACLDisabledSuite) TestTCPService(c *check.C) {
	c.Assert(s.cluster.Apply(s.logger, "testdata/acl_disabled/tcp"), checker.IsNil)
	defer s.cluster.Delete(s.logger, "testdata/acl_disabled/tcp")

	s.assertTCPServiceReachable(c, "server-tcp.test.traefik.mesh", 8080, "server-tcp", 60*time.Second)
}

// TestUDPService deploys a UDP service "server" with one Pod called "server" and asserts this service is
// reachable and that a connection has been established with this Pod.
func (s *ACLDisabledSuite) TestUDPService(c *check.C) {
	c.Assert(s.cluster.Apply(s.logger, "testdata/acl_disabled/udp"), checker.IsNil)
	defer s.cluster.Delete(s.logger, "testdata/acl_disabled/udp")

	s.assertUDPServiceReachable(c, "server-udp.test.traefik.mesh", 8080, "server-udp", 60*time.Second)
}

// TestSplitTraffic deploys an HTTP service "server" and a TrafficSplit attached to it configured to distribute equally
// the load between two service "server-v1" and "server-v2", each one having a Pod with the same name. This test ensure
// both Pods are reachable through the service "server".
func (s *ACLDisabledSuite) TestSplitTraffic(c *check.C) {
	c.Assert(s.cluster.Apply(s.logger, "testdata/acl_disabled/traffic-split"), checker.IsNil)
	defer s.cluster.Delete(s.logger, "testdata/acl_disabled/traffic-split")

	s.assertHTTPServiceReachable(c, "server-split.test.traefik.mesh:8080", "server-v1", 60*time.Second)
}

func (s *ACLDisabledSuite) assertHTTPServiceReachable(c *check.C, url, expectedHostname string, timeout time.Duration) {
	s.logger.Infof("Asserting HTTP service is reachable on %q and Pod %q has handled the request", url, expectedHostname)

	err := try.Retry(func() error {
		return s.tool.Curl(url, nil,
			try.StatusCodeIs(http.StatusOK),
			try.BodyContains("Hostname: "+expectedHostname),
		)
	}, timeout)
	c.Assert(err, checker.IsNil)
}

func (s *ACLDisabledSuite) assertTCPServiceReachable(c *check.C, url string, port int, expectedHostname string, timeout time.Duration) {
	s.logger.Infof("Asserting TCP service is reachable on '%s:%d' and a connection with Pod %q is established", url, port, expectedHostname)

	err := try.Retry(func() error {
		return s.tool.Netcat(url, port, false, try.StringContains("Hostname: "+expectedHostname))
	}, timeout)
	c.Assert(err, checker.IsNil)
}

func (s *ACLDisabledSuite) assertUDPServiceReachable(c *check.C, url string, port int, expectedHostname string, timeout time.Duration) {
	s.logger.Infof("Asserting UDP service is reachable on '%s:%d' and a connection with Pod %q is established", url, port, expectedHostname)

	err := try.Retry(func() error {
		return s.tool.Netcat(url, port, true, try.StringContains("Hostname: "+expectedHostname))
	}, timeout)
	c.Assert(err, checker.IsNil)
}
