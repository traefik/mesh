package integration

import (
	"os"

	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
	corev1 "k8s.io/api/core/v1"
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

	// Get the whoami service in whoami namespace
	service, exists, err := s.client.GetService("whoami", "whoami")
	c.Assert(err, checker.IsNil)
	c.Assert(exists, checker.True)
	// Add a fake port to the service
	service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{Name: "test-update", Port: 90})
	// Update the service
	_, err = s.client.UpdateService(service)
	c.Assert(err, checker.IsNil)

}
