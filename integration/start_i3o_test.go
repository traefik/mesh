package integration

import (
	"os"
	"time"

	traefikv1alpha1 "github.com/containous/traefik/pkg/provider/kubernetes/crd/traefik/v1alpha1"
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	var err error
	var ingressRouteTCPList *traefikv1alpha1.IngressRouteTCPList
	// Check that ingressroutetcps is created for the whoami service
	ingressRouteTCPList, err = s.clients.CrdClient.TraefikV1alpha1().IngressRouteTCPs("whoami").List(v1.ListOptions{})
	c.Assert(err, checker.IsNil)
	c.Assert(ingressRouteTCPList, checker.NotNil)
	c.Assert(len(ingressRouteTCPList.Items), checker.Equals, 1)

	c.Assert(ingressRouteTCPList.Items[0].Name, checker.Contains, "whoami-whoami")

	var ingressRouteList *traefikv1alpha1.IngressRouteList
	// Check that ingressroutes is created for the whoami-http service
	ingressRouteList, err = s.clients.CrdClient.TraefikV1alpha1().IngressRoutes("whoami").List(v1.ListOptions{})
	c.Assert(err, checker.IsNil)
	c.Assert(ingressRouteList, checker.NotNil)
	c.Assert(len(ingressRouteList.Items), checker.Equals, 1)

	c.Assert(ingressRouteList.Items[0].Name, checker.Contains, "whoami-whoami-http")

	// Get the whoami service in whoami namespace
	service, err := s.clients.KubeClient.CoreV1().Services("whoami").Get("whoami", v1.GetOptions{})
	c.Assert(err, checker.IsNil)
	// Add a fake port to the service
	service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{Name: "test-update", Port: 90})
	// Update the service
	_, err = s.clients.KubeClient.CoreV1().Services("whoami").Update(service)
	c.Assert(err, checker.IsNil)

	// FIXME remove
	time.Sleep(10 * time.Second)
	// Check that ingressroutetcs is updates for the whoami service.
	ingressRouteTCPList, err = s.clients.CrdClient.TraefikV1alpha1().IngressRouteTCPs("whoami").List(v1.ListOptions{})
	c.Assert(err, checker.IsNil)
	c.Assert(ingressRouteTCPList, checker.NotNil)
	c.Assert(len(ingressRouteTCPList.Items), checker.Equals, 2)

	c.Assert(ingressRouteTCPList.Items[0].Name, checker.Contains, "whoami-whoami")
}
