package integration

import (
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
	corev1 "k8s.io/api/core/v1"
)

// ACLDisabledSuite.
type ACLDisabledSuite struct{ BaseSuite }

func (s *ACLDisabledSuite) SetUpSuite(c *check.C) {
	requiredImages := []string{
		"containous/maesh:latest",
		"containous/whoami:v1.0.1",
		"containous/whoamitcp:v0.0.2",
		"containous/whoamiudp:v0.0.1",
		"coredns/coredns:1.6.3",
	}
	s.startk3s(c, requiredImages)
	s.startAndWaitForCoreDNS(c)
	s.createResources(c, "testdata/state-table/")
	s.createResources(c, "testdata/smi/crds/")
}

func (s *ACLDisabledSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *ACLDisabledSuite) TestHTTPService(c *check.C) {
	s.createResources(c, "testdata/acl/disabled/http")
	defer s.deleteResources(c, "testdata/acl/disabled/http")
	defer s.deleteShadowServices(c)

	s.waitForPods(c, []string{"server"})

	cmd := s.startMaeshBinaryCmd(c, false, false)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	config := s.testConfigurationWithReturn(c, "testdata/acl/disabled/http.json")

	serverSvc := s.getService(c, "server")
	serverPod := s.getPod(c, "server")

	s.checkBlockAllMiddleware(c, config)
	s.checkHTTPReadinessService(c, config)
	s.checkHTTPServiceLoadBalancer(c, config, serverSvc, []*corev1.Pod{serverPod})
}

func (s *ACLDisabledSuite) TestTCPService(c *check.C) {
	s.createResources(c, "testdata/acl/disabled/tcp")
	defer s.deleteResources(c, "testdata/acl/disabled/tcp")
	defer s.deleteShadowServices(c)

	s.waitForPods(c, []string{"server"})

	cmd := s.startMaeshBinaryCmd(c, false, false)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	config := s.testConfigurationWithReturn(c, "testdata/acl/disabled/tcp.json")

	serverSvc := s.getService(c, "server")
	serverPod := s.getPod(c, "server")

	s.checkHTTPReadinessService(c, config)
	s.checkTCPServiceLoadBalancer(c, config, serverSvc, []*corev1.Pod{serverPod})
}

func (s *ACLDisabledSuite) TestUDPService(c *check.C) {
	s.createResources(c, "testdata/acl/disabled/udp")
	defer s.deleteResources(c, "testdata/acl/disabled/udp")
	defer s.deleteShadowServices(c)

	s.waitForPods(c, []string{"server"})

	cmd := s.startMaeshBinaryCmd(c, false, false)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	config := s.testConfigurationWithReturn(c, "testdata/acl/disabled/udp.json")

	serverSvc := s.getService(c, "server")
	serverPod := s.getPod(c, "server")

	s.checkHTTPReadinessService(c, config)
	s.checkUDPServiceLoadBalancer(c, config, serverSvc, []*corev1.Pod{serverPod})
}

func (s *ACLDisabledSuite) TestSplitTraffic(c *check.C) {
	s.createResources(c, "testdata/acl/disabled/traffic-split")
	defer s.deleteResources(c, "testdata/acl/disabled/traffic-split")
	defer s.deleteShadowServices(c)

	s.waitForPods(c, []string{"server-v1", "server-v2"})

	cmd := s.startMaeshBinaryCmd(c, false, false)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	config := s.testConfigurationWithReturn(c, "testdata/acl/disabled/traffic-split.json")

	s.checkBlockAllMiddleware(c, config)
	s.checkHTTPReadinessService(c, config)

	serverV1Svc := s.getService(c, "server-v1")
	serverV1Pod := s.getPod(c, "server-v1")

	s.checkHTTPServiceLoadBalancer(c, config, serverV1Svc, []*corev1.Pod{serverV1Pod})

	serverV2Svc := s.getService(c, "server-v2")
	serverV2Pod := s.getPod(c, "server-v2")

	s.checkHTTPServiceLoadBalancer(c, config, serverV2Svc, []*corev1.Pod{serverV2Pod})
}
