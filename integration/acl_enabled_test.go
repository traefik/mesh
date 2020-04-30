package integration

import (
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
	corev1 "k8s.io/api/core/v1"
)

// ACLEnabledSuite
type ACLEnabledSuite struct{ BaseSuite }

func (s *ACLEnabledSuite) SetUpSuite(c *check.C) {
	requiredImages := []string{
		"containous/maesh:latest",
		"containous/whoami:v1.0.1",
		"containous/whoamitcp:v0.0.2",
		"coredns/coredns:1.6.3",
		"giantswarm/tiny-tools:3.9",
	}
	s.startk3s(c, requiredImages)
	s.startAndWaitForCoreDNS(c)
	err := s.installHelmMaesh(c, true, false)
	c.Assert(err, checker.IsNil)
	s.waitForMaeshControllerStarted(c)
}

func (s *ACLEnabledSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *ACLEnabledSuite) TestTrafficTargetWithACL(c *check.C) {
	s.testTrafficTarget(false, true, c)
}

// For the sake of BC, we need be check if the SMI option is handle correctly.
func (s *ACLEnabledSuite) TestTrafficTargetWithSMI(c *check.C) {
	s.testTrafficTarget(true, false, c)
}

func (s *ACLEnabledSuite) testTrafficTarget(smi, acl bool, c *check.C) {
	s.createResources(c, "resources/acl/enabled/traffic-target")
	defer s.deleteResources(c, "resources/acl/enabled/traffic-target")
	defer s.deleteShadowServices(c)

	s.waitForPods(c, []string{"client-a", "client-b", "server"})

	cmd := s.startMaeshBinaryCmd(c, smi, acl)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	config := s.testConfigurationWithReturn(c, "resources/acl/enabled/traffic-target.json")

	svc := s.getService(c, "server")
	tt := s.getTrafficTarget(c, "traffic-target")
	serverPod := s.getPod(c, "server")
	clientAPod := s.getPod(c, "client-a")

	s.checkBlockAllMiddleware(c, config)
	s.checkHTTPReadinessService(c, config)
	s.checkTrafficTargetLoadBalancer(c, config, tt, svc, []*corev1.Pod{serverPod})
	s.checkTrafficTargetWhitelistDirect(c, config, tt, svc, []*corev1.Pod{clientAPod})
}

func (s *ACLEnabledSuite) TestTrafficSplitACLEnable(c *check.C) {
	s.testTrafficSplit(false, true, c)
}

// For the sake of BC, we need be check if the SMI option is handle correctly.
func (s *ACLEnabledSuite) TestTrafficSplitSMIEnable(c *check.C) {
	s.testTrafficSplit(true, false, c)
}

func (s *ACLEnabledSuite) testTrafficSplit(smi, acl bool, c *check.C) {
	s.createResources(c, "resources/acl/enabled/traffic-split")
	defer s.deleteResources(c, "resources/acl/enabled/traffic-split")
	defer s.deleteShadowServices(c)

	s.waitForPods(c, []string{"client-a", "client-b", "server-v1", "server-v2"})

	cmd := s.startMaeshBinaryCmd(c, smi, acl)
	err := cmd.Start()

	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	config := s.testConfigurationWithReturn(c, "resources/acl/enabled/traffic-split.json")

	s.checkBlockAllMiddleware(c, config)
	s.checkHTTPReadinessService(c, config)

	tt := s.getTrafficTarget(c, "traffic-target")
	ts := s.getTrafficSplit(c, "traffic-split")
	clientAPod := s.getPod(c, "client-a")
	serverSvc := s.getService(c, "server")

	s.checkTrafficSplitWhitelistDirect(c, config, ts, serverSvc, []*corev1.Pod{clientAPod})

	serverV1Svc := s.getService(c, "server-v1")
	serverV1Pod := s.getPod(c, "server-v1")

	s.checkTrafficTargetLoadBalancer(c, config, tt, serverV1Svc, []*corev1.Pod{serverV1Pod})
	s.checkTrafficTargetWhitelistDirect(c, config, tt, serverV1Svc, []*corev1.Pod{clientAPod})
	s.checkTrafficTargetWhitelistIndirect(c, config, tt, serverV1Svc, []*corev1.Pod{clientAPod})

	serverV2Svc := s.getService(c, "server-v2")
	serverV2Pod := s.getPod(c, "server-v2")

	s.checkTrafficTargetLoadBalancer(c, config, tt, serverV2Svc, []*corev1.Pod{serverV2Pod})
	s.checkTrafficTargetWhitelistDirect(c, config, tt, serverV2Svc, []*corev1.Pod{clientAPod})
	s.checkTrafficTargetWhitelistIndirect(c, config, tt, serverV2Svc, []*corev1.Pod{clientAPod})
}
