package integration

import (
	"os"
	"time"

	"github.com/containous/i3o/integration/try"
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StartI3oSuite
type StartI3oSuite struct{ BaseSuite }

func (s *StartI3oSuite) SetUpSuite(c *check.C) {
	err := s.startk3s(c)
	c.Assert(err, checker.IsNil)
	s.waitForCoreDNSStarted(c)
	c.Assert(os.Setenv("KUBECONFIG", kubeConfigPath), checker.IsNil)
}

func (s *StartI3oSuite) TearDownSuite(c *check.C) {
	s.stopComposeProject()
}

func (s *StartI3oSuite) TestSimpleStart(c *check.C) {
	s.installHelmI3o(c)
	s.waitForI3oControllerStarted(c)
	s.startWhoami(c)

	// Check that ingressroutetcps is created for the whoami service
	err := s.try.ListIngressRouteTCPs("whoami", 20*time.Second, try.HasIngressRouteTCPListLength(1), try.HasNamesIngressRouteTCPList(try.List{"whoami-whoami"}))
	c.Assert(err, checker.IsNil)

	// Check that ingressroutes is created for the whoami-http service
	err = s.try.ListIngressRoutes("whoami", 20*time.Second, try.HasIngressRouteListLength(1), try.HasNamesIngressRouteList(try.List{"whoami-whoami-http"}))
	c.Assert(err, checker.IsNil)

	// Get the whoami service in whoami namespace
	service, err := s.clients.KubeClient.CoreV1().Services("whoami").Get("whoami", metav1.GetOptions{})
	c.Assert(err, checker.IsNil)
	// Add a fake port to the service
	service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{Name: "test-update", Port: 90})
	// Update the service
	_, err = s.clients.KubeClient.CoreV1().Services("whoami").Update(service)
	c.Assert(err, checker.IsNil)

	// Check that ingressroutetcs is updates for the whoami service.
	err = s.try.ListIngressRouteTCPs("whoami", 60*time.Second, try.HasIngressRouteTCPListLength(2), try.HasNamesIngressRouteTCPList(try.List{"whoami-whoami-5000", "whoami-whoami-5001"}))
	c.Assert(err, checker.IsNil)
}
